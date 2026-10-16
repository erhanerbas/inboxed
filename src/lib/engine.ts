import { invoke } from "@tauri-apps/api/core";

type Engine = { url: string; token: string };

let engine: Promise<Engine> | undefined;

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
  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
  return (res.status === 204 ? undefined : await res.json()) as T;
}

export const send = (path: string, method: string, body: unknown) =>
  api<void>(path, { method, body: JSON.stringify(body) });
