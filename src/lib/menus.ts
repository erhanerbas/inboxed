// Right-click menus and keyboard shortcuts share one command list, so every menu action has a
// keyboard path and every shortcut shown in a menu really works.
import {
  archive,
  data,
  forward,
  markAllRead,
  moveToInbox,
  openThread,
  reply,
  setUnread,
  toggleStar,
  trash,
  ui,
} from "./state.svelte";
import type { ThreadSummary as Thread } from "./types";

export type MenuEntry = { text: string; action: () => void; accelerator?: string; enabled?: boolean } | "separator";

type Shortcut = { key: string; mod?: boolean; shift?: boolean; ctrl?: boolean };
type Command = {
  label: (t: Thread) => string;
  shortcut?: Shortcut;
  when?: (t: Thread) => boolean;
  run: (t: Thread) => void;
};

const isMac = () => typeof document !== "undefined" && document.documentElement.dataset.platform === "mac";
const inRole = (t: Thread, role: string) =>
  data.mailboxes.some((m) => m.role === role && t.mailboxIds.includes(m.id));
const filedAway = (t: Thread) => inRole(t, "archive") || inRole(t, "trash");

// Shortcuts follow Apple Mail.
const threadCommands: (Command | "separator")[] = [
  { label: () => "Reply", shortcut: { key: "R", mod: true }, run: (t) => reply(t.id) },
  { label: () => "Reply All", shortcut: { key: "R", mod: true, shift: true }, run: (t) => reply(t.id, true) },
  { label: () => "Forward", shortcut: { key: "F", mod: true, shift: true }, run: (t) => forward(t.id) },
  "separator",
  {
    label: (t) => (t.unread ? "Mark as Read" : "Mark as Unread"),
    shortcut: { key: "U", mod: true, shift: true },
    run: (t) => setUnread(t.id, !t.unread),
  },
  { label: (t) => (t.starred ? "Unstar" : "Star"), shortcut: { key: "L", mod: true, shift: true }, run: (t) => toggleStar(t.id) },
  "separator",
  { label: () => "Archive", shortcut: { key: "A", mod: true, ctrl: true }, when: (t) => !filedAway(t), run: (t) => archive(t.id) },
  { label: () => "Move to Inbox", when: filedAway, run: (t) => moveToInbox(t.id) },
  { label: () => "Move to Trash", shortcut: { key: "Backspace" }, when: (t) => !inRole(t, "trash"), run: (t) => trash(t.id) },
];

// Ctrl+Cmd combinations only exist on macOS.
const available = (s: Shortcut) => !(s.ctrl && s.mod && !isMac());

function accelerator(s: Shortcut): string | undefined {
  if (!available(s)) return undefined;
  return [s.ctrl && "Ctrl", s.shift && "Shift", s.mod && "CmdOrCtrl", s.key].filter(Boolean).join("+");
}

function matches(s: Shortcut, e: KeyboardEvent): boolean {
  if (!available(s) || e.altKey) return false;
  const key = s.key === "Backspace" ? e.key === "Backspace" || e.key === "Delete" : e.key.toLowerCase() === s.key.toLowerCase();
  const mod = isMac() ? e.metaKey : e.ctrlKey;
  const ctrl = isMac() ? e.ctrlKey === !!s.ctrl : true;
  return key && mod === !!s.mod && e.shiftKey === !!s.shift && ctrl;
}

const applies = (c: Command, t: Thread) => !c.when || c.when(t);

export function threadMenu(t: Thread, inList = false): MenuEntry[] {
  const entries: MenuEntry[] = inList ? [{ text: "Open", action: () => openThread(t.id) }, "separator"] : [];
  for (const c of threadCommands) {
    if (c === "separator") entries.push(c);
    else if (applies(c, t)) entries.push({ text: c.label(t), action: () => c.run(t), accelerator: c.shortcut && accelerator(c.shortcut) });
  }
  return entries;
}

/** Runs the thread command bound to this key, if any. Returns whether it handled the event. */
export function runShortcut(e: KeyboardEvent, t: Thread | undefined): boolean {
  if (!t) return false;
  for (const c of threadCommands) {
    if (c !== "separator" && c.shortcut && applies(c, t) && matches(c.shortcut, e)) {
      c.run(t);
      return true;
    }
  }
  return false;
}

/** For a sidebar entry or an account (same accountId/key convention as `ui`). */
export const markAllReadMenu = (accountId: string, key: string, unread: number): MenuEntry[] => [
  { text: "Mark All as Read", enabled: unread > 0, action: () => markAllRead(accountId, key) },
];

/** For an account in the rail: everything in its inbox read, or remove it (after a confirmation). */
export const accountMenu = (accountId: string, key: string, unread: number): MenuEntry[] => [
  ...markAllReadMenu(accountId, key, unread),
  "separator",
  { text: "Remove Account…", action: () => (ui.removingAccount = accountId) },
];
