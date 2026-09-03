import { describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AnalyticsPage } from "./AnalyticsPage";

const realizedGain = vi.fn();
const dividendIncome = vi.fn();
const netWorthTrend = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({
  Service: {
    RealizedGain: (...args: unknown[]) => realizedGain(...args),
    DividendIncome: (...args: unknown[]) => dividendIncome(...args),
    AccountGain: vi.fn(),
    AccountGains: vi.fn(),
    HoldingGain: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: { NetWorthTrend: (...args: unknown[]) => netWorthTrend(...args), Overview: vi.fn(), Portfolio: vi.fn() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});
const rebuildHistoricalSnapshots = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => Promise.resolve({
      id: "origin-1",
      timezone: "UTC",
      startedAt: "2026-06-01T00:00:00.000Z",
    }),
    RebuildHistoricalSnapshots: (...args: unknown[]) => rebuildHistoricalSnapshots(...args),
  },
}));

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
    dividendIncome.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });

    renderPage();
    expect(await screen.findAllByText("NVIDIA")).not.toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: "By account" }));
    expect(screen.getAllByText("Brokerage").length).toBeGreaterThan(0);
    expect(realizedGain).toHaveBeenCalledWith({}, "30d");
  });

  it("lists realized gain groups largest-first with losses last", async () => {
    realizedGain.mockResolvedValue({
      from: "2026-01-01",
      to: "2026-01-31",
      currency: "USD",
      available: true,
      byInstrument: [
        { key: "i-loss", label: "Loss Co", gain: { amount: "-40", currency: "USD" } },
        { key: "i-small", label: "Small Co", gain: { amount: "10", currency: "USD" } },
        { key: "i-large", label: "Large Co", gain: { amount: "500", currency: "USD" } },
      ],
      byAccount: [],
    });
    dividendIncome.mockResolvedValue({
      from: "2026-01-01",
      to: "2026-01-31",
      currency: "USD",
      available: true,
      byInstrument: [
        { key: "d-small", label: "QQQ", gain: { amount: "10", currency: "USD" } },
        { key: "d-large", label: "Apple", gain: { amount: "20", currency: "USD" } },
      ],
      byAccount: [],
    });
    netWorthTrend.mockResolvedValue({ range: "30d", currency: "USD", points: [] });

    renderPage();
    const [gainLegend, incomeLegend] = await screen.findAllByTestId("signed-bar-legend");
    expect([...gainLegend.querySelectorAll("li")].map((item) => item.querySelector("span")?.textContent)).toEqual([
      "Large Co",
      "Small Co",
      "Loss Co",
    ]);
    expect([...incomeLegend.querySelectorAll("li")].map((item) => item.querySelector("span")?.textContent)).toEqual([
      "Apple",
      "QQQ",
    ]);
  });

  it("switches range and re-fetches realized gain and dividend income", async () => {
    realizedGain.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    dividendIncome.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    netWorthTrend.mockResolvedValue({ range: "1y", currency: "USD", points: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "1 year" }));
    expect(realizedGain).toHaveBeenCalledWith({}, "1y");
    expect(dividendIncome).toHaveBeenCalledWith({}, "1y");
  });

  it("renders trend range buttons from the catalog only", async () => {
    realizedGain.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    dividendIncome.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    netWorthTrend.mockResolvedValue({ range: "30d", currency: "USD", points: [] });
    renderPage();
    const group = await screen.findByRole("group", { name: "Range" });
    await waitFor(() => {
      expect(within(group).getAllByRole("button").map((button) => button.textContent)).toEqual(["30 days", "ytd", "1 year", "All history"]);
    });
    expect(within(group).getByRole("button", { name: "All history" })).toBeInTheDocument();
  });

  it("renders dividend income separately from realized gain and marks missing FX", async () => {
    realizedGain.mockResolvedValue({
      from: "2026-01-01",
      to: "2026-01-31",
      currency: "USD",
      available: true,
      byInstrument: [{ key: "i1", label: "NVIDIA", gain: { amount: "500", currency: "USD" }, available: true }],
      byAccount: [],
    });
    dividendIncome.mockResolvedValue({
      from: "2026-01-01",
      to: "2026-01-31",
      currency: "CNY",
      available: false,
      byInstrument: [{ key: "i2", label: "QQQ", gain: { amount: "0", currency: "CNY" }, available: false }],
      byAccount: [{ key: "a1", label: "Brokerage", gain: { amount: "0", currency: "CNY" }, available: false }],
    });
    netWorthTrend.mockResolvedValue({ range: "30d", currency: "USD", points: [] });

    renderPage();
    expect(await screen.findByText("Dividend income (Incomplete)")).toBeInTheDocument();
    expect(screen.getAllByText("QQQ").length).toBeGreaterThan(0);
    expect(screen.getAllByText("NVIDIA").length).toBeGreaterThan(0);
    expect(dividendIncome).toHaveBeenCalledWith({}, "30d");
  });

  it("does not rebuild the entire snapshot history when origin is older than 31 days", async () => {
    rebuildHistoricalSnapshots.mockReset();
    realizedGain.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    dividendIncome.mockResolvedValue({ from: "", to: "", currency: "USD", available: true, byInstrument: [], byAccount: [] });
    netWorthTrend.mockResolvedValue({ range: "all", currency: "USD", points: [] });

    renderPage();
    expect(await screen.findByText("Wealth trend")).toBeInTheDocument();
    expect(rebuildHistoricalSnapshots).not.toHaveBeenCalled();
  });
});
