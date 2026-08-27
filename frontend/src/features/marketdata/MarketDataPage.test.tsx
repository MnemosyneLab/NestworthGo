import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { MarketDataPage } from "./MarketDataPage";

const refreshAll = vi.fn();
const listInstruments = vi.fn();
const currentInstrumentQuote = vi.fn();
const currentFXQuote = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: { RefreshAll: () => refreshAll(), RefreshRequiredFX: vi.fn() },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => listInstruments() },
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    CurrentInstrumentQuote: (id: string) => currentInstrumentQuote(id),
    CurrentFXQuote: (a: string, b: string) => currentFXQuote(a, b),
  },
}));

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <MarketDataPage />
    </QueryClientProvider>,
  );
}

describe("MarketDataPage", () => {
  it("triggers RefreshAll and renders the per-target results", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "Global Equity Fund" }]);
    currentInstrumentQuote.mockResolvedValue({
      id: "q1",
      instrumentId: "i1",
      unitPrice: "12.5",
      currency: "USD",
      sourceKind: "manual",
      sourceKey: "",
      quotedAt: "2024-01-01T00:00:00Z",
      createdAt: "2024-01-01T00:00:00Z",
      delayed: false,
    });
    currentFXQuote.mockResolvedValue(null);
    refreshAll.mockResolvedValue({
      items: [
        { targetKey: "instrument:i1", kind: "instrument", status: "fetched" },
        { targetKey: "fx:SGD/USD", kind: "fx", status: "skipped", errorCode: "unavailable" },
      ],
      rateLimited: false,
    });
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /market data/i }));
    const results = await screen.findByTestId("refresh-results");
    expect(results).toHaveTextContent("Global Equity Fund");
    expect(results).toHaveTextContent("Updated");
    expect(results).toHaveTextContent("Latest: $12.50");
    expect(results).toHaveTextContent("SGD/USD");
    expect(results).toHaveTextContent("No saved value yet");
    expect(results).toHaveTextContent("No provider is configured for this target.");
  });

  it("shows an empty state when there are no refresh targets", async () => {
    listInstruments.mockResolvedValue([]);
    refreshAll.mockResolvedValue({ items: [], rateLimited: false });
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /market data/i }));
    expect(await screen.findByText("No saved market data needs refreshing.")).toBeInTheDocument();
  });
});
