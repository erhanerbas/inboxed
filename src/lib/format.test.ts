import { describe, expect, it } from "vitest";
import { forwardBody, formatDate, formatSize, replyQuote } from "./format";
import type { Message } from "./types";

// Thursday 15 October 2026, 14:00 local time.
const now = new Date(2026, 9, 15, 14, 0);
const at = (y: number, month: number, d: number, h = 0, min = 0) => new Date(y, month, d, h, min).toISOString();
// ICU puts a narrow no-break space before AM/PM; normalize so the assertion is readable.
const fmt = (iso: string) => formatDate(iso, now, "en-US").replace(/\s/g, " ");

describe("formatDate", () => {
  it("shows the time for today", () => expect(fmt(at(2026, 9, 15, 9, 41))).toBe("9:41 AM"));
  it("says Yesterday", () => expect(fmt(at(2026, 9, 14, 23, 0))).toBe("Yesterday"));
  it("shows the weekday within the last week", () => expect(fmt(at(2026, 9, 12, 10, 0))).toBe("Monday"));
  it("shows month and day after a week", () => expect(fmt(at(2026, 9, 8, 10, 0))).toBe("Oct 8"));
  it("shows month and day earlier this year", () => expect(fmt(at(2026, 2, 3))).toBe("Mar 3"));
  it("adds the year for earlier years", () => expect(fmt(at(2025, 11, 24))).toBe("Dec 24, 2025"));
});

describe("formatSize", () => {
  it("bytes", () => expect(formatSize(500)).toBe("500 B"));
  it("kilobytes", () => expect(formatSize(1536)).toBe("2 KB"));
  it("megabytes", () => expect(formatSize(2_400_000)).toBe("2.3 MB"));
});

const original: Message = {
  id: "acc/d1@x", threadId: "acc/d1@x", subject: "Dinner plans", draft: false, attachments: [], cc: [],
  from: { name: "Maya Chen", email: "maya@example.com" }, to: [{ name: "You", email: "you@example.com" }],
  date: "2026-10-15T12:00:00.000Z", body: "Are you free?\n\nSaturday works.",
};

describe("replyQuote", () => {
  it("quotes every line under a 'wrote:' line, after room to type", () => {
    const q = replyQuote(original, "en-US");
    expect(q.startsWith("\n\nOn ")).toBe(true);
    expect(q).toContain("Maya Chen wrote:\n> Are you free?\n>\n> Saturday works.");
  });
});

describe("forwardBody", () => {
  it("carries the original's headers, then its text", () => {
    const f = forwardBody(original, "Dinner plans", "en-US");
    expect(f).toContain("---------- Forwarded message ----------");
    expect(f).toContain("From: Maya Chen <maya@example.com>");
    expect(f).toContain("Subject: Dinner plans");
    expect(f).toContain("To: You <you@example.com>");
    expect(f.endsWith("Are you free?\n\nSaturday works.")).toBe(true);
  });
});
