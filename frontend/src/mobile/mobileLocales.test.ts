import { expect, it } from "vitest";
import i18n, { SUPPORTED_LANGUAGES } from "@/i18n";
import { loadMobileTranslation } from "./i18n";

it.each(SUPPORTED_LANGUAGES)(
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

it.each(SUPPORTED_LANGUAGES)("includes the shared loading label even when absent from every other locale in %s", async language => {
  const resource = await loadMobileTranslation(language);
  expect(resource.common).toHaveProperty("loading");
  expect(i18n.getResource(language, "translation", "common.loading")).toEqual(expect.any(String));
  expect(i18n.t("common.loading", { lng: language })).not.toBe("common.loading");
});

it.each(SUPPORTED_LANGUAGES)("loads shared control and activity labels directly in %s", async language => {
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

it("loads completed Japanese sections on mobile without fallback", async () => {
  const resource = await loadMobileTranslation("ja");
  const visit = (node: unknown, path: string) => {
    if (node !== null && typeof node === "object") {
      for (const [key, value] of Object.entries(node)) visit(value, `${path}.${key}`);
    } else if (typeof node === "string" && !path.endsWith("_one")) {
      const mobile = path.split(".").reduce<unknown>((current, key) =>
        current !== null && typeof current === "object" ? (current as Record<string, unknown>)[key] : undefined, resource);
      expect(mobile, path).toBeTypeOf("string");
      expect(mobile, path).toBe(i18n.getResource("ja", "translation", path));
    }
  };
  for (const section of ["error", "analytics", "dashboard", "groups", "icons", "directory", "institutionType", "marketData", "portfolio", "connections", "institutions", "members", "snapshot", "review", "metals", "holdingsSummary", "agent", "cloudBackup", "secretField", "retention"]) {
    visit(i18n.getResource("en", "translation", section), section);
  }
});
