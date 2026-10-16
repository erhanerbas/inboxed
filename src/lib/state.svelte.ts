import { api, send } from "./engine";
import type { Account, Mailbox, Thread, ThreadSummary } from "./types";

/** Mail data, as the engine last reported it. */
export const data = $state({
  accounts: [] as Account[],
  mailboxes: [] as Mailbox[],
  list: [] as ThreadSummary[], // the selected sidebar entry, narrowed by the search query
  thread: null as Thread | null, // the open conversation, with its messages
});

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
  error: "", // shown by the status bar in +page.svelte
});

/** Runs engine work. A failure becomes a message with a way out; it never throws. */
async function run(work: () => Promise<unknown>) {
  try {
    await work();
    ui.error = "";
  } catch (err) {
    console.error(err);
    ui.error = "Couldn't reach the mail engine. Try again, or restart Inboxed if it keeps happening.";
  }
}

let listRequest = 0;
async function loadList() {
  const n = ++listRequest;
  const params = new URLSearchParams({ account: ui.accountId, key: ui.mailboxKey, q: ui.query });
  const list = await api<ThreadSummary[]>(`/threads?${params}`);
  if (n === listRequest) data.list = list; // a newer request (fast typing in search) wins
}

async function loadThread() {
  const id = ui.threadId;
  const thread = id ? await api<Thread>(`/threads/${id}`) : null;
  if (ui.threadId === id) data.thread = thread;
}

// After any change the engine is the source of truth, unread counts included.
async function reload() {
  const [mailboxes] = await Promise.all([api<Mailbox[]>("/mailboxes"), loadList(), loadThread()]);
  data.mailboxes = mailboxes;
}

/** First load; also the status bar's "Try again". */
export const start = () =>
  run(async () => {
    data.accounts = await api<Account[]>("/accounts");
    await reload();
  });

/** Reloads the list for the current view and query (+page.svelte runs this whenever they change). */
export const showList = () => run(loadList);

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

// Flips the flags here first so the unread dot and the star respond instantly, then saves and reloads.
async function setFlags(id: string, flags: { unread?: boolean; starred?: boolean }) {
  for (const t of [...data.list, data.thread]) if (t?.id === id) Object.assign(t, flags);
  await send(`/threads/${id}`, "PATCH", flags);
  await reload();
}

export function openThread(id: string) {
  ui.threadId = id;
  return run(async () => {
    await loadThread();
    if (data.thread?.id === id && data.thread.unread) await setFlags(id, { unread: false });
  });
}

export const setUnread = (id: string, unread: boolean) => run(() => setFlags(id, { unread }));

export function toggleStar(id: string) {
  const t = data.list.find((t) => t.id === id) ?? (data.thread?.id === id ? data.thread : undefined);
  return run(() => setFlags(id, { starred: !t?.starred }));
}

export const markAllRead = (accountId: string, key: string) =>
  run(async () => {
    await send("/read-all", "POST", { account: accountId, key });
    await reload();
  });

function move(id: string, to: "archive" | "trash" | "inbox") {
  return run(async () => {
    await send(`/threads/${id}/move`, "POST", { to });
    if (to !== "inbox" && ui.threadId === id) ui.threadId = null;
    await reload();
  });
}

export const archive = (id: string) => move(id, "archive");
export const trash = (id: string) => move(id, "trash");
export const moveToInbox = (id: string) => move(id, "inbox");

export function compose(draft = { to: "", subject: "" }) {
  ui.draft = draft;
  ui.composing = true;
}

const prefixed = (prefix: string, subject: string) =>
  subject.toLowerCase().startsWith(prefix.toLowerCase()) ? subject : `${prefix} ${subject}`;
const fullThread = async (id: string) => (data.thread?.id === id ? data.thread : api<Thread>(`/threads/${id}`));

export const reply = (id: string, all = false) =>
  run(async () => {
    const thread = await fullThread(id);
    const me = data.accounts.find((a) => a.id === thread.accountId)?.email;
    const last = thread.messages[thread.messages.length - 1];
    const to = all ? thread.participants : last.from.email === me ? last.to : [last.from];
    compose({ to: to.map((a) => a.email).join(", "), subject: prefixed("Re:", thread.subject) });
  });

export const forward = (id: string) =>
  run(async () => {
    const thread = await fullThread(id);
    compose({ to: "", subject: prefixed("Fwd:", thread.subject) });
  });
