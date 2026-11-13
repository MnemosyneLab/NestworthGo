import type { BackendModule } from "i18next";
import i18n from "@/i18n/runtime";
import { DEFAULT_LANGUAGE, isSupportedLanguage, resolveLanguage, SUPPORTED_LANGUAGES } from "@/i18n/language";
import { initReactI18next } from "react-i18next";
import { errorCodes } from "@/i18n/errorCodes";
import { deepMerge } from "@/i18n/deepMerge";
import { mobilePreferences } from "./connection";

export async function loadMobileTranslation(language: string) {
  if (language === "ja") return (await import("@/i18n/locales/ja.json")).default;
  const [resource, voice, assistant, additions, rebuild, upcoming, homepage, investments, tasks] = await Promise.all([
    language === "zh-CN" ? import("@/i18n/locales/zh-CN.json") : language === "zh-TW" ? import("@/i18n/locales/zh-TW.json") : import("@/i18n/locales/en.json"),
    import("@/i18n/voice"), import("@/i18n/assistant"), import("@/i18n/additions"), import("@/i18n/rebuildData"),
    import("@/i18n/upcomingActions"), import("@/i18n/homepage"), import("@/i18n/investmentAssets"), import("@/i18n/tasks"),
  ]);
  // Match desktop's effective resources and merge order for every language.
  // Dynamic imports keep these additions out of the initial mobile entry.
  const layers = language === "zh-CN"
    ? [voice.voiceZhCN, assistant.assistantZhCN, additions.additionsZhCN, rebuild.rebuildDataZhCN, upcoming.upcomingActionsZhCN, homepage.homepageZhCN, investments.investmentAssetsZhCN, tasks.tasksZhCN]
    : language === "zh-TW"
      ? [voice.voiceZhTW, assistant.assistantZhTW, additions.additionsZhTW, rebuild.rebuildDataZhTW, upcoming.upcomingActionsZhTW, homepage.homepageZhTW, investments.investmentAssetsZhTW, tasks.tasksZhTW]
      : [voice.voiceEn, assistant.assistantEn, additions.additionsEn, rebuild.rebuildDataEn, upcoming.upcomingActionsEn, homepage.homepageEn, investments.investmentAssetsEn, tasks.tasksEn];
  let merged = resource.default;
  for (const layer of layers) merged = deepMerge(merged, layer);
  return merged;
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
