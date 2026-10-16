import { describe, expect, it } from "vitest";
import type { Mailbox } from "./types";
import { visibleMailboxes } from "./views";

const mb = (id: string, accountId: string, role: Mailbox["role"], unread = 0): Mailbox => ({
  id, accountId, name: id, role, parentId: null, unread,
});


const boxes = [
  mb("a1-inbox", "a1", "inbox", 2),
  mb("a1-receipts", "a1", null),
  mb("a2-inbox", "a2", "inbox", 3),
  mb("a2-sent", "a2", "sent"),
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
