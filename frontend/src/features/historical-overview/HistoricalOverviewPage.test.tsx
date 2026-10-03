import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { HistoricalOverviewPage } from "./HistoricalOverviewPage";
import i18n from "@/i18n";
import { formatAmount } from "@/lib/money";
import type { HistoricalOverviewCell, HistoricalOverviewResult, HistoricalOverviewRow } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

const { read, origin } = vi.hoisted(() => ({ read: vi.fn(), origin: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({ Service: { HistoricalOverview: read } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: origin } }));

function cell(value = "100", patch: Partial<HistoricalOverviewCell> = {}): HistoricalOverviewCell {
  return { status: "active", included: true, complete: true, currency: "CNY", nativeAmount: value, baseAmount: value, quantity: null, assetClass: "cash", manual: false, valueSourceAt: "2026-08-01T12:00:00Z", valueSourceId: "origin", price: null, fx: null, missing: [], ...patch };
}

function result(date = "2026-08-01", compareTo = ""): HistoricalOverviewResult {
  const row = (key: string, parentKey: string, kind: string, name: string, changed: boolean): HistoricalOverviewRow => ({ key, parentKey, kind, name, role: "asset", left: cell(), right: compareTo ? cell(changed ? "120" : "100") : null, baseChange: compareTo ? (changed ? "20" : "0") : null, nativeChange: compareTo ? (changed ? "20" : "0") : null, quantityChange: null, changed: Boolean(compareTo) && changed });
  const state = { date, cutoffAt: `${date}T23:59:59.999Z`, current: false, currency: "CNY", complete: true, assets: "200", liabilities: "0", netWorth: "200", knownAssets: "200", knownLiabilities: "0", byClass: [{ key: "cash", amount: "200", share: "100" }], byCurrency: [{ key: "CNY", amount: "200", share: "100" }] };
  const rows = [row("bank", "", "account", "Bank", true), row("cash", "bank", "cash", "CNY", true), row("steady", "", "account", "Unchanged", false)];
  return { timezone: "UTC", originDate: "2026-08-01", lastClosedDate: "2026-08-09", capturedAt: "2026-08-10T12:00:00Z", inputGeneration: 1, resolverPolicy: "market-date-daily-summary-v3", left: state, right: compareTo ? { ...state, date: compareTo, current: compareTo === "current", cutoffAt: compareTo === "current" ? "2026-08-10T12:00:00Z" : `${compareTo}T23:59:59.999Z`, assets: "220", netWorth: "220", knownAssets: "220" } : null, change: compareTo ? { assets: "20", liabilities: "0", netWorth: "20" } : null, rows };
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-10T12:00:00Z"));
  vi.clearAllMocks();
  void i18n.changeLanguage("en");
  origin.mockResolvedValue({ id: "origin", householdId: "household", startedAt: "2026-08-01T12:00:00Z", createdAt: "2026-08-01T12:00:00Z", timezone: "UTC" });
  read.mockImplementation((date: string, compare: string) => Promise.resolve(result(date, compare)));
});
afterEach(() => vi.useRealTimers());

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

function show() {
  const client = createTestQueryClient();
  const exit = vi.fn();
  const view = render(<QueryClientProvider client={client}><HistoricalOverviewPage onExit={exit} /></QueryClientProvider>);
  return { ...view, client, exit };
}

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
    data.rows![0].left = cell("0", { status: "archived", included: false, manual: true, complete: false, baseAmount: null, missing: ["fx_rate"], price: { id: "price", source: "manual", effectiveAt: "2026-07-20T00:00:00Z", marketDate: "", freshness: "manual" } });
    data.rows![0].right = compare ? cell("0", { status: "zero" }) : null;
    data.rows![2].left = null;
    return Promise.resolve(data);
  });
  show();
  await screen.findByRole("button", { name: "Bank" });
  expect(screen.getByText("Not yet present in retained history")).toBeInTheDocument();
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

it("keeps a cleared date invalid instead of silently selecting the default", async () => {
  show();
  await screen.findByRole("button", { name: "Bank" });
  const calls = read.mock.calls.length;
  fireEvent.change(screen.getByLabelText("Historical date"), { target: { value: "" } });
  expect(screen.getByLabelText("Historical date")).toHaveValue("");
  expect(screen.getByLabelText("Historical date")).toHaveAttribute("aria-invalid", "true");
  expect(screen.queryByRole("button", { name: "Bank" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Previous day" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Next day" })).toBeDisabled();
  expect(read).toHaveBeenCalledTimes(calls);
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
  expect(screen.getByLabelText("Comparison date")).toHaveValue("2026-08-08");
  await screen.findByRole("columnheader", { name: "2026-08-08" });
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
  expect(within(change).getByTitle("0 CNY")).toBeInTheDocument();
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
  const table = await screen.findByRole("table");
  expect(within(table).getByTitle("2 USD")).toHaveTextContent("+");
  expect(within(table).getByText("Not comparable")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Bank" }));
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
  await screen.findByRole("table");
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
