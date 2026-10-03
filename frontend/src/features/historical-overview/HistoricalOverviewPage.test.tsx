import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { HistoricalOverviewPage } from "./HistoricalOverviewPage";
import i18n from "@/i18n";
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
  return { timezone: "UTC", originDate: "2026-08-01", lastClosedDate: "2026-08-09", capturedAt: "2026-08-10T12:00:00Z", inputGeneration: 1, resolverPolicy: "market-date-daily-summary-v3", left: state, right: compareTo ? { ...state, date: compareTo, current: compareTo === "current" } : null, rows };
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
  expect(detail).toHaveTextContent("2026-07-20T00:00:00Z");
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
