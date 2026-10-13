import i18next from "@/i18n";
import fieldLabelKeys from "@/i18n/locales/fieldLabelKeys.json";
import * as WailsRuntime from "@wailsio/runtime";

// beta.28 reports a late native error on a separate rejected promise after
// cancel(), outside the caller's catch. Retain only weak identities of reads
// explicitly abandoned by this helper, even after its finally has run.
const abandonedNativeReads = new WeakSet<Promise<unknown>>();
let cancellationRejectionListenerInstalled = false;

function registerAbandonedNativeRead(pending: Promise<unknown>) {
  abandonedNativeReads.add(pending);
  if (cancellationRejectionListenerInstalled || typeof window === "undefined") return;
  window.addEventListener("unhandledrejection", event => {
    const error: unknown = event.reason;
    // Check identity first. Ordinary errors, live calls, other cancelled calls
    // and programming errors on an abandoned call must remain observable.
    if (typeof error !== "object" || error === null || !("promise" in error)
      || !abandonedNativeReads.has(error.promise as Promise<unknown>)) return;
    if (!(error instanceof WailsRuntime.CancelledRejectionError)) return;
    // beta.28 sets Error.cause at runtime; our ES2020 type library omits it.
    const cause = (error as Error & { cause?: unknown }).cause;
    if (!(cause instanceof WailsRuntime.Call.RuntimeError)) return;
    // The pinned runtime uses different info prefixes for cancellation callback
    // exceptions/rejections. Those failures must not be mistaken for a late
    // Call transport reply, even when they reference the same abandoned promise.
    if (!error.message.startsWith("Unhandled rejection in cancelled promise. Reason: ")) return;
    event.preventDefault();
    abandonedNativeReads.delete(error.promise);
  });
  cancellationRejectionListenerInstalled = true;
}

/**
 * WireError mirrors internal/wailsapi/apierror.WireError, the JSON shape
 * every internal/wailsapi method rejects with.
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
 * crashing the caller.
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
 * a localized field label reused from the `error.field.*` catalog namespace.
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

/** Keep the native cancellation handle before awaiting/normalizing a binding.
 * Only an aborted observer is an expected cancellation; live failures still
 * use the regular wire-error contract. Never wait for cancellation delivery
 * before starting the replacement request.
 */
export async function callCancellableService<T>(
  signal: AbortSignal,
  call: () => Promise<T> & { cancel?: () => unknown },
): Promise<T> {
  const checkAborted = () => {
    // Early macOS 12 WebKit has AbortSignal but not throwIfAborted/reason.
    if (signal.aborted) throw signal.reason ?? new DOMException("The operation was aborted.", "AbortError");
  };
  checkAborted();
  let abort: (() => void) | undefined;
  try {
    const pending = call();
    abort = () => {
      registerAbandonedNativeRead(pending);
      try { void Promise.resolve(pending.cancel?.()).catch(() => {}); }
      catch { /* Cancellation delivery must not block the replacement query. */ }
    };
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) abort();
    const result = await pending;
    checkAborted();
    return result;
  } catch (error) {
    checkAborted();
    return await callService(() => Promise.reject(error));
  } finally {
    if (abort) signal.removeEventListener("abort", abort);
  }
}
