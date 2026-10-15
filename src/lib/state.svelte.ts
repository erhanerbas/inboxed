import { accounts, mailboxes, threads } from "./mock";
import { threadsFor } from "./views";

/** All mail data. Phase 1 seeds it from mock.ts; Phase 2 loads it from the engine. */
export const data = $state({ accounts, mailboxes, threads });

const SIDEBAR_KEY = "inboxed.sidebarCollapsed";
function storedFlag(key: string): boolean {
  try {
    return localStorage.getItem(key) === "1";
  } catch {
    return false; // no storage (tests, private contexts): fall back to the default
  }
}

/** What the user is looking at. */
export const ui = $state({
  accountId: "all", // "all" = unified view across accounts
  mailboxKey: "inbox", // a MailboxRole in the unified view, a mailbox id otherwise
  threadId: null as string | null,
  query: "",
  composing: false,
  draft: { to: "", subject: "" },
  addingAccount: false,
  sidebarCollapsed: storedFlag(SIDEBAR_KEY),
});

export function selectAccount(id: string) {
  ui.accountId = id;
  ui.mailboxKey =
    id === "all" ? "inbox" : (data.mailboxes.find((m) => m.accountId === id && m.role === "inbox")?.id ?? "");
  ui.threadId = null;
}

export function selectMailbox(key: string) {
  ui.mailboxKey = key;
  ui.threadId = null;
}

export function toggleSidebar() {
  ui.sidebarCollapsed = !ui.sidebarCollapsed;
  try {
    localStorage.setItem(SIDEBAR_KEY, ui.sidebarCollapsed ? "1" : "0");
  } catch {
    // Not persisted; the toggle still works for this session.
  }
}

const find = (id: string) => data.threads.find((t) => t.id === id);

// ponytail: recounts every mailbox on each change; fine for mock data, the engine supplies counts in Phase 2.
function recount() {
  for (const m of data.mailboxes) m.unread = data.threads.filter((t) => t.unread && t.mailboxIds.includes(m.id)).length;
}

export function setUnread(id: string, unread: boolean) {
  const thread = find(id);
  if (!thread || thread.unread === unread) return;
  thread.unread = unread;
  recount();
}

export function openThread(id: string) {
  ui.threadId = id;
  setUnread(id, false);
}

export function toggleStar(id: string) {
  const thread = find(id);
  if (thread) thread.starred = !thread.starred;
}

/** Marks every thread in a sidebar entry as read (same accountId/key convention as `ui`). */
export function markAllRead(accountId: string, key: string) {
  for (const t of threadsFor(data.threads, data.mailboxes, accountId, key)) t.unread = false;
  recount();
}

/** Re-files a thread: drops it from its inbox (archive) or from everything (trash) and adds the role mailbox. */
function fileInto(id: string, role: "archive" | "trash") {
  const thread = find(id);
  const target = data.mailboxes.find((m) => m.accountId === thread?.accountId && m.role === role);
  if (!thread || !target) return;
  const inbox = data.mailboxes.find((m) => m.accountId === thread.accountId && m.role === "inbox")?.id;
  const keep = role === "trash" ? [] : thread.mailboxIds.filter((m) => m !== inbox);
  thread.mailboxIds = [...keep, target.id];
  if (ui.threadId === id) ui.threadId = null;
  recount();
}

export const archive = (id: string) => fileInto(id, "archive");
export const trash = (id: string) => fileInto(id, "trash");

/** Undoes archive and trash: back to the account's inbox, other folders kept. */
export function moveToInbox(id: string) {
  const thread = find(id);
  const boxes = data.mailboxes.filter((m) => m.accountId === thread?.accountId);
  const inbox = boxes.find((m) => m.role === "inbox");
  if (!thread || !inbox) return;
  const filedAway = boxes.filter((m) => m.role === "archive" || m.role === "trash").map((m) => m.id);
  thread.mailboxIds = [...thread.mailboxIds.filter((m) => !filedAway.includes(m) && m !== inbox.id), inbox.id];
  recount();
}

export function compose(draft = { to: "", subject: "" }) {
  ui.draft = draft;
  ui.composing = true;
}

const prefixed = (prefix: string, subject: string) =>
  subject.toLowerCase().startsWith(prefix.toLowerCase()) ? subject : `${prefix} ${subject}`;

export function reply(id: string, all = false) {
  const thread = find(id);
  if (!thread) return;
  const me = data.accounts.find((a) => a.id === thread.accountId)?.email;
  const last = thread.messages[thread.messages.length - 1];
  const to = all ? thread.participants : last.from.email === me ? last.to : [last.from];
  compose({ to: to.map((a) => a.email).join(", "), subject: prefixed("Re:", thread.subject) });
}

export function forward(id: string) {
  const thread = find(id);
  if (thread) compose({ to: "", subject: prefixed("Fwd:", thread.subject) });
}
