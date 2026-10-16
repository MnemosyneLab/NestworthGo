import { upcomingData, upcomingItem } from "@/test/upcomingActions";
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({ Service: { InstrumentHoldings: () => Promise.resolve([]), InstrumentHoldingGroups: () => Promise.resolve([]), HistoricalOverview: (...args: unknown[]) => historicalRead(...args), HistoricalOverviewSeries: (...args: unknown[]) => seriesRead(...args) } }));
/* default historical fixture */
const defaultHistorical = { capturedAt: "2026-10-03T12:00:00Z", left: { date: "2026-09-30", currency: "USD", complete: true, assets: "0", liabilities: "0", netWorth: "0", byClass: [], byCurrency: [] }, right: null, rows: [] };
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import { AppProviders, queryClient } from "./app/providers";
import i18n, { SUPPORTED_LANGUAGES } from "./i18n";
import { queryKeys } from "./queries/keys";
import { useUiStore } from "./stores/ui";
import { registerSheetConfirmation, resolveSheetConfirmation } from "./components/ui/sheet-confirmation";

const { startup, settingsLoad, settingsSave, listAccounts, accountValuations, portfolio, holdingsByAccounts, upcomingRead, scanHealth, historicalRead, historyOrigin, seriesRead, ordinaryRecord } = vi.hoisted(() => ({
  seriesRead: vi.fn(),
  ordinaryRecord: vi.fn(),
  historyOrigin: vi.fn(),
  historicalRead: vi.fn(),
  upcomingRead: vi.fn(),
  scanHealth: vi.fn(),
  startup: vi.fn().mockResolvedValue({ available: true }),
  settingsLoad: vi.fn(),
  settingsSave: vi.fn(),
  listAccounts: vi.fn(),
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
    Bootstrap: () =>
      Promise.resolve({
        household: { id: "h1", name: "Test", baseCurrency: "USD", createdAt: "", updatedAt: "" },
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
  Service: { ListInstruments: () => Promise.resolve([]) },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    InstrumentQuoteSeries: () => Promise.resolve({ range: "30d", points: [], observations: [] }),
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

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
    UpcomingActions: () => upcomingRead(),
    InvestmentReviews: () => Promise.resolve({ items: [], policy: { enabled: true, windowDays: 30, lowAnnualizedPercent: "1", lossPercent: "-1", sustainDays: 7, maxAgeDays: 7 }, asOf: "2026-10-15T12:00:00Z", timezone: "UTC" }),
    ListReviewAlertsV2: () => Promise.resolve({
      items: [], nextCursor: "", summary: { totalGroups: 0, matchedGroups: 0, pendingGroups: 0, acknowledgedGroups: 0, ignoredGroups: 0, unknownGroups: 0, pausedGroups: 0, actionableRules: 0 },
      reportRevision: "synthetic-review-v2", projectionStatus: "complete", publishedOn: "2026-10-15",
    }),
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
  listAccounts.mockResolvedValue([]);
  accountValuations.mockReset();
  accountValuations.mockResolvedValue([]);
  ordinaryRecord.mockReset();
  portfolio.mockReset();
  portfolio.mockResolvedValue({ ...emptyPortfolio });
  holdingsByAccounts.mockReset();
  holdingsByAccounts.mockResolvedValue({});
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

  it.each(SUPPORTED_LANGUAGES)("renders the app shell in %s without runtime errors", async (language) => {
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
    expect(within(nav).getByRole("button", { name: i18n.t("nav.upcomingActions") })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: i18n.t("nav.portfolio") })).toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: i18n.t("nav.holdings") })).not.toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: i18n.t("nav.dataHealth") })).toBeInTheDocument();

    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.dataHealth") }));
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

  it("switches between portfolio overview and holdings within one navigation entry", async () => {
    await i18n.changeLanguage("en");
    render(<AppProviders><App /></AppProviders>);
    await screen.findByTestId("overview-net-worth");
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.portfolio") }));
    expect(await screen.findByRole("tab", { name: "Overview" })).toHaveAttribute("aria-selected", "true");
    await userEvent.click(screen.getByRole("tab", { name: "Holdings" }));
    expect(await screen.findByText(i18n.t("portfolio.noHoldingsAccount"))).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(i18n.t("portfolio.pageTitle"));
    await userEvent.click(screen.getByRole("tab", { name: "Overview" }));
    expect(await screen.findByTestId("portfolio-page")).toBeInTheDocument();
    expect(screen.getByText(i18n.t("portfolio.noHoldingsAccount"))).not.toBeVisible();
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
  await userEvent.click(screen.getByRole("button", { name: "Return analysis" }));
  expect(await screen.findByRole("button", { name: "← Back to Accounts" })).toBeVisible();
  expect(screen.getByTestId("account-detail")).not.toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "← Back to Accounts" }));
  await waitFor(() => expect(main.scrollTop).toBe(170));
  expect(screen.getByTestId("account-detail")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Back to accounts" }));
  expect(screen.getByRole("checkbox", { name: "Show archived" })).toBeChecked();
});


it("opens historical overview from Overview, removes current queries and exits back", async () => {
  await i18n.changeLanguage("en");
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
  // The Overview entry keeps a navigation trail, but must unmount its reads.
  const entry = screen.getAllByRole("button", { name: i18n.t("nav.historicalOverview") }).find(button => !nav.contains(button));
  expect(entry).toBeDefined();
  await userEvent.click(entry!);
  await screen.findByLabelText(i18n.t("historicalOverview.date"));
  expect(screen.queryByTestId("overview-net-worth")).not.toBeInTheDocument();
  await waitFor(() => expect(queryClient.getQueryCache().find({ queryKey: ["overview"] })?.getObserversCount() ?? 0).toBe(0));
  await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.exit") }));
  expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
  await waitFor(() => expect(document.getElementById("open-historical-overview")).toHaveFocus());
  // The independent navigation entry also opens and exits without a trail.
  await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.historicalOverview") }));
  await screen.findByLabelText(i18n.t("historicalOverview.date"));
  await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.exit") }));
  expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
  await waitFor(() => expect(document.getElementById("open-historical-overview")).toHaveFocus());
});


it("retains the upcoming filter and restores its trigger after Back", async () => {
  render(<AppProviders><App /></AppProviders>);
  await userEvent.click(await screen.findByRole("button", { name: "View all 1 action" }));
  const filter = await screen.findByRole("button", { name: "Reservation issues" });
  await userEvent.click(filter);
  await userEvent.click(screen.getByText("Information affecting assessment (0 data issues, 0 notes)"));
  const health = within(screen.getByText("Information affecting assessment (0 data issues, 0 notes)").closest("details")!).getByRole("button", { name: "Data Health" });
  await userEvent.click(health);
  await userEvent.click(await screen.findByRole("button", { name: "← Back to Upcoming actions" }));
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
  await userEvent.click(screen.getByRole("button", { name: "← Back to Overview" }));
  await userEvent.click(await screen.findByRole("button", { name: "View all 2 actions" }));
  await userEvent.click(await screen.findByRole("button", { name: "View terms · Product B" }));
  await screen.findByRole("dialog", { name: "product-b" });
  await userEvent.click(screen.getByRole("button", { name: "Close product" }));
  await userEvent.click(screen.getByRole("button", { name: "← Back to Upcoming actions" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "← Back to Overview" }));
  await screen.findByTestId("overview-net-worth");
  // Flush retained-page reads before checking that no detail sheet reopens.
  await waitFor(() => expect(queryClient.isFetching()).toBe(0));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "← Back to Overview" })).not.toBeInTheDocument();
});


it("preserves same-page Data Health gap focus history", async () => {
  scanHealth.mockResolvedValue({ healthy: false, issueCount: 2, executableCount: 0, prerequisiteCount: 0, snapshotDays: 2, issues: [
    { id: "gap-a", kind: "snapshot_incomplete", severity: "warning", targetKey: "cash:a", accountId: "bank-a", label: "USD", rangeStart: "2026-09-18", rangeEnd: "2026-09-18", action: "none", executable: false },
    { id: "gap-b", kind: "snapshot_incomplete", severity: "warning", targetKey: "cash:b", accountId: "bank-b", label: "CNY", rangeStart: "2026-09-19", rangeEnd: "2026-09-19", action: "none", executable: false },
  ] });
  render(<AppProviders><App /></AppProviders>);
  await screen.findByTestId("overview-net-worth");
  await userEvent.click(within(screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") })).getByRole("button", { name: "Data Health" }));
  const rows = await screen.findAllByTestId("health-issue-snapshot_incomplete");
  expect(rows).toHaveLength(2);
  await userEvent.click(within(rows[0]).getByRole("button", { name: "View gap" }));
  await waitFor(() => expect(screen.getAllByTestId("health-issue-snapshot_incomplete")).toHaveLength(1));
  await userEvent.click(await screen.findByRole("button", { name: "← Back to Data Health" }));
  await waitFor(() => expect(screen.getAllByTestId("health-issue-snapshot_incomplete")).toHaveLength(2));
});


vi.mock("@/features/history/HistoryPage", () => ({ HistoryPage: ({ navigationFilters }: { navigationFilters?: unknown }) => <section aria-label="Synthetic transaction history"><pre>{JSON.stringify(navigationFilters)}</pre></section> }));

afterEach(() => vi.useRealTimers());

it.each([
  ["UTC", "2026-10-31T23:59:30Z", "2026-11-01T00:00:30Z", false],
  ["UTC", "2026-10-31T23:59:30Z", "2026-11-01T00:00:30Z", true],
  ["Asia/Shanghai", "2026-10-31T15:59:30Z", "2026-10-31T16:00:30Z", false],
  ["Asia/Shanghai", "2026-10-31T15:59:30Z", "2026-10-31T16:00:30Z", true],
])("returns from transaction history across a household month boundary with the same capture, dates, detail and position (%s)", async (timezone, before, after, trendEnabled) => {
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
  await userEvent.click(within(nav).getByRole("button", { name: "Historical overview" }));
  await screen.findByLabelText("Historical date");
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
  const sourceButton = within(dialog).getByRole("button", { name: "Leave panorama · View transaction history" });
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
  await userEvent.click(screen.getByRole("button", { name: "← Back to Historical overview" }));
  await waitFor(() => expect(screen.getByRole("dialog")).toHaveTextContent("Synthetic retained bank"));
  expect(within(screen.getByRole("group", { name: "View mode", hidden: true })).getByRole("button", { name: trendEnabled ? "Compare dates" : "Date vs. now", hidden: true })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByLabelText(i18n.t("historicalOverview.baselineDate"))).toHaveTextContent("2026-09-30");
  expect(screen.getAllByTitle(capturedAt).length).toBeGreaterThan(0);
  expect(document.querySelector('[role="tab"][aria-selected="true"]')).toHaveTextContent("Account details");
  expect(document.querySelector('[role="tab"][aria-selected="true"]')).toHaveAttribute("aria-selected", "true");
  await waitFor(() => expect(main.scrollTop).toBe(210));
  await waitFor(() => expect(screen.getByRole("button", { name: "Leave panorama · View transaction history" })).toHaveFocus());
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
});


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
    expect(screen.queryByRole("button", { name: "← Back to Overview" })).not.toBeInTheDocument();
  } finally { resolveSheetConfirmation(owner); }
  await userEvent.click(within(nav).getByRole("button", { name: "Accounts" }));
  await userEvent.click(await screen.findByRole("button", { name: /Checking.*Bank account/ }));
  await screen.findByTestId("account-detail");
  await userEvent.click(screen.getByRole("button", { name: "Return analysis" }));
  const back = await screen.findByRole("button", { name: "← Back to Accounts" });
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
  const analysis = screen.getByRole("button", { name: "Return analysis" });
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

it.each(SUPPORTED_LANGUAGES)("keeps investment review reachable from Overview with no upcoming items in %s", async language => {
  await i18n.changeLanguage(language);
  settingsLoad.mockResolvedValue({ ...defaultSettings, language });
  upcomingRead.mockResolvedValue(upcomingData());
  render(<AppProviders><App /></AppProviders>);
  const entry = await screen.findByRole("button", { name: i18n.t("reviewV2.openReview") });
  entry.focus();
  await userEvent.keyboard("{Enter}");
  expect(await screen.findByRole("heading", { name: i18n.t("upcomingActions.title"), level: 2 })).toBeVisible();
  expect(screen.getByText(i18n.t("upcomingActions.description"))).toBeVisible();
  expect(await screen.findByText(i18n.t("upcomingActions.empty"))).toBeVisible();
  const header = within(document.querySelector("header")!);
  expect(header.getByRole("button", { name: i18n.t("upcomingActions.refresh") })).toBeVisible();
  expect(header.queryByRole("button", { name: i18n.t("reviewV2.historyTitle") })).not.toBeInTheDocument();
  expect(header.queryByRole("button", { name: i18n.t("reviewV2.ignoredList") })).not.toBeInTheDocument();
  expect(header.queryByRole("button", { name: i18n.t("reviewV2.rules") })).not.toBeInTheDocument();
  const reviewTab = screen.getByRole("tab", { name: i18n.t("investmentReview.title") });
  reviewTab.focus(); await userEvent.keyboard("{Enter}");
  expect(await screen.findByRole("textbox", { name: i18n.t("reviewV2.search") })).toBeVisible();
  expect(screen.getByRole("button", { name: i18n.t("reviewV2.rules") })).toBeVisible();
  expect(reviewTab).toHaveAttribute("aria-selected", "true");
});
