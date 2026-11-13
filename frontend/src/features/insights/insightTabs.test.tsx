import { NavigationContext } from "@/app/NavigationContext";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps, ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
  TrendChart: ({ dates, series }: { dates: string[]; series: { values: string[] }[] }) => <div data-testid="trend-chart" data-dates={JSON.stringify(dates)} data-values={JSON.stringify(series[0].values)} />,
}));

function renderWithClient(child: ReactNode) {
  const queryClient = createTestQueryClient({ retry: false });
  const view = render(<QueryClientProvider client={queryClient}>{child}</QueryClientProvider>);
  return { queryClient, ...view };
}

function ConnectedReturnTrendTab() {
  const session = useAnalysisStore();
  return <ReturnTrendTab session={session} />;
}

function ConnectedContributionTab({ onOpenHistory }: { onOpenHistory?: ComponentProps<typeof ContributionTab>["onOpenHistory"] }) {
  const session = useAnalysisStore();
  return <ContributionTab session={session} onOpenHistory={onOpenHistory} />;
}

const available = { available: true, status: "ok", valuationForced: null };

function mockRealizedContribution() {
  contribution.mockResolvedValue({
    returnType: "realized",
    groupBy: "instrument",
    rows: [{ key: "instrument-1", label: "instrument-1", amount: { amount: "-3", currency: "USD" }, ratedDays: 0, totalDays: 0, ...available }],
    ratedDays: 0,
    totalDays: 0,
    ...available,
  });
  contributionItem.mockResolvedValue({
    key: "instrument-1",
    label: "instrument-1",
    amount: { amount: "-3", currency: "USD" },
    components: [],
    byAccount: [{ key: "account-1", accountId: "account-1", amount: { amount: "-3", currency: "USD" } }],
    historyHint: { kinds: ["buy", "sell"], from: "2026-09-01", to: "2026-09-01", accountId: "account-1", instrumentId: "instrument-1" },
    ...available,
  });
}

describe("insight tabs", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-07T12:00:00Z"));
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

  it("keeps the list and sort focus mounted while the backend changes ordering", async () => {
    const initial = await contribution();
    let finish!: (value: typeof initial) => void;
    contribution.mockImplementation((_request, _type, _group, order) => order === "amount_desc" ? Promise.resolve(initial) : new Promise(resolve => { finish = resolve; }));
    const view = renderWithClient(<ContributionTab session={useAnalysisStore.getState()} />);
    await screen.findByTestId("contribution-ranking");
    const sort = screen.getByRole("combobox", { name: "Sort" });
    const list = screen.getByRole("list", { name: "Contribution" });
    expect(screen.getByText("Sort")).toHaveClass("sr-only");
    sort.focus();
    fireEvent.change(sort, { target: { value: "amount_asc" } });
    await waitFor(() => expect(contribution).toHaveBeenCalledWith(expect.anything(), "total_return", "instrument", "amount_asc"));
    expect(screen.getByRole("list", { name: "Contribution" })).toBe(list);
    expect(screen.getByRole("combobox", { name: "Sort" })).toBe(sort);
    expect(sort).toHaveFocus();
    finish(initial);
    await waitFor(() => expect(view.queryClient.isFetching()).toBe(0));
    expect(sort).toHaveFocus();
    expect(screen.getByRole("list", { name: "Contribution" })).toBe(list);
  });

  it.each(["date", "account", "metric", "grouping"])("does not retain old contribution amounts when changing %s", async kind => {
    const view = renderWithClient(<ContributionTab session={useAnalysisStore.getState()} />);
    await screen.findByTestId("contribution-ranking");
    contribution.mockImplementation(() => new Promise(() => {}));
    const state = useAnalysisStore.getState();
    const next = kind === "date" ? { ...state, from: "2026-08-01", to: "2026-08-31" } : kind === "account" ? { ...state, scope: "account" as const, scopeId: "account-1" } : kind === "metric" ? { ...state, contributionReturnType: "dividend_interest" as const } : state;
    if (kind === "grouping") fireEvent.change(screen.getByRole("combobox", { name: "Group by" }), { target: { value: "account" } });
    else view.rerender(<QueryClientProvider client={view.queryClient}><ContributionTab session={next} /></QueryClientProvider>);
    await waitFor(() => expect(screen.queryByTestId("contribution-ranking")).not.toBeInTheDocument());
  });

  it.each([
    ["Return trend", ReturnTrendTab, returnTrend],
    ["Contribution", ContributionTab, contribution],
    ["Asset trend", AssetTrendTab, assetTrend],
    ["Categories", CategoriesTab, categories],
  ])("explains the unclosed current month in %s without querying", async (_name, Tab, query) => {
    vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
    renderWithClient(<Tab session={useAnalysisStore.getState()} />);
    expect(await screen.findByText("No closed dates in the current month yet.")).toBeInTheDocument();
    expect(screen.getByText("Today is still open. Choose an earlier period or return after the first day has closed.")).toBeInTheDocument();
    expect(query).not.toHaveBeenCalled();
    expect(screen.queryByTestId("trend-chart")).not.toBeInTheDocument();
  });

  it.each([
    ["2026-09-30T12:00:00Z", "2026-09-01", "2026-09-29"],
    ["2026-10-02T12:00:00Z", "2026-10-01", "2026-10-01"],
  ])("queries only closed current-month dates at %s", async (instant, from, to) => {
    vi.setSystemTime(new Date(instant));
    renderWithClient(<ReturnTrendTab session={useAnalysisStore.getState()} />);
    await screen.findByTestId("return-trend");
    expect(returnTrend).toHaveBeenCalledWith(expect.objectContaining({ from, to }), "cumulative_amount");
    expect(useAnalysisStore.getState()).toMatchObject({ from: "", to: "" });
  });

  it("keeps insufficient history distinct from an unclosed month", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-09-07T00:00:00Z" });
    renderWithClient(<ReturnTrendTab session={useAnalysisStore.getState()} />);
    expect(await screen.findByText("Not enough historical data to calculate returns.")).toBeInTheDocument();
    expect(screen.queryByText("No closed dates in the current month yet.")).not.toBeInTheDocument();
    expect(returnTrend).not.toHaveBeenCalled();
  });

  it("keeps Return Trend displays separate and sends linked_rate to Wails", async () => {
    renderWithClient(<ConnectedReturnTrendTab />);
    expect(await screen.findByTestId("return-trend")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Display"), { target: { value: "linked_rate" } });
    await waitFor(() => expect(returnTrend).toHaveBeenCalledWith(expect.anything(), "linked_rate"));
    expect(await screen.findByText("Points show daily Modified Dietz rates; the linked period rate is shown in the summary.")).toBeInTheDocument();
  });

  it("prepends a zero baseline without removing the first day return", async () => {
    renderWithClient(<ReturnTrendTab session={useAnalysisStore.getState()} />);
    const chart = await screen.findByTestId("trend-chart");
    expect(chart).toHaveAttribute("data-dates", JSON.stringify(["2026-08-31", "2026-09-01"]));
    expect(chart).toHaveAttribute("data-values", JSON.stringify(["0", "2"]));
  });

  it("renders component-level return sources instead of holding contributors", async () => {
    renderWithClient(<ReturnTrendTab session={useAnalysisStore.getState()} />);
    await screen.findByText("Price Change");
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(screen.queryByText("+100%")).not.toBeInTheDocument();
  });

  it("keeps date shortcuts in the shared filter bar only", async () => {
    renderWithClient(<ConnectedReturnTrendTab />);
    await screen.findByTestId("return-trend");
    expect(screen.queryByRole("button", { name: "30D" })).not.toBeInTheDocument();
  });

  it("opens a Contribution item for the active independent view", async () => {
    renderWithClient(<ConnectedContributionTab />);
    await screen.findAllByText("QQQ");
    expect(screen.getByText("Rate days 1/3")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: /QQQ/ })[0]);
    expect(await screen.findByText("Price Change")).toBeInTheDocument();
    expect(await screen.findByText("Brokerage")).toBeInTheDocument();
    expect(contributionItem).toHaveBeenCalledWith(expect.anything(), "total_return", "instrument", "instrument-1");
  });

  it("keeps entirely unknown contribution summaries unavailable without implying flat returns", async () => {
    contribution.mockResolvedValue({ returnType: "total_return", groupBy: "instrument", rows: [{ key: "instrument-1", label: "instrument-1", amount: null, rate: null, ratedDays: 0, totalDays: 1, available: false, status: "unavailable" }], summary: { amountStatus: "unavailable" }, ratedDays: 0, totalDays: 1, ...available, status: "partial" });
    renderWithClient(<ConnectedContributionTab />);
    const summary = await screen.findByLabelText("Contribution summary");
    expect(within(summary).getAllByText("Unavailable")).toHaveLength(3);
    expect(summary).not.toHaveTextContent("Flat");
  });

  it("uses backend summaries while ranking amounts independently of complete searchable details", async () => {
    contribution.mockResolvedValue({ returnType: "total_return", groupBy: "instrument", rows: [
      ...Array.from({ length: 12 }, (_, index) => ({ key: `group-${index}`, label: `Group ${index}`, amount: { amount: String(index % 2 ? -index : index), currency: "USD" }, rate: "0.01", ratedDays: 1, totalDays: 1, ...available })),
      { key: "missing", label: "Unknown amount", amount: null, status: "unavailable", available: false, ratedDays: 0, totalDays: 1 },
    ], summary: { total: { amount: "999.004", currency: "USD" }, positive: { amount: "1000", currency: "USD" }, negative: { amount: "-0.996", currency: "USD" }, amountStatus: "partial" }, ratedDays: 1, totalDays: 1, ...available });
    renderWithClient(<ConnectedContributionTab />);
    const chart = await screen.findByTestId("contribution-ranking");
    expect(within(chart).getAllByTestId("contribution-bar")).toHaveLength(10);
    expect(within(chart).getAllByTestId("contribution-bar")[0]).toHaveAttribute("data-amount", "-11");
    const summary = screen.getByLabelText("Contribution summary");
    expect(summary).toHaveTextContent("+US$999.00");
    expect(within(summary).getAllByText("Known subtotal · incomplete data")).toHaveLength(3);
    fireEvent.click(screen.getByRole("button", { name: "Positive" }));
    expect(within(chart).getAllByTestId("contribution-bar")).toHaveLength(5);
    fireEvent.change(screen.getByLabelText("Search contributions"), { target: { value: "Unknown" } });
    const missingRow = within(screen.getByRole("list", { name: "Contribution" })).getByRole("button", { name: /Unknown amount/ });
    expect(missingRow).toBeInTheDocument();
    expect(missingRow).not.toHaveTextContent("Flat");
    expect(within(chart).getAllByTestId("contribution-bar")).toHaveLength(5);
    fireEvent.change(screen.getByLabelText("Display count"), { target: { value: "all" } });
    fireEvent.click(screen.getByRole("button", { name: "Largest impact" }));
    expect(within(chart).getAllByTestId("contribution-bar")).toHaveLength(12);
    expect(summary).toHaveTextContent("+US$999.00");
  });

  it("shows separate cash currencies and opens only the selected cash contribution", async () => {
    contribution.mockResolvedValue({ returnType: "total_return", groupBy: "instrument", rows: [
      { key: "cash:USD", label: "cash:USD", amount: { amount: "3.94", currency: "CNY" }, ratedDays: 1, totalDays: 1, ...available },
      { key: "cash:SGD", label: "cash:SGD", amount: { amount: "0.84", currency: "CNY" }, ratedDays: 1, totalDays: 1, ...available },
    ], ratedDays: 1, totalDays: 1, ...available });
    contributionItem.mockResolvedValue({ key: "cash:SGD", label: "cash:SGD", amount: { amount: "0.84", currency: "CNY" }, ratedDays: 1, totalDays: 1,
      components: [], byAccount: [], historyHint: { from: "2026-09-01", to: "2026-09-01" }, ...available });
    renderWithClient(<ConnectedContributionTab />);
    expect((await screen.findAllByRole("button", { name: /Cash · USD/ }))[0]).toHaveTextContent("3.94");
    fireEvent.click((await screen.findAllByRole("button", { name: /Cash · SGD/ }))[0]);
    expect(await screen.findByRole("heading", { name: "Cash · SGD" })).toBeInTheDocument();
    await waitFor(() => expect(contributionItem).toHaveBeenCalledWith(expect.anything(), "total_return", "instrument", "cash:SGD"));
  });

  it("opens ledger activity from a contribution item with date and scope filters", async () => {
    const onOpenHistory = vi.fn();
    renderWithClient(<ConnectedContributionTab onOpenHistory={onOpenHistory} />);
    fireEvent.click((await screen.findAllByRole("button", { name: /QQQ/ }))[0]);
    fireEvent.click(await screen.findByRole("button", { name: "View in ledger activity" }));
    expect(onOpenHistory).toHaveBeenCalledWith({
      from: "2026-09-01",
      to: "2026-09-01",
      accountId: "account-1",
      instrumentId: "instrument-1",
      kinds: undefined,
    });
  });

  it.each([
    ["currency", { currency: "USD" }, true],
    ["asset class", { assetClass: "stock" }, true],
    ["exact date, account, and instrument filters", {}, false],
  ] as const)("reports ledger activity filter loss for realized contributions when applicable (%s)", async (_context, moreFilters, warns) => {
    useAnalysisStore.getState().replaceFilters({ returnType: "realized", moreFilters });
    mockRealizedContribution();
    const onOpenHistory = vi.fn();

    renderWithClient(<ConnectedContributionTab onOpenHistory={onOpenHistory} />);
    fireEvent.click((await screen.findAllByRole("button", { name: /QQQ/ }))[0]);
    const historyButton = await screen.findByRole("button", { name: "View in ledger activity" });
    const historyFilterWarning = "Ledger activity keeps the date, account, and instrument filters, but not the currency or asset-class filter used for this contribution.";
    if (warns) {
      expect(await screen.findByText(historyFilterWarning)).toBeInTheDocument();
    } else {
      expect(screen.queryByText(historyFilterWarning)).not.toBeInTheDocument();
      expect(screen.queryByText("Ledger activity will open for the full selected period, not a single account or instrument.")).not.toBeInTheDocument();
    }

    fireEvent.click(historyButton);
    expect(onOpenHistory).toHaveBeenCalledWith({
      from: "2026-09-01",
      to: "2026-09-01",
      accountId: "account-1",
      instrumentId: "instrument-1",
      kinds: ["buy", "sell"],
    });
  });

  it("starts Contribution on the navigated return type", async () => {
    useAnalysisStore.getState().setFilters({ returnType: "dividend_interest" });
    renderWithClient(<ConnectedContributionTab />);
    await screen.findByTestId("contribution");
    expect(screen.getByLabelText("Return type")).toHaveValue("dividend_interest");
    expect(contribution).toHaveBeenCalledWith(expect.anything(), "dividend_interest", "instrument", "amount_desc");
  });

  it("hides Realized rate sort and currency grouping", async () => {
    renderWithClient(<ConnectedContributionTab />);
    await screen.findAllByText("QQQ");
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
    expect(screen.getByLabelText("Granularity")).toHaveValue("day");
    expect(assetTrend).toHaveBeenCalledWith(expect.anything(), "day", "net_worth");
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

  it("opens ledger activity from a category record for the selected day and account", async () => {
    const onOpenHistory = vi.fn();
    renderWithClient(<CategoriesTab session={useAnalysisStore.getState()} onOpenHistory={onOpenHistory} />);
    fireEvent.click(await screen.findByRole("button", { name: /Brokerage/ }));
    fireEvent.click(await screen.findByRole("button", { name: "View in ledger activity" }));
    expect(onOpenHistory).toHaveBeenCalledWith({ from: "2026-09-01", to: "2026-09-01", accountId: "account-1", instrumentId: undefined }, "activity-1");
  });

  it("keeps Return type visible when Realized is unavailable so Total can be restored", async () => {
    contribution.mockImplementation((_request, returnType) => {
      if (returnType === "realized") {
        return Promise.resolve({ available: false, status: "unavailable", missingReason: "realized gain is unavailable for this filter", returnType: "realized", rows: [], valuationForced: null });
      }
      return Promise.resolve({ returnType: "total_return", groupBy: "instrument", rows: [{ key: "instrument-1", label: "instrument-1", amount: { amount: "2", currency: "USD" }, rate: "0.02", ratedDays: 1, totalDays: 3, ...available }], ratedDays: 1, totalDays: 3, ...available });
    });
    renderWithClient(<ConnectedContributionTab />);
    await screen.findAllByText("QQQ");
    fireEvent.change(screen.getByLabelText("Return type"), { target: { value: "realized" } });
    expect(await screen.findByText("realized gain is unavailable for this filter")).toBeInTheDocument();
    expect(screen.getByLabelText("Return type")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Return type"), { target: { value: "total_return" } });
    expect((await screen.findAllByText("QQQ"))[0]).toBeInTheDocument();
  });

  it("shows Categories completeness and forced-base on the main list", async () => {
    categories.mockResolvedValue({
      total: { amount: "10", currency: "USD" },
      rows: [{ key: "account-1", label: "account-1", accountId: "account-1", amount: { amount: "10", currency: "USD" } }],
      available: true,
      status: "partial",
      missingReason: "one category day is incomplete",
      valuationForced: "base",
    });
    renderWithClient(<CategoriesTab session={useAnalysisStore.getState()} />);
    expect(await screen.findByText("Partial coverage")).toBeInTheDocument();
    expect(screen.getByText("Native valuation is unavailable; showing base currency.")).toBeInTheDocument();
    expect(screen.getByText("one category day is incomplete")).toBeInTheDocument();
    expect(screen.getByLabelText("Category type")).toBeInTheDocument();
  });

  it("shows Asset Trend forced-base and coverage on the main result", async () => {
    assetTrend.mockResolvedValue({
      points: [{ period: "2026-09-01", value: { amount: "102", currency: "USD" }, rate: "0.02", ratedDays: 1, totalDays: 3, ...available }],
      summary: { amount: "102", currency: "USD" },
      rate: null,
      ratedDays: 1,
      totalDays: 3,
      available: true,
      status: "partial",
      missingReason: "one trend day is incomplete",
      valuationForced: "base",
    });
    renderWithClient(<AssetTrendTab session={useAnalysisStore.getState()} />);
    expect(await screen.findByText("Native valuation is unavailable; showing base currency.")).toBeInTheDocument();
    expect(screen.getByText("one trend day is incomplete")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Metric"), { target: { value: "return_rate" } });
    expect(await screen.findByText("Period rate unavailable · 1 of 3 days had a calculable rate.")).toBeInTheDocument();
  });
  it.each(["en", "zh-CN", "zh-TW"])("shows pre-funding cash contribution as unavailable instead of zero (%s)", async (locale) => {
    const { default: i18n } = await import("@/i18n");
    await i18n.changeLanguage(locale);
    vi.setSystemTime(new Date("2026-10-20T12:00:00Z"));
    useAnalysisStore.getState().setFilters({ from: "2026-10-02", to: "2026-10-07" });
    const reason = "funding cash is recorded after the product start";
    contribution.mockResolvedValue({
      returnType: "total_return",
      groupBy: "instrument",
      rows: [
        { key: "deposit", label: "Synthetic deposit", amount: { amount: "12.34", currency: "USD" }, rate: "0.001", ratedDays: 6, totalDays: 6, available: true, status: "partial", missingReason: reason, valuationForced: null },
        { key: "cash:USD", label: "cash:USD", amount: null, rate: null, ratedDays: 0, totalDays: 6, available: false, status: "unavailable", missingReason: reason, valuationForced: null },
      ],
      ratedDays: 6,
      totalDays: 6,
      available: true,
      status: "partial",
      missingReason: reason,
      valuationForced: null,
    });
    renderWithClient(<ConnectedContributionTab />);
    expect((await screen.findAllByText(i18n.t("insights.analysisReasons.fundingCashAfterProductStart"))).length).toBeGreaterThan(0);
    expect((await screen.findAllByText(i18n.t("insights.unavailableAmount"))).length).toBeGreaterThan(0);
    expect((await screen.findAllByText(i18n.t("insights.status.unavailable"))).length).toBeGreaterThan(0);
    expect(screen.queryByText("US$0.00")).not.toBeInTheDocument();
    expect(screen.queryByText("$0.00")).not.toBeInTheDocument();
    await i18n.changeLanguage("en");
  });

  it("opens period-specific Data Health from a partial result", async () => {
    useAnalysisStore.getState().setFilters({ from: "2026-09-01", to: "2026-09-06" });
    const open = vi.fn();
    returnTrend.mockResolvedValue({ available: true, status: "partial", rate: "0.01", points: [], sources: [], ratedDays: 3, totalDays: 6, missingValuationDays: 1, amountWithoutRateDays: 2 });
    renderWithClient(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><ReturnTrendTab session={useAnalysisStore.getState()} /></NavigationContext.Provider>);
    fireEvent.click(await screen.findByRole("button", { name: "View gaps for this period" }));
    expect(screen.getByText("This rate links 3 of 6 days.")).toBeVisible();
    expect(screen.getByText("1 day missing valuation data")).toBeVisible();
    expect(screen.getByText("2 days with complete return amounts but no calculable rate")).toBeVisible();
    expect(open).toHaveBeenCalledWith({ page: "data-health", focus: { rangeStart: "2026-09-01", rangeEnd: "2026-09-06" } });
  });

});
