import { afterEach, expect, it, vi } from "vitest";
import {
  api,
  MobileError,
  onSessionExpired,
  setMobileSession,
  type Session,
} from "./api";
import { saveBackend } from "./connection";

afterEach(() => {
  vi.unstubAllGlobals();
  setMobileSession(null);
  saveBackend("");
});
it.each([200, 401])(
  "does not deliver an old %s response into a newly paired session",
  async (status) => {
    const old: Session = {
      device: {
        id: "old-phone",
        name: "Synthetic old",
        permissions: {
          read: true,
          record: true,
          market_refresh: false,
          manual_quote: false,
        },
      },
      csrfToken: "old-csrf",
      instance: "synthetic",
    };
    setMobileSession(old);
    const expired = vi.fn();
    const unsubscribe = onSessionExpired(expired);
    let resolve!: (r: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((done) => {
            resolve = done;
          }),
      ),
    );
    const pending = api("/operations/synthetic-id").catch((e) => e);
    setMobileSession({
      ...old,
      device: { ...old.device, id: "new-phone" },
      csrfToken: "new-csrf",
    });
    resolve(
      new Response(
        JSON.stringify(
          status === 200
            ? { operationState: "committed" }
            : { error: { code: "session_expired" } },
        ),
        { status },
      ),
    );
    const result = await pending;
    expect(result).toBeInstanceOf(MobileError);
    if (!(result instanceof MobileError))
      throw new Error("expected stale-session error");
    expect(result.code).toBe("session_changed");
    expect(expired).not.toHaveBeenCalled();
    unsubscribe();
  },
);

it("keeps separate-frontend bearer credentials in memory and omits ambient cookies", async () => {
  saveBackend("https://synthetic.private.example");
  const session: Session = {
    device: {
      id: "synthetic",
      name: "Synthetic",
      permissions: {
        read: true,
        record: true,
        market_refresh: false,
        manual_quote: false,
      },
    },
    csrfToken: "synthetic-csrf",
    accessToken: "synthetic-bearer",
    instance: "synthetic-instance",
  };
  setMobileSession(session);
  const fetch = vi.fn(async () => new Response(JSON.stringify({ ok: true })));
  vi.stubGlobal("fetch", fetch);
  await api("/record-previews", "POST", { amount: "1.0009" });
  expect(fetch).toHaveBeenCalledWith(
    "https://synthetic.private.example/mobile-api/v1/record-previews",
    expect.objectContaining({
      credentials: "omit",
      redirect: "error",
      referrerPolicy: "no-referrer",
      headers: expect.objectContaining({
        Authorization: "Bearer synthetic-bearer",
        "X-CSRF-Token": "synthetic-csrf",
      }),
      body: '{"amount":"1.0009"}',
    }),
  );
  expect(JSON.stringify(localStorage)).not.toMatch(
    /synthetic-bearer|synthetic-csrf|1\.0009/,
  );
});
