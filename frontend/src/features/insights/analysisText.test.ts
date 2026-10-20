import { afterEach, expect, it } from "vitest";
import i18n from "@/i18n";
import { analysisReason } from "./analysisText";

afterEach(async () => { await i18n.changeLanguage("en"); });

it("maps the pending-return sentence and replaces unknown codes", async () => {
  expect(analysisReason(i18n.t, "valuation available; return pending")).toBe("Valuation is available; the return is still pending.");
  expect(analysisReason(i18n.t, "not_a_real_code")).toBe("Unknown");
  expect(analysisReason(i18n.t, "rate coverage is partial")).toBe("Rate coverage is partial.");
  expect(analysisReason(i18n.t, "funding cash is recorded after the product start")).toBe("Funding cash is recorded after the product start.");
  await i18n.changeLanguage("zh-CN");
  expect(analysisReason(i18n.t, "valuation available; return pending")).toBe("可显示估值，收益尚未确认。");
  expect(analysisReason(i18n.t, "funding cash is recorded after the product start")).toBe("资金到账记录晚于产品起始日。");
  expect(analysisReason(i18n.t, "not_a_real_code")).toBe("未知");
  await i18n.changeLanguage("zh-TW");
  expect(analysisReason(i18n.t, "valuation available; return pending")).toBe("可顯示估值，收益尚未確認。");
  expect(analysisReason(i18n.t, "funding cash is recorded after the product start")).toBe("資金入帳記錄晚於產品起始日。");
  expect(analysisReason(i18n.t, "not_a_real_code")).toBe("未知");
});
