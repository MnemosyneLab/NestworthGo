import { describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AnalyticsPage } from "./AnalyticsPage";

const realizedGain = vi.fn();
const netWorthTrend = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({
  Service: { RealizedGain: (...args: unknown[]) => realizedGain(...args), AccountGain: vi.fn(), HoldingGain: vi.fn() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: { NetWorthTrend: (...args: unknown[]) => netWorthTrend(...args), Overview: vi.fn(), Portfolio: vi.fn() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <AnalyticsPage />
    </QueryClientProvider>,
  );
}

describe("AnalyticsPage", () => {
  it("renders realized gain grouped by instrument and account", async () => {
    realizedGain.mockResolvedValue({
      from: "2026-01-01",
      to: "2026-01-31",
      currency: "USD",
      available: true,
      byInstrument: [{ key: "i1", label: "NVIDIA", gain: { amount: "500", currency: "USD" } }],
      byAccount: [{ key: "a1", label: "Brokerage", gain: { amount: "500", currency: "USD" } }],
    });
    netWorthTrend.mockResolvedValue({ range: "30d", currency: "USD", points: [] });

    renderPage();
    expect(await screen.findByText("NVIDIA")).toBeInTheDocument();
    expect(screen.getByText("Brokerage")).toBeInTheDocument();
    expect(realizedGain).toHaveBeenCalledWith({}, "30d");
  });

  it("switches range and re-fetches realized gain", async () => {
    realizedGain.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    netWorthTrend.mockResolvedValue({ range: "1y", currency: "USD", points: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "1 year" }));
    expect(realizedGain).toHaveBeenCalledWith({}, "1y");
  });

  it("renders trend range buttons from the catalog only", async () => {
    realizedGain.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    netWorthTrend.mockResolvedValue({ range: "30d", currency: "USD", points: [] });
    renderPage();
    const group = await screen.findByRole("group", { name: "Range" });
    await waitFor(() => {
      expect(within(group).getAllByRole("button").map((button) => button.textContent)).toEqual(["30 days", "1 year"]);
    });
    expect(within(group).queryByRole("button", { name: "All history" })).not.toBeInTheDocument();
  });
});
