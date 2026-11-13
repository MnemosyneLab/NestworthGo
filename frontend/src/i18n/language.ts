export const SUPPORTED_LANGUAGES = ["en", "zh-CN", "zh-TW", "ja"] as const;
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];
export const DEFAULT_LANGUAGE: SupportedLanguage = "en";

export function isSupportedLanguage(value: unknown): value is SupportedLanguage {
  return SUPPORTED_LANGUAGES.some(language => language === value);
}

export function resolveLanguage(preference: string, systemLocale = globalThis.navigator?.language): SupportedLanguage {
  if (isSupportedLanguage(preference)) return preference;
  if (preference !== "system") return DEFAULT_LANGUAGE;

  const parts = (systemLocale ?? "").toLowerCase().trim().split(/[.@]/, 1)[0].split(/[-_]/);
  if (parts[0] === "ja") return "ja";
  if (parts[0] !== "zh") return DEFAULT_LANGUAGE;
  for (const part of parts.slice(1)) {
    if (part === "hant") return "zh-TW";
    if (part === "hans") return "zh-CN";
  }
  return parts.some(part => part === "tw" || part === "hk" || part === "mo") ? "zh-TW" : "zh-CN";
}

/** Maps a settings/catalog language code onto the option.language.* i18next key. */
export function languageOptionKey(language: string): "en" | "zhCN" | "zhTW" | "ja" | "system" {
  switch (language) {
    case "zh-CN": return "zhCN";
    case "zh-TW": return "zhTW";
    case "ja": return "ja";
    case "system": return "system";
    default: return "en";
  }
}
