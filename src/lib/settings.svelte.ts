// What the person sets in Settings, remembered in localStorage (it's about this computer's reading, not the mail).
import inter from "@fontsource-variable/inter/wght.css?inline";
import interItalic from "@fontsource-variable/inter/wght-italic.css?inline";
import geist from "@fontsource-variable/geist/wght.css?inline";
import geistItalic from "@fontsource-variable/geist/wght-italic.css?inline";
import plex from "@fontsource-variable/ibm-plex-sans/wght.css?inline";
import plexItalic from "@fontsource-variable/ibm-plex-sans/wght-italic.css?inline";
import literata from "@fontsource-variable/literata/wght.css?inline";
import literataItalic from "@fontsource-variable/literata/wght-italic.css?inline";
import garamond from "@fontsource-variable/eb-garamond/wght.css?inline";
import garamondItalic from "@fontsource-variable/eb-garamond/wght-italic.css?inline";
import sourceSerif from "@fontsource-variable/source-serif-4/wght.css?inline";
import sourceSerifItalic from "@fontsource-variable/source-serif-4/wght-italic.css?inline";

export type MailFont = { id: string; name: string; family: string; css: string };

const SANS = `system-ui, -apple-system, "Segoe UI", sans-serif`;
const SERIF = `Georgia, "Times New Roman", serif`;

/** The fonts mail can be read in. css is the font's @font-face rules (bundled files, every subset, both styles), so
 *  the mail frame can load it too. */
export const FONTS: MailFont[] = [
  { id: "inter", name: "Inter", family: `"Inter Variable", ${SANS}`, css: inter + interItalic },
  { id: "geist", name: "Geist", family: `"Geist Variable", ${SANS}`, css: geist + geistItalic },
  { id: "plex", name: "IBM Plex Sans", family: `"IBM Plex Sans Variable", ${SANS}`, css: plex + plexItalic },
  { id: "literata", name: "Literata", family: `"Literata Variable", ${SERIF}`, css: literata + literataItalic },
  { id: "garamond", name: "EB Garamond", family: `"EB Garamond Variable", ${SERIF}`, css: garamond + garamondItalic },
  { id: "source-serif", name: "Source Serif", family: `"Source Serif 4 Variable", ${SERIF}`, css: sourceSerif + sourceSerifItalic },
  { id: "system", name: "System", family: `-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`, css: "" },
];

const KEY = "inboxed.mailFont";

const stored = () => {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
};

export const settings = $state({ mailFont: FONTS.some((f) => f.id === stored()) ? stored()! : "inter" });

export const mailFont = () => FONTS.find((f) => f.id === settings.mailFont) ?? FONTS[0];

export function setMailFont(id: string) {
  if (!FONTS.some((f) => f.id === id)) return;
  settings.mailFont = id;
  try {
    localStorage.setItem(KEY, id);
  } catch {
    // Not remembered; the choice still holds for this session.
  }
}

// The page itself needs the faces too: plain-text mail, and Settings shows each name in its own font. Files load only
// when text uses them.
if (typeof document !== "undefined") {
  const style = document.createElement("style");
  style.textContent = FONTS.map((f) => f.css).join("\n");
  document.head.append(style);
}
