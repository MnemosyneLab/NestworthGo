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
const searchInstruments = vi.fn();

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
    SupportedCurrencies: () => Promise.resolve(["USD", "CNY", "SGD"]),
    Load: () => Promise.resolve({ timezone: "UTC" }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    CoinGeckoQuoteCurrencies: () => Promise.resolve(["USD", "CNY", "SGD"]),
    SearchInstruments: (...args: unknown[]) => searchInstruments(...args),
  },
}));

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
  searchInstruments.mockReset();
  searchInstruments.mockResolvedValue([]);
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

  it("shows the ticker first and the legal name in smaller text", async () => {
    listInstruments.mockResolvedValue([{
      id: "i1",
      name: "Alpha Architect 1-3 Month Box ETF",
      symbol: "BOXX",
      type: "etf",
      quoteCurrency: "USD",
      quoteSource: "provider",
    }]);

    renderManagement();

    expect(await screen.findByText("BOXX")).toBeInTheDocument();
    expect(screen.getByText("Alpha Architect 1-3 Month Box ETF")).toBeInTheDocument();
    expect(screen.getByText("BOXX").className).toContain("font-medium");
    expect(screen.getByText("Alpha Architect 1-3 Month Box ETF").className).toContain("text-xs");
  });

  it("creates an Instrument with manual quote source", async () => {
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    expect(within(form).getByLabelText("Currency").tagName).toBe("SELECT");
    await userEvent.selectOptions(within(form).getByLabelText("Quote source"), "manual");
    await userEvent.type(within(form).getByLabelText("Name"), "NVIDIA");
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));

    expect(createInstrument).toHaveBeenCalledWith(
      expect.objectContaining({ name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }),
    );
  });

  it("edits Instrument identity while keeping currency read-only", async () => {
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
    expect(within(form).getByLabelText("Currency")).toHaveAttribute("readonly");
    expect(within(form).getByLabelText("Currency")).toHaveValue("USD");
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
      quoteCurrency: "USD",
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
    await userEvent.type(await screen.findByLabelText("Unit price (USD)"), "131.70");
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
      expect(selectValues(within(form).getByLabelText("Provider key"))).toEqual(TEST_CATALOG.instrumentProviders.filter((provider) => provider !== "coingecko"));
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
    expect(within(form).getByRole("option", { name: "ARCA - NYSE Arca" })).toBeInTheDocument();
    expect(within(form).getByRole("option", { name: "BZX - Cboe BZX" })).toBeInTheDocument();
    expect(within(form).getByRole("option", { name: "HKEX - Hong Kong" })).toBeInTheDocument();
  });

  it("filters markets by country and fills country when a market is chosen", async () => {
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await waitFor(() => expect(within(form).getByLabelText("Trading market")).toBeEnabled());
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "US");
    expect(selectValues(within(form).getByLabelText("Trading market"))).toEqual(["", "NASDAQ", "NYSE", "AMEX", "ARCA", "BZX", "EDGX", "IEX"]);
    await userEvent.selectOptions(within(form).getByLabelText("Trading market"), "NASDAQ");
    expect(within(form).getByLabelText("Country / region")).toHaveValue("US");
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "CN");
    expect(within(form).getByLabelText("Trading market")).toHaveValue("");
    expect(selectValues(within(form).getByLabelText("Trading market"))).toEqual(["", "SSE", "SZSE", "BSE"]);
    await userEvent.selectOptions(within(form).getByLabelText("Country / region"), "");
    await userEvent.selectOptions(within(form).getByLabelText("Trading market"), "HKEX");
    expect(within(form).getByLabelText("Country / region")).toHaveValue("HK");
  });

  it("defaults stock and ETF quote source to Yahoo provider", async () => {
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await waitFor(() => {
      expect(within(form).getByLabelText("Quote source")).toHaveValue("provider");
      expect(within(form).getByLabelText("Provider key")).toHaveValue("yahoo_finance");
    });
    expect(within(form).getByLabelText("Search Yahoo")).toBeInTheDocument();
    await userEvent.selectOptions(within(form).getByLabelText("Type"), "etf");
    await waitFor(() => {
      expect(within(form).getByLabelText("Quote source")).toHaveValue("provider");
      expect(within(form).getByLabelText("Provider key")).toHaveValue("yahoo_finance");
    });
    await userEvent.selectOptions(within(form).getByLabelText("Type"), "crypto");
    await waitFor(() => expect(within(form).getByLabelText("Quote source")).toHaveValue("provider"));
    expect(within(form).getByLabelText("Search cryptocurrency")).toBeInTheDocument();
    expect(within(form).queryByLabelText("Search Yahoo")).not.toBeInTheDocument();
  });

  it("keeps edit currency fixed when selecting a search result in another currency", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", type: "stock", quoteCurrency: "USD", quoteSource: "provider", providerKey: "yahoo_finance", providerSymbol: "NVDA" }]);
    searchInstruments.mockResolvedValue([{ providerKey: "yahoo_finance", providerSymbol: "NVDA", name: "NVIDIA Corporation", symbol: "NVDA", type: "stock", marketCode: "NASDAQ", countryCode: "US", quoteCurrency: "CNY", exchange: "NASDAQ" }]);
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.type(within(form).getByLabelText("Search Yahoo"), "NVDA");
    await userEvent.click(await within(form).findByRole("option", { name: "NVDA · NVIDIA Corporation · NASDAQ" }));
    expect(within(form).getByLabelText("Currency")).toHaveValue("USD");
    expect(within(form).getByLabelText("Currency")).toHaveAttribute("readonly");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    expect(updateInstrument).toHaveBeenCalledWith("i1", expect.objectContaining({ quoteCurrency: "USD", name: "NVIDIA Corporation" }));
  });

  it("selects CoinGecko coin IDs and a non-USD quote currency", async () => {
    searchInstruments.mockResolvedValue([{ providerKey: "coingecko", providerSymbol: "bitcoin", name: "Bitcoin", symbol: "BTC", type: "crypto", marketCode: "CRYPTO", quoteCurrency: "USD", exchange: "CoinGecko" }]);
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.selectOptions(within(form).getByLabelText("Type"), "crypto");
    await userEvent.type(within(form).getByLabelText("Search cryptocurrency"), "BTC");
    await userEvent.click(await within(form).findByRole("option", { name: "BTC · Bitcoin · bitcoin" }));
    expect(searchInstruments).toHaveBeenCalledWith("BTC", "crypto");
    expect(within(form).getByLabelText("Provider key")).toHaveValue("coingecko");
    expect(within(form).getByLabelText("Quote lookup symbol")).toHaveValue("bitcoin");
    await userEvent.selectOptions(within(form).getByLabelText("Currency"), "CNY");
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));
    await waitFor(() => expect(createInstrument).toHaveBeenCalledWith(expect.objectContaining({
      type: "crypto", name: "Bitcoin", symbol: "BTC", providerKey: "coingecko", providerSymbol: "bitcoin", quoteCurrency: "CNY",
    })));
  });

  it("fills identity fields from a Yahoo search result", async () => {
    searchInstruments.mockResolvedValue([{
      providerKey: "yahoo_finance",
      providerSymbol: "NVDA",
      name: "NVIDIA Corporation",
      symbol: "NVDA",
      type: "stock",
      marketCode: "NASDAQ",
      countryCode: "US",
      quoteCurrency: "USD",
      exchange: "NASDAQ",
    }]);
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.type(within(form).getByLabelText("Search Yahoo"), "NVDA");
    const option = await within(form).findByRole("option", { name: "NVDA · NVIDIA Corporation · NASDAQ" });
    await userEvent.click(option);
    expect(within(form).getByLabelText("Name")).toHaveValue("NVIDIA Corporation");
    expect(within(form).getByLabelText("Security symbol")).toHaveValue("NVDA");
    expect(within(form).getByLabelText("Country / region")).toHaveValue("US");
    expect(within(form).getByLabelText("Trading market")).toHaveValue("NASDAQ");
    expect(within(form).getByLabelText("Currency")).toHaveValue("USD");
    expect(within(form).getByLabelText("Quote source")).toHaveValue("provider");
    expect(within(form).getByLabelText("Quote lookup symbol")).toHaveValue("NVDA");
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));
    expect(createInstrument).toHaveBeenCalledWith(expect.objectContaining({
      name: "NVIDIA Corporation",
      type: "stock",
      symbol: "NVDA",
      marketCode: "NASDAQ",
      countryCode: "US",
      quoteCurrency: "USD",
      quoteSource: "provider",
      providerKey: "yahoo_finance",
      providerSymbol: "NVDA",
    }));
  });

  it("fills NYSE Arca from a Yahoo ETF search result", async () => {
    searchInstruments.mockResolvedValue([{
      providerKey: "yahoo_finance",
      providerSymbol: "SPY",
      name: "SPDR S&P 500 ETF Trust",
      symbol: "SPY",
      type: "etf",
      marketCode: "ARCA",
      countryCode: "US",
      quoteCurrency: "USD",
      exchange: "NYSEArca",
    }]);
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.selectOptions(within(form).getByLabelText("Type"), "etf");
    await userEvent.type(within(form).getByLabelText("Search Yahoo"), "SPY");
    await userEvent.click(await within(form).findByRole("option", { name: "SPY · SPDR S&P 500 ETF Trust · NYSEArca" }));
    expect(within(form).getByLabelText("Trading market")).toHaveValue("ARCA");
    expect(within(form).getByLabelText("Country / region")).toHaveValue("US");
    expect(selectValues(within(form).getByLabelText("Trading market"))).toEqual(["", "NASDAQ", "NYSE", "AMEX", "ARCA", "BZX", "EDGX", "IEX"]);
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));
    expect(createInstrument).toHaveBeenCalledWith(expect.objectContaining({
      name: "SPDR S&P 500 ETF Trust",
      type: "etf",
      symbol: "SPY",
      marketCode: "ARCA",
      countryCode: "US",
      quoteCurrency: "USD",
      quoteSource: "provider",
      providerKey: "yahoo_finance",
      providerSymbol: "SPY",
    }));
  });

  it("fills Cboe BZX from a Yahoo ETF search result", async () => {
    searchInstruments.mockResolvedValue([{
      providerKey: "yahoo_finance",
      providerSymbol: "ARKK",
      name: "ARK Innovation ETF",
      symbol: "ARKK",
      type: "etf",
      marketCode: "BZX",
      countryCode: "US",
      quoteCurrency: "USD",
      exchange: "Cboe BZX",
    }]);
    renderManagement();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.selectOptions(within(form).getByLabelText("Type"), "etf");
    await userEvent.type(within(form).getByLabelText("Search Yahoo"), "ARKK");
    await userEvent.click(await within(form).findByRole("option", { name: "ARKK · ARK Innovation ETF · Cboe BZX" }));
    expect(within(form).getByLabelText("Trading market")).toHaveValue("BZX");
    expect(within(form).getByLabelText("Country / region")).toHaveValue("US");
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));
    expect(createInstrument).toHaveBeenCalledWith(expect.objectContaining({
      name: "ARK Innovation ETF",
      type: "etf",
      symbol: "ARKK",
      marketCode: "BZX",
      countryCode: "US",
      quoteSource: "provider",
      providerKey: "yahoo_finance",
      providerSymbol: "ARKK",
    }));
  });
});
