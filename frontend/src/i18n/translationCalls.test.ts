import { expect, it } from "vitest";
import ts from "typescript";
import i18n, { SUPPORTED_LANGUAGES } from "./index";
import { NAV_GROUPS } from "../app/navigation";
import { SECTION_IDS } from "../features/settings/sections";

const sources = import.meta.glob(["../**/*.ts", "../**/*.tsx", "!../**/*.test.*", "!../test/**", "!../i18n/**"], {
  eager: true, import: "default", query: "?raw",
}) as Record<string, string>;

function literalKeys(expression: ts.Expression): string[] {
  if (ts.isStringLiteralLike(expression)) return [expression.text];
  if (ts.isConditionalExpression(expression)) return [...literalKeys(expression.whenTrue), ...literalKeys(expression.whenFalse)];
  if (ts.isParenthesizedExpression(expression)) return literalKeys(expression.expression);
  return [];
}

it("resolves literal translation calls in each language without English fallback", () => {
  const issues: string[] = [];
  for (const [file, source] of Object.entries(sources)) {
    const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true);
    function visit(node: ts.Node) {
      if (ts.isCallExpression(node) && node.arguments.length &&
        ((ts.isIdentifier(node.expression) && node.expression.text === "t") ||
          (ts.isPropertyAccessExpression(node.expression) && node.expression.name.text === "t"))) {
        for (const key of literalKeys(node.arguments[0])) {
          const options = node.arguments[1];
          const namespace = options && ts.isObjectLiteralExpression(options)
            ? options.properties.find((property): property is ts.PropertyAssignment => ts.isPropertyAssignment(property) && property.name.getText(ast) === "ns")?.initializer
            : undefined;
          const ns = namespace && ts.isStringLiteralLike(namespace) ? namespace.text : "translation";
          for (const language of SUPPORTED_LANGUAGES) {
            const counted = options && (!ts.isObjectLiteralExpression(options) || options.properties.some((property) =>
              ts.isSpreadAssignment(property) || property.name?.getText(ast) === "count"));
            const candidates = counted ? [key, `${key}_one`, `${key}_other`] : [key];
            if (!candidates.some((candidate) => typeof i18n.getResource(language, ns, candidate) === "string")) {
              const line = ast.getLineAndCharacterOfPosition(node.getStart()).line + 1;
              issues.push(`${file}:${line} ${language} ${ns}:${key}`);
            }
          }
        }
      }
      ts.forEachChild(node, visit);
    }
    visit(ast);
  }
  expect(issues).toEqual([]);
});

it("resolves the actual navigation model in every language without fallback", () => {
  const keys = NAV_GROUPS.flatMap((group) => [group.translationKey, ...group.items.map((item) => item.translationKey)]).filter((key): key is string => Boolean(key));
  for (const language of SUPPORTED_LANGUAGES) {
    for (const key of keys) expect(i18n.getResource(language, "translation", key), `${language} ${key}`).toBeTypeOf("string");
  }
});

it.each(SUPPORTED_LANGUAGES)("resolves every dynamic settings tab in %s without fallback", (language) => {
  for (const id of SECTION_IDS) {
    const key = `settings.tabs.${id}`;
    const label = i18n.getResource(language, "translation", key);
    expect(label, `${language} ${key}`).toBeTypeOf("string");
    expect(label.trim(), `${language} ${key}`).not.toBe("");
    expect(label, `${language} ${key}`).not.toBe(key);
  }
  const mobileLabel = i18n.getResource(language, "translation", "settings.tabs.mobile");
  expect(mobileLabel).toBe(i18n.getResource(language, "translation", "mobile.access"));
  expect(mobileLabel).toBe(i18n.getResource(language, "translation", "menu.mobile"));
});
