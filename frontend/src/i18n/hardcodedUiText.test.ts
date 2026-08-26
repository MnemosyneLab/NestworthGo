import { describe, expect, it } from "vitest";

const allowedLiterals = new Set(["N", "Nestworth"]);
const sourceFiles = import.meta.glob(["../features/**/*.tsx", "../app/**/*.tsx", "../components/**/*.tsx"], {
  eager: true,
  import: "default",
  query: "?raw",
}) as Record<string, string>;

describe("user-facing UI text boundaries", () => {
  it("keeps visible feature text in the translation catalog", () => {
    const literalPattern = />\s*([A-Z][A-Za-z]+(?:\s+[A-Za-z][A-Za-z-]*)*)\s*<\/[A-Za-z]/g;
    const violations = Object.entries(sourceFiles).flatMap(([file, source]) => {
      if (file.endsWith(".test.tsx")) {
        return [];
      }
      return [...source.matchAll(literalPattern)]
        .map((match) => ({ file, text: match[1].trim() }))
        .filter(({ text }) => !allowedLiterals.has(text));
    });

    expect(violations).toEqual([]);
  });
});
