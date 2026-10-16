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

// A refresh or <base> would navigate the frame itself, which no CSP directive stops: they go before rendering.
const NAVIGATION = /<meta[^>]*http-equiv\s*=\s*["']?\s*refresh[^>]*>|<base\b[^>]*>/gi;

/** The frame's document: the policy first, then the message. remote allows images, styles and fonts from the web. */
export function frameDoc(html: string, remote: boolean): string {
  const web = remote ? " https: http:" : "";
  const policy =
    `default-src 'none'; script-src 'none'; style-src 'unsafe-inline'${web}; ` +
    `img-src data:${web}; font-src data:${web}; media-src 'none'`;
  return (
    `<!doctype html><html><head><meta charset="utf-8">` +
    `<meta http-equiv="Content-Security-Policy" content="${policy}"><style>${BASE}</style></head>` +
    `<body>${html.replace(NAVIGATION, "")}</body></html>`
  );
}

const REMOTE = /<img\b[^>]*\bsrc\s*=\s*["']?\s*https?:|\bbackground\s*=\s*["']?\s*https?:|url\(\s*["']?\s*https?:|<link\b[^>]*\bhref\s*=\s*["']?\s*https?:/i;

/** Whether the message wants anything from the web (images, backgrounds, styles): "Load images" is offered then. */
export const hasRemoteContent = (html: string) => REMOTE.test(html);
