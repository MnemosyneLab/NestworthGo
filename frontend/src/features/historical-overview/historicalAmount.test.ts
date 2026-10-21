import { expect, it } from "vitest";
import i18n from "@/i18n";
import { formatAmount } from "@/lib/money";
import type { DisplayValuation } from "@/components/valuation/valuationQuality";
import { formatHistoricalAmount, historicalDisplayAmount } from "./historicalAmount";

function valuation(amount: string, availability = "full", coverage = "verified"): DisplayValuation {
  const money = { amount, currency: "USD" };
  return { total: availability === "full" ? money : null, knownSubtotal: money,
    quality: { availability, coverage, includedCount: 2, valuedCount: availability === "full" ? 2 : 1,
      unpricedCount: availability === "full" ? 0 : 1, provisionalCount: coverage === "provisional" ? 1 : 0, carriedCount: 0, staleCount: 0 } };
}

it.each([
  ["1.00505", "1.005", "US$1.01"],
  ["1.01495", "1.015", "US$1.01"],
  ["-1.00505", "-1.005", "-US$1.01"],
  ["-1.01495", "-1.015", "-US$1.01"],
])("rounds complete exact %s once instead of rounding its Money4 projection %s again", (exact, projected, expected) => {
  expect(historicalDisplayAmount(i18n.t, valuation(projected), { amount: exact, currency: "USD" })).toBe(expected);
});

it("keeps partial, provisional-only, currency and unavailable display boundaries", () => {
  const exact = { amount: "1.00505", currency: "USD" };
  expect(historicalDisplayAmount(i18n.t, valuation("1.005", "partial", "provisional"), exact))
    .toBe(`US$1.00 · ${i18n.t("valuation.knownSubtotal")}`);
  expect(historicalDisplayAmount(i18n.t, valuation("1.005", "full", "provisional"), null)).toBe("US$1.00");
  expect(historicalDisplayAmount(i18n.t, valuation("1.005"), { ...exact, currency: "CNY" })).toBe("US$1.00");
  const unavailable = valuation("0", "none", "unavailable");
  unavailable.knownSubtotal = null;
  expect(historicalDisplayAmount(i18n.t, unavailable, exact)).toBe(i18n.t("historicalOverview.unknown"));
  expect(historicalDisplayAmount(i18n.t, undefined, exact)).toBe("US$1.01");
});

it.each(["en"])("keeps signed sub-cent values visible in %s", async locale => {
  await i18n.changeLanguage(locale);
  expect(formatHistoricalAmount("0.00012", "CNY", true)).toBe(formatAmount("0.00012", "CNY"));
  expect(formatHistoricalAmount("-0.00012", "CNY", true)).toBe(formatAmount("-0.00012", "CNY"));
  expect(formatHistoricalAmount("-0.00012", "CNY")).toBe(formatAmount("-0.00012", "CNY"));
  expect(formatHistoricalAmount("-0.01", "CNY")).toBe(formatAmount("-0.01", "CNY"));
  expect(formatHistoricalAmount("0", "CNY", true)).toBe(formatAmount("0", "CNY"));
});
