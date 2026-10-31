import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { ConnectionSettings } from "./ConnectionSettings";
import {
  backendBase,
  ConnectionError,
  normalizeBackend,
  saveBackend,
  takeConnectionHint,
} from "./connection";
import {
  api,
  MobileError,
  pendingOperation,
  savePending,
  setMobileSession,
} from "./api";

beforeEach(async () => {
  await i18n.changeLanguage("en");
  localStorage.clear();
  sessionStorage.clear();
  saveBackend("");
  setMobileSession(null);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  saveBackend("");
  setMobileSession(null);
});

it.each([
  "example.com",
  "https://user:secret@example.com",
  "https://example.com/api",
  "https://example.com?",
  "https://example.com#secret",
  "https://*.example.com",
  "https://example.com\\path",
  "javascript:alert(1)",
  "http://192.168.1.2:9443",
])("rejects unsafe backend input %s", (value) => {
  expect(() => normalizeBackend(value)).toThrow(ConnectionError);
});
it("normalizes the origin and stores only the nonsecret server preference", () => {
  expect(normalizeBackend(" HTTPS://EXAMPLE.COM:443/ ")).toBe(
    "https://example.com",
  );
  saveBackend("https://server-a.example/");
  expect(backendBase()).toBe("https://server-a.example");
  savePending({ operationId: "operation-a", householdId: "household-a" });
  saveBackend("https://server-b.example");
  expect(pendingOperation()).toBeNull();
  savePending({ operationId: "operation-b", householdId: "household-b" });
  saveBackend("https://server-a.example");
  expect(pendingOperation()?.operationId).toBe("operation-a");
  expect(JSON.stringify(localStorage)).not.toMatch(/operation|household|token/);
});
it("uses a QR only as an editable address hint and removes it before requests", () => {
  window.history.replaceState(
    null,
    "",
    "/mobile.html#backend=https%3A%2F%2Fsynthetic.example",
  );
  expect(takeConnectionHint()).toBe("https://synthetic.example");
  expect(window.location.hash).toBe("");
  expect(backendBase()).toBe("");
});

it("tests without auth, preserves the active endpoint until save, and ignores a cancelled late response", async () => {
  const user = userEvent.setup();
  let resolve!: (value: Response) => void;
  const fetch = vi.fn(
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  vi.stubGlobal("fetch", fetch);
  const changed = vi.fn(() => true);
  render(<ConnectionSettings onChange={changed} />);
  await user.type(
    screen.getByLabelText("Backend HTTPS address"),
    "https://new.example/",
  );
  await user.dblClick(screen.getByRole("button", { name: "Test connection" }));
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]).toEqual([
    "https://new.example/mobile-api/v1/connection",
    expect.objectContaining({
      credentials: "omit",
      redirect: "error",
      referrerPolicy: "no-referrer",
    }),
  ]);
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  await act(async () =>
    resolve(
      new Response(
        JSON.stringify({
          service: "nestworth-mobile",
          version: 1,
          authentication: "bearer",
        }),
      ),
    ),
  );
  expect(
    screen.queryByText(/Nestworth connection is available/),
  ).not.toBeInTheDocument();
  expect(backendBase()).toBe("");
  await user.click(screen.getByRole("button", { name: "Use this server" }));
  expect(changed).toHaveBeenCalledOnce();
  expect(changed).toHaveBeenCalledWith("https://new.example");
  expect(backendBase()).toBe("");
});

it("does not deliver delayed pairing results after a server switch", async () => {
  let resolve!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const pending = api("/session", "POST", { token: "synthetic-one-use" }).catch(
    (e) => e,
  );
  setMobileSession(null);
  saveBackend("https://other.example");
  resolve(
    new Response(JSON.stringify({ accessToken: "synthetic-session-secret" })),
  );
  const error = await pending;
  expect(error).toBeInstanceOf(MobileError);
  if (!(error instanceof MobileError)) throw error;
  expect(error.code).toBe("session_changed");
  expect(JSON.stringify(localStorage)).not.toContain("secret");
});

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

it("reports an unsupported response as a protocol error and never saves it implicitly", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("<html>unrelated host</html>")),
  );
  const user = userEvent.setup();
  render(<ConnectionSettings onChange={vi.fn(() => true)} />);
  await user.type(
    screen.getByLabelText("Backend HTTPS address"),
    "https://unrelated.example",
  );
  await user.click(screen.getByRole("button", { name: "Test connection" }));
  await screen.findByText(i18n.t("mobile.transport.invalid_response"));
  expect(backendBase()).toBe("");
});

it("leaves the active server and in-flight receipt lookup intact when the owner declines a switch", async () => {
  let resolve!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const pending = api<{ operationState: string }>("/operations/original");
  const changed = vi.fn(() => false);
  const user = userEvent.setup();
  render(<ConnectionSettings onChange={changed} />);
  await user.type(
    screen.getByLabelText("Backend HTTPS address"),
    "https://other.synthetic.example",
  );
  await user.click(screen.getByRole("button", { name: "Use this server" }));
  expect(changed).toHaveBeenCalledWith("https://other.synthetic.example");
  expect(backendBase()).toBe("");
  resolve(new Response(JSON.stringify({ operationState: "committed" })));
  await expect(pending).resolves.toEqual({ operationState: "committed" });
});
