import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { NavigationContext } from "@/app/NavigationContext";
import type { InstrumentDetailContext } from "@/app/navigation";
import { InstrumentDetailPage } from "./InstrumentDetailPage";

const q = vi.hoisted(() => ({
  instruments: [] as Record<string, unknown>[],
  groups: [] as Record<string, unknown>[],
  quote: undefined as Record<string, unknown> | undefined,
  listInstruments: vi.fn(),
  listHoldingGroups: vi.fn(),
  accountGroups: {} as Record<string, Record<string, unknown>[]>,
}));
vi.mock("@/queries/investments", () => ({
  useInstruments: (includeArchived: boolean) => { q.listInstruments(includeArchived); return { data: q.instruments, isLoading: false, isError: false }; },
  useCurrentInstrumentQuote: () => ({ data: q.quote, isLoading: false, isError: false }),
}));
vi.mock("@/queries/analytics", () => ({ useInstrumentHoldingGroups: (enabled: boolean, accountId: string) => {
  q.listHoldingGroups(enabled, accountId);
  return { data: q.accountGroups[accountId] ?? q.groups, isLoading: false, isError: false };
} }));
vi.mock("@/queries/accounts", () => ({ useAccounts: () => ({ data: [{ account: { id: "broker-a", name: "Broker A" } }, { account: { id: "broker-b", name: "Broker B" } }, { account: { id: "archived-broker", name: "Old Broker", archivedAt: "2026-05-01" } }] }) }));
vi.mock("@/queries/directory", () => ({ useMembers: () => ({ data: [] }) }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { timezone: "UTC" } }) }));
vi.mock("@/queries/history", () => ({ useHistoryOrigin: () => ({ data: { timezone: "UTC", startedAt: "2026-01-01" } }) }));
vi.mock("./PricePerformancePanel", () => ({ PricePerformancePanel: () => <div data-testid="market-performance" /> }));
vi.mock("./PerformanceReturns", () => ({ PerformanceReturns: () => <div data-testid="quote-returns" /> }));
vi.mock("@/features/marketdata/QuoteHistorySheet", () => ({ QuoteHistoryContent: ({ focus, onRangeChange }: { focus: unknown; onRangeChange?: (range: { from: string; to: string }) => void }) => <>
  <pre data-testid="quote-records">{JSON.stringify(focus)}</pre>
  <button onClick={() => onRangeChange?.({ from: "2026-07-01", to: "2026-07-31" })}>Select quote period</button>
</> }));

const instrument = { id: "fund-1", householdId: "synthetic", name: "Synthetic Fund", type: "fund", quoteCurrency: "USD", iconKey: "", sortOrder: 0, quoteSource: "manual", createdAt: "2026-01-01", updatedAt: "2026-01-01" };
const money = (amount: string) => ({ amount, currency: "USD" });
const holdings = { instrumentId: "fund-1", name: "Synthetic Fund", type: "fund", symbol: "SYN", quoteCurrency: "USD", quantityUnit: "units", archived: false,
  amounts: { quantity: "10", totalCost: money("100"), currentValue: money("120"), unrealizedGain: money("20") },
  holdings: [
    { holdingId: "h-a", accountId: "broker-a", accountName: "Broker A", amounts: { quantity: "4", totalCost: money("40"), currentValue: money("48"), unrealizedGain: money("8") } },
    { holdingId: "h-b", accountId: "broker-b", accountName: "Broker B", amounts: { quantity: "6", totalCost: money("60"), currentValue: money("72"), unrealizedGain: money("12") } },
  ],
};

function show(context: InstrumentDetailContext) {
  const open = vi.fn();
  const onContextChange = vi.fn();
  const ui = (next: InstrumentDetailContext) => <NavigationContext.Provider value={{ open, openHealth: vi.fn() }}>
    <InstrumentDetailPage context={next} onContextChange={onContextChange} />
  </NavigationContext.Provider>;
  const view = render(ui(context));
  return { open, onContextChange, rerender: (next: InstrumentDetailContext) => view.rerender(ui(next)) };
}

beforeEach(() => {
  vi.clearAllMocks();
  q.instruments = [instrument];
  q.groups = [{ key: "fund", instruments: [holdings], subtotals: [] }];
  q.accountGroups = {};
  q.quote = undefined;
});

it("shows every account's backend cost, value and gain alongside the aggregate and opens its account", () => {
  const view = show({ instrumentId: "fund-1" });
  const rows = within(screen.getByRole("list", { name: "Accounts" })).getAllByRole("listitem");
  expect(rows).toHaveLength(2);
  expect(rows[0]).toHaveTextContent("US$40.00");
  expect(rows[0]).toHaveTextContent("US$48.00");
  expect(rows[1]).toHaveTextContent("US$60.00");
  expect(rows[1]).toHaveTextContent("US$72.00");
  expect(screen.getByText("US$100.00")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Open account: Broker A" }));
  expect(view.open).toHaveBeenCalledWith({ page: "accounts", accountId: "broker-a" });
});

it("keeps a single zero-quantity account visible and leaves missing valuation unavailable", () => {
  q.groups = [{ key: "fund", instruments: [{ ...holdings, holdings: [{ ...holdings.holdings[0], amounts: { quantity: "0", totalCost: { amount: "0", currency: "JPY" }, valueMissingReason: "missing_quote" } }] }], subtotals: [] }];
  show({ instrumentId: "fund-1" });
  const rows = within(screen.getByRole("list", { name: "Accounts" })).getAllByRole("listitem");
  expect(rows).toHaveLength(1);
  expect(rows[0]).toHaveTextContent("Zero quantity / closed position");
  expect(rows[0]).toHaveTextContent("¥0");
  expect(within(rows[0]).getAllByText("No current value")).toHaveLength(2);
});

it.each(["0", "0.001", "0.005", "-0.001", "-0.005"])("only labels exact zero as closed for quantity %s", quantity => {
  q.groups = [{ key: "fund", instruments: [{ ...holdings, holdings: [{ ...holdings.holdings[0], amounts: { ...holdings.holdings[0].amounts, quantity } }] }], subtotals: [] }];
  show({ instrumentId: "fund-1" });
  const list = screen.getByRole("list", { name: "Accounts" });
  expect(within(list).getByText(quantity, { exact: true })).toBeVisible();
  expect(within(list).queryByText(/Zero quantity \/ closed position/)).toBe(quantity === "0" ? within(list).getByText(/Zero quantity \/ closed position/) : null);
});

it("shows backend account figures without replacing them with household totals, and preserves scope in History", () => {
  const context = { instrumentId: "fund-1", accountId: "broker-a", from: "2026-01-01", to: "2026-09-30" };
  const view = show(context);
  expect(screen.getByText("Account: Broker A")).toBeInTheDocument();
  expect(screen.getByText("US$40.00")).toBeInTheDocument();
  expect(screen.queryByText("US$100.00")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Related transactions" }));
  expect(view.open).toHaveBeenCalledWith({ page: "history", filters: { instrumentId: "fund-1", accountId: "broker-a", from: "2026-01-01", to: "2026-09-30" } });
  view.rerender({ instrumentId: "fund-1", section: "holdings" });
  expect(screen.getByText("US$100.00")).toBeInTheDocument();
  expect(q.listInstruments).toHaveBeenCalledWith(true);
  expect(q.listHoldingGroups).toHaveBeenCalledWith(true, "broker-a");
  expect(q.listHoldingGroups).toHaveBeenCalledWith(true, "");
});

it("queries an archived account's own holding groups instead of the active-household projection", () => {
  q.groups = [];
  q.accountGroups["archived-broker"] = [{ key: "fund", instruments: [{ ...holdings,
    amounts: { quantity: "3", totalCost: money("300"), currentValue: money("330"), unrealizedGain: money("30") },
    holdings: [{ holdingId: "old-holding", accountId: "archived-broker", accountName: "Old Broker",
      amounts: { quantity: "3", totalCost: money("300"), currentValue: money("330"), unrealizedGain: money("30") } }],
  }], subtotals: [] }];
  const view = show({ instrumentId: "fund-1", accountId: "archived-broker" });
  expect(q.listHoldingGroups).toHaveBeenCalledWith(true, "archived-broker");
  expect(screen.getByText("Account: Old Broker")).toBeInTheDocument();
  expect(screen.getByText("US$300.00")).toBeInTheDocument();
  expect(screen.queryByText("No recorded holding in this scope.")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Related transactions" }));
  expect(view.open).toHaveBeenCalledWith({ page: "history", filters: { instrumentId: "fund-1", accountId: "archived-broker", from: undefined, to: undefined } });
});

it("opens archived and unowned records with source dates, while keeping maintenance focused", () => {
  q.instruments = [{ ...instrument, archivedAt: "2026-09-30" }];
  q.groups = [];
  q.quote = { unitPrice: "9.50", currency: "USD", quotedAt: "2026-09-30T12:00:00Z", sourceKind: "manual", sourceKey: "manual" };
  const view = show({ instrumentId: "fund-1", section: "data", from: "2026-09-01", to: "2026-09-30", healthFocus: { id: "gap-1", instrumentId: "fund-1", reason: "missing_history" } });
  expect(screen.getByText("Archived instrument · read only")).toBeInTheDocument();
  expect(screen.getByTestId("quote-records")).toHaveTextContent('"rangeStart":"2026-09-01"');
  expect(screen.getByTestId("quote-records")).toHaveTextContent('"rangeEnd":"2026-09-30"');
  fireEvent.click(screen.getByRole("button", { name: "Maintain data" }));
  expect(view.open).toHaveBeenCalledWith({ page: "market-data", focus: expect.objectContaining({ id: "gap-1", instrumentId: "fund-1", rangeStart: "2026-09-01", rangeEnd: "2026-09-30", action: "instrument_editor" }) });
});

it("writes edited quote dates into shared detail context for maintenance and scoped History", () => {
  const view = show({ instrumentId: "fund-1", section: "data", healthFocus: { id: "gap-1" } });
  fireEvent.click(screen.getByRole("button", { name: "Select quote period" }));
  const updated = view.onContextChange.mock.lastCall?.[0] as InstrumentDetailContext;
  expect(updated).toEqual(expect.objectContaining({ from: "2026-07-01", to: "2026-07-31",
    healthFocus: expect.objectContaining({ id: "gap-1", rangeStart: "2026-07-01", rangeEnd: "2026-07-31" }) }));
  view.rerender(updated);
  fireEvent.click(screen.getByRole("button", { name: "Maintain data" }));
  expect(view.open).toHaveBeenCalledWith({ page: "market-data", focus: expect.objectContaining({ rangeStart: "2026-07-01", rangeEnd: "2026-07-31" }) });
  view.rerender({ ...updated, section: "holdings" });
  fireEvent.click(screen.getByRole("button", { name: "Related transactions" }));
  expect(view.open).toHaveBeenCalledWith({ page: "history", filters: { instrumentId: "fund-1", accountId: undefined, from: "2026-07-01", to: "2026-07-31" } });
});

it("routes a product-owned instrument to its contract instead of presenting a market holding", () => {
  q.instruments = [{ ...instrument, managedProductId: "contract-1" }];
  q.groups = [];
  const view = show({ instrumentId: "fund-1" });
  expect(screen.getByText(/managed by a product contract/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Open available funds" }));
  expect(view.open).toHaveBeenCalledWith({ page: "available-funds", productId: "contract-1" });
  view.rerender({ instrumentId: "fund-1", section: "data" });
  expect(screen.queryByRole("region", { name: "Latest quote" })).not.toBeInTheDocument();
  expect(screen.queryByTestId("quote-records")).not.toBeInTheDocument();
});
