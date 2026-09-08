import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { useAnalysisStore } from "@/stores/analysis";
import { ReturnTrendTab } from "./ReturnTrendTab";
import { ContributionTab } from "./ContributionTab";
import { AssetTrendTab } from "./AssetTrendTab";
import { CategoriesTab } from "./CategoriesTab";

const returnTrend = vi.fn();
const contribution = vi.fn();
const contributionItem = vi.fn();
const assetTrend = vi.fn();
const categories = vi.fn();
const categoryDetail = vi.fn();
const historyOrigin = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis", () => ({
  Service: {
    ReturnTrend: (...args: unknown[]) => returnTrend(...args),
    Contribution: (...args: unknown[]) => contribution(...args),
    ContributionItem: (...args: unknown[]) => contributionItem(...args),
    AssetTrend: (...args: unknown[]) => assetTrend(...args),
    Categories: (...args: unknown[]) => categories(...args),
    CategoryDetail: (...args: unknown[]) => categoryDetail(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: { HistoryOrigin: (...args: unknown[]) => historyOrigin(...args) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { Load: () => Promise.resolve({ currency: "USD", weekStart: "monday", dateFormat: "year-first", timezone: "UTC" }) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: () => Promise.resolve([{ account: { id: "account-1", name: "Brokerage" } }]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => Promise.resolve([{ id: "instrument-1", name: "QQQ", archivedAt: null }]) },
}));
vi.mock("@/components/charts/TrendChart", () => ({
  TrendChart: () => <div data-testid="trend-chart" />,
}));

function renderWithClient(child: ReactNode) {
  const queryClient = createTestQueryClient({ retry: false });
  return render(<QueryClientProvider client={queryClient}>{child}</QueryClientProvider>);
}

function ConnectedReturnTrendTab() {
  const session = useAnalysisStore();
  return <ReturnTrendTab session={session} />;
}

const available = { available: true, status: "ok", valuationForced: null };

describe("Phase 5 insight tabs", () => {
  beforeEach(() => {
    useAnalysisStore.getState().reset();
    historyOrigin.mockReset();
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
    returnTrend.mockReset();
    contribution.mockReset();
    contributionItem.mockReset();
    assetTrend.mockReset();
    categories.mockReset();
    categoryDetail.mockReset();
    returnTrend.mockResolvedValue({ display: "cumulative_amount", points: [{ date: "2026-09-01", amount: { amount: "2", currency: "USD" }, value: { amount: "2", currency: "USD" }, rate: "0.02", ratedDays: 1, totalDays: 1, ...available }], sources: [{ key: "price_change", label: "price_change", amount: { amount: "2", currency: "USD" }, share: "1" }], amount: { amount: "2", currency: "USD" }, rate: "0.02", ratedDays: 1, totalDays: 1, ...available });
    contribution.mockResolvedValue({ returnType: "total_return", groupBy: "instrument", rows: [{ key: "instrument-1", label: "instrument-1", amount: { amount: "2", currency: "USD" }, rate: "0.02", ratedDays: 1, totalDays: 3, ...available }], ratedDays: 1, totalDays: 3, ...available });
    contributionItem.mockResolvedValue({ key: "instrument-1", label: "instrument-1", amount: { amount: "2", currency: "USD" }, rate: "0.02", ratedDays: 1, totalDays: 3, components: [{ key: "price_change", amount: { amount: "2", currency: "USD" } }], byAccount: [{ key: "account-1", accountId: "account-1", amount: { amount: "2", currency: "USD" } }], historyHint: { kinds: [], from: "2026-09-01", to: "2026-09-01", accountId: "account-1", instrumentId: "instrument-1" }, ...available });
    assetTrend.mockResolvedValue({ points: [{ period: "2026-09", value: { amount: "102", currency: "USD" }, rate: null, ratedDays: 1, totalDays: 1, ...available }], summary: { amount: "102", currency: "USD" }, rate: null, ratedDays: 1, totalDays: 1, ...available });
    categories.mockResolvedValue({ total: { amount: "10", currency: "USD" }, rows: [{ key: "account-1", label: "account-1", accountId: "account-1", amount: { amount: "10", currency: "USD" } }], ...available });
    categoryDetail.mockResolvedValue({ children: [{ key: "groceries", label: "Groceries", amount: { amount: "10", currency: "USD" } }], activityRefs: [{ date: "2026-09-01", activityId: "activity-1", accountId: "account-1", amount: { amount: "10", currency: "USD" } }], ...available });
  });

  afterEach(() => { cleanup(); vi.useRealTimers(); });

  it("keeps Return Trend displays separate and sends linked_rate to Wails", async () => {
    renderWithClient(<ConnectedReturnTrendTab />);
    expect(await screen.findByTestId("return-trend")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Display"), { target: { value: "linked_rate" } });
    await waitFor(() => expect(returnTrend).toHaveBeenCalledWith(expect.anything(), "linked_rate"));
    expect(await screen.findByText("Points show daily Modified Dietz rates; the linked period rate is shown in the summary.")).toBeInTheDocument();
  });

  it("applies a named Return Trend range through the shared filter store", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-08T12:00:00Z"));
    renderWithClient(<ConnectedReturnTrendTab />);
    await screen.findByTestId("return-trend");
    fireEvent.click(screen.getByRole("button", { name: "30D" }));
    await waitFor(() => expect(returnTrend).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-08-09", to: "2026-09-07" }), "cumulative_amount"));
    vi.useRealTimers();
  });

  it("renders component-level return sources instead of holding contributors", async () => {
    renderWithClient(<ReturnTrendTab session={useAnalysisStore.getState()} />);
    await screen.findByText("Price Change");
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(screen.queryByText("+100%")).not.toBeInTheDocument();
  });

  it("does not mark Custom as selected when the shared range is empty", async () => {
    renderWithClient(<ConnectedReturnTrendTab />);
    await screen.findByTestId("return-trend");
    expect(screen.getByRole("button", { name: "Custom" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Custom" })).not.toBeDisabled();
  });

  it("opens a Contribution item for the active independent view", async () => {
    renderWithClient(<ContributionTab session={useAnalysisStore.getState()} />);
    await screen.findByText("QQQ");
    expect(screen.getByText("◇")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /QQQ/ }));
    expect(await screen.findByText("Price Change")).toBeInTheDocument();
    expect(await screen.findByText("Brokerage")).toBeInTheDocument();
    expect(contributionItem).toHaveBeenCalledWith(expect.anything(), "total_return", "instrument", "instrument-1");
  });

  it("opens History from a contribution item with date and scope filters and empty kinds", async () => {
    const onOpenHistory = vi.fn();
    renderWithClient(<ContributionTab session={useAnalysisStore.getState()} onOpenHistory={onOpenHistory} />);
    fireEvent.click(await screen.findByRole("button", { name: /QQQ/ }));
    fireEvent.click(await screen.findByRole("button", { name: "View in History" }));
    expect(onOpenHistory).toHaveBeenCalledWith({
      from: "2026-09-01",
      to: "2026-09-01",
      accountId: "account-1",
      instrumentId: "instrument-1",
      kinds: undefined,
    });
  });

  it("starts Contribution on the navigated return type", async () => {
    useAnalysisStore.getState().setFilters({ returnType: "dividend_interest" });
    renderWithClient(<ContributionTab session={useAnalysisStore.getState()} />);
    await screen.findByTestId("contribution");
    expect(screen.getByLabelText("Return type")).toHaveValue("dividend_interest");
    expect(contribution).toHaveBeenCalledWith(expect.anything(), "dividend_interest", "instrument", "amount_desc");
  });

  it("hides Realized rate sort and currency grouping", async () => {
    renderWithClient(<ContributionTab session={useAnalysisStore.getState()} />);
    await screen.findByText("QQQ");
    fireEvent.change(screen.getByLabelText("Return type"), { target: { value: "realized" } });
    await screen.findByTestId("contribution");
    expect(screen.queryByRole("option", { name: "By currency" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "By asset class" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Return rate, high to low" })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: "By instrument" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "By account" })).toBeInTheDocument();
  });

  it("switches Asset Trend granularity and metric without changing shared filters", async () => {
    renderWithClient(<AssetTrendTab session={useAnalysisStore.getState()} />);
    await screen.findByTestId("asset-trend");
    fireEvent.change(screen.getByLabelText("Granularity"), { target: { value: "week" } });
    await screen.findByLabelText("Metric");
    fireEvent.change(screen.getByLabelText("Metric"), { target: { value: "return_rate" } });
    await waitFor(() => expect(assetTrend).toHaveBeenCalledWith(expect.anything(), "week", "return_rate"));
    expect(useAnalysisStore.getState().scope).toBe("portfolio");
  });

  it("opens a Categories detail sheet for the selected account row", async () => {
    renderWithClient(<CategoriesTab session={useAnalysisStore.getState()} />);
    await screen.findByText("Brokerage");
    fireEvent.click(screen.getByRole("button", { name: /Brokerage/ }));
    expect(await screen.findByText("Groceries")).toBeInTheDocument();
    expect(categoryDetail).toHaveBeenCalledWith(expect.anything(), "spending", "account-1");
  });

  it("opens History from a category record for the selected day and account", async () => {
    const onOpenHistory = vi.fn();
    renderWithClient(<CategoriesTab session={useAnalysisStore.getState()} onOpenHistory={onOpenHistory} />);
    fireEvent.click(await screen.findByRole("button", { name: /Brokerage/ }));
    fireEvent.click(await screen.findByRole("button", { name: "View in History" }));
    expect(onOpenHistory).toHaveBeenCalledWith({ from: "2026-09-01", to: "2026-09-01", accountId: "account-1", instrumentId: undefined });
  });
});
