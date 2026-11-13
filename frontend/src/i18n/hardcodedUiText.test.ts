import { describe, expect, it } from "vitest";

const allowedLiterals = new Set(["N", "Nestworth", "English"]);
const sourceFiles = import.meta.glob(["../features/**/*.tsx", "../app/**/*.tsx", "../components/**/*.tsx", "../mobile/**/*.tsx"], {
  eager: true,
  import: "default",
  query: "?raw",
}) as Record<string, string>;

describe("user-facing UI text boundaries", () => {
  it("keeps visible feature text in the translation catalog", () => {
    const literalPattern = />\s*([A-Z][A-Za-z]+(?:\s+[A-Za-z][A-Za-z-]*)*)\s*<\/[A-Za-z]/g;
    const attributePattern = /\b(?:aria-label|placeholder|title|alt)=(["'])([A-Z][A-Za-z]+(?:\s+[A-Za-z][A-Za-z-]*)*)\1/g;
    const violations = Object.entries(sourceFiles).flatMap(([file, source]) => {
      if (file.endsWith(".test.tsx")) {
        return [];
      }
      const texts = [
        ...Array.from(source.matchAll(literalPattern), match => match[1]),
        ...Array.from(source.matchAll(attributePattern), match => match[2]),
      ];
      return texts
        .map((text) => ({ file, text: text.trim() }))
        .filter(({ text }) => !allowedLiterals.has(text));
    });

    expect(violations).toEqual([]);
  });
});
