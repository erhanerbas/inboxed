import { describe, expect, it } from "vitest";
import { frameDoc, hasRemoteContent, QUOTE_SELECTOR } from "./html";

const csp = (doc: string) => doc.match(/Content-Security-Policy" content="([^"]*)"/)?.[1] ?? "";

describe("frameDoc", () => {
  it("blocks every remote load until asked", () => {
    const policy = csp(frameDoc("<p>Hi</p>", false));
    expect(policy).toContain("default-src 'none'");
    expect(policy).toContain("img-src data:");
    expect(policy).not.toContain("http");
  });

  it("allows remote images, styles and fonts once asked", () => {
    const policy = csp(frameDoc("<p>Hi</p>", true));
    for (const kind of ["img-src", "style-src", "font-src"]) expect(policy).toMatch(new RegExp(`${kind}[^;]*https: http:`));
    expect(policy).toContain("script-src 'none'");
  });

  it("puts the policy first, on a light page, and drops what could navigate the frame", () => {
    const doc = frameDoc('<meta http-equiv="refresh" content="0;url=https://evil.example"><base href="https://evil.example/"><p>Hi</p>', false);
    expect(doc.indexOf("Content-Security-Policy")).toBeLessThan(doc.indexOf("<p>Hi</p>"));
    expect(doc).not.toContain("refresh");
    expect(doc).not.toContain("<base");
    expect(doc).toContain("color-scheme: light");
  });

  it("hides quoted history unless shown", () => {
    const doc = frameDoc("<p>Hi</p>", false);
    expect(doc).toContain(":root:not(.ib-show-quotes)");
    expect(QUOTE_SELECTOR).toContain(".gmail_quote");
  });
});

describe("hasRemoteContent", () => {
  it("spots remote images, backgrounds and CSS urls", () => {
    expect(hasRemoteContent('<img src="https://t.example/pixel.gif">')).toBe(true);
    expect(hasRemoteContent('<td background="http://x.example/bg.png">')).toBe(true);
    expect(hasRemoteContent('<div style="background:url(https://x.example/a.png)">')).toBe(true);
    expect(hasRemoteContent("<div style=\"background: url('http://x.example/a.png')\">")).toBe(true);
  });

  it("ignores data and cid images and plain links", () => {
    expect(hasRemoteContent('<img src="data:image/png;base64,AA=="><img src="cid:a@b">')).toBe(false);
    expect(hasRemoteContent('<a href="https://example.com">site</a>')).toBe(false);
  });
});
