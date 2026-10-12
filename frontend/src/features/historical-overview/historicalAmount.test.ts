import { expect, it } from "vitest";
import i18n from "@/i18n";
import { formatAmount } from "@/lib/money";
import { formatHistoricalAmount } from "./historicalAmount";

it.each(["en", "zh-CN", "zh-TW"])("keeps signed sub-cent values visible in %s", async locale => {
  await i18n.changeLanguage(locale);
  expect(formatHistoricalAmount("0.00012", "CNY", true)).toBe("+0.00012 CNY");
  expect(formatHistoricalAmount("-0.00012", "CNY", true)).toBe("-0.00012 CNY");
  expect(formatHistoricalAmount("-0.00012", "CNY")).toBe("-0.00012 CNY");
  expect(formatHistoricalAmount("-0.01", "CNY")).toBe(formatAmount("-0.01", "CNY"));
  expect(formatHistoricalAmount("0", "CNY", true)).toBe(formatAmount("0", "CNY"));
});
