export const SUPPORTED_LANGUAGES = ["en", "zh-CN", "zh-TW"] as const;
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];
export const DEFAULT_LANGUAGE: SupportedLanguage = "en";

export function resolveLanguage(preference: string, systemLocale = globalThis.navigator?.language): SupportedLanguage {
  if (preference === "en" || preference === "zh-CN" || preference === "zh-TW") return preference;
  if (preference !== "system") return DEFAULT_LANGUAGE;

  const parts = (systemLocale ?? "").toLowerCase().trim().split(/[.@]/, 1)[0].split(/[-_]/);
  if (parts[0] !== "zh") return DEFAULT_LANGUAGE;
  for (const part of parts.slice(1)) {
    if (part === "hant") return "zh-TW";
    if (part === "hans") return "zh-CN";
  }
  return parts.some(part => part === "tw" || part === "hk" || part === "mo") ? "zh-TW" : "zh-CN";
}
