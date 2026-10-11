import { upcomingData, upcomingItem } from "@/test/upcomingActions";
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({ Service: { InstrumentHoldings: () => Promise.resolve([]), HistoricalOverview: () => Promise.resolve({ capturedAt: "2026-10-03T12:00:00Z", left: { date: "2026-09-30", currency: "USD", complete: true, assets: "0", liabilities: "0", netWorth: "0", byClass: [], byCurrency: [] }, right: null, rows: [] }) } }));
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import { AppProviders, queryClient } from "./app/providers";
import i18n, { SUPPORTED_LANGUAGES } from "./i18n";
import { useUiStore } from "./stores/ui";

const { startup, settingsLoad, settingsSave, listAccounts, accountValuations, portfolio, holdingsByAccounts, upcomingRead, scanHealth } = vi.hoisted(() => ({
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
    HistoryOrigin: () => Promise.resolve({ id: "origin-1", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" }),
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
  await userEvent.click(await screen.findByRole("button", { name: "View all 1 actions" }));
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

it("does not reopen A when Back restores its consumed request after opening B", async () => {
  upcomingRead.mockResolvedValue(upcomingData([
    upcomingItem({ id: "action-a", productId: "product-a", displayName: "Product A", primaryTarget: { kind: "product", productId: "product-a", sourceRef: null, reservationId: null } }),
    upcomingItem({ id: "action-b", productId: "product-b", displayName: "Product B", primaryTarget: { kind: "product", productId: "product-b", sourceRef: null, reservationId: null } }),
  ]));
  render(<AppProviders><App /></AppProviders>);
  await userEvent.click(await screen.findByRole("button", { name: "Verify receipt · Product A" }));
  await screen.findByRole("dialog", { name: "product-a" });
  await userEvent.click(screen.getByRole("button", { name: "Close product" }));
  await userEvent.click(screen.getByRole("button", { name: "View all 2 actions" }));
  await userEvent.click(await screen.findByRole("button", { name: "Verify receipt · Product B" }));
  await screen.findByRole("dialog", { name: "product-b" });
  // A genuinely new same-page request must still reopen B, without a trail entry.
  await userEvent.click(screen.getByRole("button", { name: "Close product" }));
  await userEvent.click(screen.getByRole("button", { name: "Verify receipt · Product B" }));
  await screen.findByRole("dialog", { name: "product-b" });
  await userEvent.click(screen.getByRole("button", { name: "← Back to Upcoming actions" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "← Back to Available funds" }));
  await waitFor(() => expect(screen.getByTestId("available-funds-page")).toBeVisible());
  // Flush the fresh overview request that caused the original erroneous reopening.
  await waitFor(() => expect(queryClient.isFetching()).toBe(0));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "← Back to Overview" })).toBeVisible();
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
