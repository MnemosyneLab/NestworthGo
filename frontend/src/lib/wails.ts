import i18next from "@/i18n";
import fieldLabelKeys from "@/i18n/locales/fieldLabelKeys.json";

/**
 * WireError mirrors internal/wailsapi/apierror.WireError, the JSON shape
 * every internal/wailsapi method rejects with (technical design Sec5).
 */
export interface WireError {
  code: string;
  field?: string;
  message: string;
}

function isWireError(value: unknown): value is WireError {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { code?: unknown }).code === "string" &&
    typeof (value as { message?: unknown }).message === "string"
  );
}

/**
 * parseWailsError recovers a WireError from a Wails-rejected Promise's
 * error. Wails v3 propagates a Go method's non-nil error as a rejected
 * Promise whose message is `err.Error()` — for every internal/wailsapi
 * method this is `apierror.WireError`'s own JSON encoding, but this
 * function is defensive: malformed JSON, a plain string, or a future
 * Wails runtime error that never went through our wrap() must resolve to
 * a safe generic "internal" WireError instead of throwing again or
 * crashing the caller (technical design Sec5).
 */
export function parseWailsError(error: unknown): WireError {
  const raw = extractMessage(error);
  if (raw) {
    try {
      const parsed = JSON.parse(raw);
      if (isWireError(parsed)) {
        return parsed;
      }
    } catch {
      // Not JSON (e.g. a raw Wails runtime error, or a network-layer
      // failure) -- fall through to the generic error below.
    }
  }
  return { code: "internal", message: raw || "An unexpected error occurred" };
}

function extractMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === "string") {
    return error;
  }
  if (error && typeof error === "object" && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string") {
      return message;
    }
  }
  return "";
}

/**
 * translateWailsError resolves a WireError's user-facing message by its
 * stable `code` (never by matching English text), optionally prefixed by
 * a localized field label reused from the ported `error.field.*` catalog
 * namespace, mirroring internal/i18n.Translator.TranslateError's
 * code-first / field-prefixed shape.
 */
export function translateWailsError(error: WireError): string {
  const message = i18next.t(error.code, { ns: "errorCode", defaultValue: i18next.t("internal", { ns: "errorCode" }) });
  const fieldKey = error.field ? (fieldLabelKeys as Record<string, string>)[error.field] : undefined;
  if (fieldKey) {
    const fieldLabel = i18next.t(fieldKey, { defaultValue: "" });
    if (fieldLabel) {
      return `${fieldLabel}: ${message}`;
    }
  }
  return message;
}

/**
 * callService wraps a generated Wails binding call so every caller gets a
 * translated, user-safe error message instead of a raw rejected-Promise
 * error. Use this in TanStack Query queryFn/mutationFn instead of calling
 * a binding directly.
 */
export async function callService<T>(call: () => Promise<T>): Promise<T> {
  try {
    return await call();
  } catch (error) {
    const wireError = parseWailsError(error);
    const translated = new Error(translateWailsError(wireError));
    (translated as Error & { wireError: WireError }).wireError = wireError;
    throw translated;
  }
}
