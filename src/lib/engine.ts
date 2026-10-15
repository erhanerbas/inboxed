import { invoke } from "@tauri-apps/api/core";

type Engine = { url: string; token: string };

let engine: Promise<Engine> | undefined;

/** Calls the Go engine's HTTP API. Every UI-to-engine request goes through here. */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const { url, token } = await (engine ??= invoke<Engine>("engine"));
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(url + path, { ...init, headers });
  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
  return res.json();
}
