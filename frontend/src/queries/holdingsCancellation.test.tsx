import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { StrictMode, type ReactNode } from "react";
import { beforeAll, expect, it, vi } from "vitest";
import { Call, objectNames, setTransport } from "@wailsio/runtime";
import { createTestQueryClient } from "@/test/queryClient";
import { useInstrumentHoldingGroups } from "./analytics";
import { usePerformanceSeries, useQuoteReturns } from "./investments";

// Own beta.28's DOM startup timer, as in the installed-runtime regression suite.
vi.hoisted(() => { vi.useFakeTimers(); });
beforeAll(async () => {
  try {
    await vi.advanceTimersByTimeAsync(50);
    expect(vi.getTimerCount()).toBe(0);
  } finally { vi.useRealTimers(); }
});

it("cancels StrictMode reads and rapid period/account changes through real generated bindings", async () => {
  const client = createTestQueryClient({ retry: false });
  const reads: Array<{ id: string; args: unknown[]; resolve: (value: unknown) => void; reject: (reason: unknown) => void }> = [];
  const cancelled = new Set<string>();
  setTransport({ call: (object, _method, _window, request: unknown) => {
    const args = request as { "call-id": string; args: unknown[] };
    if (object === objectNames.CancelCall) {
      cancelled.add(args["call-id"]);
      return Promise.resolve(undefined);
    }
    expect(object).toBe(objectNames.Call);
    return new Promise((resolve, reject) => { reads.push({ id: args["call-id"], args: args.args, resolve, reject }); });
  } });
  const wrapper = ({ children }: { children: ReactNode }) => <StrictMode><QueryClientProvider client={client}>{children}</QueryClientProvider></StrictMode>;
  const view = renderHook(({ days, account }) => ({
    series: usePerformanceSeries("synthetic-instrument", days),
    returns: useQuoteReturns("synthetic-instrument"),
    groups: useInstrumentHoldingGroups(true, account),
  }), { wrapper, initialProps: { days: 30, account: "synthetic-account-a" } });
  try {
    // StrictMode unmounts the initial observers and starts fresh reads.
    await waitFor(() => expect(reads).toHaveLength(6));
    expect(reads.slice(0, 3).every(read => cancelled.has(read.id))).toBe(true);
    expect(cancelled.size).toBe(3);
    view.rerender({ days: 7, account: "synthetic-account-b" });
    await waitFor(() => expect(reads).toHaveLength(8));
    expect(cancelled.size).toBe(5); // returns keeps the same instrument key
    const finalSeries = [...reads].reverse().find(read => read.args[1] === 7)!;
    const finalGroups = [...reads].reverse().find(read => read.args[0] === "synthetic-account-b")!;
    const liveReturns = [...reads].reverse().find(read => read.args.length === 1 && read.args[0] === "synthetic-instrument")!;
    await act(async () => {
      // The corrected Go services complete abandoned reads with zero DTOs.
      for (const read of reads) if (cancelled.has(read.id)) read.resolve(null);
      finalSeries.resolve({ windowDays: 7, points: [], unavailableReason: "insufficient_daily_history" });
      finalGroups.resolve([]);
      liveReturns.resolve({ items: [] });
    });
    await waitFor(() => expect(view.result.current.series.data?.windowDays).toBe(7));
    await waitFor(() => expect(view.result.current.groups.data).toEqual([]));
    await waitFor(() => expect(view.result.current.returns.data?.items).toEqual([]));
    expect(view.result.current.series.isError || view.result.current.groups.isError || view.result.current.returns.isError).toBe(false);

    // A real live backend error must remain observable and retryable.
    view.rerender({ days: 90, account: "synthetic-account-b" });
    await waitFor(() => expect(reads).toHaveLength(9));
    await act(async () => reads[8].reject(new Call.RuntimeError('{"code":"internal","message":"an unexpected error occurred"}')));
    await waitFor(() => expect(view.result.current.series.isError).toBe(true));
    expect(view.result.current.series.error).toHaveProperty("wireError.code", "internal");
    await act(async () => {
      const retry = view.result.current.series.refetch();
      await waitFor(() => expect(reads).toHaveLength(10));
      reads[9].resolve({ windowDays: 90, points: [], unavailableReason: "insufficient_daily_history" });
      await retry;
    });
    expect(view.result.current.series.data?.windowDays).toBe(90);
  } finally {
    view.unmount();
    client.clear();
    setTransport(null);
  }
});
