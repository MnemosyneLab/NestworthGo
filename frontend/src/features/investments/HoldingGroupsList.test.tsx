import { beforeEach, describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { HoldingGroupsList } from "./HoldingGroupsList";
import { NavigationContext } from "@/app/NavigationContext";
import { useHoldingGroupStore } from "@/stores/holdings";
import type { HoldingAmountsDTO, InstrumentHoldingGroupDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: () => Promise.resolve(null) } }));
vi.mock("./HoldingDetailPanel", () => ({ HoldingDetailPanel: () => <div>Holding details</div> }));

const amounts = (quantity: string, value: string, currency = "USD", gain = "-10"): HoldingAmountsDTO => ({ quantity, totalCost: { amount: "100", currency }, currentValue: { amount: value, currency }, unrealizedGain: { amount: gain, currency } });
function apple(): InstrumentHoldingsDTO {
  return { instrumentId: "i1", type: "stock", name: "Apple", symbol: "AAPL", quoteCurrency: "USD", quantityUnit: "", archived: false,
    amounts: amounts("3", "900"), holdings: [
      { holdingId: "h1", accountId: "a1", accountName: "Broker A", amounts: amounts("1", "300") },
      { holdingId: "h2", accountId: "a2", accountName: "Broker B", amounts: amounts("2", "600") },
      { holdingId: "h0", accountId: "a0", accountName: "Closed account position", amounts: amounts("0", "0") },
    ] };
}
const subtotal = (currency = "USD") => ({ currency, instrumentCount: 1, totalCost: { amount: "1,000".replace(",", ""), currency }, currentValue: { amount: "1200", currency }, unrealizedGain: { amount: "200", currency } });
const stockGroup = (...instruments: InstrumentHoldingsDTO[]): InstrumentHoldingGroupDTO => ({ key: "stock", instruments, subtotals: [subtotal()] });

beforeEach(() => useHoldingGroupStore.getState().reset());

describe("HoldingGroupsList", () => {
  it("shows a labelled group header with count and per-currency subtotals", () => {
    const gold: InstrumentHoldingsDTO = { ...apple(), instrumentId: "au", type: "precious_metal", name: "Gold", symbol: "XAU", quantityUnit: "g", holdings: [apple().holdings![0]] };
    render(<HoldingGroupsList scope="portfolio" groups={[stockGroup(apple()), { key: "precious_metal", instruments: [gold], subtotals: [subtotal("USD"), subtotal("CNY")] }]} />);
    const stocks = screen.getByRole("listitem", { name: "Stocks" });
    expect(within(stocks).getByText("1 holding")).toBeInTheDocument();
    expect(within(stocks).getByText("$1,000.00")).toBeInTheDocument();
    expect(within(stocks).getByText("$1,200.00")).toBeInTheDocument();
    expect(within(stocks).getByText("+$200.00")).toBeInTheDocument();
    const metals = screen.getByRole("listitem", { name: "Precious metals" });
    expect(within(metals).getAllByText(/^\+/)).toHaveLength(2);
    expect(screen.getAllByRole("listitem").map((item) => item.getAttribute("data-group"))).toEqual(["stock", "precious_metal"]);
  });

  it("collapses a group from the header, keeps the choice across remounts, and offers collapse all", async () => {
    const groups = [stockGroup(apple()), { key: "crypto", instruments: [{ ...apple(), instrumentId: "btc", type: "crypto", name: "Bitcoin", symbol: "BTC" }], subtotals: [subtotal()] }];
    const view = render(<HoldingGroupsList scope="portfolio" groups={groups} />);
    const toggle = screen.getByRole("button", { name: "Collapse Stocks" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    toggle.focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Expand Stocks" })).toHaveAttribute("aria-expanded", "false");
    expect(within(screen.getByRole("listitem", { name: "Stocks" })).queryByTestId("holdings-table")).not.toBeInTheDocument();
    expect(within(screen.getByRole("listitem", { name: "Crypto" })).getByTestId("holdings-table")).toBeInTheDocument();
    view.unmount();
    render(<HoldingGroupsList scope="portfolio" groups={groups} />);
    expect(screen.getByRole("button", { name: "Expand Stocks" })).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    expect(screen.getByRole("button", { name: "Expand Crypto" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Expand all" }));
    expect(screen.getByRole("button", { name: "Collapse Crypto" })).toBeInTheDocument();
  });

  it("remembers collapsed groups separately for Portfolio and Accounts", async () => {
    render(<HoldingGroupsList scope="portfolio" groups={[stockGroup(apple())]} />);
    await userEvent.click(screen.getByRole("button", { name: "Collapse Stocks" }));
    expect(useHoldingGroupStore.getState().collapsed).toEqual({ portfolio: ["stock"], account: [] });
  });

  it("shows authoritative totals, expands accounts by keyboard, and navigates to the account", async () => {
    const onOpenAccount = vi.fn();
    render(<HoldingGroupsList scope="portfolio" groups={[stockGroup(apple())]} onOpenAccount={onOpenAccount} />);
    expect(screen.getByRole("button", { name: "By instrument" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("2 accounts")).toBeInTheDocument();
    expect(screen.getByText("$900.00")).toBeInTheDocument();
    const expand = screen.getByRole("button", { name: "Show details for Apple" });
    expand.focus();
    await userEvent.keyboard("{Enter}");
    expect(expand).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("$300.00")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Broker B" }));
    expect(onOpenAccount).toHaveBeenCalledWith("a2");
    await userEvent.click(screen.getByRole("button", { name: "Account details" }));
    expect(screen.queryByText("$900.00")).not.toBeInTheDocument();
    expect(screen.queryByText("Closed account position")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("checkbox", { name: "Show zero holdings" }));
    expect(screen.getByRole("button", { name: "Closed account position" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "By instrument" }));
    expect(screen.getByText("3 accounts")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Hide details for Apple" })).toHaveAttribute("aria-expanded", "true");
  });

  it("opens the shared detail with the row's account scope", async () => {
    const open = vi.fn();
    render(<QueryClientProvider client={createTestQueryClient()}><NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><HoldingGroupsList scope="portfolio" groups={[stockGroup(apple())]} /></NavigationContext.Provider></QueryClientProvider>);
    await userEvent.click(screen.getByRole("button", { name: "Open AAPL details" }));
    expect(open).toHaveBeenLastCalledWith({ page: "instrument-detail", instrumentId: "i1", accountId: undefined, section: "holdings" });
    await userEvent.click(screen.getByRole("button", { name: "Account details" }));
    const brokerA = screen.getByRole("row", { name: /Broker A/ });
    await userEvent.click(within(brokerA).getByRole("button", { name: "Open AAPL details" }));
    expect(open).toHaveBeenLastCalledWith({ page: "instrument-detail", instrumentId: "i1", accountId: "a1", section: "holdings" });
  });

  it("marks losses with an arrow and spoken direction, not only color", () => {
    render(<HoldingGroupsList scope="portfolio" groups={[stockGroup(apple())]} />);
    const row = screen.getByRole("row", { name: /Apple/ });
    expect(within(row).getByText("Down")).toHaveClass("sr-only");
    expect(within(row).getByText("-$10.00")).toBeInTheDocument();
    expect(row.querySelector("svg.lucide-arrow-down-right")).not.toBeNull();
  });

  it("keeps an all-zero group accessible through the filter and labels archived metal units", async () => {
    const group = { ...apple(), type: "precious_metal", name: "Gold", archived: true, quantityUnit: "g", amounts: amounts("0", "0") };
    group.holdings = [apple().holdings![2]];
    render(<HoldingGroupsList scope="portfolio" groups={[{ key: "precious_metal", instruments: [group], subtotals: [] }]} />);
    expect(screen.queryByTestId("holdings-table")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("checkbox"));
    expect(screen.getByText("Archived")).toBeInTheDocument();
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("0 g");
  });

  it("keeps unavailable fields separate and exposes valid account values", async () => {
    const instrument = apple();
    instrument.amounts = { quantity: "3", totalCost: { amount: "200", currency: "USD" }, valueMissingReason: "current instrument price is unavailable", gainMissingReason: "current instrument price is unavailable" };
    render(<HoldingGroupsList scope="portfolio" groups={[{ key: "stock", instruments: [instrument], subtotals: [{ currency: "USD", instrumentCount: 1, totalCost: { amount: "200", currency: "USD" }, valueMissingReason: "current instrument price is unavailable", gainMissingReason: "current instrument price is unavailable" }] }]} />);
    expect(screen.getAllByText("$200.00")).toHaveLength(2);
    expect(screen.getAllByText("No current price")).toHaveLength(2);
    expect(screen.getAllByText(/· No current price/)).toHaveLength(2);
    await userEvent.click(screen.getByRole("button", { name: "Show details for Apple" }));
    expect(screen.getByText("$300.00")).toBeInTheDocument();
  });

  it("sorts amounts within currencies, keeps missing last, and does not merge identical names", async () => {
    const usd = { ...apple(), name: "Z USD" };
    const cny = { ...apple(), name: "A CNY", instrumentId: "cny", quoteCurrency: "CNY", amounts: amounts("3", "9999", "CNY") };
    const missing = { ...apple(), name: "Missing", instrumentId: "missing", amounts: { quantity: "3" } };
    const sameName = { ...apple(), name: "Z USD", instrumentId: "other", amounts: amounts("3", "1") };
    render(<HoldingGroupsList scope="portfolio" groups={[stockGroup(usd, cny, missing, sameName)]} />);
    const table = screen.getByTestId("holdings-table");
    const names = () => [...table.querySelectorAll("tbody tr")].map((r) => r.querySelector("td")?.textContent);
    expect(names().map((text) => text?.replace(/2 accounts$/, ""))).toEqual(["AAPLA CNY", "AAPLMissing", "AAPLZ USD", "AAPLZ USD"]);
    await userEvent.click(within(table).getByRole("button", { name: "Current value" }));
    expect(names().map((text) => text?.replace(/2 accounts$/, ""))).toEqual(["AAPLA CNY", "AAPLZ USD", "AAPLZ USD", "AAPLMissing"]);
    const values = () => [...table.querySelectorAll("tbody tr")].map((r) => r.children[3].textContent);
    expect(values()[1]).toBe("$1.00");
    await userEvent.click(within(table).getByRole("button", { name: "Current value" }));
    expect(values()[1]).toBe("$900.00");
    expect(names()[3]).toContain("Missing");
  });

  it("labels cash-like contract quantity as contracts, including an authoritative zero", async () => {
    const product = { ...apple(), instrumentId: "product-1", type: "other", name: "Term deposit", amounts: amounts("1", "100000"), holdings: [{ holdingId: "closed", accountId: "a", accountName: "Closed", amounts: amounts("0", "0") }, { holdingId: "open", accountId: "b", accountName: "Open", amounts: amounts("1", "100000") }] };
    render(<HoldingGroupsList scope="portfolio" groups={[{ key: "cash_like", instruments: [product], subtotals: [] }]} />);
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("Contracts: 1");
    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "Account details" }));
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("Contracts: 0");
  });

  it("renders the account scope with the same grouping, no portfolio controls and a dividend action", async () => {
    const onDividend = vi.fn();
    const contract = { ...apple(), instrumentId: "p", name: "Contract", type: "other" };
    render(<QueryClientProvider client={createTestQueryClient()}><HoldingGroupsList scope="account" accountId="a1" accountName="Broker A" groups={[stockGroup({ ...apple(), holdings: [apple().holdings![0]] }), { key: "cash_like", instruments: [contract], subtotals: [] }]} onDividend={onDividend} /></QueryClientProvider>);
    expect(screen.queryByRole("button", { name: "By instrument" })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(screen.queryByText("Contract")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show details for Apple" }));
    expect(screen.getByText("Holding details")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Collapse Stocks" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Actions for AAPL" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Cash dividend" }));
    expect(onDividend).toHaveBeenCalledWith("h1");
  });

  it("keeps closed account positions reachable without making them current sell candidates", async () => {
    const current = { ...apple(), holdings: [apple().holdings![0]] };
    const closed = { ...apple(), instrumentId: "i2", name: "Closed Fund", symbol: "CLSD", holdings: [{ ...apple().holdings![2], accountId: "a1", accountName: "Broker A" }] };
    render(<QueryClientProvider client={createTestQueryClient()}><HoldingGroupsList scope="account" accountId="a1" groups={[stockGroup(current, closed)]} onDividend={vi.fn()} /></QueryClientProvider>);
    expect(screen.getByRole("button", { name: "Current" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("Apple");
    expect(screen.getByTestId("holdings-table")).not.toHaveTextContent("Closed Fund");
    await userEvent.click(screen.getByRole("button", { name: "Closed" }));
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("Closed Fund");
    expect(screen.getByTestId("holdings-table")).not.toHaveTextContent("Apple");
    expect(screen.queryByRole("button", { name: "Actions for CLSD" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "All" }));
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("Apple");
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("Closed Fund");
    expect(screen.getByText("Totals exclude zero-quantity positions.")).toBeInTheDocument();
    expect(within(screen.getByRole("listitem", { name: "Stocks" })).getByText("2 holdings")).toBeInTheDocument();
    expect(within(screen.getByRole("listitem", { name: "Stocks" })).getByText("$1,200.00")).toBeInTheDocument();
  });
});


it.each(["portfolio", "account"] as const)("explains rounding by keyboard in the %s holdings surface without changing authoritative amounts", async (scope) => {
  const instrument = apple();
  instrument.amounts = amounts("2", "0.008", "USD", "-0.008");
  instrument.holdings = instrument.holdings!.slice(0, 2).map((holding) => ({ ...holding, amounts: amounts("1", "0.004", "USD", "-0.004") }));
  const group = stockGroup(instrument);
  group.subtotals = [{ ...subtotal(), currentValue: { amount: "0.008", currency: "USD" }, unrealizedGain: { amount: "-0.008", currency: "USD" } }];
  const before = JSON.stringify(group);
  render(<HoldingGroupsList scope={scope} accountId="a1" groups={[group]} />);
  expect(screen.getAllByText("$0.01").length).toBeGreaterThan(0);
  const help = screen.getByRole("button", { name: "About rounded amounts" });
  expect(screen.queryByText(/Totals use exact values before rounding/)).not.toBeInTheDocument();
  help.focus();
  await userEvent.keyboard("{Enter}");
  expect(await screen.findByText(/Totals use exact values before rounding/)).toBeVisible();
  await userEvent.keyboard("{Escape}");
  expect(help).toHaveFocus();
  expect(JSON.stringify(group)).toBe(before);
});
