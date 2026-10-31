import { expect, it } from "vitest";

const sources = import.meta.glob<string>(
  ["./*.ts", "./*.tsx", "../components/ui/date-picker-control.tsx"],
  { query: "?raw", import: "default", eager: true },
);
it("keeps mobile source independent of desktop runtime and never persists financial request bodies", () => {
  for (const [file, source] of Object.entries(sources)) {
    if (file.includes(".test.")) continue;
    if (!file.endsWith("/connection.ts")) expect(source, file).not.toMatch(/localStorage/);
    expect(source, file).not.toMatch(
      /@wailsio|bindings\/|@\/queries\/|dangerouslySetInnerHTML|serviceWorker/,
    );
  }
});
