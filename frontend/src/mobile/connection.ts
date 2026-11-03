// Only nonsecret preferences belong here. Sessions, tokens, drafts and financial
// query data remain in memory. Recovery references are separate session metadata.
export class ConnectionError extends Error {
  constructor(
    public code:
      | "transport"
      | "timeout"
      | "cancelled"
      | "invalid_response",
  ) {
    super(code);
  }
}
export type MobilePreferences = {
  language?: string;
  theme: "system" | "light" | "dark";
};
const preferencesKey = "nestworth.mobile.preferences.v1";
export function mobilePreferences(): MobilePreferences {
  try {
    const p = JSON.parse(localStorage.getItem(preferencesKey) ?? "{}");
    return {
      language: ["en", "zh-CN", "zh-TW"].includes(p.language)
        ? p.language
        : undefined,
      theme: ["light", "dark"].includes(p.theme) ? p.theme : "system",
    };
  } catch {
    return { theme: "system" };
  }
}
export function saveMobilePreferences(value: MobilePreferences) {
  try {
    localStorage.setItem(preferencesKey, JSON.stringify(value));
  } catch {
    /* Optional preferences only. */
  }
}
