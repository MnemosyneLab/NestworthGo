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
const returnTrend = vi.fn();
const contribution = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis", () => ({
  Service: {
    ReturnCalendar: (...args: unknown[]) => returnCalendar(...args),
    ReturnDay: (...args: unknown[]) => returnDay(...args),
    ReturnTrend: (...args: unknown[]) => returnTrend(...args),
    Contribution: (...args: unknown[]) => contribution(...args),
    ContributionItem: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: () => Promise.resolve([{ account: { id: "a1", name: "Brokerage" } }]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => Promise.resolve([{ id: "i1", name: "QQQ" }]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: { Bootstrap: () => Promise.resolve({ household: { id: "h1", baseCurrency: "USD" } }) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { Load: () => Promise.resolve({ currency: "USD", weekStart: "monday", dateFormat: "year-first", timezone: "UTC" }) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: { HistoryOrigin: (...args: unknown[]) => historyOrigin(...args) },
}));

function renderPage(onOpenAssetChanges?: (analysis: AnalysisNavigationContext) => void, options?: Parameters<typeof createTestQueryClient>[0]) {
  const queryClient = createTestQueryClient(options);
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
    returnTrend.mockReset().mockResolvedValue({ available: false });
    contribution.mockReset().mockResolvedValue({ available: false });
    returnCalendar.mockResolvedValue({
      summary: {
        beginningInvestedValue: { amount: "100", currency: "USD" },
        endingInvestedValue: { amount: "101", currency: "USD" },
        returnAmount: { amount: "1", currency: "USD" },
        returnRate: "0.01",
        ratedDays: 1,
        totalDays: 2,
      },
      months: [{ month: "2026-09", returnAmount: { amount: "100", currency: "USD" }, amountStatus: "partial", returnRate: "1.0099", ratedDays: 3, totalDays: 4 }],
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

  it.each(["trend", "contribution"] as const)("keeps the browsed month and object filters when switching to %s", async (tab) => {
    useAnalysisStore.getState().setFilters({ scope: "instrument", scopeId: "i1", valuation: "native", includeCash: false, moreFilters: { accountId: "a1", currency: "USD" } });
    renderPage();
    const user = userEvent.setup();
    await screen.findByText("September 2026");
    await user.click(screen.getByRole("button", { name: "Previous" }));
    await user.click(screen.getByRole("tab", { name: tab === "trend" ? "Return trend" : "Contribution" }));
    await waitFor(() => expect(tab === "trend" ? returnTrend : contribution).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-08-01", to: "2026-08-31", scopeKind: "instrument", scopeId: "i1", accountId: "a1", currency: "USD", valuation: "native", includeCash: false }), ... (tab === "trend" ? ["cumulative_amount"] : ["total_return", "instrument", "amount_desc"])));
    await user.click(screen.getByRole("tab", { name: "Return calendar" }));
    expect(await screen.findByText("August 2026")).toBeInTheDocument();
    expect(useAnalysisStore.getState()).toMatchObject({ from: "2026-08-01", to: "2026-08-31" });
  });

  it("updates the retained contribution view when a new metric arrives through navigation", async () => {
    useAnalysisStore.getState().setFilters({ from: "2026-08-01", to: "2026-08-31" });
    useAnalysisStore.getState().setReturnView({ tab: "contribution" });
    renderPage();
    expect(await screen.findByRole("combobox", { name: "Return type" })).toHaveValue("total_return");
    useAnalysisStore.getState().replaceFilters({ from: "2026-08-01", to: "2026-08-31", returnType: "dividend_interest" });
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Return type" })).toHaveValue("dividend_interest"));
    await waitFor(() => expect(contribution).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-08-01", to: "2026-08-31" }), "dividend_interest", "instrument", "amount_desc"));
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Return type" }), "realized");
    expect(useAnalysisStore.getState().contributionReturnType).toBe("realized");
  });

  it("normalizes grouping and sorting when retained contribution switches to realized", async () => {
    useAnalysisStore.getState().setFilters({ from: "2026-08-01", to: "2026-08-31" });
    useAnalysisStore.getState().setReturnView({ tab: "contribution" });
    renderPage();
    const user = userEvent.setup();
    await user.selectOptions(await screen.findByRole("combobox", { name: "Group by" }), "currency");
    await user.selectOptions(screen.getByRole("combobox", { name: "Sort" }), "rate_desc");
    contribution.mockClear();
    useAnalysisStore.getState().replaceFilters({ from: "2026-08-01", to: "2026-08-31", returnType: "realized" });
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Return type" })).toHaveValue("realized"));
    expect(screen.getByRole("combobox", { name: "Group by" })).toHaveValue("instrument");
    expect(screen.getByRole("combobox", { name: "Sort" })).toHaveValue("amount_desc");
    await waitFor(() => expect(contribution).toHaveBeenCalledWith(expect.anything(), "realized", "instrument", "amount_desc"));
    expect(contribution.mock.calls.filter(([, metric]) => metric === "realized").every(([, , grouping, sort]) => grouping === "instrument" && sort === "amount_desc")).toBe(true);
  });

  it("keeps a browsed year clamped to the origin when leaving the calendar", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2025-04-10T00:00:00Z" });
    renderPage();
    const user = userEvent.setup();
    await screen.findByText("September 2026");
    await user.click(screen.getByRole("button", { name: "Year" }));
    await user.click(screen.getByRole("button", { name: "Previous" }));
    await user.click(screen.getByRole("tab", { name: "Return trend" }));
    await waitFor(() => expect(returnTrend).toHaveBeenCalledWith(expect.objectContaining({ from: "2025-04-10", to: "2025-12-31" }), "cumulative_amount"));
  });

  it("keeps an explicit cross-month selection when leaving a different visible calendar month", async () => {
    useAnalysisStore.getState().setFilters({ from: "2026-06-15", to: "2026-08-12", scope: "account", scopeId: "a1" });
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();
    const user = userEvent.setup();
    await screen.findByText("September 2026");
    await user.click(screen.getByRole("tab", { name: "Return trend" }));
    await waitFor(() => expect(returnTrend).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-06-15", to: "2026-08-12", scopeKind: "account", scopeId: "a1" }), "cumulative_amount"));
  });

  it("does not store a reversed period when the month has no closed day", async () => {
    vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
    renderPage();
    const user = userEvent.setup();
    await screen.findByText("No closed dates in the current month yet.");
    await user.click(screen.getByRole("tab", { name: "Return trend" }));
    expect(useAnalysisStore.getState()).toMatchObject({ from: "", to: "" });
    expect(returnTrend).not.toHaveBeenCalled();
  });

  it("keeps the month-start calendar muted and hides the reversed filter range", async () => {
    vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
    renderPage();
    expect(await screen.findByText("No closed dates in the current month yet.")).toBeInTheDocument();
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-09-28", to: "2026-09-30" }), "", "day");
    expect(returnDay).not.toHaveBeenCalled();
    const today = screen.getByTestId("return-day-2026-10-01");
    expect(today).toHaveTextContent("Today is not closed yet");
    expect(within(today).getByRole("button")).toBeDisabled();
    expect(today).not.toHaveTextContent("0%");
    expect(screen.queryByText("2026-10-01 – 2026-09-30")).not.toBeInTheDocument();
    expect(screen.getByLabelText("From")).not.toHaveTextContent("2026-10-01");
    expect(screen.getByLabelText("To")).not.toHaveTextContent("2026-09-30");
    expect(useAnalysisStore.getState()).toMatchObject({ from: "", to: "", returnCursor: "2026-10" });
    await userEvent.setup().click(screen.getByRole("button", { name: "Previous" }));
    await waitFor(() => expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-09-01", to: "2026-09-30" }), "2026-09", "day"));
    expect(screen.queryByText("No closed dates in the current month yet.")).not.toBeInTheDocument();
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
    await user.click(screen.getByRole("button", { name: "Previous" }));
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-08-01", to: "2026-08-31" }), "2026-08", "day");
  });

  it.each(["grid", "summary"])("retains available data when the %s request fails and retries only that section", async failed => {
    const base = await returnCalendar();
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    returnCalendar.mockImplementation((_request, cursor) => Boolean(cursor) === (failed === "summary") ? Promise.reject(new Error("synthetic read failure")) : Promise.resolve(base));
    renderPage(undefined, { retry: false });
    const region = await screen.findByRole("region", { name: failed === "grid" ? "Calendar view" : "Return summary" });
    await within(region).findByRole("alert");
    if (failed === "grid") expect(within(screen.getByRole("region", { name: "Return summary" })).getByText("+US$1.00")).toBeInTheDocument();
    else expect(screen.getByTestId("return-day-2026-09-01")).toHaveTextContent("+US$1.00");
    returnCalendar.mockClear().mockResolvedValue(base);
    await userEvent.click(within(region).getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(within(region).queryByRole("alert")).not.toBeInTheDocument());
    expect(returnCalendar).toHaveBeenCalledTimes(1);
    expect(returnCalendar.mock.calls[0][1]).toBe(failed === "grid" ? "" : "2026-09");
  });

  it.each(["pending", "failed", "empty"])("keeps a selected-range summary while the independent year grid is %s", async state => {
    const base = await returnCalendar();
    useAnalysisStore.getState().setFilters({ valuation: "native", from: "2026-09-02", to: "2026-09-03" });
    const selected = { ...base, valuation: "native", currency: "CNY", summary: { ...base.summary, returnAmount: { amount: "7", currency: "CNY" } } };
    const annual = { ...base, valuation: "base", currency: "USD", valuationForced: "base" };
    let finish!: (value: typeof annual) => void;
    returnCalendar.mockImplementation((request, cursor) => {
      if (request.from !== "2026-01-01" || cursor) return Promise.resolve(selected);
      if (state === "failed") return Promise.reject(new Error("synthetic annual failure"));
      if (state === "empty") return Promise.resolve({ ...annual, available: false, cells: [] });
      return new Promise(resolve => { finish = resolve; });
    });
    renderPage(undefined, { retry: false });
    await screen.findByText("Summary valuation: Native (CNY)");
    await userEvent.click(screen.getByRole("button", { name: "Year" }));
    const summary = screen.getByRole("region", { name: "Return summary" });
    const calendar = await screen.findByRole("region", { name: "Calendar view" });
    expect(within(summary).getByText("+CN¥7.00")).toBeInTheDocument();
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-09-02", to: "2026-09-03" }), "", "day");
    await waitFor(() => expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-01-01", to: "2026-09-06" }), "", "day"));
    if (state === "pending") {
      expect(within(calendar).getByRole("status")).toBeInTheDocument();
      finish(annual);
    } else if (state === "failed") {
      await within(calendar).findByRole("alert");
      returnCalendar.mockClear().mockResolvedValue(annual);
      await userEvent.click(within(calendar).getByRole("button", { name: "Try again" }));
      await waitFor(() => expect(within(calendar).queryByRole("alert")).not.toBeInTheDocument());
      expect(returnCalendar).toHaveBeenCalledTimes(1);
      expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-01-01", to: "2026-09-06" }), "", "day");
    } else {
      expect(await within(calendar).findByText("No investment assets are available for this scope.")).toBeInTheDocument();
      expect(within(summary).getByText("+CN¥7.00")).toBeInTheDocument();
      return;
    }
    expect(await within(calendar).findByText(/Calendar valuation: Base \(USD\)/)).toBeInTheDocument();
    expect(within(calendar).getByText("Native valuation is unavailable; showing base currency.")).toBeInTheDocument();
    expect(within(calendar).getByRole("button", { name: /September/ })).toHaveTextContent("+US$100.00");
    expect(within(summary).getByText("Summary valuation: Native (CNY)")).toBeInTheDocument();
    expect(within(summary).getByText("+CN¥7.00")).toBeInTheDocument();
  });

  it.each(["native", "base", "unavailable"])("shows the day response's actual %s valuation independently of the summary", async mode => {
    const base = await returnDay();
    const calendar = await returnCalendar();
    useAnalysisStore.getState().setFilters({ valuation: "native" });
    const currency = mode === "native" ? "CNY" : mode === "base" ? "USD" : "";
    const selected = { ...calendar, valuation: "native", currency: "CNY", summary: { ...calendar.summary, returnAmount: { amount: "7", currency: "CNY" } } };
    const grid = { ...calendar, valuation: mode === "native" ? "native" : "base", currency, valuationForced: mode === "native" ? null : "base", cells: calendar.cells.map((cell: object) => ({ ...cell, returnAmount: currency ? { amount: "7", currency } : null, amountStatus: currency ? "complete" : "unavailable", available: Boolean(currency) })) };
    returnCalendar.mockImplementation((_request, cursor) => Promise.resolve(cursor ? selected : grid));
    returnDay.mockResolvedValue({ ...base, available: Boolean(currency), status: currency ? "ok" : "unavailable", valuation: mode === "native" ? "native" : "base", currency, valuationForced: mode === "native" ? null : "base", returnAmount: currency ? { amount: "7", currency } : null, amountStatus: currency ? "complete" : "unavailable" });
    renderPage();
    const date = await screen.findByTestId("return-day-2026-09-01");
    await userEvent.click(within(date).getByRole("button"));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(mode === "native" ? "Day valuation: Native (CNY)" : mode === "base" ? /Day valuation: Base \(USD\)/ : /Day valuation: Base \(Unavailable\)/)).toBeInTheDocument();
    if (mode === "native") expect(within(dialog).queryByText("Native valuation is unavailable; showing base currency.")).not.toBeInTheDocument();
    else expect(within(dialog).getByText("Native valuation is unavailable; showing base currency.")).toBeInTheDocument();
    expect(returnDay).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-08-31", to: "2026-09-06", valuation: "native" }), "2026-09-01");
  });

  it("shows the selected summary while the cross-month grid is pending", async () => {
    const base = await returnCalendar();
    returnCalendar.mockImplementation((_request, cursor) => cursor ? Promise.resolve(base) : new Promise(() => {}));
    renderPage();
    expect(await within(await screen.findByRole("region", { name: "Return summary" })).findByText("+US$1.00")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Calendar view" })).getByRole("status")).toBeInTheDocument();
  });

  it.each([false, true])("labels independent native and forced-base periods, including missing FX: %s", async missingFX => {
    const base = await returnCalendar();
    vi.setSystemTime(new Date("2026-12-10T12:00:00Z"));
    useAnalysisStore.getState().setReturnView({ cursor: "2026-11" });
    useAnalysisStore.getState().setFilters({ valuation: "native", from: "2026-11-02", to: "2026-11-03" });
    const native = { ...base, valuation: "native", currency: "CNY", summary: { ...base.summary, returnAmount: { amount: "7", currency: "CNY" } } };
    const grid = { ...base, valuation: "base", currency: missingFX ? "" : "USD", valuationForced: "base", cells: [{ ...base.cells[0], date: "2026-10-26", returnAmount: missingFX ? null : { amount: "1", currency: "USD" }, amountStatus: missingFX ? "unavailable" : "complete", status: missingFX ? "partial" : "ok" }] };
    returnCalendar.mockImplementation((request, cursor) => Promise.resolve(cursor || request.from === "2026-11-02" ? native : grid));
    renderPage();
    expect(await screen.findByText("Summary valuation: Native (CNY)")).toBeInTheDocument();
    expect(screen.getByText(missingFX ? /Calendar valuation: Base \(Unavailable\)/ : /Calendar valuation: Base \(USD\)/)).toBeInTheDocument();
    expect(screen.getByTestId("return-day-2026-10-26")).toHaveTextContent(missingFX ? "—" : "+US$1.00");
    await userEvent.click(within(screen.getByTestId("return-day-2026-10-26")).getByRole("button"));
    await screen.findByRole("dialog");
    expect(returnDay).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-10-26", to: "2026-12-06", valuation: "native" }), "2026-10-26");
  });

  it("loads the whole visible month while keeping a separate selected-range summary", async () => {
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    useAnalysisStore.getState().setFilters({ from: "2026-09-02", to: "2026-09-03" });
    renderPage();
    await screen.findByText("Partial coverage");
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-09-01", to: "2026-09-06" }), "2026-09", "day");
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-09-02", to: "2026-09-03" }), "", "day");
    expect(screen.getByTestId("return-day-2026-09-01")).toHaveTextContent("+US$1.00");
    expect(screen.getByTestId("return-day-2026-09-01")).not.toHaveAttribute("data-selected");
    expect(screen.getByTestId("return-day-2026-09-02")).toHaveAttribute("data-selected", "true");
  });

  it("keeps usable adjacent dates visible when the selected month has no investment data", async () => {
    vi.setSystemTime(new Date("2026-12-10T12:00:00Z"));
    const base = await returnCalendar();
    returnCalendar.mockImplementation((_request, cursor) => Promise.resolve(cursor ? {
      ...base, available: false, cells: [], topContributors: [],
      summary: { ...base.summary, returnAmount: null, returnRate: null },
    } : { ...base, cells: [
      { ...base.cells[0], date: "2026-10-26", returnAmount: { amount: "12.34", currency: "USD" } },
      { ...base.cells[0], date: "2026-12-01", returnAmount: { amount: "-7", currency: "USD" } },
    ] }));
    useAnalysisStore.getState().setReturnView({ cursor: "2026-11" });
    renderPage();
    expect(await screen.findByText("No investment assets are available for this scope.")).toBeInTheDocument();
    expect(screen.getByTestId("return-day-2026-10-26")).toHaveTextContent("+US$12.34");
    expect(screen.getByTestId("return-day-2026-12-01")).toHaveTextContent("-US$7.00");
    expect(screen.queryByText("Return amount")).not.toBeInTheDocument();
    await userEvent.setup().click(within(screen.getByTestId("return-day-2026-10-26")).getByRole("button"));
    await screen.findByRole("dialog");
    expect(returnDay).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-10-26", to: "2026-12-06" }), "2026-10-26");
    expect(useAnalysisStore.getState().returnCursor).toBe("2026-11");
  });

  it("loads adjacent-month gains without changing the selected summary or navigating on day click", async () => {
    vi.setSystemTime(new Date("2026-12-10T12:00:00Z"));
    const base = await returnCalendar();
    const daily = base.cells[0];
    returnCalendar.mockImplementation((request) => Promise.resolve(request.from === "2026-10-26" ? {
      ...base,
      summary: { ...base.summary, returnAmount: { amount: "999", currency: "USD" } },
      topContributors: [{ key: "grid-only", label: "Grid-only contributor", amount: { amount: "999", currency: "USD" } }],
      cells: [
        { ...daily, date: "2026-10-26", returnAmount: { amount: "12.34", currency: "USD" }, returnRate: "0.02", amountStatus: "partial", status: "partial" },
        { ...daily, date: "2026-10-27", returnAmount: null, returnRate: null, available: false, amountStatus: "unavailable", status: "unavailable" },
        { ...daily, date: "2026-12-01", returnAmount: { amount: "-7", currency: "USD" }, returnRate: "-0.03" },
      ],
    } : base));
    useAnalysisStore.getState().setReturnView({ cursor: "2026-11" });
    useAnalysisStore.getState().setFilters({ from: "2026-11-05", to: "2026-11-20" });
    renderPage();
    await waitFor(() => expect(screen.getByTestId("return-day-2026-10-26")).toHaveTextContent("+US$12.34"));
    expect(screen.getByTestId("return-day-2026-10-26")).toHaveTextContent("Oct 26");
    expect(screen.getByTestId("return-day-2026-10-26")).toHaveTextContent("+2%");
    expect(screen.getByTestId("return-day-2026-10-26")).toHaveTextContent("Known in part");
    expect(screen.getByTestId("return-day-2026-10-27")).toHaveTextContent("No data");
    expect(screen.getByTestId("return-day-2026-10-27")).not.toHaveTextContent("US$0.00");
    expect(screen.getByTestId("return-day-2026-12-01")).toHaveTextContent("-US$7.00");
    expect(screen.getByTestId("return-day-2026-12-01")).toHaveTextContent("Dec 1");
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-10-26", to: "2026-12-06" }), "", "day");
    expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-11-05", to: "2026-11-20" }), "", "day");
    expect(screen.queryByText("Grid-only contributor")).not.toBeInTheDocument();
    expect(screen.getByText("Return amount").parentElement).toHaveTextContent("+US$1.00");
    await userEvent.setup().click(within(screen.getByTestId("return-day-2026-10-26")).getByRole("button"));
    await screen.findByRole("dialog");
    expect(returnDay).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-10-26", to: "2026-12-06" }), "2026-10-26");
    expect(useAnalysisStore.getState()).toMatchObject({ returnCursor: "2026-11", from: "2026-11-05", to: "2026-11-20" });
    expect(screen.getByText("November 2026")).toBeInTheDocument();
    await userEvent.setup().keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    within(screen.getByTestId("return-day-2026-12-01")).getByRole("button").focus();
    await userEvent.setup().keyboard("{Enter}");
    await waitFor(() => expect(returnDay).toHaveBeenCalledWith(expect.objectContaining({ from: "2026-10-26", to: "2026-12-06" }), "2026-12-01"));
    expect(useAnalysisStore.getState().returnCursor).toBe("2026-11");
  });

  it("requests cross-year grid bounds while keeping adjacent today and future dates muted", async () => {
    vi.setSystemTime(new Date("2026-01-01T12:00:00Z"));
    useAnalysisStore.getState().setReturnView({ cursor: "2025-12" });
    renderPage();
    await screen.findByText("December 2025");
    await waitFor(() => expect(returnCalendar).toHaveBeenCalledWith(expect.objectContaining({ from: "2025-12-01", to: "2025-12-31" }), "", "day"));
    expect(screen.getByTestId("return-day-2026-01-01")).toHaveTextContent("Today is not closed yet");
    expect(within(screen.getByTestId("return-day-2026-01-01")).getByRole("button")).toBeDisabled();
    expect(within(screen.getByTestId("return-day-2026-01-02")).getByRole("button")).toBeDisabled();
    expect(screen.getByTestId("return-day-2026-01-02")).toHaveTextContent("—");
  });

  it("opens the day sheet and carries the day and session into Asset Changes", async () => {
    const user = userEvent.setup();
    const onOpenAssetChanges = vi.fn();
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage(onOpenAssetChanges);

    await screen.findByText("Partial coverage");
    await user.click(within(screen.getByTestId("return-day-2026-09-01")).getByRole("button"));
    expect(await screen.findByRole("button", { name: "View asset changes for this day" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "View asset changes for this day" }));

    expect(onOpenAssetChanges).toHaveBeenCalledWith(expect.objectContaining({ scope: "portfolio", includeCash: true, from: "2026-09-01", to: "2026-09-01" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("shows six largest absolute day contributors and sums the remainder", async () => {
    const user = userEvent.setup();
    const amounts = [
      ["Gold CNY", "-328.5725"], ["BOXX", "259.851"], ["Gold USD", "-92.2521"],
      ["NVDA", "76.2728"], ["ES3", "7.279"], ["cash:SGD", "3.9335"], ["Silver", "-0.651"],
    ];
    returnDay.mockResolvedValue({
      date: "2026-09-01",
      returnAmount: { amount: "-74.1393", currency: "USD" },
      amountStatus: "complete",
      returnRate: "-0.0016135066203864",
      composition: [],
      contributors: amounts.map(([key, amount]) => ({ key, label: key, amount: { amount, currency: "USD" }, rate: null, ratedDays: 1, totalDays: 1 })),
      ratedDays: 1,
      totalDays: 1,
      issues: [],
      available: true,
      status: "ok",
      valuationForced: null,
    });
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();

    await screen.findByText("Partial coverage");
    await user.click(within(screen.getByTestId("return-day-2026-09-01")).getByRole("button"));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Gold CNY")).toBeInTheDocument();
    expect(within(dialog).getByText("BOXX")).toBeInTheDocument();
    expect(within(dialog).getByText("Gold USD")).toBeInTheDocument();
    expect(within(dialog).getByText("cash:SGD")).toBeInTheDocument();
    expect(within(dialog).queryByText("Silver")).not.toBeInTheDocument();
    expect(within(dialog).getByText("Other").parentElement).toHaveTextContent("-US$0.65");
    expect(within(dialog).getByText("Return amount").parentElement).toHaveTextContent("-US$74.14");
  });

  it("distinguishes a partial zero day from a complete zero day", async () => {
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();

    await screen.findByText("Partial coverage");
    expect(screen.getByTestId("return-day-2026-09-02")).toHaveTextContent("Rate days 0/1");
    expect(screen.getByTestId("return-day-2026-09-03")).not.toHaveTextContent("Rate days 0/1");
    expect(screen.getByTestId("return-day-2026-09-03")).not.toHaveClass("bg-success/8");
    expect(screen.getByTestId("return-day-2026-09-03")).not.toHaveClass("bg-destructive/7");
  });

  it("uses backend monthly summaries in the year view with one ReturnCalendar request", async () => {
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
    returnCalendar.mockResolvedValue({ available: false, status: "unavailable", cells: [], issues: [], topContributors: [], summary: { ratedDays: 0, totalDays: 0, beginningInvestedValue: null, endingInvestedValue: null, returnAmount: null, returnRate: null } });
    useAnalysisStore.getState().reset();
    renderPage();
    expect(await screen.findByText("No investment assets are available for this scope.")).toBeInTheDocument();
    expect(screen.queryByTestId("return-day-2026-09-01")).not.toBeInTheDocument();
    cleanup();

    returnCalendar.mockResolvedValue({ available: false, status: "unavailable", cells: [{ date: "2026-09-01", available: false, status: "unavailable", ratedDays: 0, totalDays: 1, returnAmount: null, returnRate: null, composition: [], contributors: [], issues: [], valuationForced: null }], issues: [], topContributors: [], summary: { ratedDays: 0, totalDays: 1, beginningInvestedValue: null, endingInvestedValue: null, returnAmount: null, returnRate: null } });
    useAnalysisStore.getState().setReturnView({ cursor: "2026-09" });
    renderPage();
    expect(await screen.findByText("Not enough historical data to calculate returns.")).toBeInTheDocument();
  });
});
