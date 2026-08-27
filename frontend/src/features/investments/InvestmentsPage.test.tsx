import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
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
const currentInstrumentQuote = vi.fn();

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
const saveManualQuote = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    SaveManualInstrumentQuote: (...args: unknown[]) => saveManualQuote(...args),
    CurrentInstrumentQuote: (...args: unknown[]) => currentInstrumentQuote(...args),
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
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

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
  saveManualQuote.mockReset();
  currentInstrumentQuote.mockReset();
  saveManualQuote.mockResolvedValue({ id: "q1", instrumentId: "i1", unitPrice: "131.70" });
  currentInstrumentQuote.mockResolvedValue(null);

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
  it("shows the latest saved instrument price in the instrument list", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "provider" }]);
    currentInstrumentQuote.mockResolvedValue({
      id: "q1",
      instrumentId: "i1",
      unitPrice: "131.70",
      currency: "USD",
      sourceKind: "provider",
      sourceKey: "yahoo_finance",
      quotedAt: "2024-01-01T00:00:00Z",
      createdAt: "2024-01-01T00:00:00Z",
      delayed: true,
    });

    renderPage();

    expect(await screen.findByText("Latest: $131.70")).toBeInTheDocument();
  });

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
          missingReason: "current instrument price is unavailable",
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

  it("saves a manual instrument quote from Set price", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await screen.findByText("NVIDIA");
    await userEvent.click(screen.getByRole("button", { name: "Set price" }));
    await userEvent.type(await screen.findByLabelText("Unit price"), "131.70");
    const priceButtons = screen.getAllByRole("button", { name: "Set price" });
    await userEvent.click(priceButtons[priceButtons.length - 1]);
    expect(saveManualQuote).toHaveBeenCalledWith("i1", "131.70", "");
  });

  it("shows a validation error when Add holding is submitted empty", async () => {
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    await userEvent.click(screen.getByRole("tab", { name: "Holdings" }));
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));
    await screen.findByLabelText("Account");
    const submitButtons = screen.getAllByRole("button", { name: "Add holding" });
    await userEvent.click(submitButtons[submitButtons.length - 1]);
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose an account, instrument, and quantity.");
    expect(createHolding).not.toHaveBeenCalled();
  });

  it("renders instrument type options from the catalog only", async () => {
    const { TEST_CATALOG, selectValues } = await import("@/test/catalog");
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add instrument" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await waitFor(() => {
      expect(selectValues(within(form).getByLabelText("Type"))).toEqual(TEST_CATALOG.instrumentTypes);
    });
  });
});
