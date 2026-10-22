import { frameDoc } from "./html";
import type { Attachment } from "./types";

export type PreviewKind = "pdf" | "image" | "docx" | "sheet" | "text";

/** Bigger files offer Save instead: converting them would stall the window. */
export const MAX_PREVIEW = 20 * 1024 ** 2;

const BY_TYPE: Record<string, PreviewKind> = {
  "application/pdf": "pdf",
  "image/png": "image",
  "image/jpeg": "image",
  "image/gif": "image",
  "image/webp": "image",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "sheet",
  "text/csv": "sheet",
  "text/plain": "text",
};
const BY_EXTENSION: Record<string, PreviewKind> = {
  pdf: "pdf",
  png: "image",
  jpg: "image",
  jpeg: "image",
  gif: "image",
  webp: "image",
  docx: "docx",
  xlsx: "sheet",
  csv: "sheet",
  txt: "text",
  md: "text",
  log: "text",
};

/** How Inboxed shows an attachment, or null when it can't (only Save is offered then). Senders often send a generic
 *  type, so the name decides when the type doesn't. */
export function previewKind(a: Pick<Attachment, "name" | "type" | "size">): PreviewKind | null {
  if (a.size > MAX_PREVIEW) return null;
  const ext = a.name.toLowerCase().split(".").pop() ?? "";
  return BY_TYPE[a.type?.toLowerCase().split(";")[0].trim() ?? ""] ?? BY_EXTENSION[ext] ?? null;
}

// Documents bring their own page layout (docx-preview's grey desk and pages; a spreadsheet's grid).
const DOCUMENT = `<style>body { padding: 0; font-size: 13px; }
body > table { margin: 16px; } table { border-collapse: collapse; } td, th { border: 1px solid #d4d4d4; padding: 2px 6px; white-space: nowrap; }
.docx-wrapper { padding: 24px 0 !important; min-height: 100vh; box-sizing: border-box; background: #e5e5e5 !important; }
.docx-wrapper > section.docx { box-shadow: 0 1px 4px rgb(0 0 0 / 0.15) !important; }</style>`;

/** A converted document (Word, Excel) for a sandboxed frame: the mail frame's policy, so it runs nothing and loads
 *  nothing from the web; its pictures are data: URLs. */
export const previewDoc = (html: string) => frameDoc(DOCUMENT + html, false);
