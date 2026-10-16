import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync("src/app.css", "utf8");

function token(name: string): string {
  const match = css.match(new RegExp(`--color-${name}:\\s*(#[0-9a-f]{6})\\b`, "i"));
  if (!match) throw new Error(`missing 6-digit hex token --color-${name}`);
  return match[1];
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

describe("text tokens reach WCAG AA (4.5:1) on every surface", () => {
  for (const text of ["ink", "ink-muted", "accent"]) {
    for (const surface of ["canvas", "sidebar", "panel", "raised", "selected"]) {
      it(`${text} on ${surface}`, () => {
        expect(contrast(token(text), token(surface))).toBeGreaterThanOrEqual(4.5);
      });
    }
  }
});

it("the danger button's label (canvas on danger) reaches 4.5:1", () => {
  expect(contrast(token("canvas"), token("danger"))).toBeGreaterThanOrEqual(4.5);
});
