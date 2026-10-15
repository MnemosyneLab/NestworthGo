import { NavigationContext, type ObjectNavigation } from "@/app/NavigationContext";
import i18n from "@/i18n";
import { upcomingData } from "@/test/upcomingActions";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AvailableFundsPage } from "./AvailableFundsPage";
import type { LiquidityOverviewDTO, LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const investmentReviews = vi.fn();
const upcomingActions = vi.fn(() => Promise.resolve(upcomingData()));
const overview = vi.fn();
const listProducts = vi.fn();
const listAccounts = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
    InvestmentReviews: () => investmentReviews(),
    UpcomingActions: () => upcomingActions(),
    Overview: (...args: unknown[]) => overview(...args),
    ListProducts: (...args: unknown[]) => listProducts(...args),
    Product: vi.fn(),
    ListOperations: vi.fn(),
    SavePolicy: vi.fn(),
    ResetPolicy: vi.fn(),
    SaveReservation: vi.fn(),
    ReleaseReservation: vi.fn(),
    PreviewProductOperation: vi.fn(),
    RecordProductOperation: vi.fn(),
    AppendProductValuation: vi.fn(),
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: () => Promise.resolve([]),
  },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { Load: () => Promise.resolve({ timezone: "Asia/Singapore" }), SupportedCurrencies: () => Promise.resolve(["USD", "EUR"]) },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: () => Promise.resolve({ timezone: "Asia/Singapore" }) } }));

function money(amount: string, currency = "USD") {
  return { amount, currency };
}

function source(partial: Partial<LiquiditySourceDTO> & Pick<LiquiditySourceDTO, "sourceKey" | "displayName" | "accountId">): LiquiditySourceDTO {
  return {
    sourceRef: { kind: "account_cash", accountId: partial.accountId },
    nativeCurrency: "USD",
    currentNativeValue: money("10000"),
    valueAsOf: "2026-09-20",
    priceEvidence: null,
    fxEvidence: null,
    policyOrigin: "explicit",
    policy: null,
    contractState: null,
    displayState: "redeemable",
    reservationRequested: money("0"),
    normalRoute: { kind: "normal", eligibleOn: "2026-09-20", receiptOn: "2026-09-20", grossNative: money("10000"), feeNative: money("0"), netNative: money("10000"), status: "complete", amountBasis: "current_value", actionRequired: "none", assumptions: [], missingReasons: [] },
    earlyRoute: null,
    bucketResults: [],
    reasons: [],
    assumptions: [],
    excluded: false,
    dueUnconfirmed: false,
    productId: null,
    ...partial,
  };
}

function fixtureOverview(overrides: Partial<LiquidityOverviewDTO> = {}): LiquidityOverviewDTO {
  const cash = source({
    sourceKey: "cash",
    displayName: "Bank cash",
    accountId: "acc-1",
    bucketResults: [
      { horizonOn: "2026-09-20", selectedRoute: { kind: "normal", eligibleOn: "2026-09-20", receiptOn: "2026-09-20", grossNative: money("10000"), feeNative: money("0"), netNative: money("10000"), status: "complete", amountBasis: "current_value", actionRequired: "none", assumptions: [], missingReasons: [] }, netNative: money("10000"), netBase: money("10000"), appliedReserveNative: money("2000"), unreservedNative: money("8000"), reserveShortfallNative: null, status: "complete", reasons: [] },
      { horizonOn: "2026-09-27", selectedRoute: { kind: "normal", eligibleOn: "2026-09-20", receiptOn: "2026-09-20", grossNative: money("10000"), feeNative: money("0"), netNative: money("10000"), status: "complete", amountBasis: "current_value", actionRequired: "none", assumptions: [], missingReasons: [] }, netNative: money("10000"), netBase: money("10000"), appliedReserveNative: money("2000"), unreservedNative: money("8000"), reserveShortfallNative: null, status: "complete", reasons: [] },
      { horizonOn: "2026-10-20", selectedRoute: { kind: "normal", eligibleOn: "2026-09-20", receiptOn: "2026-09-20", grossNative: money("10000"), feeNative: money("0"), netNative: money("10000"), status: "complete", amountBasis: "current_value", actionRequired: "none", assumptions: [], missingReasons: [] }, netNative: money("10000"), netBase: money("10000"), appliedReserveNative: money("2000"), unreservedNative: money("8000"), reserveShortfallNative: null, status: "complete", reasons: [] },
    ],
  });
  return {
    asOf: "2026-09-20T04:00:00Z",
    localDate: "2026-09-20",
    timezone: "Asia/Singapore",
    baseCurrency: "USD",
    assumptions: ["assumed_zero_exit_fee"],
    buckets: [
      { horizonOn: "2026-09-20", status: "complete", fullAvailable: money("10000"), knownAvailableSubtotal: money("10000"), appliedReserveSubtotal: money("2000"), fullUnreserved: money("8000"), knownUnreservedSubtotal: money("8000"), unknownSourceCount: 0, excludedSourceCount: 1, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
      { horizonOn: "2026-09-27", status: "complete", fullAvailable: money("33200"), knownAvailableSubtotal: money("33200"), appliedReserveSubtotal: money("7000"), fullUnreserved: money("26200"), knownUnreservedSubtotal: money("26200"), unknownSourceCount: 0, excludedSourceCount: 1, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
      { horizonOn: "2026-10-20", status: "complete", fullAvailable: money("38190"), knownAvailableSubtotal: money("38190"), appliedReserveSubtotal: money("7000"), fullUnreserved: money("31190"), knownUnreservedSubtotal: money("31190"), unknownSourceCount: 0, excludedSourceCount: 1, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
    ],
    sources: [cash],
    unresolvedReservations: [],
    ...overrides,
  };
}

function renderPage(navigation: ObjectNavigation | null = null) {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <NavigationContext.Provider value={navigation}><AvailableFundsPage /></NavigationContext.Provider>
    </QueryClientProvider>,
  );
}

describe("AvailableFundsPage", () => {
  beforeEach(() => {
    overview.mockReset();
    listProducts.mockReset();
    listAccounts.mockReset();
    listProducts.mockResolvedValue([]);
    listAccounts.mockResolvedValue([]);
  });

  it("requires a choice between eligible accounts and names the chosen account", async () => {
    overview.mockResolvedValue(fixtureOverview());
    listAccounts.mockResolvedValue(["Bank A", "Bank B"].map((name, index) => ({ account: {
      id: `acc-${index + 1}`, name, defaultCurrency: "USD", trackingMode: "holdings", balanceSheetRole: "asset", accountType: "bank", archivedAt: null,
    } })));
    renderPage();
    await userEvent.click(await screen.findByRole("button", {name: "Add product"}));
    expect(await screen.findByRole("heading", {name: "Choose an account"})).toBeVisible();
    expect(screen.queryByLabelText("Principal")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", {name: "Bank B"}));
    expect(await screen.findByText("Account: Bank B")).toBeVisible();
    expect(screen.getByLabelText("Principal")).toBeVisible();
  });

  it("renders the empty state without inventing zero totals", async () => {
    overview.mockResolvedValue(fixtureOverview({ sources: [], buckets: [
      { horizonOn: "2026-09-20", status: "unavailable", fullAvailable: null, knownAvailableSubtotal: null, appliedReserveSubtotal: null, fullUnreserved: null, knownUnreservedSubtotal: null, unknownSourceCount: 0, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
    ] }));
    renderPage();
    expect(await screen.findByTestId("available-funds-page")).toBeInTheDocument();
    expect(screen.getByText("No assets to evaluate")).toBeInTheDocument();
    expect(screen.queryByTestId("horizon-available-2026-09-20")).not.toBeInTheDocument();
  });

  it("shows backend totals and does not substitute unknown amounts with zero", async () => {
    overview.mockResolvedValue(fixtureOverview({
      buckets: [
        { horizonOn: "2026-09-20", status: "partial", fullAvailable: null, knownAvailableSubtotal: money("10000"), appliedReserveSubtotal: money("2000"), fullUnreserved: null, knownUnreservedSubtotal: money("8000"), unknownSourceCount: 1, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [{ currency: "EUR", status: "complete", fullAvailable: money("100", "EUR"), knownAvailableSubtotal: money("100", "EUR"), appliedReserveSubtotal: money("0", "EUR"), fullUnreserved: money("100", "EUR"), knownUnreservedSubtotal: money("100", "EUR") }], warnings: [] },
        { horizonOn: "2026-09-27", status: "partial", fullAvailable: null, knownAvailableSubtotal: money("33200"), appliedReserveSubtotal: money("7000"), fullUnreserved: null, knownUnreservedSubtotal: money("26200"), unknownSourceCount: 1, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
        { horizonOn: "2026-10-20", status: "partial", fullAvailable: null, knownAvailableSubtotal: money("38190"), appliedReserveSubtotal: money("7000"), fullUnreserved: null, knownUnreservedSubtotal: money("31190"), unknownSourceCount: 1, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] },
      ],
    }));
    renderPage();
    expect(await screen.findByTestId("horizon-available-2026-09-20")).toHaveTextContent("$8,000.00");
    expect(screen.getByTestId("horizon-available-2026-09-20")).not.toHaveTextContent("$0.00");
    expect(within(screen.getByTestId("funds-comparison")).getAllByText("Partial")).toHaveLength(3);
    expect(screen.queryByLabelText("Summary currency")).not.toBeInTheDocument();
    expect(screen.getAllByText("€100.00").length).toBeGreaterThan(0);
  });

  it("shows base totals and native details while preserving missing currencies as unknown", async () => {
    const data = fixtureOverview();
    data.buckets!.forEach((bucket, index) => {
      bucket.nativeCurrencyGroups = ["USD", "SGD"].filter(currency => index !== 1 || currency !== "SGD").map(currency => ({
        currency, status: "complete", fullAvailable: money(String(100 + index), currency),
        knownAvailableSubtotal: money(String(100 + index), currency), appliedReserveSubtotal: money("0", currency),
        fullUnreserved: money(String(100 + index), currency), knownUnreservedSubtotal: money(String(100 + index), currency),
      }));
    });
    overview.mockResolvedValue(data);
    renderPage();
    await screen.findByTestId("funds-comparison");
    expect(screen.queryByLabelText("Summary currency")).not.toBeInTheDocument();
    const breakdown = screen.getByRole("table", { name: "Selected date breakdown" });
    expect(within(breakdown).getByRole("rowheader", { name: "SGD" })).toBeInTheDocument();
    expect(screen.getByTestId("horizon-available-2026-09-20")).toHaveTextContent("$8,000.00");
    await userEvent.click(screen.getByTestId("horizon-2026-09-27"));
    expect(screen.getByTestId("horizon-2026-09-27")).toHaveAttribute("aria-pressed", "true");

    expect(screen.getByTestId("horizon-2026-09-20")).toHaveAttribute("aria-pressed", "false");
    const sgdRow = within(breakdown).getByRole("rowheader", { name: "SGD" }).closest("tr")!;
    expect(sgdRow).toHaveTextContent("Unknown");
    expect(sgdRow).not.toHaveTextContent("0.00");
  });

  it("keeps secondary row actions in a menu", async () => {
    overview.mockResolvedValue(fixtureOverview());
    renderPage();
    await screen.findByTestId("available-funds-page");
    expect(screen.queryByRole("button", { name: "Edit source rules" })).not.toBeInTheDocument();
    const row = screen.getAllByText("Bank cash")[0].closest("tr")!;
    await userEvent.click(within(row).getByRole("button", { name: /Actions for/ }));
    expect(await screen.findByRole("menuitem", { name: "Manage reservations" })).toBeVisible();
    await userEvent.click(screen.getByRole("menuitem", { name: "Edit source rules" }));
    expect(await screen.findByRole("dialog")).toBeVisible();
    expect(screen.queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("keeps due-unconfirmed sources out of spendable unreserved amounts", async () => {
    const due = source({
      sourceKey: "due-1",
      displayName: "Due deposit",
      accountId: "acc-1",
      productId: "prod-1",
      displayState: "due_unconfirmed",
      dueUnconfirmed: true,
      currentNativeValue: money("20000"),
      bucketResults: [
        { horizonOn: "2026-09-20", selectedRoute: null, netNative: null, netBase: null, appliedReserveNative: null, unreservedNative: null, reserveShortfallNative: null, status: "complete", reasons: ["due_unconfirmed"] },
      ],
    });
    overview.mockResolvedValue(fixtureOverview({
      sources: [due],
      buckets: [{ horizonOn: "2026-09-20", status: "complete", fullAvailable: money("0"), knownAvailableSubtotal: money("0"), appliedReserveSubtotal: money("0"), fullUnreserved: money("0"), knownUnreservedSubtotal: money("0"), unknownSourceCount: 0, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] }],
    }));
    renderPage();
    expect((await screen.findAllByText("Due, receipt unconfirmed")).length).toBeGreaterThan(0);
    const row = screen.getAllByText("Due deposit")[0].closest("tr");
    expect(row).toBeTruthy();
    expect(within(row!).getAllByText("Due, receipt unconfirmed").length).toBeGreaterThan(0);
    expect(within(row!).queryByText("$0.00")).not.toBeInTheDocument();
    expect(screen.queryByText("Due deposit is due. Receipt is unconfirmed.")).not.toBeInTheDocument();
    expect(screen.queryByTestId("overview-upcoming-actions-card")).not.toBeInTheDocument();
  });

  it("retries after a load failure", async () => {
    overview.mockRejectedValue(new Error("unavailable"));
    const queryClient = createTestQueryClient({ retry: false });
    render(
      <QueryClientProvider client={queryClient}>
        <AvailableFundsPage />
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load available funds.");
    overview.mockResolvedValue(fixtureOverview({ sources: [] }));
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("No assets to evaluate")).toBeInTheDocument();
  });

  it("sends early-withdrawal and custom horizon parameters", async () => {
    overview.mockResolvedValue(fixtureOverview());
    renderPage();
    await screen.findByTestId("available-funds-page");
    expect(overview).toHaveBeenCalledWith({ customHorizonOn: undefined, includeEarlyWithdrawal: false });
    await userEvent.click(screen.getByLabelText("Consider early withdrawal"));
    expect(overview).toHaveBeenCalledWith({ customHorizonOn: undefined, includeEarlyWithdrawal: true });
    await userEvent.click(screen.getByLabelText("Custom date"));
    await userEvent.selectOptions(screen.getByRole("combobox", { name: /year/i }), "2026");
    await userEvent.selectOptions(screen.getByRole("combobox", { name: /month/i }), "10");
    await userEvent.click(screen.getByRole("grid").querySelector('[data-day="2026-11-01"] button') as HTMLElement);
    expect(overview.mock.calls.some((call) => call[0].customHorizonOn === "2026-11-01" && call[0].includeEarlyWithdrawal === true)).toBe(true);
  });

  it("selects a horizon card with the keyboard", async () => {
    overview.mockResolvedValue(fixtureOverview());
    renderPage();
    const today = await screen.findByTestId("horizon-2026-09-20");
    today.focus();
    expect(today).toHaveAttribute("aria-pressed", "true");
    await userEvent.keyboard("{Tab}");
    await userEvent.keyboard("{Enter}");
    expect(screen.getByTestId("horizon-2026-09-27")).toHaveAttribute("aria-pressed", "true");
  });
});

it("shows requested, applied and shortfall reservations separately", async () => {
  const data = fixtureOverview();
  const cash = data.sources![0];
  cash.reservationRequested = money("12000");
  cash.bucketResults![0].appliedReserveNative = money("10000");
  cash.bucketResults![0].reserveShortfallNative = money("2000");
  overview.mockResolvedValue(data);
  renderPage();
  const row = (await screen.findAllByText("Bank cash")).map((node) => node.closest("tr")).find(Boolean)!;
  expect(within(row).getByText("Requested: $12,000.00")).toBeInTheDocument();
  await userEvent.click(within(row).getByText("Requested: $12,000.00"));
  expect(within(row).getByText("Applied: $10,000.00")).toBeInTheDocument();
  expect(within(row).getByText("Shortfall: $2,000.00")).toBeInTheDocument();
});

it("explains reservations that do not apply by the selected horizon", async () => {
  const data = fixtureOverview();
  data.sources![0].reservationRequested = money("2000");
  data.sources![0].bucketResults![0].selectedRoute = null;
  data.sources![0].bucketResults![0].netNative = null;
  data.sources![0].bucketResults![0].appliedReserveNative = null;
  overview.mockResolvedValue(data);
  renderPage();
  await userEvent.click(await screen.findByText("Requested: $2,000.00"));
  expect(screen.getByText("Not applied by this date: $2,000.00")).toBeVisible();
});

it("shows a known future lock clearly and keeps the normal route when early access is absent", async () => {
  const data = fixtureOverview();
  const asset = data.sources![0];
  asset.displayState = "locked";
  asset.normalRoute!.receiptOn = "2026-09-27";
  asset.earlyRoute = null;
  asset.bucketResults![0] = { ...asset.bucketResults![0], selectedRoute: null, netNative: null, unreservedNative: null, appliedReserveNative: null };
  overview.mockResolvedValue(data);
  renderPage();
  const row = (await screen.findAllByText("Bank cash"))[0].closest("tr")!;
  expect(within(row).getByText("Not available by this date")).toBeVisible();
  expect(within(row).queryByText("Unknown")).not.toBeInTheDocument();
  expect(within(row).queryByText(/Requested:/)).not.toBeInTheDocument();
  await userEvent.click(screen.getByLabelText("Consider early withdrawal"));
  expect(within(row).getByText("2026-09-27")).toBeVisible();
  await userEvent.click(screen.getByTestId("horizon-2026-09-27"));
  expect(within(row).getByText("$8,000.00")).toBeVisible();
  expect(screen.queryByRole("button", { name: /^Locked/ })).not.toBeInTheDocument();
});


describe("Available Funds scope", () => {
  it.each(["en", "zh-CN", "zh-TW"])("omits upcoming actions and investment review panels and entries in %s", async language => {
    await i18n.changeLanguage(language);
    investmentReviews.mockClear();
    upcomingActions.mockClear();
    overview.mockResolvedValue(fixtureOverview());
    listProducts.mockResolvedValue([]);
    listAccounts.mockResolvedValue([]);
    renderPage({ open: vi.fn(), openHealth: vi.fn() });
    await screen.findByTestId("available-funds-page");
    expect(screen.queryByTestId("overview-upcoming-actions-card")).not.toBeInTheDocument();
    expect(screen.queryByText(i18n.t("upcomingActions.title"))).not.toBeInTheDocument();
    expect(screen.queryByText(i18n.t("investmentReview.title"))).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: i18n.t("investmentReview.view") })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: i18n.t("upcomingActions.viewAll", { count: 0 }) })).not.toBeInTheDocument();
    expect(investmentReviews).not.toHaveBeenCalled();
    expect(upcomingActions).not.toHaveBeenCalled();
    expect(screen.getByLabelText(i18n.t("availableFunds.customDate"))).toBeVisible();
    expect(screen.getByTestId("horizon-2026-09-20")).toBeVisible();
    expect(screen.getAllByText("Bank cash")[0]).toBeVisible();
  });
});


describe("Available Funds visual hierarchy and interaction", () => {
  function earlyFixture() {
    const data = fixtureOverview();
    const route = (kind: string, receiptOn: string, net: string, fee: string) => ({ kind, eligibleOn: receiptOn, receiptOn, grossNative: money("20000"), feeNative: money(fee), netNative: money(net), status: "complete", amountBasis: "current_value", actionRequired: kind === "early" ? "early_withdrawal" : "withdraw", assumptions: [], missingReasons: [] });
    const deposit = source({ sourceKey: "cd", displayName: "Term deposit", accountId: "acc-1", displayState: "locked", productId: "prod-1", currentNativeValue: money("20000"), normalRoute: route("normal", "2026-12-20", "20000", "0"), earlyRoute: route("early", "2026-09-22", "19700", "300") });
    const result = (horizonOn: string, early: boolean) => early
      ? { horizonOn, selectedRoute: route("early", "2026-09-22", "19700", "300"), netNative: money("19700"), netBase: money("19700"), appliedReserveNative: money("0"), unreservedNative: money("19700"), reserveShortfallNative: null, status: "complete", reasons: [] }
      : { horizonOn, selectedRoute: null, netNative: null, netBase: null, appliedReserveNative: null, unreservedNative: null, reserveShortfallNative: null, status: "complete", reasons: [] };
    deposit.bucketResults = [result("2026-09-20", false), result("2026-09-27", true), result("2026-10-20", true)];
    data.sources = [...data.sources!, deposit];
    return data;
  }

  it("explains identical horizon totals with a same-as cue and marks growth", async () => {
    const data = fixtureOverview();
    data.buckets![2] = { ...data.buckets![1], horizonOn: "2026-10-20" };
    overview.mockResolvedValue(data);
    renderPage();
    await screen.findByTestId("funds-comparison");
    expect(within(screen.getByTestId("horizon-2026-09-20")).queryByText(/More than|Same as|Less than/)).not.toBeInTheDocument();
    expect(within(screen.getByTestId("horizon-2026-09-27")).getByText("More than Today")).toBeVisible();
    expect(within(screen.getByTestId("horizon-2026-10-20")).getByText("Same as Within 7 days")).toBeVisible();
  });

  it("shows the comparison cue only when both horizons have complete totals", async () => {
    const data = fixtureOverview();
    const today = data.buckets![0];
    data.buckets![1] = { ...today, horizonOn: "2026-09-27", fullUnreserved: null };
    data.buckets![2] = { ...today, horizonOn: "2026-10-20" };
    overview.mockResolvedValue(data);
    renderPage();
    await screen.findByTestId("funds-comparison");
    for (const date of ["2026-09-20", "2026-09-27", "2026-10-20"]) {
      expect(within(screen.getByTestId(`horizon-${date}`)).queryByText(/More than|Same as|Less than/)).not.toBeInTheDocument();
    }
    expect(within(screen.getByTestId("horizon-2026-09-27")).getByText("Partial")).toBeVisible();
  });

  it("keeps the comparison cue for complete totals next to a partial horizon", async () => {
    const data = fixtureOverview();
    data.buckets![2] = { ...data.buckets![2], fullUnreserved: null };
    overview.mockResolvedValue(data);
    renderPage();
    await screen.findByTestId("funds-comparison");
    expect(within(screen.getByTestId("horizon-2026-09-27")).getByText("More than Today")).toBeVisible();
    expect(within(screen.getByTestId("horizon-2026-10-20")).queryByText(/More than|Same as|Less than/)).not.toBeInTheDocument();
  });

  it("keeps the cumulative and assumption notes in one info popover instead of repeating them", async () => {
    overview.mockResolvedValue(fixtureOverview());
    renderPage();
    await screen.findByTestId("funds-comparison");
    expect(screen.queryByText(/Do not add the date totals together/)).not.toBeInTheDocument();
    expect(screen.queryByText("Assumptions and omitted costs")).not.toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "About these amounts" })[0]);
    expect(await screen.findByText(/Do not add the date totals together/)).toBeVisible();
    expect(screen.getByText(/Based on current assets/)).toBeVisible();
  });

  it("marks early withdrawal on affected cards and rows only after the toggle is on", async () => {
    overview.mockImplementation((request: { includeEarlyWithdrawal: boolean }) => Promise.resolve(request.includeEarlyWithdrawal ? earlyFixture() : fixtureOverview()));
    renderPage();
    await screen.findByTestId("funds-comparison");
    expect(screen.queryByText("Includes early withdrawal")).not.toBeInTheDocument();
    const toggle = screen.getByLabelText("Consider early withdrawal");
    await userEvent.click(toggle);
    expect(toggle).toBeChecked();
    await waitFor(() => expect(within(screen.getByTestId("horizon-2026-09-27")).getByText("Includes early withdrawal")).toBeVisible());
    expect(within(screen.getByTestId("horizon-2026-09-20")).queryByText("Includes early withdrawal")).not.toBeInTheDocument();
    const row = screen.getAllByText("Term deposit")[0].closest("tr")!;
    expect(within(row).getByText("Early withdrawal")).toBeVisible();
    expect(within(row).getByText("Cost: $300.00")).toBeVisible();
    await userEvent.click(toggle);
    await waitFor(() => expect(screen.queryByText("Includes early withdrawal")).not.toBeInTheDocument());
  });

  it("shows filter counts, hides empty filters, and recovers from an empty filter", async () => {
    const data = fixtureOverview();
    data.sources![0].bucketResults![0] = { ...data.sources![0].bucketResults![0], selectedRoute: null, netNative: null, unreservedNative: null, appliedReserveNative: null };
    overview.mockResolvedValue(data);
    renderPage();
    await screen.findByTestId("funds-comparison");
    const group = screen.getByRole("group", { name: "Assets" });
    expect(within(group).getByRole("button", { name: "All 1" })).toHaveAttribute("aria-pressed", "true");
    expect(within(group).getByRole("button", { name: "Locked 1" })).toBeVisible();
    expect(within(group).queryByRole("button", { name: /^Excluded/ })).not.toBeInTheDocument();
    expect(within(group).queryByRole("button", { name: /^Needs information/ })).not.toBeInTheDocument();
    await userEvent.click(within(group).getByRole("button", { name: "Locked 1" }));
    expect(within(group).getByRole("button", { name: "Locked 1" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getAllByText("Bank cash").length).toBeGreaterThan(0);
    await userEvent.click(within(group).getByRole("button", { name: "All 1" }));
    expect(within(group).getByRole("button", { name: "All 1" })).toHaveAttribute("aria-pressed", "true");
  });

  it("keeps an active filter visible after a horizon change empties it, with a way back", async () => {
    const data = fixtureOverview();
    data.sources![0].bucketResults![0] = { ...data.sources![0].bucketResults![0], selectedRoute: null, netNative: null, unreservedNative: null, appliedReserveNative: null };
    overview.mockResolvedValue(data);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Locked 1" }));
    await userEvent.click(screen.getByTestId("horizon-2026-09-27"));
    expect(screen.getByRole("button", { name: "Locked 0" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("No sources match this filter.")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Show all" }));
    expect(screen.getAllByText("Bank cash").length).toBeGreaterThan(0);
  });

  it("offers a shortcut from the needs-information notice to the affected sources", async () => {
    const data = fixtureOverview();
    const mmf = source({ sourceKey: "mmf", displayName: "Money fund", accountId: "acc-1", displayState: "needs_info", reasons: ["missing_policy"], bucketResults: [
      { horizonOn: "2026-09-20", selectedRoute: null, netNative: null, netBase: null, appliedReserveNative: null, unreservedNative: null, reserveShortfallNative: null, status: "partial", reasons: ["missing_policy"] },
    ] });
    data.sources = [...data.sources!, mmf];
    data.buckets![0] = { ...data.buckets![0], status: "partial", fullUnreserved: null };
    overview.mockResolvedValue(data);
    renderPage();
    expect(await screen.findByText("Availability needs more information")).toBeVisible();
    await userEvent.click(await screen.findByRole("button", { name: "Show items" }));
    expect(screen.getByRole("button", { name: /^Needs information/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getAllByText("Money fund").length).toBeGreaterThan(0);
    expect(screen.queryByText("Bank cash")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show items" })).not.toBeInTheDocument();
  });

  it.each(["zh-CN", "zh-TW"])("localizes the new card cues in %s", async (language) => {
    await i18n.changeLanguage(language);
    const data = fixtureOverview();
    data.buckets![2] = { ...data.buckets![2], fullUnreserved: null };
    data.buckets![1] = { ...data.buckets![1], fullUnreserved: data.buckets![0].fullUnreserved, knownUnreservedSubtotal: data.buckets![0].knownUnreservedSubtotal };
    overview.mockResolvedValue(data);
    renderPage();
    const cards = await screen.findByTestId("funds-comparison");
    expect(within(cards).getByText(i18n.t("availableFunds.partialShort"))).toBeVisible();
    expect(i18n.t("availableFunds.partialShort")).not.toBe("Partial");
    expect(within(cards).getByText(i18n.t("availableFunds.sameAs", { label: i18n.t("availableFunds.today") }))).toBeVisible();
    const previousLabel = screen.getByTestId("horizon-2026-09-27").querySelector("span.text-sm")!.textContent!;
    const partialCard = screen.getByTestId("horizon-2026-10-20");
    for (const key of ["sameAs", "moreThan", "lessThan"]) {
      expect(partialCard).not.toHaveTextContent(i18n.t(`availableFunds.${key}`, { label: previousLabel }));
    }
    await i18n.changeLanguage("en");
  });
});
