import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { InvestmentsPage } from "./InvestmentsPage";

const listInstruments = vi.fn();
const listAccounts = vi.fn();
const createHolding = vi.fn();
const holdingsByAccounts = vi.fn();
const accountGains = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: (...args: unknown[]) => listInstruments(...args),
    CreateInstrument: vi.fn(),
    UpdateInstrument: vi.fn(),
    ArchiveInstrument: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args),
    CreateHolding: (...args: unknown[]) => createHolding(...args),
    ArchiveHolding: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({
  Service: { AccountGains: (...args: unknown[]) => accountGains(...args), AccountGain: vi.fn(), RealizedGain: vi.fn(), HoldingGain: vi.fn() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    AppendManualInstrumentQuote: vi.fn(),
    CurrentInstrumentQuote: vi.fn().mockResolvedValue(null),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: (...args: unknown[]) => listAccounts(...args) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: {
    Bootstrap: () =>
      Promise.resolve({
        household: { id: "h1", name: "Test", baseCurrency: "USD", createdAt: "", updatedAt: "" },
        members: [],
        institutions: [],
        groups: [],
      }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { SupportedCurrencies: () => Promise.resolve(["USD"]) },
}));

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <InvestmentsPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listInstruments.mockReset();
  listAccounts.mockReset();
  createHolding.mockReset();
  holdingsByAccounts.mockReset();
  accountGains.mockReset();
  listInstruments.mockResolvedValue([]);
  listAccounts.mockResolvedValue([
    { account: { id: "acc-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
  ]);
  holdingsByAccounts.mockResolvedValue({ "acc-1": [] });
  accountGains.mockResolvedValue([{ accountId: "acc-1", holdings: [], available: true }]);
  createHolding.mockResolvedValue({ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" });
});

describe("InvestmentsPage", () => {
  it("does not duplicate instrument management on the holdings page", async () => {
    renderPage();
    expect(await screen.findByText(/Instrument identity and quotes live on Market Data/i)).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Instruments" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add instrument" })).not.toBeInTheDocument();
  });

  it("creates a Holding for a Holdings-mode Account", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add holding" }));

    await screen.findByLabelText("Account");
    await userEvent.selectOptions(screen.getByLabelText("Account"), "acc-1");
    await userEvent.selectOptions(await screen.findByLabelText("Instrument"), "i1");
    await userEvent.type(screen.getByLabelText("Quantity"), "10");
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));

    expect(createHolding).toHaveBeenCalledWith({ accountId: "acc-1", instrumentId: "i1", quantity: "10" });
  });

  it("does not offer cash-only multi-currency accounts for investment holdings", async () => {
    listAccounts.mockResolvedValue([
      { account: { id: "cash-1", name: "Travel cash", accountType: "cash_on_hand", trackingMode: "holdings" }, ownership: [], latestValue: null },
    ]);
    renderPage();

    expect(await screen.findByText("No investment account available")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add holding" })).not.toBeInTheDocument();
  });

  it("shows per-Holding cost/gain columns from AnalyticsService.HoldingGain data", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    holdingsByAccounts.mockResolvedValue({ "acc-1": [{ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" }] });
    accountGains.mockResolvedValue([{
      accountId: "acc-1",
      available: true,
      holdings: [
        {
          holdingId: "h1",
          accountId: "acc-1",
          instrumentId: "i1",
          instrumentName: "NVIDIA",
          quantity: "10",
          averageCost: { amount: "100", currency: "USD" },
          totalCost: { amount: "1000", currency: "USD" },
          currentValue: { amount: "1500", currency: "USD" },
          realizedGain: { amount: "0", currency: "USD" },
          unrealizedGain: { amount: "500", currency: "USD" },
          available: true,
        },
      ],
    }]);

    renderPage();

    expect(await screen.findByRole("columnheader", { name: "Cost" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Current value" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Unrealized gain" })).toBeInTheDocument();
    const table = screen.getByTestId("holdings-table");
    expect(table).toHaveTextContent("$1,000.00");
    expect(table).toHaveTextContent("$1,500.00");
    expect(table).toHaveTextContent("$500.00");
  });

  it("sorts the holdings index by numeric values, names, and missing market values", async () => {
    listInstruments.mockResolvedValue([
      { id: "i-missing", name: "No Price Fund", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "i-high", name: "Beta Fund", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "i-low", name: "Alpha Fund", quoteCurrency: "USD", quoteSource: "manual" },
    ]);
    holdingsByAccounts.mockResolvedValue({
      "acc-1": [
        { id: "h-missing", accountId: "acc-1", instrumentId: "i-missing", quantity: "5" },
        { id: "h-high", accountId: "acc-1", instrumentId: "i-high", quantity: "1" },
        { id: "h-low", accountId: "acc-1", instrumentId: "i-low", quantity: "2" },
      ],
    });
    accountGains.mockResolvedValue([{
      accountId: "acc-1",
      available: false,
      holdings: [
        {
          holdingId: "h-missing",
          accountId: "acc-1",
          instrumentId: "i-missing",
          instrumentName: "No Price Fund",
          quantity: "5",
          averageCost: { amount: "2", currency: "USD" },
          totalCost: { amount: "10", currency: "USD" },
          realizedGain: { amount: "0", currency: "USD" },
          available: false,
          missingReason: "current instrument price is unavailable",
        },
        {
          holdingId: "h-high",
          accountId: "acc-1",
          instrumentId: "i-high",
          instrumentName: "Beta Fund",
          quantity: "1",
          averageCost: { amount: "100", currency: "USD" },
          totalCost: { amount: "100", currency: "USD" },
          currentValue: { amount: "100", currency: "USD" },
          realizedGain: { amount: "0", currency: "USD" },
          unrealizedGain: { amount: "0", currency: "USD" },
          available: true,
        },
        {
          holdingId: "h-low",
          accountId: "acc-1",
          instrumentId: "i-low",
          instrumentName: "Alpha Fund",
          quantity: "2",
          averageCost: { amount: "1", currency: "USD" },
          totalCost: { amount: "2", currency: "USD" },
          currentValue: { amount: "20", currency: "USD" },
          realizedGain: { amount: "0", currency: "USD" },
          unrealizedGain: { amount: "18", currency: "USD" },
          available: true,
        },
      ],
    }]);

    renderPage();
    const table = await screen.findByTestId("holdings-table");
    const rowNames = () => Array.from(table.querySelectorAll("tbody tr")).map((row) => row.querySelector("td")?.textContent?.trim());

    await waitFor(() => expect(rowNames()).toEqual(["Beta Fund", "Alpha Fund", "No Price Fund"]));

    await userEvent.click(within(table).getByRole("button", { name: "Current value" }));
    await waitFor(() => expect(rowNames()).toEqual(["Alpha Fund", "Beta Fund", "No Price Fund"]));

    await userEvent.click(within(table).getByRole("button", { name: "Cost" }));
    await waitFor(() => expect(rowNames()).toEqual(["Alpha Fund", "No Price Fund", "Beta Fund"]));

    await userEvent.click(within(table).getByRole("button", { name: "Instrument" }));
    await waitFor(() => expect(rowNames()).toEqual(["Alpha Fund", "Beta Fund", "No Price Fund"]));
  });

  it("shows an unavailable badge when a Holding's gain cannot be computed", async () => {
    holdingsByAccounts.mockResolvedValue({ "acc-1": [{ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" }] });
    accountGains.mockResolvedValue([{
      accountId: "acc-1",
      available: false,
      holdings: [
        {
          holdingId: "h1",
          accountId: "acc-1",
          instrumentId: "i1",
          instrumentName: "NVIDIA",
          quantity: "10",
          averageCost: { amount: "100", currency: "USD" },
          totalCost: { amount: "1000", currency: "USD" },
          realizedGain: { amount: "0", currency: "USD" },
          available: false,
          missingReason: "current instrument price is unavailable",
        },
      ],
    }]);

    renderPage();

    expect(await screen.findByText("No current price")).toBeInTheDocument();
  });

  it("shows a validation error when Add holding is submitted empty", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add holding" }));
    await screen.findByLabelText("Account");
    const submitButtons = screen.getAllByRole("button", { name: "Add holding" });
    await userEvent.click(submitButtons[submitButtons.length - 1]);
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose an account, instrument, and quantity.");
    expect(createHolding).not.toHaveBeenCalled();
  });
});
