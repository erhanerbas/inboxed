import { describe, expect, it } from "vitest";
import { splitQuoted } from "./quote";

describe("splitQuoted", () => {
  it("leaves a body without quotes alone", () => {
    expect(splitQuoted("Hi\n\nThanks")).toEqual({ text: "Hi\n\nThanks", quoted: "" });
  });

  it("splits at an 'On … wrote:' attribution", () => {
    expect(splitQuoted("Sounds good.\n\nOn Tue, Maya Chen wrote:\n> See you then")).toEqual({
      text: "Sounds good.",
      quoted: "On Tue, Maya Chen wrote:\n> See you then",
    });
  });

  it("splits a trailing block of > lines", () => {
    expect(splitQuoted("Yes.\n> Are you coming?\n>\n> Sam")).toEqual({
      text: "Yes.",
      quoted: "> Are you coming?\n>\n> Sam",
    });
  });

  it("keeps interleaved replies as text", () => {
    const body = "> Question one?\nAnswer one.\n> Question two?\nAnswer two.";
    expect(splitQuoted(body)).toEqual({ text: body, quoted: "" });
  });

  it("splits at an Outlook original-message marker", () => {
    expect(splitQuoted("See below.\n\n-----Original Message-----\nFrom: Sam")).toEqual({
      text: "See below.",
      quoted: "-----Original Message-----\nFrom: Sam",
    });
  });

  it("returns empty text when the whole body is quoted", () => {
    const body = "-----Original Message-----\nFrom: Studio North\n\nHi Daniel";
    expect(splitQuoted(body)).toEqual({ text: "", quoted: body });
  });
});
