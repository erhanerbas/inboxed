import { describe, expect, it } from "vitest";
import { previewDoc, previewKind } from "./preview";

const file = (name: string, type?: string, size = 1000) => ({ name, type, size });

describe("previewKind", () => {
  it("knows PDFs, pictures, Word, spreadsheets and text, by type or by name", () => {
    expect(previewKind(file("a.bin", "application/pdf"))).toBe("pdf");
    expect(previewKind(file("Report.PDF"))).toBe("pdf");
    expect(previewKind(file("p.jpg", "image/jpeg"))).toBe("image");
    expect(previewKind(file("p.webp"))).toBe("image");
    expect(previewKind(file("x", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"))).toBe("docx");
    expect(previewKind(file("Plan.docx", "application/octet-stream"))).toBe("docx");
    expect(previewKind(file("x", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"))).toBe("sheet");
    expect(previewKind(file("budget.xlsx"))).toBe("sheet");
    expect(previewKind(file("list.csv", "text/csv"))).toBe("sheet");
    expect(previewKind(file("notes.txt", "text/plain"))).toBe("text");
    expect(previewKind(file("README.md"))).toBe("text");
  });
  it("has nothing for old Office formats, slides, unknown files and big ones", () => {
    for (const name of ["old.doc", "old.xls", "deck.pptx", "a.zip", "x.svg"]) expect(previewKind(file(name))).toBe(null);
    expect(previewKind(file("big.pdf", "application/pdf", 21 * 1024 ** 2))).toBe(null);
  });
});

describe("previewDoc", () => {
  it("runs nothing and loads nothing from the web", () => {
    const doc = previewDoc('<a href="javascript:alert(1)">x</a><script>alert(1)</script>');
    expect(doc).toContain("script-src 'none'");
    expect(doc).toContain("default-src 'none'");
    expect(doc).not.toMatch(/img-src[^;]*https/);
  });
});
