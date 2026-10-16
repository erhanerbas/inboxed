import { api, EngineError, follow, send } from "./engine";
import { addresses, forwardBody, replyQuote } from "./format";
import type { Account, AccountSettings, Draft, Mailbox, Message, Thread, ThreadSummary } from "./types";

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
  draft: { accountId: "", to: "", cc: "", subject: "", body: "" } as Draft,
  composeError: "", // why the compose panel couldn't send or save; shown in the panel
  composeKey: 0, // a new message re-mounts the panel (focus, state)
  addingAccount: false,
  sidebarCollapsed: storedFlag(SIDEBAR_KEY),
  error: "", // shown by the status bar in +page.svelte
  removingAccount: null as string | null, // the account the remove-account confirmation is asking about
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

// A thread that's gone (its draft was sent, it was deleted on another device) closes rather than reporting an error.
async function loadThread() {
  const id = ui.threadId;
  let thread: Thread | null = null;
  if (id) {
    try {
      thread = await api<Thread>(`/threads/${encodeURIComponent(id)}`);
    } catch (err) {
      if (!(err instanceof EngineError && err.status === 404)) throw err;
    }
  }
  if (ui.threadId !== id) return;
  data.thread = thread;
  if (!thread) ui.threadId = null;
}

// After any change the engine is the source of truth, unread counts included.
async function reload() {
  const [mailboxes] = await Promise.all([api<Mailbox[]>("/mailboxes"), loadList(), loadThread()]);
  data.mailboxes = mailboxes;
}

async function refreshAll() {
  data.accounts = await api<Account[]>("/accounts");
  await reload();
}

let refreshing = false;
let missed = false;

// The first event reloads at once; a burst after it (a big import reports often) becomes at most one more reload a
// second, and reloads never overlap.
async function scheduleRefresh() {
  if (refreshing) {
    missed = true;
    return;
  }
  refreshing = true;
  do {
    missed = false;
    await run(refreshAll);
    await new Promise((r) => setTimeout(r, 1000));
  } while (missed);
  refreshing = false;
}

let following = false;

// Stays connected to the engine's change stream, reconnecting with backoff (1 s up to 30 s) after a drop, and
// reloading once on reconnect so nothing missed while away is lost.
async function followChanges() {
  if (following) return;
  following = true;
  let delay = 1000;
  for (;;) {
    const opened = Date.now();
    try {
      await follow(scheduleRefresh);
    } catch (err) {
      console.error(err);
    }
    if (Date.now() - opened > 30_000) delay = 1000; // it was up for a while: this is a new outage
    await new Promise((r) => setTimeout(r, delay));
    await new Promise((r) => setTimeout(r, delay));
    delay = Math.min(delay * 2, 30_000);
    scheduleRefresh();
  }
}

/** First load; also the status bar's "Try again". Opens the live connection once. */
export const start = () => {
  followChanges();
  return run(refreshAll);
};

/** Adds an IMAP/SMTP account. Resolves to a message for the form when the engine refuses, or null when added. */
export async function addAccount(settings: AccountSettings): Promise<string | null> {
  try {
    await api("/accounts", { method: "POST", body: JSON.stringify(settings) });
  } catch (err) {
    if (err instanceof EngineError && err.status === 400) return err.message;
    console.error(err);
    return "Couldn't reach the mail engine. Try again, or restart Inboxed if it keeps happening.";
  }
  await start();
  return null;
}

export const removeAccount = (id: string) =>
  run(async () => {
    await api(`/accounts/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (ui.accountId === id) selectAccount("all");
    else if (data.thread?.accountId === id) ui.threadId = null; // its mail is gone, so its open thread closes too
    await refreshAll();
  });

export const syncNow = (id: string) =>
  run(async () => {
    await api(`/accounts/${encodeURIComponent(id)}/sync`, { method: "POST" });
    await refreshAll();
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

// Flips the flags here first so the unread dot and the star respond instantly, then saves and reloads. If saving
// fails the flip is undone, so the screen never shows a change the engine doesn't have.
async function setFlags(id: string, flags: { unread?: boolean; starred?: boolean }) {
  const touched = [...data.list, data.thread].filter((t) => t?.id === id) as ThreadSummary[];
  const before = touched.map((t) => ({ unread: t.unread, starred: t.starred }));
  touched.forEach((t) => Object.assign(t, flags));
  try {
    await send(`/threads/${encodeURIComponent(id)}`, "PATCH", flags);
  } catch (err) {
    touched.forEach((t, i) => Object.assign(t, before[i]));
    throw err;
  }
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
    await send(`/threads/${encodeURIComponent(id)}/move`, "POST", { to });
    if (to !== "inbox" && ui.threadId === id) ui.threadId = null;
    await reload();
  });
}

export const archive = (id: string) => move(id, "archive");
export const trash = (id: string) => move(id, "trash");
export const moveToInbox = (id: string) => move(id, "inbox");

/** Accounts that can send: IMAP ones (demo accounts can't). */
export const senders = () => data.accounts.filter((a) => a.provider === "imap");

let pristine = ""; // the draft as the panel opened, so closing an untouched one saves nothing

/** Opens the panel on a new message. One already open is saved first; if it can't be, it stays as it was. */
export async function compose(draft: Partial<Draft> = {}) {
  if (ui.composing && (await closeCompose())) return;
  const from = senders().find((a) => a.id === ui.accountId) ?? senders()[0];
  ui.draft = { accountId: from?.id ?? "", to: "", cc: "", subject: "", body: "", ...draft };
  pristine = JSON.stringify(ui.draft);
  ui.composeError = "";
  ui.composeKey++;
  ui.composing = true;
}

// Why the panel couldn't send or save, for the panel to show: the engine's words for a refusal, else what to do.
function failure(err: unknown): string {
  if (err instanceof EngineError && err.status === 400) return (ui.composeError = err.message);
  if (err instanceof EngineError && err.status === 404) return (ui.composeError = "That account was removed. Choose another one.");
  console.error(err);
  return (ui.composeError = "Couldn't reach the mail engine. Try again, or restart Inboxed if it keeps happening.");
}

/** Sends the open draft. Resolves to null once sent (the panel closes), else to the reason, with the panel open. */
export async function sendDraft(): Promise<string | null> {
  try {
    await send("/send", "POST", ui.draft);
  } catch (err) {
    return failure(err);
  }
  ui.composing = false;
  await run(reload);
  return null;
}

/** Closes the panel, saving the draft first if anything was typed. If it can't be saved, the panel stays open. */
export async function closeCompose(): Promise<string | null> {
  const d = ui.draft;
  const typed = d.to || d.cc || d.subject || d.body.trim();
  if (typed && JSON.stringify(d) !== pristine) {
    if (!d.accountId) return (ui.composeError = "Add an account to save this draft, or discard it.");
    try {
      await send("/drafts", "PUT", d);
    } catch (err) {
      return failure(err);
    }
    await run(reload);
  }
  ui.composing = false;
  return null;
}

/** Closes the panel and deletes the draft it was resumed from, if any. */
export function discardDraft() {
  const id = ui.draft.draftId;
  ui.composing = false;
  if (!id) return Promise.resolve();
  return run(async () => {
    await api(`/drafts/${encodeURIComponent(id)}`, { method: "DELETE" });
    await reload();
  });
}

/** Resumes a saved draft. It stays a reply when the conversation has other messages. */
export async function editDraft(m: Message) {
  const thread = data.thread?.id === m.threadId ? data.thread : null;
  const reply = thread?.messages.some((x) => !x.draft && x.id !== m.id);
  await compose({
    accountId: thread?.accountId ?? m.id.split("/")[0],
    to: addresses(m.to),
    cc: addresses(m.cc),
    subject: m.subject,
    body: m.body,
    threadId: reply ? m.threadId : undefined,
    draftId: m.id,
  });
}

// The message a reply or forward is about: the newest that isn't a draft of mine.
const lastSent = (t: Thread) => t.messages.findLast((m) => !m.draft) ?? t.messages[t.messages.length - 1];

const prefixed = (prefix: string, subject: string) =>
  subject.toLowerCase().startsWith(prefix.toLowerCase()) ? subject : `${prefix} ${subject}`;
const fullThread = async (id: string) => (data.thread?.id === id ? data.thread : api<Thread>(`/threads/${encodeURIComponent(id)}`));

export const reply = (id: string, all = false) =>
  run(async () => {
    const thread = await fullThread(id);
    const me = data.accounts.find((a) => a.id === thread.accountId)?.email;
    const last = lastSent(thread);
    const to = all ? thread.participants : last.from.email === me ? last.to : [last.from];
    await compose({
      accountId: thread.accountId,
      threadId: thread.id,
      to: addresses(to),
      subject: prefixed("Re:", thread.subject),
      body: replyQuote(last),
    });
  });

export const forward = (id: string) =>
  run(async () => {
    const thread = await fullThread(id);
    const last = lastSent(thread);
    await compose({ accountId: thread.accountId, subject: prefixed("Fwd:", thread.subject), body: forwardBody(last, thread.subject) });
  });
