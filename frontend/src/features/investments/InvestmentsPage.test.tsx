import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { InvestmentsPage } from "./InvestmentsPage";

const listInstruments = vi.fn();
const createInstrument = vi.fn();
const archiveInstrument = vi.fn().mockResolvedValue(undefined);
const listAccounts = vi.fn();
const createHolding = vi.fn();
const holdingsByAccounts = vi.fn();
const accountGain = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: (...args: unknown[]) => listInstruments(...args),
    CreateInstrument: (...args: unknown[]) => createInstrument(...args),
    ArchiveInstrument: (...args: unknown[]) => archiveInstrument(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args),
    CreateHolding: (...args: unknown[]) => createHolding(...args),
    ArchiveHolding: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({
  Service: { AccountGain: (...args: unknown[]) => accountGain(...args), RealizedGain: vi.fn(), HoldingGain: vi.fn() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({ Service: {} }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: (...args: unknown[]) => listAccounts(...args) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { SupportedCurrencies: () => Promise.resolve(["USD"]) },
}));

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <InvestmentsPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listInstruments.mockReset();
  createInstrument.mockReset();
  archiveInstrument.mockClear();
  listAccounts.mockReset();
  createHolding.mockReset();
  holdingsByAccounts.mockReset();
  accountGain.mockReset();

  listInstruments.mockResolvedValue([]);
  createInstrument.mockResolvedValue({ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" });
  listAccounts.mockResolvedValue([
    { account: { id: "acc-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
  ]);
  holdingsByAccounts.mockResolvedValue({ "acc-1": [] });
  accountGain.mockResolvedValue({ accountId: "acc-1", holdings: [], available: true });
  createHolding.mockResolvedValue({ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" });
});

describe("InvestmentsPage", () => {
  it("creates an Instrument with manual quote source", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.type(within(form).getByLabelText("Name"), "NVIDIA");
    await userEvent.click(within(form).getByRole("button", { name: "Add instrument" }));

    expect(createInstrument).toHaveBeenCalledWith(
      expect.objectContaining({ name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }),
    );
  });

  it("creates a Holding for a Holdings-mode Account", async () => {
    renderPage();
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    await userEvent.click(screen.getByRole("tab", { name: "Holdings" }));
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));

    await screen.findByLabelText("Account");
    await userEvent.selectOptions(screen.getByLabelText("Account"), "acc-1");
    await userEvent.selectOptions(await screen.findByLabelText("Instrument"), "i1");
    await userEvent.type(screen.getByLabelText("Quantity"), "10");
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));

    expect(createHolding).toHaveBeenCalledWith({ accountId: "acc-1", instrumentId: "i1", quantity: "10" });
  });

  it("shows per-Holding cost/gain columns from AnalyticsService.HoldingGain data", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    holdingsByAccounts.mockResolvedValue({ "acc-1": [{ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" }] });
    accountGain.mockResolvedValue({
      accountId: "acc-1",
      available: true,
      holdings: [
        {
          holdingId: "h1",
          accountId: "acc-1",
          instrumentId: "i1",
          instrumentName: "NVIDIA",
          quantity: "10",
          averageCost: { amount: "100", currency: "USD" },
          totalCost: { amount: "1000", currency: "USD" },
          currentValue: { amount: "1500", currency: "USD" },
          realizedGain: { amount: "0", currency: "USD" },
          unrealizedGain: { amount: "500", currency: "USD" },
          available: true,
        },
      ],
    });

    renderPage();
    await userEvent.click(screen.getByRole("tab", { name: "Holdings" }));

    expect(await screen.findByRole("columnheader", { name: "Cost" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Current value" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Unrealized gain" })).toBeInTheDocument();
    const table = screen.getByTestId("holdings-table");
    expect(table).toHaveTextContent("$1,000.00");
    expect(table).toHaveTextContent("$1,500.00");
    expect(table).toHaveTextContent("$500.00");
  });

  it("shows an unavailable badge when a Holding's gain cannot be computed", async () => {
    holdingsByAccounts.mockResolvedValue({ "acc-1": [{ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" }] });
    accountGain.mockResolvedValue({
      accountId: "acc-1",
      available: false,
      holdings: [
        {
          holdingId: "h1",
          accountId: "acc-1",
          instrumentId: "i1",
          instrumentName: "NVIDIA",
          quantity: "10",
          averageCost: { amount: "100", currency: "USD" },
          totalCost: { amount: "1000", currency: "USD" },
          realizedGain: { amount: "0", currency: "USD" },
          available: false,
          missingReason: "no current price",
        },
      ],
    });

    renderPage();
    await userEvent.click(screen.getByRole("tab", { name: "Holdings" }));

    expect(await screen.findByText("No current price")).toBeInTheDocument();
  });

  it("archives an Instrument after confirming the AlertDialog", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await screen.findByText("NVIDIA");
    await userEvent.click(screen.getByRole("button", { name: "Archive" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
    expect(archiveInstrument).toHaveBeenCalledWith("i1", true);
  });
});
