import type { BackendModule } from "i18next";
import i18n from "@/i18n/runtime";
import { DEFAULT_LANGUAGE, isSupportedLanguage, resolveLanguage, SUPPORTED_LANGUAGES } from "@/i18n/language";
import { initReactI18next } from "react-i18next";
import { errorCodes } from "@/i18n/errorCodes";
import {
  marketHealthEn,
  marketHealthZhCN,
  marketHealthZhTW,
} from "@/i18n/marketHealth";
import { assistantEn, assistantZhCN, assistantZhTW } from "@/i18n/assistant";
import { deepMerge } from "@/i18n/deepMerge";
import { mobilePreferences } from "./connection";

// These desktop additions replace older base-catalog shapes and placeholders.
// Keep the lightweight mobile bundle aligned without importing all desktop
// overlays; mobileLocale tests compare these effective values to desktop.
const sharedOverrides = {
  en: { history: { side: "Side" }, onboarding: { removeMember: "Remove member {{index}}" } },
  "zh-CN": { history: { side: "方向" }, onboarding: { removeMember: "移除成员 {{index}}" } },
  "zh-TW": { history: { side: "方向" }, onboarding: { removeMember: "移除成員 {{index}}" } },
};

export async function loadMobileTranslation(language: string) {
  if (language === "ja") return (await import("@/i18n/locales/ja.json")).default;
  const resource =
    language === "zh-CN"
      ? await import("@/i18n/locales/zh-CN.json")
      : language === "zh-TW"
        ? await import("@/i18n/locales/zh-TW.json")
        : await import("@/i18n/locales/en.json");
  const additions =
    language === "zh-CN"
      ? marketHealthZhCN
      : language === "zh-TW"
        ? marketHealthZhTW
        : marketHealthEn;
  const shared = sharedOverrides[language === "zh-CN" || language === "zh-TW" ? language : "en"];
  return deepMerge(deepMerge(deepMerge(resource.default, language === "zh-CN" ? assistantZhCN : language === "zh-TW" ? assistantZhTW : assistantEn), shared), {
    dataHealth: additions.dataHealth,
    marketData: additions.marketData,
  });
}

const backend: BackendModule = {
  type: "backend",
  init() {},
  read(language, namespace, callback) {
    if (namespace === "errorCode") {
      callback(null, errorCodes[isSupportedLanguage(language) ? language : DEFAULT_LANGUAGE]);
      return;
    }
    void loadMobileTranslation(language)
      .then((resource) => callback(null, resource))
      .catch((error) => callback(error, false));
  },
};
export function initializeMobileI18n() {
  return i18n
    .use(backend)
    .use(initReactI18next)
    .init({
      lng: resolveLanguage(mobilePreferences().language ?? "system"),
      fallbackLng: DEFAULT_LANGUAGE,
      supportedLngs: [...SUPPORTED_LANGUAGES],
      load: "currentOnly",
      ns: ["translation", "errorCode"],
      defaultNS: "translation",
      interpolation: { escapeValue: false },
      returnNull: false,
    });
}
