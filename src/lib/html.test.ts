import { describe, expect, it } from "vitest";
import { frameDoc, hasRemoteContent, mailtoDraft, QUOTE_SELECTOR } from "./html";

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

  it("puts the policy first, on a light page, and defuses meta, base and link tags", () => {
    const doc = frameDoc(
      '<meta http-equiv="refresh" content="0;url=https://evil.example"><BASE href="https://evil.example/">' +
        '<link x="rel=stylesheet>" rel=preconnect href="//t.example"><p>Hi</p>',
      false,
    );
    expect(doc.indexOf("Content-Security-Policy")).toBeLessThan(doc.indexOf("<p>Hi</p>"));
    const body = doc.slice(doc.indexOf("<body>"));
    expect(body).not.toMatch(/<(meta|base|link)\b/i); // WebKit preconnects for <link> despite the CSP
    expect(csp(doc)).toContain("base-uri 'none'");
    expect(csp(doc)).toContain("form-action 'none'");
    expect(doc).toContain("color-scheme: light");
  });

  it("keeps stylesheet links once remote content is allowed", () => {
    expect(frameDoc('<link rel="stylesheet" href="https://x.example/a.css">', true)).toContain("<link");
  });

  it("gives protocol-relative URLs a scheme (srcdoc would resolve them against the app)", () => {
    const doc = frameDoc('<img src="//x.example/a.png"><div style="background:url(//x.example/b.png)">', true);
    expect(doc).toContain('src="https://x.example/a.png"');
    expect(doc).toContain("url(https://x.example/b.png)");
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

  it("spots srcset, poster, svg images, @import and protocol-relative sources", () => {
    expect(hasRemoteContent('<img srcset="https://x.example/a.png 2x">')).toBe(true);
    expect(hasRemoteContent('<video poster="http://x.example/p.png">')).toBe(true);
    expect(hasRemoteContent('<svg><image href="https://x.example/i.png"/></svg>')).toBe(true);
    expect(hasRemoteContent('<style>@import "https://x.example/a.css";</style>')).toBe(true);
    expect(hasRemoteContent('<img src="//x.example/a.png">')).toBe(true);
  });

  it("ignores data and cid images and plain links", () => {
    expect(hasRemoteContent('<img src="data:image/png;base64,AA=="><img src="cid:a@b">')).toBe(false);
    expect(hasRemoteContent('<a href="https://example.com">site</a>')).toBe(false);
  });
});

describe("mailtoDraft", () => {
  it("reads to, cc, subject and body", () => {
    expect(mailtoDraft("mailto:maya@x?cc=sam@x&subject=Hi%20there&body=Line%201")).toEqual({
      to: "maya@x", cc: "sam@x", subject: "Hi there", body: "Line 1",
    });
    expect(mailtoDraft("mailto:?to=lee@x").to).toBe("lee@x");
  });

  it("survives a malformed escape", () => {
    expect(mailtoDraft("mailto:bad%zz@x").to).toBe("bad%zz@x");
  });
});
