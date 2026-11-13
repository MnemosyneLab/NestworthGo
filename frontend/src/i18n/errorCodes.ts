import type { SupportedLanguage } from "./language";
import en from "./locales/en.errorCode.json";
import ja from "./locales/ja.errorCode.json";
import zhCN from "./locales/zh-CN.errorCode.json";
import zhTW from "./locales/zh-TW.errorCode.json";

// Stable backend error codes form the separate i18next errorCode namespace.
// The locale coverage test checks that each language provides the same keys.
export const errorCodes = {
  en,
  ja,
  "zh-CN": zhCN,
  "zh-TW": zhTW,
} satisfies Record<SupportedLanguage, Record<string, string>>;
