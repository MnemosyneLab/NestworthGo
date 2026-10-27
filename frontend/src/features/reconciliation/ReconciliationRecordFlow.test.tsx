import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { ReconciliationPage } from "./ReconciliationPage";

// Mock data/service boundaries, retaining the actual RecordChangeForm, Sheet,
// selects and inputs so a holding-only default cannot pass this regression.
const q = vi.hoisted(() => ({
  previewChange: vi.fn(),
  accounts: [
    {
      account: {
        id: "one",
        name: "Synthetic brokerage",
        defaultCurrency: "USD",
        trackingMode: "holdings",
        accountType: "brokerage",
        balanceSheetRole: "asset",
      },
    },
  ],
  instruments: [
    {
      id: "asset-one",
      name: "Synthetic ETF",
      type: "etf",
      quoteCurrency: "USD",
      quoteSource: "manual",
    },
  ],
  holdings: [
    {
      id: "held-one",
      accountId: "one",
      accountName: "Synthetic brokerage",
      instrumentId: "asset-one",
      instrumentName: "Synthetic ETF",
      quantity: "2",
    },
  ],
  session: {
    id: "session-one",
    householdId: "household",
    status: "in_progress",
    revision: 1,
    startedAt: "2026-09-01T12:00:00Z",
    note: "",
    closingNote: "",
    partial: false,
    counts: { pending: 1, confirmed: 0, skipped: 0, unresolved: 0 },
    accounts: [
      {
        accountId: "one",
        name: "Synthetic brokerage",
        currency: "USD",
        mode: "holdings",
        financialRevision: 12,
        priorRevision: 0,
        outcome: "partial",
        recheckRequired: false,
        counts: { pending: 1, confirmed: 0, skipped: 0, unresolved: 0 },
        items: [
          {
            id: "holding-item",
            accountId: "one",
            accountName: "Synthetic brokerage",
            kind: "holding",
            holdingId: "held-one",
            instrumentId: "asset-one",
            label: "Synthetic ETF",
            currency: "USD",
            value: "2",
            cost: "10",
            financialRevision: 12,
            recheckRequired: false,
            outcome: "pending",
            note: "",
            source: { reference: "", asOfDate: "2026-09-01" },
          },
        ],
      },
    ],
  },
}));
vi.mock("@/queries/reconciliation", () => ({
  useReconciliationSession: () => ({ data: q.session }),
  useReconciliationSessions: () => ({ data: [] }),
  useReconciliationLinks: () => ({ data: [] }),
  useReconciliationChanges: () => ({ data: [] }),
  ReconciliationService: {},
}));
vi.mock("@/queries/accounts", () => ({
  useAccounts: () => ({ data: q.accounts }),
  useAccountValuations: () => ({ data: [] }),
}));
vi.mock("@/queries/investments", () => ({
  useInstruments: () => ({ data: q.instruments }),
  useAllHoldingsFlat: () => ({ data: q.holdings }),
  useCurrentFXQuote: () => ({ data: null }),
  useCurrentInstrumentQuote: () => ({ data: null }),
  useFXPreferences: () => ({ data: [] }),
  useCreateInstrument: () => ({}),
  useSetFXPreference: () => ({}),
}));
vi.mock("@/queries/history", () => ({
  useHistoryOrigin: () => ({
    data: { id: "origin", startedAt: "2026-09-01T00:00:00Z", timezone: "UTC" },
  }),
  useHistoryMutationAllowed: () => ({ data: {} }),
  usePreviewChange: () => ({ mutate: q.previewChange, reset: vi.fn() }),
  usePreviewRepeatChange: () => ({ reset: vi.fn() }),
  useConfirmRepeatChange: () => ({}),
  usePreviewFixChange: () => ({ reset: vi.fn() }),
  useRecordChange: () => ({}),
  useFixChange: () => ({}),
}));
vi.mock("@/queries/settings", () => ({
  useSettings: () => ({
    data: { weekStart: "monday", dateFormat: "iso", timezone: "UTC" },
  }),
  useSupportedCurrencies: () => ({ data: ["USD"] }),
}));
vi.mock("@/queries/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { useCatalog: () => ({ data: TEST_CATALOG }) };
});
vi.mock("@/queries/household", () => ({
  useBootstrap: () => ({
    data: {
      household: { id: "household", baseCurrency: "USD" },
      members: [],
      groups: [],
      institutions: [],
    },
  }),
}));
vi.mock("@/queries/marketdata", () => ({
  useRefreshFX: () => ({}),
  useRefreshInstrument: () => ({}),
}));

const originalFixtures = structuredClone({ accounts: q.accounts, holdings: q.holdings, session: q.session });
beforeEach(async () => {
  vi.clearAllMocks();
  q.accounts.splice(0, q.accounts.length, ...structuredClone(originalFixtures.accounts));
  q.holdings.splice(0, q.holdings.length, ...structuredClone(originalFixtures.holdings));
  q.session = structuredClone(originalFixtures.session);
  await i18n.changeLanguage("en");
});

for (const locale of ["en"]) {
  it(`preselects the known buy instrument and previews without reselecting it (${locale})`, async () => {
    await i18n.changeLanguage(locale);
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <ReconciliationPage sessionId="session-one" />
      </QueryClientProvider>,
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: i18n.t("reconciliation.recordMissing"),
      }),
    );
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/Synthetic ETF/, { selector: "p" })).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(i18n.t("history.instrument"))).not.toBeInTheDocument();
    expect(within(dialog).getByLabelText(i18n.t("history.side"))).toHaveValue(
      "buy",
    );
    expect(within(dialog).queryByLabelText(i18n.t("history.settlementAccount"))).not.toBeInTheDocument();
    expect(within(dialog).getByText(/Account being reconciled: Synthetic brokerage/)).toBeInTheDocument();
    fireEvent.change(
      within(dialog).getByLabelText(i18n.t("history.quantity")),
      { target: { value: "1" } },
    );
    fireEvent.change(
      within(dialog).getByLabelText(i18n.t("history.grossTotal")),
      { target: { value: "10" } },
    );
    fireEvent.click(
      within(dialog).getByRole("button", { name: i18n.t("common.preview") }),
    );
    expect(q.previewChange).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "trade",
        side: "buy",
        instrumentId: "asset-one",
        holdingId: "held-one",
        settlementAccountId: "one",
        reconciliationLink: {
          sessionId: "session-one",
          accountId: "one",
          itemId: "holding-item",
          expectedRevision: 1,
          financialRevision: 12,
        },
      }),
      expect.any(Object),
    );
  });
}

it("retains the exact holding across dividend, quantity and incoming/outgoing transfer drafts", async () => {
  q.accounts.push({ account: { ...q.accounts[0].account, id: "two", name: "Synthetic other brokerage" } });
  q.holdings.push({ ...q.holdings[0], id: "held-two", accountId: "two", accountName: "Synthetic other brokerage" });
  render(<QueryClientProvider client={createTestQueryClient()}><ReconciliationPage sessionId="session-one" /></QueryClientProvider>);
  fireEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.recordMissing") }));
  const dialog = await screen.findByRole("dialog");
  fireEvent.click(within(dialog).getByRole("radio", { name: i18n.t("history.kind.cash_dividend") }));
  expect(within(dialog).queryByLabelText(i18n.t("history.holding"))).not.toBeInTheDocument();
  fireEvent.change(within(dialog).getByLabelText(i18n.t("history.amount")), { target: { value: "5" } });
  fireEvent.click(within(dialog).getByRole("button", { name: i18n.t("common.preview") }));
  expect(q.previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ kind: "cash_dividend", holdingId: "held-one", accountId: "one" }), expect.anything());
  fireEvent.click(within(dialog).getByRole("radio", { name: i18n.t("history.kind.position_transfer") }));
  expect(within(dialog).queryByLabelText(i18n.t("history.fromHolding"))).not.toBeInTheDocument();
  fireEvent.change(within(dialog).getByLabelText(i18n.t("history.toAccount")), { target: { value: "two" } });
  fireEvent.change(within(dialog).getByLabelText(i18n.t("history.quantity")), { target: { value: "1" } });
  fireEvent.click(within(dialog).getByRole("button", { name: i18n.t("common.preview") }));
  expect(q.previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ fromHoldingId: "held-one", toAccountId: "two", accountId: "one" }), expect.anything());
  fireEvent.click(within(dialog).getByLabelText(i18n.t("reconciliation.transferIncoming")));
  expect(within(dialog).queryByLabelText(i18n.t("history.toAccount"))).not.toBeInTheDocument();
  fireEvent.change(within(dialog).getByLabelText(i18n.t("history.fromHolding")), { target: { value: "held-two" } });
  fireEvent.change(within(dialog).getByLabelText(i18n.t("history.quantity")), { target: { value: "1" } });
  fireEvent.click(within(dialog).getByRole("button", { name: i18n.t("common.preview") }));
  expect(q.previewChange).toHaveBeenLastCalledWith(expect.objectContaining({ fromHoldingId: "held-two", toHoldingId: "held-one", toAccountId: "", accountId: "one" }), expect.anything());
});
