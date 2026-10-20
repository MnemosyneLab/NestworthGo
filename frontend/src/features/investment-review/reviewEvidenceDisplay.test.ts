import { afterEach, describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { reviewLeafActual, reviewReasonLabel, reviewReasonList } from "./reviewEvidenceDisplay";

const REASON_CODES = [
  "daily_close_missing", "daily_close_ineligible", "invalid_request", "baseline_missing_or_incomparable", "baseline_too_old",
  "daily_performance_unknown", "annualized_unavailable", "unsupported_instrument", "historical_type_unknown", "historical_amount_unknown",
  "currency_mismatch", "fx_missing", "fx_date_untrusted", "fx_stale", "fx_time_untrusted", "nonexistent_local_date", "history_origin",
  "historical_instrument_metadata", "historical_quantity", "historical_cost",
];

afterEach(async () => { await i18n.changeLanguage("en"); });

describe("review evidence display", () => {
  it.each(["en", "zh-CN", "zh-TW"])("translates every engine reason code in %s", async language => {
    await i18n.changeLanguage(language);
    for (const code of REASON_CODES) {
      const label = reviewReasonLabel(i18n.t, code);
      expect(label, code).not.toBe(code);
      expect(label, code).not.toMatch(/_/);
    }
  });

  it("has the requested daily close wording in English and Simplified Chinese", async () => {
    expect(reviewReasonLabel(i18n.t, "daily_close_missing")).toBe("No closing price for this day");
    await i18n.changeLanguage("zh-CN");
    expect(reviewReasonLabel(i18n.t, "daily_close_missing")).toBe("当日无收盘价");
  });

  it("falls back to the raw code for unknown reasons and joins lists with the locale separator", async () => {
    expect(reviewReasonLabel(i18n.t, "future_code")).toBe("future_code");
    expect(reviewReasonList(i18n.t, ["fx_missing", "historical_cost"])).toBe("Exchange rate missing, Cost basis unknown on this day");
    await i18n.changeLanguage("zh-CN");
    expect(reviewReasonList(i18n.t, ["fx_missing", "historical_cost"])).toBe("缺少汇率、当日成本未知");
  });

  it("formats money, percentages and types without reparsing numbers", async () => {
    expect(reviewLeafActual(i18n.t, { metric: "amount", actual: "126.5000", currency: "USD" })).toBe("$126.50");
    expect(reviewLeafActual(i18n.t, { metric: "cost", actual: "1234567.8912", currency: "USD" })).toBe("$1,234,567.89");
    expect(reviewLeafActual(i18n.t, { metric: "price_change", actual: "-3.456" })).toBe("-3.46%");
    expect(reviewLeafActual(i18n.t, { metric: "annualized", actual: "4.5" })).toBe("4.50%");
    expect(reviewLeafActual(i18n.t, { metric: "type", actual: "mutual_fund" })).toBe("Fund");
    expect(reviewLeafActual(i18n.t, { metric: "amount" })).toBe("Unknown");
  });
});

describe("review count plurals", () => {
  const counts = { hits: 1, falseCount: 1, unknowns: 1 };
  it("uses singular and plural English forms", () => {
    expect(i18n.t("reviewV2.windowCounts", counts)).toBe("1 hit · 1 miss · 1 unknown");
    expect(i18n.t("reviewV2.windowCounts", { hits: 0, falseCount: 2, unknowns: 3 })).toBe("0 hits · 2 misses · 3 unknown");
    expect(i18n.t("reviewV2.historyTriggered", { date: "2026-10-02", hits: 1, from: "2026-10-01", to: "2026-10-02" })).toBe("Triggered 2026-10-02 · 1 hit day · 2026-10-01–2026-10-02");
    expect(i18n.t("reviewV2.historyTriggered", { date: "2026-10-02", hits: 2, from: "2026-10-01", to: "2026-10-02" })).toContain("2 hit days");
    expect(i18n.t("reviewV2.ruleSummary", { count: 1, mode: "Single", timezone: "UTC" })).toBe("1 condition · Single · UTC");
    expect(i18n.t("reviewV2.ruleSummary", { count: 2, mode: "Single", timezone: "UTC" })).toBe("2 conditions · Single · UTC");
    expect(i18n.t("reviewV2.unknownDays", { count: 1 })).toBe("1 unknown day");
    expect(i18n.t("reviewV2.unknownDays", { count: 4 })).toBe("4 unknown days");
    expect(i18n.t("reviewV2.windowRolling", { hits: 1, needed: 5, days: 1, unknowns: 0 })).toBe("1/5 hits in 1 day · 0 unknown");
    expect(i18n.t("reviewV2.coverageWindow", { from: "a", to: "b", hits: 1, unknowns: 1 })).toBe("a–b · 1 hit · 1 unknown day");
    expect(i18n.t("reviewV2.previewResult", { from: "a", to: "b", hits: 1, misses: 1, unknowns: 1 })).toBe("a–b: 1 hit, 1 miss, 1 unknown");
    expect(i18n.t("reviewV2.summaryCounts", { total: 1, matched: 1, pending: 1, actionable: 1, unknown: 0 })).toBe("1 of 1 investment · 1 needs attention · 1 actionable rule · 0 with missing data");
    expect(i18n.t("reviewV2.summaryCounts", { total: 5, matched: 2, pending: 2, actionable: 3, unknown: 1 })).toBe("2 of 5 investments · 2 need attention · 3 actionable rules · 1 with missing data");
  });

  it.each([["zh-CN", "命中1天 · 未命中1天 · 数据不足1天", "1项条件 · 单次 · UTC"], ["zh-TW", "命中1天 · 未命中1天 · 資料不足1天", "1項條件 · 單次 · UTC"]])("keeps the original %s wording", async (language, window, rule) => {
    await i18n.changeLanguage(language);
    expect(i18n.t("reviewV2.windowCounts", counts)).toBe(window);
    expect(i18n.t("reviewV2.ruleSummary", { count: 1, mode: language === "zh-CN" ? "单次" : "單次", timezone: "UTC" })).toBe(rule);
    expect(i18n.t("reviewV2.windowRolling", { hits: 5, needed: 5, days: 10, unknowns: 2 })).toBe(language === "zh-CN" ? "10天内命中5/5天 · 2天数据不足" : "10天內命中5/5天 · 2天資料不足");
    expect(i18n.t("reviewV2.summaryCounts", { total: 5, matched: 2, pending: 2, actionable: 3, unknown: 1 })).toBe(language === "zh-CN" ? "共5个标的，筛选匹配2个 · 2个待关注 · 3条可处理规则 · 1个数据不足" : "共5個標的，篩選符合2個 · 2個待關注 · 3條可處理規則 · 1個資料不足");
  });
});
