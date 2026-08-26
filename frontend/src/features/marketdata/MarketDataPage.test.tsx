import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MarketDataPage } from "./MarketDataPage";

const refreshAll = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: { RefreshAll: () => refreshAll(), RefreshRequiredFX: vi.fn() },
}));

function renderPage() {
  const queryClient = new QueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <MarketDataPage />
    </QueryClientProvider>,
  );
}

describe("MarketDataPage", () => {
  it("triggers RefreshAll and renders the per-target results", async () => {
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
    expect(results).toHaveTextContent("instrument:i1");
    expect(results).toHaveTextContent("fetched");
    expect(results).toHaveTextContent("fx:SGD/USD");
  });

  it("shows an empty state when there are no refresh targets", async () => {
    refreshAll.mockResolvedValue({ items: [], rateLimited: false });
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /market data/i }));
    expect(await screen.findByText("No refresh targets found.")).toBeInTheDocument();
  });
});
