import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { PortfolioPage } from "./PortfolioPage";

const portfolio = vi.fn();
const listAccounts = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: {
    Portfolio: () => portfolio(),
    PortfolioTrend: () => Promise.resolve({ range: "30d", currency: "USD", points: [] }),
    Overview: vi.fn(),
    NetWorthTrend: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: () => listAccounts() },
}));

function renderPage(onOpenAccount?: (id: string) => void) {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <PortfolioPage onOpenAccount={onOpenAccount} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  portfolio.mockReset();
  listAccounts.mockReset();
  listAccounts.mockResolvedValue([]);
});

describe("PortfolioPage", () => {
  it("shows only backend-included accounts and opens account detail", async () => {
    const onOpenAccount = vi.fn();
    portfolio.mockResolvedValue({
      currency: "USD",
      complete: true,
      valuedSubtotal: { amount: "320000", currency: "USD" },
      accounts: [
        {
          account: { id: "brk-1", name: "MooMoo SG Brokerage", accountType: "brokerage", includeInPortfolio: true },
          complete: true,
          baseValue: { amount: "180000", currency: "USD" },
          institutionName: "MooMoo SG",
          components: [],
          missingInputs: [],
        },
      ],
      missingInputs: [],
      byInstrumentType: [{ key: "stock", label: "stock", amount: { amount: "180000", currency: "USD" }, shareBps: 10000 }],
      byCurrency: [],
      byCountry: [],
    });
    renderPage(onOpenAccount);
    expect(await screen.findByTestId("portfolio-total")).toHaveTextContent("$320,000.00");
    expect(screen.getByText(/manual-value accounts are not in the portfolio/i)).toBeInTheDocument();
    expect(screen.getAllByText("Stock").length).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole("button", { name: "MooMoo SG Brokerage" }));
    expect(onOpenAccount).toHaveBeenCalledWith("brk-1");
  });

  it("shows an empty state when no accounts are included", async () => {
    portfolio.mockResolvedValue({
      currency: "USD",
      complete: true,
      accounts: [],
      missingInputs: [],
      byInstrumentType: [],
      byCurrency: [],
      byCountry: [],
    });
    renderPage();
    expect(await screen.findByText("No holdings yet")).toBeInTheDocument();
  });

  it("does not treat incomplete valuation as zero", async () => {
    portfolio.mockResolvedValue({
      currency: "USD",
      complete: false,
      valuedSubtotal: { amount: "1000", currency: "USD" },
      accounts: [
        {
          account: { id: "brk-1", name: "MooMoo", accountType: "brokerage" },
          complete: false,
          baseValue: { amount: "1000", currency: "USD" },
          components: [],
          missingInputs: [{ kind: "instrument_price", accountId: "brk-1" }],
        },
      ],
      missingInputs: [{ kind: "instrument_price", accountId: "brk-1" }],
      byInstrumentType: [],
      byCurrency: [],
      byCountry: [],
    });
    renderPage();
    expect(await screen.findByText(/excludes values that still need a price/i)).toBeInTheDocument();
    expect(screen.getByText("Partial valuation")).toBeInTheDocument();
    expect(screen.getByTestId("portfolio-total")).toHaveTextContent("$1,000.00");
  });
});
