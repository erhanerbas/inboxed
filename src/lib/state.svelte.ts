import { accounts, mailboxes, threads } from "./mock";

/** All mail data. Phase 1 seeds it from mock.ts; Phase 2 loads it from the engine. */
export const data = $state({ accounts, mailboxes, threads });

/** What the user is looking at. */
export const ui = $state({
  accountId: "all", // "all" = unified view across accounts
  mailboxKey: "inbox", // a MailboxRole in the unified view, a mailbox id otherwise
  threadId: null as string | null,
  query: "",
  composing: false,
  addingAccount: false,
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

export function openThread(id: string) {
  ui.threadId = id;
  const thread = data.threads.find((t) => t.id === id);
  if (!thread?.unread) return;
  thread.unread = false;
  for (const m of data.mailboxes) if (thread.mailboxIds.includes(m.id)) m.unread--;
}
