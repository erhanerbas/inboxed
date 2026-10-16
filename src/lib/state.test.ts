import { beforeEach, describe, expect, it, vi } from "vitest";

// A fake engine: records every call, serves one thread, and can be told to fail or to answer slowly.
const engine = vi.hoisted(() => {
  const thread = {
    id: "t1", accountId: "a2", mailboxIds: ["a2-inbox"], subject: "Inboxed design review", snippet: "Done.",
    participants: [{ name: "Maya Chen", email: "maya@work.example" }, { name: "Daniel Okafor", email: "daniel@work.example" }],
    lastDate: "2026-10-15T13:25:00.000Z", unread: true, starred: false, hasAttachments: false,
    messages: [
      { id: "m1", threadId: "t1", from: { name: "You", email: "you@work.example" }, to: [{ name: "Maya Chen", email: "maya@work.example" }], cc: [], date: "2026-10-14T10:00:00.000Z", body: "Hi", attachments: [] },
      { id: "m2", threadId: "t1", from: { name: "Maya Chen", email: "maya@work.example" }, to: [{ name: "You", email: "you@work.example" }], cc: [], date: "2026-10-15T13:25:00.000Z", body: "Done.", attachments: [] },
    ],
  };
  return {
    thread,
    calls: [] as { path: string; method: string; body?: unknown }[],
    fail: false,
    lists: [] as ((v: unknown) => void)[], // pending /threads? responses when `slow` is set
    slow: false,
  };
});

vi.mock("./engine", () => {
  const api = async (path: string, init: RequestInit = {}) => {
    if (engine.fail) throw new Error("engine down");
    engine.calls.push({ path, method: init.method ?? "GET", body: init.body ? JSON.parse(init.body as string) : undefined });
    if (path.startsWith("/threads?") && engine.slow) return new Promise((resolve) => engine.lists.push(resolve));
    if (path.startsWith("/threads/") && !path.endsWith("/move")) return structuredClone(engine.thread);
    if (path === "/accounts") return [{ id: "a2", name: "Work", email: "you@work.example", provider: "outlook", color: "" }];
    return init.method && init.method !== "GET" ? undefined : [];
  };
  return { api, send: (path: string, method: string, body: unknown) => api(path, { method, body: JSON.stringify(body) }) };
});

let s: typeof import("./state.svelte");
beforeEach(async () => {
  vi.resetModules();
  Object.assign(engine, { calls: [], fail: false, slow: false, lists: [] });
  engine.thread.unread = true;
  s = await import("./state.svelte");
  await s.start();
  engine.calls.length = 0;
});

const writes = () => engine.calls.filter((c) => c.method !== "GET");

describe("opening", () => {
  it("loads the thread and marks it read on the engine", async () => {
    await s.openThread("t1");
    expect(s.data.thread?.id).toBe("t1");
    expect(writes()).toEqual([{ path: "/threads/t1", method: "PATCH", body: { unread: false } }]);
    expect(engine.calls.some((c) => c.path === "/mailboxes")).toBe(true); // counts come back from the engine
  });

  it("doesn't write when the thread is already read", async () => {
    engine.thread.unread = false;
    await s.openThread("t1");
    expect(writes()).toEqual([]);
  });
});

describe("filing", () => {
  it("archive tells the engine and closes the open thread", async () => {
    await s.openThread("t1");
    engine.calls.length = 0;
    await s.archive("t1");
    expect(writes()).toEqual([{ path: "/threads/t1/move", method: "POST", body: { to: "archive" } }]);
    expect(s.ui.threadId).toBe(null);
  });

  it("filing another thread leaves the open one open", async () => {
    await s.openThread("t1");
    await s.trash("t9");
    expect(s.ui.threadId).toBe("t1");
  });

  it("mark all as read sends the view", async () => {
    await s.markAllRead("all", "inbox");
    expect(writes()).toEqual([{ path: "/read-all", method: "POST", body: { account: "all", key: "inbox" } }]);
  });
});

describe("drafts", () => {
  it("reply goes to the last sender, reply all to everyone but you", async () => {
    await s.reply("t1");
    expect(s.ui.draft).toEqual({ to: "maya@work.example", subject: "Re: Inboxed design review" });
    await s.reply("t1", true);
    expect(s.ui.draft.to).toBe("maya@work.example, daniel@work.example");
    await s.forward("t1");
    expect(s.ui.draft).toEqual({ to: "", subject: "Fwd: Inboxed design review" });
  });
});

describe("failures and races", () => {
  it("a failed call leaves a message with a way to recover, and success clears it", async () => {
    engine.fail = true;
    await s.archive("t1");
    expect(s.ui.error).toMatch(/try again/i);
    engine.fail = false;
    await s.start();
    expect(s.ui.error).toBe("");
  });

  it("a stale list response never replaces a newer one", async () => {
    engine.slow = true;
    s.ui.query = "old";
    const first = s.showList();
    s.ui.query = "new";
    const second = s.showList();
    engine.lists[1]([{ id: "new" }]);
    engine.lists[0]([{ id: "old" }]);
    await Promise.all([first, second]);
    expect(s.data.list.map((t) => t.id)).toEqual(["new"]);
  });
});
