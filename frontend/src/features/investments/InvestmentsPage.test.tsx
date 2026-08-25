import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { InvestmentsPage } from "./InvestmentsPage";

const listInstruments = vi.fn();
const createInstrument = vi.fn();
const listAccounts = vi.fn();
const createHolding = vi.fn();
const holdingsByAccounts = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: (...args: unknown[]) => listInstruments(...args),
    CreateInstrument: (...args: unknown[]) => createInstrument(...args),
    ArchiveInstrument: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args),
    CreateHolding: (...args: unknown[]) => createHolding(...args),
    ArchiveHolding: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({ Service: {} }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: (...args: unknown[]) => listAccounts(...args) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { SupportedCurrencies: () => Promise.resolve(["USD"]) },
}));

function renderPage() {
  const queryClient = new QueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <InvestmentsPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listInstruments.mockReset();
  createInstrument.mockReset();
  listAccounts.mockReset();
  createHolding.mockReset();
  holdingsByAccounts.mockReset();

  listInstruments.mockResolvedValue([]);
  createInstrument.mockResolvedValue({ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" });
  listAccounts.mockResolvedValue([
    { account: { id: "acc-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
  ]);
  holdingsByAccounts.mockResolvedValue({ "acc-1": [] });
  createHolding.mockResolvedValue({ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" });
});

describe("InvestmentsPage", () => {
  it("creates an Instrument with manual quote source", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "Add" }));
    const form = await screen.findByRole("form", { name: "Instrument form" });
    await userEvent.type(within(form).getByLabelText("Name"), "NVIDIA");
    await userEvent.click(within(form).getByRole("button", { name: "Add" }));

    expect(createInstrument).toHaveBeenCalledWith(
      expect.objectContaining({ name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }),
    );
  });

  it("creates a Holding for a Holdings-mode Account", async () => {
    renderPage();
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    await userEvent.click(screen.getByRole("tab", { name: "Holdings" }));
    await userEvent.click(screen.getByRole("button", { name: "Add" }));

    await screen.findByLabelText("Account");
    await userEvent.selectOptions(screen.getByLabelText("Account"), "acc-1");
    await userEvent.selectOptions(await screen.findByLabelText("Instrument"), "i1");
    await userEvent.type(screen.getByLabelText("Quantity"), "10");
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));

    expect(createHolding).toHaveBeenCalledWith({ accountId: "acc-1", instrumentId: "i1", quantity: "10" });
  });
});
