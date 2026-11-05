// @vitest-environment node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";

it("maps every backend budget subcode to an exact desktop diagnostic", () => {
  const backend = readFileSync(resolve(process.cwd(), "../internal/assistant/budget_failure.go"), "utf8");
  const frontend = readFileSync(resolve(process.cwd(), "src/features/assistant/ModelConversation.tsx"), "utf8");
  const rules = [...backend.matchAll(/"([a-z_]+)":\s+true/g)].map((match) => match[1]).sort();
  const codes = [...frontend.matchAll(/"assistant_budget_exceeded\.([a-z_]+)":/g)].map((match) => match[1]).sort();
  expect(rules.length).toBeGreaterThan(0);
  expect(codes).toEqual(rules);
});
