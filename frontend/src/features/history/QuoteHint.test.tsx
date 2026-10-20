import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { QuoteHint } from "./QuoteHint";

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { Load: () => Promise.resolve({ timezone: "Pacific/Auckland" }) },
}));

function renderHint(quote: Parameters<typeof QuoteHint>[0]["quote"]) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <QuoteHint quote={quote} sourceKind="agent" onUpdate={vi.fn()} isUpdating={false} />
    </QueryClientProvider>,
  );
}

describe("QuoteHint", () => {
  it("shows a source date for Agent NAV without inventing a quote time", async () => {
    renderHint({
      id: "q1", instrumentId: "i1", unitPrice: "1.0197", currency: "USD", sourceKind: "agent", sourceKey: "agent",
      observationKind: "close", priceBasis: "agent_unit_nav_v1", effectiveDate: "2026-09-28", timestampBasis: "date_label",
      quotedAt: "2026-09-27T16:00:00Z", createdAt: "2026-09-29T12:00:00Z", delayed: false,
    });
    expect(await screen.findByText(/as of 2026-09-28/)).toBeInTheDocument();
    expect(screen.getByText(/Agent supplied/)).toBeInTheDocument();
    expect(screen.queryByText(/12:00 AM/)).not.toBeInTheDocument();
  });

  it("asks for an MCP quote when an Agent instrument has no value", async () => {
    renderHint(null);
    expect(await screen.findByText("Ask your Agent to supply this quote through MCP.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update" })).not.toBeInTheDocument();
  });
});
