import * as WailsRuntime from "@wailsio/runtime";
import { callService } from "./wails";

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
