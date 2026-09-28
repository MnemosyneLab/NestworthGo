import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import type { OverviewPage as OverviewPageComponent } from "./OverviewPage";

const { scanHealth } = vi.hoisted(() => ({ scanHealth: vi.fn() }));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({ Service: {} }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { Load: () => Promise.resolve({ timezone: "UTC" }) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    ScanMarketDataHealth: () => scanHealth(),
    GetCurrentSyncJob: () => Promise.resolve({ jobId: "" }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
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
  },
}));
vi.mock("@wailsio/runtime", () => ({
  Events: { On: () => () => undefined },
}));

const defaultHeadlines = {
  historyStarted: true,
  recentActivities: [
    {
      id: "a1",
      kind: "cash_in",
      reason: "contribution",
      effectiveLocalDate: "2026-01-01",
      effects: [{ accountId: "acc-1", money: { amount: "1000", currency: "USD" } }],
    },
  ],
  accountLabels: [{ id: "acc-1", name: "Checking" }],
  instrumentLabels: [],
  holdingLabels: [],
};

// Each test needs a different mocked Overview() response, so the binding
// is re-mocked per test with vi.doMock + a fresh dynamic import, rather
// than one hoisted vi.mock factory shared by the whole file.
async function renderWithMockedOverview(response: unknown, props: { onOpenDataHealth?: () => void } = {}) {
  vi.doMock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
    Service: { Overview: () => Promise.resolve(response) },
  }));
  vi.resetModules();
  const { OverviewPage: FreshOverviewPage }: { OverviewPage: typeof OverviewPageComponent } = await import("./OverviewPage");

  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <FreshOverviewPage {...props} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  scanHealth.mockReset();
  scanHealth.mockResolvedValue({ healthy: true, issueCount: 0, issues: [] });
  vi.resetModules();
});

describe("OverviewPage", () => {
  // This fixture-driven check ensures Overview's displayed net worth, assets,
  // and liabilities exactly match the same fixture's Go-computed result. The mocked
  // response below is the exact JSON
  // internal/wailsapi/portfolio.TestOverviewMultiOwnerFixtureMatchesFrontendGolden
  // asserts Go computes for its multi-owner (60/40 split) fixture; if
  // either side changes, the other must change with it.
  it("matches the Go-computed multi-owner fixture", async () => {
    await renderWithMockedOverview({
      currency: "USD",
      accountCount: 2,
      complete: true,
      missingInputs: [],
      assets: "1000",
      liabilities: "300",
      netWorth: "700",
      assetsByType: [{ key: "cash", label: "cash", amount: "1000", shareBps: 10000 }],
      liabilitiesByType: [{ key: "credit_card", label: "credit_card", amount: "300", shareBps: 10000 }],
      byMember: [
        { key: "alice", label: "Alice", amount: "600", shareBps: 6000 },
        { key: "bob", label: "Bob", amount: "400", shareBps: 4000 },
      ],
      byInstitution: [],
      byGroup: [],
      byAccountType: [{ key: "bank_account", label: "bank_account", amount: "1000", shareBps: 10000 }],
      ...defaultHeadlines,
    });

    expect(await screen.findByTestId("overview-net-worth")).toHaveTextContent("$700.00");
    expect(screen.getByTestId("overview-assets")).toHaveTextContent("$1,000.00");
    expect(screen.getByTestId("overview-liabilities")).toHaveTextContent("$300.00");
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("60.0%")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("40.0%")).toBeInTheDocument();
    expect(screen.getByText("Current valuation complete")).toBeInTheDocument();
    expect(screen.getAllByText("Cash").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Credit card").length).toBeGreaterThan(0);
    expect(screen.getByText("By account type")).toBeInTheDocument();
    expect(screen.getByText("Bank account")).toBeInTheDocument();
    expect(screen.getByText(/groups each real-world account as a whole/i)).toBeInTheDocument();
    expect(screen.getByText(/Updated /)).toBeInTheDocument();
    expect(await screen.findByText("Added $1,000.00 to Checking (Contribution)")).toBeInTheDocument();
    expect(await screen.findByTestId("data-health-indicator")).toHaveTextContent("Data Health ✓");
  });

  it("lists institutions from largest to smallest", async () => {
    await renderWithMockedOverview({
      currency: "USD",
      accountCount: 2,
      complete: true,
      missingInputs: [],
      assets: "1500",
      liabilities: "0",
      netWorth: "1500",
      assetsByType: [{ key: "cash", label: "cash", amount: "1500", shareBps: 10000 }],
      liabilitiesByType: [],
      byMember: [],
      byInstitution: [
        { key: "broker", label: "Brokerage", amount: "1000", shareBps: 6667 },
        { key: "bank", label: "Bank", amount: "500", shareBps: 3333 },
      ],
      byGroup: [],
      byAccountType: [],
      ...defaultHeadlines,
    });

    const labels = (await screen.findAllByText(/Brokerage|Bank/)).map((node) => node.textContent);
    expect(labels.indexOf("Brokerage")).toBeGreaterThanOrEqual(0);
    expect(labels.indexOf("Brokerage")).toBeLessThan(labels.indexOf("Bank"));
  });

  it("shows an incomplete-data banner listing missing inputs, never a fabricated zero", async () => {
    const onOpenDataHealth = vi.fn();
    await renderWithMockedOverview(
      {
        currency: "USD",
        accountCount: 1,
        complete: false,
        missingInputs: [{ kind: "instrument_price", accountId: "acc-1", instrumentName: "NVIDIA" }],
        assets: "0",
        liabilities: "0",
        netWorth: "0",
        assetsByType: [],
        liabilitiesByType: [],
        byMember: [],
        byInstitution: [],
        byGroup: [],
        ...defaultHeadlines,
        recentActivities: [],
      },
      { onOpenDataHealth },
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("NVIDIA");
    expect(screen.getByText(/excludes values that still need a price/i)).toBeInTheDocument();
    expect(screen.getByText("1 instrument needs a manual price.")).toBeInTheDocument();
    expect(screen.queryByText("1 instrument prices need a refresh.")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Fix in Data Health" }));
    expect(onOpenDataHealth).toHaveBeenCalled();
  });

  it("asks to refresh provider-sourced prices on Market Data", async () => {
    await renderWithMockedOverview({
      currency: "USD",
      accountCount: 1,
      complete: false,
      missingInputs: [{ kind: "instrument_price", accountId: "acc-1", instrumentId: "i1", instrumentName: "NVIDIA", quoteSource: "provider" }],
      assets: "0",
      liabilities: "0",
      netWorth: "0",
      assetsByType: [],
      liabilitiesByType: [],
      byMember: [],
      byInstitution: [],
      byGroup: [],
      ...defaultHeadlines,
      instrumentLabels: [{ id: "i1", name: "NVIDIA", quoteSource: "provider" }],
      recentActivities: [],
    });

    expect(await screen.findByText("1 instrument price needs a refresh.")).toBeInTheDocument();
    expect(screen.queryByText("1 instrument needs a manual price.")).not.toBeInTheDocument();
  });

  it("uses archived account, instrument, and holding names in a recent position transfer", async () => {
    await renderWithMockedOverview({
      currency: "USD",
      accountCount: 2,
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
      historyStarted: true,
      accountLabels: [
        { id: "old-brokerage", name: "Archived Brokerage" },
        { id: "retirement", name: "Retirement" },
      ],
      instrumentLabels: [{ id: "i1", name: "Archived NVIDIA", quoteSource: "manual" }],
      holdingLabels: [
        { id: "h1", accountId: "old-brokerage", instrumentId: "i1", name: "Archived Brokerage · Archived NVIDIA" },
        { id: "h2", accountId: "retirement", instrumentId: "i1", name: "Retirement · Archived NVIDIA" },
      ],
      recentActivities: [
        {
          id: "transfer-1",
          kind: "position_transfer",
          reason: "other",
          effectiveLocalDate: "2026-01-01",
          effects: [
            { role: "transfer_from", direction: "removed", holdingId: "h1", instrumentId: "i1", quantity: "2" },
            { role: "transfer_to", direction: "added", holdingId: "h2", instrumentId: "i1", quantity: "2" },
          ],
        },
      ],
    });

    expect(await screen.findByText("Moved 2 from Archived Brokerage · Archived NVIDIA to Retirement · Archived NVIDIA")).toBeInTheDocument();
  });
});


const completeOverview = {
  currency: "USD", accountCount: 1, complete: true, missingInputs: [],
  assets: "1000", liabilities: "0", netWorth: "1000", assetsByType: [],
  liabilitiesByType: [], byMember: [], byInstitution: [], byGroup: [], byAccountType: [],
  ...defaultHeadlines,
};

it("keeps historical issues actionable when current valuation is complete", async () => {
  scanHealth.mockResolvedValue({ healthy: false, issueCount: 1, issues: [{ kind: "snapshot_outdated" }] });
  const onOpenDataHealth = vi.fn();
  await renderWithMockedOverview(completeOverview, { onOpenDataHealth });
  expect(await screen.findByText("Current valuation complete")).toBeInTheDocument();
  const action = await screen.findByRole("button", { name: "Fix in Data Health" });
  expect(screen.queryByText("Nothing needs attention right now.")).not.toBeInTheDocument();
  await userEvent.click(action);
  expect(onOpenDataHealth).toHaveBeenCalledOnce();
});

it.each(["loading", "failed"])("does not claim all clear while health is %s", async (state) => {
  if (state === "loading") scanHealth.mockImplementation(() => new Promise(() => {}));
  else scanHealth.mockRejectedValue(new Error("health unavailable"));
  await renderWithMockedOverview(completeOverview);
  await screen.findByText("Current valuation complete");
  const message = state === "loading" ? "Checking historical data health…" : "Historical data health could not be checked.";
  expect((await screen.findAllByText(message)).length).toBeGreaterThan(0);
  expect(screen.queryByText("Nothing needs attention right now.")).not.toBeInTheDocument();
});
