import i18n from "./runtime";
import { initReactI18next } from "react-i18next";

import en from "./locales/en.json";
import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";
import { errorCodesEn, errorCodesZhCN, errorCodesZhTW } from "./errorCodes";
import { additionsEn, additionsZhCN, additionsZhTW } from "./additions";
import { rebuildDataEn, rebuildDataZhCN, rebuildDataZhTW } from "./rebuildData";
import { upcomingActionsEn, upcomingActionsZhCN, upcomingActionsZhTW } from "./upcomingActions";
import { homepageEn, homepageZhCN, homepageZhTW } from "./homepage";
import { investmentAssetsEn, investmentAssetsZhCN, investmentAssetsZhTW } from "./investmentAssets";
import { assistantEn, assistantZhCN, assistantZhTW } from "./assistant";
import { voiceEn, voiceZhCN, voiceZhTW } from "./voice";
import { tasksEn, tasksZhCN, tasksZhTW } from "./tasks";
import { deepMerge } from "./deepMerge";

import { DEFAULT_LANGUAGE, resolveLanguage } from "./language";
export { DEFAULT_LANGUAGE, resolveLanguage, SUPPORTED_LANGUAGES, type SupportedLanguage } from "./language";

void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(en, voiceEn), assistantEn), additionsEn), rebuildDataEn), upcomingActionsEn), homepageEn), investmentAssetsEn), tasksEn), errorCode: errorCodesEn },
    "zh-CN": { translation: deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(zhCN, voiceZhCN), assistantZhCN), additionsZhCN), rebuildDataZhCN), upcomingActionsZhCN), homepageZhCN), investmentAssetsZhCN), tasksZhCN), errorCode: errorCodesZhCN },
    "zh-TW": { translation: deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(deepMerge(zhTW, voiceZhTW), assistantZhTW), additionsZhTW), rebuildDataZhTW), upcomingActionsZhTW), homepageZhTW), investmentAssetsZhTW), tasksZhTW), errorCode: errorCodesZhTW },
  },
  lng: resolveLanguage("system"),
  fallbackLng: DEFAULT_LANGUAGE,
  ns: ["translation", "errorCode"],
  defaultNS: "translation",
  interpolation: { escapeValue: false },
  returnNull: false,
});

export function setLanguage(preference: string, systemLocale?: string) {
  const resolved = resolveLanguage(preference, systemLocale);
  if (i18n.language !== resolved) void i18n.changeLanguage(resolved);
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
