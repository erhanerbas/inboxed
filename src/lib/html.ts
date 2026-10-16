// HTML mail is shown in a sandboxed iframe (no scripts, forms or navigation) whose own Content-Security-Policy blocks
// every remote load until the reader asks for images. These build what goes in it.

/** Where common mail apps put quoted history: hidden until the reader shows it. */
export const QUOTE_SELECTOR = [
  'blockquote[type="cite"]',
  ".gmail_quote",
  ".yahoo_quoted",
  "#divRplyFwdMsg",
  "#divRplyFwdMsg ~ *",
  "#appendonsend",
  "#appendonsend ~ *",
  ".moz-cite-prefix",
  ".moz-cite-prefix ~ *",
].join(", ");

// Mail is designed for white paper: it gets a light page of its own rather than the app's dark one.
const BASE = `:root { color-scheme: light; }
body { margin: 0; padding: 20px 24px; display: flow-root; background: #fff; color: #1c1b1b; overflow-wrap: anywhere;
  font: 15px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
img { max-width: 100%; height: auto; }
pre { white-space: pre-wrap; }
:root:not(.ib-show-quotes) :is(${QUOTE_SELECTOR}) { display: none !important; }`;

// <meta> (refresh), <base> and <link> become unknown, inert elements: renaming the tag can't be dodged the way
// matching its attributes can. Links are kept once remote content is allowed (stylesheets then load); before that,
// WebKit would still preconnect to a <link>'s host despite the CSP, telling a tracker the mail was opened.
const INERT_ALWAYS = /<(\/?)(meta|base)\b/gi;
const INERT_LINK = /<(\/?)(link)\b/gi;
// Protocol-relative URLs would resolve against the app (tauri://…) inside srcdoc: give them https.
const PROTOCOL_RELATIVE = /(\b(?:src|href|srcset|poster|background)\s*=\s*["']?\s*|url\(\s*["']?\s*)\/\/(?=[\w.-])/gi;

/** The frame's document: the policy first, then the message. remote allows images, styles and fonts from the web. */
export function frameDoc(html: string, remote: boolean): string {
  const web = remote ? " https: http:" : "";
  const policy =
    `default-src 'none'; script-src 'none'; style-src 'unsafe-inline'${web}; ` +
    `img-src data:${web}; font-src data:${web}; media-src 'none'; base-uri 'none'; form-action 'none'`;
  let body = html.replace(INERT_ALWAYS, "<$1x-$2").replace(PROTOCOL_RELATIVE, "$1https://");
  if (!remote) body = body.replace(INERT_LINK, "<$1x-$2");
  return (
    `<!doctype html><html><head><meta charset="utf-8">` +
    `<meta http-equiv="Content-Security-Policy" content="${policy}"><style>${BASE}</style></head>` +
    `<body>${body}</body></html>`
  );
}

const REMOTE =
  /<img\b[^>]*\bsrc\s*=\s*["']?\s*(?:https?:)?\/\/|\bsrcset\s*=\s*["']?[^"'>]*(?:https?:)?\/\/|\b(?:background|poster)\s*=\s*["']?\s*(?:https?:)?\/\/|url\(\s*["']?\s*(?:https?:)?\/\/|<link\b[^>]*\bhref\s*=\s*["']?\s*(?:https?:)?\/\/|<image\b[^>]*\bhref\s*=\s*["']?\s*(?:https?:)?\/\/|@import\s+["']?\s*(?:url\()?\s*["']?(?:https?:)?\/\//i;

/** Whether the message wants anything from the web (images, backgrounds, styles): "Load images" is offered then. */
export const hasRemoteContent = (html: string) => REMOTE.test(html);

function decode(s: string) {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

/** What a mailto: link asks for, as a draft: to (path or ?to=), cc, subject, body. */
export function mailtoDraft(href: string): { to: string; cc: string; subject: string; body: string } {
  const [path, query = ""] = href.replace(/^mailto:/i, "").split("?");
  const params = new Map<string, string>();
  for (const pair of query.split("&")) {
    const [k, v = ""] = pair.split("=");
    if (k) params.set(k.toLowerCase(), decode(v)); // "+" is a literal plus in mailto (RFC 6068)
  }
  return {
    to: [decode(path), params.get("to") ?? ""].filter(Boolean).join(", "),
    cc: params.get("cc") ?? "",
    subject: params.get("subject") ?? "",
    body: params.get("body") ?? "",
  };
}
