import { afterEach, expect, it } from "vitest";
import i18n, { resolveLanguage } from "@/i18n";

afterEach(async () => { await i18n.changeLanguage("en"); });

it("uses English singular and plural forms for count sentences", () => {
  expect(i18n.t("connections.gapDays", { count: 1 })).toBe("1 day has incomplete data");
  expect(i18n.t("connections.gapDays", { count: 2 })).toBe("2 days have incomplete data");
  expect(i18n.t("inputBytes.count", { count: 1, limit: 20 })).toBe("1 / 20 byte");
  expect(i18n.t("inputBytes.count", { count: 2, limit: 20 })).toBe("2 / 20 bytes");
  expect(i18n.t("accounts.compositionView.expand", { count: 1 })).toBe("Show all 1 item");
  expect(i18n.t("accounts.compositionView.expand", { count: 3 })).toBe("Show all 3 items");
  expect(i18n.t("valuation.unpriced", { count: 1 })).toBe("1 item has no usable valuation");
  expect(i18n.t("moneyDisplay.missingCount", { count: 1 })).toBe("1 item needs attention");
  expect(i18n.t("reviewV2.summaryCounts", { total: 1, matched: 1, pending: 1, actionable: 1, unknown: 0 })).toBe("1 of 1 investment · 1 needs attention · 1 actionable rule · 0 with missing data");
  expect(i18n.t("diagnosticsSettings.levels.debug")).toBe("Debug");
});

it("translates the same plural keys and the debug log level", async () => {
  await i18n.changeLanguage("zh-CN");
  expect(i18n.t("connections.gapDays", { count: 1 })).toBe("1 天的数据不完整");
  expect(i18n.t("diagnosticsSettings.levels.debug")).toBe("调试");
  expect(i18n.t("diagnosticsSettings.levels.error")).toBe("错误");
  expect(i18n.t("reviewV2.count.needsAttention", { count: 1 })).toBe("1个待关注");
  await i18n.changeLanguage("zh-TW");
  expect(i18n.t("diagnosticsSettings.levels.debug")).toBe("除錯");
  expect(i18n.t("diagnosticsSettings.levels.warn")).toBe("警告");
  expect(i18n.t("institutionType.employer")).toBe("僱主");
  expect(i18n.t("reviewV2.count.needsAttention", { count: 1 })).toBe("1個待關注");
});

it("resolves Japanese system settings and uses Japanese count forms", () => {
  expect(resolveLanguage("system", "ja-JP")).toBe("ja");
  expect(i18n.t("overview.accountCount", { lng: "ja", count: 1 })).toBe("有効な口座 1 件");
  expect(i18n.t("overview.accountCount", { lng: "ja", count: 2 })).toBe("有効な口座 2 件");
  expect(i18n.t("analytics.missingQuotes", { lng: "ja", count: 1 })).toBe("1 件の相場データがありません");
  expect(i18n.t("analytics.missingQuotes", { lng: "ja", count: 2 })).toBe("2 件の相場データがありません");
  expect(i18n.t("analytics.snapshotNeedsRepair", { lng: "ja", date: "2026-01-01" })).toBe("2026-01-01 以降の履歴スナップショットを修復する必要があります。");
});

it("distinguishes Japanese funding, attribution, institution and account terms", () => {
  expect(i18n.t("history.reason.contribution", { lng: "ja" })).toBe("資金の拠出");
  expect(i18n.t("insights.contribution", { lng: "ja" })).toBe("寄与");
  expect(i18n.t("institutionType.brokerage", { lng: "ja" })).toBe("証券会社");
  expect(i18n.t("icons.brokerage", { lng: "ja" })).toBe("証券会社");
  expect(i18n.t("enum.brokerage", { lng: "ja" })).toBe("証券口座");
  expect(i18n.t("valuation.estimatedChange", { lng: "ja" })).toContain("資金の流入・流出");
});

it("preserves the meaning of Japanese market-data coverage and repair notices", () => {
  expect(i18n.t("marketData.work.missing_or_due", { lng: "ja" })).toBe("価格がないか、更新時期です");
  expect(i18n.t("portfolio.completenessDescription", { lng: "ja" })).toContain("ゼロとして扱われることはありません");
  expect(i18n.t("connections.currentVsHistory", { lng: "ja" })).toContain("現在の相場では過去のデータ不足を補えません");
});

it("keeps Japanese backup and agent safety explanations explicit", () => {
  expect(i18n.t("cloudBackup.connectionPassed", { lng: "ja" })).toContain("書き込み権限を保証しません");
  expect(i18n.t("cloudBackup.recoveryHelp", { lng: "ja" })).toContain("スキーマ 21、22、23");
  expect(i18n.t("agent.writeHelp", { lng: "ja" })).toContain("書き込みごとに確認を求めません");
  expect(i18n.t("retention.warning", { lng: "ja" })).toContain("元に戻せません");
});
