import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { paginateActivityPage } from "@/test/activityPage";
import { HistoryPage } from "./HistoryPage";
import { localDateInTimeZone } from "@/features/history/historyStartDate";
import { ChangeCommandKind } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";

const { bootstrap } = vi.hoisted(() => ({ bootstrap: vi.fn() }));
const historyOrigin = vi.fn();
const startHistory = vi.fn();
const startHistoryWithCosts = vi.fn();
const startingPointDraft = vi.fn();
const listActivities = vi.fn();
const listActivityPage = vi.fn();
const getActivity = vi.fn();
const previewChange = vi.fn();
const previewFixChange = vi.fn();
const recordChange = vi.fn();
const undoChange = vi.fn();
const fixChange = vi.fn();
const listAccounts = vi.fn();
const listInstruments = vi.fn();
const holdingsByAccounts = vi.fn();
const settingsLoad = vi.fn();
const listFXPreferences = vi.fn();
const currentFXQuote = vi.fn();
const setFXPreference = vi.fn();
const currentInstrumentQuote = vi.fn();
const refreshFX = vi.fn();
const refreshInstrument = vi.fn();
const historyMutationAllowed = vi.fn();
const dailySnapshotState = vi.fn();
const rebuildHistoricalSnapshots = vi.fn();
const refreshListeners = new Map<string, (event: { data: unknown }) => void>();
const startRefresh = (operation: (...args: unknown[]) => Promise<unknown>) => async (requestId: string, ...args: unknown[]) => {
  const result = await operation(...args);
  queueMicrotask(() => refreshListeners.get("marketdata.refresh.completed")?.({ data: { requestId, result } }));
};

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      refreshListeners.set(name, listener);
      return () => refreshListeners.delete(name);
    },
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => historyOrigin(),
    StartHistory: (...args: unknown[]) => startHistory(...args),
    StartHistoryWithCosts: (...args: unknown[]) => startHistoryWithCosts(...args),
    StartingPointDraft: () => startingPointDraft(),
    ListActivities: () => listActivities(),
    Activity: (...args: unknown[]) => getActivity(...args),
    ListActivityPage: (...args: unknown[]) => listActivityPage(...args),
    PreviewChange: (...args: unknown[]) => previewChange(...args),
    PreviewFixChange: (...args: unknown[]) => previewFixChange(...args),
    RecordChange: (...args: unknown[]) => recordChange(...args),
    UndoChange: (...args: unknown[]) => undoChange(...args),
    FixChange: (...args: unknown[]) => fixChange(...args),
    HistoryMutationAllowed: () => historyMutationAllowed(),
    DailySnapshotState: (...args: unknown[]) => dailySnapshotState(...args),
    RebuildHistoricalSnapshots: (...args: unknown[]) => rebuildHistoricalSnapshots(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: () => Promise.resolve([]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: (...args: unknown[]) => listInstruments(...args), CreateInstrument: vi.fn() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: { HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    ListFXPreferences: () => listFXPreferences(),
    CurrentFXQuote: (...args: unknown[]) => currentFXQuote(...args),
    SetFXPreference: (...args: unknown[]) => setFXPreference(...args),
    CurrentInstrumentQuote: (...args: unknown[]) => currentInstrumentQuote(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    StartRefreshFX: (...args: unknown[]) => startRefresh(refreshFX)(...args as [string, ...unknown[]]),
    StartRefreshInstrument: (...args: unknown[]) => startRefresh(refreshInstrument)(...args as [string, ...unknown[]]),
    CancelRefresh: vi.fn(async () => undefined),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => settingsLoad(),
    SupportedCurrencies: () => Promise.resolve(["USD", "SGD", "CNY"]),
    FXProviders: () => Promise.resolve(["frankfurter"]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: {
    Bootstrap: () => bootstrap(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <HistoryPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  historyOrigin.mockReset();
  startHistory.mockReset();
  startHistoryWithCosts.mockReset();
  startingPointDraft.mockReset();
  listActivities.mockReset();
  listActivityPage.mockReset();
  getActivity.mockReset();
  previewChange.mockReset();
  recordChange.mockReset();
  undoChange.mockReset();
  fixChange.mockReset();
  previewFixChange.mockReset();
  listAccounts.mockReset();
  listInstruments.mockReset();
  holdingsByAccounts.mockReset();
  settingsLoad.mockReset();
  listFXPreferences.mockReset();
  currentFXQuote.mockReset();
  setFXPreference.mockReset();
  currentInstrumentQuote.mockReset();
  refreshFX.mockReset();
  refreshInstrument.mockReset();
  historyMutationAllowed.mockReset();
  dailySnapshotState.mockReset();
  rebuildHistoricalSnapshots.mockReset();
  bootstrap.mockReset();
  bootstrap.mockResolvedValue({
    household: { id: "h1", name: "Test", baseCurrency: "USD", createdAt: "", updatedAt: "" },
    members: [],
    institutions: [],
    groups: [],
  });
  startingPointDraft.mockResolvedValue([]);
  listActivities.mockResolvedValue([]);
  listActivityPage.mockImplementation(async (request: { afterId?: string; afterEffectiveAt?: string; afterCreatedAt?: string; limit?: number } = {}) =>
    paginateActivityPage(await listActivities(), request),
  );
  getActivity.mockResolvedValue(null);
  listAccounts.mockResolvedValue([
    { account: { id: "acc-1", name: "Checking", trackingMode: "balance" }, ownership: [], latestValue: null },
  ]);
  listInstruments.mockResolvedValue([]);
  holdingsByAccounts.mockResolvedValue({});
  settingsLoad.mockResolvedValue({ timezone: "system", fxProvider: "frankfurter" });
  listFXPreferences.mockResolvedValue([]);
  currentFXQuote.mockResolvedValue(null);
  setFXPreference.mockResolvedValue({ currencyA: "EUR", currencyB: "USD", sourceKind: "provider" });
  currentInstrumentQuote.mockResolvedValue(null);
  refreshFX.mockResolvedValue({ items: [{ targetKey: "fx:EUR/USD", kind: "fx", status: "fetched" }], rateLimited: false });
  refreshInstrument.mockResolvedValue({ items: [{ targetKey: "instrument:instrument-1", kind: "instrument", status: "fetched" }], rateLimited: false });
  historyMutationAllowed.mockResolvedValue({});
  dailySnapshotState.mockResolvedValue({ householdId: "h1", dirtyFrom: null, lastCompletedClosedOn: null });
  rebuildHistoricalSnapshots.mockResolvedValue(0);
});

describe("HistoryPage", () => {
  it("prompts to Start History when none exists yet", async () => {
    historyOrigin.mockResolvedValue(null);
    renderPage();
    expect(await screen.findByRole("heading", { name: "Start history" })).toBeInTheDocument();
    expect(screen.getByLabelText("Start date")).toHaveValue(localDateInTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone) ?? "");
  });

  it("starts history with the resolved settings timezone", async () => {
    historyOrigin.mockResolvedValue(null);
    startHistory.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    const timezoneInput = await screen.findByLabelText("Timezone");
    expect(timezoneInput).toHaveValue(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(timezoneInput).toHaveAttribute("readonly");
    const resolvedTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(screen.getByLabelText("Start date")).toHaveValue(localDateInTimeZone(resolvedTimezone) ?? "");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));
    expect(startHistory).toHaveBeenCalledWith(resolvedTimezone);
    expect(startHistoryWithCosts).not.toHaveBeenCalled();
  });

  it("starts history with unit costs when holdings already exist", async () => {
    historyOrigin.mockResolvedValue(null);
    startingPointDraft.mockResolvedValue([
      { holdingId: "h1", instrumentId: "i1", instrumentName: "Vanguard S&P 500 ETF", currency: "USD", quantity: "100", unitCost: "" },
    ]);
    startHistoryWithCosts.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    const timezoneInput = await screen.findByLabelText("Timezone");
    expect(timezoneInput).toHaveValue(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(timezoneInput).toHaveAttribute("readonly");
    const costInput = await screen.findByLabelText(/Vanguard S&P 500 ETF/);
    await userEvent.type(costInput, "430.25");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));
    expect(startHistoryWithCosts).toHaveBeenCalledWith(Intl.DateTimeFormat().resolvedOptions().timeZone, { h1: "430.25" });
    expect(startHistory).not.toHaveBeenCalled();
  });

  it("previews then records a money_added change", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    previewChange.mockResolvedValue({
      activity: { id: "a1", kind: "cash_in", effects: [] },
      effects: [],
      resulting: [{ target: "account_value", name: "Checking", amount: "1000", currency: "USD" }],
    });
    recordChange.mockResolvedValue({ activity: { id: "a1", kind: "cash_in" }, effects: [], resulting: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });

    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "1000");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));

    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1000", currency: "USD" }));
    expect(await within(form).findByRole("status")).toHaveTextContent("Checking");

    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
    expect(recordChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1000" }));
  });

  it("previews money_added in the household base currency after hydrate, not CNY", async () => {
    bootstrap.mockResolvedValue({
      household: { id: "h1", name: "Test", baseCurrency: "SGD", createdAt: "", updatedAt: "" },
      members: [],
      institutions: [],
      groups: [],
    });
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    previewChange.mockResolvedValue({
      activity: { id: "a1", kind: "cash_in", effects: [] },
      effects: [],
      resulting: [{ target: "account_value", name: "Checking", amount: "1000", currency: "SGD" }],
    });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "1000");
    expect(within(form).getByLabelText("Currency")).toHaveValue("SGD");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", currency: "SGD" }));
    expect(previewChange.mock.calls[0][0].currency).not.toBe("CNY");
  });

  it("previews a cash dividend against a holding", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    listAccounts.mockResolvedValue([
      { account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
    ]);
    listInstruments.mockResolvedValue([
      { id: "instrument-1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" },
    ]);
    holdingsByAccounts.mockResolvedValue({
      "brokerage-1": [{ id: "holding-1", accountId: "brokerage-1", instrumentId: "instrument-1", quantity: "10" }],
    });
    previewChange.mockResolvedValue({
      activity: { id: "div-1", kind: "cash_dividend", effects: [] },
      effects: [],
      resulting: [{ target: "account_cash", name: "Brokerage", amount: "25", currency: "USD" }],
    });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeCashDividend);
    await userEvent.selectOptions(within(form).getByLabelText("Holding"), "holding-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "25");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));

    expect(previewChange).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "cash_dividend",
        holdingId: "holding-1",
        amount: "25",
        currency: "USD",
      }),
    );
  });

  it("shows archived instrument names on historical dividends without offering them in the new form", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockImplementation(async (includeArchived?: boolean) => {
      const nvidia = { id: "instrument-1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual", archivedAt: "2026-06-01T00:00:00Z" };
      const apple = { id: "instrument-2", name: "Apple", quoteCurrency: "USD", quoteSource: "manual", archivedAt: null };
      return includeArchived ? [nvidia, apple] : [apple];
    });
    listAccounts.mockResolvedValue([
      { account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
    ]);
    holdingsByAccounts.mockResolvedValue({
      "brokerage-1": [
        { id: "holding-archived", accountId: "brokerage-1", instrumentId: "instrument-1", quantity: "10", archivedAt: null },
        { id: "holding-live", accountId: "brokerage-1", instrumentId: "instrument-2", quantity: "4", archivedAt: null },
        { id: "holding-gone", accountId: "brokerage-1", instrumentId: "instrument-2", quantity: "1", archivedAt: "2026-06-01T00:00:00Z" },
      ],
    });
    listActivities.mockResolvedValue([
      {
        id: "div-archived",
        kind: "cash_dividend",
        effectiveAt: "2026-01-15T00:00:00.000Z",
        effectiveLocalDate: "2026-01-15",
        dividendDetail: { holdingId: "holding-archived", instrumentId: "instrument-1", amount: { amount: "25", currency: "USD" } },
        effects: [{ accountId: "brokerage-1", money: { amount: "25", currency: "USD" } }],
      },
    ]);

    renderPage();
    expect(await screen.findByText("Dividend $25.00 from NVIDIA into Brokerage")).toBeInTheDocument();
    expect(screen.queryByText(/unknown instrument/i)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeCashDividend);
    const holdingSelect = within(form).getByLabelText("Holding");
    expect(within(holdingSelect).getByRole("option", { name: /Apple/ })).toBeInTheDocument();
    expect(within(holdingSelect).queryByRole("option", { name: /NVIDIA/ })).not.toBeInTheDocument();
    expect(within(holdingSelect).queryByRole("option", { name: /holding-gone/ })).not.toBeInTheDocument();
  });

  it("previews a trade with visible currencies and the existing matching holding", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    listAccounts.mockResolvedValue([
      { account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
    ]);
    listInstruments.mockResolvedValue([
      { id: "instrument-1", name: "NVIDIA", quoteCurrency: "EUR", quoteSource: "manual" },
    ]);
    holdingsByAccounts.mockResolvedValue({
      "brokerage-1": [{ id: "holding-1", accountId: "brokerage-1", instrumentId: "instrument-1", quantity: "0" }],
    });
    previewChange.mockResolvedValue({
      activity: { id: "trade-1", kind: "buy", effects: [] },
      effects: [],
      resulting: [{ target: "holding_quantity", name: "NVIDIA", quantity: "10" }],
    });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeTrade);
    await within(form).findByRole("option", { name: /NVIDIA/ });
    await userEvent.selectOptions(within(form).getByLabelText("Settlement account"), "brokerage-1");
    await userEvent.selectOptions(within(form).getByLabelText("Instrument"), "instrument-1");
    await userEvent.type(within(form).getByLabelText("Quantity"), "10");
    await userEvent.type(within(form).getByLabelText("Gross total"), "1000");
    await userEvent.type(within(form).getByLabelText("Fee (optional)"), "5");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));

    expect(previewChange).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "trade",
        settlementAccountId: "brokerage-1",
        instrumentId: "instrument-1",
        holdingId: "holding-1",
        grossCurrency: "EUR",
        feeCurrency: "EUR",
      }),
    );
  });

  it("defaults trade gross from the latest unit price while keeping it editable", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "instrument-1", name: "NVIDIA", quoteCurrency: "EUR", quoteSource: "manual" }]);
    listAccounts.mockResolvedValue([{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings", defaultCurrency: "CNY" }, ownership: [], latestValue: null }]);
    holdingsByAccounts.mockResolvedValue({ "brokerage-1": [] });
    currentInstrumentQuote.mockResolvedValue({ id: "quote-1", instrumentId: "instrument-1", unitPrice: "123.456", currency: "EUR", quotedAt: "2026-08-23T00:00:00Z" });
    previewChange.mockResolvedValue({ activity: { id: "trade-1", kind: "buy", effects: [] }, effects: [], resulting: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeTrade);
    await userEvent.selectOptions(within(form).getByLabelText("Settlement account"), "brokerage-1");
    await userEvent.selectOptions(within(form).getByLabelText("Instrument"), "instrument-1");
    await userEvent.type(within(form).getByLabelText("Quantity"), "10");
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));

    await waitFor(() => expect(within(form).getByLabelText("Gross total")).toHaveValue("1234.56"));
    expect(within(form).getByLabelText("Gross total")).not.toHaveAttribute("disabled");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ gross: "1234.56", grossCurrency: "EUR" }));
  });

  it("preserves a manually overridden trade gross when quantity changes", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "instrument-1", name: "NVIDIA", quoteCurrency: "EUR", quoteSource: "manual" }]);
    listAccounts.mockResolvedValue([{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings", defaultCurrency: "CNY" }, ownership: [], latestValue: null }]);
    holdingsByAccounts.mockResolvedValue({ "brokerage-1": [] });
    currentInstrumentQuote.mockResolvedValue({ id: "quote-1", instrumentId: "instrument-1", unitPrice: "123.456", currency: "EUR", quotedAt: "2026-08-23T00:00:00Z" });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeTrade);
    await userEvent.selectOptions(within(form).getByLabelText("Settlement account"), "brokerage-1");
    await userEvent.selectOptions(within(form).getByLabelText("Instrument"), "instrument-1");
    await userEvent.type(within(form).getByLabelText("Quantity"), "10");
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));
    await waitFor(() => expect(within(form).getByLabelText("Gross total")).toHaveValue("1234.56"));

    const gross = within(form).getByLabelText("Gross total");
    const quantity = within(form).getByLabelText("Quantity");
    await userEvent.clear(gross);
    await userEvent.type(gross, "999");
    await userEvent.clear(quantity);
    await userEvent.type(quantity, "11");
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));

    expect(gross).toHaveValue("999");
    await userEvent.clear(gross);
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));
    await waitFor(() => expect(gross).toHaveValue("1358.02"));
  });

  it("updates a new direct FX pair after saving its provider preference", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings", defaultCurrency: "CNY" }, ownership: [], latestValue: null }]);
    holdingsByAccounts.mockResolvedValue({ "brokerage-1": [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeFXConversion);
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "brokerage-1");
    const currencies = within(form).getAllByLabelText("Currency");
    await userEvent.selectOptions(currencies[0], "USD");
    await userEvent.selectOptions(currencies[1], "SGD");

    const update = await within(form).findByRole("button", { name: "Update" });
    await userEvent.click(update);
    await waitFor(() => expect(setFXPreference).toHaveBeenCalledWith("USD", "SGD", "provider"));
    await waitFor(() => expect(refreshFX).toHaveBeenCalledWith("USD", "SGD"));
    expect(currentFXQuote).not.toHaveBeenCalledWith("USD", "USD");
  });

  it("lists activities as a plain-language sentence and undoes one after confirmation", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([
      {
        id: "a1",
        kind: "cash_in",
        reason: "contribution",
        effectiveLocalDate: "2026-01-01",
        effects: [{ accountId: "acc-1", money: { amount: "1000", currency: "USD" } }],
      },
    ]);
    undoChange.mockResolvedValue({ activity: { id: "a2", kind: "reversal" }, effects: [], resulting: [] });

    renderPage();
    const list = await screen.findByTestId("activity-list");
    expect(list).toHaveTextContent("Added $1,000.00 to Checking (Contribution)");
    expect(list).not.toHaveTextContent("cash_in");
    await userEvent.click(await within(list).findByRole("button", { name: "Details" }));
    expect(await screen.findByText("Local date and time")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await userEvent.click(await within(list).findByRole("button", { name: "Undo" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Undo" }));
    expect(undoChange).toHaveBeenCalledWith("a1");
  });

  it("uses archived account names in history but excludes them from the account filter", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([
      { account: { id: "active", name: "Checking", trackingMode: "balance" }, ownership: [], latestValue: null },
      { account: { id: "archived", name: "Closed Savings", trackingMode: "balance", archivedAt: "2026-01-02" }, ownership: [], latestValue: null },
    ]);
    listActivities.mockResolvedValue([
      {
        id: "a1",
        kind: "cash_out",
        reason: "expense",
        effectiveLocalDate: "2026-01-01",
        effects: [{ accountId: "archived", money: { amount: "25", currency: "USD" } }],
      },
    ]);

    renderPage();

    expect(await screen.findByText("Removed $25.00 from Closed Savings (Expense)")).toBeInTheDocument();
    const accountFilter = screen.getByLabelText("Account");
    expect(within(accountFilter).getByRole("option", { name: "Checking" })).toBeInTheDocument();
    expect(within(accountFilter).queryByRole("option", { name: "Closed Savings" })).not.toBeInTheDocument();
    expect(listAccounts).toHaveBeenCalledWith(expect.objectContaining({ includeArchived: true }));
  });

  it("loads a reversal's original activity by ID and labels every effect direction", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([
      { account: { id: "checking", name: "Checking", trackingMode: "balance" }, ownership: [], latestValue: null },
      { account: { id: "brokerage", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
    ]);
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA" }]);
    holdingsByAccounts.mockResolvedValue({ brokerage: [{ id: "h1", instrumentId: "i1" }] });
    listActivities.mockResolvedValue([
      {
        id: "reversal-1",
        kind: "reversal",
        reversesActivityId: "original-outside-page",
        effectiveLocalDate: "2026-01-02",
        effects: [
          { id: "e1", role: "amount", direction: "removed", accountId: "checking", money: { amount: "100", currency: "USD" } },
          { id: "e2", role: "quantity", direction: "added", holdingId: "h1", instrumentId: "i1", quantity: "2" },
        ],
      },
    ]);
    getActivity.mockResolvedValue({
      id: "original-outside-page",
      kind: "cash_in",
      reason: "income",
      effectiveLocalDate: "2025-01-01",
      effects: [{ accountId: "checking", money: { amount: "100", currency: "USD" } }],
    });

    renderPage();
    const list = await screen.findByTestId("activity-list");
    await userEvent.click(within(list).getByRole("button", { name: "Details" }));

    expect(await screen.findByText((_, node) => node?.textContent === "Original activity: Added $100.00 to Checking (Income)")).toBeInTheDocument();
    expect(screen.getByText("Checking · Amount removed: $100.00")).toBeInTheDocument();
    expect(screen.getByText("Brokerage · NVIDIA · Quantity added: 2")).toBeInTheDocument();
    expect(getActivity).toHaveBeenCalledWith("original-outside-page");
  });

  it("fixes a change, pre-filling the form from the original activity's effects", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([
      {
        id: "a1",
        kind: "cash_in",
        effectiveLocalDate: "2026-01-01",
        reason: "contribution",
        effects: [{ role: "amount", direction: "added", target: "account_value", accountId: "acc-1", money: { amount: "1000", currency: "USD" } }],
      },
    ]);
    previewFixChange.mockResolvedValue({ activity: { id: "a1", kind: "cash_in" }, effects: [], resulting: [{ target: "account_value", name: "Checking", amount: "1200", currency: "USD" }] });
    fixChange.mockResolvedValue({ activity: { id: "a1", kind: "cash_in", correctionGroupId: "g1" }, effects: [], resulting: [] });

    renderPage();
    const list = await screen.findByTestId("activity-list");
    await userEvent.click(await within(list).findByRole("button", { name: "Fix" }));

    const form = await screen.findByRole("form", { name: "Record change" });
    expect(within(form).getByLabelText("Account")).toHaveValue("acc-1");
    expect(within(form).getByLabelText("Amount")).toHaveValue("1000");

    await userEvent.clear(within(form).getByLabelText("Amount"));
    await userEvent.type(within(form).getByLabelText("Amount"), "1200");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));

    // Regression: the Fix form's Preview step must call PreviewFixChange
    // (which inverts the original Activity first), never plain
    // PreviewChange (which would double-count it).
    expect(previewFixChange).toHaveBeenCalledWith("a1", expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1200" }));
    expect(previewFixChange.mock.calls[0][1]).toEqual(expect.objectContaining({ effectiveAt: "", effectiveLocalDate: "", effectiveLocalTime: "" }));
    expect(previewChange).not.toHaveBeenCalled();

    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));

    expect(fixChange).toHaveBeenCalledWith("a1", expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1200" }));
  });

  it("hides Fix/Undo for a reversal and for an already-fixed change", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([
      { id: "a1", kind: "cash_in", effectiveLocalDate: "2026-01-01", effects: [] },
      { id: "a2", kind: "reversal", effectiveLocalDate: "2026-01-02", reversesActivityId: "a1", effects: [] },
    ]);

    renderPage();
    const list = await screen.findByTestId("activity-list");
    const items = await within(list).findAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(within(items[0]).queryByRole("button", { name: "Fix" })).not.toBeInTheDocument();
    expect(within(items[0]).queryByRole("button", { name: "Undo" })).not.toBeInTheDocument();
    expect(within(items[1]).queryByRole("button", { name: "Fix" })).not.toBeInTheDocument();
  });

  it("uses a currency select from the supported catalog instead of free text", async () => {
    const { selectValues } = await import("@/test/catalog");
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    const currency = within(form).getByLabelText("Currency");
    expect(currency.tagName).toBe("SELECT");
    await waitFor(() => {
      expect(selectValues(currency)).toEqual(["USD", "SGD", "CNY"]);
    });
  });

  it("shows the History Origin timezone after history has started", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    expect(await screen.findByTestId("history-origin-timezone")).toHaveTextContent("UTC");
  });

  it("fills the other FX amount from a direct USD/SGD quote when household base is CNY", async () => {
    bootstrap.mockResolvedValue({
      household: { id: "h1", name: "Test", baseCurrency: "CNY", createdAt: "", updatedAt: "" },
      members: [],
      institutions: [],
      groups: [],
    });
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings", defaultCurrency: "CNY" }, ownership: [], latestValue: null }]);
    currentFXQuote.mockResolvedValue({
      id: "fx-1",
      baseCurrency: "USD",
      quoteCurrency: "SGD",
      rate: "0.92",
      quotedAt: "2026-08-23T00:00:00Z",
    });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeFXConversion);
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "brokerage-1");
    const currencies = within(form).getAllByLabelText("Currency");
    await userEvent.selectOptions(currencies[0], "USD");
    await userEvent.selectOptions(currencies[1], "SGD");
    await userEvent.type(within(form).getByLabelText("Sold"), "100");
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));
    await waitFor(() => expect(["92", "92.00"]).toContain((within(form).getByLabelText("Bought") as HTMLInputElement).value));
  });

  it("submits an FX fee and keeps Preview disabled until bought currency is chosen", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings", defaultCurrency: "CNY" }, ownership: [], latestValue: null }]);
    previewChange.mockResolvedValue({ activity: { id: "fx-1", kind: "fx_conversion", effects: [] }, effects: [], resulting: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeFXConversion);
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "brokerage-1");
    await userEvent.type(within(form).getByLabelText("Sold"), "100");
    await userEvent.type(within(form).getByLabelText("Fee"), "1.50");
    expect(within(form).getByRole("button", { name: "Preview" })).toBeDisabled();

    const currencies = within(form).getAllByLabelText("Currency");
    await userEvent.selectOptions(currencies[0], "USD");
    await userEvent.selectOptions(currencies[1], "SGD");
    await userEvent.type(within(form).getByLabelText("Bought"), "92");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(previewChange).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "fx_conversion",
        sold: "100",
        soldCurrency: "USD",
        bought: "92",
        boughtCurrency: "SGD",
        fee: "1.50",
        feeCurrency: "USD",
      }),
    );
  });

  it("limits sell to positive holdings of the settlement account and hides create instrument", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings", defaultCurrency: "USD" }, ownership: [], latestValue: null }]);
    listInstruments.mockResolvedValue([
      { id: "instrument-1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "instrument-2", name: "Apple", quoteCurrency: "USD", quoteSource: "manual" },
    ]);
    holdingsByAccounts.mockResolvedValue({
      "brokerage-1": [
        { id: "holding-1", accountId: "brokerage-1", instrumentId: "instrument-1", quantity: "10" },
        { id: "holding-2", accountId: "brokerage-1", instrumentId: "instrument-2", quantity: "0" },
      ],
    });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeTrade);
    await userEvent.selectOptions(within(form).getByLabelText("Settlement account"), "brokerage-1");
    await userEvent.selectOptions(within(form).getByLabelText("Side"), "sell");
    expect(within(form).queryByRole("button", { name: "Create instrument" })).not.toBeInTheDocument();
    const instrument = within(form).getByLabelText("Instrument");
    expect(instrument).not.toBeDisabled();
    expect(within(instrument).getByRole("option", { name: /NVIDIA/ })).toBeInTheDocument();
    expect(within(instrument).queryByRole("option", { name: /Apple/ })).not.toBeInTheDocument();
  });

  it("disables value-update Preview while the new value equals the current amount", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([
      {
        account: { id: "acc-1", name: "Checking", trackingMode: "balance", defaultCurrency: "USD" },
        ownership: [],
        latestValue: { amount: { amount: "1000", currency: "USD" } },
      },
    ]);

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Type of change"), ChangeCommandKind.ChangeValueUpdate);
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
    expect(within(form).getByLabelText("New value")).toHaveValue("1000");
    expect(within(form).getByRole("button", { name: "Preview" })).toBeDisabled();
    await userEvent.clear(within(form).getByLabelText("New value"));
    await userEvent.type(within(form).getByLabelText("New value"), "1100");
    expect(within(form).getByRole("button", { name: "Preview" })).toBeEnabled();
  });

  it("loads every activity across pages when timestamps are identical", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    const stamp = "2026-01-01T12:00:00.000Z";
    const activities = Array.from({ length: 51 }, (_, index) => ({
      id: `act-${String(index).padStart(3, "0")}`,
      kind: "cash_in",
      reason: "contribution",
      effectiveAt: stamp,
      createdAt: stamp,
      effectiveLocalDate: "2026-01-01",
      effects: [{ accountId: "acc-1", money: { amount: String(index + 1), currency: "USD" } }],
    }));
    listActivities.mockResolvedValue(activities);

    renderPage();
    const list = await screen.findByTestId("activity-list");
    expect(within(list).getAllByRole("listitem")).toHaveLength(50);
    expect(list).toHaveTextContent("Added $51.00 to Checking");
    expect(list).not.toHaveTextContent("Added $1.00 to Checking");

    await userEvent.click(screen.getByTestId("history-load-more"));
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(51));
    expect(list).toHaveTextContent("Added $1.00 to Checking");
    expect(screen.queryByTestId("history-load-more")).not.toBeInTheDocument();
    expect(listActivityPage.mock.calls.some((call) => !call[0]?.afterId && call[0]?.limit === 50)).toBe(true);
    expect(
      listActivityPage.mock.calls.some(
        (call) =>
          call[0]?.afterId === "act-001" &&
          call[0]?.afterEffectiveAt === stamp &&
          call[0]?.afterCreatedAt === stamp,
      ),
    ).toBe(true);
    const renderedIds = within(list).getAllByRole("listitem").map((item) => item.textContent);
    expect(new Set(renderedIds).size).toBe(51);
  });
});
