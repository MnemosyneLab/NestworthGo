import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import { assistantEn, assistantZhCN, assistantZhTW } from "../src/i18n/assistant";

// Source inventory is a Node tooling check, separate from browser TypeScript.
it("names every shared MCP and built-in tool in all three languages", () => {
  const root = resolve(process.cwd(), "..");
  const contract = JSON.parse(readFileSync(resolve(root, "internal/mcpserver/testdata/discovery-contract.json"), "utf8"));
  const names = new Set(Object.values(contract).flatMap(mode => Object.keys(mode)));
  // Include opt-in writers omitted from the public default discovery snapshot,
  // as well as the desktop-only tools. New registrations require labels here.
  for (const directory of ["internal/mcpserver/", "internal/builtinai/"]) {
    const dir = resolve(root, directory);
    for (const file of readdirSync(dir).filter(file => file.endsWith(".go") && !file.endsWith("_test.go"))) {
      const source = readFileSync(resolve(dir, file), "utf8");
      for (const match of source.matchAll(/(?:readTool|analysisTool|writeTool|ledgerWriteTool)\([^\n]*?"([a-z][a-z_]+)"|Name: "([a-z][a-z_]+)"/g)) names.add(match[1] || match[2]);
    }
  }
  expect(names.has("list_activities")).toBe(true);
  expect(names.has("operate_page_control")).toBe(true);
  expect(names.has("create_investment_review_rule")).toBe(true);
  for (const bundle of [assistantEn, assistantZhCN, assistantZhTW]) {
    for (const name of names) expect(bundle.aiChat.source, name).toHaveProperty(name);
    expect(Object.keys(bundle.aiChat.source).sort()).toEqual(Object.keys(assistantEn.aiChat.source).sort());
  }
  for (const name of names) {
    expect(assistantZhCN.aiChat.source[name]).toMatch(/[\u3400-\u9fff]/);
    expect(assistantZhTW.aiChat.source[name]).toMatch(/[\u3400-\u9fff]/);
  }
});

