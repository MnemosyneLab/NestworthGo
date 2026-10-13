import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { HistoricalOverviewPage } from "./HistoricalOverviewPage";
import { NavigationContext } from "@/app/NavigationContext";
import i18n from "@/i18n";
import { shiftCivilDate } from "./dates";
import type { HistoricalOverviewSeriesResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { formatAmount } from "@/lib/money";
import { formatHistoricalAmount } from "./historicalAmount";
import type { HistoricalOverviewCell, HistoricalOverviewResult, HistoricalOverviewRow } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

// Page orchestration tests deliberately inject invalid/empty values. Real picker
// interaction is covered separately in HistoricalDatePicker.test.tsx.
vi.mock("@/components/ui/date-picker", async () => {
  const { forwardRef } = await import("react");
  return { DatePicker: forwardRef<HTMLInputElement, import("react").ComponentProps<typeof import("@/components/ui/date-picker").DatePicker>>(
    function PickerStub({ onChange, clearable: _clearable, allowFuture: _future, ...props }, ref) {
      return <input {...props} ref={ref as import("react").Ref<HTMLInputElement>} type="date" onChange={event => onChange(event.target.value)} />;
    }),
  };
});

const { read, origin, seriesRead } = vi.hoisted(() => ({ read: vi.fn(), origin: vi.fn(), seriesRead: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({ Service: { HistoricalOverview: read, HistoricalOverviewSeries: seriesRead } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: origin } }));

function cell(value: string | null = "100", patch: Partial<HistoricalOverviewCell> = {}): HistoricalOverviewCell {
  return { status: "active", included: true, complete: true, currency: "CNY", nativeAmount: value, baseAmount: value, quantity: null, assetClass: "cash", manual: false, valueSourceAt: "2026-08-01T12:00:00Z", valueSourceId: "origin", price: null, fx: null, missing: [], missingDetails: [], ...patch };
}

function result(date = "2026-08-01", compareTo = ""): HistoricalOverviewResult {
  const row = (key: string, parentKey: string, kind: string, name: string, changed: boolean): HistoricalOverviewRow => ({ key, accountId: "", holdingId: "", instrumentId: "", parentKey, kind, name, role: "asset", left: cell(), right: compareTo ? cell(changed ? "120" : "100") : null, baseChange: compareTo ? (changed ? "20" : "0") : null, nativeChange: compareTo ? (changed ? "20" : "0") : null, quantityChange: null, changed: Boolean(compareTo) && changed });
  const state = { date, cutoffAt: `${date}T23:59:59.999Z`, current: false, currency: "CNY", complete: true, assets: "200", liabilities: "0", netWorth: "200", knownAssets: "200", knownLiabilities: "0", byClass: [{ key: "cash", amount: "200", share: "100" }], byCurrency: [{ key: "CNY", amount: "200", share: "100" }] };
  const rows = [row("bank", "", "account", "Bank", true), row("cash", "bank", "cash", "CNY", true), row("steady", "", "account", "Unchanged", false)];
  return { categories: [], timezone: "UTC", originDate: "2026-08-01", lastClosedDate: "2026-08-09", capturedAt: "2026-08-10T12:00:00Z", inputGeneration: 1, resolverPolicy: "market-date-daily-summary-v3", left: state, right: compareTo ? { ...state, date: compareTo, current: compareTo === "current", cutoffAt: compareTo === "current" ? "2026-08-10T12:00:00Z" : `${compareTo}T23:59:59.999Z`, assets: "220", netWorth: "220", knownAssets: "220" } : null, change: compareTo ? { assets: "20", liabilities: "0", netWorth: "20" } : null, rows };
}

function seriesResult(from: string, to: string): HistoricalOverviewSeriesResult {
  const overview = result(from, to); overview.capturedAt = new Date().toISOString();
  const points: HistoricalOverviewSeriesResult["points"] = [];
  for (let date = from; date <= to; date = shiftCivilDate(date, 1)) {
    const state = date === to ? overview.right! : overview.left;
    points.push({ date, cutoffAt: `${date}T23:59:59Z`, available: true, gapReason: "", assets: state.assets, liabilities: state.liabilities, netWorth: state.netWorth, knownAssets: state.knownAssets, knownLiabilities: state.knownLiabilities, assetsComplete: true, liabilitiesComplete: true, assetCoverage: { included: 2, known: 2, missing: 0 }, liabilityCoverage: { included: 0, known: 0, missing: 0 }, missing: [], scopeChanged: false, basisChanged: false, connectAssets: date !== from, connectLiabilities: date !== from, connectNetWorth: date !== from });
  }
  return { overview, points, from, to, work: { leaves: 2, queries: 1, candidateFacts: 10, candidateBytes: 500 } };
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-10T12:00:00Z"));
  vi.clearAllMocks();
  void i18n.changeLanguage("en");
  origin.mockResolvedValue({ id: "origin", householdId: "household", startedAt: "2026-08-01T12:00:00Z", createdAt: "2026-08-01T12:00:00Z", timezone: "UTC" });
  read.mockImplementation((date: string, compare: string) => Promise.resolve(result(date, compare)));
  seriesRead.mockImplementation((from: string, to: string) => Promise.resolve(seriesResult(from, to)));
});
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

it.each(["en", "zh-CN", "zh-TW"].flatMap(locale => ["light", "dark"].map(theme => ({ locale, theme }))))(
  "keeps full monetary tokens accessible in single and comparison summaries ($locale, $theme)", async ({ locale, theme }) => {
    await i18n.changeLanguage(locale);
    document.documentElement.classList.toggle("dark", theme === "dark");
    read.mockImplementation((date: string, compare: string) => {
      const data = result(date, compare);
      data.left.assets = "1011800";
      if (data.right) data.right.assets = "1061940";
      if (data.change) data.change.assets = "50140";
      return Promise.resolve(data);
    });
    try {
      show();
      await screen.findByRole("button", { name: "Bank" });
      const checkAmount = (value: string, signed = false) => {
        const amount = screen.getByTitle(`${value} CNY`);
        expect(amount.textContent).toBe(`${signed ? "+" : ""}${formatAmount(value, "CNY")}`);
        // DOM guards against the original break-words/three-fixed-columns
        // contract. Native retest, not jsdom, establishes pixel-level fit.
        expect(amount).toHaveClass("whitespace-nowrap");
        const definition = amount.closest("dd")!;
        expect(definition).toHaveAttribute("tabindex", "0");
        expect(definition).toHaveClass("overflow-x-auto");
        expect(definition).not.toHaveClass("break-words", "truncate");
        expect(definition.closest("dl")).toHaveClass("grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))]");
      };
      checkAmount("1011800");
      await userEvent.selectOptions(screen.getByLabelText(i18n.t("historicalOverview.compare")), "current");
      await screen.findByRole("region", { name: i18n.t("historicalOverview.totalChange") });
      checkAmount("1011800");
      checkAmount("1061940");
      checkAmount("50140", true);
    } finally {
      document.documentElement.classList.remove("dark");
    }
  },
);

function show(primaryView = false) {
  const client = createTestQueryClient();
  const exit = vi.fn();
  const view = render(<QueryClientProvider client={client}><HistoricalOverviewPage onExit={exit} /></QueryClientProvider>);
  if (!primaryView) void screen.findByRole("tab", { name: i18n.t("historicalOverview.view.accounts") }, { timeout: 5000 }).then(button => {
    if (button.isConnected) fireEvent.click(button);
  }).catch(() => {});
  return { ...view, client, exit };
}

it.each(["en", "zh-CN", "zh-TW"].flatMap(locale => [false, true].map(compare => ({ locale, compare }))))(
  "provides one keyboard tab stop and named panels for historical views ($locale, compare=$compare)", async ({ locale, compare }) => {
    await i18n.changeLanguage(locale);
    const user = userEvent.setup();
    show(true);
    await screen.findByRole("tab", { name: i18n.t("historicalOverview.view.allocation") });
    if (compare) {
      await user.selectOptions(screen.getByLabelText(i18n.t("historicalOverview.compare")), "date");
      await screen.findByRole("tab", { name: i18n.t("historicalOverview.view.delta") });
    }
    const tablist = screen.getByRole("tablist", { name: i18n.t("historicalOverview.views") });
    const tabs = within(tablist).getAllByRole("tab");
    const reads = read.mock.calls.length;
    const checkSelection = (tab: HTMLElement) => {
      expect(tab).toHaveAttribute("aria-selected", "true");
      expect(tabs.filter(item => item.tabIndex === 0)).toEqual([tab]);
      const panel = screen.getByRole("tabpanel", { name: tab.textContent! });
      expect(panel).toHaveAttribute("aria-labelledby", tab.id);
      expect(tab).toHaveAttribute("aria-controls", panel.id);
    };
    checkSelection(tabs[0]);
    tabs[0].focus();
    await user.keyboard("{ArrowRight}");
    expect(tabs[1]).toHaveFocus();
    checkSelection(tabs[1]);
    await user.keyboard("{End}");
    expect(tabs[tabs.length - 1]).toHaveFocus();
    checkSelection(tabs[tabs.length - 1]);
    await user.keyboard("{Home}");
    expect(tabs[0]).toHaveFocus();
    checkSelection(tabs[0]);
    await user.tab();
    expect(screen.getByRole("tabpanel", { name: tabs[0].textContent! })).toHaveFocus();
    await user.tab({ shift: true });
    expect(tabs[0]).toHaveFocus();
    await user.keyboard("{ArrowLeft}");
    expect(tabs[tabs.length - 1]).toHaveFocus();
    checkSelection(tabs[tabs.length - 1]);
    expect(read).toHaveBeenCalledTimes(reads);
    if (compare) {
      await user.selectOptions(screen.getByLabelText(i18n.t("historicalOverview.compare")), "none");
      const allocation = await screen.findByRole("tab", { name: i18n.t("historicalOverview.view.allocation") });
      expect(screen.queryByRole("tab", { name: i18n.t("historicalOverview.view.delta") })).not.toBeInTheDocument();
      expect(within(screen.getByRole("tablist")).getAllByRole("tab").filter(tab => tab.tabIndex === 0)).toEqual([allocation]);
      expect(screen.getByRole("tabpanel", { name: allocation.textContent! })).toBeInTheDocument();
    }
  },
);

it("opens a closed day, expands using keyboard and repeatedly dismisses read-only details", async () => {
  const user = userEvent.setup();
  const { exit } = show();
  await screen.findByRole("button", { name: "Bank" });
  expect(read).toHaveBeenLastCalledWith("2026-08-01", "");
  expect(screen.getByLabelText("Historical date")).toHaveFocus();
  expect(screen.getByLabelText("Historical date")).toHaveAttribute("min", "2026-08-01");
  expect(screen.getByLabelText("Historical date")).toHaveAttribute("max", "2026-08-09");
  const expand = screen.getByRole("button", { name: "Expand Bank" });
  expand.focus();
  await user.keyboard("{Enter}");
  expect(expand).toHaveAttribute("aria-expanded", "true");
  await user.click(screen.getByRole("button", { name: "CNY cash" }));
  expect(await screen.findByRole("dialog")).toHaveTextContent("2026-08-01");
  expect(screen.queryByRole("button", { name: /edit|save|record change/i })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "Bank" }));
  await screen.findByRole("dialog");
  await user.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(read).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole("button", { name: "Back to current" }));
  expect(exit).toHaveBeenCalledOnce();
});

it("compares aligned rows, filters changes and freezes current until an explicit refresh", async () => {
  const user = userEvent.setup();
  const { client } = show();
  await screen.findByRole("button", { name: "Bank" });
  await user.selectOptions(screen.getByLabelText("Compare with"), "current");
  await waitFor(() => expect(read).toHaveBeenLastCalledWith("2026-08-01", "current"));
  await screen.findByText("Captured current state, not today’s close");
  await user.click(screen.getByLabelText("Only changed rows"));
  expect(screen.queryByRole("button", { name: "Unchanged" })).not.toBeInTheDocument();
  const reads = read.mock.calls.length;
  await act(async () => { await client.invalidateQueries(); });
  expect(read).toHaveBeenCalledTimes(reads);
  await user.click(screen.getByRole("button", { name: "Refresh both dates" }));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(reads + 1));
});

it("ignores late responses after changing date and clears an open detail on comparison changes", async () => {
  const user = userEvent.setup();
  let resolveOld!: (data: HistoricalOverviewResult) => void;
  read.mockImplementation((date: string, compare: string) => date === "2026-08-02" ? new Promise(resolve => { resolveOld = resolve; }) : Promise.resolve(result(date, compare)));
  show();
  await screen.findByRole("button", { name: "Bank" });
  fireEvent.change(screen.getByLabelText("Historical date"), { target: { value: "2026-08-02" } });
  await waitFor(() => expect(read).toHaveBeenLastCalledWith("2026-08-02", ""));
  fireEvent.change(screen.getByLabelText("Historical date"), { target: { value: "2026-08-03" } });
  await screen.findByRole("columnheader", { name: "2026-08-03" });
  await act(async () => resolveOld(result("2026-08-02")));
  expect(screen.queryByRole("columnheader", { name: "2026-08-02" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Bank" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByRole("heading", { name: "2026-08-03" })).toBeInTheDocument();
  await user.keyboard("{Escape}");
  await user.selectOptions(screen.getByLabelText("Compare with"), "date");
  fireEvent.change(screen.getByLabelText("Comparison date"), { target: { value: "2026-08-05" } });
  await waitFor(() => expect(read).toHaveBeenLastCalledWith("2026-08-03", "2026-08-05"));
  await screen.findByRole("columnheader", { name: "2026-08-05" });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("shows unknown, zero, absent, archived and manual evidence without fetching current details", async () => {
  const user = userEvent.setup();
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    data.left.complete = false; data.left.netWorth = null;
    data.rows![0].left = cell("0", { status: "archived", included: false, manual: true, complete: false, baseAmount: null, missing: ["fx_rate"], price: { sourceKey: "", value: null, currency: "", baseCurrency: "", quoteCurrency: "", operation: "", observationKind: "", priceBasis: "", timestampBasis: "", sourcePolicyVersion: "", id: "price", source: "manual", effectiveAt: "2026-07-20T00:00:00Z", marketDate: "", freshness: "manual" } });
    data.rows![0].right = compare ? cell("0", { status: "zero" }) : null;
    data.rows![2].left = null;
    return Promise.resolve(data);
  });
  show();
  await screen.findByRole("button", { name: "Bank" });
  expect(screen.getAllByText("Not yet present in retained history").length).toBeGreaterThan(0);
  expect(screen.getAllByText("Unknown").length).toBeGreaterThan(0);
  await user.click(screen.getByRole("button", { name: "Bank" }));
  const detail = await screen.findByRole("dialog");
  expect(detail).toHaveTextContent("Exchange rate is missing");
  expect(within(detail).getByTitle("2026-07-20T00:00:00Z")).toHaveAttribute("datetime", "2026-07-20T00:00:00Z");
  expect(detail).toHaveTextContent("Archived");
  expect(read).toHaveBeenCalledTimes(1);
});

it("handles absent history and first open day without requesting a snapshot", async () => {
  origin.mockResolvedValueOnce(null);
  const first = show();
  await screen.findByText(i18n.t("insights.noOrigin"));
  expect(read).not.toHaveBeenCalled();
  first.unmount();
  origin.mockResolvedValueOnce({ startedAt: "2026-08-10T00:00:00Z", timezone: "UTC" });
  show();
  await screen.findByText("No closed history day yet");
  expect(read).not.toHaveBeenCalled();
});

it("recovers from errors, rejects future dates and localizes the read-only labels", async () => {
  const user = userEvent.setup();
  read.mockRejectedValueOnce(new Error("read failed"));
  show();
  await screen.findByText("Historical overview could not be loaded");
  await user.click(screen.getByRole("button", { name: i18n.t("common.retryAction") }));
  await screen.findByRole("button", { name: "Bank" });
  const count = read.mock.calls.length;
  fireEvent.change(screen.getByLabelText("Historical date"), { target: { value: "2026-08-10" } });
  await screen.findByText("Choose a recorded, closed day");
  expect(read).toHaveBeenCalledTimes(count);
  await act(async () => { await i18n.changeLanguage("zh-CN"); });
  expect(screen.getByRole("heading", { name: "历史全景" })).toBeInTheDocument();
  expect(screen.getByText("只读")).toBeInTheDocument();
  await act(async () => { await i18n.changeLanguage("zh-TW"); });
  expect(screen.getByText("唯讀")).toBeInTheDocument();
});


it("preserves sub-cent decimal strings in read-only detail", async () => {
  const snapshot = result();
  snapshot.rows![0].left = cell("0.00012");
  read.mockResolvedValue(snapshot);
  show();
  await userEvent.click(await screen.findByRole("button", { name: "Bank" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/Exact base amount/)).toHaveTextContent("0.00012 CNY");
  expect(within(dialog).getByText(new RegExp(i18n.t("historicalOverview.original")))).toHaveTextContent("0.00012 CNY");
});

it("keeps a cleared date invalid across a month boundary instead of selecting a default", async () => {
  const { client, rerender } = show();
  await screen.findByRole("button", { name: "Bank" });
  const calls = read.mock.calls.length;
  fireEvent.change(screen.getByLabelText("Historical date"), { target: { value: "" } });
  expect(screen.getByLabelText("Historical date")).toHaveValue("");
  expect(screen.getByLabelText("Historical date")).toHaveAttribute("aria-invalid", "true");
  expect(screen.queryByRole("button", { name: "Bank" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Previous day" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Next day" })).toBeDisabled();
  vi.setSystemTime(new Date("2026-09-01T00:00:30Z"));
  rerender(<QueryClientProvider client={client}><HistoricalOverviewPage isActive={false} onExit={vi.fn()} /></QueryClientProvider>);
  rerender(<QueryClientProvider client={client}><HistoricalOverviewPage isActive onExit={vi.fn()} /></QueryClientProvider>);
  expect(screen.getByLabelText("Historical date")).toHaveValue("");
  expect(screen.getByLabelText("Historical date")).toHaveAttribute("aria-invalid", "true");
  expect(read).toHaveBeenCalledTimes(calls);
});

it("keeps an explicit empty initial date invalid after the origin loads", async () => {
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><HistoricalOverviewPage initialDate="" onExit={vi.fn()} /></QueryClientProvider>);
  const input = await screen.findByLabelText("Historical date");
  expect(input).toHaveValue("");
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(read).not.toHaveBeenCalled();
});

it("keeps an empty comparison date invalid and does not drop comparison silently", async () => {
  show();
  await screen.findByRole("button", { name: "Bank" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "date");
  await screen.findByRole("columnheader", { name: "2026-08-09" });
  const calls = read.mock.calls.length;
  fireEvent.change(screen.getByLabelText("Comparison date"), { target: { value: "" } });
  expect(screen.getByLabelText("Comparison date")).toHaveValue("");
  expect(screen.getByRole("button", { name: "Refresh both dates" })).toBeDisabled();
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  expect(read).toHaveBeenCalledTimes(calls);
});

it("keeps the parent account in a cash detail and restores keyboard focus after closing", async () => {
  show();
  await userEvent.click(await screen.findByRole("button", { name: "Expand Bank" }));
  const trigger = screen.getByRole("button", { name: "CNY cash" });
  trigger.focus();
  await userEvent.keyboard("{Enter}");
  const detail = await screen.findByRole("dialog");
  expect(detail).toHaveTextContent("Bank");
  expect(within(detail).getByRole("heading", { name: "CNY cash" })).toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(trigger).toHaveFocus());
});

it("recovers from invalid input using bounded date shortcuts", async () => {
  show();
  await screen.findByRole("button", { name: "Bank" });
  await userEvent.click(screen.getByRole("button", { name: "Last closed day" }));
  await screen.findByRole("columnheader", { name: "2026-08-09" });
  expect(screen.getByRole("button", { name: "Next day" })).toBeDisabled();
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "date");
  expect(screen.getByLabelText("Comparison date")).toHaveValue("2026-08-09");
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "none");
  await userEvent.click(screen.getByRole("button", { name: "Starting point" }));
  await screen.findByRole("columnheader", { name: "2026-08-01" });
  expect(screen.getByRole("button", { name: "Previous day" })).toBeDisabled();
  fireEvent.change(screen.getByLabelText("Historical date"), { target: { value: "" } });
  expect(screen.getByRole("button", { name: "Last month end" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Starting point" }));
  expect(screen.getByLabelText("Historical date")).toHaveValue("2026-08-01");
  expect(screen.getByLabelText("Historical date")).toHaveAttribute("aria-invalid", "false");
  await screen.findByRole("button", { name: "Bank" });
  expect(screen.getByRole("region", { name: "Scrollable historical comparison" })).toHaveAttribute("tabindex", "0");
});

it("selects the actual previous month end when history covers that date", async () => {
  origin.mockResolvedValue({ startedAt: "2026-06-01T00:00:00Z", timezone: "UTC" });
  show();
  await screen.findByRole("columnheader", { name: "2026-07-31" });
  await userEvent.click(screen.getByRole("button", { name: "Last closed day" }));
  await screen.findByRole("columnheader", { name: "2026-08-09" });
  await userEvent.click(screen.getByRole("button", { name: "Last month end" }));
  await screen.findByRole("columnheader", { name: "2026-07-31" });
  expect(read).toHaveBeenLastCalledWith("2026-07-31", "");
});

it("shows authoritative total deltas and keeps unknown distinct from zero", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    if (compare) data.change = { assets: null, liabilities: "0", netWorth: null };
    return Promise.resolve(data);
  });
  show();
  await screen.findByRole("button", { name: "Bank" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "current");
  const change = await screen.findByRole("region", { name: "Total change (right minus left)" });
  expect(within(change).getAllByText("Unknown")).toHaveLength(2);
  expect(within(change).getAllByTitle("0 CNY").length).toBeGreaterThan(0);
  expect(change).toHaveTextContent("Balance changes are not investment returns");
});

it("shows known native changes when FX is missing and never compares absence as zero", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    const foreign = data.rows![0];
    foreign.left = cell("10", { currency: "USD", complete: false, baseAmount: null, missing: ["fx_rate"] });
    foreign.right = compare ? cell("12", { currency: "USD", complete: false, baseAmount: null, missing: ["fx_rate"] }) : null;
    foreign.nativeChange = compare ? "2" : null;
    foreign.baseChange = null;
    data.rows![2].left = null;
    data.rows![2].baseChange = null;
    data.rows![2].nativeChange = null;
    return Promise.resolve(data);
  });
  show();
  await screen.findByRole("button", { name: "Bank" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "current");
  const table = await screen.findByRole("table", { name: "Historical account and holding comparison" });
  expect(within(table).getByTitle("2 USD")).toHaveTextContent("+");
  expect(within(table).getByText("Not comparable")).toBeInTheDocument();
  await userEvent.click(within(table).getByRole("button", { name: "Bank" }));
  const detail = await screen.findByRole("dialog");
  expect(within(detail).getByText("+2 USD")).toBeInTheDocument();
  expect(within(detail).getAllByText("Exchange rate is missing")).toHaveLength(2);
});

it("deduplicates repeated refresh clicks and replaces both captured sides together", async () => {
  show();
  await screen.findByRole("button", { name: "Bank" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "current");
  await screen.findByText("Captured current state, not today’s close");
  let resolveRefresh!: (data: HistoricalOverviewResult) => void;
  read.mockImplementationOnce(() => new Promise(resolve => { resolveRefresh = resolve; }));
  const before = read.mock.calls.length;
  await userEvent.dblClick(screen.getByRole("button", { name: "Refresh both dates" }));
  expect(read).toHaveBeenCalledTimes(before + 1);
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Refresh both dates" })).toBeDisabled();
  const updated = result("2026-08-01", "current");
  updated.capturedAt = "2026-08-10T12:30:00Z";
  updated.right!.cutoffAt = updated.capturedAt;
  await act(async () => resolveRefresh(updated));
  await screen.findByRole("table", { name: "Historical account and holding comparison" });
  expect(screen.getByTitle("2026-08-10T12:30:00Z")).toBeInTheDocument();
  expect(screen.queryByTitle("2026-08-10T12:00:00Z")).not.toBeInTheDocument();
});

it("hides a failed refresh's old totals and permits an explicit retry", async () => {
  show();
  await screen.findByRole("button", { name: "Bank" });
  read.mockRejectedValueOnce(new Error("refresh failed"));
  await userEvent.click(screen.getByRole("button", { name: "Refresh date" }));
  await screen.findByRole("alert");
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Bank" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: i18n.t("common.retryAction") }));
  await screen.findByRole("button", { name: "Bank" });
});

it("preserves changed children when their account's movements offset", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    if (compare) {
      data.change = { assets: "0", liabilities: "0", netWorth: "0" };
      data.rows![0].baseChange = "0";
      data.rows![0].nativeChange = "0";
      data.rows!.push({ ...data.rows![1], key: "fund", kind: "holding", name: "Fund", baseChange: "-20", nativeChange: "-20" });
    }
    return Promise.resolve(data);
  });
  show();
  await screen.findByRole("button", { name: "Bank" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "current");
  await userEvent.click(await screen.findByRole("checkbox", { name: "Only changed rows" }));
  await userEvent.click(screen.getByRole("button", { name: "Expand Bank" }));
  expect(screen.getByRole("button", { name: "CNY cash" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Fund" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Unchanged" })).not.toBeInTheDocument();
  const collapse = screen.getByRole("button", { name: "Collapse Bank" });
  await userEvent.click(collapse);
  expect(screen.queryByRole("button", { name: "Fund" })).not.toBeInTheDocument();
});

it("explains same-date comparisons and recovers from an empty changes-only view", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    if (date === compare) {
      data.rows!.forEach(row => { row.changed = false; });
      data.change = { assets: "0", liabilities: "0", netWorth: "0" };
    }
    return Promise.resolve(data);
  });
  show();
  await screen.findByRole("button", { name: "Bank" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "date");
  fireEvent.change(screen.getByLabelText("Comparison date"), { target: { value: "2026-08-01" } });
  await screen.findByText("Both sides show the same closed day. Choose another date to see changes.");
  await userEvent.click(screen.getByRole("checkbox", { name: "Only changed rows" }));
  await screen.findByText("No changed rows");
  await userEvent.click(screen.getByRole("checkbox", { name: "Only changed rows" }));
  expect(screen.getByRole("button", { name: "Bank" })).toBeInTheDocument();
});

it("retries a history-origin failure without issuing a premature snapshot read", async () => {
  origin.mockRejectedValueOnce(new Error("origin unavailable")).mockRejectedValueOnce(new Error("origin unavailable"));
  show();
  await screen.findByRole("alert", {}, { timeout: 3000 });
  expect(read).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: i18n.t("common.retryAction") }));
  await screen.findByRole("button", { name: "Bank" });
  expect(read).toHaveBeenCalledOnce();
});


it("ignores a late read after exit and reopening with the same query client", async () => {
  let resolveOld!: (data: HistoricalOverviewResult) => void;
  read.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve; }));
  const first = show();
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  first.unmount();
  const fresh = result();
  fresh.rows![0].name = "Fresh capture";
  read.mockResolvedValue(fresh);
  render(<QueryClientProvider client={first.client}><HistoricalOverviewPage onExit={vi.fn()} /></QueryClientProvider>);
  await screen.findByRole("button", { name: "Fresh capture" });
  await act(async () => resolveOld(result()));
  expect(screen.queryByRole("button", { name: "Bank" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Fresh capture" })).toBeInTheDocument();
  expect(read).toHaveBeenCalledTimes(2);
});

it.each(["inactive", "invalid", "unmount"])("cancels a pending comparison when the page becomes %s and ignores late replies", async action => {
  let resolveOld!: (data: HistoricalOverviewResult) => void;
  const cancel = vi.fn();
  const pending = Object.assign(new Promise<HistoricalOverviewResult>(done => { resolveOld = done; }), { cancel });
  read.mockReturnValueOnce(pending);
  const client = createTestQueryClient();
  const view = render(<QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-01" initialContext={{ compareTo: "current", view: "accounts" }} onExit={vi.fn()} /></QueryClientProvider>);
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  if (action === "inactive") view.rerender(<QueryClientProvider client={client}><HistoricalOverviewPage isActive={false} onExit={vi.fn()} /></QueryClientProvider>);
  else if (action === "invalid") fireEvent.change(screen.getByLabelText(i18n.t("historicalOverview.baselineDate")), { target: { value: "" } });
  else view.unmount();
  await waitFor(() => expect(cancel).toHaveBeenCalledOnce());
  await act(async () => resolveOld(result("2026-08-01", "current")));
  expect(screen.queryByRole("button", { name: "Bank" })).not.toBeInTheDocument();
  expect(read).toHaveBeenCalledOnce();
  if (action === "inactive") {
    view.rerender(<QueryClientProvider client={client}><HistoricalOverviewPage isActive onExit={vi.fn()} /></QueryClientProvider>);
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
    await screen.findByRole("button", { name: "Bank" });
  }
});

it.each(["resolve", "reject"])("resumes an unfinished capture after History Back and ignores its late %s", async lateReply => {
  let resolveOld!: (data: HistoricalOverviewResult) => void;
  let rejectOld!: (error: Error) => void;
  const cancel = vi.fn();
  const pending = Object.assign(new Promise<HistoricalOverviewResult>((resolve, reject) => {
    resolveOld = resolve; rejectOld = reject;
  }), { cancel });
  const fresh = result("2026-08-03", "current");
  fresh.rows![0].name = "Resumed capture";
  read.mockReturnValueOnce(pending).mockResolvedValue(fresh);
  const client = createTestQueryClient();
  function HistoryBackHarness() {
    const [history, setHistory] = useState(false);
    const navigation = { open: () => setHistory(true), openHealth: vi.fn() };
    return <QueryClientProvider client={client}><NavigationContext.Provider value={navigation}>
      <div hidden={history}>
        {/* A pending page hides its History links. The harness supplies a
            navigation race while retaining the page exactly as App does. */}
        <button onClick={() => navigation.open()}>Navigate to synthetic History</button>
        <HistoricalOverviewPage initialDate="2026-08-03" initialContext={{ compareTo: "current", view: "accounts" }} isActive={!history} onExit={vi.fn()} />
      </div>
      {history && <section aria-label="Synthetic History"><button onClick={() => setHistory(false)}>Back to panorama</button></section>}
    </NavigationContext.Provider></QueryClientProvider>;
  }
  render(<HistoryBackHarness />);
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  expect(screen.getByRole("status", { name: i18n.t("historicalOverview.loading") })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Navigate to synthetic History" }));
  await screen.findByRole("region", { name: "Synthetic History" });
  await waitFor(() => expect(cancel).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Back to panorama" }));
  await screen.findByRole("button", { name: "Resumed capture" });
  expect(read).toHaveBeenCalledTimes(2);
  expect(read).toHaveBeenLastCalledWith("2026-08-03", "current");
  await act(async () => {
    if (lateReply === "resolve") resolveOld(result("2026-08-03", "current"));
    else rejectOld(new Error("Synthetic late abandoned read failure"));
  });
  expect(screen.getByRole("button", { name: "Resumed capture" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Bank" })).not.toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.getByLabelText(i18n.t("historicalOverview.baselineDate"))).toHaveValue("2026-08-03");
  expect(screen.getByLabelText(i18n.t("historicalOverview.compare"))).toHaveValue("current");
  expect(screen.getByRole("tab", { name: i18n.t("historicalOverview.view.accounts") })).toHaveAttribute("aria-selected", "true");
  expect(cancel).toHaveBeenCalledOnce();
  expect(read).toHaveBeenCalledTimes(2);
});

it("keeps account/category scope changes local to the successful final capture after rapid dates", async () => {
  let resolveOld!: (data: HistoricalOverviewResult) => void;
  const cancel = vi.fn();
  read.mockReturnValueOnce(Object.assign(new Promise<HistoricalOverviewResult>(done => { resolveOld = done; }), { cancel }));
  show(true);
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByLabelText("Historical date"), { target: { value: "2026-08-02" } });
  await screen.findByRole("region", { name: "Asset allocation" });
  await waitFor(() => expect(cancel).toHaveBeenCalledOnce());
  await act(async () => resolveOld(result()));
  await userEvent.click(document.getElementById("historical-category-asset-byClass-cash")!);
  await userEvent.click(await screen.findByRole("button", { name: "Bank" }));
  expect(await screen.findByRole("dialog")).toHaveTextContent("2026-08-02");
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("button", { name: "Clear category filter" }));
  await userEvent.click(screen.getByRole("button", { name: "Unchanged" }));
  expect(await screen.findByRole("dialog")).toHaveTextContent("2026-08-02");
  expect(read).toHaveBeenCalledTimes(2);
});

it("shows source time in the household zone and retains the exact UTC instant", async () => {
  origin.mockResolvedValue({ id: "origin", startedAt: "2026-08-01T12:00:00Z", timezone: "Asia/Singapore" });
  const data = result();
  data.timezone = "Asia/Singapore";
  data.rows![0].left = cell("100", { manual: true, valueSourceAt: "2026-08-01T12:00:00Z" });
  read.mockResolvedValue(data);
  show();
  await userEvent.click(await screen.findByRole("button", { name: "Bank" }));
  const detail = await screen.findByRole("dialog");
  expect(detail).toHaveTextContent("Asia/Singapore");
  const source = within(detail).getByTitle("2026-08-01T12:00:00Z");
  expect(source).toHaveAttribute("datetime", "2026-08-01T12:00:00Z");
  expect(source).toHaveTextContent("08:00:00 PM");
  expect(detail).toHaveTextContent("This is not a new appraisal");
});


it("does not claim a manual value was carried forward when no value is known", async () => {
  const data = result();
  data.rows![0].left = cell("0", { status: "unknown", manual: true, complete: false, nativeAmount: null, baseAmount: null, valueSourceAt: "", missing: ["account_value"] });
  read.mockResolvedValue(data);
  show();
  await userEvent.click(await screen.findByRole("button", { name: "Bank" }));
  const detail = await screen.findByRole("dialog");
  expect(detail).toHaveTextContent("Recorded account value is missing");
  expect(detail).not.toHaveTextContent("Last retained manual value carried forward");
});

it("keeps the explicit A/B direction through reverse dates and swap", async () => {
  show(true);
  await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.click(screen.getByRole("button", { name: "Compare dates" }));
  expect(screen.getByLabelText("Comparison date")).toHaveValue("2026-08-09");
  fireEvent.change(screen.getByLabelText("A baseline"), { target: { value: "2026-08-08" } });
  await waitFor(() => expect(read).toHaveBeenLastCalledWith("2026-08-08", "2026-08-09"));
  await userEvent.click(screen.getByRole("button", { name: "Swap A and B" }));
  await waitFor(() => expect(read).toHaveBeenLastCalledWith("2026-08-09", "2026-08-08"));
  expect(screen.getByText("Reverse comparison: the change is still B − A.")).toBeInTheDocument();
});

it("links an asset category to the all-account view without changing the metric cards", async () => {
  show(true);
  await screen.findByRole("region", { name: "Asset allocation" });
  const before = read.mock.calls.length;
  const allocation = screen.getByRole("region", { name: "Asset allocation" });
  await userEvent.click(within(allocation).getByRole("button", { name: "Original currency" }));
  await userEvent.click(within(allocation).getByRole("button", { name: "CNY" }));
  expect(screen.getByText("Category: CNY")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Clear category filter" })).toHaveFocus();
  expect(screen.getAllByTitle("200 CNY").length).toBeGreaterThan(0);
  expect(read).toHaveBeenCalledTimes(before);
  await userEvent.click(screen.getByRole("button", { name: "Clear category filter" }));
  expect(screen.getByRole("tab", { name: "Account details" })).toHaveFocus();
  expect(screen.getByRole("button", { name: "Unchanged" })).toBeInTheDocument();
});

it("shows debt as a separate balance group and expands more than ten comparable accounts", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    if (compare) {
      data.rows = [...data.rows!, ...Array.from({ length: 11 }, (_, index): HistoricalOverviewRow => ({
        key: `extra-${index}`, accountId: "", holdingId: "", instrumentId: "", parentKey: "", kind: "account", name: `Extra ${index}`, role: "asset",
        left: cell("100"), right: cell("101"), baseChange: "1", nativeChange: "1", quantityChange: null, changed: true,
      })), {
        key: "loan", accountId: "", holdingId: "", instrumentId: "", parentKey: "", kind: "account", name: "Loan", role: "liability",
        left: cell("200"), right: cell("100"), baseChange: "-100", nativeChange: "-100", quantityChange: null, changed: true,
      }];
    }
    return Promise.resolve(data);
  });
  show(true);
  await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "date");
  const overview = await screen.findByRole("region", { name: "Account balance changes" });
  expect(within(overview).getByText("Showing 10 of 12 comparable accounts")).toBeInTheDocument();
  await userEvent.click(within(overview).getByRole("button", { name: "Expand all comparable accounts" }));
  expect(within(overview).getByText("Showing 12 of 12 comparable accounts")).toBeInTheDocument();
  await userEvent.click(within(overview).getByRole("button", { name: "Liabilities" }));
  expect(within(overview).getByRole("button", { name: "Loan" })).toBeInTheDocument();
  expect(overview).toHaveTextContent("Debt bars are balance changes, not net-worth contributions");
});

it("does not draw a shared amount axis for inconsistent base currencies", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    if (data.right) data.right.currency = "USD";
    return Promise.resolve(data);
  });
  show(true);
  await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "date");
  const overview = await screen.findByRole("region", { name: "Account balance changes" });
  expect(within(overview).queryByRole("img")).not.toBeInTheDocument();
  expect(overview).toHaveTextContent("Base currencies differ");
  expect(screen.getByRole("region", { name: "Total change (right minus left)" })).toHaveTextContent("Base currencies differ");
});

it("does not show a phantom B valuation warning for a complete single day", async () => {
  show(true);
  const allocation = await screen.findByRole("region", { name: "Asset allocation" });
  expect(within(allocation).getByRole("img")).toBeInTheDocument();
  expect(allocation).not.toHaveTextContent("A shows valued assets only");
  expect(allocation).not.toHaveTextContent("B shows valued assets only");
});

it("keeps a single-day asset composition complete when only debt valuation is missing", async () => {
  const data = result();
  data.left = { ...data.left, complete: false, liabilities: null, netWorth: null };
  data.rows!.push({
    key: "loan", accountId: "", holdingId: "", instrumentId: "", parentKey: "", kind: "account", name: "Loan", role: "liability",
    left: cell(null, { complete: false, missing: ["account_value"] }), right: null,
    baseChange: null, nativeChange: null, quantityChange: null, changed: false,
  }, {
    key: "loan:balance", accountId: "", holdingId: "", instrumentId: "", parentKey: "loan", kind: "balance", name: "Loan", role: "liability",
    left: cell(null, { complete: false, missing: ["account_value"] }), right: null,
    baseChange: null, nativeChange: null, quantityChange: null, changed: false,
  });
  read.mockResolvedValue(data);
  show(true);
  const allocation = await screen.findByRole("region", { name: "Asset allocation" });
  expect(within(allocation).getByRole("img")).toBeInTheDocument();
  expect(allocation).not.toHaveTextContent("A shows valued assets only");
  expect(allocation).not.toHaveTextContent("B shows valued assets only");
  expect(screen.getByRole("region", { name: "Total change (right minus left)" })).toHaveTextContent("Unknown");
});

it("shows the B valuation warning only when a real B side has missing assets", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    if (data.right) {
      data.right = { ...data.right, complete: false, assets: null, netWorth: null };
      data.rows!.push({
        key: "missing-stock", accountId: "", holdingId: "", instrumentId: "", parentKey: "bank", kind: "holding", name: "Missing stock", role: "asset",
        left: cell("5"), right: cell(null, { complete: false, missing: ["fx_rate"] }),
        baseChange: null, nativeChange: null, quantityChange: null, changed: true,
      });
    }
    return Promise.resolve(data);
  });
  show(true);
  await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "date");
  await userEvent.click(await screen.findByRole("tab", { name: "Category comparison" }));
  const comparison = await screen.findByRole("region", { name: "Category comparison" });
  expect(comparison).toHaveTextContent("B shows valued assets only; 1 included asset item(s) missing valuation.");
  expect(comparison).not.toHaveTextContent("A shows valued assets only");
});

it("keeps zero-to-active assets and active-to-zero debt in their comparable balance charts", async () => {
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    if (compare) {
      data.rows![0].left = cell("0", { status: "zero" });
      data.rows![0].right = cell("100", { status: "active" });
      data.rows![0].baseChange = "100";
      data.rows!.push({ key: "loan", accountId: "", holdingId: "", instrumentId: "", parentKey: "", kind: "account", name: "Loan", role: "liability",
        left: cell("100", { status: "active" }), right: cell("0", { status: "zero" }),
        baseChange: "-100", nativeChange: "-100", quantityChange: null, changed: true });
    }
    return Promise.resolve(data);
  });
  show(true);
  await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.selectOptions(screen.getByLabelText("Compare with"), "date");
  const changes = await screen.findByRole("region", { name: "Account balance changes" });
  expect(within(changes).getByText("Showing 1 of 1 comparable accounts")).toBeInTheDocument();
  expect(within(changes).getByRole("button", { name: "Bank" })).toBeInTheDocument();
  await userEvent.click(within(changes).getByRole("button", { name: "Liabilities" }));
  expect(within(changes).getByText("Showing 1 of 1 comparable accounts")).toBeInTheDocument();
  expect(within(changes).getByRole("button", { name: "Loan" })).toBeInTheDocument();
});

it.each(["en", "zh-CN", "zh-TW"])("keeps negative sub-cent values exact in cards, cells and account changes (%s)", async locale => {
  await i18n.changeLanguage(locale);
  read.mockImplementation((date: string, compare: string) => {
    const data = result(date, compare);
    data.left = { ...data.left, assets: "-0.00012", netWorth: "-0.00012", knownAssets: "-0.00012", byClass: [{ key: "cash", amount: "-0.00012", share: "-100" }] };
    data.rows![0].left = cell("-0.00012");
    if (data.right && data.change) {
      data.right = { ...data.right, assets: "-0.00024", netWorth: "-0.00024", knownAssets: "-0.00024" };
      data.change = { ...data.change, assets: "-0.00012", netWorth: "-0.00012" };
      data.rows![0].right = cell("-0.00024");
      data.rows![0].baseChange = "-0.00012";
    }
    return Promise.resolve(data);
  });
  show(true);
  await screen.findByRole("region", { name: i18n.t("historicalOverview.allocation") });
  const metrics = screen.getByRole("region", { name: i18n.t("historicalOverview.totalChange") });
  expect(within(metrics).getAllByTitle("-0.00012 CNY")[0]).toHaveTextContent("-0.00012 CNY");
  await userEvent.click(screen.getByRole("tab", { name: i18n.t("historicalOverview.view.accounts") }));
  const accounts = screen.getByRole("region", { name: i18n.t("historicalOverview.table") });
  expect(within(accounts).getAllByTitle("-0.00012 CNY")[0]).toHaveTextContent("-0.00012 CNY");
  await userEvent.selectOptions(screen.getByLabelText(i18n.t("historicalOverview.compare")), "date");
  await userEvent.click(await screen.findByRole("tab", { name: i18n.t("historicalOverview.view.delta") }));
  const changes = await screen.findByRole("region", { name: i18n.t("historicalOverview.accountDeltaTitle") });
  expect(changes).toHaveTextContent("-0.00012 CNY");
});


it.each(["en", "zh-CN", "zh-TW"])("restores category role, dimension, position and keyboard focus after drilling into account leaves (%s)", async locale => {
  await i18n.changeLanguage(locale);
  const data = result("2026-08-01", "2026-08-09");
  data.categories = [{ role: "asset", dimension: "byCurrency", key: "CNY", left: { status: "complete", amount: "200", knownAmount: "200", share: "100", shareBasis: "total", includedCount: 1, missingRowKeys: [] }, right: { status: "complete", amount: "220", knownAmount: "220", share: "100", shareBasis: "total", includedCount: 1, missingRowKeys: [] }, amountChange: "20", shareChangePp: "0", amountReasons: [], shareReasons: [] }];
  read.mockResolvedValue(data);
  const client = createTestQueryClient();
  render(<div id="main-content"><QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-01" initialContext={{ compareTo: "2026-08-09", view: "allocation", dimension: "byCurrency" }} onExit={vi.fn()} /></QueryClientProvider></div>);
  const category = await screen.findByRole("button", { name: "CNY" });
  const main = document.getElementById("main-content")!; main.scrollTop = 180;
  category.focus(); await userEvent.keyboard("{Enter}");
  expect(await screen.findByRole("button", { name: i18n.t("historicalOverview.clearFilter") })).toHaveFocus();
  expect(screen.getByRole("tabpanel", { name: i18n.t("historicalOverview.view.accounts") })).toBeInTheDocument();
  expect(screen.getByRole("tab", { name: i18n.t("historicalOverview.view.accounts") })).toHaveAttribute("tabindex", "0");
  const count = read.mock.calls.length;
  await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.returnToCategories") }));
  await waitFor(() => expect(screen.getByRole("button", { name: "CNY" })).toHaveFocus());
  expect(screen.getByRole("tabpanel", { name: i18n.t("historicalOverview.categoryCompare") })).toBeInTheDocument();
  expect(screen.getByRole("tab", { name: i18n.t("historicalOverview.categoryCompare") })).toHaveAttribute("tabindex", "0");
  expect(main.scrollTop).toBe(180);
  expect(screen.getByRole("button", { name: i18n.t("historicalOverview.byCurrency") })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByLabelText(i18n.t("historicalOverview.compareDate"))).toHaveValue("2026-08-09");
  expect(read).toHaveBeenCalledTimes(count);
});

it.each(["en", "zh-CN", "zh-TW"])("shows applied evidence and concrete side-specific missing inputs without new reads (%s)", async locale => {
  await i18n.changeLanguage(locale);
  const data = result("2026-08-01", "current");
  const evidence = { id: "quote-id", source: "provider", sourceKey: "recorded-provider", effectiveAt: "2026-08-01T21:00:00Z", marketDate: "2026-08-01", freshness: "stale", value: "0.0000012", currency: "USD", baseCurrency: "", quoteCurrency: "", operation: "", observationKind: "daily_close", priceBasis: "raw_close", timestampBasis: "session_close", sourcePolicyVersion: "stored-policy" };
  data.rows![0].left = cell("100", { price: evidence, fx: { ...evidence, id: "fx-id", value: "0.125", currency: "", baseCurrency: "CNY", quoteCurrency: "USD", operation: "divide", priceBasis: "", observationKind: "daily_reference" } });
  data.rows![0].right = cell(null, { complete: false, currency: "USD", nativeAmount: "10", missing: ["fx_rate"], missingDetails: [{ kind: "fx_rate", baseCurrency: "CNY", quoteCurrency: "USD", source: "manual", instrumentId: "instrument-id" }] });
  read.mockResolvedValue(data);
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-01" initialContext={{ compareTo: "current", view: "accounts" }} onExit={vi.fn()} /></QueryClientProvider>);
  await userEvent.click(await screen.findByRole("button", { name: "Bank" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog).toHaveTextContent("0.0000012 USD"); expect(dialog).toHaveTextContent("1 CNY = 0.125 USD");
  expect(dialog).toHaveTextContent("recorded-provider"); expect(dialog).toHaveTextContent("quote-id"); expect(dialog).toHaveTextContent("raw_close");
  expect(dialog).toHaveTextContent(i18n.t("historicalOverview.fxOperation.divide"));
  expect(dialog).toHaveTextContent("USD ↔ CNY"); expect(dialog).toHaveTextContent("instrument-id");
  expect(read).toHaveBeenCalledTimes(1);
});

it("uses explicit identity and ordered dates for a History navigation, keeping current capture semantics clear", async () => {
  const data = result("2026-08-09", "2026-08-01");
  data.rows![0].accountId = "explicit-account"; data.rows![0].instrumentId = "explicit-instrument";
  read.mockResolvedValue(data); const open = vi.fn(); const client = createTestQueryClient();
  render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-09" initialContext={{ compareTo: "2026-08-01", view: "accounts" }} onExit={vi.fn()} /></QueryClientProvider></NavigationContext.Provider>);
  await userEvent.click(await screen.findByRole("button", { name: "Bank" }));
  await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Leave panorama · View transaction history" }));
  expect(open).toHaveBeenCalledWith({ page: "history", filters: { accountId: "explicit-account", instrumentId: "explicit-instrument", from: "2026-08-01", to: "2026-08-09" } });
  expect(read).toHaveBeenCalledTimes(1);
});

it.each(["table", "card"])("groups category-matching and full-account context in the %s detail path", async path => {
  const data = result("2026-08-01", "2026-08-09");
  const usd = { ...data.rows![1], key: "usd", name: "USD", left: cell("10", { currency: "USD" }), right: cell("10", { currency: "USD" }) };
  data.rows!.push(usd);
  read.mockResolvedValue(data);
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-01" initialContext={{ compareTo: "2026-08-09", view: "accounts", categoryFilter: { role: "asset", dimension: "byCurrency", key: "CNY" }, expanded: ["bank"] }} onExit={vi.fn()} /></QueryClientProvider>);
  const trigger = await screen.findByRole("button", { name: path === "table" ? "Bank" : i18n.t("historicalOverview.openCard", { name: "Bank" }) });
  await userEvent.click(trigger);
  const dialog = await screen.findByRole("dialog");
  const cnyLabel = i18n.t("historicalOverview.cashCurrency", { currency: "CNY" });
  const usdLabel = i18n.t("historicalOverview.cashCurrency", { currency: "USD" });
  const matched = within(dialog).getByRole("region", { name: i18n.t("historicalOverview.matchingComponents") });
  const other = within(dialog).getByRole("region", { name: i18n.t("historicalOverview.otherAccountComponents") });
  expect(within(matched).getByRole("button", { name: cnyLabel })).toBeInTheDocument();
  expect(within(matched).queryByRole("button", { name: usdLabel })).not.toBeInTheDocument();
  expect(within(other).getByRole("button", { name: usdLabel })).toBeInTheDocument();
  expect(dialog).toHaveTextContent(i18n.t("historicalOverview.detailCategoryScope"));
  await userEvent.click(within(other).getByRole("button", { name: usdLabel }));
  expect(screen.getByRole("dialog")).toHaveTextContent(i18n.t("historicalOverview.outsideCategoryFilter"));
  await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Back to Bank" }));
  expect(within(within(screen.getByRole("dialog")).getByRole("region", { name: i18n.t("historicalOverview.matchingComponents") })).getByRole("button", { name: cnyLabel })).toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
});


it.each((["en", "zh-CN", "zh-TW"] as const).flatMap(locale => (["byClass", "byCurrency"] as const).map(dimension => ({ locale, dimension }))))(
  "uses canonical category labels in both page and detail filter chips ($locale, $dimension)", async ({ locale, dimension }) => {
    await i18n.changeLanguage(locale);
    const data = result();
    data.rows![1].left!.assetClass = "property";
    read.mockResolvedValue(data);
    const client = createTestQueryClient();
    const key = dimension === "byClass" ? "property" : "CNY";
    const label = dimension === "byClass" ? i18n.t("enum.property") : "CNY";
    const chip = i18n.t("historicalOverview.filteredCategory", { category: label });
    render(<QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-01" initialContext={{ view: "accounts", categoryFilter: { role: "asset", dimension, key } }} onExit={vi.fn()} /></QueryClientProvider>);
    const account = await screen.findByRole("button", { name: "Bank" });
    expect(screen.getByText(chip)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Unchanged" })).not.toBeInTheDocument();
    await userEvent.click(account);
    expect(within(await screen.findByRole("dialog")).getByText(chip)).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(account).toHaveFocus();
    await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.clearFilter") }));
    expect(screen.getByRole("button", { name: "Unchanged" })).toBeInTheDocument();
  },
);

// jsdom cannot measure flex layout. Guard the nonshrinking target and existing
// viewport/scroll/header constraints; Linux pointer recheck verifies pixel size.
it.each([{ width: 390, scale: 1 }, { width: 195, scale: 2 }])(
  "keeps return-to-account usable inside a narrow scrolling Sheet ($width px, scale $scale)", async ({ width, scale }) => {
    vi.stubGlobal("visualViewport", Object.assign(new EventTarget(), { width, height: 220, offsetLeft: 0, offsetTop: 0, scale }));
    const data = result();
    const name = "Long retained bank account name ".repeat(5).trim();
    data.rows![0].name = name;
    read.mockResolvedValue(data);
    const client = createTestQueryClient();
    render(<QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-01" initialContext={{ view: "accounts" }} onExit={vi.fn()} /></QueryClientProvider>);
    const account = await screen.findByRole("button", { name });
    await userEvent.click(account);
    await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: i18n.t("historicalOverview.cashCurrency", { currency: "CNY" }) }));
    const panel = screen.getByRole("dialog");
    const back = within(panel).getByRole("button", { name: i18n.t("historicalOverview.returnToAccount", { name }) });
    expect(back).toHaveClass("h-auto", "min-h-8", "shrink-0", "self-start", "max-w-full", "whitespace-normal");
    expect(back).not.toHaveClass("w-full");
    expect(panel).toHaveClass("flex", "flex-col", "overflow-y-auto");
    expect(panel.parentElement).toHaveStyle({ width: `${width}px`, height: "220px" });
    expect(panel.querySelector("[data-sheet-close]")?.parentElement).toHaveClass("sticky", "shrink-0");
    back.focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("dialog")).toHaveAccessibleName(name);
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(account).toHaveFocus();
  },
);


it("atomically enables a new series capture and keeps metric, dates and tabs local", async () => {
  const { client } = show(true);
  await screen.findByRole("region", { name: "Asset allocation" });
  const originalReads = read.mock.calls.length;
  await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
  expect(seriesRead).not.toHaveBeenCalled();
  let resolve!: (value: HistoricalOverviewSeriesResult) => void;
  seriesRead.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  await userEvent.click(screen.getByRole("button", { name: "Enable trend · New capture" }));
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveAttribute("aria-label", i18n.t("historicalOverview.loading"));
  const capture = seriesResult("2026-08-01", "2026-08-09");
  capture.overview.capturedAt = "2026-08-10T12:30:00Z";
  await act(async () => resolve(capture));
  const trend = await screen.findByRole("region", { name: "Time trend" });
  expect(screen.getAllByTitle(capture.overview.capturedAt).length).toBeGreaterThan(0);
  await userEvent.click(within(trend).getByRole("button", { name: "Assets" }));
  await userEvent.click(within(trend).getByRole("button", { name: "2026-08-03" }));
  await userEvent.click(screen.getByRole("tab", { name: "Account details" }));
  await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
  expect(within(screen.getByRole("region", { name: "Time trend" })).getByRole("button", { name: "Assets" })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("button", { name: "2026-08-03" })).toHaveAttribute("aria-pressed", "true");
  await act(async () => client.invalidateQueries());
  expect(seriesRead).toHaveBeenCalledTimes(1); expect(read).toHaveBeenCalledTimes(originalReads);
  const row = screen.getByRole("button", { name: "2026-08-03" }).closest("tr")!;
  await userEvent.click(within(row).getByRole("button", { name: "Set as A · Recapture range" }));
  await waitFor(() => expect(seriesRead).toHaveBeenCalledTimes(2));
  expect(seriesRead).toHaveBeenLastCalledWith("2026-08-03", "2026-08-09");
  expect(screen.getByLabelText(i18n.t("historicalOverview.baselineDate"))).toHaveValue("2026-08-03");
  await userEvent.click(within(screen.getByRole("button", { name: "2026-08-03" }).closest("tr")!).getByRole("button", { name: "Open day · New capture" }));
  await waitFor(() => expect(read).toHaveBeenLastCalledWith("2026-08-03", ""));
  expect(screen.getByLabelText("Compare with")).toHaveValue("none");
});

it("keeps failed range capture and budget rejection explicit without showing old totals", async () => {
  show(true); await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
  seriesRead.mockRejectedValueOnce(new Error(JSON.stringify({ code: "historical_series_budget", message: "budget exceeded" })));
  await userEvent.click(screen.getByRole("button", { name: "Enable trend · New capture" }));
  expect(await screen.findByText(i18n.t("historicalOverview.trend.budgetError"))).toBeInTheDocument();
  expect(screen.queryByRole("region", { name: "Total change (right minus left)" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: i18n.t("common.retryAction") }));
  await screen.findByRole("region", { name: "Time trend" });
  expect(seriesRead).toHaveBeenCalledTimes(2);
});

it("uses nullable series subtotals in endpoint cards instead of legacy P1 zero placeholders", async () => {
  seriesRead.mockImplementation((from: string, to: string) => {
    const capture = seriesResult(from, to);
    const points = capture.points!;
    const last = points[points.length - 1];
    for (const state of [capture.overview.left, capture.overview.right!]) {
      state.assets = null; state.liabilities = null; state.netWorth = null; state.complete = false;
      state.knownAssets = "0"; state.knownLiabilities = "0"; // Unchanged P1 contract.
    }
    for (const point of points) {
      point.assets = null; point.liabilities = null; point.netWorth = null;
      point.knownAssets = null; point.knownLiabilities = null;
      point.assetsComplete = false; point.liabilitiesComplete = false;
      point.assetCoverage = { included: 1, known: 0, missing: 1 };
      point.liabilityCoverage = { included: 1, known: 0, missing: 1 };
    }
    // B retains one observed zero alongside an unknown leaf; its subtotal is
    // known zero, distinct from A's entirely unknown subtotal.
    last.knownAssets = "0"; last.assetCoverage = { included: 2, known: 1, missing: 1 };
    return Promise.resolve(capture);
  });
  show(true); await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
  await userEvent.click(screen.getByRole("button", { name: "Enable trend · New capture" }));
  const summaries = await screen.findByRole("region", { name: i18n.t("historicalOverview.totalChange") });
  expect(within(summaries).getAllByText(`${i18n.t("historicalOverview.knownSubtotal")}: ${i18n.t("historicalOverview.unknown")}`)).toHaveLength(3);
  expect(within(summaries).getAllByText(`${i18n.t("historicalOverview.knownSubtotal")}: ${formatHistoricalAmount("0", "CNY")}`)).toHaveLength(1);
});

it("cancels obsolete range work and suppresses a late result after explicit date selection", async () => {
  show(true); await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
  let resolve!: (value: HistoricalOverviewSeriesResult) => void;
  const cancel = vi.fn();
  const pending = Object.assign(new Promise<HistoricalOverviewSeriesResult>(done => { resolve = done; }), { cancel });
  seriesRead.mockReturnValueOnce(pending);
  await userEvent.click(screen.getByRole("button", { name: "Enable trend · New capture" }));
  fireEvent.change(screen.getByLabelText(i18n.t("historicalOverview.baselineDate")), { target: { value: "2026-08-04" } });
  await waitFor(() => expect(cancel).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(read).toHaveBeenLastCalledWith("2026-08-04", "2026-08-09"));
  await act(async () => resolve(seriesResult("2026-08-01", "2026-08-09")));
  expect(screen.getByLabelText(i18n.t("historicalOverview.baselineDate"))).toHaveValue("2026-08-04");
  expect(screen.queryByRole("button", { name: "Set as A · Recapture range" })).not.toBeInTheDocument();
});

it("allows all 31 civil labels and rejects a 32-label enable without truncation", async () => {
  vi.setSystemTime(new Date("2026-09-02T12:00:00Z"));
  read.mockImplementation((from: string, to: string) => Promise.resolve({ ...result(from, to), lastClosedDate: "2026-09-01" }));
  show(true); await screen.findByRole("region", { name: "Asset allocation" });
  await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
  fireEvent.change(screen.getByLabelText(i18n.t("historicalOverview.trend.from")), { target: { value: "2026-08-01" } });
  fireEvent.change(screen.getByLabelText(i18n.t("historicalOverview.trend.to")), { target: { value: "2026-09-01" } });
  const enable = screen.getByRole("button", { name: "Enable trend · New capture" });
  expect(enable).toBeDisabled(); expect(seriesRead).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText(i18n.t("historicalOverview.trend.to")), { target: { value: "2026-08-31" } });
  expect(enable).toBeEnabled();
  await userEvent.click(enable);
  await waitFor(() => expect(seriesRead).toHaveBeenLastCalledWith("2026-08-01", "2026-08-31"));
  expect(within(await screen.findByRole("region", { name: "Time trend" })).getAllByRole("row")).toHaveLength(32);
});
