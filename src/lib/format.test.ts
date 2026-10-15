import { describe, expect, it } from "vitest";
import { formatDate, formatSize } from "./format";

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
