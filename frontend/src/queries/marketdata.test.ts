import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, render, waitFor } from "@testing-library/react";
import { createElement, Fragment, type ReactNode } from "react";
import { createTestQueryClient } from "@/test/queryClient";
import { queryKeys } from "@/queries/keys";

const syncHarness = vi.hoisted(() => {
  const listeners = new Map<string, Set<(event: { data: unknown }) => void>>();
  return {
    listeners,
    currentSync: vi.fn(async (): Promise<{ jobId: string; outcome?: string; sequence?: number }> => ({ jobId: "" })),
    emit(name: string) {
      for (const listener of listeners.get(name) ?? []) listener({ data: undefined });
    },
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      const current = syncHarness.listeners.get(name) ?? new Set();
      current.add(listener);
      syncHarness.listeners.set(name, current);
      return () => {
        current.delete(listener);
        if (current.size === 0) syncHarness.listeners.delete(name);
      };
    },
  },
}));

vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    GetCurrentSyncJob: syncHarness.currentSync,
  },
}));

import { MarketDataSyncWorkspaceObserver, useCurrentSyncJob } from "@/queries/marketdata";

function PageSyncObserver() {
  useCurrentSyncJob();
  return null;
}

function Provider({ client, children }: { client: ReturnType<typeof createTestQueryClient>; children?: ReactNode }) {
  return createElement(QueryClientProvider, { client }, children);
}

describe("workspace market-data sync observation", () => {
  afterEach(() => {
    syncHarness.listeners.clear();
    syncHarness.currentSync.mockReset();
    syncHarness.currentSync.mockResolvedValue({ jobId: "" });
    vi.restoreAllMocks();
  });

  it("keeps the workspace listener after the page observer unmounts", async () => {
    const client = createTestQueryClient({ retry: false });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const page = createElement(PageSyncObserver);
    const workspace = createElement(MarketDataSyncWorkspaceObserver);
    const view = render(createElement(Provider, { client }, createElement(Fragment, null, workspace, page)));

    await waitFor(() => expect(syncHarness.listeners.get("marketdata.sync.completed")?.size).toBe(1));
    view.rerender(createElement(Provider, { client }, createElement(Fragment, null, workspace, null)));

    act(() => syncHarness.emit("marketdata.sync.completed"));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.marketdata.currentSync });
    expect(syncHarness.listeners.get("marketdata.sync.completed")?.size).toBe(1);

    view.unmount();
    expect(syncHarness.listeners.get("marketdata.sync.completed")).toBeUndefined();
  });

  it("invalidates completed data once with both observers", async () => {
    const client = createTestQueryClient({ retry: false });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const view = render(createElement(Provider, { client }, createElement(Fragment, null,
      createElement(MarketDataSyncWorkspaceObserver), createElement(PageSyncObserver))));
    await waitFor(() => expect(syncHarness.currentSync).toHaveBeenCalled());
    act(() => client.setQueryData(queryKeys.marketdata.currentSync, { jobId: "job-1", outcome: "running", sequence: 1 }));
    act(() => client.setQueryData(queryKeys.marketdata.currentSync, { jobId: "job-1", outcome: "completed", sequence: 2 }));
    const completionCalls = () => invalidate.mock.calls.filter(([arg]) =>
      JSON.stringify(arg) === JSON.stringify({ queryKey: queryKeys.history.snapshotState })).length;
    await waitFor(() => expect(completionCalls()).toBe(1));
    act(() => client.setQueryData(queryKeys.marketdata.currentSync, null));
    expect(completionCalls()).toBe(1);
    view.unmount();
  });

  it("compensates when a running job disappears", async () => {
    syncHarness.currentSync.mockResolvedValueOnce({ jobId: "job-2", outcome: "running", sequence: 3 });
    const client = createTestQueryClient({ retry: false });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const view = render(createElement(Provider, { client }, createElement(MarketDataSyncWorkspaceObserver)));
    await waitFor(() => expect(client.getQueryData(queryKeys.marketdata.currentSync)).toMatchObject({ jobId: "job-2" }));
    await waitFor(() => expect(syncHarness.listeners.get("marketdata.sync.completed")?.size).toBe(1));
    act(() => client.setQueryData(queryKeys.marketdata.currentSync, null));
    await waitFor(() => expect(invalidate.mock.calls.filter(([arg]) =>
      JSON.stringify(arg) === JSON.stringify({ queryKey: queryKeys.history.snapshotState })).length).toBe(1));
    view.unmount();
  });

  it("observes an initial terminal job and each changed terminal signature once", async () => {
    syncHarness.currentSync.mockResolvedValueOnce({ jobId: "job-3", outcome: "cancelled", sequence: 4 });
    const client = createTestQueryClient({ retry: false });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const view = render(createElement(Provider, { client }, createElement(Fragment, null,
      createElement(MarketDataSyncWorkspaceObserver), createElement(PageSyncObserver))));
    const count = () => invalidate.mock.calls.filter(([arg]) =>
      JSON.stringify(arg) === JSON.stringify({ queryKey: queryKeys.history.snapshotState })).length;
    await waitFor(() => expect(count()).toBe(1));
    act(() => client.setQueryData(queryKeys.marketdata.currentSync, { jobId: "job-3", outcome: "cancelled", sequence: 5 }));
    await waitFor(() => expect(count()).toBe(2));
    view.unmount();
  });

  it("observes the same terminal signature once for a replacement QueryClient", async () => {
    const terminal = { jobId: "job-4", outcome: "completed", sequence: 6 };
    syncHarness.currentSync.mockResolvedValue(terminal);
    const first = createTestQueryClient({ retry: false });
    const second = createTestQueryClient({ retry: false });
    first.setQueryData(queryKeys.marketdata.currentSync, terminal);
    second.setQueryData(queryKeys.marketdata.currentSync, terminal);
    const firstInvalidation = vi.spyOn(first, "invalidateQueries");
    const secondInvalidation = vi.spyOn(second, "invalidateQueries");
    const workspace = createElement(MarketDataSyncWorkspaceObserver);
    const view = render(createElement(Provider, { client: first }, workspace));
    const count = (spy: typeof firstInvalidation) => spy.mock.calls.filter(([arg]) =>
      JSON.stringify(arg) === JSON.stringify({ queryKey: queryKeys.history.snapshotState })).length;
    await waitFor(() => expect(count(firstInvalidation)).toBe(1));
    view.rerender(createElement(Provider, { client: second }, workspace));
    await waitFor(() => expect(count(secondInvalidation)).toBe(1));
    view.unmount();
  });
});
