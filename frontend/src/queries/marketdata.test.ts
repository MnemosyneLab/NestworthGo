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
    currentSync: vi.fn(async () => ({ jobId: "" })),
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
});
