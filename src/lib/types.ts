export type Provider = "gmail" | "outlook" | "yahoo" | "imap";

export type Account = {
  id: string;
  name: string;
  email: string;
  provider: Provider;
  color: string; // identity color for the rail avatar and list dots
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

export type Thread = {
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
  messages: Message[]; // oldest first
};
