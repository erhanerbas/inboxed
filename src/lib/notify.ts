import { getCurrentWindow } from "@tauri-apps/api/window";
import { isPermissionGranted, requestPermission, sendNotification } from "@tauri-apps/plugin-notification";
import type { Account, NewMail } from "./types";

const inTauri = () => "__TAURI_INTERNALS__" in window;

/** One notification for a pass's new mail: who and what for one message, how many and from whom for several. */
export function summarize(batch: NewMail[], accounts: Account[]): { title: string; body: string } {
  const first = batch[0];
  const account = accounts.length > 1 ? accounts.find((a) => a.id === first.accountId)?.name : undefined;
  const named = (title: string) => (account ? `${title} · ${account}` : title);
  if (batch.length === 1) return { title: named(first.from), body: [first.subject, first.snippet].filter(Boolean).join("\n") };
  const senders = [...new Set(batch.map((m) => m.from))];
  const body =
    senders.length === 1
      ? senders[0]
      : senders.length === 2
        ? `${senders[0]} and ${senders[1]}`
        : `${senders[0]}, ${senders[1]} and ${senders.length - 2} more`;
  return { title: named(`${batch.length} new messages`), body };
}

let allowed: Promise<boolean> | undefined; // asked once, at the first new mail rather than at launch

/** Shows a native notification with sound. A refusal or failure is logged: the banner and badge still work. */
export async function deliver(n: { title: string; body: string }) {
  try {
    if (inTauri()) {
      allowed ??= (async () => (await isPermissionGranted()) || (await requestPermission()) === "granted")();
      if (await allowed) sendNotification({ title: n.title, body: n.body, sound: "Ping" });
    } else if ("Notification" in window) {
      allowed ??= (async () => Notification.permission === "granted" || (await Notification.requestPermission()) === "granted")();
      if (await allowed) new Notification(n.title, { body: n.body }).onclick = () => window.focus();
    }
  } catch (err) {
    allowed = undefined;
    console.error(err);
  }
}

/** The Dock badge: unread mail in the inboxes; 0 clears it. Only the app has a Dock icon. */
export function setBadge(count: number) {
  if (inTauri()) getCurrentWindow().setBadgeCount(count || undefined).catch(console.error);
}
