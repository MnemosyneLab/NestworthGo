import { describe, expect, it } from "vitest";
import en from "./locales/en.json";
import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";
import fieldLabelKeysEn from "./locales/fieldLabelKeys.json";
import { errorCodesEn, errorCodesZhCN, errorCodesZhTW } from "./errorCodes";
import { additionsEn, additionsZhCN, additionsZhTW } from "./additions";
import { deepMerge } from "./deepMerge";

/**
 * Every i18next key that exists in one locale bundle must exist in every
 * other supported locale, so no `zh-CN`/`zh-TW` render falls back to a raw
 * key or to English text.
 */

type Tree = Record<string, unknown>;

function flattenKeys(tree: Tree, prefix = ""): string[] {
  const keys: string[] = [];
  for (const [key, value] of Object.entries(tree)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === "object" && !Array.isArray(value)) {
      keys.push(...flattenKeys(value as Tree, path));
    } else {
      keys.push(path);
    }
  }
  return keys;
}

function assertSameKeySet(label: string, base: Tree, others: Record<string, Tree>) {
  const baseKeys = new Set(flattenKeys(base));
  for (const [locale, tree] of Object.entries(others)) {
    const otherKeys = new Set(flattenKeys(tree));
    const missing = [...baseKeys].filter((key) => !otherKeys.has(key));
    const extra = [...otherKeys].filter((key) => !baseKeys.has(key));
    expect({ locale, label, missing }).toEqual({ locale, label, missing: [] });
    expect({ locale, label, extra }).toEqual({ locale, label, extra: [] });
  }
}

describe("i18next locale coverage", () => {
  const insightsKeys = [
    "nav.returnAnalysis",
    "nav.assetChanges",
    "insights.returnAnalysis",
    "insights.assetChanges",
    "insights.description",
    "insights.calendar",
    "insights.trend",
    "insights.contribution",
    "insights.drivers",
    "insights.assetTrend",
    "insights.categories",
    "insights.filters",
    "insights.moreFilters",
    "insights.scope",
    "insights.currency",
    "insights.assetClass",
    "insights.member",
    "insights.instrument",
    "insights.valuation",
    "insights.valuationBase",
    "insights.valuationNative",
    "insights.valuationForced",
    "insights.cash",
    "insights.cashIncluded",
    "insights.cashExcluded",
    "insights.from",
    "insights.to",
    "insights.reset",
    "insights.apply",
    "insights.previous",
    "insights.next",
    "insights.today",
    "insights.month",
    "insights.year",
    "insights.coverage",
    "insights.issues",
    "insights.partial",
    "insights.todayMuted",
    "insights.empty",
    "insights.noInvestments",
    "insights.noInvestmentAssets",
    "insights.noInvestmentAssetsHint",
    "insights.historyInsufficient",
    "insights.historyInsufficientHint",
    "insights.noOrigin",
    "insights.noOriginHint",
    "insights.scopeRequired",
    "insights.scopeRequiredHint",
    "insights.loading",
    "insights.error",
    "insights.summary",
    "insights.beginning",
    "insights.ending",
    "insights.returnAmount",
    "insights.returnRate",
    "insights.composition",
    "insights.contributors",
    "insights.daySheet",
    "insights.close",
    "insights.viewAssetChanges",
    "insights.noData",
    "insights.placeholder",
    "insights.notAvailableYet",
    "insights.components.priceChange",
    "insights.components.fxImpact",
    "insights.components.dividendInterest",
    "insights.components.investmentFee",
    "insights.components.cashFxImpact",
    "insights.components.residual",
    "common.all",
  ];
  const changeDriversKeys = [
    "insights.assetChangesDescription",
    "insights.netWorthChange",
    "insights.accountValueChange",
    "insights.assetValueChange",
    "insights.investmentAssetChange",
    "insights.beginningValue",
    "insights.endingValue",
    "insights.change",
    "insights.amount",
    "insights.attribution",
    "insights.cashFlows",
    "insights.market",
    "insights.other",
    "insights.residualGroup",
    "insights.externalFlows",
    "insights.income",
    "insights.spending",
    "insights.liabilityImpact",
    "insights.adjustments",
    "insights.total",
    "insights.byInstrument",
    "insights.byAccount",
    "insights.componentDays",
    "insights.viewInHistory",
    "insights.openReturnAnalysis",
    "insights.driverDetails",
    "insights.noDriverDetails",
    "insights.noAssetChangeData",
    "insights.noAssetChangeDataHint",
    "insights.unavailableAmount",
    "insights.waterfallMismatch",
    "insights.waterfallIncomplete",
    "insights.residualIssues",
    "insights.historyRangeWider",
  ];

  it("the ported translation catalog has identical keys across en/zh-CN/zh-TW", () => {
    assertSameKeySet("translation", en, { "zh-CN": zhCN, "zh-TW": zhTW });
  });

  it("the additions.ts catalog has identical keys across en/zh-CN/zh-TW", () => {
    assertSameKeySet("additions", additionsEn, { "zh-CN": additionsZhCN, "zh-TW": additionsZhTW });
  });

  it("contains the Insights vocabulary in every locale", () => {
    for (const [locale, tree] of Object.entries({ en: additionsEn, "zh-CN": additionsZhCN, "zh-TW": additionsZhTW })) {
      const keys = new Set(flattenKeys(tree));
      expect({ locale, missing: insightsKeys.filter((key) => !keys.has(key)) }).toEqual({ locale, missing: [] });
    }
  });

  it("contains the Change Drivers vocabulary in every locale", () => {
    for (const [locale, tree] of Object.entries({ en: additionsEn, "zh-CN": additionsZhCN, "zh-TW": additionsZhTW })) {
      const keys = new Set(flattenKeys(tree));
      expect({ locale, missing: changeDriversKeys.filter((key) => !keys.has(key)) }).toEqual({ locale, missing: [] });
    }
  });

  it("the errorCode namespace has identical keys across en/zh-CN/zh-TW, keyed 1:1 by domain.ErrorCode", () => {
    assertSameKeySet("errorCode", errorCodesEn, { "zh-CN": errorCodesZhCN, "zh-TW": errorCodesZhTW });
  });

  it("no locale has an empty string for a key present in another locale (a common silent-drop mistake)", () => {
    const merged = {
      en: deepMerge(en, additionsEn),
      "zh-CN": deepMerge(zhCN, additionsZhCN),
      "zh-TW": deepMerge(zhTW, additionsZhTW),
    };
    for (const [locale, tree] of Object.entries(merged)) {
      const emptyKeys = flattenKeys(tree).filter((key) => {
        const value = key.split(".").reduce<unknown>((node, segment) => (node as Tree)?.[segment], tree);
        return typeof value === "string" && value.trim() === "";
      });
      expect({ locale, emptyKeys }).toEqual({ locale, emptyKeys: [] });
    }
  });

  it("fieldLabelKeys (error field -> i18next key map) covers a key for every field used across the ported error.field.* namespace", () => {
    const fieldLabelValues = Object.values(fieldLabelKeysEn as Record<string, string>);
    const translationKeys = new Set(flattenKeys(en));
    const missingTargets = fieldLabelValues.filter((targetKey) => !translationKeys.has(targetKey));
    expect(missingTargets).toEqual([]);
  });
});
