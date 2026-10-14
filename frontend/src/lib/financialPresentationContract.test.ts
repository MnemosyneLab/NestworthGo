import { expect, it } from "vitest";

const sources = import.meta.glob<string>([
  "../features/marketdata/MarketDataPage.tsx", "../features/marketdata/QuoteHistorySheet.tsx",
  "../features/investments/InstrumentManagement.tsx", "../features/history/RecordChangeForm.tsx",
  "../features/accounts/AccountActionSheets.tsx", "../features/historical-overview/HistoricalOverviewPage.tsx",
  "../features/historical-overview/HistoricalTrend.tsx",
], { query: "?raw", import: "default", eager: true });

it("keeps unit quotes out of ordinary-money formatting and internal evidence out of GUI text", () => {
  for (const source of Object.values(sources)) {
    expect(source).not.toMatch(/formatAmount\([^,\n]*(?:unitPrice|rawPrice),/);
  }
  expect(sources["../features/historical-overview/HistoricalOverviewPage.tsx"]).not.toMatch(/<dd>\{value\.(?:id|sourceKey|sourcePolicyVersion|priceBasis|timestampBasis)\}/);
  expect(sources["../features/historical-overview/HistoricalTrend.tsx"]).not.toContain("${missing.key}");
});
