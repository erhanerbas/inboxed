import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync("src/app.css", "utf8");
const dark = css.slice(css.indexOf(':root[data-theme="dark"]'));

// Light values live in @theme; the dark theme overrides some of them.
function token(name: string, theme: "light" | "dark"): string {
  const find = (src: string) => src.match(new RegExp(`--color-${name}:\\s*(#[0-9a-f]{6})\\b`, "i"))?.[1];
  const value = (theme === "dark" && find(dark.slice(0, dark.indexOf("}")))) || find(css);
  if (!value) throw new Error(`missing 6-digit hex token --color-${name}`);
  return value;
}

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

const surfaces = ["canvas", "sidebar", "panel", "raised", "float", "selected"];

for (const theme of ["light", "dark"] as const) {
  describe(`${theme} theme`, () => {
    for (const text of ["ink", "ink-muted"]) {
      for (const surface of surfaces) {
        it(`${text} reaches 4.5:1 on ${surface}`, () => {
          expect(contrast(token(text, theme), token(surface, theme))).toBeGreaterThanOrEqual(4.5);
        });
      }
    }
    // Accent marks (unread dots, stars, focus rings, selection bars) are not text: 3:1 is the bar.
    for (const surface of surfaces) {
      it(`accent reaches 3:1 on ${surface}`, () => {
        expect(contrast(token("accent", theme), token(surface, theme))).toBeGreaterThanOrEqual(3);
      });
    }
    it("labels on accent and danger fills reach 4.5:1", () => {
      expect(contrast(token("on-accent", theme), token("accent", theme))).toBeGreaterThanOrEqual(4.5);
      expect(contrast(token("on-danger", theme), token("danger", theme))).toBeGreaterThanOrEqual(4.5);
    });
  });
}
