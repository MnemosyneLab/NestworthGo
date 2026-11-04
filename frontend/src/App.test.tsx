import { upcomingData, upcomingItem } from "@/test/upcomingActions";
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({ Service: { InstrumentHoldings: () => Promise.resolve([]), InstrumentHoldingGroups: () => instrumentGroups(), HistoricalOverview: (...args: unknown[]) => historicalRead(...args), HistoricalOverviewSeries: (...args: unknown[]) => seriesRead(...args) } }));
/* default historical fixture */
const defaultHistorical = { capturedAt: "2026-10-03T12:00:00Z", left: { date: "2026-09-30", currency: "USD", complete: true, assets: "0", liabilities: "0", netWorth: "0", byClass: [], byCurrency: [] }, right: null, rows: [] };
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { flushSync } from "react-dom";
import { useAnalysisStore } from "./stores/analysis";
import App from "./App";
import { AppProviders, queryClient } from "./app/providers";
import i18n from "./i18n";
import { queryKeys } from "./queries/keys";
import { reviewV2Key } from "./queries/investmentReviewV2";
import { useUiStore } from "./stores/ui";
import { registerSheetConfirmation, resolveSheetConfirmation } from "./components/ui/sheet-confirmation";
import { useObjectNavigation } from "./app/NavigationContext";

const onboardingFixture = vi.hoisted(() => ({ enabled: false, completed: false }));

const { startup, settingsLoad, settingsSave, listAccounts, listInstruments, accountValuations, portfolio, holdingsByAccounts, upcomingRead, scanHealth, historicalRead, historyOrigin, seriesRead, ordinaryRecord, useRealInstrumentDetail, reviewAlerts, instrumentGroups, reconciliationList, reconciliationLast, reconciliationGet } = vi.hoisted(() => ({
  seriesRead: vi.fn(),
  ordinaryRecord: vi.fn(),
  useRealInstrumentDetail: { enabled: false },
  reviewAlerts: vi.fn(),
  instrumentGroups: vi.fn(),
  reconciliationList: vi.fn(),
  reconciliationLast: vi.fn(),
  reconciliationGet: vi.fn(),
  historyOrigin: vi.fn(),
  historicalRead: vi.fn(),
  upcomingRead: vi.fn(),
  scanHealth: vi.fn(),
  startup: vi.fn().mockResolvedValue({ available: true }),
  settingsLoad: vi.fn(),
  settingsSave: vi.fn(),
  listAccounts: vi.fn(),
  listInstruments: vi.fn(),
  accountValuations: vi.fn(),
  portfolio: vi.fn(),
  holdingsByAccounts: vi.fn(),
}));

const emptyPortfolio = {
  currency: "USD",
  complete: true,
  accounts: [] as unknown[],
  missingInputs: [] as unknown[],
  byInstrumentType: [] as unknown[],
  byCurrency: [] as unknown[],
  byCountry: [] as unknown[],
};

const checkingAccount = {
  account: {
    id: "acc-1",
    name: "Checking",
    accountType: "bank_account",
    balanceSheetRole: "asset",
    trackingMode: "balance",
    defaultCurrency: "USD",
    sortOrder: 0,
    includeInNetWorth: true,
    includeInPortfolio: true,
    includeInLiquidAssets: false,
    createdAt: "",
    updatedAt: "",
  },
  ownership: [{ memberId: "alice", shareBps: 10000 }],
  latestValue: {
    id: "v1",
    accountId: "acc-1",
    valueKind: "balance",
    amount: { amount: "1000", currency: "USD" },
    effectiveAt: "",
    createdAt: "",
  },
};

const checkingValuation = {
  account: checkingAccount.account,
  ownership: checkingAccount.ownership,
  complete: true,
  components: [],
  missingInputs: [],
  baseValue: { amount: "1000", currency: "USD" },
};

const defaultSettings = {
  schema_version: 1,
  appearance: "system",
  accent: "indigo",
  language: "en",
  timezone: "system",
  weekStart: "monday",
  dateFormat: "iso",
  timeFormat: "24h",
  currency: "USD",
  decimalSeparator: ".",
  groupingSeparator: ",",
  decimalPlaces: 2,
  windowWidth: 1100,
  windowHeight: 720,
  fxProvider: "frankfurter",
};

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: () => () => undefined,
  },
}));

// The generated Wails binding calls Call.ByID under the hood, which tries
// to reach the Wails runtime bridge that does not exist in jsdom.
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app", () => ({
  Service: {
    AppInfo: () => Promise.resolve({ name: "Nestworth", appId: "com.nestworth.app", version: "v0.2.0", build: "1" }),
    Startup: () => startup(),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => settingsLoad(),
    Save: (...args: unknown[]) => settingsSave(...args),
    Reset: vi.fn(),
    SupportedCurrencies: () => Promise.resolve(["USD"]),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

// This smoke test only exercises the shell + placeholder pages, so
// Bootstrap resolves with an existing Household (skipping Onboarding);
// OnboardingPage has its own dedicated tests.
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: {
    CompleteOnboarding: async () => { onboardingFixture.completed = true; return {}; },
    Bootstrap: () =>
      Promise.resolve({
        household: onboardingFixture.enabled && !onboardingFixture.completed ? null : { id: "h1", name: "Test", baseCurrency: "USD", createdAt: "", updatedAt: "" },
        members: [],
        institutions: [],
        groups: [],
      }),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: {
    Overview: () =>
      Promise.resolve({
        currency: "USD",
        accountCount: 1,
        complete: true,
        missingInputs: [],
        assets: "0",
        liabilities: "0",
        netWorth: "0",
        assetsByType: [],
        liabilitiesByType: [],
        byMember: [],
        byInstitution: [],
        byGroup: [],
        byAccountType: [],
      }),
    Portfolio: () => portfolio(),
    PortfolioTrend: () => Promise.resolve({ range: "30d", currency: "USD", points: [] }),
    NetWorthTrend: () => Promise.resolve({ range: "30d", currency: "USD", points: [] }),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: historyOrigin,
    HistoryMutationAllowed: () => Promise.resolve({}),
    PreviewChange: () => Promise.resolve({ resulting: [{ name: "Synthetic checking", amount: "1125", currency: "USD" }] }),
    RecordChange: (...args: unknown[]) => ordinaryRecord(...args),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: (...args: unknown[]) => accountValuations(...args),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: (...args: unknown[]) => listInstruments(...args) },
}));
vi.mock("@/features/investments/InstrumentDetailPage", async () => {
  const actual = await vi.importActual<typeof import("@/features/investments/InstrumentDetailPage")>("@/features/investments/InstrumentDetailPage");
  return { InstrumentDetailPage: (props: Parameters<typeof actual.InstrumentDetailPage>[0]) => {
    const navigation = useObjectNavigation();
    if (useRealInstrumentDetail.enabled) return <actual.InstrumentDetailPage {...props} />;
    const { context, onContextChange } = props;
    return <section data-testid="instrument-detail-page"><pre>{JSON.stringify(context)}</pre>
      <button onClick={() => onContextChange({ ...context, section: "data" })}>Show data records</button>
      <button onClick={() => onContextChange({ ...context, from: "2026-07-01", to: "2026-07-31" })}>Set synthetic quote period</button>
      <button id="instrument-detail-maintain" onClick={() => navigation?.open({ page: "market-data", focus: { instrumentId: context.instrumentId, action: "instrument_editor", rangeStart: "2026-07-01", rangeEnd: "2026-07-31" } })}>Maintain data</button>
      <button onClick={() => navigation?.open({ page: "accounts", create: true })}>Create synthetic account</button>
      <button onClick={() => navigation?.open({ page: "history", filters: { instrumentId: context.instrumentId, from: context.from, to: context.to } })}>Related transactions</button>
    </section>;
  } };
});

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    InstrumentQuoteSeries: () => Promise.resolve({ range: "30d", points: [], observations: [] }),
    CurrentInstrumentQuote: () => Promise.resolve(null),
    InstrumentQuotePerformance: () => Promise.resolve({ item: null, points: [], unavailableReason: "insufficient_daily_history" }),
    InstrumentQuoteReturns: () => Promise.resolve({ items: [] }),
    InstrumentQuotePerformanceSeries: () => Promise.resolve({ item: null, points: [] }),
    FXQuoteSeries: () => Promise.resolve({ range: "30d", points: [], observations: [] }),
    ListFXPreferences: () => Promise.resolve([]),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    ScanMarketDataHealth: () => scanHealth(),
    GetCurrentSyncJob: () => Promise.resolve({ jobId: "" }),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data", () => ({
  Service: {
    LastBackupStatus: () => Promise.resolve({ available: false }),
    CreateBackup: () => Promise.resolve({ cancelled: true }),
    ExportJSON: () => Promise.resolve({ cancelled: true }),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: {
    InspectBackup: () => Promise.resolve({ cancelled: true }),
    ConfirmRestore: () => Promise.resolve({ restartRequired: false }),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation", () => ({
  Service: {
    List: () => reconciliationList(),
    LastSuccess: (id: string) => reconciliationLast(id),
    Get: (id: string) => reconciliationGet(id),
    Links: () => Promise.resolve([]),
    Changes: () => Promise.resolve([]),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
    UpcomingActions: () => upcomingRead(),
    ListReviewAlertsV2: (...args: unknown[]) => reviewAlerts(...args),
    ListReviewRulesV2: () => Promise.resolve([]),
    ReconcileInvestmentReviewsV2: () => Promise.resolve({}),
    Overview: () => Promise.resolve({
      asOf: "",
      localDate: "2026-09-20",
      timezone: "UTC",
      baseCurrency: "USD",
      assumptions: [],
      buckets: [
        { horizonOn: "2026-09-20", status: "unavailable", fullAvailable: null, knownAvailableSubtotal: null, appliedReserveSubtotal: null, fullUnreserved: null, knownUnreservedSubtotal: null, unknownSourceCount: 0, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
        { horizonOn: "2026-09-27", status: "unavailable", fullAvailable: null, knownAvailableSubtotal: null, appliedReserveSubtotal: null, fullUnreserved: null, knownUnreservedSubtotal: null, unknownSourceCount: 0, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
        { horizonOn: "2026-10-20", status: "unavailable", fullAvailable: null, knownAvailableSubtotal: null, appliedReserveSubtotal: null, fullUnreserved: null, knownUnreservedSubtotal: null, unknownSourceCount: 0, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
      ],
      sources: [],
      unresolvedReservations: [],
    }),
    ListProducts: () => Promise.resolve([]),
    Product: vi.fn(),
    ListOperations: vi.fn(),
  },
}));

beforeEach(() => {
  onboardingFixture.enabled = false; onboardingFixture.completed = false;
  queryClient.clear();
  historicalRead.mockReset().mockResolvedValue(defaultHistorical);
  seriesRead.mockReset();
  historyOrigin.mockReset().mockResolvedValue({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" });
  scanHealth.mockReset().mockResolvedValue({ healthy: true, issueCount: 0, executableCount: 0, prerequisiteCount: 0, snapshotDays: 0, issues: [] });
  upcomingRead.mockReset().mockResolvedValue(upcomingData([upcomingItem()]));
  startup.mockReset();
  startup.mockResolvedValue({ available: true });
  settingsLoad.mockReset();
  settingsLoad.mockResolvedValue({ ...defaultSettings });
  settingsSave.mockReset();
  settingsSave.mockResolvedValue(undefined);
  listAccounts.mockReset();
  listInstruments.mockReset();
  listInstruments.mockResolvedValue([]);
  listAccounts.mockResolvedValue([]);
  accountValuations.mockReset();
  accountValuations.mockResolvedValue([]);
  ordinaryRecord.mockReset();
  portfolio.mockReset();
  portfolio.mockResolvedValue({ ...emptyPortfolio });
  holdingsByAccounts.mockReset();
  holdingsByAccounts.mockResolvedValue({});
  useRealInstrumentDetail.enabled = false;
  reviewAlerts.mockReset().mockResolvedValue({ items: [], nextCursor: "", summary: { totalGroups: 0, matchedGroups: 0, pendingGroups: 0, acknowledgedGroups: 0, ignoredGroups: 0, unknownGroups: 0, pausedGroups: 0, actionableRules: 0 }, reportRevision: "synthetic-review-v2", projectionStatus: "complete", publishedOn: "2026-10-15" });
  instrumentGroups.mockReset().mockResolvedValue([]);
  reconciliationList.mockReset().mockResolvedValue([]);
  reconciliationLast.mockReset().mockResolvedValue(null);
  reconciliationGet.mockReset().mockResolvedValue(null);
  useUiStore.setState({ appearance: "system", accent: "indigo" });
  window.matchMedia =
    window.matchMedia ||
    (vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })) as unknown as typeof window.matchMedia);
});

describe("App shell smoke test", () => {
  it("does not leave a settings load failure on an infinite loading screen", async () => {
    settingsLoad.mockRejectedValueOnce(new Error("settings failed"));
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );

    expect(await screen.findByRole("alert", {}, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.queryByText(i18n.t("ui.state.loadingWorkspace"))).not.toBeInTheDocument();
  });

  it("hydrates saved appearance and language before rendering the workspace", async () => {
    settingsLoad.mockResolvedValue({ ...defaultSettings, appearance: "dark", language: "zh-CN" });
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );

    expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
    // The trend query can finish after the net-worth summary has rendered.
    await waitFor(() => expect(screen.queryByText(i18n.t("ui.state.loadingPage"))).not.toBeInTheDocument());
    await waitFor(() => expect(document.documentElement.classList.contains("dark")).toBe(true));
    expect(i18n.language).toBe("zh-CN");
  });

  it("persists a header language change with the complete settings value", async () => {
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    await screen.findByTestId("overview-net-worth");

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Language" }), "zh-CN");
    await waitFor(() =>
      expect(settingsSave).toHaveBeenCalledWith({
        ...defaultSettings,
        language: "zh-CN",
      }),
    );
  });

  it("rolls back a failed header save", async () => {
    settingsSave.mockRejectedValue(new Error("save failed"));
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    await screen.findByTestId("overview-net-worth");

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Language" }), "zh-CN");
    await waitFor(() => expect(i18n.language).toBe("en"));
    expect(screen.getByRole("combobox", { name: "Language" })).toHaveValue("en");
  });

  it.each(["en"] as const)("renders the app shell in %s without runtime errors", async (language) => {
    await i18n.changeLanguage(language);
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    // Overview is the default landing page and renders its net worth figure
    // without throwing.
    expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
    // Every nav item's label is a real, non-fallback translation (a
    // missing i18next key falls back to the raw key text, which would
    // never match a real nav item's rendered label).
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    expect(within(nav).getByRole("button", { name: i18n.t("nav.overview") })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: i18n.t("nav.directory") })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: i18n.t("nav.availableFunds") })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: new RegExp(i18n.t("nav.tasks")) })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: i18n.t("nav.portfolio") })).toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: i18n.t("nav.holdings") })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: i18n.t("nav.dataHealth") })).not.toBeInTheDocument();

    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.marketData") }));
    await userEvent.click(await screen.findByRole("tab", { name: i18n.t("dataMaintenance.issues") }));
    expect(await screen.findByTestId("data-health-page")).toBeInTheDocument();

    // Settings is a real surface and remains reachable from the grouped nav.
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.settings") }));
    expect(await screen.findByRole("form", { name: i18n.t("settings.formLabel") })).toBeInTheDocument();
    expect(screen.getByLabelText(i18n.t("about.title"))).toBeInTheDocument();
  });

  it("renders the blocked-startup page when the database is unavailable", async () => {
    await i18n.changeLanguage("en");
    startup.mockResolvedValue({ available: false, code: "database_unavailable", field: "database" });
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(i18n.t("startup.unavailableTitle"));
    expect(screen.queryByTestId("overview-net-worth")).not.toBeInTheDocument();
  });

  it("opens holdings first and keeps allocation as a secondary investment view", async () => {
    await i18n.changeLanguage("en");
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.portfolio") }));
    expect(await screen.findByRole("tab", { name: "My holdings" })).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByText(i18n.t("portfolio.noHoldingsAccount"))).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(i18n.t("portfolio.pageTitle"));
    await userEvent.click(screen.getByRole("tab", { name: "Allocation & trend" }));
    expect(await screen.findByTestId("portfolio-page")).toBeInTheDocument();
    expect(screen.getByText(i18n.t("portfolio.noHoldingsAccount"))).not.toBeVisible();
  });

  it("returns from shared instrument details to the same saved-instrument view and accepts another object", async () => {
    await i18n.changeLanguage("en");
    listInstruments.mockResolvedValue([
      { id: "fund-one", householdId: "synthetic", name: "Fund One", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 0 },
      { id: "fund-two", householdId: "synthetic", name: "Fund Two", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 1, archivedAt: "2026-09-30" },
    ]);
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: "Investment assets" }));
    await userEvent.click(await screen.findByRole("tab", { name: "All instruments" }));
    await userEvent.click(await screen.findByRole("button", { name: "Open Fund One details" }));
    expect(await screen.findByTestId("instrument-detail-page")).toHaveTextContent('"instrumentId":"fund-one"');
    await userEvent.click(screen.getByRole("button", { name: "Show data records" }));
    expect(screen.getByTestId("instrument-detail-page")).toHaveTextContent('"section":"data"');
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Investment assets(?: ·|$)/ }));
    expect(screen.getByRole("tab", { name: "All instruments" })).toHaveAttribute("aria-selected", "true");
    await userEvent.click(await screen.findByRole("button", { name: "Open Fund Two details" }));
    expect(await screen.findByTestId("instrument-detail-page")).toHaveTextContent('"instrumentId":"fund-two"');
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Investment assets(?: ·|$)/ }));
    expect(screen.getByRole("tab", { name: "All instruments" })).toHaveAttribute("aria-selected", "true");
  });

  it("keeps the data-maintenance search when returning from an instrument record", async () => {
    await i18n.changeLanguage("en");
    listInstruments.mockResolvedValue([
      { id: "fund-one", householdId: "synthetic", name: "Fund One", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 0 },
      { id: "fund-two", householdId: "synthetic", name: "Fund Two", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 1 },
    ]);
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: "Data maintenance" }));
    const search = await screen.findByLabelText(i18n.t("marketData.searchInstruments"), {}, { timeout: 5000 });
    await userEvent.type(search, "One");
    await userEvent.click(await screen.findByRole("button", { name: "Open Fund One details" }));
    expect(await screen.findByTestId("instrument-detail-page")).toHaveTextContent('"instrumentId":"fund-one"');
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Data maintenance(?: ·|$)/ }));
    expect(screen.getByLabelText(i18n.t("marketData.searchInstruments"))).toHaveValue("One");
    expect(screen.getByRole("button", { name: "Open Fund One details" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open Fund Two details" })).not.toBeInTheDocument();
  });

  it("keeps each maintenance tab mounted through tab changes and detail return", async () => {
    await i18n.changeLanguage("en");
    listInstruments.mockResolvedValue([
      { id: "fund-one", householdId: "synthetic", name: "Fund One", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 0 },
      { id: "fund-two", householdId: "synthetic", name: "Fund Two", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 1 },
    ]);
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: "Data maintenance" }));
    const search = await screen.findByLabelText(i18n.t("marketData.searchInstruments"), {}, { timeout: 5000 });
    await userEvent.type(search, "One");
    await userEvent.click(screen.getByRole("tab", { name: i18n.t("marketData.fx") }));
    await userEvent.click(screen.getByRole("tab", { name: "Issues & repair" }));
    expect(await screen.findByTestId("data-health-page")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "Quotes & FX" }));
    expect(screen.getByRole("tab", { name: i18n.t("marketData.fx") })).toHaveAttribute("aria-selected", "true");
    await userEvent.click(screen.getByRole("tab", { name: "Instruments" }));
    expect(screen.getByLabelText(i18n.t("marketData.searchInstruments"))).toHaveValue("One");
    await userEvent.click(await screen.findByRole("button", { name: "Open Fund One details" }));
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Data maintenance(?: ·|$)/ }));
    expect(screen.getByLabelText(i18n.t("marketData.searchInstruments"))).toHaveValue("One");
    expect(screen.queryByRole("button", { name: "Open Fund Two details" })).not.toBeInTheDocument();
  });

  it("returns from scoped History to the edited instrument period and maintenance search", async () => {
    await i18n.changeLanguage("en");
    listInstruments.mockResolvedValue([{ id: "fund-one", householdId: "synthetic", name: "Fund One", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 0 }]);
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: "Data maintenance" }));
    await userEvent.type(await screen.findByLabelText(i18n.t("marketData.searchInstruments"), {}, { timeout: 5000 }), "One");
    await userEvent.click(await screen.findByRole("button", { name: "Open Fund One details" }));
    await userEvent.click(screen.getByRole("button", { name: "Set synthetic quote period" }));
    await userEvent.click(screen.getByRole("button", { name: "Related transactions" }));
    expect(await screen.findByRole("region", { name: "Synthetic transaction history" })).toHaveTextContent('"from":"2026-07-01","to":"2026-07-31"');
    await userEvent.click(screen.getByRole("button", { name: new RegExp(`^Go back to ${i18n.t("instrumentDetail.title")}`) }));
    expect(screen.getByTestId("instrument-detail-page")).toHaveTextContent('"from":"2026-07-01"');
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Data maintenance(?: ·|$)/ }));
    expect(screen.getByLabelText(i18n.t("marketData.searchInstruments"))).toHaveValue("One");
  });

  it("restores each focused maintenance request and the prior search on Back", async () => {
    await i18n.changeLanguage("en");
    listInstruments.mockResolvedValue([
      { id: "fund-one", householdId: "synthetic", name: "Fund One", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 0 },
      { id: "fund-two", householdId: "synthetic", name: "Fund Two", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 1 },
    ]);
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: "Data maintenance" }));
    const search = await screen.findByLabelText(i18n.t("marketData.searchInstruments"), {}, { timeout: 5000 });
    await userEvent.type(search, "One");
    await userEvent.click(await screen.findByRole("button", { name: "Open Fund One details" }));
    await screen.findByTestId("instrument-detail-page");
    await userEvent.click(screen.getByRole("button", { name: "Show data records" }));
    await userEvent.click(screen.getByRole("button", { name: "Maintain data" }));
    await userEvent.click(await screen.findByRole("button", { name: "Close" }));

    // A nested Details → Maintain request must not replace the first
    // request's dismissed state when Back restores its older request ID.
    await userEvent.click(screen.getByRole("button", { name: "Open Fund One details" }));
    await userEvent.click(screen.getByRole("button", { name: "Maintain data" }));
    await userEvent.click(await screen.findByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Instrument details(?: ·|$)/ }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(document.getElementById("instrument-detail-maintain")));
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Data maintenance(?: ·|$)/ }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Open Fund One details" }));
    await userEvent.click(screen.getByRole("button", { name: "Show data records" }));
    await userEvent.click(screen.getByRole("button", { name: "Maintain data" }));
    expect(await screen.findByRole("textbox", { name: "Name" })).toHaveValue("Fund One");
    await userEvent.click(await screen.findByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Instrument details(?: ·|$)/ }));
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Data maintenance(?: ·|$)/ }));

    expect(screen.getByLabelText(i18n.t("marketData.searchInstruments"))).toHaveValue("");
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Instrument details(?: ·|$)/ }));
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Data maintenance(?: ·|$)/ }));
    expect(screen.getByLabelText(i18n.t("marketData.searchInstruments"))).toHaveValue("One");
    await waitFor(() => expect(document.activeElement).toHaveAccessibleName("Open Fund One details"));

    await userEvent.click(within(nav).getByRole("button", { name: "Data maintenance" }));
    await userEvent.clear(screen.getByLabelText(i18n.t("marketData.searchInstruments")));
    await userEvent.type(screen.getByLabelText(i18n.t("marketData.searchInstruments")), "Two");
    await userEvent.click(await screen.findByRole("button", { name: "Open Fund Two details" }));
    await userEvent.click(screen.getByRole("button", { name: "Show data records" }));
    await userEvent.click(screen.getByRole("button", { name: "Maintain data" }));
    expect(await screen.findByRole("textbox", { name: "Name" })).toHaveValue("Fund Two");
  }, 15_000);

  it("returns from market health to market data with the navigation trail intact", async () => {
    await i18n.changeLanguage("en");
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.marketData") }));
    expect((await screen.findAllByTestId("data-health-indicator")).find(element => !element.closest("[hidden]"))!).toBeInTheDocument();
    await userEvent.click(screen.getAllByTestId("data-health-indicator").find(element => !element.closest("[hidden]"))!);
    expect(await screen.findByTestId("data-health-page")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Data maintenance(?: ·|$)/ }));
    expect((await screen.findAllByTestId("data-health-indicator")).find(element => !element.closest("[hidden]"))!).toBeInTheDocument();
  });

  it("opens the Available funds page from the workspace nav", async () => {
    await i18n.changeLanguage("en");
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.availableFunds") }));
    expect(await screen.findByTestId("available-funds-page")).toBeInTheDocument();
  });

  it("returns to the accounts list after opening an account from Portfolio", async () => {
    await i18n.changeLanguage("en");
    listAccounts.mockResolvedValue([checkingAccount]);
    accountValuations.mockResolvedValue([checkingValuation]);
    portfolio.mockResolvedValue({ ...emptyPortfolio, accounts: [checkingValuation] });
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    await screen.findByTestId("overview-net-worth");

    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.portfolio") }));
    await userEvent.click(await screen.findByRole("tab", { name: "Allocation & trend" }));
    await userEvent.click(await screen.findByRole("button", { name: "Checking" }));
    expect(await screen.findByTestId("account-detail")).toBeInTheDocument();

    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.overview") }));
    await screen.findByTestId("overview-net-worth");
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.accounts") }));

    expect(await screen.findByTestId("accounts-table")).toBeInTheDocument();
    expect(screen.queryByTestId("account-detail")).not.toBeInTheDocument();
  });

  it("unmounts inactive workspace pages so their queries do not stay subscribed", async () => {
    await i18n.changeLanguage("en");
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();

    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.settings") }));
    expect(await screen.findByRole("form", { name: i18n.t("settings.formLabel") })).toBeInTheDocument();
    expect(screen.queryByTestId("overview-net-worth")).not.toBeInTheDocument();

    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.overview") }));
    expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
    expect(screen.queryByRole("form", { name: i18n.t("settings.formLabel") })).not.toBeInTheDocument();
  });
});


it.each(["lavender", "mint", "sky", "peach"] as const)("restores %s accent on cold load and settings reload", async (accent) => {
  settingsLoad.mockResolvedValue({ ...defaultSettings, accent });
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", accent));
  settingsLoad.mockResolvedValue({ ...defaultSettings, accent: "indigo" });
  await queryClient.invalidateQueries({ queryKey: ["settings"] });
  await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "indigo"));
});

it("returns from account analysis with the source archive filter and scroll position intact", async () => {
  await i18n.changeLanguage("en");
  listAccounts.mockResolvedValue([checkingAccount]);
  accountValuations.mockResolvedValue([checkingValuation]);
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await userEvent.click(within(nav).getByRole("button", { name: "Accounts" }));
  await userEvent.click(await screen.findByRole("checkbox", { name: "Show archived" }));
  await userEvent.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  await screen.findByTestId("account-detail");
  const main = document.getElementById("main-content")!;
  main.scrollTop = 170;
  await userEvent.click(within(screen.getByRole("banner")).getByRole("button", { name: "Return analysis" }));
  expect(await screen.findByRole("button", { name: /^Go back to Accounts(?: ·|$)/ })).toBeVisible();
  expect(screen.getByTestId("account-detail")).not.toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Accounts(?: ·|$)/ }));
  await waitFor(() => expect(main.scrollTop).toBe(170));
  expect(screen.getByTestId("account-detail")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Back to accounts" }));
  expect(screen.getByRole("checkbox", { name: "Show archived" })).toBeChecked();
});


it("opens Wealth history from Overview, removes current queries and exits back", async () => {
  await i18n.changeLanguage("en");
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  // The Overview entry keeps a navigation trail, but must unmount its reads.
  const entry = screen.getAllByRole("button", { name: i18n.t("nav.historicalOverview") }).find(button => !nav.contains(button));
  expect(entry).toBeDefined();
  await userEvent.click(entry!);
  await screen.findByRole("button", { name: i18n.t("historicalOverview.date") });
  expect(screen.queryByTestId("overview-net-worth")).not.toBeInTheDocument();
  await waitFor(() => expect(queryClient.getQueryCache().find({ queryKey: ["overview"] })?.getObserversCount() ?? 0).toBe(0));
  await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.exit") }));
  expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
  await waitFor(() => expect(document.getElementById("main-content")).toHaveFocus());
  // The explicit current-overview action focuses the stable main content while reads reload.
  await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.historicalOverview") }));
  await screen.findByRole("button", { name: i18n.t("historicalOverview.date") });
  await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.exit") }));
  expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
  await waitFor(() => expect(document.getElementById("main-content")).toHaveFocus());
});


it("retains the upcoming filter and restores its trigger after Back", async () => {
  render(<AppProviders><App /></AppProviders>);
  await userEvent.click(await screen.findByRole("button", { name: "View all 1 action" }));
  // Tasks is lazy-loaded: finish that boundary before querying its controls.
  await vi.dynamicImportSettled();
  const filter = await screen.findByRole("button", { name: "Reservation issues" });
  await userEvent.click(filter);
  const health = await screen.findByRole("button", { name: i18n.t("upcomingActions.checksEntry", { count: 0 }) });
  await userEvent.click(health);
  await userEvent.click(await screen.findByRole("button", { name: /^Go back to Tasks & checks(?: ·|$)/ }));
  await waitFor(() => expect(filter).toHaveAttribute("aria-pressed", "true"));
  await waitFor(() => expect(health).toHaveFocus());
  expect(screen.queryByRole("button", { name: "Verify receipt" })).not.toBeInTheDocument();
});


// Keep the actual App trail and liquidity page; stub only the financial detail sheet.
vi.mock("@/features/liquidity/ProductSheets", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/liquidity/ProductSheets")>();
  return {
    ...actual,
    ProductDetailSheet: ({ open, productId, onOpenChange }: { open: boolean; productId: string; onOpenChange: (open: boolean) => void }) => open ? <div role="dialog" aria-label={productId}><button onClick={() => onOpenChange(false)}>Close product</button></div> : null,
  };
});

it("returns through Upcoming actions to open another product and does not reopen it on Back", async () => {
  upcomingRead.mockResolvedValue(upcomingData([
    upcomingItem({ id: "action-a", productId: "product-a", displayName: "Product A", phase: "blocked", primaryTarget: { kind: "product", productId: "product-a", sourceRef: null, reservationId: null } }),
    upcomingItem({ id: "action-b", productId: "product-b", displayName: "Product B", phase: "blocked", primaryTarget: { kind: "product", productId: "product-b", sourceRef: null, reservationId: null } }),
  ]));
  render(<AppProviders><App /></AppProviders>);
  await userEvent.click(await screen.findByRole("button", { name: "View terms · Product A" }));
  await screen.findByRole("dialog", { name: "product-a" });
  await userEvent.click(screen.getByRole("button", { name: "Close product" }));
  expect(screen.queryByRole("button", { name: "View all 2 actions" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Overview(?: ·|$)/ }));
  await userEvent.click(await screen.findByRole("button", { name: "View all 2 actions" }));
  await userEvent.click(await screen.findByRole("button", { name: "View terms · Product B" }));
  await screen.findByRole("dialog", { name: "product-b" });
  await userEvent.click(screen.getByRole("button", { name: "Close product" }));
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Tasks & checks(?: ·|$)/ }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Overview(?: ·|$)/ }));
  await screen.findByTestId("overview-net-worth");
  // Flush retained-page reads before checking that no detail sheet reopens.
  await waitFor(() => expect(queryClient.isFetching()).toBe(0));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /^Go back to Overview(?: ·|$)/ })).not.toBeInTheDocument();
});


it("preserves same-page Data Health gap focus history", async () => {
  scanHealth.mockResolvedValue({ healthy: false, issueCount: 2, executableCount: 0, prerequisiteCount: 0, snapshotDays: 2, issues: [
    { id: "gap-a", kind: "snapshot_incomplete", severity: "warning", targetKey: "cash:a", accountId: "bank-a", label: "USD", rangeStart: "2026-09-18", rangeEnd: "2026-09-18", action: "none", executable: false },
    { id: "gap-b", kind: "snapshot_incomplete", severity: "warning", targetKey: "cash:b", accountId: "bank-b", label: "CNY", rangeStart: "2026-09-19", rangeEnd: "2026-09-19", action: "none", executable: false },
  ] });
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  await userEvent.click(within(screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") })).getByRole("button", { name: "Data maintenance" }));
  await userEvent.click(await screen.findByRole("tab", { name: "Issues & repair" }));
  const rows = await screen.findAllByTestId("health-issue-snapshot_incomplete");
  expect(rows).toHaveLength(2);
  await userEvent.click(within(rows[0]).getByRole("button", { name: "View gap" }));
  await waitFor(() => expect(screen.getAllByTestId("health-issue-snapshot_incomplete")).toHaveLength(1));
  await userEvent.click(await screen.findByRole("button", { name: /^Go back to Data Health(?: ·|$)/ }));
  await waitFor(() => expect(screen.getAllByTestId("health-issue-snapshot_incomplete")).toHaveLength(2));
});


vi.mock("@/features/history/HistoryPage", () => ({ HistoryPage: ({ navigationFilters }: { navigationFilters?: unknown }) => <section aria-label="Synthetic transaction history"><pre>{JSON.stringify(navigationFilters)}</pre></section> }));

afterEach(() => vi.useRealTimers());

it.each([
  ["UTC", "2026-10-31T23:59:30Z", "2026-11-01T00:00:30Z", false],
  ["UTC", "2026-10-31T23:59:30Z", "2026-11-01T00:00:30Z", true],
  ["Asia/Shanghai", "2026-10-31T15:59:30Z", "2026-10-31T16:00:30Z", false],
  ["Asia/Shanghai", "2026-10-31T15:59:30Z", "2026-10-31T16:00:30Z", true],
])("returns from ledger activity across a household month boundary with the same capture, dates, detail and position (%s)", async (timezone, before, after, trendEnabled) => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(before as string));
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone, startedAt: "2026-01-01T00:00:00Z" });
  await i18n.changeLanguage("en");
  const cell = { status: "active", included: true, complete: true, currency: "USD", nativeAmount: "10", baseAmount: "10", assetClass: "cash", manual: false, missing: [], missingDetails: [] };
  const state = { ...defaultHistorical.left, date: "2026-09-30", knownAssets: "10", knownLiabilities: "0" };
  historicalRead.mockImplementation((date: string, compareTo: string) => Promise.resolve({ ...defaultHistorical, capturedAt: new Date().toISOString(), categories: [], timezone, originDate: "2026-01-01", lastClosedDate: "2026-10-11", inputGeneration: 7, resolverPolicy: "policy", left: { ...state, date }, right: compareTo ? { ...state, date: compareTo, current: compareTo === "current" } : null, rows: [{ key: "account", accountId: "explicit-account", kind: "account", name: "Synthetic retained bank", role: "asset", parentKey: "", left: cell, right: compareTo ? cell : null, changed: false }, { key: "leaf", accountId: "explicit-account", kind: "balance", name: "USD", role: "asset", parentKey: "account", left: cell, right: compareTo ? cell : null, changed: false }] }));
  seriesRead.mockImplementation(async (from: string, to: string) => {
    const overview = await historicalRead(from, to);
    return { overview, from, to, work: { leaves: 1, candidateFacts: 1, candidateBytes: 1, queries: 1 }, points: [from, to].map((date, index) => ({ date, cutoffAt: `${date}T23:59:59Z`, available: true, gapReason: "", assets: "10", liabilities: "0", netWorth: "10", knownAssets: "10", knownLiabilities: "0", assetsComplete: true, liabilitiesComplete: true, assetCoverage: { included: 1, known: 1, missing: 0 }, liabilityCoverage: { included: 0, known: 0, missing: 0 }, missing: [], scopeChanged: false, basisChanged: false, connectAssets: index > 0, connectLiabilities: index > 0, connectNetWorth: index > 0 })) };
  });
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await userEvent.click(within(nav).getByRole("button", { name: "Wealth history" }));
  await screen.findByLabelText("Historical date", {}, { timeout: 5000 });
  await userEvent.click(within(screen.getByRole("group", { name: "View mode" })).getByRole("button", { name: "Date vs. now" }));
  if (trendEnabled) {
    await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
    await userEvent.click(screen.getByRole("button", { name: "Enable trend · New capture" }));
    const trend = await screen.findByRole("region", { name: "Time trend" });
    await userEvent.click(within(trend).getByRole("button", { name: "Assets" }));
    await userEvent.click(within(trend).getByRole("button", { name: "2026-09-30" }));
  }
  await userEvent.click(await screen.findByRole("tab", { name: "Account details" }));
  await userEvent.click(await screen.findByRole("button", { name: "Synthetic retained bank" }));
  const dialog = await screen.findByRole("dialog");
  const sourceButton = within(dialog).getByRole("button", { name: "Leave wealth history · View ledger activity" });
  const main = document.getElementById("main-content")!; main.scrollTop = 210;
  const capturedReads = historicalRead.mock.calls.length; const seriesReads = seriesRead.mock.calls.length;
  const capturedAt = new Date(before as string).toISOString();
  expect(screen.getByLabelText(i18n.t("historicalOverview.baselineDate"))).toHaveTextContent("2026-09-30");
  expect(screen.getAllByTitle(capturedAt).length).toBeGreaterThan(0);
  await userEvent.click(sourceButton);
  const history = await screen.findByRole("region", { name: "Synthetic transaction history" });
  expect(history).toHaveTextContent('"accountId":"explicit-account"');
  expect(history).toHaveTextContent('"from":"2026-09-30"'); expect(history).toHaveTextContent(trendEnabled ? '"to":"2026-10-30"' : '"to":"2026-10-11"');
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  vi.setSystemTime(new Date(after as string));
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Wealth history(?: ·|$)/ }));
  await waitFor(() => expect(screen.getByRole("dialog")).toHaveTextContent("Synthetic retained bank"));
  expect(within(screen.getByRole("group", { name: "View mode", hidden: true })).getByRole("button", { name: trendEnabled ? "Compare dates" : "Date vs. now", hidden: true })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByLabelText(i18n.t("historicalOverview.baselineDate"))).toHaveTextContent("2026-09-30");
  expect(screen.getAllByTitle(capturedAt).length).toBeGreaterThan(0);
  expect(document.querySelector('[role="tab"][aria-selected="true"]')).toHaveTextContent("Account details");
  expect(document.querySelector('[role="tab"][aria-selected="true"]')).toHaveAttribute("aria-selected", "true");
  await waitFor(() => expect(main.scrollTop).toBe(210));
  await waitFor(() => expect(screen.getByRole("button", { name: "Leave wealth history · View ledger activity" })).toHaveFocus());
  expect(historicalRead).toHaveBeenCalledTimes(capturedReads); expect(seriesRead).toHaveBeenCalledTimes(seriesReads);
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByRole("button", { name: "Synthetic retained bank" })).toHaveFocus();
  if (trendEnabled) {
    await userEvent.click(screen.getByRole("tab", { name: "Time trend" }));
    const trend = screen.getByRole("region", { name: "Time trend" });
    expect(within(trend).getByRole("button", { name: "Assets" })).toHaveAttribute("aria-pressed", "true");
    expect(within(trend).getByRole("button", { name: "2026-09-30" })).toHaveAttribute("aria-pressed", "true");
    expect(seriesRead).toHaveBeenCalledTimes(seriesReads);
  }
}, 15000);


it("blocks top-level and object navigation while a sheet confirmation is unresolved", async () => {
  await i18n.changeLanguage("en");
  listAccounts.mockResolvedValue([checkingAccount]);
  accountValuations.mockResolvedValue([checkingValuation]);
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const owner = {};
  registerSheetConfirmation(owner);
  try {
    await userEvent.click(within(nav).getByRole("button", { name: "Accounts" }));
    expect(screen.getByTestId("overview-net-worth")).toBeVisible();
    await userEvent.click(document.getElementById("open-historical-overview")!);
    expect(screen.getByTestId("overview-net-worth")).toBeVisible();
    expect(screen.queryByRole("button", { name: /^Go back to Overview(?: ·|$)/ })).not.toBeInTheDocument();
  } finally { resolveSheetConfirmation(owner); }
  await userEvent.click(within(nav).getByRole("button", { name: "Accounts" }));
  await userEvent.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  await screen.findByTestId("account-detail");
  await userEvent.click(within(screen.getByRole("banner")).getByRole("button", { name: "Return analysis" }));
  const back = await screen.findByRole("button", { name: /^Go back to Accounts(?: ·|$)/ });
  registerSheetConfirmation(owner);
  try {
    await userEvent.click(back);
    expect(back).toBeVisible();
    expect(screen.getByTestId("account-detail")).not.toBeVisible();
  } finally { resolveSheetConfirmation(owner); }
  await userEvent.click(back);
  expect(screen.getByTestId("account-detail")).toBeVisible();
});


it.each(["startup", "settings", "bootstrap"])("keeps the workspace through a failed %s refresh while confirmation is unresolved", async read => {
  await i18n.changeLanguage("en");
  render(<AppProviders><App /></AppProviders>);
  const original = await screen.findByTestId("overview-net-worth");
  const key = read === "startup" ? queryKeys.app.startup : read === "settings" ? queryKeys.settings.all : queryKeys.household.bootstrap;
  const query = queryClient.getQueryCache().find({ queryKey: key })!;
  const originalFn = query.options.queryFn;
  // Inject a read failure at the actual query boundary without changing data.
  query.setOptions({ ...query.options, retry: false, queryFn: async () => { throw new Error("read unavailable"); } });
  const owner = {};
  registerSheetConfirmation(owner);
  try {
    await queryClient.refetchQueries({ queryKey: key });
    await waitFor(() => expect(query.state.status).toBe("error"));
    expect(screen.getByTestId("overview-net-worth")).toBe(original);
    expect(screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") })).toBeVisible();
  } finally {
    resolveSheetConfirmation(owner);
    query.setOptions({ ...query.options, queryFn: originalFn });
    await queryClient.refetchQueries({ queryKey: key });
  }
});


it("blocks App and object navigation during an actual ordinary submission and its unknown recovery", async () => {
  await i18n.changeLanguage("en");
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2020-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([checkingAccount]); accountValuations.mockResolvedValue([checkingValuation]);
  let reject!: (error: Error) => void;
  ordinaryRecord.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; })).mockResolvedValue({});
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await userEvent.click(within(nav).getByRole("button", { name: "Accounts" }));
  const overviewNav = within(nav).getByRole("button", { name: "Overview" });
  await userEvent.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  const detail = await screen.findByTestId("account-detail");
  const analysis = within(screen.getByRole("banner")).getByRole("button", { name: "Return analysis" });
  await userEvent.click(screen.getByRole("button", { name: "Update balance" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  const value = within(form).getByLabelText("New value");
  await userEvent.click(value); await userEvent.keyboard("{Control>}a{/Control}1125");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(ordinaryRecord).toHaveBeenCalledTimes(1));
  await userEvent.click(overviewNav); await userEvent.click(analysis);
  expect(detail).toBeVisible(); expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  await act(async () => { reject(new Error("outcome unknown")); });
  const retry = await within(form).findByRole("button", { name: "Retry same submission" });
  await userEvent.click(overviewNav); await userEvent.click(analysis);
  expect(detail).toBeVisible(); expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  expect(ordinaryRecord).toHaveBeenCalledTimes(1);
  await userEvent.click(retry);
  await waitFor(() => expect(screen.queryByRole("form", { name: "Record change" })).not.toBeInTheDocument());
  expect(ordinaryRecord.mock.calls[1]).toEqual(ordinaryRecord.mock.calls[0]);
  await userEvent.click(overviewNav);
  expect(screen.getByTestId("overview-net-worth")).toBeVisible();
});

it.each(["zh-TW"] as const)("keeps investment review reachable from Overview with no upcoming items in %s", async language => {
  await i18n.changeLanguage(language);
  settingsLoad.mockResolvedValue({ ...defaultSettings, language });
  upcomingRead.mockResolvedValue(upcomingData());
  render(<AppProviders><App /></AppProviders>);
  const entry = await screen.findByRole("button", { name: new RegExp(i18n.t("tasks.tabs.review")) });
  entry.focus();
  await userEvent.keyboard("{Enter}");
  expect(await screen.findByRole("heading", { name: i18n.t("tasks.title"), level: 2 })).toBeVisible();
  expect(screen.getByText(i18n.t("tasks.description"))).toBeVisible();
  const reviewTab = screen.getByRole("tab", { name: i18n.t("reviewV2.title") });
  expect(reviewTab).toHaveAttribute("aria-selected", "true");
  expect(await screen.findByRole("textbox", { name: i18n.t("reviewV2.search") })).toBeVisible();
  const header = within(document.querySelector("header")!);
  expect(header.getByRole("button", { name: i18n.t("reviewV2.refresh") })).toBeVisible();
  expect(header.getByRole("button", { name: i18n.t("reviewV2.historyTitle") })).toBeVisible();
  expect(header.getByRole("button", { name: i18n.t("reviewV2.ignoredList") })).toBeVisible();
  expect(header.getByRole("button", { name: i18n.t("reviewV2.rules") })).toBeVisible();
  expect(reviewTab).toHaveAttribute("aria-selected", "true");
});

it("preserves filtered page two through the real instrument route and focuses both Back destinations", async () => {
  const user = userEvent.setup();
  await i18n.changeLanguage("en");
  const instrument = { id: "synthetic-second", householdId: "synthetic-household", name: "Synthetic Second Holding", type: "mutual_fund", quoteCurrency: "USD", symbol: "SYN2", iconKey: "", sortOrder: 0, quoteSource: "manual", createdAt: "2026-01-01", updatedAt: "2026-01-01" };
  const firstInstrument = { ...instrument, id: "synthetic-first", name: "Synthetic First Holding", symbol: "SYN1" };
  const amounts = { quantity: "3", totalCost: { amount: "30", currency: "USD" }, currentValue: { amount: "36", currency: "USD" }, unrealizedGain: { amount: "6", currency: "USD" } };
  listInstruments.mockResolvedValue([firstInstrument, instrument]);
  listAccounts.mockResolvedValue([{ account: { id: "synthetic-broker", name: "Synthetic Broker" } }]);
  instrumentGroups.mockResolvedValue([{ key: "mutual_fund", subtotals: [], instruments: [{ instrumentId: instrument.id, name: instrument.name, type: instrument.type, symbol: instrument.symbol, quoteCurrency: "USD", quantityUnit: "shares", archived: false, amounts, holdings: [{ holdingId: "synthetic-position", accountId: "synthetic-broker", accountName: "Synthetic Broker", amounts }] }] }]);
  reviewAlerts.mockImplementation(async (request: { cursor?: string; filter: { status: string; instrumentType: string; query: string } }) => {
    const secondPage = request.cursor === "synthetic-page-two";
    const row = secondPage ? instrument : firstInstrument;
    return {
      items: [{ instrumentId: row.id, displayName: row.name, instrumentType: row.type, currency: "USD", status: "pending", rules: [] }],
      nextCursor: secondPage ? "" : "synthetic-page-two", summary: { totalGroups: 2, matchedGroups: 2, pendingGroups: 2, acknowledgedGroups: 0, ignoredGroups: 0, unknownGroups: 0, pausedGroups: 0, actionableRules: 0 },
      reportRevision: "synthetic-review-page", projectionStatus: "complete", publishedOn: "2026-10-15",
    };
  });
  useRealInstrumentDetail.enabled = true;
  upcomingRead.mockResolvedValue(upcomingData());

  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const overviewReviewTrigger = screen.getByRole("button", { name: new RegExp(i18n.t("tasks.tabs.review")) });
  await user.click(overviewReviewTrigger);
  await user.click(await screen.findByRole("tab", { name: i18n.t("reviewV2.title") }));
  await screen.findByRole("heading", { name: "Synthetic First Holding" });
  await user.type(screen.getByRole("textbox", { name: i18n.t("reviewV2.search") }), "Synthetic");
  await user.selectOptions(screen.getByRole("combobox", { name: i18n.t("reviewV2.typeFilter") }), "mutual_fund");
  await user.click(screen.getByRole("button", { name: i18n.t("reviewV2.status.pending") }));
  const selectedFilter = { status: "pending", instrumentType: "mutual_fund", query: "Synthetic" };
  await waitFor(() => expect(reviewAlerts).toHaveBeenCalledWith(expect.objectContaining({ filter: selectedFilter })));
  await user.click(screen.getByRole("button", { name: i18n.t("reviewV2.next") }));
  await screen.findByRole("heading", { name: "Synthetic Second Holding" });
  await waitFor(() => expect(reviewAlerts).toHaveBeenCalledWith(expect.objectContaining({ cursor: "synthetic-page-two", filter: selectedFilter })));
  const reviewReadCount = reviewAlerts.mock.calls.length;

  const secondActions = screen.getByRole("button", { name: i18n.t("reviewV2.moreFor", { name: "Synthetic Second Holding" }) });
  await user.click(secondActions);
  await user.click(await screen.findByRole("menuitem", { name: i18n.t("reviewV2.viewHoldings") }));
  expect(await screen.findByTestId("instrument-detail-page")).toBeVisible();
  expect(screen.getByRole("heading", { name: "SYN2", level: 2 })).toBeVisible();
  expect(screen.getByRole("tab", { name: i18n.t("instrumentDetail.holdings") })).toHaveAttribute("aria-selected", "true");
  expect((await screen.findAllByText("US$36.00"))[0]).toBeVisible();
  expect(instrumentGroups).toHaveBeenCalledWith();

  await queryClient.invalidateQueries({ queryKey: reviewV2Key });
  await waitFor(() => expect(reviewAlerts.mock.calls.length).toBeGreaterThan(reviewReadCount));
  const hiddenReads = reviewAlerts.mock.calls.slice(reviewReadCount);
  expect(hiddenReads.every((call) => {
    const request = call[0] as { cursor?: string; filter?: { status?: string } };
    return request.filter?.status === "unignored" && !request.cursor;
  })).toBe(true);
  const afterHidden = reviewAlerts.mock.calls.length;
  await user.click(screen.getByRole("button", { name: /^Go back to / }));
  expect(await screen.findByRole("textbox", { name: i18n.t("reviewV2.search") })).toHaveValue("Synthetic");
  expect(screen.getByRole("combobox", { name: i18n.t("reviewV2.typeFilter") })).toHaveValue("mutual_fund");
  expect(screen.getByRole("button", { name: i18n.t("reviewV2.status.pending") })).toHaveAttribute("aria-pressed", "true");
  expect(await screen.findByRole("heading", { name: "Synthetic Second Holding" })).toBeVisible();
  const reviewHeading = document.getElementById("tasks-heading");
  expect(reviewHeading).not.toBeNull();
  await waitFor(() => expect(reviewHeading).toHaveFocus());
  expect(reviewHeading).toBeVisible();
  expect(reviewHeading?.closest("[hidden], [inert], [aria-hidden='true']")).toBeNull();
  await waitFor(() => expect(reviewAlerts.mock.calls.length).toBeGreaterThan(afterHidden));

  await user.click(screen.getByRole("button", { name: /^Go back to / }));
  expect(await screen.findByTestId("overview-net-worth")).toBeVisible();
  await waitFor(() => expect(overviewReviewTrigger).toHaveFocus());
  expect(overviewReviewTrigger).toBeVisible();
  expect(overviewReviewTrigger.closest("[hidden], [inert], [aria-hidden='true']")).toBeNull();
});

describe("reconciliation navigation boundaries", () => {
  // Each case seeds its own relevant navigation state and preserves a complete
  // Back/reset journey. Keeping independent contracts separate avoids making
  // their combined interaction cost compete for one unchanged 5s test budget.
  async function openCheckingAccount(rememberReconciliation = false) {
    const user = userEvent.setup();
    await i18n.changeLanguage("en");
    const savingsAccount = {
      ...checkingAccount,
      account: { ...checkingAccount.account, id: "acc-2", name: "Savings" },
      latestValue: { ...checkingAccount.latestValue, id: "v2", accountId: "acc-2" },
    };
    const savingsValuation = { ...checkingValuation, account: savingsAccount.account };
    const counts = { pending: 1, confirmed: 0, skipped: 0, unresolved: 0 };
    listAccounts.mockResolvedValue([checkingAccount, savingsAccount]);
    accountValuations.mockResolvedValue([checkingValuation, savingsValuation]);
    reconciliationList.mockResolvedValue([
      { id: "open-checking", status: "in_progress", accountIds: ["acc-1"], accountNames: ["Checking"], counts },
      { id: "open-savings", status: "in_progress", accountIds: ["acc-2"], accountNames: ["Savings"], counts },
    ]);
    reconciliationLast.mockResolvedValue({ id: "session-last", completedAt: "2026-10-20T10:20:00Z" });
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    if (rememberReconciliation) {
      // Back restores the prior Tasks tab. Seed the global reconciliation entry
      // that preceded the account-local journey in the original scenario.
      await user.click(await screen.findByRole("button", { name: "Reconciliation, 2 unfinished" }));
      await vi.dynamicImportSettled();
      expect(await screen.findByRole("checkbox", { name: "Savings · USD" })).toBeChecked();
    }
    await user.click(within(nav).getByRole("button", { name: "Accounts" }));
    await user.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
    return { user, nav };
  }

  it("retains an explicit session through Back and blocks navigation during confirmation", async () => {
    const { user, nav } = await openCheckingAccount();
    await user.click(await screen.findByRole("button", { name: "View last successful record" }));
    // Wait for the cold Tasks route before querying its controls.
    await vi.dynamicImportSettled();
    expect(await screen.findByRole("button", { name: "Back to session list" })).toBeVisible();

    await user.click(screen.getByRole("tab", { name: /Upcoming actions/ }));
    await user.click(await screen.findByRole("button", { name: i18n.t("upcomingActions.checksEntry", { count: 0 }) }));
    await vi.dynamicImportSettled();
    await user.click(await screen.findByRole("button", { name: /^Go back to Tasks & checks(?: ·|$)/ }));
    await user.click(screen.getByRole("tab", { name: /Reconciliation/ }));
    expect(screen.getByRole("button", { name: "Back to session list" })).toBeVisible();

    const owner = {};
    registerSheetConfirmation(owner);
    try {
      await user.click(within(nav).getByRole("button", { name: /Tasks & checks/ }));
      expect(screen.getByRole("button", { name: "Back to session list" })).toBeVisible();
    } finally {
      resolveSheetConfirmation(owner);
    }
  });

  it("resets reconciliation to the global list unless the target names an account or session", async () => {
    const { user, nav } = await openCheckingAccount();
    await user.click(await screen.findByRole("button", { name: "View last successful record" }));
    await vi.dynamicImportSettled();
    expect(await screen.findByRole("button", { name: "Back to session list" })).toBeVisible();
    await user.click(within(nav).getByRole("button", { name: "Overview" }));
    await screen.findByTestId("overview-net-worth");
    await user.click(await screen.findByRole("button", { name: "Reconciliation, 2 unfinished" }));
    expect(await screen.findByText(/Checking · In progress/)).toBeVisible();
    expect(screen.getByText(/Savings · In progress/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Back to session list" })).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Checking · USD" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Savings · USD" })).toBeChecked();

    await user.click(within(nav).getByRole("button", { name: "Accounts" }));
    await user.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
    await user.click(screen.getByRole("button", { name: "Open account reconciliation" }));
    expect(await screen.findByRole("checkbox", { name: "Checking · USD" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Savings · USD" })).not.toBeChecked();
    expect(screen.getByText(/Savings · In progress/)).not.toBeVisible();
  });

  it("restores a local account session through Back and resets top-level Tasks to all accounts", async () => {
    const { user, nav } = await openCheckingAccount(true);
    await user.click(await screen.findByRole("button", { name: "Open account reconciliation" }));
    await vi.dynamicImportSettled();
    expect(await screen.findByRole("checkbox", { name: "Checking · USD" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Savings · USD" })).not.toBeChecked();
    // Continue locally: the navigation target contains only the account, not this session.
    await user.click(screen.getByRole("button", { name: "Continue session" }));
    expect(await screen.findByRole("button", { name: "Back to session list" })).toBeVisible();
    await user.click(screen.getByRole("tab", { name: /Upcoming actions/ }));
    await user.click(await screen.findByRole("button", { name: i18n.t("upcomingActions.checksEntry", { count: 0 }) }));
    await vi.dynamicImportSettled();
    await user.click(await screen.findByRole("button", { name: /^Go back to Tasks & checks(?: ·|$)/ }));
    await user.click(screen.getByRole("tab", { name: /Reconciliation/ }));
    expect(screen.getByRole("button", { name: "Back to session list" })).toBeVisible();
    expect(reconciliationGet).toHaveBeenCalledWith("open-checking");

    await user.click(screen.getByRole("button", { name: /^Go back to Accounts(?: ·|$)/ }));
    expect(await screen.findByTestId("account-detail")).toBeVisible();
    await user.click(within(nav).getByRole("button", { name: /Tasks & checks/ }));
    expect(await screen.findByRole("checkbox", { name: "Savings · USD" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Checking · USD" })).toBeChecked();
    expect(screen.queryByRole("button", { name: "Back to session list" })).not.toBeInTheDocument();
  });
});

it("opens the global reconciliation list with completed history when Tasks is reselected on Tasks", async () => {
  await i18n.changeLanguage("en");
  listAccounts.mockResolvedValue([checkingAccount]);
  accountValuations.mockResolvedValue([checkingValuation]);
  reconciliationList.mockResolvedValue([
    { id: "local-session", status: "in_progress", accountIds: ["acc-1"], accountNames: ["Checking"], counts: { pending: 1, confirmed: 0, skipped: 0, unresolved: 0 } },
  ]);
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  await userEvent.click(await screen.findByRole("button", { name: "Reconciliation, 1 unfinished" }));
  const filter = screen.getByLabelText(i18n.t("reconciliation.history"));
  await userEvent.selectOptions(filter, "paused");
  await userEvent.click(screen.getByRole("checkbox", { name: "Checking · USD" }));
  await userEvent.click(screen.getByRole("tab", { name: /Upcoming actions/ }));
  await userEvent.click(screen.getByRole("tab", { name: /Reconciliation/ }));
  expect(filter).toHaveValue("paused");
  expect(screen.getByRole("checkbox", { name: "Checking · USD" })).not.toBeChecked();
  await userEvent.click(await screen.findByRole("button", { name: "Continue session" }));
  expect(await screen.findByRole("button", { name: "Back to session list" })).toBeVisible();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const owner = {};
  registerSheetConfirmation(owner);
  try {
    await userEvent.click(within(nav).getByRole("button", { name: /Tasks & checks/ }));
    expect(screen.getByRole("button", { name: "Back to session list" })).toBeVisible();
  } finally {
    resolveSheetConfirmation(owner);
  }
  await userEvent.click(within(nav).getByRole("button", { name: /Tasks & checks/ }));
  expect(await screen.findByRole("button", { name: "Continue session" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "Back to session list" })).not.toBeInTheDocument();
  expect(screen.getByLabelText(i18n.t("reconciliation.history"))).toHaveValue("completed");
  expect(screen.getByRole("checkbox", { name: "Checking · USD" })).toBeChecked();
});

it.each([false, true])("restores the original reconciliation entry after a nested account reconciliation (local session: %s)", async (continueSession) => {
  await i18n.changeLanguage("en");
  const savings = { ...checkingAccount, account: { ...checkingAccount.account, id: "acc-2", name: "Savings" }, latestValue: { ...checkingAccount.latestValue, accountId: "acc-2" } };
  listAccounts.mockResolvedValue([checkingAccount, savings]);
  accountValuations.mockResolvedValue([checkingValuation, { ...checkingValuation, account: savings.account }]);
  const counts = { pending: 0, confirmed: 0, skipped: 0, unresolved: 0 };
  reconciliationList.mockResolvedValue([
    { id: "session-a", status: "in_progress", accountIds: ["acc-1"], accountNames: ["Checking"], counts },
    { id: "session-b", status: "in_progress", accountIds: ["acc-2"], accountNames: ["Savings"], counts },
  ]);
  reconciliationGet.mockImplementation((id: string) => Promise.resolve({ id, householdId: "synthetic", status: "in_progress", revision: 1, startedAt: "2026-10-20T10:00:00Z", note: `Synthetic session ${id}`, closingNote: "", partial: false, accounts: [], counts }));
  scanHealth.mockResolvedValue({ healthy: false, issueCount: 1, executableCount: 0, prerequisiteCount: 0, snapshotDays: 0, issues: [
    { id: "savings-value", kind: "manual_value_missing", severity: "warning", accountId: "acc-2", label: "Savings", action: "none", executable: false },
  ] });
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await userEvent.click(within(nav).getByRole("button", { name: "Accounts" }));
  await userEvent.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Open account reconciliation" }));
  await userEvent.selectOptions(await screen.findByLabelText(i18n.t("reconciliation.history")), "paused");
  if (continueSession) {
    await userEvent.click(screen.getByRole("button", { name: "Continue session" }));
    await userEvent.click(await screen.findByRole("button", { name: "Finish session" }));
    await userEvent.type(screen.getByLabelText("Closing explanation"), "Synthetic A draft");
    await userEvent.keyboard("{Escape}");
  }
  await userEvent.click(screen.getByRole("tab", { name: /Upcoming actions/ }));
  await userEvent.click(await screen.findByRole("button", { name: i18n.t("upcomingActions.checksEntry", { count: 0 }) }));
  await userEvent.click(await screen.findByRole("button", { name: "View gap" }));
  await screen.findByRole("dialog");
  await userEvent.keyboard("{Escape}");
  expect(await screen.findByTestId("account-detail")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Open account reconciliation" }));
  expect(await screen.findByRole("checkbox", { name: "Savings · USD" })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: "Checking · USD" })).not.toBeChecked();
  const nestedFilter = screen.getByRole("combobox", { name: i18n.t("reconciliation.history") });
  expect(nestedFilter).toHaveValue("completed");
  await userEvent.selectOptions(nestedFilter, "completed");
  await userEvent.click(screen.getByRole("button", { name: "Continue session" }));
  expect(await screen.findByText(/Synthetic session session-b/)).toBeVisible();
  const owner = {};
  registerSheetConfirmation(owner);
  try {
    await userEvent.click(screen.getByRole("button", { name: /^Go back to Accounts(?: ·|$)/ }));
    expect(screen.getByText(/Synthetic session session-b/)).toBeVisible();
  } finally {
    resolveSheetConfirmation(owner);
  }
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Accounts(?: ·|$)/ }));
  // Forward retains the nested entry and its editable session while it is hidden.
  expect(screen.getByText(/Synthetic session session-b/)).not.toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Data Health(?: ·|$)/ }));
  await userEvent.click(screen.getByRole("button", { name: /^Go back to Tasks & checks(?: ·|$)/ }));
  await userEvent.click(screen.getByRole("tab", { name: /Reconciliation/ }));
  if (continueSession) {
    expect(await screen.findByText(/Synthetic session session-a/)).toBeVisible();
    expect(screen.getByText(/Synthetic session session-b/)).not.toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Finish session" }));
    expect(screen.getByLabelText("Closing explanation")).toHaveValue("Synthetic A draft");
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Back to session list" }));
  }
  expect(screen.getByRole("checkbox", { name: "Checking · USD" })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: "Savings · USD" })).not.toBeChecked();
  expect(screen.getByLabelText(i18n.t("reconciliation.history"))).toHaveValue("paused");
// This traverses both reconciliation workspaces and the intervening account,
// health and task pages. Main CI measured 5.087s (PR153: 4.634s), so allow
// runner variation for this integration flow without changing other budgets.
}, 10_000);


describe("global desktop navigation history", () => {
  async function ready() {
    listAccounts.mockResolvedValue([checkingAccount]);
    accountValuations.mockResolvedValue([checkingValuation]);
    const user = userEvent.setup();
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    const select = async (name: string) => { await user.click(within(nav).getByRole("button", { name })); await vi.dynamicImportSettled(); };
    const controls = () => within(screen.getByRole("group", { name: "Navigation history" }));
    return { user, select, controls };
  }

  it("traverses A → B → C → Back → Forward and truncates the forward branch on Back → D", async () => {
    const { user, select, controls } = await ready();
    expect(controls().getByRole("button", { name: /^Go back/ })).toBeDisabled();
    expect(controls().getByRole("button", { name: /^Go forward/ })).toBeDisabled();
    expect(controls().getByRole("button", { name: "Recent pages" })).toBeDisabled();
    await select("Accounts");
    await screen.findByTestId("accounts-table");
    await select("Settings");
    await screen.findByRole("form", { name: i18n.t("settings.formLabel") });
    await user.click(controls().getByRole("button", { name: /^Go back/ }));
    expect(screen.getByTestId("accounts-table")).toBeVisible();
    await user.click(controls().getByRole("button", { name: /^Go forward/ }));
    expect(await screen.findByRole("form", { name: i18n.t("settings.formLabel") })).toBeVisible();
    await user.click(controls().getByRole("button", { name: /^Go back/ }));
    await select(i18n.t("nav.portfolio"));
    await screen.findByRole("tab", { name: i18n.t("investmentAssets.holdings") });
    expect(controls().getByRole("button", { name: /^Go forward/ })).toBeDisabled();
  });

  it("uses Recent as new navigation, deduplicates destinations and returns focus on Escape", async () => {
    const { user, select, controls } = await ready();
    await select("Accounts");
    await screen.findByTestId("accounts-table");
    await select("Settings");
    await screen.findByRole("form", { name: i18n.t("settings.formLabel") });
    const trigger = controls().getByRole("button", { name: "Recent pages" });
    trigger.focus();
    await user.keyboard("{ArrowDown}");
    expect(await screen.findAllByRole("menuitem", { name: "Accounts" })).toHaveLength(1);
    await user.keyboard("{Escape}");
    expect(trigger).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    await user.click(await screen.findByRole("menuitem", { name: "Accounts" }));
    expect(screen.getByTestId("accounts-table")).toBeVisible();
    await user.click(controls().getByRole("button", { name: /^Go back/ }));
    expect(await screen.findByRole("form", { name: i18n.t("settings.formLabel") })).toBeVisible();
  });

  it("blocks Back, Forward and Recent without moving their history while confirmation is unresolved", async () => {
    const { user, select, controls } = await ready();
    await select("Accounts");
    await screen.findByTestId("accounts-table");
    await select("Settings");
    await screen.findByRole("form", { name: i18n.t("settings.formLabel") });
    await user.click(controls().getByRole("button", { name: /^Go back/ }));
    const owner = {};
    registerSheetConfirmation(owner);
    try {
      await user.click(controls().getByRole("button", { name: /^Go back/ }));
      await user.click(controls().getByRole("button", { name: /^Go forward/ }));
      await user.click(controls().getByRole("button", { name: "Recent pages" }));
      await user.click(await screen.findByRole("menuitem", { name: "Settings" }));
      expect(screen.getByTestId("accounts-table")).toBeVisible();
    } finally { resolveSheetConfirmation(owner); }
    await user.click(controls().getByRole("button", { name: /^Go forward/ }));
    expect(await screen.findByRole("form", { name: i18n.t("settings.formLabel") })).toBeVisible();
  });
});

it("keeps historical captures fixed through Back and Forward and gives new visits independent sessions", async () => {
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  const historical = within(nav).getByRole("button", { name: i18n.t("nav.historicalOverview") });
  await user.click(historical);
  await screen.findByRole("button", { name: i18n.t("historicalOverview.date") });
  await waitFor(() => expect(historicalRead).toHaveBeenCalledTimes(1));
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  expect(historicalRead).toHaveBeenCalledTimes(1);
  await user.click(controls.getByRole("button", { name: /^Go forward/ }));
  await user.click(historical);
  await waitFor(() => expect(historicalRead).toHaveBeenCalledTimes(2));
  expect(screen.getAllByRole("heading", { name: i18n.t("historicalOverview.title") })).toHaveLength(1);
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  expect(historicalRead).toHaveBeenCalledTimes(2);
});

it("revisits the exact account and handles deletion without selecting a different account", async () => {
  listAccounts.mockResolvedValue([checkingAccount]);
  accountValuations.mockResolvedValue([checkingValuation]);
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  await user.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  await screen.findByTestId("account-detail");
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  expect(screen.getByTestId("accounts-table")).toBeVisible();
  await user.click(controls.getByRole("button", { name: /^Go forward/ }));
  expect(screen.getByRole("heading", { name: "Checking" })).toBeVisible();
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  listAccounts.mockResolvedValue([]);
  await act(async () => { await queryClient.invalidateQueries({ queryKey: ["accounts"] }); });
  await user.click(controls.getByRole("button", { name: /^Go forward/ }));
  expect(await screen.findByText(i18n.t("connections.objectUnavailable"))).toBeVisible();
  expect(screen.queryByTestId("account-detail")).not.toBeInTheDocument();
});

it("clears all navigation when the household identity changes", async () => {
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  const controls = () => within(screen.getByRole("group", { name: "Navigation history" }));
  expect(controls().getByRole("button", { name: /^Go back/ })).toBeEnabled();
  act(() => {
    queryClient.setQueryData(queryKeys.household.bootstrap, { household: { id: "synthetic-new-household", name: "Synthetic replacement", baseCurrency: "USD" }, members: [], institutions: [], groups: [] });
  });
  await waitFor(() => expect(controls().getByRole("button", { name: /^Go back/ })).toBeDisabled());
  expect(controls().getByRole("button", { name: /^Go forward/ })).toBeDisabled();
  expect(controls().getByRole("button", { name: "Recent pages" })).toBeDisabled();
});


it("executes repeated explicit create intents once each and never replays them on Forward", async () => {
  listInstruments.mockResolvedValue([{ id: "synthetic-fund", householdId: "synthetic", name: "Synthetic Fund", type: "fund", quoteCurrency: "USD", quoteSource: "manual", iconKey: "", sortOrder: 0 }]);
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await user.click(within(nav).getByRole("button", { name: i18n.t("nav.marketData") }));
  await user.click(await screen.findByRole("button", { name: "Open Synthetic Fund details" }));
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  for (let attempt = 0; attempt < 2; attempt++) {
    await user.click(await screen.findByRole("button", { name: "Create synthetic account" }));
    expect(await screen.findByRole("dialog")).toBeVisible();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.click(controls.getByRole("button", { name: /^Go back/ }));
  }
  await user.click(controls.getByRole("button", { name: /^Go forward/ }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(within(nav).getByRole("button", { name: "Accounts" })).toHaveAttribute("aria-current", "page");
});

it("keeps both same-account reconciliation drafts reachable after forward truncation and MRU replacement", async () => {
  listAccounts.mockResolvedValue([checkingAccount]);
  accountValuations.mockResolvedValue([checkingValuation]);
  const counts = { pending: 0, confirmed: 0, skipped: 0, unresolved: 0 };
  reconciliationList.mockResolvedValue([{ id: "synthetic-shared-session", status: "in_progress", accountIds: ["acc-1"], accountNames: ["Checking"], counts }]);
  reconciliationGet.mockResolvedValue({ id: "synthetic-shared-session", householdId: "synthetic", status: "in_progress", revision: 1, startedAt: "2026-10-20T10:00:00Z", note: "Synthetic opening", closingNote: "", partial: false, accounts: [], counts });
  const user = userEvent.setup();
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  await user.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  for (const draft of ["Synthetic draft A", "Synthetic draft B"]) {
    await user.click(await screen.findByRole("button", { name: "Open account reconciliation" }));
    await user.click(await screen.findByRole("button", { name: "Continue session" }));
    await user.click(await screen.findByRole("button", { name: "Finish session" }));
    expect(screen.getByLabelText("Closing explanation")).toHaveValue("");
    await user.type(screen.getByLabelText("Closing explanation"), draft);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.getByRole("button", { name: "Finish session" })).toHaveFocus());
    await user.click(controls.getByRole("button", { name: /^Go back/ }));
  }
  await user.click(within(nav).getByRole("button", { name: "Overview" }));
  for (const [workspace, draft] of [[1, "Synthetic draft A"], [2, "Synthetic draft B"]] as const) {
    const trigger = controls.getByRole("button", { name: "Recent pages" });
    trigger.focus();
    await user.keyboard("{ArrowDown}");
    const item = await screen.findByRole("menuitem", { name: `Tasks & checks · Checking · Reconciliation · Workspace ${workspace}` });
    if (workspace === 1) {
      const owner = {};
      registerSheetConfirmation(owner);
      try {
        await user.click(item);
        expect(within(nav).getByRole("button", { name: "Overview" })).toHaveAttribute("aria-current", "page");
      } finally { resolveSheetConfirmation(owner); }
      trigger.focus();
      await user.keyboard("{ArrowDown}");
    }
    await user.click(await screen.findByRole("menuitem", { name: `Tasks & checks · Checking · Reconciliation · Workspace ${workspace}` }));
    await user.click(await screen.findByRole("button", { name: "Finish session" }));
    expect(screen.getByLabelText("Closing explanation")).toHaveValue(draft);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.getByRole("button", { name: "Finish session" })).toHaveFocus());
    await user.click(within(nav).getByRole("button", { name: "Overview" }));
  }
}, 10_000);

it("forks a Recent historical visit without changing the original date or frozen capture", async () => {
  historicalRead.mockImplementation((date: string) => Promise.resolve({ ...defaultHistorical, capturedAt: `2026-11-01T12:00:0${historicalRead.mock.calls.length}Z`, left: { ...defaultHistorical.left, date } }));
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  await user.click(within(nav).getByRole("button", { name: i18n.t("nav.historicalOverview") }));
  const dateA = (await screen.findByRole("button", { name: "Historical date" })).textContent!;
  await waitFor(() => expect(historicalRead).toHaveBeenCalledTimes(1));
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  controls.getByRole("button", { name: "Recent pages" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("menuitem", { name: `Wealth history · ${dateA}` }));
  expect(historicalRead).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole("button", { name: i18n.t("historicalOverview.previous") }));
  await waitFor(() => expect(historicalRead).toHaveBeenCalledTimes(2));
  const dateB = screen.getByRole("button", { name: "Historical date" }).textContent!;
  expect(dateB).not.toBe(dateA);
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  controls.getByRole("button", { name: "Recent pages" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("menuitem", { name: `Wealth history · ${dateA}` }));
  expect(screen.getByRole("button", { name: "Historical date" })).toHaveTextContent(dateA);
  expect(historicalRead).toHaveBeenCalledTimes(2);
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  expect(screen.getByRole("button", { name: "Historical date" })).toHaveTextContent(dateB);
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  expect(screen.getByRole("button", { name: "Historical date" })).toHaveTextContent(dateA);
  expect(historicalRead).toHaveBeenCalledTimes(2);
}, 10_000);


it("lands on Accounts after first onboarding without losing the completion callback to a ledger remount", async () => {
  onboardingFixture.enabled = true;
  render(<AppProviders><App /></AppProviders>);
  const user = userEvent.setup();
  const form = await screen.findByRole("form", { name: "Onboarding" });
  await user.type(within(form).getByLabelText(/household name/i), "Synthetic onboarding household");
  await user.type(within(form).getByLabelText("Member 1 name"), "Synthetic member");
  await user.selectOptions(within(form).getByLabelText(/base currency/i), "USD");
  await user.click(within(form).getByRole("button", { name: /get started/i }));
  const nav = await screen.findByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await waitFor(() => expect(within(nav).getByRole("button", { name: "Accounts" })).toHaveAttribute("aria-current", "page"));
  expect(screen.getByRole("button", { name: /^Go back/ })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Recent pages" })).toBeDisabled();
});

it("refreshes a Recent historical fork without changing the source visit's frozen result", async () => {
  historicalRead.mockImplementation((date: string) => Promise.resolve({ ...defaultHistorical, capturedAt: `2026-11-01T12:00:0${historicalRead.mock.calls.length}Z`, left: { ...defaultHistorical.left, date } }));
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  await user.click(within(nav).getByRole("button", { name: i18n.t("nav.historicalOverview") }));
  await screen.findAllByTitle("2026-11-01T12:00:01Z");
  const date = screen.getByRole("button", { name: "Historical date" }).textContent!;
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  controls.getByRole("button", { name: "Recent pages" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("menuitem", { name: `Wealth history · ${date}` }));
  await user.click(screen.getByRole("button", { name: "Refresh date" }));
  await screen.findAllByTitle("2026-11-01T12:00:02Z");
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  expect(screen.getAllByTitle("2026-11-01T12:00:01Z").some(node => !node.closest("[hidden]"))).toBe(true);
  expect(historicalRead).toHaveBeenCalledTimes(2);
  await user.click(controls.getByRole("button", { name: /^Go forward/ }));
  await user.click(controls.getByRole("button", { name: /^Go forward/ }));
  expect(screen.getAllByTitle("2026-11-01T12:00:02Z").some(node => !node.closest("[hidden]"))).toBe(true);
  expect(historicalRead).toHaveBeenCalledTimes(2);
}, 10_000);

it("replaces an edited visit's obsolete Recent date after Back instead of advertising a released capture", async () => {
  historicalRead.mockImplementation((date: string) => Promise.resolve({ ...defaultHistorical, left: { ...defaultHistorical.left, date } }));
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  await user.click(within(nav).getByRole("button", { name: i18n.t("nav.historicalOverview") }));
  const dateA = (await screen.findByRole("button", { name: "Historical date" })).textContent!;
  await waitFor(() => expect(historicalRead).toHaveBeenCalledTimes(1));
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  await user.click(controls.getByRole("button", { name: /^Go back/ }));
  await user.click(screen.getByRole("button", { name: i18n.t("historicalOverview.previous") }));
  await waitFor(() => expect(historicalRead).toHaveBeenCalledTimes(2));
  const dateB = screen.getByRole("button", { name: "Historical date" }).textContent!;
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  controls.getByRole("button", { name: "Recent pages" }).focus();
  await user.keyboard("{ArrowDown}");
  expect(screen.queryByRole("menuitem", { name: `Wealth history · ${dateA}` })).not.toBeInTheDocument();
  await user.click(await screen.findByRole("menuitem", { name: `Wealth history · ${dateB}` }));
  expect(screen.getByRole("button", { name: "Historical date" })).toHaveTextContent(dateB);
  expect(historicalRead).toHaveBeenCalledTimes(2);
}, 10_000);


it("restores source scroll when the external analysis store forces an intermediate navigation commit", async () => {
  listAccounts.mockResolvedValue([checkingAccount]);
  accountValuations.mockResolvedValue([checkingValuation]);
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  await user.click(await screen.findByRole("checkbox", { name: "Show archived" }));
  await user.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  await screen.findByTestId("account-detail");
  const main = document.getElementById("main-content")!;
  main.scrollTop = 170;
  await user.click(within(screen.getByRole("banner")).getByRole("button", { name: "Return analysis" }));
  const original = useAnalysisStore.setState;
  const setState = vi.spyOn(useAnalysisStore, "setState").mockImplementation(state => {
    flushSync(() => original(state));
  });
  try {
    await user.click(screen.getByRole("button", { name: /^Go back to Accounts(?: ·|$)/ }));
    await waitFor(() => expect(main.scrollTop).toBe(170));
    expect(screen.getByTestId("account-detail")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Back to accounts" }));
    expect(screen.getByRole("checkbox", { name: "Show archived" })).toBeChecked();
  } finally { setState.mockRestore(); }
});

it.each(["back", "recent"] as const)("restores same-page %s scroll only after the destination account DOM is committed", async route => {
  const long = { ...checkingAccount, account: { ...checkingAccount.account, id: "synthetic-long", name: "Long synthetic account" } };
  listAccounts.mockResolvedValue([checkingAccount, long]);
  accountValuations.mockResolvedValue([checkingValuation, { ...checkingValuation, account: long.account }]);
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  const controls = within(screen.getByRole("group", { name: "Navigation history" }));
  await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  await screen.findByTestId("accounts-table");
  const main = document.getElementById("main-content")!;
  let top = 0;
  const attempts: Array<{ requested: number; height: number }> = [];
  // jsdom has no layout. Model native scroll clamping from the currently
  // committed list/account DOM; the real browser fixture measures actual CSS heights.
  const height = () => main.querySelector('[data-testid="accounts-table"]') ? 2400
    : document.querySelector("header h1")?.textContent?.includes("Long synthetic account") ? 3000 : 500;
  Object.defineProperties(main, {
    clientHeight: { configurable: true, get: () => 600 },
    scrollHeight: { configurable: true, get: height },
    scrollTop: { configurable: true, get: () => top, set: (value: number) => {
      attempts.push({ requested: value, height: height() });
      top = Math.max(0, Math.min(value, Math.max(0, height() - 600)));
    } },
  });
  if (route === "recent") {
    await user.click(screen.getByRole("button", { name: /Long synthetic account.*Bank account/ }));
    await screen.findByRole("heading", { name: "Long synthetic account" });
    main.scrollTop = 900;
    await user.click(within(nav).getByRole("button", { name: "Accounts" }));
  } else main.scrollTop = 900;
  await user.click(screen.getByRole("button", { name: /Checking.*Bank account/ }));
  await screen.findByTestId("account-detail");
  expect(height()).toBe(500);
  expect(main.scrollTop).toBe(0);
  const original = useAnalysisStore.setState;
  const setState = vi.spyOn(useAnalysisStore, "setState").mockImplementation(state => {
    flushSync(() => original(state));
  });
  try {
    if (route === "back") await user.click(controls.getByRole("button", { name: /^Go back/ }));
    else {
      await user.click(controls.getByRole("button", { name: "Recent pages" }));
      await user.click(await screen.findByRole("menuitem", { name: "Accounts · Long synthetic account" }));
    }
    await waitFor(() => expect(main.scrollTop).toBe(900));
    expect(attempts.filter(attempt => attempt.requested === 900).every(attempt => attempt.height > 600)).toBe(true);
    if (route === "back") expect(screen.getByTestId("accounts-table")).toBeVisible();
    else expect(screen.getByRole("heading", { name: "Long synthetic account" })).toBeVisible();
  } finally { setState.mockRestore(); }
});


it.each(["en", "zh-CN", "zh-TW"])("provides one global history return and a distinct account-list breadcrumb in %s", async language => {
  settingsLoad.mockResolvedValue({ ...defaultSettings, language });
  listAccounts.mockResolvedValue([checkingAccount]);
  accountValuations.mockResolvedValue([checkingValuation]);
  portfolio.mockResolvedValue({ ...emptyPortfolio, accounts: [checkingValuation] });
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await user.click(within(nav).getByRole("button", { name: i18n.t("nav.portfolio") }));
  await user.click(await screen.findByRole("tab", { name: i18n.t("investmentAssets.allocation") }));
  await user.click(await screen.findByRole("button", { name: "Checking" }));
  await screen.findByTestId("account-detail");
  const controls = within(screen.getByRole("group", { name: i18n.t("navigationHistory.label") }));
  const backName = i18n.t("navigationHistory.backTo", { destination: `${i18n.t("nav.portfolio")} · ${i18n.t("investmentAssets.allocation")}` });
  expect(controls.getByRole("button", { name: backName })).toHaveAttribute("title", backName);
  expect(within(document.getElementById("main-content")!).queryByRole("button", { name: backName })).not.toBeInTheDocument();
  const breadcrumb = screen.getByRole("button", { name: i18n.t("accounts.backToList") });
  expect(breadcrumb).toHaveTextContent(i18n.t("nav.accounts"));
  const owner = {};
  registerSheetConfirmation(owner);
  try {
    await user.click(breadcrumb);
    expect(screen.getByTestId("account-detail")).toBeVisible();
  } finally { resolveSheetConfirmation(owner); }
  await user.click(breadcrumb);
  expect(await screen.findByTestId("accounts-table")).toBeVisible();
  await user.click(controls.getByRole("button", { name: backName }));
  expect(await screen.findByRole("tab", { name: i18n.t("investmentAssets.allocation") })).toHaveAttribute("aria-selected", "true");
});

it.each(["settings", "return-analysis"] as const)("opens current overview from history entered via %s and restores the frozen visit on Back", async source => {
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const user = userEvent.setup();
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  await user.click(within(nav).getByRole("button", { name: source === "settings" ? "Settings" : "Return analysis" }));
  await vi.dynamicImportSettled();
  await user.click(within(nav).getByRole("button", { name: "Wealth history" }));
  const exit = await screen.findByRole("button", { name: "View current overview" });
  await waitFor(() => expect(historicalRead).toHaveBeenCalledTimes(1));
  const reads = historicalRead.mock.calls.length;
  const owner = {};
  registerSheetConfirmation(owner);
  try {
    await user.click(exit);
    expect(screen.getByRole("button", { name: "View current overview" })).toBeVisible();
  } finally { resolveSheetConfirmation(owner); }
  await user.click(exit);
  expect(await screen.findByTestId("overview-net-worth")).toBeVisible();
  await user.click(screen.getByRole("button", { name: /^Go back to Wealth history/ }));
  expect(await screen.findByRole("button", { name: "View current overview" })).toBeVisible();
  expect(historicalRead).toHaveBeenCalledTimes(reads);
});
