import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { ProductCorrection } from "./ProductCorrection";
import type { ProductDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
const q = vi.hoisted(() => ({
  product: undefined as ProductDTO | undefined,
  terms: vi.fn(),
  value: vi.fn(),
  operation: vi.fn(),
}));
vi.mock("@/queries/liquidity", () => ({
  useProduct: () => ({ data: { product: q.product }, isError: false }),
}));
vi.mock("@/queries/history", () => ({
  useHistoryOrigin: () => ({ data: { timezone: "UTC" } }),
}));
vi.mock("@/queries/settings", () => ({
  useSettings: () => ({
    data: { weekStart: "monday", dateFormat: "iso", timezone: "UTC" },
  }),
}));
vi.mock("@/queries/reconciliation", () => ({
  ReconciliationService: {
    PreviewProductTerms: q.terms,
    PreviewProductValuation: q.value,
    PreviewProductOperation: q.operation,
  },
}));
vi.mock("@/lib/wails", () => ({ callService: (f: () => unknown) => f() }));
const money = (amount: string) => ({ amount, currency: "EUR" });
const link = {
  sessionId: "session",
  accountId: "account",
  itemId: "product:product",
  expectedRevision: 1,
  financialRevision: 4,
};
beforeEach(async () => {
  await i18n.changeLanguage("en");
  vi.clearAllMocks();
  q.product = {
    id: "product",
    accountId: "account",
    holdingId: "holding",
    instrumentId: "instrument",
    kind: "term_deposit",
    name: "Synthetic deposit",
    note: "Opening terms",
    currency: "EUR",
    principal: money("1000"),
    currentValue: money("1000"),
    currentCostBasis: money("1000"),
    startOn: "2026-01-01",
    maturityOn: "2026-09-20",
    interestMode: "none",
    annualRate: null,
    maturityInterest: null,
    interestPaidThroughOn: null,
    state: "open",
    displayState: "due_unconfirmed",
    revision: 3,
    predecessorId: null,
    successorId: null,
    nextAction: "confirm_receipt",
    policy: {
      id: "policy",
      sourceRef: {
        kind: "holding",
        accountId: "account",
        holdingId: "holding",
        currency: null,
      },
      accessKind: "on_date",
      unlockOn: "2026-09-20",
      settlementDays: 0,
      dayBasis: "calendar",
      receiptOnOverride: null,
      accessibleAmountCap: null,
      normalExitFee: money("0"),
      earlyKind: "not_allowed",
      earlySettlementDays: null,
      earlyDayBasis: null,
      earlyFee: null,
      earlyAmountMode: null,
      earlyGrossAmount: null,
      confirmedAt: null,
      note: "",
      revision: 4,
    },
  };
  q.terms.mockResolvedValue({ planId: "terms" });
  q.value.mockResolvedValue({ planId: "value" });
  q.operation.mockResolvedValue({ planId: "operation" });
});
function mount() {
  const onPreview = vi.fn();
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <ProductCorrection
        productId="product"
        link={link}
        onPreview={onPreview}
      />
    </QueryClientProvider>,
  );
  return onPreview;
}
it("uses the existing deposit fields and real maturity picker without recording receipt", async () => {
  mount();
  expect(
    screen.getByText(/Maturity changes the contract date/),
  ).toBeInTheDocument();
  await userEvent.click(screen.getByLabelText("Maturity date"));
  const grid = await screen.findByRole("grid");
  await userEvent.click(
    grid.querySelector('[data-day="2026-09-21"] button') as HTMLElement,
  );
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.terms).toHaveBeenCalledWith(
    expect.objectContaining({
      productId: "product",
      reconciliationLink: link,
      terms: expect.objectContaining({ maturityOn: "2026-09-21" }),
    }),
  );
  expect(q.operation).not.toHaveBeenCalled();
});
it("keeps the valuation route and shows only the conditional receipt fields", async () => {
  q.product!.kind = "locked_product";
  mount();
  fireEvent.change(
    screen.getByLabelText("Correct product terms or record actual receipt"),
    { target: { value: "settle" } },
  );
  expect(screen.getByLabelText("Gross proceeds")).toBeInTheDocument();
  expect(screen.queryByLabelText("Interest received")).not.toBeInTheDocument();
  fireEvent.change(
    screen.getByLabelText("Correct product terms or record actual receipt"),
    { target: { value: "value" } },
  );
  await userEvent.type(screen.getByLabelText("Current value"), "1000.0001");
  expect(screen.queryByLabelText("Gross proceeds")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.value).toHaveBeenCalledWith(link, "product", "1000.0001", "");
  expect(q.operation).not.toHaveBeenCalled();
  expect(
    within(
      screen.getByLabelText("Correct product terms or record actual receipt"),
    ).getByRole("option", { name: "Update current valuation" }),
  ).toBeInTheDocument();
});
