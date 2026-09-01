import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { InvestmentsPage } from "./InvestmentsPage";

const listInstruments = vi.fn();
const createInstrument = vi.fn();
const updateInstrument = vi.fn();
const archiveInstrument = vi.fn().mockResolvedValue(undefined);
const listAccounts = vi.fn();
const createHolding = vi.fn();
const holdingsByAccounts = vi.fn();
const accountGain = vi.fn();
const currentInstrumentQuote = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: (...args: unknown[]) => listInstruments(...args),
    CreateInstrument: (...args: unknown[]) => createInstrument(...args),
    UpdateInstrument: (...args: unknown[]) => updateInstrument(...args),
    ArchiveInstrument: (...args: unknown[]) => archiveInstrument(...args),
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
  Service: { AccountGain: (...args: unknown[]) => accountGain(...args), RealizedGain: vi.fn(), HoldingGain: vi.fn() },
}));
const saveManualQuote = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    SaveManualInstrumentQuote: (...args: unknown[]) => saveManualQuote(...args),
    CurrentInstrumentQuote: (...args: unknown[]) => currentInstrumentQuote(...args),
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
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

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
  createInstrument.mockReset();
  updateInstrument.mockReset();
  archiveInstrument.mockClear();
  listAccounts.mockReset();
  createHolding.mockReset();
  holdingsByAccounts.mockReset();
  accountGain.mockReset();
  saveManualQuote.mockReset();
  currentInstrumentQuote.mockReset();
  saveManualQuote.mockResolvedValue({ id: "q1", instrumentId: "i1", unitPrice: "131.70" });
  currentInstrumentQuote.mockResolvedValue(null);

  listInstruments.mockResolvedValue([]);
  createInstrument.mockResolvedValue({ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" });
  updateInstrument.mockResolvedValue({ id: "i1", name: "NVIDIA Corp", quoteCurrency: "USD", quoteSource: "manual" });
  listAccounts.mockResolvedValue([
    { account: { id: "acc-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
  ]);
  holdingsByAccounts.mockResolvedValue({ "acc-1": [] });
  accountGain.mockResolvedValue({ accountId: "acc-1", holdings: [], available: true });
  createHolding.mockResolvedValue({ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" });
});

describe("InvestmentsPage", () => {
  it("shows the latest saved instrument price in the instrument list", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "provider" }]);
    currentInstrumentQuote.mockResolvedValue({
      id: "q1",
      instrumentId: "i1",
      unitPrice: "131.70",
      currency: "USD",
      sourceKind: "provider",
      sourceKey: "yahoo_finance",
      quotedAt: "2024-01-01T00:00:00Z",
      createdAt: "2024-01-01T00:00:00Z",
      delayed: true,
    });

    renderPage();

    expect(await screen.findByText("Latest: $131.70")).toBeInTheDocument();
  });

  it("groups instruments by type and sorts by currency then name", async () => {
    listInstruments.mockResolvedValue([
      { id: "etf-1", name: "QQQ", type: "etf", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "stock-usd", name: "Apple", type: "stock", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "stock-cny", name: "Kweichow", type: "stock", quoteCurrency: "CNY", quoteSource: "manual" },
    ]);

    renderPage();
    const stockGroup = await screen.findByTestId("instrument-type-group-stock");
    const etfGroup = screen.getByTestId("instrument-type-group-etf");
    expect(stockGroup.compareDocumentPosition(etfGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(stockGroup).getByText("Kweichow").compareDocumentPosition(within(stockGroup).getByText("Apple")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("creates an Instrument with manual quote source", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.type(within(form).getByLabelText("Name"), "NVIDIA");
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));

    expect(createInstrument).toHaveBeenCalledWith(
      expect.objectContaining({ name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }),
    );
  });

  it("edits all Instrument identity fields", async () => {
    listInstruments.mockResolvedValue([{
      id: "i1",
      householdId: "h1",
      name: "NVIDIA",
      type: "stock",
      quoteCurrency: "USD",
      symbol: "NVDA",
      marketCode: "NASDAQ",
      countryCode: "US",
      isin: "US67066G1040",
      note: "Old note",
      iconKey: "stock",
      sortOrder: 0,
      quoteSource: "manual",
      createdAt: "",
      updatedAt: "",
    }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    const name = within(form).getByLabelText("Name");
    await userEvent.clear(name);
    await userEvent.type(name, "NVIDIA Corp");
    const symbol = within(form).getByLabelText("Security symbol");
    await userEvent.clear(symbol);
    await userEvent.type(symbol, "NVDA.O");
    await userEvent.selectOptions(within(form).getByLabelText("Trading market"), "SGX");
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "SG");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));

    expect(updateInstrument).toHaveBeenCalledWith("i1", expect.objectContaining({
      replace: true,
      name: "NVIDIA Corp",
      symbol: "NVDA.O",
      marketCode: "SGX",
      countryCode: "SG",
      isin: "US67066G1040",
      note: "Old note",
    }));
  });

  it("creates a Holding for a Holdings-mode Account", async () => {
    renderPage();
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    await userEvent.click(screen.getByRole("tab", { name: "All holdings index" }));
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));

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
    await userEvent.click(screen.getByRole("tab", { name: "All holdings index" }));

    expect(await screen.findByText("No investment account available")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add holding" })).not.toBeInTheDocument();
  });

  it("shows per-Holding cost/gain columns from AnalyticsService.HoldingGain data", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    holdingsByAccounts.mockResolvedValue({ "acc-1": [{ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" }] });
    accountGain.mockResolvedValue({
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
    });

    renderPage();
    await userEvent.click(screen.getByRole("tab", { name: "All holdings index" }));

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
    accountGain.mockResolvedValue({
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
    });

    renderPage();
    await userEvent.click(screen.getByRole("tab", { name: "All holdings index" }));
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
    accountGain.mockResolvedValue({
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
    });

    renderPage();
    await userEvent.click(screen.getByRole("tab", { name: "All holdings index" }));

    expect(await screen.findByText("No current price")).toBeInTheDocument();
  });

  it("archives an Instrument after confirming the AlertDialog", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await screen.findByText("NVIDIA");
    await userEvent.click(screen.getByRole("button", { name: "Archive" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
    expect(archiveInstrument).toHaveBeenCalledWith("i1", true);
  });

  it("saves a manual instrument quote from Set price", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await screen.findByText("NVIDIA");
    expect(screen.queryByRole("button", { name: "Icon" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Set price" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
    await userEvent.click(await screen.findByRole("button", { name: "Set price" }));
    await userEvent.type(await screen.findByLabelText("Unit price"), "131.70");
    const priceButtons = screen.getAllByRole("button", { name: "Set price" });
    await userEvent.click(priceButtons[priceButtons.length - 1]);
    expect(saveManualQuote).toHaveBeenCalledWith("i1", "131.70", "");
  });

  it("shows a validation error when Add holding is submitted empty", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await userEvent.click(screen.getByRole("tab", { name: "All holdings index" }));
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));
    await screen.findByLabelText("Account");
    const submitButtons = screen.getAllByRole("button", { name: "Add holding" });
    await userEvent.click(submitButtons[submitButtons.length - 1]);
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose an account, instrument, and quantity.");
    expect(createHolding).not.toHaveBeenCalled();
  });

  it("renders provider key options from the catalog when quote source is provider", async () => {
    const { TEST_CATALOG, selectValues } = await import("@/test/catalog");
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.selectOptions(within(form).getByLabelText("Quote source"), "provider");
    await waitFor(() => {
      expect(selectValues(within(form).getByLabelText("Provider key"))).toEqual(TEST_CATALOG.instrumentProviders);
    });
    expect(within(form).queryByPlaceholderText(/yahoo_finance/i)).not.toBeInTheDocument();
  });

  it("submits the catalog provider key when quote source is provider", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.type(within(form).getByLabelText("Name"), "NVIDIA");
    await userEvent.selectOptions(within(form).getByLabelText("Quote source"), "provider");
    await userEvent.type(within(form).getByLabelText("Quote lookup symbol"), "NVDA");
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));

    expect(createInstrument).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "NVIDIA",
        quoteSource: "provider",
        providerKey: "yahoo_finance",
        providerSymbol: "NVDA",
      }),
    );
  });

  it("renders instrument type options from the catalog only", async () => {
    const { TEST_CATALOG, selectValues } = await import("@/test/catalog");
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await waitFor(() => {
      expect(selectValues(within(form).getByLabelText("Type"))).toEqual(TEST_CATALOG.instrumentTypes);
    });
  });
});
