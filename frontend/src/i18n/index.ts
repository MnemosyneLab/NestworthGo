import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import en from "./locales/en.json";
import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";
import { errorCodesEn, errorCodesZhCN, errorCodesZhTW } from "./errorCodes";
import { additionsEn, additionsZhCN, additionsZhTW } from "./additions";
import { deepMerge } from "./deepMerge";

// The supported locales are English, Simplified Chinese, and Traditional
// Chinese, with complete key coverage enforced by localeCoverage.test.ts.
export const SUPPORTED_LANGUAGES = ["en", "zh-CN", "zh-TW"] as const;
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];

export const DEFAULT_LANGUAGE: SupportedLanguage = "en";

void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: deepMerge(en, additionsEn), errorCode: errorCodesEn },
    "zh-CN": { translation: deepMerge(zhCN, additionsZhCN), errorCode: errorCodesZhCN },
    "zh-TW": { translation: deepMerge(zhTW, additionsZhTW), errorCode: errorCodesZhTW },
  },
  lng: DEFAULT_LANGUAGE,
  fallbackLng: DEFAULT_LANGUAGE,
  ns: ["translation", "errorCode"],
  defaultNS: "translation",
  interpolation: { escapeValue: false },
  returnNull: false,
});

/**
 * Maps `internal/settings.Language` ("system" | "en" | "zh-CN" | "zh-TW")
 * to one of the three loaded i18next languages, resolving "system" against
 * the browser/webview's own locale. SettingsService is the source of truth
 * for the persisted preference; this function only
 * implements the "system" resolution rule client-side, mirroring
 * internal/i18n.ResolveSystemLanguage's zh-Hant/zh-Hans script-tag logic.
 */
export function resolveLanguage(preference: string): SupportedLanguage {
  if (preference === "en" || preference === "zh-CN" || preference === "zh-TW") {
    return preference;
  }
  if (preference !== "system") {
    return DEFAULT_LANGUAGE;
  }
  const locale = (navigator.language || DEFAULT_LANGUAGE).toLowerCase();
  if (!locale.startsWith("zh")) {
    return DEFAULT_LANGUAGE;
  }
  if (locale.includes("hant") || locale.includes("-tw") || locale.includes("-hk") || locale.includes("-mo")) {
    return "zh-TW";
  }
  return "zh-CN";
}

export function setLanguage(preference: string) {
  const resolved = resolveLanguage(preference);
  void i18n.changeLanguage(resolved);
  return resolved;
}

/** Maps a settings/catalog language code onto the option.language.* i18next key. */
export function languageOptionKey(language: string): "en" | "zhCN" | "zhTW" | "system" {
  if (language === "zh-CN") {
    return "zhCN";
  }
  if (language === "zh-TW") {
    return "zhTW";
  }
  if (language === "system") {
    return "system";
  }
  return "en";
}

export default i18n;
