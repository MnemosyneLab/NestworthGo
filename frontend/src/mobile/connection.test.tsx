import { afterEach, expect, it, vi } from "vitest";
import { ConnectionError, mobilePreferences, saveMobilePreferences } from "./connection";
import { api, setMobileSession } from "./api";
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); setMobileSession(null); localStorage.clear(); });
it("times out a request without treating a confirmation as rejected", async () => {
  vi.useFakeTimers();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      (_url, options: RequestInit) =>
        new Promise((_resolve, reject) => {
          options.signal?.addEventListener("abort", () =>
            reject(new DOMException("aborted", "AbortError")),
          );
        }),
    ),
  );
  const pending = api(
    "/record-confirmations",
    "POST",
    { planId: "immutable-plan" },
    { timeout: 100 },
  ).catch((e) => e);
  await vi.advanceTimersByTimeAsync(101);
  const error = await pending;
  expect(error).toBeInstanceOf(ConnectionError);
  if (!(error instanceof ConnectionError)) throw error;
  expect(error.code).toBe("timeout");
  expect("operationState" in error).toBe(false);
});


it("keeps only language and appearance preferences in storage", () => {
  saveMobilePreferences({language: "zh-CN", theme: "dark"});
  expect(mobilePreferences()).toEqual({language: "zh-CN", theme: "dark"});
  expect(JSON.stringify(localStorage)).not.toMatch(/token|csrf|amount|account/);
});
