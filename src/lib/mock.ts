import { splitQuoted } from "./quote";
import type { Account, Address, Mailbox, Message, Thread } from "./types";

const MIN = 1;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;
const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString();

const you1: Address = { name: "You", email: "you@example.com" };
const you2: Address = { name: "You", email: "you@work.example" };
const maya: Address = { name: "Maya Chen", email: "maya@work.example" };
const daniel: Address = { name: "Daniel Okafor", email: "daniel@work.example" };
const priya: Address = { name: "Priya Nair", email: "priya@work.example" };
const sam: Address = { name: "Sam Rivera", email: "sam@example.net" };
const skyline: Address = { name: "Skyline Air", email: "no-reply@skyline-air.example" };
const store: Address = { name: "Example Store", email: "orders@store.example" };
const grace: Address = { name: "Grace Bell", email: "grace@lettings.example" };

// Account colors stay clear of the orange accent hue, which means "unread" and "selected".
export const accounts: Account[] = [
  { id: "a1", name: "Personal", email: you1.email, provider: "gmail", color: "oklch(0.5 0.11 160)" },
  { id: "a2", name: "Work", email: you2.email, provider: "outlook", color: "oklch(0.5 0.12 250)" },
];

const box = (accountId: string, id: string, name: string, role: Mailbox["role"], parent?: string): Mailbox => ({
  id: `${accountId}-${id}`,
  accountId,
  name,
  role,
  parentId: parent ? `${accountId}-${parent}` : null,
  unread: 0,
});

export const mailboxes: Mailbox[] = [
  box("a1", "inbox", "Inbox", "inbox"),
  box("a1", "starred", "Starred", "starred"),
  box("a1", "drafts", "Drafts", "drafts"),
  box("a1", "sent", "Sent", "sent"),
  box("a1", "archive", "Archive", "archive"),
  box("a1", "spam", "Spam", "spam"),
  box("a1", "trash", "Trash", "trash"),
  box("a1", "receipts", "Receipts", null),
  box("a1", "travel", "Travel", null),
  box("a1", "travel-2026", "2026", null, "travel"),
  box("a2", "inbox", "Inbox", "inbox"),
  box("a2", "drafts", "Drafts", "drafts"),
  box("a2", "sent", "Sent Items", "sent"),
  box("a2", "archive", "Archive", "archive"),
  box("a2", "trash", "Deleted Items", "trash"),
  box("a2", "projects", "Projects", null),
  box("a2", "projects-inboxed", "Inboxed", null, "projects"),
  box("a2", "projects-launch", "Launch", null, "projects"),
];

let messageCount = 0;
function msg(from: Address, to: Address[], minutesAgo: number, body: string, attachments: Message["attachments"] = []): Message {
  return { id: `m${++messageCount}`, threadId: "", from, to, cc: [], date: ago(minutesAgo), body, attachments };
}

function thread(
  id: string,
  accountId: string,
  boxes: string[],
  subject: string,
  flags: { unread?: boolean; starred?: boolean },
  messages: Message[],
): Thread {
  const me = accounts.find((a) => a.id === accountId)!.email;
  const last = messages[messages.length - 1];
  const everyone = messages.flatMap((m) => [m.from, ...m.to]).filter((a) => a.email !== me);
  for (const m of messages) m.threadId = id;
  return {
    id,
    accountId,
    mailboxIds: boxes.map((b) => `${accountId}-${b}`),
    subject,
    snippet: (splitQuoted(last.body).text || last.body).replace(/\s+/g, " ").slice(0, 160),
    participants: [...new Map(everyone.map((a) => [a.email, a])).values()],
    lastDate: last.date,
    unread: flags.unread ?? false,
    starred: flags.starred ?? false,
    hasAttachments: messages.some((m) => m.attachments.length > 0),
    messages,
  };
}

export const threads: Thread[] = [
  thread("t1", "a2", ["inbox", "projects-inboxed"], "Inboxed design review: sidebar and thread view", { unread: true }, [
    msg(maya, [you2, daniel], 2 * DAY, "Hi both,\n\nI've pushed the first pass of the sidebar and the thread view to the design file. The sidebar follows the reference we liked: an icon rail for accounts, then the mailbox tree with thin guide lines for nested folders.\n\nCould you take a look before Thursday?\n\nMaya"),
    msg(daniel, [maya, you2], DAY, "Looks great. One question: do we show unread counts on every folder or only the inbox?\n\nOn Tue, Maya Chen wrote:\n> Could you take a look before Thursday?"),
    msg(you2, [maya, daniel], 20 * HOUR, "Every folder, but only the inbox count gets the accent colour. Everything else stays muted so the inbox reads first.\n\nOn Wed, Daniel Okafor wrote:\n> do we show unread counts on every folder or only the inbox?"),
    msg(maya, [you2, daniel], 35 * MIN, "Done. I updated both screens and collapsed older messages in long threads, so the latest reply is always the first thing you read.\n\nOn Wed, You wrote:\n> Every folder, but only the inbox count gets the accent colour.\n> Everything else stays muted so the inbox reads first."),
  ]),
  thread("t2", "a1", ["inbox", "travel-2026"], "Your flight to Lisbon is confirmed", { unread: true }, [
    msg(skyline, [you1], 2 * HOUR, "Booking reference: K7Q2XP\n\nOutbound: Fri 6 Nov, 07:40 to Lisbon\nReturn: Mon 9 Nov, 18:15\n\nYour boarding pass is attached. Online check-in opens 36 hours before departure.", [
      { name: "boarding-pass.pdf", size: 182_000 },
      // Real mail often repeats inline-image names.
      { name: "image001.png", size: 4_200 },
      { name: "image001.png", size: 3_900 },
    ]),
  ]),
  thread("t3", "a1", ["inbox"], "Dinner on Saturday?", { starred: true }, [
    msg(sam, [you1], DAY + 3 * HOUR, "Are you free Saturday? Thinking of the new place on Harbour Street, around 8."),
    msg(you1, [sam], DAY, "Saturday works. 8 is perfect, I'll book a table.\n\nOn Thu, Sam Rivera wrote:\n> Are you free Saturday? Thinking of the new place on Harbour Street, around 8."),
  ]),
  thread("t4", "a2", ["inbox"], "Q4 roadmap: please review before Friday", { unread: true }, [
    msg(priya, [you2], 5 * HOUR, "Hi,\n\nAttached are the Q4 roadmap draft and the estimates sheet. The two open questions are on page 3: whether Outlook support ships in the first release, and how much history we import by default.\n\nComments by Friday would be great.\n\nPriya", [
      { name: "roadmap-q4.pdf", size: 2_400_000 },
      { name: "estimates.xlsx", size: 48_000 },
    ]),
  ]),
  thread("t5", "a1", ["inbox", "receipts"], "Re: Fwd: Your receipt from Example Store for order #A-2026-10-12-000123 (3 items, shipped in two separate packages)", {}, [
    msg(store, [you1], 3 * DAY, "Thanks for your order. Track both packages here:\nhttps://store.example/orders/A-2026-10-12-000123/tracking?packages=PKG-88213-ZX,PKG-88214-ZY&utm_source=receipt&utm_medium=email\n\nItems: 3\nTotal: €84.97"),
  ]),
  thread("t6", "a1", ["sent"], "Re: Apartment viewing", {}, [
    msg(you1, [grace], 400 * DAY, "Thanks Grace, Tuesday at 6 works for me.\n\nOn Mon, Grace Bell wrote:\n> Would Tuesday at 6pm suit you for the viewing?"),
  ]),
  thread("t7", "a2", ["inbox"], "Fwd: Brand guidelines", {}, [
    msg(daniel, [you2], 40 * DAY, "-----Original Message-----\nFrom: Studio North <hello@studio-north.example>\nSubject: Brand guidelines\n\nHi Daniel, the final brand guidelines are attached. Let us know if anything is missing.", [
      { name: "brand-guidelines.pdf", size: 5_800_000 },
    ]),
  ]),
];

// Unread counts follow the threads so the two can never disagree.
for (const m of mailboxes) m.unread = threads.filter((t) => t.unread && t.mailboxIds.includes(m.id)).length;
