import { invoke } from "@tauri-apps/api/core";
import type { NewMail } from "./types";

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

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const { url, token } = await (engine ??= connect());
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  if (typeof init.body === "string") headers.set("Content-Type", "application/json");
  const res = await fetch(url + path, { ...init, headers });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new EngineError(res.status, body.error ?? res.statusText);
  }
  return res;
}

/** Calls the Go engine's HTTP API. Every UI-to-engine request goes through here. */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await request(path, init);
  return (res.status === 204 ? undefined : await res.json()) as T;
}

/** A file's content from the engine (an attachment, for the preview). */
export const fetchFile = async (path: string) => (await request(path)).blob();

/** Stores a file for a message being written; resolves to its hash, which the draft names it by. */
export const upload = async (file: Blob) => (await api<{ hash: string }>("/files", { method: "POST", body: file })).hash;

export const send = (path: string, method: string, body: unknown) =>
  api<void>(path, { method, body: JSON.stringify(body) });

/** Splits a Server-Sent Events buffer into its complete events (name and data) and the unfinished rest. */
export function parseEvents(buffer: string): { events: { name: string; data: string }[]; rest: string } {
  const blocks = buffer.replace(/\r\n?/g, "\n").split("\n\n");
  const rest = blocks.pop() ?? "";
  const events = blocks.flatMap((block) => {
    const lines = block.split("\n");
    const name = lines.find((l) => l.startsWith("event: "))?.slice("event: ".length);
    const data = lines.filter((l) => l.startsWith("data: ")).map((l) => l.slice("data: ".length)).join("\n");
    return name ? [{ name, data }] : [];
  });
  return { events, rest };
}

/**
 * Follows the engine's event stream until it ends or fails: onChange for every change, onMail with each batch of
 * new mail. fetch rather than EventSource, because EventSource can't send the token (and it must never go in a URL).
 */
export async function follow(onChange: () => void, onMail: (batch: NewMail[]) => void): Promise<void> {
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
    for (const e of parsed.events) {
      if (e.name === "mail") onMail(JSON.parse(e.data));
    }
    if (parsed.events.some((e) => e.name === "change")) onChange();
  }
}
