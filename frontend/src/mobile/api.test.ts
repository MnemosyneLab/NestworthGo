import { afterEach, expect, it, vi } from "vitest";
import {
  api,
  MobileError,
  onSessionExpired,
  setMobileSession,
  type Session,
} from "./api";

afterEach(() => {
  vi.unstubAllGlobals();
  setMobileSession(null);
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
