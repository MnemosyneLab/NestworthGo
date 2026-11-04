// Persist only an explicitly remembered random session credential and preferences.
// Never store the shared token, CSRF, drafts or financial query data.
export class ConnectionError extends Error {
  constructor(
    public code: "transport" | "timeout" | "cancelled" | "invalid_response",
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

const credentialKey = "nestworth.mobile.session.v1";
export type RememberedCredential = { accessToken: string; expiresAt: string };
export function forgetMobileCredential() {
  try {
    localStorage.removeItem(credentialKey);
  } catch {
    /* Storage unavailable. */
  }
}
export function rememberedMobileCredential(): RememberedCredential | null {
  try {
    const value = JSON.parse(localStorage.getItem(credentialKey) ?? "null");
    if (!value) return null;
    if (
      typeof value.accessToken !== "string" ||
      !/^[A-Za-z0-9_-]{43}$/.test(value.accessToken) ||
      typeof value.expiresAt !== "string" ||
      !Number.isFinite(Date.parse(value.expiresAt)) ||
      Date.parse(value.expiresAt) <= Date.now()
    ) {
      forgetMobileCredential();
      return null;
    }
    return { accessToken: value.accessToken, expiresAt: value.expiresAt };
  } catch {
    forgetMobileCredential();
    return null;
  }
}
export function rememberMobileCredential(value: RememberedCredential): boolean {
  try {
    localStorage.setItem(credentialKey, JSON.stringify(value));
    return localStorage.getItem(credentialKey) === JSON.stringify(value);
  } catch {
    forgetMobileCredential();
    return false;
  }
}
