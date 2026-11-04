import { afterEach, beforeEach, expect, it, vi } from "vitest";

const oldCredential = {
  accessToken: "a".repeat(43),
  expiresAt: new Date(Date.now() + 600000).toISOString(),
};
const newCredential = {
  accessToken: "b".repeat(43),
  expiresAt: oldCredential.expiresAt,
};
const session = (credential: typeof oldCredential) => ({
  accessToken: credential.accessToken,
  csrfToken: "synthetic-csrf",
  instance: "synthetic",
  device: {
    id: credential === oldCredential ? "synthetic-old" : "synthetic-new",
    name: "Synthetic",
    permissions: {
      read: true,
      record: true,
      market_refresh: true,
      manual_quote: true,
    },
    expiresAt: credential.expiresAt,
  },
});
async function tab() {
  vi.resetModules();
  return { api: await import("./api"), storage: await import("./connection") };
}
beforeEach(() => {
  localStorage.clear();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
});
it.each(["401", "logout"])(
  "an old tab's %s cannot delete a newer tab's remembered login",
  async (mode) => {
    const a = await tab();
    a.storage.rememberMobileCredential(oldCredential);
    a.api.setMobileSession(session(oldCredential));
    let respond!: (value: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            respond = resolve;
          }),
      ),
    );
    const request = a.api
      .api(
        mode === "logout" ? "/session" : "/overview",
        mode === "logout" ? "DELETE" : "GET",
      )
      .catch(() => undefined);
    const b = await tab();
    b.storage.rememberMobileCredential(newCredential);
    b.api.setMobileSession(session(newCredential));
    respond(
      new Response(
        JSON.stringify(
          mode === "logout"
            ? { ok: true }
            : { error: { code: "session_expired" } },
        ),
        { status: mode === "logout" ? 200 : 401 },
      ),
    );
    await request;
    if (mode === "logout") a.api.setMobileSession(null);
    expect(b.storage.rememberedMobileCredential()).toEqual(newCredential);
  },
);

it("a late same-tab 401 cannot clear a restored replacement session", async () => {
  const a = await tab();
  a.storage.rememberMobileCredential(oldCredential);
  a.api.setMobileSession(session(oldCredential));
  let respond!: (r: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          respond = done;
        }),
    ),
  );
  const oldRequest = a.api.api("/overview").catch(() => undefined);
  const b = await tab();
  b.storage.rememberMobileCredential(newCredential);
  a.api.restoreMobileCredential();
  respond(
    new Response('{"error":{"code":"session_expired"}}', { status: 401 }),
  );
  await oldRequest;
  expect(b.storage.rememberedMobileCredential()).toEqual(newCredential);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, init: RequestInit) => {
      expect(
        (init.headers as Record<string, string>).Authorization ===
          `Bearer ${newCredential.accessToken}`,
      ).toBe(true);
      return new Response("{}");
    }),
  );
  await a.api.api("/session");
});
it("a memory-only login cannot discard another tab's remembered session", async () => {
  const a = await tab();
  const b = await tab();
  b.storage.rememberMobileCredential(newCredential);
  expect(a.api.persistMobileLogin(session(oldCredential), false)).toBe(true);
  a.api.setMobileSession(session(oldCredential));
  a.api.setMobileSession(null);
  expect(b.storage.rememberedMobileCredential()).toEqual(newCredential);
});
it("a rejected anonymous login never clears another tab's remembered session", async () => {
  const a = await tab();
  const b = await tab();
  b.storage.rememberMobileCredential(newCredential);
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response('{"error":{"code":"session_expired"}}', { status: 401 }),
    ),
  );
  await a.api
    .api("/session", "POST", { token: "SYNTHETIC" }, { anonymous: true })
    .catch(() => undefined);
  expect(b.storage.rememberedMobileCredential()).toEqual(newCredential);
});
it("expired-entry cleanup cannot remove a login published between read and remove", async () => {
  const a = await tab();
  const b = await tab();
  a.storage.rememberMobileCredential({
    ...oldCredential,
    expiresAt: "2000-01-01T00:00:00Z",
  });
  const key =
    "nestworth.mobile.session.v2." +
    localStorage.getItem("nestworth.mobile.session.v2");
  const original = Storage.prototype.getItem;
  let interleaved = false;
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (
    this: Storage,
    k: string,
  ) {
    const raw = original.call(this, k);
    if (k === key && !interleaved) {
      interleaved = true;
      b.storage.rememberMobileCredential(newCredential);
    }
    return raw;
  });
  expect(a.storage.rememberedMobileCredential()).toEqual(newCredential);
  expect(b.storage.rememberedMobileCredential()).toEqual(newCredential);
});
it("a failed concurrent publication cleans only its own entry", async () => {
  const a = await tab();
  const b = await tab();
  const original = Storage.prototype.setItem;
  let interleaved = false;
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(function (
    this: Storage,
    k: string,
    v: string,
  ) {
    if (k === "nestworth.mobile.session.v2" && !interleaved) {
      interleaved = true;
      b.storage.rememberMobileCredential(newCredential);
      throw Error("synthetic failure");
    }
    return original.call(this, k, v);
  });
  expect(a.storage.rememberMobileCredential(oldCredential)).toBe(false);
  expect(b.storage.rememberedMobileCredential()).toEqual(newCredential);
});
it("storage events read current selection without changing a tab's request identity", async () => {
  const a = await tab();
  a.storage.rememberMobileCredential(oldCredential);
  a.api.setMobileSession(session(oldCredential));
  const b = await tab();
  b.storage.rememberMobileCredential(newCredential);
  let observed: unknown;
  const unsubscribe = a.storage.onMobileCredentialStorageChange(() => {
    observed = a.storage.rememberedMobileCredential();
  });
  window.dispatchEvent(
    new StorageEvent("storage", {
      key: "nestworth.mobile.session.v2",
      newValue: "stale-event-value",
    }),
  );
  expect(observed).toEqual(newCredential);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, init: RequestInit) => {
      expect(
        (init.headers as Record<string, string>).Authorization ===
          `Bearer ${oldCredential.accessToken}`,
      ).toBe(true);
      return new Response("{}");
    }),
  );
  await a.api.api("/session");
  unsubscribe();
});
it("a revoked current selection never falls back to a legacy credential", async () => {
  const a = await tab();
  localStorage.setItem(
    "nestworth.mobile.session.v1",
    JSON.stringify(oldCredential),
  );
  expect(a.storage.rememberedMobileCredential()).toEqual(oldCredential);
  a.storage.rememberMobileCredential(newCredential);
  a.storage.forgetMobileCredential(newCredential.accessToken);
  expect(a.storage.rememberedMobileCredential()).toBeNull();
});
