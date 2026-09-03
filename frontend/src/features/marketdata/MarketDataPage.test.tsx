import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { MarketDataPage } from "./MarketDataPage";
import { formatTimestamp } from "@/lib/time";

const refreshAll = vi.fn();
const listInstruments = vi.fn();
const currentInstrumentQuote = vi.fn();
const currentFXQuote = vi.fn();
const listFXPreferences = vi.fn();
const setFXPreference = vi.fn();
const refreshRequiredFX = vi.fn();
const instrumentQuoteSeries = vi.fn();
const fxQuoteSeries = vi.fn();
const overview = vi.fn();
const setInstrumentQuoteSource = vi.fn();
const appendManualFXQuote = vi.fn();
const cancelRefresh = vi.fn(async () => undefined);
const refreshListeners = new Map<string, (event: { data: unknown }) => void>();
const startRefresh = (operation: () => Promise<unknown>) => async (requestId: string) => {
  const result = await operation();
  queueMicrotask(() => refreshListeners.get("marketdata.refresh.completed")?.({ data: { requestId, status: "completed", result } }));
};

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      refreshListeners.set(name, listener);
      return () => refreshListeners.delete(name);
    },
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    StartRefreshAll: (requestId: string) => startRefresh(refreshAll)(requestId),
    StartRefreshRequiredFX: (requestId: string) => startRefresh(refreshRequiredFX)(requestId),
    StartRefreshMissingOrStale: (requestId: string) => startRefresh(refreshAll)(requestId),
    CancelRefresh: () => cancelRefresh(),
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: () => listInstruments(),
    SetInstrumentQuoteSource: (...args: unknown[]) => setInstrumentQuoteSource(...args),
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    CurrentInstrumentQuote: (id: string) => currentInstrumentQuote(id),
    CurrentFXQuote: (a: string, b: string) => currentFXQuote(a, b),
    ListFXPreferences: () => listFXPreferences(),
    SetFXPreference: (a: string, b: string, source: string) => setFXPreference(a, b, source),
    AppendManualFXQuote: (...args: unknown[]) => appendManualFXQuote(...args),
    InstrumentQuoteSeries: (...args: unknown[]) => instrumentQuoteSeries(...args),
    FXQuoteSeries: (...args: unknown[]) => fxQuoteSeries(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: { Overview: () => overview() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => Promise.resolve({ timezone: "Pacific/Auckland", fxProvider: "frankfurter" }),
    SupportedCurrencies: () => Promise.resolve(["USD", "CNY", "SGD"]),
    FXProviders: () => Promise.resolve(["frankfurter"]),
  },
}));

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <MarketDataPage />
    </QueryClientProvider>,
  );
}

describe("MarketDataPage", () => {
  beforeEach(() => {
    refreshAll.mockReset();
    refreshRequiredFX.mockReset();
    listInstruments.mockReset();
    currentInstrumentQuote.mockReset();
    currentFXQuote.mockReset();
    listFXPreferences.mockReset();
    setFXPreference.mockReset();
    setInstrumentQuoteSource.mockReset();
    appendManualFXQuote.mockReset();
    cancelRefresh.mockReset();
    overview.mockReset();
    instrumentQuoteSeries.mockReset();
    fxQuoteSeries.mockReset();
    instrumentQuoteSeries.mockResolvedValue({ range: "30d", points: [], observations: [], outsideRange: false });
    fxQuoteSeries.mockResolvedValue({ range: "30d", points: [], observations: [], outsideRange: false });
    listInstruments.mockResolvedValue([]);
    currentInstrumentQuote.mockResolvedValue(null);
    currentFXQuote.mockResolvedValue(null);
    listFXPreferences.mockResolvedValue([]);
    overview.mockResolvedValue({ missingInputs: [] });
  });

  it("shows saved instrument prices and FX rates before a refresh", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "Global Equity Fund", quoteCurrency: "USD", quoteSource: "provider" }]);
    currentInstrumentQuote.mockResolvedValue({
      id: "q1",
      instrumentId: "i1",
      unitPrice: "12.5",
      currency: "USD",
      sourceKind: "provider",
      sourceKey: "yahoo_finance",
      quotedAt: "2024-01-01T00:00:00Z",
      createdAt: "2024-01-01T00:00:00Z",
      delayed: false,
    });
    listFXPreferences.mockResolvedValue([
      {
        householdId: "h1",
        currencyA: "CNY",
        currencyB: "SGD",
        sourceKind: "provider",
        createdAt: "2024-01-01T00:00:00Z",
        updatedAt: "2024-01-01T00:00:00Z",
      },
    ]);
    currentFXQuote.mockResolvedValue({
      id: "fx1",
      householdId: "h1",
      baseCurrency: "CNY",
      quoteCurrency: "SGD",
      rate: "0.19",
      sourceKind: "provider",
      sourceKey: "frankfurter",
      quotedAt: "2024-01-01T00:00:00Z",
      createdAt: "2024-01-01T00:00:00Z",
      delayed: false,
    });

    renderPage();

    await screen.findByText("Global Equity Fund");
    await screen.findByText("Latest: $12.50");
    await screen.findByText("1 CNY = 0.19 SGD");
    const savedData = screen.getByTestId("saved-market-data");
    const savedInstrument = screen.getByTestId("saved-instrument-i1");
    const savedFX = screen.getByTestId("saved-fx-CNY/SGD");
    expect(savedInstrument).toHaveClass("grid-cols-[minmax(0,1fr)_auto]");
    expect(within(savedInstrument).getByRole("button", { name: "View history" }).parentElement).toHaveClass("row-start-1");
    expect(savedFX).toHaveClass("grid-cols-[minmax(0,1fr)_auto]");
    expect(within(savedFX).getByRole("button", { name: "View history" }).parentElement).toHaveClass("row-start-1");
    expect(savedData).toHaveTextContent("Global Equity Fund");
    expect(savedData).toHaveTextContent("Latest: $12.50");
    expect(savedData).toHaveTextContent("CNY/SGD");
    expect(savedData).toHaveTextContent("1 CNY = 0.19 SGD");
    expect(savedData).toHaveTextContent(formatTimestamp("2024-01-01T00:00:00Z", "Pacific/Auckland", "en"));
  });

  it("saves a manual FX observation without changing its preference", async () => {
    appendManualFXQuote.mockResolvedValue({
      id: "fx-manual",
      baseCurrency: "SGD",
      quoteCurrency: "USD",
      rate: "1.25",
      sourceKind: "manual",
    });
    renderPage();
    await waitFor(() => expect(screen.getByLabelText("Base currency")).toHaveValue("USD"));
    await userEvent.selectOptions(screen.getByLabelText("Base currency"), "SGD");
    await userEvent.selectOptions(screen.getByLabelText("Quote currency"), "USD");
    await userEvent.type(screen.getByLabelText("Rate"), "1.25");
    await userEvent.click(screen.getByRole("button", { name: "Save manual FX quote" }));
    expect(appendManualFXQuote).toHaveBeenCalledWith("SGD", "USD", "1.25", "");
  });

  it("groups saved data as FX then instrument type, sorted by currency", async () => {
    listInstruments.mockResolvedValue([
      { id: "etf-1", name: "QQQ", type: "etf", quoteCurrency: "USD", quoteSource: "provider" },
      { id: "stock-usd", name: "Apple", type: "stock", quoteCurrency: "USD", quoteSource: "provider" },
      { id: "stock-cny", name: "Kweichow", type: "stock", quoteCurrency: "CNY", quoteSource: "provider" },
    ]);
    listFXPreferences.mockResolvedValue([
      {
        householdId: "h1",
        currencyA: "USD",
        currencyB: "SGD",
        sourceKind: "provider",
        createdAt: "2024-01-01T00:00:00Z",
        updatedAt: "2024-01-01T00:00:00Z",
      },
      {
        householdId: "h1",
        currencyA: "CNY",
        currencyB: "SGD",
        sourceKind: "provider",
        createdAt: "2024-01-01T00:00:00Z",
        updatedAt: "2024-01-01T00:00:00Z",
      },
    ]);

    renderPage();
    const fxGroup = await screen.findByTestId("market-data-group-fx");
    const stockGroup = screen.getByTestId("market-data-group-stock");
    const etfGroup = screen.getByTestId("market-data-group-etf");
    expect(fxGroup.compareDocumentPosition(stockGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(stockGroup.compareDocumentPosition(etfGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      within(fxGroup).getByTestId("saved-fx-CNY/SGD").compareDocumentPosition(within(fxGroup).getByTestId("saved-fx-SGD/USD"))
        & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(within(stockGroup).getByText("Kweichow").compareDocumentPosition(within(stockGroup).getByText("Apple")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("configures a provider for an FX pair without a source and refreshes it", async () => {
    overview.mockResolvedValue({ missingInputs: [{ kind: "fx_rate", baseCurrency: "CNY", quoteCurrency: "SGD" }] });
    setFXPreference.mockResolvedValue({
      householdId: "h1",
      currencyA: "CNY",
      currencyB: "SGD",
      sourceKind: "provider",
      createdAt: "2024-01-01T00:00:00Z",
      updatedAt: "2024-01-01T00:00:00Z",
    });
    refreshAll.mockResolvedValue({ items: [{ targetKey: "fx:CNY/SGD", kind: "fx", status: "fetched" }], rateLimited: false });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Use provider" }));

    expect(setFXPreference).toHaveBeenCalledWith("CNY", "SGD", "provider");
    expect(await screen.findByTestId("refresh-results")).toHaveTextContent("CNY/SGD");
    expect(refreshAll).toHaveBeenCalledTimes(1);
  });

  it("triggers RefreshAll and renders the per-target results", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "Global Equity Fund" }]);
    currentInstrumentQuote.mockResolvedValue({
      id: "q1",
      instrumentId: "i1",
      unitPrice: "12.5",
      currency: "USD",
      sourceKind: "manual",
      sourceKey: "",
      quotedAt: "2024-01-01T00:00:00Z",
      createdAt: "2024-01-01T00:00:00Z",
      delayed: false,
    });
    currentFXQuote.mockResolvedValue(null);
    refreshAll.mockResolvedValue({
      items: [
        { targetKey: "instrument:i1", kind: "instrument", status: "fetched" },
        { targetKey: "fx:SGD/USD", kind: "fx", status: "skipped", errorCode: "unavailable" },
      ],
      rateLimited: false,
    });
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /force refresh all/i }));
    const results = await screen.findByTestId("refresh-results");
    expect(results).toHaveTextContent("Global Equity Fund");
    expect(results).toHaveTextContent("Updated");
    expect(results).toHaveTextContent("Latest: $12.50");
    expect(results).toHaveTextContent("SGD/USD");
    expect(results).toHaveTextContent("No saved value yet");
    expect(results).toHaveTextContent("No provider is configured for this target.");
  });

  it("cancels an active refresh and ignores its completion", async () => {
    let releaseRefresh: (() => void) | undefined;
    refreshAll.mockImplementation(() => new Promise((resolve) => {
      releaseRefresh = () => resolve({ items: [], rateLimited: false });
    }));
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /force refresh all/i }));
    const cancelButton = await screen.findByRole("button", { name: "Cancel refresh" });
    await userEvent.click(cancelButton);
    expect(cancelRefresh).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("status")).toHaveTextContent("Refresh cancelled");
    releaseRefresh?.();
    expect(screen.queryByTestId("refresh-results")).not.toBeInTheDocument();
  });

  it("shows an empty state when there are no refresh targets", async () => {
    listInstruments.mockResolvedValue([]);
    refreshAll.mockResolvedValue({ items: [], rateLimited: false });
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /force refresh all/i }));
    expect(await screen.findByText("No saved market data needs refreshing.")).toBeInTheDocument();
  });

  it("opens local quote history without calling a provider", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "Global Equity Fund", quoteCurrency: "USD", quoteSource: "provider" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "View history" }));
    expect(await screen.findByRole("heading", { name: "Global Equity Fund price history" })).toBeInTheDocument();
    expect(await screen.findByText("No local history is saved yet.")).toBeInTheDocument();
  });

  it("offers show-all when local facts exist outside the selected range", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "Global Equity Fund", quoteCurrency: "USD", quoteSource: "provider" }]);
    instrumentQuoteSeries.mockResolvedValue({ range: "30d", points: [], observations: [], outsideRange: true });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "View history" }));
    expect(await screen.findByText("No local observations in this range.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show all history" })).toBeInTheDocument();
  });

  it("loads swapped FX history as a new directional series instead of the sorted pair cache", async () => {
    listFXPreferences.mockResolvedValue([
      {
        householdId: "h1",
        currencyA: "CNY",
        currencyB: "USD",
        sourceKind: "provider",
        createdAt: "2024-01-01T00:00:00Z",
        updatedAt: "2024-01-01T00:00:00Z",
      },
    ]);
    fxQuoteSeries.mockImplementation(async (base: string, quote: string) => {
      if (base === "USD" && quote === "CNY") {
        return {
          range: "30d",
          baseCurrency: "USD",
          quoteCurrency: "CNY",
          points: [
            { quotedAt: "2026-08-20T00:00:00Z", value: "7", sourceKind: "manual", sourceKey: "", delayed: false },
          ],
          observations: [
            { quotedAt: "2026-08-20T00:00:00Z", value: "7", sourceKind: "manual", sourceKey: "", delayed: false },
          ],
          outsideRange: false,
        };
      }
      return {
        range: "30d",
        baseCurrency: "CNY",
        quoteCurrency: "USD",
        points: [
          { quotedAt: "2026-08-20T00:00:00Z", value: "0.14", sourceKind: "manual", sourceKey: "", delayed: false },
          { quotedAt: "2026-08-21T00:00:00Z", value: "0.15", sourceKind: "provider", sourceKey: "frankfurter", delayed: true },
        ],
        observations: [
          { quotedAt: "2026-08-21T00:00:00Z", value: "0.15", sourceKind: "provider", sourceKey: "frankfurter", delayed: true },
          { quotedAt: "2026-08-20T00:00:00Z", value: "0.14", sourceKind: "manual", sourceKey: "", delayed: false },
        ],
        outsideRange: false,
      };
    });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "View history" }));
    expect(await screen.findByRole("heading", { name: "CNY/USD rate history" })).toBeInTheDocument();
    expect(fxQuoteSeries).toHaveBeenCalledWith("CNY", "USD", "30d", "all");
    await userEvent.click(screen.getByText("View data table"));
    expect(screen.getByText("Provider · frankfurter")).toBeInTheDocument();
    expect(screen.getAllByText("Delayed").length).toBeGreaterThan(0);
    expect(screen.getByText("Not delayed")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Swap direction" }));
    expect(await screen.findByRole("heading", { name: "USD/CNY rate history" })).toBeInTheDocument();
    expect(fxQuoteSeries).toHaveBeenCalledWith("USD", "CNY", "30d", "all");
    expect(fxQuoteSeries.mock.calls[0]?.slice(0, 2)).toEqual(["CNY", "USD"]);
    expect(fxQuoteSeries.mock.calls[1]?.slice(0, 2)).toEqual(["USD", "CNY"]);
  });
});
