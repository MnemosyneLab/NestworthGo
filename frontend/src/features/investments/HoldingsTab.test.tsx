vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: () => historyOrigin() } }));
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { queryKeys } from "@/queries/keys";
import { HoldingsTab } from "./HoldingsTab";
import i18n from "@/i18n";

const historyOrigin = vi.fn();
const listInstruments = vi.fn();
const listAccounts = vi.fn();
const createHolding = vi.fn();
const holdingsByAccounts = vi.fn();
const instrumentHoldingGroups = vi.fn();
const positionImport = vi.fn();
const listProducts = vi.fn();
vi.mock("./PositionImportForm", () => ({ PositionImportForm: (props: { account?: { id: string } }) => {
  positionImport(props);
  return <div data-testid="position-import-form">Dated position import</div>;
} }));

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
  Service: { InstrumentHoldingGroups: (...args: unknown[]) => instrumentHoldingGroups(...args) },
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
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: { ListProducts: (...args: unknown[]) => listProducts(...args), Overview: vi.fn() },
}));

function renderPage(props: ComponentProps<typeof HoldingsTab> = {}, queryClient = createTestQueryClient({ retry: false })) {
  return render(
    <QueryClientProvider client={queryClient}>
      <HoldingsTab {...props} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  historyOrigin.mockResolvedValue(null);
  listInstruments.mockReset();
  listAccounts.mockReset();
  createHolding.mockReset();
  listProducts.mockReset();
  listProducts.mockResolvedValue([]);
  holdingsByAccounts.mockReset();
  instrumentHoldingGroups.mockReset();
  listInstruments.mockResolvedValue([]);
  listAccounts.mockResolvedValue([
    { account: { id: "acc-1", name: "Brokerage", trackingMode: "holdings" }, ownership: [], latestValue: null },
  ]);
  holdingsByAccounts.mockResolvedValue({ "acc-1": [] });
  instrumentHoldingGroups.mockResolvedValue([]);
  createHolding.mockResolvedValue({ id: "h1", accountId: "acc-1", instrumentId: "i1", quantity: "10" });
});

describe("HoldingsTab", () => {
  it("loads product exclusions only for the ordinary sheet and blocks selection until fresh exclusions arrive", async () => {
    let resolveProducts!: (value: unknown[]) => void;
    listProducts.mockImplementation(() => new Promise((resolve) => { resolveProducts = resolve; }));
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage();
    expect(await screen.findByText(i18n.t("portfolio.noHoldings"))).toBeInTheDocument();
    expect(listProducts).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));
    await waitFor(() => expect(listProducts).toHaveBeenCalledTimes(1));
    expect(screen.getByLabelText("Instrument")).toBeDisabled();
    const submitButtons = screen.getAllByRole("button", { name: "Add holding" });
    const submit = submitButtons[submitButtons.length - 1];
    expect(submit).toBeDisabled();
    resolveProducts([{ product: { instrumentId: "i1" } }]);
    await waitFor(() => expect(screen.getByLabelText("Instrument")).not.toBeDisabled());
    expect(screen.getByLabelText("Instrument")).not.toHaveTextContent("NVIDIA");
  });

  it("does not submit with a stale cached product list during refresh", async () => {
    const client = createTestQueryClient({ retry: false });
    client.setQueryData(queryKeys.liquidity.products("", true), []);
    let resolveProducts!: (value: unknown[]) => void;
    listProducts.mockImplementation(() => new Promise((resolve) => { resolveProducts = resolve; }));
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", quoteCurrency: "USD", quoteSource: "manual" }]);
    renderPage({}, client);
    await userEvent.click(await screen.findByRole("button", { name: "Add holding" }));
    await waitFor(() => expect(listProducts).toHaveBeenCalled());
    expect(screen.getByLabelText("Instrument")).toBeDisabled();
    resolveProducts([{ product: { instrumentId: "i1" } }]);
    await waitFor(() => expect(screen.getByLabelText("Instrument")).not.toBeDisabled());
    expect(screen.getByLabelText("Instrument")).not.toHaveTextContent("NVIDIA");
    expect(createHolding).not.toHaveBeenCalled();
  });

  it("keeps product lookup errors inside the sheet and retries there", async () => {
    listProducts.mockRejectedValueOnce(new Error("unavailable")).mockResolvedValue([]);
    renderPage();
    await screen.findByText(i18n.t("portfolio.noHoldings"));
    await userEvent.click(screen.getByRole("button", { name: "Add holding" }));
    await screen.findByRole("button", { name: "Try again" });
    expect(screen.getByText(i18n.t("portfolio.noHoldings"))).toBeInTheDocument();
    expect(screen.getByLabelText("Instrument")).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(screen.getByLabelText("Instrument")).not.toBeDisabled());
  });
  it("does not duplicate instrument management on the holdings page", async () => {
    renderPage();
    expect(await screen.findByText(i18n.t("portfolio.noHoldings"))).toBeInTheDocument();
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
    await waitFor(() => expect(instrumentHoldingGroups).toHaveBeenCalledTimes(2));
  });

  it("routes existing holdings to the dated import form after history starts", async () => {
    historyOrigin.mockResolvedValue({ id: "origin" });
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Record existing holding" }));
    expect(screen.getByTestId("position-import-form")).toBeInTheDocument();
    expect(createHolding).not.toHaveBeenCalled();
  });

  it("opens the import draft in the newly created investment account", async () => {
    historyOrigin.mockResolvedValue({ id: "origin" });
    renderPage({ draftRequest: { requestId: 1, accountId: "acc-1" } });
    expect(await screen.findByRole("dialog", { name: "Record existing holding" })).toBeInTheDocument();
    expect(positionImport).toHaveBeenCalledWith(expect.objectContaining({ account: expect.objectContaining({ id: "acc-1" }) }));
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
