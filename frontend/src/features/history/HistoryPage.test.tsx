import { RecordChangeForm } from "./RecordChangeForm";
import { activityToInitialCommand } from "./activityToCommand";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { emptyChangeRequest } from "./activityToCommand";
import { useFixChange } from "@/queries/history";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, renderHook, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { queryKeys } from "@/queries/keys";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { paginateActivityPage } from "@/test/activityPage";
import { HistoryPage } from "./HistoryPage";
import { AccountActionSheet } from "@/features/accounts/AccountActionSheets";
import { localDateInTimeZone } from "@/features/history/historyStartDate";
import i18n from "@/i18n";
import { toast } from "sonner";
import { ChangeCommandKind } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";

const { bootstrap } = vi.hoisted(() => ({ bootstrap: vi.fn() }));
const historyOrigin = vi.fn();
const startHistory = vi.fn();
const startHistoryWithCosts = vi.fn();
const startHistoryOnDate = vi.fn();
const startingPointDraft = vi.fn();
const listActivities = vi.fn();
const listActivityPage = vi.fn();
const getActivity = vi.fn();
const previewChange = vi.fn();
const previewRepeat = vi.fn();
const confirmRepeat = vi.fn();
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
const scanHealth = vi.fn();
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
    StartHistoryOnDate: (...args: unknown[]) => startHistoryOnDate(...args),
    StartingPointDraft: () => startingPointDraft(),
    Activity: (...args: unknown[]) => getActivity(...args),
    ListActivityPage: (...args: unknown[]) => listActivityPage(...args),
    PreviewRepeatChange: (...args: unknown[]) => previewRepeat(...args),
    ConfirmRepeatChange: (...args: unknown[]) => confirmRepeat(...args),
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
    ScanMarketDataHealth: () => scanHealth(),
    StartRefreshFX: (...args: unknown[]) => startRefresh(refreshFX)(...args as [string, ...unknown[]]),
    StartRefreshInstrument: (...args: unknown[]) => startRefresh(refreshInstrument)(...args as [string, ...unknown[]]),
    CancelRefresh: vi.fn(async () => undefined),
    SearchInstruments: vi.fn().mockResolvedValue([]),
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

function renderPage(filters?: { kinds?: string[]; accountId?: string; instrumentId?: string; from?: string; to?: string }, focus?: { activityId: string; requestId: number }, onFiltersChange?: (filters: { kinds?: string[]; accountId?: string; instrumentId?: string; from?: string; to?: string }) => void) {
  const queryClient = createTestQueryClient();
  const rendered = render(
    <QueryClientProvider client={queryClient}>
      <HistoryPage navigationFilters={filters} navigationActivityId={focus?.activityId} navigationRequestId={focus?.requestId} onFiltersChange={onFiltersChange} />
    </QueryClientProvider>,
  );
  return { ...rendered, queryClient };
}

beforeEach(() => {
  historyOrigin.mockReset();
  startHistory.mockReset();
  startHistoryWithCosts.mockReset();
  startHistoryOnDate.mockReset();
  startingPointDraft.mockReset();
  listActivities.mockReset();
  listActivityPage.mockReset();
  getActivity.mockReset();
  previewRepeat.mockReset(); previewRepeat.mockResolvedValue({ planId: "repeat-plan", resulting: [{ name: "Checking", amount: "200", currency: "USD" }] });
  confirmRepeat.mockReset(); confirmRepeat.mockResolvedValue({});
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
  scanHealth.mockReset();
  scanHealth.mockResolvedValue({ issueCount: 0, issues: [] });
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
  it("reuses date shortcuts and moves periods through today in the history timezone", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-27T17:00:00Z"));
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "Asia/Singapore", startedAt: "2026-09-01T00:00:00+08:00" });
    try {
      renderPage({ accountId: "acc-1" });
      await userEvent.click(await screen.findByRole("button", { name: "Last week" }));
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: "acc-1", fromLocalDate: "2026-09-22", toLocalDate: "2026-09-28" })));
      await userEvent.click(await screen.findByRole("button", { name: "Previous period" }));
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ fromLocalDate: "2026-09-15", toLocalDate: "2026-09-21" })));
      await userEvent.click(await screen.findByRole("button", { name: "Next period" }));
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ fromLocalDate: "2026-09-22", toLocalDate: "2026-09-28" })));
      expect(await screen.findByRole("button", { name: "Next period" })).toBeDisabled();
      await userEvent.click(screen.getByRole("button", { name: "1D" }));
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ fromLocalDate: "2026-09-28", toLocalDate: "2026-09-28" })));
    } finally { vi.useRealTimers(); }
  });

  it("applies navigation filters from another page", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "instrument-1", name: "ETF", archivedAt: null }]);
    renderPage({ kinds: ["buy"], accountId: "acc-1", instrumentId: "instrument-1", from: "2026-01-01", to: "2026-01-31" });
    await screen.findByText("No activity yet");
    expect(listActivityPage).toHaveBeenCalledWith(expect.objectContaining({
      accountId: "acc-1",
      instrumentId: "instrument-1",
      kinds: ["buy"],
      fromLocalDate: "2026-01-01",
      toLocalDate: "2026-01-31",
      limit: 50,
    }));
  });

  it("opens an exact activity outside the first filtered page and keeps the list context", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    const activity = { id: "exact-activity", kind: "cash_in", effectiveAt: "2026-01-05T12:00:00Z", effectiveLocalDate: "2026-01-05", createdAt: "2026-01-05T12:00:00Z", note: "Exact record note", effects: [], resulting: [], repeatEligible: false, repeatIneligibleReason: "" };
    getActivity.mockResolvedValue(activity);
    listActivityPage.mockResolvedValue({ activities: [{ ...activity, id: "other-record", note: "Other record" }], hasMore: true, next: { id: "other-record", effectiveAt: activity.effectiveAt, createdAt: activity.createdAt } });
    const filters = { accountId: "acc-1", from: "2026-01-05", to: "2026-01-05" };
    const view = renderPage(filters, { activityId: activity.id, requestId: 1 });
    expect(await within(await screen.findByRole("dialog")).findByText("Exact record note")).toBeInTheDocument();
    expect(getActivity).toHaveBeenCalledWith(activity.id);
    expect(listActivityPage).toHaveBeenCalledWith(expect.objectContaining({ accountId: "acc-1", fromLocalDate: "2026-01-05", toLocalDate: "2026-01-05", limit: 50 }));
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByRole("combobox", { name: "Account" })).toHaveValue("acc-1");
    view.rerender(<QueryClientProvider client={view.queryClient}><HistoryPage navigationFilters={filters} navigationActivityId={activity.id} navigationRequestId={2} /></QueryClientProvider>);
    expect(await within(await screen.findByRole("dialog")).findByText("Exact record note")).toBeInTheDocument();
  }, 10000);

  it("reports the current History filters for return navigation without resetting edited fields", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    const onFiltersChange = vi.fn();
    renderPage(undefined, { activityId: "", requestId: 1 }, onFiltersChange);
    const user = userEvent.setup();
    await user.selectOptions(await screen.findByRole("combobox", { name: "Account" }), "acc-1");
    await waitFor(() => expect(onFiltersChange).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: "acc-1" })));
    await user.selectOptions(screen.getByRole("combobox", { name: "Kind" }), "cash_in");
    await waitFor(() => expect(onFiltersChange).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: "acc-1", kinds: ["cash_in"] })));
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(onFiltersChange).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: undefined, kinds: undefined })));
  });

  it.each(["en", "zh-CN", "zh-TW"])("keeps archived filters selectable after clearing and updates rows (%s)", async (locale) => {
    await i18n.changeLanguage(locale);
    const t = i18n.t.bind(i18n);
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([
      { account: { id: "archived-account", name: "Old brokerage", archivedAt: "2026-01-01" }, ownership: [] },
      { account: { id: "other-archived-account", name: "Hidden archive", archivedAt: "2026-01-01" }, ownership: [] },
      { account: { id: "active-account", name: "Current brokerage", archivedAt: null }, ownership: [] },
    ]);
    listInstruments.mockResolvedValue([
      { id: "archived-instrument", name: "Full old fund", symbol: "000001", shortName: "Family fund", archivedAt: "2026-01-01" },
      { id: "other-archived-instrument", name: "Hidden fund", shortName: "Family fund", archivedAt: "2026-01-01" },
      { id: "active-instrument", name: "Full new fund", symbol: "000002", shortName: "Family fund", archivedAt: null },
    ]);
    listActivityPage.mockImplementation(async (request: { accountId?: string; instrumentId?: string }) => ({
      activities: [{ id: "synthetic-row", kind: "buy", effectiveLocalDate: "2026-01-01",
        tradeDetail: { instrumentId: request.instrumentId ?? "active-instrument", quantity: "2", gross: { amount: "20", currency: "USD" } },
        effects: [{ accountId: request.accountId ?? "active-account", role: "principal", money: { amount: "20", currency: "USD" } }],
      }], hasMore: false,
    }));
    const filters = { accountId: "archived-account", instrumentId: "archived-instrument" };
    let view = renderPage(filters);
    try {
      let account = await screen.findByRole("combobox", { name: t("history.filterAccount") });
      let instrument = screen.getByRole("combobox", { name: t("history.filterInstrument") });
      expect(account).toHaveValue(filters.accountId);
      expect(instrument).toHaveValue(filters.instrumentId);
      expect(within(account).getByRole("option", { name: `Old brokerage · ${t("common.archived")}` })).toHaveProperty("selected", true);
      expect(within(instrument).getByRole("option", { name: `Family fund · 000001 · ${t("common.archived")}` })).toHaveProperty("selected", true);
      expect(within(account).getByRole("option", { name: `Hidden archive · ${t("common.archived")}` })).toBeInTheDocument();
      expect(within(instrument).getByRole("option", { name: `Family fund · ${t("common.archived")}` })).toBeInTheDocument();
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining(filters)));
      expect(screen.getByRole("list")).toHaveTextContent("Family fund");
      expect(screen.getByRole("list")).toHaveTextContent("Old brokerage");

      await userEvent.selectOptions(account, "active-account");
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: "active-account", instrumentId: filters.instrumentId })));
      account = await screen.findByRole("combobox", { name: t("history.filterAccount") });
      expect(within(account).getByRole("option", { name: `Old brokerage · ${t("common.archived")}` })).toBeInTheDocument();
      await waitFor(() => expect(screen.getByRole("list")).toHaveTextContent("Current brokerage"));
      instrument = screen.getByRole("combobox", { name: t("history.filterInstrument") });
      await userEvent.selectOptions(instrument, "active-instrument");
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: "active-account", instrumentId: "active-instrument" })));
      instrument = await screen.findByRole("combobox", { name: t("history.filterInstrument") });
      expect(within(instrument).getByRole("option", { name: `Family fund · 000001 · ${t("common.archived")}` })).toBeInTheDocument();
      expect(instrument).toHaveValue("active-instrument");

      account = screen.getByRole("combobox", { name: t("history.filterAccount") });
      await userEvent.selectOptions(account, "");
      instrument = await screen.findByRole("combobox", { name: t("history.filterInstrument") });
      await userEvent.selectOptions(instrument, "");
      await waitFor(() => {
        const request = listActivityPage.mock.lastCall?.[0];
        expect(request?.accountId).toBeUndefined();
        expect(request?.instrumentId).toBeUndefined();
      });
      account = await screen.findByRole("combobox", { name: t("history.filterAccount") });
      instrument = screen.getByRole("combobox", { name: t("history.filterInstrument") });
      expect(account).toHaveValue(""); expect(instrument).toHaveValue("");
      await userEvent.selectOptions(account, "other-archived-account");
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: "other-archived-account" })));
      instrument = await screen.findByRole("combobox", { name: t("history.filterInstrument") });
      await userEvent.selectOptions(instrument, "other-archived-instrument");
      await waitFor(() => expect(listActivityPage).toHaveBeenLastCalledWith(expect.objectContaining({ accountId: "other-archived-account", instrumentId: "other-archived-instrument" })));
      // App navigation unmounts History; returning reconstructs its saved filters.
      view.unmount();
      view = renderPage(filters);
      expect(await screen.findByRole("combobox", { name: t("history.filterAccount") })).toHaveValue(filters.accountId);
      expect(screen.getByRole("combobox", { name: t("history.filterInstrument") })).toHaveValue(filters.instrumentId);
      await waitFor(() => expect(screen.getByRole("list")).toHaveTextContent("Old brokerage"));
      expect(listAccounts).toHaveBeenCalledWith({ includeArchived: true });
      expect(listInstruments).toHaveBeenCalledWith(true);
    } finally {
      view.unmount();
      await i18n.changeLanguage("en");
    }
  });

  it("treats an empty kinds payload as all activity kinds", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "instrument-1", name: "ETF", archivedAt: null }]);
    renderPage({ kinds: [], accountId: "acc-1", instrumentId: "instrument-1", from: "2026-01-01", to: "2026-01-31" });
    await screen.findByText("No activity yet");
    expect(listActivityPage).toHaveBeenCalledWith(expect.objectContaining({
      accountId: "acc-1",
      instrumentId: "instrument-1",
      fromLocalDate: "2026-01-01",
      toLocalDate: "2026-01-31",
      limit: 50,
    }));
    expect(listActivityPage.mock.calls[0][0].kinds).toBeUndefined();
  });

  it("prompts to Start History when none exists yet", async () => {
    historyOrigin.mockResolvedValue(null);
    renderPage();
    expect(await screen.findByRole("heading", { name: "Start history" })).toBeInTheDocument();
    expect(screen.getByLabelText("Start date")).toHaveTextContent(localDateInTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone) ?? "");
  });

  it("starts history with the resolved settings timezone", async () => {
    historyOrigin.mockResolvedValue(null);
    startHistoryOnDate.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    const timezoneInput = await screen.findByLabelText("Timezone");
    expect(timezoneInput).toHaveValue(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(timezoneInput).toHaveAttribute("readonly");
    const resolvedTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(screen.getByLabelText("Start date")).toHaveTextContent(localDateInTimeZone(resolvedTimezone) ?? "");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));
    expect(startHistoryOnDate).toHaveBeenCalledWith(resolvedTimezone, localDateInTimeZone(resolvedTimezone), null);
    expect(startHistory).not.toHaveBeenCalled();
    expect(startHistoryWithCosts).not.toHaveBeenCalled();
  });

  it("starts History on the selected earlier date", async () => {
    historyOrigin.mockResolvedValue(null);
    startHistoryOnDate.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const today = localDateInTimeZone(timezone)!;
    const selected = `${today.slice(0, 7)}-01`;
    await userEvent.click(await screen.findByLabelText("Start date"));
    const calendar = await screen.findByRole("grid");
    await userEvent.click(calendar.querySelector(`[data-day="${selected}"] button`) as HTMLElement);
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));

    expect(startHistoryOnDate).toHaveBeenCalledWith(timezone, selected, null);
    expect(startHistory).not.toHaveBeenCalled();
  });

  it("starts history with unit costs when holdings already exist", async () => {
    historyOrigin.mockResolvedValue(null);
    startingPointDraft.mockResolvedValue([
      { holdingId: "h1", instrumentId: "i1", instrumentName: "Vanguard S&P 500 ETF", currency: "USD", quantity: "100", unitCost: "" },
    ]);
    startHistoryOnDate.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    const timezoneInput = await screen.findByLabelText("Timezone");
    expect(timezoneInput).toHaveValue(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(timezoneInput).toHaveAttribute("readonly");
    const costInput = await screen.findByLabelText(/Vanguard S&P 500 ETF/);
    await userEvent.type(costInput, "430.25");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));
    const resolvedTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(startHistoryOnDate).toHaveBeenCalledWith(resolvedTimezone, localDateInTimeZone(resolvedTimezone), { h1: "430.25" });
    expect(startHistoryWithCosts).not.toHaveBeenCalled();
    expect(startHistory).not.toHaveBeenCalled();
  });

  it("previews then records a money_added change", async () => {
    const success = vi.spyOn(toast, "success");
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    previewChange.mockResolvedValue({
      activity: { id: "a1", kind: "cash_in", effects: [] },
      effects: [],
      resulting: [{ target: "account_value", name: "Checking", amount: "1000", currency: "USD" }],
    });
    recordChange.mockResolvedValue({ activity: { id: "a1", kind: "cash_in" }, effects: [], resulting: [] });

    renderPage({ from: "2026-08-01", to: "2026-08-31" });
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });

    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "1000");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));

    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1000", currency: "USD" }));
    expect(await within(form).findByRole("status")).toHaveTextContent("Checking");

    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
    expect(recordChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1000" }));
    await waitFor(() => expect(success).toHaveBeenCalledWith(i18n.t("history.recorded"), expect.objectContaining({ action: expect.objectContaining({ label: i18n.t("insights.viewInHistory"), onClick: expect.any(Function) }) })));
    success.mockRestore();
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

  it("validates adjustment unit costs before IPC and accepts an exact eight-place correction", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "i1", name: "QQQM", quoteCurrency: "USD" }]);
    holdingsByAccounts.mockResolvedValue({
      "acc-1": [{ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "3" }],
    });
    previewChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangePositionAdjustment}`) }));
    await userEvent.selectOptions(within(form).getByLabelText("Holding"), "h1");
    await userEvent.type(within(form).getByLabelText("Quantity"), "7");
    const cost = within(form).getByLabelText(/Unit cost/);
    const previewButton = within(form).getByRole("button", { name: "Preview" });
    for (const invalid of ["296.6742857142857", "-1", "1e2", "1,000", "01", "1000000000000"]) {
      await userEvent.clear(cost);
      await userEvent.type(cost, invalid);
      expect(cost).toHaveAttribute("aria-invalid", "true");
      expect(within(form).getByRole("alert")).toHaveTextContent("8 decimal places");
      expect(previewButton).toBeDisabled();
      await userEvent.click(previewButton);
    }
    expect(previewChange).not.toHaveBeenCalled();
    for (const valid of ["0", "117.513", "999999999999.99999999", ""]) {
      await userEvent.clear(cost);
      if (valid) await userEvent.type(cost, valid);
      expect(cost).toHaveAttribute("aria-invalid", "false");
      expect(previewButton).toBeEnabled();
    }
    await userEvent.type(cost, "296.67428571");
    await userEvent.click(previewButton);
    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ unitCost: "296.67428571", quantity: "7" }));
  });

  it("clears a failed adjustment preview when the cost is corrected", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "i1", name: "QQQM", quoteCurrency: "USD" }]);
    holdingsByAccounts.mockResolvedValue({
      "acc-1": [{ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "3" }],
    });
    previewChange.mockRejectedValueOnce(new Error(JSON.stringify({ code: "validation", field: "unitCost", message: "must be a canonical non-negative decimal with up to eight fractional digits" })));
    previewChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangePositionAdjustment}`) }));
    await userEvent.selectOptions(within(form).getByLabelText("Holding"), "h1");
    await userEvent.type(within(form).getByLabelText("Quantity"), "7");
    const cost = within(form).getByLabelText(/Unit cost/);
    await userEvent.type(cost, "295");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent("Unit cost: Check this field and try again.");
    await userEvent.clear(cost);
    await userEvent.type(cost, "295.378");
    expect(within(form).queryByRole("alert")).not.toBeInTheDocument();
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(await within(form).findByRole("button", { name: "Confirm" })).toBeEnabled();
    expect(previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ unitCost: "295.378" }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeCashDividend}`) }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeCashDividend}`) }));
    const holdingSelect = within(form).getByLabelText("Holding");
    expect(within(holdingSelect).getByRole("option", { name: /Apple/ })).toBeInTheDocument();
    expect(within(holdingSelect).queryByRole("option", { name: /NVIDIA/ })).not.toBeInTheDocument();
    expect(within(holdingSelect).queryByRole("option", { name: /holding-gone/ })).not.toBeInTheDocument();
  });

  it("keeps compact Buy choices distinct and submits the selected instrument ID", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockResolvedValue([
      { account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
    ]);
    const metadata = { name: "Synthetic exceptionally long full legal instrument name", quoteCurrency: "USD", quoteSource: "manual", providerKey: "synthetic-provider", providerSymbol: "long-provider-symbol" };
    listInstruments.mockResolvedValue([
      { ...metadata, id: "instrument-1", shortName: "Fund", symbol: "ABC", marketCode: "X1" },
      { ...metadata, id: "instrument-2", shortName: "Fund", symbol: "ABC", marketCode: "X2" },
      { ...metadata, id: "instrument-3", shortName: "", symbol: "CODE" },
      { ...metadata, id: "instrument-4", shortName: "SAME", symbol: "SAME" },
    ]);
    holdingsByAccounts.mockResolvedValue({ "brokerage-1": [] });
    previewChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <AccountActionSheet action="buy" historyStarted record={{
          account: {
            id: "brokerage-1", householdId: "h1", name: "Brokerage", trackingMode: "holdings",
            accountType: "brokerage", balanceSheetRole: "asset", defaultCurrency: "USD", iconKey: "",
            includeInNetWorth: true, includeInPortfolio: true, includeInLiquidAssets: false,
            sortOrder: 0, createdAt: "", updatedAt: "",
          }, ownership: [],
        }} onClose={vi.fn()} onRecorded={vi.fn()} />
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    const form = await screen.findByRole("form", { name: "Record change" });
    await within(form).findByRole("option", { name: "Fund · ABC [X1]" });
    const select = within(form).getByLabelText("Instrument");
    expect(select.tagName).toBe("SELECT");
    expect(within(select).getAllByRole("option").map((option) => option.textContent)).toEqual([
      i18n.t("history.selectEmpty"), "Fund · ABC [X1]", "Fund · ABC [X2]", "CODE", "SAME",
    ]);
    select.focus();
    expect(select).toHaveFocus();
    await user.selectOptions(select, "instrument-2");
    expect(select).toHaveValue("instrument-2");
    await user.tab();
    expect(select).not.toHaveFocus();
    await user.type(within(form).getByLabelText("Quantity"), "2");
    await user.type(within(form).getByLabelText("Gross total"), "20");
    await user.click(within(form).getByRole("button", { name: "Preview" }));
    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ instrumentId: "instrument-2" }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeTrade}`) }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeTrade}`) }));
    await userEvent.selectOptions(within(form).getByLabelText("Settlement account"), "brokerage-1");
    await userEvent.selectOptions(within(form).getByLabelText("Instrument"), "instrument-1");
    await userEvent.type(within(form).getByLabelText("Quantity"), "10");
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));

    await waitFor(() => expect(within(form).getByLabelText("Gross total")).toHaveValue("1234.56"));
    expect(within(form).getByLabelText("Gross total")).not.toHaveAttribute("disabled");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ gross: "1234.56", grossCurrency: "EUR" }));
  });

  it.each(["USD", "SGD"])("clears trade amounts when switching instruments to %s and recalculates from the new quote", async (currency) => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([
      { id: "boxx", name: "BOXX", quoteCurrency: "USD", quoteSource: "manual" },
      { id: "qqqm", name: "QQQM", quoteCurrency: currency, quoteSource: "manual" },
    ]);
    listAccounts.mockResolvedValue([{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null }]);
    holdingsByAccounts.mockResolvedValue({ "brokerage-1": [
      { id: "h-boxx", accountId: "brokerage-1", instrumentId: "boxx", quantity: "48" },
      { id: "h-qqqm", accountId: "brokerage-1", instrumentId: "qqqm", quantity: "10" },
    ] });
    currentInstrumentQuote.mockImplementation(async (id: string) => ({
      id: `quote-${id}`, instrumentId: id, unitPrice: id === "boxx" ? "118" : "305",
      currency: id === "boxx" ? "USD" : currency, quotedAt: "2026-08-23T00:00:00Z",
    }));
    previewChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeTrade}`) }));
    await userEvent.selectOptions(within(form).getByLabelText("Settlement account"), "brokerage-1");
    const instrument = within(form).getByLabelText("Instrument");
    const quantity = within(form).getByLabelText("Quantity");
    const gross = within(form).getByLabelText("Gross total");
    const fee = within(form).getByLabelText("Fee (optional)");
    await userEvent.selectOptions(instrument, "boxx");
    await userEvent.type(quantity, "2");
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));
    await waitFor(() => expect(gross).toHaveValue("236"));
    await userEvent.type(fee, "1");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(await within(form).findByRole("button", { name: "Confirm" })).toBeEnabled();

    await userEvent.selectOptions(instrument, "qqqm");
    expect(quantity).toHaveValue("");
    expect(gross).toHaveValue("");
    expect(fee).toHaveValue("");
    expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Preview" })).toBeDisabled();
    expect(within(form).getByLabelText("Settlement account")).toHaveValue("brokerage-1");
    await userEvent.type(quantity, "3");
    await userEvent.click(within(form).getByRole("button", { name: "Calculate" }));
    await waitFor(() => expect(gross).toHaveValue("915"));
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    expect(previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ instrumentId: "qqqm", holdingId: "h-qqqm", quantity: "3", gross: "915", grossCurrency: currency, fee: "", feeCurrency: currency }));

    await userEvent.clear(gross);
    await userEvent.type(gross, "900");
    await userEvent.selectOptions(instrument, "boxx");
    expect(quantity).toHaveValue("");
    expect(gross).toHaveValue("");
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeTrade}`) }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeFXConversion}`) }));
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
    await userEvent.click(await within(list).findByRole("button", { name: "Actions" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Undo" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Undo" }));
    expect(undoChange).toHaveBeenCalledWith("a1");
  });

  it("uses archived account names in history but excludes them from the account filter", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listAccounts.mockImplementation(async (filter?: { includeArchived?: boolean }) => [
      { account: { id: "active", name: "Checking", trackingMode: "balance" }, ownership: [], latestValue: null },
      ...(filter?.includeArchived ? [{ account: { id: "archived", name: "Closed Savings", trackingMode: "balance", archivedAt: "2026-01-02" }, ownership: [], latestValue: null }] : []),
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
    await userEvent.click(await within(list).findByRole("button", { name: "Actions" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Fix" }));

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
    expect(within(items[0]).queryByRole("button", { name: "Actions" })).not.toBeInTheDocument();
    expect(within(items[1]).queryByRole("button", { name: "Actions" })).not.toBeInTheDocument();
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeFXConversion}`) }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeFXConversion}`) }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeTrade}`) }));
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
    await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeValueUpdate}`) }));
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


it("does not advertise today's dirty cursor as snapshot repair", async () => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-09-01T00:00:00Z" });
  dailySnapshotState.mockResolvedValue({ householdId: "h1", dirtyFrom: "2026-09-17", lastCompletedClosedOn: "2026-09-16" });
  scanHealth.mockResolvedValue({ issueCount: 0, issues: [] });
  renderPage();
  const repair = await screen.findByRole("button", { name: "Repair snapshots" });
  expect(repair).toBeDisabled();
  expect(screen.queryByText(/Snapshots need repair from/)).not.toBeInTheDocument();
});

it("repairs the same closed range advertised by Data Health", async () => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "Asia/Singapore", startedAt: "2026-09-01T00:00:00Z" });
  scanHealth.mockResolvedValue({ issueCount: 1, issues: [{ kind: "snapshot_outdated", executable: true, rangeStart: "2026-09-02", rangeEnd: "2026-09-16" }] });
  renderPage();
  const repair = await screen.findByRole("button", { name: "Repair snapshots" });
  await waitFor(() => expect(repair).toBeEnabled());
  await userEvent.click(repair);
  expect(rebuildHistoricalSnapshots).toHaveBeenCalledWith("2026-09-02", "2026-09-16");
});

vi.mock("@/features/liquidity/ProductSheets", () => ({
  ProductDetailSheet: ({ productId }: { productId: string }) => <div data-testid="managed-product">{productId}</div>,
}));

it("routes product activities to product management instead of generic fix or undo", async () => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  listActivities.mockResolvedValue([{ id: "managed", kind: "cash_in", reason: "interest", effectiveLocalDate: "2026-09-22", effects: [], productContext: { productId: "deposit-1", operationId: "operation-1", purpose: "interest" } }]);
  renderPage();
  const list = await screen.findByTestId("activity-list");
  expect(within(list).queryByRole("button", { name: "Actions" })).not.toBeInTheDocument();
  await userEvent.click(within(list).getByRole("button", { name: "Manage product operation" }));
  expect(await screen.findByTestId("managed-product")).toHaveTextContent("deposit-1");
  expect(previewFixChange).not.toHaveBeenCalled();
  expect(undoChange).not.toHaveBeenCalled();
});


// NW-001: the confirmation must belong to the exact command that was reviewed.
for (const correcting of [false, true]) {
  it(`rejects an obsolete ${correcting ? "Fix" : "Record"} preview and freezes confirmation input`, async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
    const previewCall = correcting ? previewFixChange : previewChange;
    const recordCall = correcting ? fixChange : recordChange;
    let resolvePreview!: (value: unknown) => void;
    let resolveRecord!: (value: unknown) => void;
    previewCall.mockImplementation(() => new Promise((resolve) => { resolvePreview = resolve; }));
    recordCall.mockImplementation(() => new Promise((resolve) => { resolveRecord = resolve; }));
    if (correcting) listActivities.mockResolvedValue([{
      id: "a1", kind: "cash_in", effectiveLocalDate: "2026-01-01", reason: "contribution",
      effects: [{ role: "amount", direction: "added", target: "account_value", accountId: "acc-1", money: { amount: "100", currency: "USD" } }],
    }]);
    renderPage();
    if (correcting) {
      const list = await screen.findByTestId("activity-list");
      await userEvent.click(await within(list).findByRole("button", { name: "Actions" }));
      await userEvent.click(await screen.findByRole("menuitem", { name: "Fix" }));
    } else await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });
    if (!correcting) {
      await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
      await userEvent.type(within(form).getByLabelText("Amount"), "100");
    }
    const amount = within(form).getByLabelText("Amount");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewCall).toHaveBeenCalledTimes(1));
    await userEvent.clear(amount);
    await userEvent.type(amount, "200");
    await act(async () => resolvePreview({ resulting: [{ name: "OBSOLETE", amount: "1100", currency: "USD" }] }));
    const nextPreview = await within(form).findByRole("button", { name: "Preview" });
    expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
    expect(within(form).queryByText(/OBSOLETE/)).not.toBeInTheDocument();
    await userEvent.click(nextPreview);
    await act(async () => resolvePreview({ resulting: [{ name: "CURRENT", amount: "1200", currency: "USD" }] }));
    await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(recordCall).toHaveBeenCalledTimes(1));
    expect(amount).toBeDisabled();
    const submitted = recordCall.mock.calls[0][correcting ? 1 : 0];
    expect(submitted.amount).toBe("200");
    await userEvent.type(amount, "9");
    expect(submitted.amount).toBe("200");
    await act(async () => resolveRecord({ activity: {}, effects: [], resulting: [] }));
  });
}

it("does not attach an old preview to a closed and reopened form", async () => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  const resolvePreviews: ((value: unknown) => void)[] = [];
  previewChange.mockImplementation(() => new Promise((resolve) => { resolvePreviews.push(resolve); }));
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
  let form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
  await userEvent.type(within(form).getByLabelText("Amount"), "100");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(previewChange).toHaveBeenCalled());
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("form", { name: "Record change" })).not.toBeInTheDocument());
  await userEvent.click(screen.getByRole("button", { name: /record change/i }));
  form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
  await userEvent.type(within(form).getByLabelText("Amount"), "200");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(resolvePreviews).toHaveLength(2));
  await act(async () => resolvePreviews[1]({ resulting: [{ name: "NEW SESSION", amount: "1200", currency: "USD" }] }));
  expect(await within(form).findByRole("button", { name: "Confirm" })).toBeEnabled();
  await act(async () => resolvePreviews[0]({ resulting: [{ name: "OLD SESSION", amount: "1100", currency: "USD" }] }));
  await waitFor(() => expect(within(form).getByRole("button", { name: "Confirm" })).toBeEnabled());
  expect(within(form).getByRole("status")).toHaveTextContent("NEW SESSION");
  expect(within(form).queryByText(/OLD SESSION/)).not.toBeInTheDocument();
});

it.each([false, true])("transfers a position using toAccountId (target already holds instrument: %s)", async (hasTargetHolding) => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([
    { account: { id: "from", name: "Source Brokerage", trackingMode: "holdings", defaultCurrency: "USD", balanceSheetRole: "asset" }, ownership: [] },
    { account: { id: "to", name: "Empty Brokerage", trackingMode: "holdings", defaultCurrency: "USD", balanceSheetRole: "asset" }, ownership: [] },
  ]);
  listInstruments.mockResolvedValue([{ id: "i1", name: "ETF", quoteCurrency: "USD", quoteSource: "manual", archivedAt: null }]);
  holdingsByAccounts.mockResolvedValue({ from: [{ id: "h1", instrumentId: "i1", quantity: "10" }], to: hasTargetHolding ? [{ id: "h2", instrumentId: "i1", quantity: "5" }] : [] });
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("radio", { name: "Investment transfer" }));
  const from = form.querySelector<HTMLSelectElement>("#change-from-holding")!;
  const to = form.querySelector<HTMLSelectElement>("#change-to-account")!;
  await waitFor(() => expect(from.options.length).toBe(hasTargetHolding ? 3 : 2));
  await userEvent.selectOptions(from, "h1");
  expect(to).not.toBeNull();
  expect(Array.from(to.options).some((o) => o.text.includes("Empty Brokerage"))).toBe(true);
  expect(Array.from(to.options).some((o) => o.value === "from")).toBe(false);
  await userEvent.selectOptions(to, "to");
  previewChange.mockResolvedValue({ resulting: [] });
  await userEvent.type(within(form).getByLabelText(/Quantity/), "1");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ fromHoldingId: "h1", toAccountId: "to", toHoldingId: "", quantity: "1" })));
});

it.each([false, true])("fixes an existing investment transfer with an existing target holding (change target: %s)", async (changeTarget) => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue(["from", "to", "other", "empty", "wrong"].map((id) => ({ account: { id, name: `${id} Brokerage`, accountType: "brokerage", trackingMode: "holdings", defaultCurrency: "USD", balanceSheetRole: "asset" }, ownership: [] })));
  listInstruments.mockResolvedValue([{ id: "i1", name: "ETF", quoteCurrency: "USD", archivedAt: null }, { id: "i2", name: "Other ETF", quoteCurrency: "USD", archivedAt: null }]);
  holdingsByAccounts.mockResolvedValue({ from: [{ id: "h1", instrumentId: "i1", quantity: "8" }], to: [{ id: "h2", instrumentId: "i1", quantity: "2" }], other: [{ id: "h3", instrumentId: "i1", quantity: "0" }], empty: [], wrong: [{ id: "h4", instrumentId: "i2", quantity: "1" }] });
  listActivities.mockResolvedValue([{
    id: "transfer-1", kind: "position_transfer", effectiveLocalDate: "2026-01-02", note: "Original transfer",
    effects: [
      { role: "transfer_from", direction: "removed", target: "holding_quantity", accountId: "from", holdingId: "h1", instrumentId: "i1", quantity: "2" },
      { role: "transfer_to", direction: "added", target: "holding_quantity", accountId: "to", holdingId: "h2", instrumentId: "i1", quantity: "2" },
    ],
  }]);
  previewFixChange.mockResolvedValue({ resulting: [{ name: "ETF", quantity: "3" }] });
  fixChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
  renderPage();
  const list = await screen.findByTestId("activity-list");
  await userEvent.click(await within(list).findByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Fix" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  const target = form.querySelector<HTMLSelectElement>("#change-to-account")!;
  await waitFor(() => expect(target).toHaveValue("to"));
  if (changeTarget) await userEvent.selectOptions(target, "other");
  await userEvent.clear(within(form).getByLabelText(/Quantity/));
  await userEvent.type(within(form).getByLabelText(/Quantity/), "3");
  await userEvent.clear(within(form).getByLabelText("Note"));
  await userEvent.type(within(form).getByLabelText("Note"), "Corrected transfer");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(previewFixChange).toHaveBeenCalledWith("transfer-1", expect.objectContaining({ fromHoldingId: "h1", toHoldingId: changeTarget ? "h3" : "h2", toAccountId: "", quantity: "3", note: "Corrected transfer" })));
  const reviewed = previewFixChange.mock.calls[0][1];
  expect(Array.from(target.options).map((option) => option.value)).not.toEqual(expect.arrayContaining(["empty"]));
  expect(Array.from(target.options).map((option) => option.value)).not.toEqual(expect.arrayContaining(["wrong"]));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(fixChange).toHaveBeenCalledWith("transfer-1", { ...reviewed, mutationId: expect.any(String) }));
  expect(previewChange).not.toHaveBeenCalled();
  expect(recordChange).not.toHaveBeenCalled();
});

async function openTimedCashForm() {
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
  await userEvent.type(within(form).getByLabelText("Amount"), "1");
  return form;
}

async function selectManualMinute(form: HTMLElement, minute: string) {
  await userEvent.click(within(form).getByLabelText("Local time"));
  const lists = await screen.findAllByRole("list", { name: "Local time" });
  await userEvent.click(within(lists[1]).getByRole("button", { name: minute }));
  await userEvent.keyboard("{Escape}");
}

it.each([
  ["UTC", "45.000", "12:34"], ["Asia/Shanghai", "45.000", "20:34"],
  ["UTC", "00.123", "12:34"], ["Asia/Shanghai", "00.123", "20:34"],
])("freezes the exact default time after a partial-minute origin (%s %s)", async (timezone, seconds, localTime) => {
  vi.useFakeTimers({ toFake: ["Date"] });
  const captured = "2026-10-02T12:34:50.789Z";
  vi.setSystemTime(new Date(captured));
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone, startedAt: `2026-10-02T12:34:${seconds}Z` });
  previewChange.mockResolvedValue({ resulting: [{ name: "Checking", amount: "1", currency: "USD" }] });
  recordChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
  try {
    const form = await openTimedCashForm();
    expect(within(form).getByLabelText("Local time")).toHaveTextContent(localTime);
    vi.setSystemTime(new Date("2026-10-02T12:35:55.999Z"));
    await userEvent.type(within(form).getByLabelText("Note"), "Waited across a minute");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ effectiveAt: captured, effectiveLocalDate: "", effectiveLocalTime: "" })));
    expect(within(form).getByText(/Captured time:/)).toHaveTextContent(`${localTime}:50.789`);
    const reviewed = previewChange.mock.calls[0][0];
    vi.setSystemTime(new Date("2026-10-02T12:36:01.000Z"));
    await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(recordChange).toHaveBeenCalledWith({ ...reviewed, mutationId: expect.any(String) }));
  } finally { vi.useRealTimers(); }
});

it.each([
  ["UTC", "45.000", false], ["Asia/Shanghai", "00.123", false],
  ["UTC", "00.000", true], ["Asia/Shanghai", "00.000", true],
])("validates manual origin-minute precision (%s %s allowed=%s)", async (timezone, seconds, allowed) => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-02T12:34:50.789Z"));
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone, startedAt: `2026-10-02T12:34:${seconds}Z` });
  previewChange.mockResolvedValue({ resulting: [] });
  try {
    const form = await openTimedCashForm();
    // Selecting the same displayed minute is still an explicit manual edit.
    await selectManualMinute(form, "34");
    const button = within(form).getByRole("button", { name: "Preview" });
    if (!allowed) {
      expect(button).toBeDisabled();
      expect(within(form).getByText(/This minute starts before the Starting point/)).toBeInTheDocument();
      expect(previewChange).not.toHaveBeenCalled();
      vi.setSystemTime(new Date("2026-10-02T12:35:10.000Z"));
      await selectManualMinute(form, "35");
    }
    await userEvent.click(button);
    await waitFor(() => expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ effectiveAt: "", effectiveLocalDate: "2026-10-02", effectiveLocalTime: `${timezone === "UTC" ? "12" : "20"}:${allowed ? "34" : "35"}` })));
  } finally { vi.useRealTimers(); }
});

it("invalidates a delayed preview when recapturing within the same minute and captures afresh on reopen", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-02T12:34:50.789Z"));
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-10-02T12:34:45.123Z" });
  const resolvePreviews: ((value: unknown) => void)[] = [];
  previewChange.mockImplementation(() => new Promise(resolve => resolvePreviews.push(resolve)));
  try {
    let form = await openTimedCashForm();
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(resolvePreviews).toHaveLength(1));
    vi.setSystemTime(new Date("2026-10-02T12:34:55.123Z"));
    await userEvent.click(within(form).getByRole("button", { name: "Use current time" }));
    await act(async () => resolvePreviews[0]({ resulting: [{ name: "STALE", amount: "1" }] }));
    expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
    await userEvent.click(await within(form).findByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ effectiveAt: "2026-10-02T12:34:55.123Z" })));
    await act(async () => resolvePreviews[1]({ resulting: [] }));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("form", { name: "Record change" })).not.toBeInTheDocument());
    vi.setSystemTime(new Date("2026-10-02T12:35:01.234Z"));
    await userEvent.click(screen.getByRole("button", { name: /record change/i }));
    form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "2");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ effectiveAt: "2026-10-02T12:35:01.234Z" })));
    await act(async () => resolvePreviews[2]({ resulting: [] }));
  } finally { vi.useRealTimers(); }
});

it("keeps captured time across kind changes and invalidates a pending preview when editing the date", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-02T12:34:50.789Z"));
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-10-01T00:00:00.000Z" });
  const resolvePreviews: ((value: unknown) => void)[] = [];
  previewChange.mockImplementation(() => new Promise(resolve => resolvePreviews.push(resolve)));
  try {
    const form = await openTimedCashForm();
    vi.setSystemTime(new Date("2026-10-02T12:35:55.999Z"));
    await userEvent.click(within(form).getByRole("radio", { name: "Money removed" }));
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "1");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ kind: "money_removed", effectiveAt: "2026-10-02T12:34:50.789Z", effectiveLocalDate: "", effectiveLocalTime: "" })));
    await userEvent.click(within(form).getByLabelText("Local date"));
    const calendar = await screen.findByRole("grid");
    await userEvent.click(calendar.querySelector('[data-day="2026-10-01"] button') as HTMLElement);
    await act(async () => resolvePreviews[0]({ resulting: [{ name: "OLD DATE", amount: "1" }] }));
    expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
    expect(within(form).getByText(/Manual time in UTC/)).toBeInTheDocument();
    await userEvent.click(await within(form).findByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ effectiveAt: "", effectiveLocalDate: "2026-10-01", effectiveLocalTime: "12:34" })));
    await act(async () => resolvePreviews[1]({ resulting: [] }));
    await userEvent.click(within(form).getByRole("button", { name: "Use current time" }));
    await userEvent.click(await within(form).findByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ effectiveAt: "2026-10-02T12:35:55.999Z", effectiveLocalDate: "", effectiveLocalTime: "" })));
    await act(async () => resolvePreviews[2]({ resulting: [] }));
  } finally { vi.useRealTimers(); }
});

it("Repeat opens a fresh editable draft, invalidates edits/back and confirms the latest plan once", async () => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "old-income", repeatEligible: true, kind: "cash_in", reason: "income", effectiveAt: "2026-01-01T12:00:00Z", effectiveLocalDate: "2026-01-01", note: "Salary", effects: [{ role: "amount", direction: "added", target: "account_value", accountId: "acc-1", money: { amount: "100", currency: "USD" } }] }]);
  renderPage();
  const list = await screen.findByTestId("activity-list");
  const open = async () => { await userEvent.click(within(list).getByRole("button", { name: "Actions" })); await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" })); return screen.findByRole("form", { name: "Record change" }); };
  let form = await open();
  expect(within(form).getByLabelText("Amount")).toHaveValue("100");
  expect(within(form).getByLabelText("Account")).toHaveValue("acc-1");
  expect(within(form).getByLabelText("Note")).toHaveValue("Salary");
  expect(confirmRepeat).not.toHaveBeenCalled(); expect(recordChange).not.toHaveBeenCalled();
  await userEvent.type(within(form).getByLabelText("Amount"), "0");
  await userEvent.keyboard("{Escape}"); form = await open();
  expect(within(form).getByLabelText("Amount")).toHaveValue("100");
  within(form).getByRole("button", { name: "Preview" }).focus();
  await userEvent.keyboard("{Enter}");
  await within(form).findByRole("button", { name: "Confirm" });
  expect(previewRepeat).toHaveBeenCalledWith("old-income", expect.objectContaining({ amount: "100", reason: "income", accountId: "acc-1" }));
  expect(previewRepeat.mock.calls[0][1].effectiveAt).not.toBe("2026-01-01T12:00:00Z");
  expect(previewRepeat.mock.calls[0][1].effectiveLocalDate).not.toBe("2026-01-01");
  expect(previewRepeat.mock.calls[0][1].mutationId).toBeUndefined();
  await userEvent.click(within(form).getByRole("button", { name: "Back to draft" }));
  expect(within(form).getByLabelText("Amount")).toHaveValue("100");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  confirmRepeat.mockRejectedValueOnce(new Error(JSON.stringify({ code: "stale_preview", field: "previewToken", message: "fresh preview required" })));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await within(form).findByRole("alert");
  expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
  confirmRepeat.mockClear();
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await within(form).findByRole("button", { name: "Confirm" });
  await userEvent.type(within(form).getByLabelText("Amount"), "0");
  expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
  previewRepeat.mockResolvedValueOnce({ planId: "edited-plan", resulting: [{ name: "Checking", amount: "1100", currency: "USD" }] });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  const confirm = await within(form).findByRole("button", { name: "Confirm" });
  const previewsBeforeUncertainConfirm = previewRepeat.mock.calls.length;
  confirmRepeat.mockRejectedValueOnce(new Error("IPC outcome unknown"));
  await userEvent.dblClick(confirm);
  await waitFor(() => expect(confirmRepeat).toHaveBeenCalledTimes(1));
  await within(form).findByRole("alert");
  expect(previewRepeat).toHaveBeenCalledTimes(previewsBeforeUncertainConfirm);
  expect(within(form).getByText(/Retry this retained confirmation first/)).toBeInTheDocument();
  await waitFor(() => expect(within(form).getByRole("button", { name: "Retry same confirmation" })).toBeEnabled());
  await userEvent.click(within(form).getByRole("button", { name: "Retry same confirmation" }));
  await waitFor(() => expect(confirmRepeat).toHaveBeenCalledTimes(2));
  expect(confirmRepeat.mock.calls.map(([id]) => id)).toEqual(["edited-plan", "edited-plan"]);
  expect(recordChange).not.toHaveBeenCalled(); expect(fixChange).not.toHaveBeenCalled();
});

it.each([
  { code: "already_undone", field: "sourceActivityId", previewDisabled: true, help: /Close this draft and choose another eligible activity/ },
  { code: "validation", field: "amount", previewDisabled: false, help: /Review a fresh preview if the source is still eligible/ },
  { code: "stale_preview", field: "previewToken", previewDisabled: false, help: /Review a fresh preview if the source is still eligible/ },
])("does not offer retained confirmation retry after definite $code refusal", async ({ code, field, previewDisabled, help }) => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  const confirm = await within(form).findByRole("button", { name: "Confirm" });
  confirmRepeat.mockRejectedValueOnce(new Error(JSON.stringify({ code, field, message: "definite refusal" })));
  await userEvent.click(confirm);
  await within(form).findByRole("alert");
  expect(within(form).queryByRole("button", { name: "Retry same confirmation" })).not.toBeInTheDocument();
  expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
  expect(within(form).getByText(help)).toBeInTheDocument();
  expect(within(form).getByRole("button", { name: "Preview" }).hasAttribute("disabled")).toBe(previewDisabled);
  expect(confirmRepeat).toHaveBeenCalledTimes(1);
  if (previewDisabled) {
    await userEvent.type(within(form).getByLabelText("Note"), " again");
    expect(within(form).getByRole("button", { name: "Preview" })).toBeDisabled();
    expect(within(form).getByText(help)).toBeInTheDocument();
    expect(previewRepeat).toHaveBeenCalledTimes(1);
  }
});

it.each(["already_undone", "validation"])("keeps a Preview-time %s source refusal blocked after edits", async (code) => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  // History still offers Repeat, but the authoritative source check has changed.
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  previewRepeat.mockRejectedValueOnce(new Error(JSON.stringify({ code, field: "sourceActivityId", message: "source refused" })));
  renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await within(form).findByRole("alert");
  expect(within(form).getByText(/Close this draft and choose another eligible activity/)).toBeInTheDocument();
  expect(within(form).getByRole("button", { name: "Preview" })).toBeDisabled();
  await userEvent.type(within(form).getByLabelText("Note"), " again");
  expect(within(form).getByText(/Close this draft and choose another eligible activity/)).toBeInTheDocument();
  expect(within(form).getByRole("button", { name: "Preview" })).toBeDisabled();
  expect(previewRepeat).toHaveBeenCalledTimes(1);
  expect(confirmRepeat).not.toHaveBeenCalled();
});

it("ignores a source refusal from a Preview invalidated by an edit", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  let rejectPreview: ((error: Error) => void) | undefined;
  previewRepeat.mockImplementationOnce(() => new Promise((_, reject) => { rejectPreview = reject; }));
  renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await userEvent.type(within(form).getByLabelText("Note"), " changed");
  await act(async () => { rejectPreview?.(new Error(JSON.stringify({ code: "already_undone", field: "sourceActivityId", message: "late refusal" }))); });
  expect(within(form).queryByText(/Close this draft and choose another eligible activity/)).not.toBeInTheDocument();
  await waitFor(() => expect(within(form).getByRole("button", { name: "Preview" })).toBeEnabled());
});

it("retains the uncertain confirmation when Back or field edits are attempted", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  confirmRepeat.mockRejectedValueOnce(new Error("IPC outcome unknown"));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await within(form).findByRole("alert");
  await userEvent.click(within(form).getByRole("button", { name: "Back to draft" }));
  expect(within(form).getByRole("button", { name: "Back to draft" })).toBeDisabled();
  expect(within(form).getByLabelText("Amount")).toBeDisabled();
  expect(within(form).getByText(/Retry this retained confirmation first/)).toBeInTheDocument();
  expect(within(form).queryByText(/draft has changed/)).not.toBeInTheDocument();
  expect(within(form).getByRole("button", { name: "Retry same confirmation" })).toBeEnabled();
  expect(previewRepeat).toHaveBeenCalledTimes(1);
  expect(confirmRepeat).toHaveBeenCalledTimes(1);
});

it.each(["planId", undefined])("retains recovery identity when the Repeat plan is unavailable (field %s)", async (field) => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  const confirm = await within(form).findByRole("button", { name: "Confirm" });
  confirmRepeat.mockRejectedValueOnce(new Error(JSON.stringify({ code: "stale_preview", field, message: "plan unavailable" })));
  await userEvent.click(confirm);
  await within(form).findByRole("alert");
  expect(within(form).getByText(i18n.t("history.repeatPlanUnavailableHelp"))).toBeInTheDocument();
  expect(within(form).getByRole("button", { name: "Retry same confirmation" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
  expect(confirmRepeat).toHaveBeenCalledTimes(1);
  expect(previewRepeat).toHaveBeenCalledTimes(1);
  if (field === "planId") {
    for (const language of ["zh-CN", "zh-TW"]) {
      await act(async () => { await i18n.changeLanguage(language); });
      expect(within(form).getByText(i18n.t("history.repeatPlanUnavailableHelp"))).toBeInTheDocument();
    }
  }
});

it("returns keyboard focus to Repeat's Actions trigger after Escape", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  renderPage();
  const trigger = within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" });
  await userEvent.click(trigger);
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  await screen.findByRole("form", { name: "Record change" });
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(trigger).toHaveFocus());
});

it.each(["Escape", "Close", "success"])("returns focus to the Details trigger after Repeat %s", async (exit) => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  renderPage();
  const trigger = within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Details" });
  await userEvent.click(trigger);
  await userEvent.click(await screen.findByRole("button", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  if (exit === "Escape") await userEvent.keyboard("{Escape}");
  else if (exit === "Close") await userEvent.click(screen.getByRole("button", { name: "Close" }));
  else {
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(confirmRepeat).toHaveBeenCalledOnce());
  }
  await waitFor(() => expect(trigger).toHaveFocus());
});

it("returns focus to the History filter if Repeat's Details trigger disappears", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><HistoryPage /></QueryClientProvider>);
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Details" }));
  await userEvent.click(await screen.findByRole("button", { name: "Repeat as new draft" }));
  await screen.findByRole("form", { name: "Record change" });
  listActivities.mockResolvedValue([]);
  await act(async () => { await client.invalidateQueries({ queryKey: ["history", "activityPage"] }); });
  await waitFor(() => expect(screen.queryByTestId("activity-list")).not.toBeInTheDocument());
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.getByLabelText("Kind")).toHaveFocus());
});

it("Repeat cannot preview a missing account", async () => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  listActivities.mockResolvedValue([{ id: "income", repeatEligible: true, kind: "cash_in", reason: "income", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "deleted", money: { amount: "100", currency: "USD" } }] }]);
  renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  expect(within(form).getByRole("button", { name: "Preview" })).toBeDisabled();
  expect(previewRepeat).not.toHaveBeenCalled();
});

it.each(["kind filter", "unloaded next page"])("does not offer Repeat when authoritative eligibility excludes a reversal hidden by %s", async (hiddenBy) => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2025-01-01T00:00:00Z" });
  listActivityPage.mockResolvedValue({ activities: [{ id: "undone-income", repeatEligible: false, kind: "cash_in", reason: "income", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "100", currency: "USD" } }] }], hasMore: hiddenBy === "unloaded next page", next: { id: "cursor", effectiveAt: "2025-12-31T00:00:00Z", createdAt: "2025-12-31T00:00:00Z" } });
  renderPage(hiddenBy === "kind filter" ? { kinds: ["cash_in"] } : undefined);
  const list = await screen.findByTestId("activity-list");
  await userEvent.click(within(list).getByRole("button", { name: "Actions" }));
  expect(screen.queryByRole("menuitem", { name: "Repeat as new draft" })).not.toBeInTheDocument();
  expect(previewRepeat).not.toHaveBeenCalled();
});

it("rejects a source reversed after its editable draft was opened", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2025-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "income", repeatEligible: true, kind: "cash_in", reason: "income", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "100", currency: "USD" } }] }]);
  renderPage();
  const list = await screen.findByTestId("activity-list");
  await userEvent.click(within(list).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  previewRepeat.mockRejectedValueOnce(new Error(JSON.stringify({ code: "already_undone", field: "sourceActivityId", message: "a reversed source cannot be repeated" })));
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(previewRepeat).toHaveBeenCalledWith("income", expect.objectContaining({ reason: "income" })));
  await within(form).findByRole("alert");
  expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
  expect(confirmRepeat).not.toHaveBeenCalled();
});

it.each(["sell", "dividend", "adjust", "transfer"] as const)("distinguishes equal-quantity share classes with duplicate aliases in the %s selector", async (flow) => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([
    { account: { id: "brokerage", name: "Brokerage", trackingMode: "holdings", accountType: "brokerage", defaultCurrency: "USD", balanceSheetRole: "asset" }, ownership: [] },
    { account: { id: "destination", name: "Destination", trackingMode: "holdings", accountType: "brokerage", defaultCurrency: "USD", balanceSheetRole: "asset" }, ownership: [] },
  ]);
  listInstruments.mockResolvedValue([
    { id: "class-a", shortName: "Family fund", name: "Share class A", symbol: "000001", isin: "ISIN-A", marketCode: "CN", quoteCurrency: "USD", quoteSource: "manual" },
    { id: "class-b", shortName: "Family fund", name: "Share class B", symbol: "000002", isin: "ISIN-B", marketCode: "CN", quoteCurrency: "USD", quoteSource: "manual" },
  ]);
  holdingsByAccounts.mockResolvedValue({ brokerage: [
    { id: "holding-a", instrumentId: "class-a", quantity: "10" },
    { id: "holding-b", instrumentId: "class-b", quantity: "10" },
  ], destination: [] });
  previewChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
  renderPage();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: /record change/i }));
  const form = screen.getByRole("form", { name: "Record change" });
  const kind = { sell: ChangeCommandKind.ChangeTrade, dividend: ChangeCommandKind.ChangeCashDividend, adjust: ChangeCommandKind.ChangePositionAdjustment, transfer: ChangeCommandKind.ChangePositionTransfer }[flow];
  await user.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${kind}`) }));
  if (flow === "sell") {
    await user.selectOptions(within(form).getByLabelText("Settlement account"), "brokerage");
    await user.selectOptions(within(form).getByLabelText("Side"), "sell");
  }
  const selector = within(form).getByLabelText(flow === "sell" ? "Instrument" : flow === "transfer" ? "From holding" : "Holding");
  const a = await within(selector).findByRole("option", { name: /^Family fund · 000001 · Brokerage/ });
  const b = within(selector).getByRole("option", { name: /^Family fund · 000002 · Brokerage/ });
  expect(a).not.toHaveTextContent("ISIN-A");
  expect(b).not.toHaveTextContent("ISIN-B");
  expect(a).toHaveTextContent("Brokerage · 10");
  expect(b).toHaveTextContent("Brokerage · 10");
  expect(a.textContent).not.toBe(b.textContent);
  await user.selectOptions(selector, b);
  if (flow === "dividend") await user.type(within(form).getByLabelText("Amount"), "25");
  else {
    await user.type(within(form).getByLabelText(/Quantity/), "1");
    if (flow === "sell") await user.type(within(form).getByLabelText(i18n.t("history.grossTotal")), "25");
    if (flow === "transfer") await user.selectOptions(within(form).getByLabelText("To account"), "destination");
  }
  await user.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(previewChange).toHaveBeenCalledWith(expect.objectContaining(flow === "transfer" ? { fromHoldingId: "holding-b", toAccountId: "destination" } : flow === "sell" ? { holdingId: "holding-b", instrumentId: "class-b", side: "sell" } : { holdingId: "holding-b" })));
});

it("keeps duplicate-alias starting-point cost inputs distinct and submits each holding ID", async () => {
  historyOrigin.mockResolvedValue(null);
  startingPointDraft.mockResolvedValue([
    { holdingId: "holding-a", instrumentId: "class-a", instrumentName: "Family fund · 000001 · Share class A · USD · ISIN-A", quantity: "10", currency: "USD", unitCost: "" },
    { holdingId: "holding-b", instrumentId: "class-b", instrumentName: "Family fund · 000002 · Share class B · USD · ISIN-B", quantity: "10", currency: "USD", unitCost: "" },
  ]);
  startHistoryOnDate.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  renderPage();
  const user = userEvent.setup();
  const a = await screen.findByLabelText(/Family fund · 000001 · Share class A.*ISIN-A.*10/);
  const b = screen.getByLabelText(/Family fund · 000002 · Share class B.*ISIN-B.*10/);
  expect(a).not.toBe(b);
  await user.type(a, "12.34");
  await user.type(b, "56.78");
  await user.click(screen.getByRole("button", { name: "Start history" }));
  await waitFor(() => expect(startHistoryOnDate).toHaveBeenCalledWith(expect.any(String), expect.any(String), { "holding-a": "12.34", "holding-b": "56.78" }));
});

it.each(["success", "uncertain"])("retains a deferred Repeat confirmation across Escape, Close and backdrop (%s)", async (outcome) => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  let resolve!: (value: object) => void;
  let reject!: (error: Error) => void;
  confirmRepeat.mockImplementationOnce(() => new Promise((done, fail) => { resolve = done; reject = fail; }));
  renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(confirmRepeat).toHaveBeenCalledWith("repeat-plan"));
  await userEvent.keyboard("{Escape}");
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  await userEvent.click(screen.getByRole("dialog").parentElement!.previousElementSibling!);
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  if (outcome === "uncertain") {
    await act(async () => { reject(new Error("IPC outcome unknown")); });
    await within(form).findByRole("alert");
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("dialog").parentElement!.previousElementSibling!);
    expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
    expect(within(form).getByRole("button", { name: "Back to draft" })).toBeDisabled();
    expect(within(form).getByLabelText("Amount")).toBeDisabled();
    expect(previewRepeat).toHaveBeenCalledTimes(1);
    await userEvent.click(within(form).getByRole("button", { name: "Retry same confirmation" }));
    expect(confirmRepeat.mock.calls.map(([id]) => id)).toEqual(["repeat-plan", "repeat-plan"]);
  } else { await act(async () => { resolve({}); }); }
  await waitFor(() => expect(screen.queryByRole("form", { name: "Record change" })).not.toBeInTheDocument());
  expect(confirmRepeat).toHaveBeenCalledTimes(outcome === "uncertain" ? 2 : 1);
  // Completion releases the guard. A later intentional Repeat starts a draft.
  await userEvent.click(within(screen.getByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  expect(await screen.findByRole("button", { name: "Preview" })).toBeEnabled();
  expect(previewRepeat).toHaveBeenCalledTimes(1);
  await userEvent.keyboard("{Escape}");
});


it("offers only cash assets for ordinary transfer endpoints", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([
    { account: { id: "bank", name: "Bank", trackingMode: "balance", balanceSheetRole: "asset", defaultCurrency: "USD" } },
    { account: { id: "broker", name: "Broker", trackingMode: "holdings", balanceSheetRole: "asset", defaultCurrency: "USD" } },
    { account: { id: "loan", name: "Loan", trackingMode: "balance", balanceSheetRole: "liability", defaultCurrency: "USD" } },
    { account: { id: "house", name: "House", trackingMode: "manual_value", balanceSheetRole: "asset", defaultCurrency: "USD" } },
  ]);
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: "Record change" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("radio", { name: "Cash transfer" }));
  for (const label of [i18n.t("history.fromAccount"), i18n.t("history.toAccount")]) {
    const select = within(form).getByLabelText(label);
    expect(within(select).getAllByRole("option").map(option => option.getAttribute("value"))).toEqual(["", "bank", "broker"]);
  }
});

it("suggests the filtered active account and instrument for a new record without guessing a date", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([{ account: { id: "broker", name: "Synthetic Broker", trackingMode: "holdings", balanceSheetRole: "asset", defaultCurrency: "EUR" } }]);
  listInstruments.mockResolvedValue([{ id: "stock", name: "Synthetic Stock", quoteCurrency: "USD", instrumentType: "stock" }]);
  renderPage({ accountId: "broker", instrumentId: "stock", from: "2026-08-01", to: "2026-08-31" });
  await userEvent.click(await screen.findByRole("button", { name: "Record change" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  expect(within(form).getByLabelText("Account")).toHaveValue("broker");
  expect(within(form).getByLabelText("Currency")).toHaveValue("EUR");
  await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeTrade}`) }));
  expect(within(form).getByLabelText("Settlement account")).toHaveValue("broker");
  expect(within(form).getByLabelText("Instrument")).toHaveValue("stock");
  expect(form.querySelector("#change-gross-currency")).toHaveValue("USD");
  expect(form.querySelector("#change-effective-date")).not.toHaveTextContent(/2026-08-01|08\/01\/2026/);
});

it("does not prefill archived history objects into a new record", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([{ account: { id: "old-account", name: "Old Broker", trackingMode: "holdings", balanceSheetRole: "asset", defaultCurrency: "EUR", archivedAt: "2026-08-31T00:00:00Z" } }]);
  listInstruments.mockResolvedValue([{ id: "old-stock", name: "Old Stock", quoteCurrency: "USD", instrumentType: "stock", archivedAt: "2026-08-31T00:00:00Z" }]);
  renderPage({ accountId: "old-account", instrumentId: "old-stock" });
  await userEvent.click(await screen.findByRole("button", { name: "Record change" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  expect(within(form).getByLabelText("Account")).toHaveValue("");
  await userEvent.click(within(form).getByRole("radio", { name: i18n.t(`history.kind.${ChangeCommandKind.ChangeTrade}`) }));
  expect(within(form).getByLabelText("Settlement account")).toHaveValue("");
  expect(within(form).getByLabelText("Instrument")).toHaveValue("");
});


it.each(["accounts", "origin", "activities"])("keeps pending and uncertain Repeat through a failed %s refresh", async read => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listActivities.mockResolvedValue([{ id: "cash", repeatEligible: true, kind: "cash_in", reason: "gift", effectiveLocalDate: "2026-01-01", effects: [{ accountId: "acc-1", money: { amount: "1", currency: "USD" } }] }]);
  let reject!: (error: Error) => void;
  confirmRepeat.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; }));
  const { queryClient } = renderPage();
  await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Repeat as new draft" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  const mock = read === "accounts" ? listAccounts : read === "origin" ? historyOrigin : listActivityPage;
  mock.mockRejectedValue(new Error("read unavailable"));
  const key = read === "accounts" ? queryKeys.accounts.all : read === "origin" ? queryKeys.history.origin : ["history", "activityPage"];
  await act(async () => { await queryClient.invalidateQueries({ queryKey: key }); });
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  expect(hasUnresolvedSheetConfirmation()).toBe(true);
  await act(async () => { reject(new Error("confirmation outcome unknown")); });
  await within(form).findByRole("button", { name: "Retry same confirmation" });
  await act(async () => { await queryClient.invalidateQueries({ queryKey: key }); });
  await userEvent.keyboard("{Escape}");
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  expect(previewRepeat).toHaveBeenCalledTimes(1);
  await userEvent.click(within(form).getByRole("button", { name: "Retry same confirmation" }));
  await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
  expect(confirmRepeat.mock.calls.map(([id]) => id)).toEqual(["repeat-plan", "repeat-plan"]);
});


it.each(["", "2.00000001"])("previews a new same-currency cash transfer using the read-only received amount (fee %s)", async fee => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([
    { account: { id: "bank", name: "Synthetic Bank", trackingMode: "balance", balanceSheetRole: "asset", defaultCurrency: "CNY" } },
    { account: { id: "savings", name: "Synthetic Savings", trackingMode: "balance", balanceSheetRole: "asset", defaultCurrency: "CNY" } },
    { account: { id: "usd", name: "Synthetic USD", trackingMode: "balance", balanceSheetRole: "asset", defaultCurrency: "USD" } },
  ]);
  previewChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
  recordChange.mockResolvedValue({});
  renderPage();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Record change" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await user.click(within(form).getByRole("radio", { name: "Cash transfer" }));
  await user.selectOptions(within(form).getByLabelText("From account"), "bank");
  await user.selectOptions(within(form).getByLabelText("To account"), "savings");
  const preview = within(form).getByRole("button", { name: "Preview" });
  expect(preview).toBeDisabled();
  await user.type(within(form).getByLabelText("Sent"), "100.00000001");
  expect(within(form).getByLabelText("Received")).toHaveValue("100.00000001");
  expect(within(form).getByLabelText("Received")).toHaveAttribute("readonly");
  if (fee) await user.type(within(form).getByLabelText("Fee (optional)"), fee);
  await waitFor(() => expect(preview).toBeEnabled());
  await user.click(preview);
  await waitFor(() => expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({
    fromAccountId: "bank", toAccountId: "savings", sent: "100.00000001", received: "100.00000001",
    sentCurrency: "CNY", receivedCurrency: "CNY", ...(fee ? { fee } : {}), feeCurrency: "CNY",
  })));
  await user.click(await within(form).findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(recordChange).toHaveBeenCalledWith(expect.objectContaining(previewChange.mock.calls[0][0])));
});


it("requires explicit received input for cross-currency cash transfer", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([
    { account: { id: "bank", name: "Bank", trackingMode: "balance", balanceSheetRole: "asset", defaultCurrency: "CNY" } },
    { account: { id: "usd", name: "USD", trackingMode: "balance", balanceSheetRole: "asset", defaultCurrency: "USD" } },
  ]);
  previewChange.mockResolvedValue({ activity: {}, effects: [], resulting: [] });
  renderPage();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Record change" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  await user.click(within(form).getByRole("radio", { name: "Cash transfer" }));
  await user.selectOptions(within(form).getByLabelText("From account"), "bank");
  await user.selectOptions(within(form).getByLabelText("To account"), "usd");
  await user.type(within(form).getByLabelText("Sent"), "100.00000001");
  const preview = within(form).getByRole("button", { name: "Preview" });
  expect(preview).toBeDisabled();
  expect(within(form).getByLabelText("Received")).not.toHaveAttribute("readonly");
  await user.type(within(form).getByLabelText("Received"), "14.00000001");
  await waitFor(() => expect(preview).toBeEnabled());
  await user.click(preview);
  await waitFor(() => expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({
    sent: "100.00000001", received: "14.00000001", sentCurrency: "CNY", receivedCurrency: "USD",
  })));
});


async function openOrdinaryRecoveryForm(family: "record" | "fix") {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  previewChange.mockResolvedValue({ resulting: [{ name: "Synthetic Checking", amount: "1125", currency: "USD" }] });
  previewFixChange.mockResolvedValue({ resulting: [{ name: "Synthetic Checking", amount: "1025", currency: "USD" }] });
  if (family === "fix") listActivities.mockResolvedValue([{ id: "source-1", kind: "cash_in", reason: "income", effectiveAt: "2026-01-02T12:00:00Z", effectiveLocalDate: "2026-01-02", effects: [{ role: "amount", direction: "added", target: "account_value", accountId: "acc-1", money: { amount: "100", currency: "USD" } }] }]);
  const view = renderPage();
  if (family === "record") await userEvent.click(await screen.findByRole("button", { name: "Record change" }));
  else {
    await userEvent.click(within(await screen.findByTestId("activity-list")).getByRole("button", { name: "Actions" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Fix" }));
  }
  const form = await screen.findByRole("form", { name: "Record change" });
  if (family === "record") await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
  await userEvent.clear(within(form).getByLabelText("Amount"));
  await userEvent.type(within(form).getByLabelText("Amount"), "125");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await within(form).findByRole("button", { name: "Confirm" });
  return { ...view, form };
}

it.each(["record", "fix"] as const)("synchronously guards ordinary %s, retains unknown intent and explicitly retries the same ID", async family => {
  const mutation = family === "record" ? recordChange : fixChange;
  let reject!: (error: Error) => void;
  mutation.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; }));
  mutation.mockResolvedValue({});
  const { form } = await openOrdinaryRecoveryForm(family);
  const confirm = within(form).getByRole("button", { name: "Confirm" });
  act(() => {
    confirm.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
    expect(hasUnresolvedSheetConfirmation()).toBe(true);
    confirm.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
    fireEvent.change(within(form).getByLabelText("Amount"), { target: { value: "999" } });
    fireEvent.keyDown(document, { key: "Escape" });
  });
  await waitFor(() => expect(mutation).toHaveBeenCalledTimes(1));
  const submitted = mutation.mock.calls[0];
  const command = submitted[family === "record" ? 0 : 1];
  expect(command).toMatchObject({ amount: "125", mutationId: expect.any(String) });
  if (family === "fix") expect(submitted[0]).toBe("source-1");
  expect(within(form).getByLabelText("Amount")).toHaveValue("125");
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("dialog").parentElement!.previousElementSibling!);
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  await act(async () => { reject(new Error("transport lost the outcome")); });
  const retry = await within(form).findByRole("button", { name: "Retry same submission" });
  expect(within(form).getByLabelText("Amount")).toBeDisabled();
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  expect(mutation).toHaveBeenCalledTimes(1);
  await userEvent.click(retry);
  await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
  expect(mutation.mock.calls[1]).toEqual(submitted);
  await waitFor(() => expect(screen.queryByRole("form", { name: "Record change" })).not.toBeInTheDocument());
});

it.each(["record", "fix"] as const)("releases ordinary %s only for an audited precommit refusal", async family => {
  const mutation = family === "record" ? recordChange : fixChange;
  mutation.mockRejectedValueOnce(new Error(JSON.stringify({ code: family === "record" ? "insufficient_balance" : "cannot_fix_change", message: "precommit refusal" })));
  const { form } = await openOrdinaryRecoveryForm(family);
  await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
  await within(form).findByRole("button", { name: "Preview" });
  expect(hasUnresolvedSheetConfirmation()).toBe(false);
  expect(within(form).getByLabelText("Amount")).toBeEnabled();
  expect(screen.getByRole("button", { name: "Close" })).toBeEnabled();
  await userEvent.keyboard("{Escape}");
});

it.each([
  { code: "conflict", field: "inputGeneration" },
  { code: "validation", field: "localDate" },
  { code: "decimal_overflow" },
  { code: "history_not_started" },
])("keeps postcommit-shaped Fix failure $code recoverable with the original source and ID", async error => {
  fixChange.mockRejectedValueOnce(new Error(JSON.stringify({ ...error, message: "snapshot rebuild failed after financial commit" }))).mockResolvedValue({});
  const { form } = await openOrdinaryRecoveryForm("fix");
  await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
  const retry = await within(form).findByRole("button", { name: "Retry same submission" });
  expect(hasUnresolvedSheetConfirmation()).toBe(true);
  expect(within(form).getByLabelText("Amount")).toBeDisabled();
  expect(within(form).getByText(i18n.t("history.submissionRetryHelp"))).toBeInTheDocument();
  const original = fixChange.mock.calls[0];
  await userEvent.click(retry);
  await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
  expect(fixChange.mock.calls[1]).toEqual(original);
  expect(previewFixChange).toHaveBeenCalledTimes(1);
});

it.each(["record", "fix"] as const)("preserves ordinary %s ownership through failed background reads", async family => {
  const mutation = family === "record" ? recordChange : fixChange;
  let reject!: (error: Error) => void;
  mutation.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; })).mockResolvedValue({});
  const { form, queryClient } = await openOrdinaryRecoveryForm(family);
  await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
  listAccounts.mockRejectedValue(new Error("read unavailable"));
  historyOrigin.mockRejectedValue(new Error("origin unavailable"));
  listActivityPage.mockRejectedValue(new Error("history unavailable"));
  await act(async () => { await queryClient.invalidateQueries({ queryKey: queryKeys.accounts.all }); await queryClient.invalidateQueries({ queryKey: queryKeys.history.origin }); await queryClient.invalidateQueries({ queryKey: ["history", "activityPage"] }); });
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  expect(hasUnresolvedSheetConfirmation()).toBe(true);
  await act(async () => { reject(new Error("outcome unknown")); });
  expect(await within(form).findByRole("button", { name: "Retry same submission" })).toBeEnabled();
  expect(mutation).toHaveBeenCalledTimes(1);
  await userEvent.click(within(form).getByRole("button", { name: "Retry same submission" }));
  await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
  expect(mutation.mock.calls[1]).toEqual(mutation.mock.calls[0]);
});

function recoveryHarness(client: ReturnType<typeof createTestQueryClient>, childKey: string, onRecorded: () => void, source?: string, ownerKey = "owner") {
  return <QueryClientProvider client={client}><Sheet key={ownerKey} open><SheetContent><SheetHeader><SheetTitle>Synthetic recovery</SheetTitle></SheetHeader>
    <RecordChangeForm key={childKey} initial={{ ...emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded, "USD"), accountId: "acc-1", amount: "125" }} fixActivityId={source} onRecorded={onRecorded} />
  </SheetContent></Sheet></QueryClientProvider>;
}

it.each(["record", "fix"] as const)("restores pending and unknown ordinary %s after a child remount", async family => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  previewChange.mockResolvedValue({ resulting: [{ name: "Saved review", amount: "1125", currency: "USD" }] });
  previewFixChange.mockResolvedValue({ resulting: [{ name: "Saved review", amount: "1025", currency: "USD" }] });
  const mutation = family === "record" ? recordChange : fixChange;
  let reject!: (error: Error) => void;
  mutation.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; })).mockResolvedValue({});
  const client = createTestQueryClient(); const onRecorded = vi.fn(); const source = family === "fix" ? "source-1" : undefined;
  const view = render(recoveryHarness(client, "first", onRecorded, source));
  await userEvent.click(await screen.findByRole("button", { name: "Preview" }));
  await userEvent.click(await screen.findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(mutation).toHaveBeenCalledTimes(1));
  const original = mutation.mock.calls[0];
  view.rerender(recoveryHarness(client, "pending-remount", onRecorded, source));
  expect(screen.getByRole("button", { name: i18n.t("common.pending") })).toBeDisabled();
  expect(screen.getByLabelText("Amount")).toHaveValue("125");
  await act(async () => { reject(new Error("lost IPC result")); });
  await screen.findByRole("button", { name: "Retry same submission" });
  view.rerender(recoveryHarness(client, "unknown-remount", onRecorded, source));
  expect(screen.getByText(/Saved review:/)).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Retry same submission" }));
  await waitFor(() => expect(onRecorded).toHaveBeenCalledTimes(1));
  expect(mutation.mock.calls[1]).toEqual(original);
  expect(hasUnresolvedSheetConfirmation()).toBe(false);
});

it("does not let a detached ordinary submission close a newer Sheet owner", async () => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  previewChange.mockResolvedValue({ resulting: [] });
  let finishOld!: (value: object) => void;
  recordChange.mockImplementationOnce(() => new Promise(done => { finishOld = done; })).mockImplementationOnce(() => new Promise(() => {}));
  const client = createTestQueryClient(); const oldDone = vi.fn(); const newDone = vi.fn();
  const view = render(recoveryHarness(client, "first", oldDone));
  await userEvent.click(await screen.findByRole("button", { name: "Preview" }));
  await userEvent.click(await screen.findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(recordChange).toHaveBeenCalledTimes(1));
  view.rerender(recoveryHarness(client, "second", newDone, undefined, "new-owner"));
  await userEvent.click(await screen.findByRole("button", { name: "Preview" }));
  await userEvent.click(await screen.findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(recordChange).toHaveBeenCalledTimes(2));
  await act(async () => { finishOld({}); });
  expect(oldDone).not.toHaveBeenCalled(); expect(newDone).not.toHaveBeenCalled();
  expect(hasUnresolvedSheetConfirmation()).toBe(true);
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
});

it.each(["household", "session", "fix-source"])("blocks recovery dispatch and stale success after a changed %s", async change => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  previewFixChange.mockResolvedValue({ resulting: [] });
  let finish!: (value: object) => void;
  fixChange.mockImplementationOnce(() => new Promise(done => { finish = done; }));
  const client = createTestQueryClient(); const onRecorded = vi.fn();
  const view = render(recoveryHarness(client, "first", onRecorded, "source-1"));
  await userEvent.click(await screen.findByRole("button", { name: "Preview" }));
  await userEvent.click(await screen.findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(fixChange).toHaveBeenCalledTimes(1));
  if (change === "household") await act(async () => { client.setQueryData(queryKeys.household.bootstrap, { household: { id: "other", baseCurrency: "USD" } }); });
  if (change === "session") {
    const otherClient = createTestQueryClient();
    otherClient.setQueryData(queryKeys.household.bootstrap, client.getQueryData(queryKeys.household.bootstrap));
    view.rerender(recoveryHarness(otherClient, "new-session", onRecorded, "source-1"));
  }
  if (change === "fix-source") view.rerender(recoveryHarness(client, "new-source", onRecorded, "source-2"));
  await act(async () => { finish({}); });
  expect(onRecorded).not.toHaveBeenCalled();
  expect(hasUnresolvedSheetConfirmation()).toBe(true);
  const retry = await screen.findByRole("button", { name: "Retry same submission" });
  expect(retry).toBeDisabled();
  fireEvent.click(retry);
  expect(fixChange).toHaveBeenCalledTimes(1);
});

it("assigns distinct hook intents to identical Fix payloads for different sources and prevents automatic retry", async () => {
  fixChange.mockRejectedValueOnce(new Error("unknown first outcome")).mockResolvedValue({});
  const client = createTestQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: 2 } });
  const { result } = renderHook(() => useFixChange(), { wrapper: ({ children }) => <QueryClientProvider client={client}>{children}</QueryClientProvider> });
  const replacement = { ...emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded, "USD"), accountId: "acc-1", amount: "125" };
  await act(async () => { await expect(result.current.mutateAsync({ activityId: "source-1", replacement })).rejects.toThrow(); });
  await act(async () => { await result.current.mutateAsync({ activityId: "source-2", replacement }); });
  expect(fixChange).toHaveBeenCalledTimes(2);
  expect(fixChange.mock.calls[0][1].mutationId).not.toBe(fixChange.mock.calls[1][1].mutationId);
});

it.each(["reconciliationLink", "financialRevision", "expectedRevision"])("releases Record after receipt-first reconciliation refusal %s",async field=>{
 recordChange.mockRejectedValueOnce(new Error(JSON.stringify({code:"conflict",field,message:"renewed review required"})));
 const {form}=await openOrdinaryRecoveryForm("record");await userEvent.click(within(form).getByRole("button",{name:"Confirm"}));await within(form).findByRole("button",{name:"Preview"});expect(hasUnresolvedSheetConfirmation()).toBe(false);expect(screen.getByRole("button",{name:"Close"})).toBeEnabled();await userEvent.keyboard("{Escape}");
});

it.each([false, true])("retains the original sold-out holding in Fix (archived=%s)", async archived => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  const archivedAt = archived ? "2026-03-01T00:00:00Z" : null;
  listAccounts.mockImplementation(async (filter?: { includeArchived?: boolean }) => !archived || filter?.includeArchived ? [{ account: { id: "brokerage-1", name: "Brokerage", trackingMode: "holdings", defaultCurrency: "USD", archivedAt }, ownership: [], latestValue: null }] : []);
  listInstruments.mockResolvedValue([{ id: "instrument-1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual", archivedAt }]);
  holdingsByAccounts.mockResolvedValue({ "brokerage-1": [{ id: "holding-1", accountId: "brokerage-1", instrumentId: "instrument-1", quantity: "0", archivedAt }] });
  previewFixChange.mockResolvedValue({ resulting: [] });
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><RecordChangeForm initial={{ kind: ChangeCommandKind.ChangeTrade, side: "sell", settlementAccountId: "brokerage-1", holdingId: "holding-1", instrumentId: "instrument-1", quantity: "10", gross: "100", grossCurrency: "USD", feeCurrency: "USD", added: true }} fixActivityId="sold" onRecorded={vi.fn()} /></QueryClientProvider>);
  const form = await screen.findByRole("form", { name: "Record change" });
  await waitFor(() => expect(within(form).getByLabelText("Instrument")).toHaveValue("holding-1"));
  expect(within(form).getByLabelText("Instrument")).toHaveTextContent("Brokerage");
  await userEvent.type(within(form).getByLabelText("Note"), "Corrected note");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(previewFixChange).toHaveBeenCalledWith("sold", expect.objectContaining({ holdingId: "holding-1", instrumentId: "instrument-1", quantity: "10", gross: "100" })));
  expect(fixChange).not.toHaveBeenCalled();
});

it.each(["1200", ""])("uses the historical absolute target for note-only Fix (%s)", async target => {
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  listAccounts.mockResolvedValue([{ account: { id: "acc-1", name: "Checking", trackingMode: "balance", defaultCurrency: "USD" }, ownership: [], latestValue: { amount: { amount: "9999", currency: "USD" } } }]);
  previewFixChange.mockResolvedValue({ resulting: [] });
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><RecordChangeForm initial={{ kind: ChangeCommandKind.ChangeValueUpdate, accountId: "acc-1", newValue: target, newValueCurrency: "USD", reason: "other", note: "Original", added: true }} fixActivityId="observation" onRecorded={vi.fn()} /></QueryClientProvider>);
  const form = await screen.findByRole("form", { name: "Record change" });
  await userEvent.type(within(form).getByLabelText("Note"), " updated");
  expect(within(form).getByLabelText("New value")).toHaveValue(target);
  const preview = within(form).getByRole("button", { name: "Preview" });
  if (!target) { expect(preview).toBeDisabled(); expect(previewFixChange).not.toHaveBeenCalled(); }
  else { await userEvent.click(preview); expect(previewFixChange).toHaveBeenCalledWith("observation", expect.objectContaining({ newValue: "1200", note: "Original updated" })); }
  expect(fixChange).not.toHaveBeenCalled();
});

// Set VITE_NESTWORTH_TEST_DTO_JSON after TestValueCorrectionDTOIntegration to
// exercise the same flow with its real isolated SQLite DTO.
it("previews and confirms a historical DTO through the Fix form", async () => {
  const dto = import.meta.env.VITE_NESTWORTH_TEST_DTO_JSON
    ? JSON.parse(import.meta.env.VITE_NESTWORTH_TEST_DTO_JSON) as ActivityDTO
    : { id: "observation", kind: "value_update", reason: "other", effects: [], resulting: [{ target: "account_value", accountId: "acc-1", amount: "1200", currency: "USD" }] } as unknown as ActivityDTO;
  const accountId = dto.resulting![0].accountId!;
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  listAccounts.mockResolvedValue([{ account: { id: accountId, name: "Checking", trackingMode: "balance", defaultCurrency: "USD" }, ownership: [], latestValue: { amount: { amount: "9999", currency: "USD" } } }]);
  previewFixChange.mockResolvedValue({ resulting: dto.resulting });
  fixChange.mockResolvedValue({ activity: { id: "fixed" }, resulting: dto.resulting });
  const client = createTestQueryClient();
  const recorded = vi.fn();
  render(<QueryClientProvider client={client}><Sheet open><SheetContent><SheetHeader><SheetTitle>Fix synthetic observation</SheetTitle></SheetHeader><RecordChangeForm initial={activityToInitialCommand(dto)} fixActivityId={dto.id} onRecorded={recorded} /></SheetContent></Sheet></QueryClientProvider>);
  const form = await screen.findByRole("form", { name: "Record change" });
  expect(within(form).getByLabelText("New value")).toHaveValue("1200");
  await userEvent.type(within(form).getByLabelText("Note"), "Updated from DTO");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(previewFixChange).toHaveBeenCalledWith(dto.id, expect.objectContaining({ accountId, newValue: "1200", note: "Updated from DTO" })));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(fixChange).toHaveBeenCalledWith(dto.id, expect.objectContaining({ accountId, newValue: "1200", note: "Updated from DTO", mutationId: expect.any(String) })));
  await waitFor(() => expect(recorded).toHaveBeenCalledTimes(1));
});
