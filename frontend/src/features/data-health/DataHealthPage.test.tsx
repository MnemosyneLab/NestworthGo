import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import type { HealthFocus } from "@/app/navigation";
import { NavigationContext, type ObjectNavigation } from "@/app/NavigationContext";
import { DataHealthPage, focusedHealthIssues } from "./DataHealthPage";
import type { HealthIssueDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

const scanHealth = vi.fn();
const previewSync = vi.fn();
const startSync = vi.fn();
const getCurrentSyncJob = vi.fn(async () => ({ jobId: "" }));
const refreshAll = vi.fn();
const listAccounts = vi.fn();

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

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: (filter: unknown) => listAccounts(filter) },
}));

function renderPage(props: { focus?: HealthFocus; onOpenSettings?: () => void; onOpenMarketData?: () => void } = {}, navigation: ObjectNavigation | null = null) {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <NavigationContext.Provider value={navigation}><DataHealthPage {...props} /></NavigationContext.Provider>
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
    listAccounts.mockReset();
    listAccounts.mockResolvedValue([{ account: { id: "broker", name: "Family brokerage" } }, { account: { id: "archived", name: "Previous bank", archivedAt: "2026-09-21T00:00:00Z" } }]);
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

  it("shows pending daily references for the focused date without offering premature repair", async () => {
    scanHealth.mockResolvedValue({ healthy:false, incompleteSince:"2026-09-21", coverageThrough:"2026-09-21", lastFinalizedMarketDate:"2026-09-21", issueCount:1, executableCount:0, prerequisiteCount:0, snapshotDays:0,
      issues:[{id:"pending-gold",kind:"history_pending",severity:"warning",targetKey:"instrument:gold",instrumentId:"gold",label:"Gold",provider:"yahoo_finance",rangeStart:"2026-09-21",rangeEnd:"2026-09-21",action:"none",executable:false,reason:"daily_reference_pending",nextCheckAt:"2026-09-22T12:00:00+08:00"}] });
    renderPage({focus:{rangeStart:"2026-09-21",rangeEnd:"2026-09-21"}});
    expect(await screen.findByText("Waiting for daily reference")).toBeInTheDocument();
    expect(screen.getByText(/Daily reference is not yet finalized/)).toBeInTheDocument();
    expect(screen.getByText(/2026-09-22 12:00:00\+08:00/)).toBeInTheDocument();
    expect(screen.queryByText("All data is healthy")).not.toBeInTheDocument();
    expect(screen.getByTestId("repair-all")).toBeDisabled();
    expect(previewSync).not.toHaveBeenCalled();
    expect(startSync).not.toHaveBeenCalled();
  });

  it("explains Agent quote gaps without offering manual entry or provider repair", async () => {
    scanHealth.mockResolvedValue({
      healthy: false, issueCount: 1, executableCount: 0, prerequisiteCount: 1, snapshotDays: 0,
      issues: [{ id: "agent-1", kind: "missing_agent_price", severity: "blocking", targetKey: "instrument:i1", instrumentId: "i1", label: "Private Fund", action: "none", executable: false, reason: "agent_data_required" }],
    });
    renderPage();
    const issue = await screen.findByTestId("health-issue-missing_agent_price");
    expect(issue).toHaveTextContent("Ask your Agent to supply this quote through MCP.");
    expect(within(issue).queryByRole("button", { name: "Add manual data" })).not.toBeInTheDocument();
    expect(screen.getByTestId("repair-all")).toBeDisabled();
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
    const onOpenMarketData = vi.fn();
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
    renderPage({ onOpenSettings, onOpenMarketData });
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
    expect(onOpenMarketData).toHaveBeenCalled();
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

  it("renders repairable Yahoo gaps without Cannot automatically repair", async () => {
    scanHealth.mockResolvedValue({
      healthy: false,
      incompleteSince: "2026-09-02",
      issueCount: 2,
      executableCount: 1,
      prerequisiteCount: 0,
      snapshotDays: 0,
      issues: [
        {
          id: "yahoo-msft",
          kind: "missing_instrument_history",
          severity: "blocking",
          groupKey: "instrument:i2",
          targetKey: "instrument:i2",
          label: "Microsoft",
          provider: "yahoo_finance",
          instrumentId: "i2",
          rangeStart: "2026-09-02",
          rangeEnd: "2026-09-04",
          action: "repair",
          executable: true,
        },
        {
          id: "gold",
          kind: "unsupported_coverage",
          severity: "warning",
          groupKey: "instrument:g1",
          targetKey: "instrument:g1",
          label: "Gold",
          provider: "yahoo_finance",
          instrumentId: "g1",
          action: "none",
          reason: "instrument_type_unsupported",
          executable: false,
        },
      ],
    });
    renderPage();
    expect(await screen.findByText("Microsoft")).toBeInTheDocument();
    expect(screen.getByText("Executable repairs")).toBeInTheDocument();
    expect(screen.getAllByText(/yahoo_finance/).length).toBeGreaterThan(0);
    const microsoftRow = screen.getByText("Microsoft").closest("li");
    expect(microsoftRow).not.toHaveTextContent("Not auto-repairable");
    expect(microsoftRow).toHaveTextContent("Included in Repair All");
    const goldRow = screen.getByText("Gold").closest("li");
    expect(goldRow).toHaveTextContent("Not auto-repairable");
    expect(goldRow).toHaveTextContent("This instrument type is not supported for automatic history");
  });

  it("disables Repair All when only prerequisites remain", async () => {
    scanHealth.mockResolvedValue({
      healthy: false,
      incompleteSince: "2026-09-05",
      issueCount: 2,
      executableCount: 0,
      prerequisiteCount: 2,
      snapshotDays: 0,
      issues: [
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
          action: "manual_entry",
          executable: false,
        },
      ],
    });
    renderPage();
    const repair = await screen.findByTestId("repair-all");
    expect(repair).toBeDisabled();
    expect(previewSync).not.toHaveBeenCalled();
    expect(startSync).not.toHaveBeenCalled();
  });
});

 it("keeps a focused blocked snapshot visible with prerequisites outside its dates", async () => {
   scanHealth.mockResolvedValue({ healthy: false, issueCount: 1, issues: [
     { id: "snap", kind: "snapshot_outdated", severity: "blocking", targetKey: "snapshot", collapsed: true, executable: false, action: "repair", rangeStart: "2026-08-01", rangeEnd: "2026-09-01" },
     { id: "price", kind: "missing_manual_price", severity: "blocking", targetKey: "instrument:A", label: "Manual A", instrumentId: "A", collapsed: false, executable: false, action: "manual_entry", rangeStart: "2026-08-01", rangeEnd: "2026-08-02" },
   ] });
   renderPage({ focus: { rangeStart: "2026-09-01", rangeEnd: "2026-09-01" } });
   expect(await screen.findByTestId("health-issue-snapshot_outdated")).toBeInTheDocument();
   expect(await screen.findByText("Manual A")).toBeInTheDocument();
   expect(screen.queryByRole("button", { name: "Preview repair" })).not.toBeInTheDocument();
   expect(screen.getByTestId("repair-all")).toBeDisabled();
   expect(screen.getByText(/Repair all covers the entire household/)).toBeInTheDocument();
 });

describe("Snapshot issue context", () => {
  beforeEach(() => {
    listAccounts.mockResolvedValue([{ account: { id: "broker", name: "Family brokerage" } }, { account: { id: "archived", name: "Previous bank", archivedAt: "2026-09-21T00:00:00Z" } }]);
    getCurrentSyncJob.mockResolvedValue({ jobId: "" });
  });

  it("identifies account and currency, combines only matching adjacent dates, and preserves inspection scope", async () => {
    const issue = (id: string, accountId: string, currency: string, date: string, reason = "snapshot_incomplete") => ({
      id, kind: "snapshot_incomplete", severity: "warning", groupKey: "snapshot", targetKey: "snapshot", accountId,
      label: currency, rangeStart: date, rangeEnd: date, rangeCount: 1, reason, action: "none", executable: false,
    });
    const issues = [
      issue("usd-18", "broker", "USD", "2026-09-18"), issue("usd-19", "broker", "USD", "2026-09-19"), issue("usd-21", "broker", "USD", "2026-09-21"),
      issue("sgd-18", "broker", "SGD", "2026-09-18"), issue("sgd-19", "broker", "SGD", "2026-09-19"),
      issue("old-18", "archived", "USD", "2026-09-18"), issue("old-19", "archived", "USD", "2026-09-19"),
      issue("reason-18", "broker", "USD", "2026-09-18", "missing_current_input"),
    ];
    scanHealth.mockResolvedValue({ healthy: false, issueCount: 8, executableCount: 0, prerequisiteCount: 0, snapshotDays: 3, issues });
    const navigation = { open: vi.fn(), openHealth: vi.fn() };
    renderPage({}, navigation);
    const rows = await screen.findAllByTestId("health-issue-snapshot_incomplete");
    await waitFor(() => expect(rows).toHaveLength(5));
    expect(screen.getByText("8")).toBeInTheDocument();
    expect(screen.getByText("Family brokerage · SGD")).toBeInTheDocument();
    expect(screen.getByText("Previous bank · USD")).toBeInTheDocument();
    expect(screen.queryByText("snapshot", { exact: true })).not.toBeInTheDocument();
    const first = rows[0];
    expect(first).toHaveTextContent("Family brokerage · USD");
    expect(first).toHaveTextContent("2026-09-18–2026-09-19");
    expect(first).toHaveTextContent("2 affected days");
    await userEvent.click(within(first).getByRole("button", { name: "View gap" }));
    expect(navigation.open).toHaveBeenCalledWith({ page: "data-health", focus: expect.objectContaining({ accountId: "broker", rangeStart: "2026-09-18", rangeEnd: "2026-09-19", id: "", label: "Family brokerage · USD", snapshotComponentLabel: "USD" }) });
    expect(navigation.openHealth).not.toHaveBeenCalled();
    expect(listAccounts).toHaveBeenCalledWith(expect.objectContaining({ includeArchived: true }));
    expect(issues[0].rangeEnd).toBe("2026-09-18");
  });

  it("uses a translated name for household snapshot issues without an account", async () => {
    scanHealth.mockResolvedValue({ healthy: false, issueCount: 1, executableCount: 0, prerequisiteCount: 0, snapshotDays: 1, issues: [
      { id: "snapshot", kind: "snapshot_incomplete", severity: "warning", targetKey: "snapshot", groupKey: "snapshot", rangeStart: "2026-09-18", rangeEnd: "2026-09-18", action: "none", executable: false },
    ] });
    renderPage();
    expect(await screen.findByText("Household valuation snapshot")).toBeVisible();
    expect(screen.queryByText("snapshot", { exact: true })).not.toBeInTheDocument();
    expect(screen.getByTestId("repair-all")).toBeDisabled();
  });

  it("keeps a grouped cash focus on its currency and dates without including other assets", async () => {
    const issue = (id: string, label: string, date: string, instrumentId?: string): HealthIssueDTO => ({
      id, kind: "snapshot_incomplete", severity: "warning", groupKey: "snapshot", targetKey: "snapshot", accountId: "broker",
      label, instrumentId, rangeStart: date, rangeEnd: date, rangeCount: 1, action: "none", executable: false,
    });
    const issues = [
      issue("sgd-18", "SGD", "2026-09-18"), issue("sgd-19", "SGD", "2026-09-19"), issue("sgd-21", "SGD", "2026-09-21"),
      issue("usd-18", "USD", "2026-09-18"), issue("stock-18", "Equity fund", "2026-09-18", "fund"),
      issue("same-name-stock", "SGD", "2026-09-18", "other-fund"),
      { ...issue("other-kind", "SGD", "2026-09-18"), kind: "incomplete_valuation" },
      { ...issue("other-reason", "SGD", "2026-09-18"), reason: "missing_current_input" },
      { ...issue("other-account", "SGD", "2026-09-18"), accountId: "archived" },
    ];
    scanHealth.mockResolvedValue({ healthy: false, issueCount: issues.length, executableCount: 0, prerequisiteCount: 0, snapshotDays: 3, issues });
    const navigation = { open: vi.fn(), openHealth: vi.fn() };
    const page = renderPage({}, navigation);
    const rows = await screen.findAllByTestId("health-issue-snapshot_incomplete");
    const groupedRow = rows.find((row) => row.textContent?.includes("2 affected days"))!;
    await userEvent.click(within(groupedRow).getByRole("button", { name: "View gap" }));
    const focus = navigation.open.mock.calls[0][0].focus as HealthFocus;
    expect(focus.snapshotComponentLabel).toBe("SGD");
    expect(focus.label).toBe("Family brokerage · SGD");
    expect(focusedHealthIssues(issues, focus).map((item) => item.id)).toEqual(["sgd-18", "sgd-19"]);
    page.unmount();
    renderPage({ focus });
    const focusedRows = await screen.findAllByTestId("health-issue-snapshot_incomplete");
    expect(focusedRows).toHaveLength(1);
    expect(focusedRows[0]).toHaveTextContent("Family brokerage · SGD");
    expect(focusedRows[0]).toHaveTextContent("2026-09-18–2026-09-19");
    expect(screen.queryByText("Family brokerage · USD")).not.toBeInTheDocument();
    expect(screen.queryByText("Family brokerage · Equity fund")).not.toBeInTheDocument();
  });
});
