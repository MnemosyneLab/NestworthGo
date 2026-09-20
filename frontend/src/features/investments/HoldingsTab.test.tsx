vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: () => historyOrigin() } }));
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { HoldingsTab } from "./HoldingsTab";

const historyOrigin = vi.fn();
const listInstruments = vi.fn();
const listAccounts = vi.fn();
const createHolding = vi.fn();
const holdingsByAccounts = vi.fn();
const instrumentHoldings = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: (...args: unknown[]) => listInstruments(...args),
    CreateInstrument: vi.fn(),
    UpdateInstrument: vi.fn(),
    ArchiveInstrument: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args),
    CreateHolding: (...args: unknown[]) => createHolding(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({
  Service: { InstrumentHoldings: (...args: unknown[]) => instrumentHoldings(...args) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    AppendManualInstrumentQuote: vi.fn(),
    CurrentInstrumentQuote: vi.fn().mockResolvedValue(null),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: (...args: unknown[]) => listAccounts(...args) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: {
    Bootstrap: () =>
      Promise.resolve({
        household: { id: "h1", name: "Test", baseCurrency: "USD", createdAt: "", updatedAt: "" },
        members: [],
        institutions: [],
        groups: [],
      }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { SupportedCurrencies: () => Promise.resolve(["USD"]) },
}));

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <HoldingsTab />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  historyOrigin.mockResolvedValue(null);
  listInstruments.mockReset();
  listAccounts.mockReset();
  createHolding.mockReset();
  holdingsByAccounts.mockReset();
  instrumentHoldings.mockReset();
  listInstruments.mockResolvedValue([]);
  listAccounts.mockResolvedValue([
    { account: { id: "acc-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
  ]);
  holdingsByAccounts.mockResolvedValue({ "acc-1": [] });
  instrumentHoldings.mockResolvedValue([]);
  createHolding.mockResolvedValue({ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" });
});

describe("HoldingsTab", () => {
  it("does not duplicate instrument management on the holdings page", async () => {
    renderPage();
    expect(await screen.findByText(/Instrument identity and quotes live on Market Data/i)).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Instruments" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add instrument" })).not.toBeInTheDocument();
  });

  it("creates a Holding for a Holdings-mode Account", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add holding" }));

    await screen.findByLabelText("Account");
    await userEvent.selectOptions(screen.getByLabelText("Account"), "acc-1");
    await userEvent.selectOptions(await screen.findByLabelText("Instrument"), "i1");
    await userEvent.type(screen.getByLabelText("Quantity"), "10");
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));

    expect(createHolding).toHaveBeenCalledWith({ accountId: "acc-1", instrumentId: "i1", quantity: "10" });
    await waitFor(() => expect(instrumentHoldings).toHaveBeenCalledTimes(2));
  });

  it("accepts an explicit unit cost after history starts", async () => {
    historyOrigin.mockResolvedValue({ id: "origin" });
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add holding" }));
    await userEvent.selectOptions(screen.getByLabelText("Account"), "acc-1");
    await userEvent.selectOptions(screen.getByLabelText("Instrument"), "i1");
    await userEvent.type(screen.getByLabelText("Quantity"), "10");
    await userEvent.type(screen.getByLabelText("Unit cost"), "80");
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));
    expect(createHolding).toHaveBeenCalledWith({ accountId: "acc-1", instrumentId: "i1", quantity: "10", unitCost: "80" });
  });

  it("does not offer cash-only multi-currency accounts for investment holdings", async () => {
    listAccounts.mockResolvedValue([
      { account: { id: "cash-1", name: "Travel cash", accountType: "cash_on_hand", trackingMode: "holdings" }, ownership: [], latestValue: null },
    ]);
    renderPage();

    expect(await screen.findByText("No investment account available")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add holding" })).not.toBeInTheDocument();
  });

  it("shows a validation error when Add holding is submitted empty", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add holding" }));
    await screen.findByLabelText("Account");
    const submitButtons = screen.getAllByRole("button", { name: "Add holding" });
    await userEvent.click(submitButtons[submitButtons.length - 1]);
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose an account, instrument, and quantity.");
    expect(createHolding).not.toHaveBeenCalled();
  });
});
