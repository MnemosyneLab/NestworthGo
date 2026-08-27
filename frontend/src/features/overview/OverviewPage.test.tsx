import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import type { OverviewPage as OverviewPageComponent } from "./OverviewPage";

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => Promise.resolve({ id: "origin-1", timezone: "UTC" }),
    ListActivities: () =>
      Promise.resolve([
        {
          id: "a1",
          kind: "cash_in",
          reason: "contribution",
          effectiveLocalDate: "2026-01-01",
          effects: [{ accountId: "acc-1", money: { amount: "1000", currency: "USD" } }],
        },
      ]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: () =>
      Promise.resolve([{ account: { id: "acc-1", name: "Checking" }, ownership: [], latestValue: null }]),
  },
}));
const listInstruments = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => listInstruments() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({ Service: {} }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({ Service: {} }));

// Each test needs a different mocked Overview() response, so the binding
// is re-mocked per test with vi.doMock + a fresh dynamic import, rather
// than one hoisted vi.mock factory shared by the whole file.
async function renderWithMockedOverview(response: unknown) {
  vi.doMock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
    Service: { Overview: () => Promise.resolve(response) },
  }));
  vi.resetModules();
  const { OverviewPage: FreshOverviewPage }: { OverviewPage: typeof OverviewPageComponent } = await import("./OverviewPage");

  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <FreshOverviewPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listInstruments.mockReset();
  listInstruments.mockResolvedValue([]);
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
    });

    expect(await screen.findByTestId("overview-net-worth")).toHaveTextContent("$700.00");
    expect(screen.getByTestId("overview-assets")).toHaveTextContent("$1,000.00");
    expect(screen.getByTestId("overview-liabilities")).toHaveTextContent("$300.00");
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("60.0%")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("40.0%")).toBeInTheDocument();
    expect(screen.getByText("Complete")).toBeInTheDocument();
    expect(screen.getByText("Cash")).toBeInTheDocument();
    expect(screen.getByText("Credit card")).toBeInTheDocument();
    expect(screen.getByText(/Updated /)).toBeInTheDocument();
    expect(await screen.findByText("Added $1,000.00 to Checking (Contribution)")).toBeInTheDocument();
  });

  it("shows an incomplete-data banner listing missing inputs, never a fabricated zero", async () => {
    await renderWithMockedOverview({
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
    });

    expect(await screen.findByRole("alert")).toHaveTextContent("NVIDIA");
    expect(screen.getByText(/excludes values that still need a price/i)).toBeInTheDocument();
    expect(screen.getByText("1 instrument needs a manual price.")).toBeInTheDocument();
    expect(screen.queryByText("1 instrument prices need a refresh.")).not.toBeInTheDocument();
  });

  it("asks to refresh provider-sourced prices on Market Data", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteSource: "provider" }]);
    await renderWithMockedOverview({
      currency: "USD",
      accountCount: 1,
      complete: false,
      missingInputs: [{ kind: "instrument_price", accountId: "acc-1", instrumentId: "i1", instrumentName: "NVIDIA" }],
      assets: "0",
      liabilities: "0",
      netWorth: "0",
      assetsByType: [],
      liabilitiesByType: [],
      byMember: [],
      byInstitution: [],
      byGroup: [],
    });

    expect(await screen.findByText("1 instrument price needs a refresh.")).toBeInTheDocument();
    expect(screen.queryByText("1 instrument needs a manual price.")).not.toBeInTheDocument();
  });
});
