import { addresses } from "./format";
import type { Contact } from "./types";

/** The contacts a query brings up, in the engine's order: it starts a word of the name, the address, or a piece of
 *  the address's local part (ada.l@… for "l"). Nobody in exclude (lowercased addresses). */
export function matchContacts(all: Contact[], query: string, exclude: string[], limit = 8): Contact[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  const skip = new Set(exclude);
  const starts = (c: Contact) => {
    const email = c.email.toLowerCase();
    const words = [...c.name.toLowerCase().split(/[\s,"'()]+/), email, ...email.split("@")[0].split(/[._-]/)];
    return words.some((w) => w.startsWith(q));
  };
  return all.filter((c) => !skip.has(c.email.toLowerCase()) && starts(c)).slice(0, limit);
}

/** The comma-separated piece of an address field around the caret: what suggestions are for. */
export function tokenAt(value: string, caret: number): { start: number; end: number; query: string } {
  const start = value.lastIndexOf(",", caret - 1) + 1;
  const comma = value.indexOf(",", caret);
  const end = comma === -1 ? value.length : comma;
  return { start, end, query: value.slice(start, end).trim() };
}

/** The field with the piece at the caret replaced by the contact (quoted as the engine reads it), and where the caret
 *  goes: after it, ready for the next. */
export function pick(value: string, caret: number, c: Contact): { value: string; caret: number } {
  const { start, end } = tokenAt(value, caret);
  const before = value.slice(0, start).replace(/\s*$/, start ? " " : "");
  const rest = value.slice(end).replace(/^,\s*/, "");
  const inserted = `${before}${addresses([c])}, `;
  return { value: inserted + rest, caret: inserted.length };
}

/** The addresses in a field, lowercased. */
export const emailsIn = (field: string) =>
  [...field.matchAll(/<([^<>\s]+@[^<>\s]+)>|([^\s<>,"]+@[^\s<>,"]+)/g)].map((m) => (m[1] ?? m[2]).toLowerCase());

/** The field with the contact added at the end, unless they're in it already. */
export function addRecipient(field: string, c: Contact): string {
  if (emailsIn(field).includes(c.email.toLowerCase())) return field;
  const kept = field.replace(/[\s,]*$/, "");
  return kept ? `${kept}, ${addresses([c])}` : addresses([c]);
}
