import { beforeEach, describe, expect, it, vi } from "vitest";

// Fresh copy of the mock-seeded state for every test.
let s: typeof import("./state.svelte");
beforeEach(async () => {
  vi.resetModules();
  s = await import("./state.svelte");
});

const unread = (id: string) => s.data.mailboxes.find((m) => m.id === id)!.unread;
const thread = (id: string) => s.data.threads.find((t) => t.id === id)!;

describe("read state", () => {
  it("opening an unread thread lowers its mailbox counts once", () => {
    expect(unread("a2-inbox")).toBe(2);
    s.openThread("t1");
    s.openThread("t1");
    expect(unread("a2-inbox")).toBe(1);
    expect(unread("a2-projects-inboxed")).toBe(0);
  });

  it("marking a thread unread raises the counts again", () => {
    s.setUnread("t3", true);
    expect(unread("a1-inbox")).toBe(2);
  });

  it("mark all as read clears every thread in that view", () => {
    s.markAllRead("all", "inbox");
    expect(unread("a1-inbox") + unread("a2-inbox")).toBe(0);
    expect(thread("t2").unread).toBe(false);
  });
});

describe("filing", () => {
  it("archive moves a thread from the inbox to its account's archive", () => {
    s.openThread("t2");
    s.archive("t2");
    expect(thread("t2").mailboxIds).toEqual(["a1-travel-2026", "a1-archive"]);
    expect(s.ui.threadId).toBe(null);
  });

  it("move to trash keeps only the trash mailbox", () => {
    s.trash("t4");
    expect(thread("t4").mailboxIds).toEqual(["a2-trash"]);
    expect(unread("a2-inbox")).toBe(1);
  });
});

describe("restoring", () => {
  it("move to inbox undoes archive and trash", () => {
    s.archive("t2");
    s.moveToInbox("t2");
    expect(thread("t2").mailboxIds).toEqual(["a1-travel-2026", "a1-inbox"]);
    s.trash("t4");
    s.moveToInbox("t4");
    expect(thread("t4").mailboxIds).toEqual(["a2-inbox"]);
    expect(unread("a2-inbox")).toBe(2);
  });
});

describe("drafts", () => {
  it("reply addresses the last sender and prefixes the subject once", () => {
    s.reply("t1");
    expect(s.ui.composing).toBe(true);
    expect(s.ui.draft).toEqual({ to: "maya@work.example", subject: "Re: Inboxed design review: sidebar and thread view" });
    s.reply("t5");
    expect(s.ui.draft.subject).toBe("Re: Fwd: Your receipt from Example Store for order #A-2026-10-12-000123 (3 items, shipped in two separate packages)");
  });

  it("reply all addresses everyone except you", () => {
    s.reply("t1", true);
    expect(s.ui.draft.to).toBe("maya@work.example, daniel@work.example");
  });

  it("forward leaves the recipient empty", () => {
    s.forward("t2");
    expect(s.ui.draft).toEqual({ to: "", subject: "Fwd: Your flight to Lisbon is confirmed" });
  });
});
