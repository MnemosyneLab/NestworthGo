import { expect, it, vi } from "vitest";
import { applyModelNavigation, type ModelPageState } from "./modelNavigation";
function state() {
  return {
    stamp: "current",
    surface: "desktop",
    blocked: (): boolean => false,
    dispatch: vi.fn(),
  } satisfies ModelPageState;
}
it("rechecks page generation after consuming backend intent", async () => {
  const current = state();
  const resolve = vi.fn(async () => {
    current.stamp = "new-page";
    return { kind: "overview", accountId: "" };
  });
  expect(await applyModelNavigation("current", () => current, resolve)).toBe(
    "stale_context",
  );
  expect(current.dispatch).not.toHaveBeenCalled();
  expect(resolve).toHaveBeenCalledTimes(1);
});
it("blocks unresolved financial review before and after IPC", async () => {
  const current = state();
  current.blocked = () => true;
  const resolve = vi.fn(async () => ({ kind: "accounts", accountId: "" }));
  expect(await applyModelNavigation("current", () => current, resolve)).toBe(
    "blocked",
  );
  expect(resolve).not.toHaveBeenCalled();
  current.blocked = () => false;
  resolve.mockImplementation(async () => {
    current.blocked = () => true;
    return { kind: "accounts", accountId: "" };
  });
  expect(await applyModelNavigation("current", () => current, resolve)).toBe(
    "blocked",
  );
  expect(current.dispatch).not.toHaveBeenCalled();
});
it("allows only controlled read-only pages, never arbitrary routes or mobile", async () => {
  const current = state();
  expect(
    await applyModelNavigation(
      "current",
      () => current,
      async () => ({ kind: "overview", accountId: "" }),
    ),
  ).toBe("opened");
  expect(current.dispatch).toHaveBeenCalledWith({ page: "overview" });
  current.dispatch.mockClear();
  expect(
    await applyModelNavigation(
      "current",
      () => current,
      async () => ({ kind: "arbitrary-route", accountId: "" }),
    ),
  ).toBe("invalid_target");
  expect(current.dispatch).not.toHaveBeenCalled();
  const resolve = vi.fn(async () => ({ kind: "overview", accountId: "" }));
  expect(
    await applyModelNavigation(
      "current",
      () => ({ ...current, surface: "mobile" }),
      resolve,
    ),
  ).toBe("stale_context");
  expect(resolve).not.toHaveBeenCalled();
});

it("does not report opened when the application navigation guard rejects dispatch", async () => {
  const current = state();
  current.dispatch.mockReturnValue(false);
  expect(
    await applyModelNavigation(
      "current",
      () => current,
      async () => ({ kind: "overview", accountId: "" }),
    ),
  ).toBe("blocked");
});

it("maps only authorized fixed analysis views to semantic navigation", async () => {
  const current = state();
  expect(await applyModelNavigation("current", () => current, async () => ({kind: "return-analysis", accountId: "", tab: "trend"}))).toBe("opened");
  expect(current.dispatch).toHaveBeenCalledWith({page: "return-analysis", tab: "trend"});
  current.dispatch.mockClear();
  expect(await applyModelNavigation("current", () => current, async () => ({kind: "return-analysis", accountId: "", tab: "write"}))).toBe("invalid_target");
  expect(current.dispatch).not.toHaveBeenCalled();
});

it.each(["asset-changes", "historical-overview", "settings"])("opens the fixed %s root without a financial permission claim",async(kind)=>{const current=state();expect(await applyModelNavigation("current",()=>current,async()=>({kind,accountId:""}))).toBe("opened");expect(current.dispatch).toHaveBeenCalledWith({page:kind});});
