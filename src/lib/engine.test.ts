import { describe, expect, it } from "vitest";
import { parseEvents } from "./engine";

describe("parseEvents", () => {
  it("keeps each event's data", () => {
    expect(parseEvents('event: mail\ndata: [{"threadId":"t"}]\n\n').events).toEqual([{ name: "mail", data: '[{"threadId":"t"}]' }]);
  });

  it("splits complete events and keeps the unfinished tail", () => {
    const { events, rest } = parseEvents(": connected\n\nevent: change\ndata: {}\n\nevent: cha");
    expect(events.map((e) => e.name)).toEqual(["change"]);
    expect(rest).toBe("event: cha");
  });

  it("accepts CRLF line endings", () => {
    expect(parseEvents("event: change\r\ndata: {}\r\n\r\n").events.map((e) => e.name)).toEqual(["change"]);
  });

  it("ignores comments and keeps the order", () => {
    expect(parseEvents(": ping\n\nevent: change\ndata: {}\n\nevent: change\ndata: {}\n\n").events.map((e) => e.name)).toEqual([
      "change",
      "change",
    ]);
  });
});
