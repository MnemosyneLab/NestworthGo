import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { useAnalysisStore } from "@/stores/analysis";
import type { AnalysisNavigationContext } from "@/app/navigation";
import { ReturnAnalysisPage } from "./ReturnAnalysisPage";

const returnCalendar = vi.fn();
const returnDay = vi.fn();
const historyOrigin = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis", () => ({
  Service: {
    ReturnCalendar: (...args: unknown[]) => returnCalendar(...args),
    ReturnDay: (...args: unknown[]) => returnDay(...args),
    ReturnTrend: vi.fn(),
    Contribution: vi.fn(),
    ContributionItem: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: () => Promise.resolve([{ account: { id: "a1", name: "Brokerage" } }]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => Promise.resolve([{ id: "i1", name: "QQQ" }]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { Load: () => Promise.resolve({ currency: "USD", weekStart: "monday", dateFormat: "year-first", timezone: "UTC" }) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: { HistoryOrigin: (...args: unknown[]) => historyOrigin(...args) },
}));

function renderPage(onOpenAssetChanges?: (analysis: AnalysisNavigationContext) => void) {
  const queryClient = createTestQueryClient();
  return render(<QueryClientProvider client={queryClient}><ReturnAnalysisPage onOpenAssetChanges={onOpenAssetChanges} /></QueryClientProvider>);
}

describe("ReturnAnalysisPage", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-07T12:00:00Z"));
    useAnalysisStore.getState().reset();
    returnCalendar.mockReset();
    returnDay.mockReset();
    historyOrigin.mockReset();
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    returnCalendar.mockResolvedValue({
      summary: {
        beginningInvestedValue: { amount: "100", currency: "USD" },
        endingInvestedValue: { amount: "101", currency: "USD" },
        returnAmount: { amount: "1", currency: "USD" },
        returnRate: "0.01",
        ratedDays: 1,
        totalDays: 2,
      },
      cells: [{
        date: "2026-09-01",
        beginningInvestedValue: { amount: "100", currency: "USD" },
        endingInvestedValue: { amount: "101", currency: "USD" },
        returnAmount: { amount: "1", currency: "USD" },
        returnRate: "0.01",
        composition: [{ component: "price_change", amount: { amount: "1", currency: "USD" } }],
        contributors: [],
        ratedDays: 1,
        totalDays: 1,
        issues: [],
        available: true,
        status: "ok",
        valuationForced: null,
      }, {
        date: "2026-09-02",
        returnAmount: { amount: "0", currency: "USD" },
        returnRate: null,
        composition: [],
        contributors: [],
        ratedDays: 0,
        totalDays: 1,
        issues: [{ date: "2026-09-02", status: "partial", missingReason: "quote missing" }],
        available: true,
        status: "partial",
        valuationForced: null,
      }, {
        date: "2026-09-03",
        returnAmount: { amount: "0", currency: "USD" },
        returnRate: "0",
        composition: [],
        contributors: [],
        ratedDays: 1,
        totalDays: 1,
        issues: [],
        available: true,
        status: "ok",
        valuationForced: null,
      }, {
        date: "2026-09-07",
        returnAmount: { amount: "99", currency: "USD" },
        returnRate: "0.99",
        composition: [{ component: "price_change", amount: { amount: "99", currency: "USD" } }],
        contributors: [],
        ratedDays: 1,
        totalDays: 1,
        issues: [],
        available: true,
        status: "ok",
        valuationForced: null,
      }],
      topContributors: [],
      issues: [{ date: "2026-09-02", status: "partial", missingReason: "quote missing" }],
      available: true,
      status: "partial",
      valuationForced: null,
    });
    returnDay.mockResolvedValue({
      date: "2026-09-01",
      returnAmount: { amount: "1", currency: "USD" },
      returnRate: "0.01",
      composition: [{ component: "price_change", amount: { amount: "1", currency: "USD" } }],
      contributors: [],
      ratedDays: 1,
      totalDays: 1,
      issues: [],
      available: true,
      status: "ok",
      valuationForced: null,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("marks partial coverage and opens the period issues list", async () => {
    const user = userEvent.setup();
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();

    expect(await screen.findByText("Partial coverage")).toBeInTheDocument();
    expect(screen.getByText("Today is not closed yet")).toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Issues/ }));
    expect(await screen.findByText("quote missing")).toBeInTheDocument();
  });

  it("uses the visible month as the calendar cursor when navigating", async () => {
    const user = userEvent.setup();
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    useAnalysisStore.getState().setFilters({ from: "2026-01-01", to: "2026-12-31" });
    renderPage();
    await screen.findByText("September 2026");
    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-01-01", to: "2026-09-06" }), "2026-10", "day");
  });

  it("opens the day sheet and carries the day and session into Asset Changes", async () => {
    const user = userEvent.setup();
    const onOpenAssetChanges = vi.fn();
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage(onOpenAssetChanges);

    await screen.findByText("Partial coverage");
    await user.click(within(screen.getByTestId("return-day-2026-09-01")).getByRole("button"));
    expect(await screen.findByRole("button", { name: "View Asset Changes for This Day" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "View Asset Changes for This Day" }));

    expect(onOpenAssetChanges).toHaveBeenCalledWith(expect.objectContaining({ scope: "portfolio", includeCash: true, from: "2026-09-01", to: "2026-09-01" }));
  });

  it("distinguishes a partial zero day from a complete zero day", async () => {
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();

    await screen.findByText("Partial coverage");
    expect(screen.getByTestId("return-day-2026-09-02")).toHaveTextContent("◇");
    expect(screen.getByTestId("return-day-2026-09-03")).not.toHaveTextContent("◇");
    expect(screen.getByTestId("return-day-2026-09-03")).not.toHaveClass("bg-success/8");
    expect(screen.getByTestId("return-day-2026-09-03")).not.toHaveClass("bg-destructive/7");
  });

  it("folds year cells into monthly cards with one ReturnCalendar request", async () => {
    const user = userEvent.setup();
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();

    await screen.findByText("Partial coverage");
    returnCalendar.mockClear();
    await user.click(screen.getByRole("button", { name: "Year" }));
    await waitFor(() => expect(returnCalendar).toHaveBeenCalledTimes(1));
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-01-01", to: "2026-09-06" }), "", "day");
    expect(await screen.findByText("+100.99%")).toBeInTheDocument();
  });

  it("uses product empty states for missing history, no investments, and insufficient history", async () => {
    historyOrigin.mockResolvedValueOnce(null);
    renderPage();
    expect(await screen.findByText("History is not ready for return analysis.")).toBeInTheDocument();
    cleanup();

    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    returnCalendar.mockResolvedValueOnce({ available: false, status: "unavailable", cells: [], issues: [], topContributors: [], summary: { ratedDays: 0, totalDays: 0, beginningInvestedValue: null, endingInvestedValue: null, returnAmount: null, returnRate: null } });
    useAnalysisStore.getState().reset();
    renderPage();
    expect(await screen.findByText("No investment assets are available for this scope.")).toBeInTheDocument();
    cleanup();

    returnCalendar.mockResolvedValueOnce({ available: false, status: "unavailable", cells: [{ date: "2026-09-01", available: false, status: "unavailable", ratedDays: 0, totalDays: 1, returnAmount: null, returnRate: null, composition: [], contributors: [], issues: [], valuationForced: null }], issues: [], topContributors: [], summary: { ratedDays: 0, totalDays: 1, beginningInvestedValue: null, endingInvestedValue: null, returnAmount: null, returnRate: null } });
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();
    expect(await screen.findByText("Not enough historical data to calculate returns.")).toBeInTheDocument();
  });
});
