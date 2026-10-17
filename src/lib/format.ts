import type { Address, Message } from "./types";

/** Mail-list style date: time today, "Yesterday", weekday this week, then a date. */
export function formatDate(iso: string, now = new Date(), locale?: string): string {
  const d = new Date(iso);
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString(locale, { hour: "numeric", minute: "2-digit" });
  }
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (d.toDateString() === yesterday.toDateString()) return "Yesterday";
  if (now.getTime() - d.getTime() < 7 * 86_400_000) return d.toLocaleDateString(locale, { weekday: "long" });
  if (d.getFullYear() === now.getFullYear()) return d.toLocaleDateString(locale, { month: "short", day: "numeric" });
  return d.toLocaleDateString(locale, { year: "numeric", month: "short", day: "numeric" });
}

/** The heading a message list groups a date under: Today, Yesterday, This week, Last week (weeks start on Monday),
 *  then the month, with its year when that isn't this year. */
export function dateGroup(iso: string, now = new Date(), locale?: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "Today"; // the engine stores mail a moment before its date
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate());
  const daysAgo = Math.round((day(now).getTime() - day(d).getTime()) / 86_400_000); // round: DST days aren't 24h
  if (daysAgo <= 0) return "Today";
  if (daysAgo === 1) return "Yesterday";
  const intoWeek = (now.getDay() + 6) % 7; // days since Monday
  if (daysAgo <= intoWeek) return "This week";
  if (daysAgo <= intoWeek + 7) return "Last week";
  return d.toLocaleDateString(locale, d.getFullYear() === now.getFullYear() ? { month: "long" } : { month: "long", year: "numeric" });
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

const fullDate = (iso: string, locale?: string) =>
  new Date(iso).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });
const person = (a: Address) => (a.name && a.name !== a.email ? `${a.name} <${a.email}>` : a.email);

/** What a reply starts with: room to type, then the message it answers, quoted line by line. */
export function replyQuote(m: Message, locale?: string): string {
  const quoted = m.body.split("\n").map((line) => (line ? `> ${line}` : ">"));
  return `\n\nOn ${fullDate(m.date, locale)}, ${m.from.name || m.from.email} wrote:\n${quoted.join("\n")}`;
}

/** What a forward starts with: room to type, the original's headers, then its text. */
export function forwardBody(m: Message, subject: string, locale?: string): string {
  return [
    "\n\n---------- Forwarded message ----------",
    `From: ${person(m.from)}`,
    `Date: ${fullDate(m.date, locale)}`,
    `Subject: ${subject}`,
    `To: ${m.to.map(person).join(", ")}`,
    "",
    m.body,
  ].join("\n");
}

/** Addresses as the To and Cc fields take them. Names are quoted, so one with a comma can't split an address. */
export function addresses(list: Address[]): string {
  return list.map((a) => (a.name && a.name !== a.email ? `"${a.name.replace(/["\\]/g, "\\$&")}" <${a.email}>` : a.email)).join(", ");
}
