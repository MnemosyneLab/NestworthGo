import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AvailableFundsPage } from "./AvailableFundsPage";
import type { LiquidityOverviewDTO, LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const overview = vi.fn();
const listProducts = vi.fn();
const listAccounts = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
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
  Service: { Load: () => Promise.resolve({ timezone: "Asia/Singapore" }) },
}));

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

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <AvailableFundsPage />
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
    expect(await screen.findByTestId("horizon-available-2026-09-20")).toHaveTextContent("Unknown");
    expect(screen.getByTestId("horizon-available-2026-09-20")).not.toHaveTextContent("$0.00");
    expect(screen.getByText(/Partial: \$10,000.00/)).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText("Currency display"), "native");
    expect(screen.getByTestId("horizon-available-2026-09-20")).toHaveTextContent("€100.00");
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
    expect(await screen.findByText("Due, receipt unconfirmed")).toBeInTheDocument();
    const row = screen.getByText("Due deposit").closest("tr");
    expect(row).toBeTruthy();
    expect(within(row as HTMLElement).getAllByText("Unknown").length).toBeGreaterThan(0);
    expect(screen.getByText("Due deposit is due. Receipt is unconfirmed.")).toBeInTheDocument();
  });

  it("retries after a load failure", async () => {
    overview.mockRejectedValueOnce(new Error("unavailable"));
    overview.mockResolvedValue(fixtureOverview({ sources: [] }));
    renderPage();
    expect(await screen.findByText("Could not load available funds.")).toBeInTheDocument();
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
    fireEvent.change(screen.getByLabelText("Custom date"), { target: { value: "2026-11-01" } });
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
