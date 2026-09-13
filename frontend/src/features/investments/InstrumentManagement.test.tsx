import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { InstrumentManagement } from "./InstrumentManagement";
import { TEST_CATALOG, selectValues } from "@/test/catalog";

const listInstruments = vi.fn();
const createInstrument = vi.fn();
const updateInstrument = vi.fn();
const archiveInstrument = vi.fn().mockResolvedValue(undefined);
const currentInstrumentQuote = vi.fn();
const appendManualQuote = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: (...args: unknown[]) => listInstruments(...args),
    CreateInstrument: (...args: unknown[]) => createInstrument(...args),
    UpdateInstrument: (...args: unknown[]) => updateInstrument(...args),
    ArchiveInstrument: (...args: unknown[]) => archiveInstrument(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    AppendManualInstrumentQuote: (...args: unknown[]) => appendManualQuote(...args),
    CurrentInstrumentQuote: (...args: unknown[]) => currentInstrumentQuote(...args),
  },
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
  Service: {
    SupportedCurrencies: () => Promise.resolve(["USD"]),
    Load: () => Promise.resolve({ timezone: "UTC" }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

function renderManagement() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <InstrumentManagement pageId="market-data" active />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listInstruments.mockReset();
  createInstrument.mockReset();
  updateInstrument.mockReset();
  archiveInstrument.mockClear();
  appendManualQuote.mockReset();
  currentInstrumentQuote.mockReset();
  appendManualQuote.mockResolvedValue({ id: "q1", instrumentId: "i1", unitPrice: "131.70" });
  currentInstrumentQuote.mockResolvedValue(null);
  listInstruments.mockResolvedValue([]);
  createInstrument.mockResolvedValue({ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" });
  updateInstrument.mockResolvedValue({ id: "i1", name: "NVIDIA Corp", quoteCurrency: "USD", quoteSource: "manual" });
});

describe("InstrumentManagement", () => {
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

    renderManagement();

    expect(await screen.findByText("Latest: $131.70")).toBeInTheDocument();
  });

  it("groups instruments by type and sorts by currency then name", async () => {
    listInstruments.mockResolvedValue([
      { id: "etf-1", name: "QQQ", type: "etf", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "stock-usd", name: "Apple", type: "stock", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "stock-cny", name: "Kweichow", type: "stock", quoteCurrency: "CNY", quoteSource: "manual" },
    ]);

    renderManagement();
    const stockGroup = await screen.findByTestId("instrument-type-group-stock");
    const etfGroup = screen.getByTestId("instrument-type-group-etf");
    expect(stockGroup.compareDocumentPosition(etfGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(stockGroup).getByText("Kweichow").compareDocumentPosition(within(stockGroup).getByText("Apple")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("creates an Instrument with manual quote source", async () => {
    renderManagement();
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
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    const name = within(form).getByLabelText("Name");
    await userEvent.clear(name);
    await userEvent.type(name, "NVIDIA Corp");
    const symbol = within(form).getByLabelText("Security symbol");
    await userEvent.clear(symbol);
    await userEvent.type(symbol, "NVDA.O");
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "SG");
    await userEvent.selectOptions(within(form).getByLabelText("Trading market"), "SGX");
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

  it("archives an Instrument after confirming the AlertDialog", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderManagement();
    await screen.findByText("NVIDIA");
    await userEvent.click(screen.getByRole("button", { name: "Archive" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
    expect(archiveInstrument).toHaveBeenCalledWith("i1", true);
  });

  it("saves a manual instrument quote from Set price", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderManagement();
    await screen.findByText("NVIDIA");
    expect(screen.queryByRole("button", { name: "Icon" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Set price" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
    await userEvent.click(await screen.findByRole("button", { name: "Set price" }));
    await userEvent.type(await screen.findByLabelText("Unit price"), "131.70");
    const priceButtons = screen.getAllByRole("button", { name: "Set price" });
    await userEvent.click(priceButtons[priceButtons.length - 1]);
    expect(appendManualQuote).toHaveBeenCalledWith("i1", "131.70", "", false);
  });

  it("keeps manual quote entry available without changing a provider preference", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "provider" }]);
    renderManagement();
    await screen.findByText("NVIDIA");
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(await screen.findByRole("button", { name: "Set price" })).toBeInTheDocument();
  });

  it("renders provider key options from the catalog when quote source is provider", async () => {
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.selectOptions(within(form).getByLabelText("Quote source"), "provider");
    await waitFor(() => {
      expect(selectValues(within(form).getByLabelText("Provider key"))).toEqual(TEST_CATALOG.instrumentProviders);
    });
    expect(within(form).queryByPlaceholderText(/yahoo_finance/i)).not.toBeInTheDocument();
  });

  it("submits the catalog provider key when quote source is provider", async () => {
    renderManagement();
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
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await waitFor(() => {
      expect(selectValues(within(form).getByLabelText("Type"))).toEqual(TEST_CATALOG.instrumentTypes);
    });
  });

  it("places name, then type, then icon, and labels markets as code plus short name", async () => {
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    const name = await within(form).findByLabelText("Name");
    const type = within(form).getByLabelText("Type");
    const icon = within(form).getByLabelText("Choose icon");
    expect(name.compareDocumentPosition(type) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(type.compareDocumentPosition(icon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(form).getByRole("option", { name: "NASDAQ - Nasdaq" })).toBeInTheDocument();
    expect(within(form).getByRole("option", { name: "NYSE - New York" })).toBeInTheDocument();
    expect(within(form).getByRole("option", { name: "HKEX - Hong Kong" })).toBeInTheDocument();
  });

  it("filters markets by country and fills country when a market is chosen", async () => {
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await waitFor(() => expect(within(form).getByLabelText("Trading market")).toBeEnabled());
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "US");
    expect(selectValues(within(form).getByLabelText("Trading market"))).toEqual(["", "NASDAQ", "NYSE", "AMEX"]);
    await userEvent.selectOptions(within(form).getByLabelText("Trading market"), "NASDAQ");
    expect(within(form).getByLabelText("Country / region")).toHaveValue("US");
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "CN");
    expect(within(form).getByLabelText("Trading market")).toHaveValue("");
    expect(selectValues(within(form).getByLabelText("Trading market"))).toEqual(["", "SSE", "SZSE", "BSE"]);
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "");
    await userEvent.selectOptions(within(form).getByLabelText("Trading market"), "HKEX");
    expect(within(form).getByLabelText("Country / region")).toHaveValue("HK");
  });
});
