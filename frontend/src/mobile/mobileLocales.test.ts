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

it.each(SUPPORTED_LANGUAGES)("loads the complete desktop catalog on mobile in %s", async language => {
  const resource = await loadMobileTranslation(language);
  expect(resource).toEqual(i18n.getResourceBundle(language, "translation"));
});

it.each(SUPPORTED_LANGUAGES)("loads every mobile English key and placeholder directly in %s", async language => {
  const flatten = (node: unknown, prefix = "", result = new Map<string, string>()) => {
    if (node !== null && typeof node === "object") {
      for (const [key, value] of Object.entries(node)) flatten(value, prefix ? `${prefix}.${key}` : key, result);
    } else if (typeof node === "string") result.set(prefix, node);
    return result;
  };
  const placeholders = (value: string) => [
    ...[...value.matchAll(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g)].map(match => match[1]).sort(),
    ...[...value.matchAll(/%(?:\d+\$)?[dsfg]/g)].map(match => match[0]),
  ];
  const english = flatten(await loadMobileTranslation("en"));
  const actual = flatten(await loadMobileTranslation(language));
  const pluralForms = new Intl.PluralRules(language).resolvedOptions().pluralCategories;
  const issues: string[] = [];
  for (const [key, value] of english) {
    if (key.endsWith("_one") && !pluralForms.includes("one")) continue;
    const translated = actual.get(key);
    if (!translated?.trim()) issues.push(`${key}: missing or empty`);
    else if (placeholders(value).join("|") !== placeholders(translated).join("|")) issues.push(`${key}: placeholders differ`);
  }
  for (const [key, value] of actual) {
    for (const match of value.matchAll(/\$t\(\s*([^,\s)]+)/g)) {
      const reference = match[1];
      if (!actual.has(reference) && !pluralForms.every(form => actual.has(`${reference}_${form}`))) {
        issues.push(`${key}: nested translation ${reference} is missing`);
      }
    }
  }
  expect(issues).toEqual([]);
});
