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
  saveMobilePreferences({language: "ja", theme: "dark"});
  expect(mobilePreferences()).toEqual({language: "ja", theme: "dark"});
  expect(JSON.stringify(localStorage)).not.toMatch(/token|csrf|amount|account/);
});

it("validates and expires only the random remembered credential", async () => {
  const { rememberMobileCredential, rememberedMobileCredential } =
    await import("./connection");
  const value = {
    accessToken: "s".repeat(43),
    expiresAt: new Date(Date.now() + 60000).toISOString(),
  };
  expect(rememberMobileCredential(value)).toBe(true);
  expect(rememberedMobileCredential()).toEqual(value);
  const entry = `nestworth.mobile.session.v2.${localStorage.getItem("nestworth.mobile.session.v2")}`;
  localStorage.setItem(entry, "broken");
  expect(rememberedMobileCredential()).toBeNull();
  expect(localStorage.getItem(entry)).toBeNull();
  rememberMobileCredential({
    ...value,
    expiresAt: new Date(Date.now() - 1).toISOString(),
  });
  expect(rememberedMobileCredential()).toBeNull();
});
it("degrades safely when storage is disabled", async () => {
  const { rememberMobileCredential, rememberedMobileCredential } =
    await import("./connection");
  vi.stubGlobal("localStorage", {
    getItem() {
      throw Error("blocked");
    },
    setItem() {
      throw Error("blocked");
    },
    removeItem() {
      throw Error("blocked");
    },
  });
  expect(rememberedMobileCredential()).toBeNull();
  expect(
    rememberMobileCredential({
      accessToken: "s".repeat(43),
      expiresAt: "2099-01-01",
    }),
  ).toBe(false);
});
