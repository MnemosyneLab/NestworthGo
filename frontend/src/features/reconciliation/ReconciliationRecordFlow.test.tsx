import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

beforeEach(async () => {
  vi.clearAllMocks();
  await i18n.changeLanguage("en");
});

for (const locale of ["en", "zh-CN", "zh-TW"]) {
  it(`preselects the known buy instrument and previews without reselecting it (${locale})`, async () => {
    await i18n.changeLanguage(locale);
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <ReconciliationPage sessionId="session-one" />
      </QueryClientProvider>,
    );
    await userEvent.click(
      screen.getByRole("button", {
        name: i18n.t("reconciliation.recordMissing"),
      }),
    );
    const dialog = screen.getByRole("dialog");
    const instrument = within(dialog).getByLabelText(
      i18n.t("history.instrument"),
    );
    expect(instrument).toHaveValue("asset-one");
    expect(
      within(instrument).getByRole("option", { name: "Synthetic ETF" }),
    ).toHaveProperty("selected", true);
    expect(within(dialog).getByLabelText(i18n.t("history.side"))).toHaveValue(
      "buy",
    );
    expect(
      within(dialog).getByLabelText(i18n.t("history.settlementAccount")),
    ).toHaveValue("one");
    fireEvent.change(
      within(dialog).getByLabelText(i18n.t("history.quantity")),
      { target: { value: "1" } },
    );
    fireEvent.change(
      within(dialog).getByLabelText(i18n.t("history.grossTotal")),
      { target: { value: "10" } },
    );
    await userEvent.click(
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
