import i18n, { type BackendModule } from "i18next";
import { initReactI18next } from "react-i18next";
import {
  errorCodesEn,
  errorCodesZhCN,
  errorCodesZhTW,
} from "@/i18n/errorCodes";
import {
  marketHealthEn,
  marketHealthZhCN,
  marketHealthZhTW,
} from "@/i18n/marketHealth";
import { deepMerge } from "@/i18n/deepMerge";

export async function loadMobileTranslation(language: string) {
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
  return deepMerge(resource.default, {
    dataHealth: additions.dataHealth,
    marketData: additions.marketData,
  });
}

import { mobilePreferences } from "./connection";

const backend: BackendModule = {
  type: "backend",
  init() {},
  read(language, namespace, callback) {
    if (namespace === "errorCode") {
      callback(
        null,
        language === "zh-CN"
          ? errorCodesZhCN
          : language === "zh-TW"
            ? errorCodesZhTW
            : errorCodesEn,
      );
      return;
    }
    void loadMobileTranslation(language)
      .then((resource) => callback(null, resource))
      .catch((error) => callback(error, false));
  },
};
export function initializeMobileI18n() {
  const locale = (navigator.language || "en").toLowerCase();
  const language = locale.startsWith("zh")
    ? /hant|tw|hk|mo/.test(locale)
      ? "zh-TW"
      : "zh-CN"
    : "en";
  return i18n
    .use(backend)
    .use(initReactI18next)
    .init({
      lng: mobilePreferences().language ?? language,
      fallbackLng: "en",
      supportedLngs: ["en", "zh-CN", "zh-TW"],
      load: "currentOnly",
      ns: ["translation", "errorCode"],
      defaultNS: "translation",
      interpolation: { escapeValue: false },
      returnNull: false,
    });
}
