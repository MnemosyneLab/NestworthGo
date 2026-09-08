import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { useAnalysisStore } from "@/stores/analysis";
import type { AnalysisNavigationContext } from "@/app/navigation";
import { AssetChangesPage } from "./AssetChangesPage";

const assetChange = vi.fn();
const assetDriverDetail = vi.fn();
const historyOrigin = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis", () => ({
  Service: {
    AssetChange: (...args: unknown[]) => assetChange(...args),
    AssetDriverDetail: (...args: unknown[]) => assetDriverDetail(...args),
    AssetTrend: vi.fn(),
    Categories: vi.fn(),
    CategoryDetail: vi.fn(),
    ReturnCalendar: vi.fn(),
    ReturnDay: vi.fn(),
    ReturnTrend: vi.fn(),
    Contribution: vi.fn(),
    ContributionItem: vi.fn(),
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
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", () => ({
  Service: { Catalog: () => Promise.resolve({}) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: { ListMembers: () => Promise.resolve([]) },
}));

function renderPage(props: { onOpenHistory?: (filters: { from?: string; to?: string; accountId?: string; instrumentId?: string }) => void; onOpenReturnAnalysis?: (analysis: AnalysisNavigationContext) => void } = {}) {
  const queryClient = createTestQueryClient();
  return render(<QueryClientProvider client={queryClient}><AssetChangesPage {...props} /></QueryClientProvider>);
}

describe("AssetChangesPage", () => {
  beforeEach(() => {
    useAnalysisStore.getState().reset();
    useAnalysisStore.getState().setFilters({ from: "2026-09-01", to: "2026-09-06" });
    historyOrigin.mockReset();
    assetChange.mockReset();
    assetDriverDetail.mockReset();
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    assetChange.mockResolvedValue({
      summary: {
        beginningValue: { amount: "100", currency: "USD" },
        endingValue: { amount: "125", currency: "USD" },
        change: { amount: "25", currency: "USD" },
      },
      waterfall: [
        { key: "income", label: "Income", bucket: "income", amount: { amount: "10", currency: "USD" } },
        { key: "price_change", label: "Price change", bucket: "price_change", amount: { amount: "20", currency: "USD" } },
        { key: "residual", label: "Unexplained difference", bucket: "residual", amount: { amount: "-5", currency: "USD" } },
      ],
      groups: [
        { key: "cash", label: "Cash flows", amount: { amount: "10", currency: "USD" }, rows: [{ key: "income", label: "Income", bucket: "income", amount: { amount: "10", currency: "USD" } }] },
        { key: "market", label: "Market & investment", amount: { amount: "20", currency: "USD" }, rows: [
          { key: "price_change", label: "Price change", bucket: "price_change", amount: { amount: "20", currency: "USD" } },
          { key: "dividend_interest", label: "Dividend & interest", bucket: "dividend_interest", amount: { amount: "20", currency: "USD" } },
        ] },
        { key: "other", label: "Other", amount: { amount: "-5", currency: "USD" }, rows: [{ key: "residual", label: "Unexplained difference", bucket: "residual", amount: { amount: "-5", currency: "USD" } }] },
      ],
      available: true,
      status: "partial",
      missingReason: "one component is incomplete",
      valuationForced: null,
    });
    assetDriverDetail.mockResolvedValue({
      driverKey: "residual",
      byInstrument: [],
      byAccount: [],
      residualDetails: [{ date: "2026-09-02", componentKey: "account-1/holding:h1/instrument:instrument-1", accountId: "account-1", holdingId: "h1", instrumentId: "instrument-1", amount: { amount: "-5", currency: "USD" } }],
      available: true,
      status: "partial",
      missingReason: "one component is incomplete",
      valuationForced: null,
    });
  });

  afterEach(() => cleanup());

  it("renders a reconciling summary, waterfall, grouped drivers, and residual detail", async () => {
    const user = userEvent.setup();
    renderPage();
    expect(await screen.findByText("Net Worth Change")).toBeInTheDocument();
    expect(screen.getByText("+$25.00")).toBeInTheDocument();
    expect(screen.getByTestId("asset-waterfall")).toBeInTheDocument();
    expect(screen.getByText("Change attribution")).toBeInTheDocument();
    expect(screen.getByText("Cash flows")).toBeInTheDocument();
    expect(screen.getByText("Market & Investment")).toBeInTheDocument();
    expect(screen.getByText("Other")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Income/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Price Change/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Dividend & Interest/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Unexplained difference/ })).toHaveClass("border-warning/40");
    expect(assetChange).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-09-01", to: "2026-09-06" }));
    await screen.findByRole("option", { name: "Brokerage" });

    await user.click(screen.getByRole("button", { name: /Unexplained difference/ }));
    expect(await screen.findByText("Component / day")).toBeInTheDocument();
    expect(screen.getByText("account-1/holding:h1/instrument:instrument-1")).toBeInTheDocument();
    expect(screen.getByText("2026-09-02")).toBeInTheDocument();
    expect(await screen.findByText("Brokerage · QQQ")).toBeInTheDocument();
    expect(assetDriverDetail).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-09-01", to: "2026-09-06" }), "residual");
  });

  it("opens the matching History day and dimensions from residual detail", async () => {
    const user = userEvent.setup();
    const onOpenHistory = vi.fn();
    renderPage({ onOpenHistory });
    await user.click(await screen.findByRole("button", { name: /Unexplained difference/ }));
    await user.click(await screen.findByRole("button", { name: "View in History" }));
    expect(onOpenHistory).toHaveBeenCalledWith({ from: "2026-09-02", to: "2026-09-02", accountId: "account-1", instrumentId: "instrument-1" });
    expect(screen.queryByRole("button", { name: "Open Return Analysis" })).not.toBeInTheDocument();
  });

  it("opens Contribution from a Price Change driver with the shared analysis window", async () => {
    const user = userEvent.setup();
    const onOpenReturnAnalysis = vi.fn();
    assetDriverDetail.mockResolvedValue({
      driverKey: "price_change",
      byInstrument: [{ key: "instrument-1", instrumentId: "instrument-1", label: "QQQ", amount: { amount: "20", currency: "USD" } }],
      byAccount: [{ key: "account-1", accountId: "account-1", label: "Brokerage", amount: { amount: "20", currency: "USD" } }],
      residualDetails: [],
      available: true,
      status: "ok",
      missingReason: null,
      valuationForced: null,
    });
    renderPage({ onOpenReturnAnalysis });
    await user.click(await screen.findByRole("button", { name: /Price Change/ }));
    await user.click(await screen.findByRole("button", { name: "Open Return Analysis" }));
    expect(onOpenReturnAnalysis).toHaveBeenCalledWith(expect.objectContaining({
      scope: "portfolio",
      includeCash: true,
      from: "2026-09-01",
      to: "2026-09-06",
      returnType: "total_return",
    }));
  });

  it("does not offer Contribution from cash-flow drivers", async () => {
    const user = userEvent.setup();
    const onOpenReturnAnalysis = vi.fn();
    assetDriverDetail.mockResolvedValue({
      driverKey: "income",
      byInstrument: [],
      byAccount: [{ key: "account-1", accountId: "account-1", label: "Brokerage", amount: { amount: "10", currency: "USD" } }],
      residualDetails: [],
      available: true,
      status: "ok",
      missingReason: null,
      valuationForced: null,
    });
    renderPage({ onOpenReturnAnalysis });
    await user.click(await screen.findByRole("button", { name: /Income/ }));
    await waitFor(() => expect(assetDriverDetail).toHaveBeenCalledWith(expect.anything(), "income"));
    expect(screen.queryByRole("button", { name: "Open Return Analysis" })).not.toBeInTheDocument();
    expect(onOpenReturnAnalysis).not.toHaveBeenCalled();
  });

  it("does not render a zero-value driver row and preserves the partial marker", async () => {
    renderPage();
    expect(await screen.findByText("Partial coverage")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Zero/ })).not.toBeInTheDocument();
    expect(within(screen.getByTestId("change-drivers")).getByText("one component is incomplete")).toBeInTheDocument();
  });

  it("shows the product empty state when no asset days are available", async () => {
    assetChange.mockResolvedValueOnce({ available: false, status: "unavailable", missingReason: "no usable asset days are available for this period", waterfall: [], groups: [], summary: {}, valuationForced: null });
    renderPage();
    expect(await screen.findByText("No asset change data for this period.")).toBeInTheDocument();
    expect(screen.getByText("no usable asset days are available for this period")).toBeInTheDocument();
  });
});
