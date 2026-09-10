import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { DataHealthPage } from "./DataHealthPage";

const scanHealth = vi.fn();
const previewSync = vi.fn();
const startSync = vi.fn();
const getCurrentSyncJob = vi.fn(async () => ({ jobId: "" }));
const refreshAll = vi.fn();

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: () => () => undefined,
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    ScanMarketDataHealth: () => scanHealth(),
    GetCurrentSyncJob: () => getCurrentSyncJob(),
    PreviewMarketDataSync: (request: unknown) => previewSync(request),
    StartMarketDataSync: (request: unknown) => startSync(request),
    CancelSyncJob: vi.fn(),
    StartRefreshAll: (...args: unknown[]) => refreshAll(...args),
    StartRefreshMissingOrStale: (...args: unknown[]) => refreshAll(...args),
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { Load: () => Promise.resolve({ timezone: "Asia/Singapore" }) },
}));

function renderPage(props: { onOpenSettings?: () => void; onOpenMarketData?: () => void; onOpenInvestments?: () => void } = {}) {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <DataHealthPage {...props} />
    </QueryClientProvider>,
  );
}

describe("DataHealthPage", () => {
  beforeEach(() => {
    scanHealth.mockReset();
    previewSync.mockReset();
    startSync.mockReset();
    getCurrentSyncJob.mockReset();
    refreshAll.mockReset();
    getCurrentSyncJob.mockResolvedValue({ jobId: "" });
    previewSync.mockResolvedValue({
      estimatedRequestCount: 4,
      instrumentTargets: 1,
      fxTargets: 1,
      latestInstrumentCount: 1,
      latestFxCount: 1,
      snapshotWorkEstimate: 2,
      unresolved: [{ targetKey: "provider:tiingo", code: "missing_key", reason: "missing" }],
    });
    startSync.mockResolvedValue({ job: { jobId: "job-1", outcome: "running", phase: "plan", completedTargets: 0, targetCount: 1 }, attached: false, conflict: false });
  });

  it("opens with a local scan and does not start provider work", async () => {
    scanHealth.mockResolvedValue({
      healthy: true,
      coverageThrough: "2026-09-09",
      lastFinalizedMarketDate: "2026-09-09",
      issueCount: 0,
      executableCount: 0,
      prerequisiteCount: 0,
      snapshotDays: 0,
      issues: [],
    });
    renderPage();
    expect(await screen.findByTestId("data-health-page")).toBeInTheDocument();
    expect(await screen.findByText("All data is healthy")).toBeInTheDocument();
    expect(scanHealth).toHaveBeenCalled();
    expect(previewSync).not.toHaveBeenCalled();
    expect(startSync).not.toHaveBeenCalled();
    expect(refreshAll).not.toHaveBeenCalled();
  });

  it("keeps executable repairs and manual prerequisites separate", async () => {
    const onOpenSettings = vi.fn();
    const onOpenInvestments = vi.fn();
    scanHealth.mockResolvedValue({
      healthy: false,
      incompleteSince: "2026-09-05",
      issueCount: 3,
      executableCount: 1,
      prerequisiteCount: 2,
      snapshotDays: 0,
      issues: [
        {
          id: "hist",
          kind: "missing_instrument_history",
          severity: "blocking",
          groupKey: "instrument:i1",
          targetKey: "instrument:i1",
          label: "AAPL",
          provider: "tiingo",
          instrumentId: "i1",
          rangeStart: "2026-09-02",
          rangeEnd: "2026-09-04",
          action: "repair",
          executable: true,
        },
        {
          id: "key",
          kind: "missing_provider_key",
          severity: "blocking",
          groupKey: "provider_key:tiingo",
          targetKey: "provider:tiingo",
          label: "tiingo",
          provider: "tiingo",
          action: "provider_settings",
          executable: false,
        },
        {
          id: "manual",
          kind: "missing_manual_price",
          severity: "blocking",
          groupKey: "instrument:m1",
          targetKey: "instrument:m1",
          label: "ABC Bank Wealth",
          instrumentId: "m1",
          rangeStart: "2026-09-06",
          action: "manual_entry",
          executable: false,
        },
      ],
    });
    renderPage({ onOpenSettings, onOpenInvestments });
    expect(await screen.findByText("Data incomplete since 2026-09-05")).toBeInTheDocument();
    expect(screen.getByText("Executable repairs")).toBeInTheDocument();
    expect(screen.getByText("Prerequisites")).toBeInTheDocument();
    expect(screen.getByText("AAPL")).toBeInTheDocument();
    expect(screen.getByText("ABC Bank Wealth")).toBeInTheDocument();
    expect(previewSync).not.toHaveBeenCalled();
    expect(startSync).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "Open provider settings" }));
    expect(onOpenSettings).toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Add manual data" }));
    expect(onOpenInvestments).toHaveBeenCalled();
  });

  it("shows a repair preview before starting Repair All", async () => {
    scanHealth.mockResolvedValue({
      healthy: false,
      incompleteSince: "2026-09-05",
      issueCount: 1,
      executableCount: 1,
      prerequisiteCount: 0,
      snapshotDays: 2,
      issues: [
        {
          id: "hist",
          kind: "missing_instrument_history",
          severity: "blocking",
          groupKey: "instrument:i1",
          targetKey: "instrument:i1",
          label: "AAPL",
          action: "repair",
          executable: true,
        },
      ],
    });
    renderPage();
    await screen.findByTestId("repair-all");
    await userEvent.click(screen.getByTestId("repair-all"));
    expect(await screen.findByTestId("repair-preview")).toBeInTheDocument();
    expect(startSync).not.toHaveBeenCalled();
    await userEvent.click(screen.getByTestId("confirm-repair"));
    await waitFor(() => expect(startSync).toHaveBeenCalledWith({ scope: "repair_all" }));
  });
});
