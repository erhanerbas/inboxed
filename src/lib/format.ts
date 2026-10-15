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

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}
