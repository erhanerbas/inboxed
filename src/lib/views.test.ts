import { describe, expect, it } from "vitest";
import type { Mailbox, Thread } from "./types";
import { threadsFor, visibleMailboxes } from "./views";

const mb = (id: string, accountId: string, role: Mailbox["role"], unread = 0): Mailbox => ({
  id, accountId, name: id, role, parentId: null, unread,
});

const th = (id: string, accountId: string, mailboxIds: string[], lastDate: string, subject: string): Thread => ({
  id, accountId, mailboxIds, subject, lastDate,
  snippet: "", participants: [{ name: "Maya Chen", email: "maya@work.example" }],
  unread: false, starred: false, hasAttachments: false, messages: [],
});

const boxes = [
  mb("a1-inbox", "a1", "inbox", 2),
  mb("a1-receipts", "a1", null),
  mb("a2-inbox", "a2", "inbox", 3),
  mb("a2-sent", "a2", "sent"),
];

const threads = [
  th("t1", "a1", ["a1-inbox"], "2026-10-14T09:00:00.000Z", "Flight"),
  th("t2", "a2", ["a2-inbox"], "2026-10-15T09:00:00.000Z", "Roadmap"),
  th("t3", "a1", ["a1-receipts"], "2026-10-13T09:00:00.000Z", "Receipt"),
];

describe("visibleMailboxes", () => {
  it("returns an account's own mailboxes", () => {
    expect(visibleMailboxes(boxes, "a1").map((m) => m.id)).toEqual(["a1-inbox", "a1-receipts"]);
  });

  it("merges roles across accounts in the unified view and sums unread", () => {
    expect(visibleMailboxes(boxes, "all")).toEqual([
      { id: "inbox", accountId: "all", name: "Inbox", role: "inbox", parentId: null, unread: 5 },
      { id: "sent", accountId: "all", name: "Sent", role: "sent", parentId: null, unread: 0 },
    ]);
  });
});

describe("threadsFor", () => {
  it("shows every account's inbox in the unified view, newest first", () => {
    expect(threadsFor(threads, boxes, "all", "inbox").map((t) => t.id)).toEqual(["t2", "t1"]);
  });

  it("uses the mailbox id in an account view", () => {
    expect(threadsFor(threads, boxes, "a1", "a1-receipts").map((t) => t.id)).toEqual(["t3"]);
  });

  it("shows starred threads by flag, not by mailbox membership", () => {
    const starred = [
      { ...th("s1", "a1", ["a1-inbox"], "2026-10-15T09:00:00.000Z", "Flagged"), starred: true },
      { ...th("s2", "a2", ["a2-inbox"], "2026-10-14T09:00:00.000Z", "Work flagged"), starred: true },
      th("s3", "a1", ["a1-starred"], "2026-10-13T09:00:00.000Z", "Unstarred but still filed"),
    ];
    const withStarred = [...boxes, mb("a1-starred", "a1", "starred")];
    expect(threadsFor(starred, withStarred, "all", "starred").map((t) => t.id)).toEqual(["s1", "s2"]);
    expect(threadsFor(starred, withStarred, "a1", "a1-starred").map((t) => t.id)).toEqual(["s1"]);
  });

  it("searches subject and participants case-insensitively", () => {
    expect(threadsFor(threads, boxes, "all", "inbox", "roadMAP").map((t) => t.id)).toEqual(["t2"]);
    expect(threadsFor(threads, boxes, "all", "inbox", "maya").map((t) => t.id)).toEqual(["t2", "t1"]);
  });
});
