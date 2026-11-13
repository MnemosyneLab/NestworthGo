import { expect, it } from "vitest";
import i18n from "@/i18n";
import { loadMobileTranslation } from "./i18n";

it.each(["en", "zh-CN", "zh-TW"])(
  "loads canonical actionable health and job translations for %s",
  async (language) => {
    const resource = await loadMobileTranslation(language);
    for (const kind of [
      "missing_manual_price",
      "missing_manual_fx",
      "missing_instrument_history",
      "snapshot_outdated",
    ]) {
      expect(
        resource.dataHealth.kind[kind as keyof typeof resource.dataHealth.kind],
      ).toBe(
        i18n.getResource(language, "translation", `dataHealth.kind.${kind}`),
      );
    }
    expect(resource.mobile.manualHelp).toBe(
      i18n.getResource(language, "translation", "mobile.manualHelp"),
    );
  },
);

it.each(["en", "zh-CN", "zh-TW"])("includes the shared loading label even when absent from every other locale in %s", async language => {
  const resource = await loadMobileTranslation(language);
  expect(resource.common).toHaveProperty("loading");
  expect(i18n.getResource(language, "translation", "common.loading")).toEqual(expect.any(String));
  expect(i18n.t("common.loading", { lng: language })).not.toBe("common.loading");
});

it.each(["en", "zh-CN", "zh-TW"])("loads shared control and activity labels directly in %s", async language => {
  const resource = await loadMobileTranslation(language);
  const labels = {
    "common.retryAction": resource.common.retryAction,
    "history.selectEmpty": resource.history.selectEmpty,
    "history.reasonLabel": resource.history.reasonLabel,
    "history.kind.reversal": resource.history.kind.reversal,
    "datePicker.nextMonth": resource.datePicker.nextMonth,
  };
  for (const [key, label] of Object.entries(labels)) {
    expect(label, key).toEqual(expect.any(String));
    expect(label, key).toBe(i18n.getResource(language, "translation", key));
  }
});
