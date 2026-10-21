import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { MarketDataSyncBar } from "./MarketDataSyncBar";

const previewSync = vi.fn();
const startSync = vi.fn();

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { timezone: "UTC" } }) }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    GetCurrentSyncJob: async () => ({ jobId: "" }),
    PreviewMarketDataSync: (request: unknown) => previewSync(request),
    StartMarketDataSync: (request: unknown) => startSync(request),
  },
}));

function plan(estimatedRequestCount = 3, snapshotWorkEstimate = 0) {
  return {
    estimatedRequestCount, snapshotWorkEstimate,
    instrumentTargets: 0, fxTargets: 0, latestInstrumentCount: 0, latestFxCount: 0,
    items: [], unresolved: [],
  };
}

function deferred() {
  let resolve!: (value: ReturnType<typeof plan>) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<ReturnType<typeof plan>>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function renderBar() {
  return render(<QueryClientProvider client={createTestQueryClient()}>
    <MarketDataSyncBar latestRefreshing={false} onRefreshMissing={vi.fn()} onRefreshAll={vi.fn()} onCancelLatest={vi.fn()} />
  </QueryClientProvider>);
}

async function openPreview(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "More actions" }));
  await user.click(await screen.findByRole("button", { name: "Sync and repair" }));
  return screen.findByRole("button", { name: "Start sync" });
}

describe("MarketDataSyncBar preview readiness", () => {
  beforeEach(() => {
    previewSync.mockReset().mockResolvedValue(plan());
    startSync.mockReset().mockResolvedValue({ attached: false, conflict: false });
  });

  it("shows initial pending/error state and only starts after a successful retry", async () => {
    const user = userEvent.setup();
    const initial = deferred();
    const retry = deferred();
    previewSync.mockReturnValueOnce(initial.promise).mockReturnValueOnce(retry.promise);
    renderBar();
    const start = await openPreview(user);
    expect(start).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("Loading");
    await user.click(start);
    expect(startSync).not.toHaveBeenCalled();

    await act(async () => initial.reject(new Error("Preview unavailable")));
    // callService translates unstructured IPC failures to safe localized copy.
    expect(await screen.findByRole("alert")).toHaveTextContent("An unexpected error occurred.");
    expect(start).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(previewSync).toHaveBeenLastCalledWith({ scope: "repair_all", forceRecheck: false });
    expect(screen.getByRole("status")).toHaveTextContent("Loading");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(start).toBeDisabled();

    await act(async () => retry.resolve(plan()));
    await waitFor(() => expect(start).toBeEnabled());
    await user.click(start);
    expect(startSync).toHaveBeenCalledExactlyOnceWith({ scope: "repair_all", forceRecheck: false });
  });

  it("requires a successful current-option preview after force recheck changes or fails", async () => {
    const user = userEvent.setup();
    const changed = deferred();
    const retry = deferred();
    previewSync.mockResolvedValueOnce(plan()).mockReturnValueOnce(changed.promise).mockReturnValueOnce(retry.promise);
    renderBar();
    const start = await openPreview(user);
    await waitFor(() => expect(start).toBeEnabled());
    const forceRecheck = screen.getByRole("checkbox", { name: "Force recheck recent observations" });
    forceRecheck.focus();
    await user.keyboard(" ");
    expect(forceRecheck).toBeChecked();
    expect(previewSync).toHaveBeenLastCalledWith({ scope: "repair_all", forceRecheck: true });
    expect(start).toBeDisabled();
    expect(screen.queryByTestId("sync-preview")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Loading");

    await act(async () => changed.reject(new Error("Recheck preview failed")));
    expect(await screen.findByRole("alert")).toHaveTextContent("An unexpected error occurred.");
    expect(start).toBeDisabled();
    await user.click(start);
    expect(startSync).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(previewSync).toHaveBeenLastCalledWith({ scope: "repair_all", forceRecheck: true });
    expect(start).toBeDisabled();
    await act(async () => retry.resolve(plan(7)));
    await waitFor(() => expect(start).toBeEnabled());
    expect(screen.getByTestId("sync-preview")).toHaveTextContent("7 estimated provider requests");
    await user.click(start);
    expect(startSync).toHaveBeenCalledExactlyOnceWith({ scope: "repair_all", forceRecheck: true });
  });

  it("uses the latest mutation observer when options change again before completion", async () => {
    const user = userEvent.setup();
    const forced = deferred();
    const normal = deferred();
    previewSync.mockResolvedValueOnce(plan()).mockReturnValueOnce(forced.promise).mockReturnValueOnce(normal.promise);
    renderBar();
    const start = await openPreview(user);
    await waitFor(() => expect(start).toBeEnabled());
    const forceRecheck = screen.getByRole("checkbox");
    await user.click(forceRecheck);
    await user.click(forceRecheck);
    expect(forceRecheck).not.toBeChecked();
    expect(start).toBeDisabled();
    await act(async () => forced.resolve(plan(9)));
    expect(start).toBeDisabled();
    expect(screen.queryByTestId("sync-preview")).not.toBeInTheDocument();
    await act(async () => normal.resolve(plan(2)));
    await waitFor(() => expect(start).toBeEnabled());
    expect(screen.getByTestId("sync-preview")).toHaveTextContent("2 estimated provider requests");
    await user.click(start);
    expect(startSync).toHaveBeenCalledExactlyOnceWith({ scope: "repair_all", forceRecheck: false });
  });

  it("requires a fresh preview after closing and reopening a successful review", async () => {
    const user = userEvent.setup();
    const reopened = deferred();
    previewSync.mockResolvedValueOnce(plan()).mockReturnValueOnce(reopened.promise);
    renderBar();
    await openPreview(user);
    await screen.findByTestId("sync-preview");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    const start = await openPreview(user);
    expect(previewSync).toHaveBeenCalledTimes(2);
    expect(start).toBeDisabled();
    expect(screen.queryByTestId("sync-preview")).not.toBeInTheDocument();
    await act(async () => reopened.resolve(plan(4)));
    await waitFor(() => expect(start).toBeEnabled());
    expect(screen.getByTestId("sync-preview")).toHaveTextContent("4 estimated provider requests");
  });

  it("keeps a dismissed pending preview closed when it completes", async () => {
    const user = userEvent.setup();
    const pending = deferred();
    previewSync.mockReturnValueOnce(pending.promise);
    renderBar();
    await openPreview(user);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await act(async () => pending.resolve(plan()));
    await waitFor(() => expect(screen.getByRole("button", { name: "More actions" })).toBeEnabled());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(startSync).not.toHaveBeenCalled();
  });

  it.each([0, 2])("preserves supported zero-provider-request plans with %i snapshot days", async (snapshotDays) => {
    const user = userEvent.setup();
    previewSync.mockResolvedValue(plan(0, snapshotDays));
    renderBar();
    const start = await openPreview(user);
    await waitFor(() => expect(start).toBeEnabled());
    expect(screen.getByTestId("sync-preview")).toHaveTextContent("0 estimated provider requests");
    await user.click(start);
    expect(startSync).toHaveBeenCalledExactlyOnceWith({ scope: "repair_all", forceRecheck: false });
  });
});
