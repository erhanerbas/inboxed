import { splitQuoted } from "./quote";

// Compose writes HTML (the Tiptap editor in editor.ts). These turn text into the editor's HTML and the editor's HTML
// into what's sent. String work only: the editor's output is regular, and tests run without a DOM.

/** The fonts offered: ones recipients are likely to have, each with fallbacks. Default sets no font at all. */
export const FONTS = [
  { label: "Default", stack: "" },
  { label: "Arial", stack: "Arial, Helvetica, sans-serif" },
  { label: "Helvetica", stack: "Helvetica, Arial, sans-serif" },
  { label: "Verdana", stack: "Verdana, Geneva, sans-serif" },
  { label: "Georgia", stack: "Georgia, 'Times New Roman', serif" },
  { label: "Garamond", stack: "Garamond, 'Hoefler Text', Baskerville, Georgia, serif" },
  { label: "Times New Roman", stack: "'Times New Roman', Times, serif" },
  { label: "Courier New", stack: "'Courier New', Courier, monospace" },
];
export const SIZES = [8, 9, 10, 11, 12, 14, 16, 18, 20, 24, 28, 36];
export const BASE_SIZE = 11;
export const BASE_FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif";
// Swatches for white paper: mail is read on white.
export const COLORS = ["#000000", "#595959", "#c00000", "#e46c0a", "#bf8f00", "#00803c", "#007c80", "#0b5394", "#7030a0", "#c2185b"];
export const HIGHLIGHTS = ["#fff2a8", "#c9f2c7", "#cfe2ff", "#ffd6e7", "#ffe0b2", "#e5d4ff", "#e7e6e6"];

const escape = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

/** One paragraph per line, as the editor writes them. */
export const textToHtml = (text: string) => text.split("\n").map((line) => `<p>${escape(line)}</p>`).join("");

/** A reply's quoted history: the attribution as written, each run of "> " lines a quote (one level taken off). */
export function quoteHtml(quoted: string): string {
  const out: string[] = [];
  let run: string[] = [];
  const flush = () => {
    if (run.length) out.push(`<blockquote type="cite">${textToHtml(run.join("\n"))}</blockquote>`);
    run = [];
  };
  for (const line of quoted.split("\n")) {
    if (line.startsWith(">")) run.push(line.replace(/^> ?/, ""));
    else {
      flush();
      out.push(textToHtml(line));
    }
  }
  flush();
  return out.join("");
}

/** A draft's text as the editor's HTML. A reply's quote (splitQuoted) is quoted; the panel folds exactly that part. */
export function draftHtml(body: string, reply: boolean): string {
  if (!reply) return textToHtml(body);
  const { text, quoted } = splitQuoted(body);
  return quoted ? textToHtml(text) + quoteHtml(quoted) : textToHtml(body);
}

const WRAP = `<div style="font-family: ${BASE_FONT}; font-size: ${BASE_SIZE}pt">`;

/** What's sent: the base font around it, and paragraphs single-spaced as in the editor (mail apps space <p> apart, and
 *  show an empty one as nothing). Mentions are links already. */
export function emailHtml(html: string): string {
  if (html.startsWith(WRAP)) return html;
  const spaced = html
    .replace(/<p( style="([^"]*)")?>/g, (_, __, style) => `<p style="margin: 0${style ? `; ${style}` : ""}">`)
    .replace(/(<p [^>]*>)(<\/p>)/g, "$1<br>$2");
  return `${WRAP}${spaced}</div>`;
}

const signatureHtml = (sig: string) => `<p></p>${textToHtml(`-- \n${sig}`)}`;

/** swapSignature for the editor's HTML: the old account's signature becomes the new one's (or goes, or is added at
 *  the end of what's written, above tail, a folded quote). An edited signature is left alone. */
export function swapSignatureHtml(html: string, from: string, to: string, tail = ""): string {
  const next = to.trim() ? signatureHtml(to.trim()) : "";
  const old = from.trim() && signatureHtml(from.trim());
  if (old && html.includes(old)) return html.replace(old, next);
  if (old || !next) return html;
  const head = tail && html.endsWith(tail) ? html.slice(0, -tail.length) : html;
  return head + next + (head === html ? "" : tail);
}

type Node = { type: string; text?: string; attrs?: Record<string, unknown>; content?: Node[] };

/** The plain-text version of the editor's document (editor.getJSON()), as mail apps without HTML show it: one line per
 *  paragraph, quoted lines marked "> " (a reply's history must not read as the sender's words), list items dashed or
 *  numbered, mentions as @Name. */
export function plainText(doc: Node): string {
  const inline = (n: Node): string =>
    n.type === "text" ? (n.text ?? "") : n.type === "hardBreak" ? "\n" : n.type === "mention" ? `@${n.attrs?.label ?? ""}` : (n.content ?? []).map(inline).join("");
  const blocks = (nodes: Node[] = []): string[] =>
    nodes.flatMap((n) => {
      if (n.type === "blockquote") return blocks(n.content).map((line) => `> ${line}`.trimEnd());
      if (n.type === "bulletList" || n.type === "orderedList") {
        return (n.content ?? []).flatMap((item, i) => {
          const [first = "", ...rest] = blocks(item.content);
          return [`${n.type === "bulletList" ? "-" : `${i + 1}.`} ${first}`, ...rest.map((l) => `   ${l}`)];
        });
      }
      return n.type === "paragraph" ? inline(n).split("\n") : blocks(n.content);
    });
  return blocks(doc.content).join("\n");
}
