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

// The selector contains only an opaque record ID. Immutable per-save entries
// prevent a stale tab's read/remove pair from deleting a concurrent new login.
const legacyCredentialKey = "nestworth.mobile.session.v1";
const credentialKey = "nestworth.mobile.session.v2";
const entryPrefix = `${credentialKey}.`;
export type RememberedCredential = { accessToken: string; expiresAt: string };
const knownEntries = new Map<string, Set<string>>();
function entryKey(id: string | null) {
  return id && /^[a-f0-9]{32}$/.test(id) ? entryPrefix + id : null;
}
function parseCredential(raw: string | null): RememberedCredential | null {
  if (!raw) return null;
  try {
    const value = JSON.parse(raw);
    if (
      typeof value?.accessToken !== "string" ||
      !/^[A-Za-z0-9_-]{43}$/.test(value.accessToken) ||
      typeof value.expiresAt !== "string" ||
      !Number.isFinite(Date.parse(value.expiresAt))
    )
      return null;
    return { accessToken: value.accessToken, expiresAt: value.expiresAt };
  } catch {
    return null;
  }
}
function removeEntry(key: string, raw: string | null) {
  // New credentials always have different entry keys, even if another tab
  // publishes between this comparison and removal. Never remove the selector.
  if (raw !== null && localStorage.getItem(key) === raw)
    localStorage.removeItem(key);
}
function trackEntry(value: RememberedCredential, key: string) {
  const entries = knownEntries.get(value.accessToken) ?? new Set<string>();
  entries.add(key);
  knownEntries.set(value.accessToken, entries);
}
export function forgetMobileCredential(expectedToken: string) {
  if (!expectedToken) return;
  try {
    const keys = new Set(knownEntries.get(expectedToken));
    const selected = entryKey(localStorage.getItem(credentialKey));
    if (selected) keys.add(selected);
    keys.add(legacyCredentialKey);
    for (const key of keys) {
      const raw = localStorage.getItem(key);
      if (parseCredential(raw)?.accessToken === expectedToken)
        removeEntry(key, raw);
    }
  } catch {
    /* Storage unavailable; do not guess which credential to remove. */
  }
  knownEntries.delete(expectedToken);
}
export function rememberedMobileCredential(): RememberedCredential | null {
  try {
    // A concurrent publication can leave an unselected entry. Reap only expired
    // immutable entries; never sweep a different tab's live/in-progress save.
    for (const key of Object.keys(localStorage)) {
      if (!key.startsWith(entryPrefix)) continue;
      const raw = localStorage.getItem(key);
      const value = parseCredential(raw);
      if (!value || Date.parse(value.expiresAt) <= Date.now())
        removeEntry(key, raw);
    }
    const selected = localStorage.getItem(credentialKey);
    // Once v2 is selected, missing/revoked records never fall back to old grants.
    const key = selected === null ? legacyCredentialKey : entryKey(selected);
    if (!key) return null;
    const raw = localStorage.getItem(key);
    const value = parseCredential(raw);
    if (!value || Date.parse(value.expiresAt) <= Date.now()) {
      removeEntry(key, raw);
      return null;
    }
    trackEntry(value, key);
    return value;
  } catch {
    return null;
  }
}
export function rememberMobileCredential(value: RememberedCredential): boolean {
  let key: string | undefined;
  const raw = JSON.stringify(value);
  try {
    const previousKey = entryKey(localStorage.getItem(credentialKey));
    const previousRaw = previousKey ? localStorage.getItem(previousKey) : null;
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    const id = Array.from(bytes, (byte) =>
      byte.toString(16).padStart(2, "0"),
    ).join("");
    key = entryPrefix + id;
    localStorage.setItem(key, raw);
    localStorage.setItem(credentialKey, id);
    if (
      localStorage.getItem(credentialKey) !== id ||
      localStorage.getItem(key) !== raw
    ) {
      removeEntry(key, raw);
      return false;
    }
    trackEntry(value, key);
    try {
      if (previousKey) removeEntry(previousKey, previousRaw);
    } catch {
      /* The new entry was published; old-entry cleanup is optional. */
    }
    return true;
  } catch {
    // A failed/losing publication may remove only its own immutable entry.
    try {
      if (key) removeEntry(key, raw);
    } catch {
      /* Storage unavailable. */
    }
    return false;
  }
}
export function onMobileCredentialStorageChange(callback: () => void) {
  const listener = (event: StorageEvent) => {
    try {
      if (event.storageArea && event.storageArea !== localStorage) return;
    } catch {
      return;
    }
    if (
      event.key === null ||
      event.key === legacyCredentialKey ||
      event.key === credentialKey ||
      event.key.startsWith(entryPrefix)
    )
      callback();
  };
  window.addEventListener("storage", listener);
  return () => window.removeEventListener("storage", listener);
}
