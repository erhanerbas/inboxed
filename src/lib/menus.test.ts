import { beforeEach, describe, expect, it, vi } from "vitest";

let s: typeof import("./state.svelte");
let m: typeof import("./menus");
beforeEach(async () => {
  vi.resetModules();
  s = await import("./state.svelte");
  m = await import("./menus");
});

const thread = (id: string) => s.data.threads.find((t) => t.id === id)!;
const labels = (entries: import("./menus").MenuEntry[]) => entries.map((e) => (e === "separator" ? "—" : e.text));
const key = (k: string, mods: Partial<KeyboardEvent> = {}) =>
  ({ key: k, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...mods }) as KeyboardEvent;

describe("threadMenu", () => {
  it("offers state-aware actions for an inbox thread", () => {
    expect(labels(m.threadMenu(thread("t1"), true))).toEqual([
      "Open", "—", "Reply", "Reply All", "Forward", "—", "Mark as Read", "Star", "—", "Archive", "Move to Trash",
    ]);
  });

  it("swaps Archive for Move to Inbox once a thread is archived", () => {
    s.archive("t3");
    expect(labels(m.threadMenu(thread("t3")))).toEqual([
      "Reply", "Reply All", "Forward", "—", "Mark as Unread", "Unstar", "—", "Move to Inbox", "Move to Trash",
    ]);
  });

  it("menu actions change the thread", () => {
    const markRead = m.threadMenu(thread("t1")).find((e) => e !== "separator" && e.text === "Mark as Read");
    (markRead as Exclude<import("./menus").MenuEntry, "separator">).action();
    expect(thread("t1").unread).toBe(false);
  });
});

describe("runShortcut (non-mac: Ctrl is the command key)", () => {
  it("runs the matching command on the thread", () => {
    expect(m.runShortcut(key("U", { ctrlKey: true, shiftKey: true }), thread("t3"))).toBe(true);
    expect(thread("t3").unread).toBe(true);
    expect(m.runShortcut(key("Backspace"), thread("t3"))).toBe(true);
    expect(thread("t3").mailboxIds).toEqual(["a1-trash"]);
  });

  it("ignores keys that aren't commands, and missing threads", () => {
    expect(m.runShortcut(key("u"), thread("t3"))).toBe(false);
    expect(m.runShortcut(key("U", { ctrlKey: true, shiftKey: true }), undefined)).toBe(false);
  });
});
