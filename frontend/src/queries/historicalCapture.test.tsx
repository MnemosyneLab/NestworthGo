import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { beforeEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { useHistoricalOverview } from "./historicalOverview";
import { useHistoricalSeries } from "./historicalSeries";

const { read, series } = vi.hoisted(() => ({ read: vi.fn(), series: vi.fn() }));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({
  Service: { HistoricalOverview: read, HistoricalOverviewSeries: series },
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const cancel = vi.fn();
  const promise = Object.assign(new Promise<T>((yes, no) => { resolve = yes; reject = no; }), { cancel });
  return { promise, resolve, reject, cancel };
}

function setup() {
  const client = createTestQueryClient();
  return { client, wrapper: ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider> };
}

beforeEach(() => { read.mockReset(); series.mockReset(); });

it.each(["", "2026-08-31", "current"])("cancels ten rapid repeated selections and accepts only the final capture (%s)", async compare => {
  const calls = Array.from({ length: 11 }, () => deferred<{ left: { date: string }; amount: string }>());
  calls.forEach(call => read.mockReturnValueOnce(call.promise));
  const { wrapper, client } = setup();
  const view = renderHook(({ date }) => useHistoricalOverview(date, compare, true), { wrapper, initialProps: { date: "2026-08-01" } });
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  for (let i = 1; i <= 10; i++) {
    view.rerender({ date: i % 2 ? "2026-08-02" : "2026-08-01" });
    await waitFor(() => expect(read).toHaveBeenCalledTimes(i + 1));
    expect(calls[i - 1].cancel).toHaveBeenCalledTimes(1);
    expect(calls[i].cancel).not.toHaveBeenCalled();
  }
  const final = { left: { date: "2026-08-01" }, amount: "0.00000012" };
  await act(async () => calls[10].resolve(final));
  await waitFor(() => expect(view.result.current.data).toEqual(final));
  await act(async () => {
    // Replies can race cancellation in either direction, including rejection.
    calls.slice(0, 10).forEach((call, i) => i % 2 ? call.reject(new Error("abandoned read")) : call.resolve({ left: { date: "old" }, amount: "999" }));
  });
  expect(view.result.current.data).toEqual(final);
  expect(view.result.current.isError).toBe(false);
  await act(async () => client.invalidateQueries());
  expect(read).toHaveBeenCalledTimes(12);
  view.unmount();
  expect(calls[10].cancel).not.toHaveBeenCalled();
});

it("cancels same-key disabled observers on mode changes and resumes with a fresh capture", async () => {
  const first = deferred(); const range = deferred(); const final = deferred();
  read.mockReturnValueOnce(first.promise).mockReturnValueOnce(final.promise);
  series.mockReturnValueOnce(range.promise);
  const { wrapper } = setup();
  const view = renderHook(({ mode }) => ({
    overview: useHistoricalOverview("2026-08-01", "2026-08-03", mode === "overview"),
    trend: useHistoricalSeries("2026-08-01", "2026-08-03", mode === "trend", 0),
  }), { wrapper, initialProps: { mode: "overview" } });
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  view.rerender({ mode: "trend" });
  await waitFor(() => expect(first.cancel).toHaveBeenCalledTimes(1));
  expect(series).toHaveBeenCalledTimes(1);
  view.rerender({ mode: "overview" });
  await waitFor(() => expect(range.cancel).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  await act(async () => { first.resolve({ old: true }); range.reject(new Error("cancelled range")); final.resolve({ final: true }); });
  await waitFor(() => expect(view.result.current.overview.data).toEqual({ final: true }));
  expect(view.result.current.overview.isError).toBe(false);
});

it("cancels an invalid or inactive selection without clearing a settled retained capture", async () => {
  const pending = deferred(); read.mockReturnValueOnce(pending.promise).mockResolvedValue({ final: true });
  const { wrapper } = setup();
  const view = renderHook(({ enabled }) => useHistoricalOverview("2026-08-01", "", enabled), { wrapper, initialProps: { enabled: true } });
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  view.rerender({ enabled: false });
  await waitFor(() => expect(pending.cancel).toHaveBeenCalledOnce());
  await act(async () => pending.resolve({ old: true }));
  expect(view.result.current.data).toBeUndefined();
  view.rerender({ enabled: true });
  await waitFor(() => expect(view.result.current.data).toEqual({ final: true }));
  view.rerender({ enabled: false }); view.rerender({ enabled: true });
  expect(read).toHaveBeenCalledTimes(2);
});

it("cancels on navigation/unmount and isolates another account's new page session", async () => {
  const old = deferred(); const fresh = deferred();
  read.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
  const { wrapper } = setup();
  const first = renderHook(() => useHistoricalOverview("2026-08-01", "current", true), { wrapper });
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  first.unmount(); expect(old.cancel).toHaveBeenCalledOnce();
  const second = renderHook(() => useHistoricalOverview("2026-08-01", "current", true), { wrapper });
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  await act(async () => { fresh.resolve({ accountId: "new-account", amount: "1.00000001" }); old.resolve({ accountId: "old-account", amount: "99" }); });
  await waitFor(() => expect(second.result.current.data).toEqual({ accountId: "new-account", amount: "1.00000001" }));
  expect(fresh.cancel).not.toHaveBeenCalled();
});

it("keeps live failures as translated errors and allows an explicit successful retry", async () => {
  read.mockRejectedValueOnce(new Error(JSON.stringify({ code: "invalid_change_time", field: "localDate", message: "invalid date" }))).mockResolvedValue({ final: true });
  const { wrapper } = setup();
  const view = renderHook(() => useHistoricalOverview("2026-08-01", "", true), { wrapper });
  await waitFor(() => expect(view.result.current.isError).toBe(true));
  expect(view.result.current.error).toHaveProperty("wireError.code", "invalid_change_time");
  expect(read).toHaveBeenCalledOnce();
  await act(async () => { await view.result.current.refetch(); });
  await waitFor(() => expect(view.result.current.data).toEqual({ final: true }));
});
