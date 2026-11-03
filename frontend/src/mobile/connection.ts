// Only nonsecret preferences belong here. Sessions, tokens, drafts and financial
// query data remain in memory. Recovery references are separate session metadata.
export class ConnectionError extends Error {
  constructor(
    public code:
      | "invalid_url"
      | "insecure_backend"
      | "transport"
      | "timeout"
      | "cancelled"
      | "invalid_response",
  ) {
    super(code);
  }
}
const settingsKey = "nestworth.mobile.connection.v1";
export function normalizeBackend(value: string): string {
  let input = value.trim();
  if (
    /^(?:\d+\.){3}\d+:\d+$/.test(input) ||
    /^\[[0-9a-f:]+\]:\d+$/i.test(input)
  )
    input = `http://${input}`;
  if (!input) return "";
  let url: URL;
  try {
    url = new URL(input);
  } catch {
    throw new ConnectionError("invalid_url");
  }
  if (url.protocol !== "https:" && url.protocol !== "http:")
    throw new ConnectionError("insecure_backend");
  if (url.protocol === "http:") {
    const host = url.hostname;
    const octets = host.split(".").map(Number);
    const privateV4 =
      octets.length === 4 &&
      (octets[0] === 10 ||
        octets[0] === 127 ||
        (octets[0] === 192 && octets[1] === 168) ||
        (octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31));
    const privateV6 = /^\[(?:f[cd][0-9a-f]{2}:|::1\])/i.test(host);
    // Require an explicit valid port and canonical numeric IP, including IPv6 brackets.
    const authority = input.slice(7).replace(/\/$/, "");
    const port = authority.slice(authority.lastIndexOf(":") + 1);
    if (
      (!privateV4 && !privateV6) ||
      !/^\d+$/.test(port) ||
      +port < 1 ||
      +port > 65535 ||
      authority !== `${host}:${port}`
    )
      throw new ConnectionError("invalid_url");
  }
  if (
    !/^https?:\/\/[^/?#]+\/?$/i.test(input) ||
    [...input].some(
      (char) => char.charCodeAt(0) <= 32 || char.charCodeAt(0) === 127,
    ) ||
    input.includes("\\")
  )
    throw new ConnectionError("invalid_url");
  if (
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    /[?#\\]/.test(input) ||
    url.pathname !== "/" ||
    !url.hostname ||
    url.hostname.includes("*")
  )
    throw new ConnectionError("invalid_url");
  return url.origin === window.location.origin ? "" : url.origin;
}
let backend = "";
try {
  backend = normalizeBackend(localStorage.getItem(settingsKey) ?? "");
} catch {
  /* Invalid saved settings cannot change the default boundary. */
}
export function backendBase() {
  return backend;
}
export function saveBackend(value: string) {
  backend = normalizeBackend(value);
  try {
    localStorage.setItem(settingsKey, backend);
  } catch {
    /* Storage may be disabled; in-memory use still works. */
  }
}

// QR carries an address only. Remove the fragment when showing settings and
// never consume credentials from links. A scan does not change a saved server.
export function takeConnectionHint(): string | null {
  const fragment = window.location.hash;
  if (!fragment.startsWith("#backend=")) return null;
  window.history.replaceState(
    null,
    "",
    window.location.pathname + window.location.search,
  );
  try {
    return normalizeBackend(decodeURIComponent(fragment.slice(9)));
  } catch {
    return null;
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

// HTTP LAN pages use the existing memory-only bearer transport because Secure
// cookies cannot be sent over HTTP. External HTTP targets are opened as pages.
export function usesBearerTransport(base = backendBase()) {
  return !!base || window.location.protocol === "http:";
}
export function httpPageTarget(value: string): string | null {
  const base = normalizeBackend(value);
  return base.startsWith("http://") ? `${base}/mobile.html` : null;
}
