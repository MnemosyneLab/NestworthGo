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
