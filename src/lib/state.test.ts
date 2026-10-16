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
    reject: "" as string, // when set, POST /accounts fails with this 400 message
    refuse: "" as string, // when set, writes to this path fail with a 400 "the server said no"
    gone: "" as string, // when set, GET /threads/<gone> is a 404
    emit: undefined as undefined | (() => void), // sends a change event, as the engine's stream would
  };
});

vi.mock("./engine", () => {
  class EngineError extends Error {
    constructor(
      public status: number,
      message: string,
    ) {
      super(message);
    }
  }
  const api = async (path: string, init: RequestInit = {}) => {
    if (engine.fail) throw new Error("engine down");
    if (path === "/accounts" && init.method === "POST" && engine.reject) throw new EngineError(400, engine.reject);
    if (engine.refuse && path.startsWith(engine.refuse) && init.method) throw new EngineError(400, "the server said no");
    if (engine.gone && path === `/threads/${engine.gone}`) throw new EngineError(404, "not found");
    engine.calls.push({ path, method: init.method ?? "GET", body: init.body ? JSON.parse(init.body as string) : undefined });
    if (path.startsWith("/threads?") && engine.slow) return new Promise((resolve) => engine.lists.push(resolve));
    if (path.startsWith("/threads/") && !path.endsWith("/move")) return structuredClone(engine.thread);
    if (path === "/accounts") return [{ id: "a2", name: "Work", email: "you@work.example", provider: "imap", color: "" }];
    return init.method && init.method !== "GET" ? undefined : [];
  };
  return {
    api,
    send: (path: string, method: string, body: unknown) => api(path, { method, body: JSON.stringify(body) }),
    EngineError,
    follow: (onChange: () => void) => {
      engine.emit = onChange;
      return new Promise<void>(() => {}); // stays connected
    },
  };
});

let s: typeof import("./state.svelte");
beforeEach(async () => {
  vi.resetModules();
  Object.assign(engine, { calls: [], fail: false, slow: false, lists: [], reject: "", refuse: "", gone: "" });
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

describe("composing", () => {
  it("reply goes to the last sender, reply all to everyone but you", async () => {
    await s.reply("t1");
    expect(s.ui.draft).toMatchObject({ accountId: "a2", threadId: "t1", to: '"Maya Chen" <maya@work.example>', subject: "Re: Inboxed design review" });
    expect(s.ui.draft.body).toMatch(/Maya Chen wrote:\n> Done\.$/);
    await s.reply("t1", true);
    expect(s.ui.draft.to).toBe('"Maya Chen" <maya@work.example>, "Daniel Okafor" <daniel@work.example>');
    await s.forward("t1");
    expect(s.ui.draft).toMatchObject({ to: "", subject: "Fwd: Inboxed design review" });
    expect(s.ui.draft.threadId).toBeUndefined(); // a forward starts a new conversation
    expect(s.ui.draft.body).toContain("Forwarded message");
  });

  it("send posts the draft, closes the panel and reloads", async () => {
    await s.reply("t1");
    s.ui.draft.body = "Thanks!" + s.ui.draft.body;
    expect(await s.sendDraft()).toBe(null);
    expect(writes()).toEqual([{ path: "/send", method: "POST", body: { ...s.ui.draft } }]);
    expect(s.ui.composing).toBe(false);
    expect(engine.calls.some((c) => c.path === "/mailboxes")).toBe(true);
  });

  it("a refused send keeps the panel open with the server's reason", async () => {
    s.compose({ to: "nobody@example.com" });
    engine.refuse = "/send";
    expect(await s.sendDraft()).toBe("the server said no");
    expect(s.ui.composing).toBe(true);
  });

  it("closing an unchanged reply saves nothing; closing after typing saves a draft", async () => {
    await s.reply("t1");
    expect(await s.closeCompose()).toBe(null);
    expect(writes()).toEqual([]);
    s.compose();
    s.ui.draft.subject = "Plans";
    expect(await s.closeCompose()).toBe(null);
    expect(writes()).toEqual([{ path: "/drafts", method: "PUT", body: { ...s.ui.draft } }]);
    expect(s.ui.composing).toBe(false);
  });

  it("closing keeps unsaved text when the draft can't be saved", async () => {
    s.compose();
    s.ui.draft.body = "Half a thought";
    engine.refuse = "/drafts";
    expect(await s.closeCompose()).toBe("the server said no");
    expect(s.ui.composing).toBe(true);
    expect(s.ui.draft.body).toBe("Half a thought");
  });

  it("opening another message while the panel has changes saves them first", async () => {
    s.compose();
    s.ui.draft.body = "A long thought";
    await s.compose({ to: "sam@x" });
    expect(writes()).toEqual([{ path: "/drafts", method: "PUT", body: expect.objectContaining({ body: "A long thought" }) }]);
    expect(s.ui.draft.to).toBe("sam@x");
  });

  it("if those changes can't be saved, the open panel stays as it was", async () => {
    s.compose();
    s.ui.draft.body = "Keep me";
    engine.refuse = "/drafts";
    await s.compose({ to: "sam@x" });
    expect(s.ui.draft.body).toBe("Keep me");
    expect(s.ui.composeError).toBe("the server said no");
  });

  it("closing with no account to save to keeps the text", async () => {
    s.compose();
    s.ui.draft.accountId = "";
    s.ui.draft.body = "Nowhere to go";
    expect(await s.closeCompose()).toMatch(/Add an account/);
    expect(s.ui.composing).toBe(true);
  });

  it("reply answers the last message that isn't a draft", async () => {
    const mine = { ...engine.thread.messages[0], id: "m3", date: "2026-10-16T09:00:00.000Z", body: "Draft text", draft: true };
    engine.thread.messages.push(mine);
    try {
      await s.reply("t1");
      expect(s.ui.draft.to).toBe('"Maya Chen" <maya@work.example>');
      expect(s.ui.draft.body).toMatch(/> Done\.$/);
    } finally {
      engine.thread.messages.pop();
    }
  });

  it("discard deletes the saved draft", async () => {
    s.compose({ draftId: "a2/x@y" });
    await s.discardDraft();
    expect(writes()).toEqual([{ path: "/drafts/a2%2Fx%40y", method: "DELETE", body: undefined }]);
    expect(s.ui.composing).toBe(false);
  });

  it("edit draft resumes it in the panel", async () => {
    await s.openThread("t1");
    s.editDraft({ ...engine.thread.messages[1], subject: "Re: Inboxed design review", draft: true });
    expect(s.ui.draft).toMatchObject({ accountId: "a2", draftId: "m2", threadId: "t1", to: '"You" <you@work.example>', body: "Done." });
    expect(s.ui.composing).toBe(true);
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

  it("a flag change that fails to save is undone, so the screen never shows unsaved state", async () => {
    const { messages, ...summary } = structuredClone(engine.thread);
    s.data.list = [{ ...summary, starred: false }];
    engine.fail = true;
    await s.toggleStar("t1");
    expect(s.data.list[0].starred).toBe(false);
    expect(s.ui.error).not.toBe("");
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

describe("accounts", () => {
  const settings = {
    name: "Home", email: "you@example.com", username: "", password: "secret",
    imap: { host: "imap.example.com", port: 993, security: "tls" as const },
    smtp: { host: "smtp.example.com", port: 465, security: "tls" as const },
  };

  it("adding an account posts the settings and reloads the accounts", async () => {
    expect(await s.addAccount(settings)).toBe(null);
    expect(writes()[0]).toEqual({ path: "/accounts", method: "POST", body: settings });
    expect(engine.calls.some((c) => c.path === "/accounts" && c.method === "GET")).toBe(true);
  });

  it("a refused account returns the engine's message for the form and leaves the status bar alone", async () => {
    engine.reject = "imap.example.com didn't accept the username or password.";
    expect(await s.addAccount(settings)).toBe(engine.reject);
    expect(s.ui.error).toBe("");
  });

  it("removing the selected account goes back to all inboxes", async () => {
    s.ui.accountId = "a2";
    await s.removeAccount("a2");
    expect(writes()).toContainEqual({ path: "/accounts/a2", method: "DELETE", body: undefined });
    expect(s.ui.accountId).toBe("all");
  });

  it("Sync now asks the engine to sync the account", async () => {
    await s.syncNow("a2");
    expect(writes()).toContainEqual({ path: "/accounts/a2/sync", method: "POST", body: undefined });
  });
});

describe("ids in URLs", () => {
  it("escapes thread ids, which contain a slash for IMAP mail", async () => {
    await s.openThread("acc-1/d1@example.com");
    expect(engine.calls.some((c) => c.path === "/threads/acc-1%2Fd1%40example.com")).toBe(true);
  });
});

describe("removing an account", () => {
  it("closes an open thread of that account even when another view is selected", async () => {
    await s.openThread("t1"); // t1 belongs to a2
    s.ui.accountId = "all";
    await s.removeAccount("a2");
    expect(s.ui.threadId).toBe(null);
    expect(s.ui.error).toBe("");
  });
});

describe("live updates", () => {
  it("an event reloads at once, and a burst after it becomes one more reload a second later", async () => {
    engine.emit?.();
    engine.emit?.();
    engine.emit?.();
    await new Promise((r) => setTimeout(r, 300));
    expect(engine.calls.filter((c) => c.path === "/accounts").length).toBe(1);
    await new Promise((r) => setTimeout(r, 1000));
    expect(engine.calls.filter((c) => c.path === "/accounts").length).toBe(2);
  });
});

describe("an open thread that's gone", () => {
  it("closes quietly on the next reload (sent draft, deleted on another device)", async () => {
    await s.openThread("t1");
    engine.gone = "t1";
    await s.markAllRead("all", "inbox");
    expect(s.ui.threadId).toBe(null);
    expect(s.data.thread).toBe(null);
    expect(s.ui.error).toBe("");
  });
});
