import { describe, expect, it } from "vitest";
import { draftHtml, emailHtml, plainText, quoteHtml, swapSignatureHtml, textToHtml } from "./rich";

describe("textToHtml", () => {
  it("makes a paragraph of every line, escaped", () => {
    expect(textToHtml("a\n\nb<&")).toBe("<p>a</p><p></p><p>b&lt;&amp;</p>");
    expect(textToHtml("")).toBe("<p></p>");
  });
});

describe("quoteHtml", () => {
  it("keeps the attribution and quotes the quoted lines", () => {
    expect(quoteHtml("On Monday, Maya wrote:\n> Hi\n>\n> there")).toBe(
      '<p>On Monday, Maya wrote:</p><blockquote type="cite"><p>Hi</p><p></p><p>there</p></blockquote>',
    );
    expect(quoteHtml("> > older")).toBe('<blockquote type="cite"><p>&gt; older</p></blockquote>');
  });
});

describe("draftHtml", () => {
  it("is the text, and a reply's quote quoted", () => {
    expect(draftHtml("Hi\n\n-- \nSam", false)).toBe("<p>Hi</p><p></p><p>-- </p><p>Sam</p>");
    expect(draftHtml("\n\nOn Monday, Maya wrote:\n> Hi", true)).toBe(
      '<p></p><p>On Monday, Maya wrote:</p><blockquote type="cite"><p>Hi</p></blockquote>',
    );
  });
});

describe("emailHtml", () => {
  it("wraps in the base font and keeps paragraphs single-spaced, empty ones a line high", () => {
    const out = emailHtml('<p style="text-align: center">x</p><p></p><p>y</p>');
    expect(out).toMatch(/^<div style="font-family: [^"]+; font-size: 11pt">/);
    expect(out).toContain('<p style="margin: 0; text-align: center">x</p><p style="margin: 0"><br></p><p style="margin: 0">y</p></div>');
  });
  it("leaves mentions alone and doesn't wrap twice", () => {
    const mention = '<p><a href="mailto:ada@x" data-type="mention" data-id="ada@x" data-label="Ada">@Ada</a></p>';
    expect(emailHtml(mention)).toContain('<a href="mailto:ada@x" data-type="mention" data-id="ada@x" data-label="Ada">@Ada</a>');
    expect(emailHtml(emailHtml("<p>x</p>"))).toBe(emailHtml("<p>x</p>"));
  });
});

describe("swapSignatureHtml", () => {
  it("swaps, removes, adds (above a folded quote), and leaves an edited one alone", () => {
    expect(swapSignatureHtml("<p>Hi</p><p></p><p>-- </p><p>Sam</p>", "Sam", "Samuel")).toBe("<p>Hi</p><p></p><p>-- </p><p>Samuel</p>");
    expect(swapSignatureHtml("<p>Hi</p><p></p><p>-- </p><p>Sam</p>", "Sam", "")).toBe("<p>Hi</p>");
    expect(swapSignatureHtml("<p>Hi</p>", "", "Sam")).toBe("<p>Hi</p><p></p><p>-- </p><p>Sam</p>");
    expect(swapSignatureHtml("<p></p><blockquote>q</blockquote>", "", "Sam", "<blockquote>q</blockquote>")).toBe(
      "<p></p><p></p><p>-- </p><p>Sam</p><blockquote>q</blockquote>",
    );
    expect(swapSignatureHtml("<p>Hi</p><p></p><p>-- </p><p>Sam (edited)</p>", "Sam (old)", "New")).toBe(
      "<p>Hi</p><p></p><p>-- </p><p>Sam (edited)</p>",
    );
  });
});

describe("plainText", () => {
  it("writes the editor's document as mail text: quotes marked, lists numbered or dashed, mentions by name", () => {
    const doc = {
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "Hi " }, { type: "mention", attrs: { id: "ada@x", label: "Ada" } }] },
        { type: "paragraph" },
        { type: "bulletList", content: [{ type: "listItem", content: [{ type: "paragraph", content: [{ type: "text", text: "one" }] }] }] },
        { type: "orderedList", content: [{ type: "listItem", content: [{ type: "paragraph", content: [{ type: "text", text: "first" }] }] }] },
        { type: "paragraph", content: [{ type: "text", text: "a" }, { type: "hardBreak" }, { type: "text", text: "b" }] },
        { type: "paragraph", content: [{ type: "text", text: "On Monday, Maya wrote:" }] },
        {
          type: "blockquote",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "Hi" }] },
            { type: "blockquote", content: [{ type: "paragraph", content: [{ type: "text", text: "older" }] }] },
          ],
        },
      ],
    };
    expect(plainText(doc)).toBe("Hi @Ada\n\n- one\n1. first\na\nb\nOn Monday, Maya wrote:\n> Hi\n> > older");
  });
});
