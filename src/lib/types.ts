export type Provider = "gmail" | "outlook" | "yahoo" | "imap";

export type Account = {
  id: string;
  name: string;
  email: string;
  provider: Provider;
  color: string; // identity color for the rail avatar and list dots
  sync?: ImportProgress;
};

export type MailboxRole = "inbox" | "starred" | "drafts" | "sent" | "archive" | "spam" | "trash" | null;

export type Mailbox = {
  id: string;
  accountId: string;
  name: string;
  role: MailboxRole; // null = user folder
  parentId: string | null;
  unread: number;
};

export type Address = { name: string; email: string };

export type Message = {
  id: string;
  threadId: string;
  from: Address;
  to: Address[];
  cc: Address[];
  date: string; // ISO 8601, UTC
  body: string; // plain text; HTML rendering arrives in Phase 6
  attachments: { name: string; size: number }[];
};

/** A row in a list: everything but the messages. */
export type ThreadSummary = {
  id: string;
  accountId: string;
  mailboxIds: string[];
  subject: string;
  snippet: string;
  participants: Address[]; // everyone except the account owner
  lastDate: string;
  unread: boolean;
  starred: boolean;
  hasAttachments: boolean;
};

/** An open conversation. */
export type Thread = ThreadSummary & { messages: Message[] }; // oldest first

/** An account's mail import, as the engine reports it while running. */
export type ImportProgress = { state: "importing" | "done" | "error"; done: number; total: number; error?: string };

export type Server = { host: string; port: number; security: "tls" | "starttls" };

/** What the add-account form sends. */
export type AccountSettings = {
  name: string;
  email: string;
  username: string; // empty means the email address
  password: string;
  imap: Server;
  smtp: Server;
};
