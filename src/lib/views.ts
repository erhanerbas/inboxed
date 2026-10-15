import type { Mailbox, MailboxRole, Thread } from "./types";

export const ROLES: { role: NonNullable<MailboxRole>; label: string }[] = [
  { role: "inbox", label: "Inbox" },
  { role: "starred", label: "Starred" },
  { role: "drafts", label: "Drafts" },
  { role: "sent", label: "Sent" },
  { role: "archive", label: "Archive" },
  { role: "spam", label: "Spam" },
  { role: "trash", label: "Trash" },
];

/**
 * Sidebar entries. For one account: its mailboxes. For the unified view ("all"):
 * one entry per role present in any account, keyed by role, with unread summed.
 */
export function visibleMailboxes(all: Mailbox[], accountId: string): Mailbox[] {
  if (accountId !== "all") return all.filter((m) => m.accountId === accountId);
  return ROLES.filter(({ role }) => all.some((m) => m.role === role)).map(({ role, label }) => ({
    id: role,
    accountId: "all",
    name: label,
    role,
    parentId: null,
    unread: all.filter((m) => m.role === role).reduce((sum, m) => sum + m.unread, 0),
  }));
}

/** Threads in the selected sidebar entry matching the search query, newest first. */
export function threadsFor(
  threads: Thread[],
  mailboxes: Mailbox[],
  accountId: string,
  key: string,
  query = "",
): Thread[] {
  const ids = accountId === "all" ? mailboxes.filter((m) => m.role === key).map((m) => m.id) : [key];
  const q = query.trim().toLowerCase();
  const matches = (t: Thread) =>
    [t.subject, t.snippet, ...t.participants.map((p) => `${p.name} ${p.email}`)].some((s) =>
      s.toLowerCase().includes(q),
    );
  return threads
    .filter((t) => t.mailboxIds.some((id) => ids.includes(id)) && (!q || matches(t)))
    .sort((a, b) => b.lastDate.localeCompare(a.lastDate));
}
