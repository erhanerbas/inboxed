import { invoke } from "@tauri-apps/api/core";

type Engine = { url: string; token: string };

let engine: Promise<Engine> | undefined;

/** The engine answered and said no. For a 400 the message is written for people (form errors). */
export class EngineError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

// Inside Tauri the shell hands over the sidecar's address. In a plain browser (development, headless checks)
// VITE_ENGINE_URL and VITE_ENGINE_TOKEN point at `npm run dev:engine`.
function connect(): Promise<Engine> {
  if ("__TAURI_INTERNALS__" in window) return invoke<Engine>("engine");
  const { VITE_ENGINE_URL: url, VITE_ENGINE_TOKEN: token } = import.meta.env;
  return url && token
    ? Promise.resolve({ url, token })
    : Promise.reject(new Error("No engine: run inside Tauri, or start `npm run dev:engine` with .env.development.local"));
}

/** Calls the Go engine's HTTP API. Every UI-to-engine request goes through here. */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const { url, token } = await (engine ??= connect());
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  if (init.body) headers.set("Content-Type", "application/json");
  const res = await fetch(url + path, { ...init, headers });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new EngineError(res.status, body.error ?? res.statusText);
  }
  return (res.status === 204 ? undefined : await res.json()) as T;
}

export const send = (path: string, method: string, body: unknown) =>
  api<void>(path, { method, body: JSON.stringify(body) });

/** Splits a Server-Sent Events buffer into the names of its complete events and the unfinished rest. */
export function parseEvents(buffer: string): { names: string[]; rest: string } {
  const blocks = buffer.split("\n\n");
  const rest = blocks.pop() ?? "";
  const names = blocks.flatMap((block) =>
    block
      .split("\n")
      .filter((line) => line.startsWith("event: "))
      .map((line) => line.slice("event: ".length)),
  );
  return { names, rest };
}

/**
 * Follows the engine's change stream, calling onChange for every change event, until the stream ends or fails.
 * fetch rather than EventSource, because EventSource can't send the token (and it must never go in a URL).
 */
export async function follow(onChange: () => void): Promise<void> {
  const { url, token } = await (engine ??= connect());
  const res = await fetch(url + "/events", { headers: { Authorization: `Bearer ${token}` } });
  if (!res.ok || !res.body) throw new EngineError(res.status, "The engine's event stream isn't available.");
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) return;
    const parsed = parseEvents(buffer + value);
    buffer = parsed.rest;
    if (parsed.names.includes("change")) onChange();
  }
}
