import { afterEach, expect, it } from "vitest";
import i18n from "@/i18n";

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
