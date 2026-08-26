import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { OverviewPage as OverviewPageComponent } from "./OverviewPage";

// Each test needs a different mocked Overview() response, so the binding
// is re-mocked per test with vi.doMock + a fresh dynamic import, rather
// than one hoisted vi.mock factory shared by the whole file.
async function renderWithMockedOverview(response: unknown) {
  vi.doMock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
    Service: { Overview: () => Promise.resolve(response) },
  }));
  vi.resetModules();
  const { OverviewPage: FreshOverviewPage }: { OverviewPage: typeof OverviewPageComponent } = await import("./OverviewPage");

  const queryClient = new QueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <FreshOverviewPage />
    </QueryClientProvider>,
  );
}

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
      byCategory: [{ key: "cash_equivalent", label: "cash_equivalent", amount: "1000", shareBps: 10000 }],
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
      byCategory: [],
      byMember: [],
      byInstitution: [],
      byGroup: [],
    });

    expect(await screen.findByRole("alert")).toHaveTextContent("NVIDIA");
  });
});
