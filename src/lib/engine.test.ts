import { describe, expect, it } from "vitest";
import { parseEvents } from "./engine";

describe("parseEvents", () => {
  it("splits complete events and keeps the unfinished tail", () => {
    const { names, rest } = parseEvents(": connected\n\nevent: change\ndata: {}\n\nevent: cha");
    expect(names).toEqual(["change"]);
    expect(rest).toBe("event: cha");
  });

  it("ignores comments and keeps the order", () => {
    expect(parseEvents(": ping\n\nevent: change\ndata: {}\n\nevent: change\ndata: {}\n\n").names).toEqual(["change", "change"]);
  });
});
