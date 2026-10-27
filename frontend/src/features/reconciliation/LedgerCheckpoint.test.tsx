import { beforeEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import i18n from "@/i18n";
import { LedgerCheckpoint } from "./LedgerCheckpoint";
import { ReconciliationLedgerContext } from "./ReconciliationLedgerContext";
import type { ReconciliationItem } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
vi.mock("./useReconciliationTimestamp", () => ({ useReconciliationTimestamp: () => (value: string) => value }));
beforeEach(async () => { await i18n.changeLanguage("en"); });
const item = (kind: string, value: string) => ({ id: "synthetic", accountId: "account", accountName: "Synthetic account", label: "Synthetic item", kind, value, currency: "USD", source: { reference: "", asOfDate: "" }, outcome: "pending", note: "" }) as ReconciliationItem;
it("preserves tiny and full holding quantities instead of displaying rounded zero", () => {
  const { rerender } = render(<LedgerCheckpoint item={item("holding", "0.000000000123456789")} />);
  expect(screen.getByText(/Ledger quantity:/).closest("p")).toHaveTextContent("0.000000000123456789");
  rerender(<LedgerCheckpoint item={item("holding", "123456789.12345678")} />);
  expect(screen.getByText(/Ledger quantity:/).closest("p")).toHaveTextContent("123456789.12345678");
});
it("distinguishes ledger cash, exact debt balances and positive source inputs without mutating evidence", () => {
  const debt = { ...item("balance", "100.001"), referenceValue: "90", referenceComparable: true, outcome: "confirmed" };
  render(<ReconciliationLedgerContext.Provider value={new Set(["account"])}><LedgerCheckpoint item={debt} evidence /></ReconciliationLedgerContext.Provider>);
  expect(screen.getByText(/Ledger balance:/).closest("p")).toHaveTextContent("-100.001 USD");
  expect(screen.getByText(/Value shown by your source/)).toHaveTextContent("-US$90.00");
  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(debt.value).toBe("100.001"); expect(debt.referenceValue).toBe("90");
});
it("shows cash currency, managed principal, and completeness context without inventing a number", () => {
  const { rerender } = render(<LedgerCheckpoint item={item("cash", "12")} />);
  expect(screen.getByText(/Ledger balance:/).closest("p")).toHaveTextContent("US$12.00");
  rerender(<LedgerCheckpoint item={item("product", "250")} />);
  expect(screen.getByText(/Principal:/).closest("p")).toHaveTextContent("US$250.00");
  rerender(<LedgerCheckpoint item={{ ...item("scope", ""), referenceValue: "Three synthetic currencies" }} evidence />);
  expect(screen.queryByText(/Ledger balance:/)).not.toBeInTheDocument();
  expect(screen.getByText(/Three synthetic currencies/)).toBeInTheDocument();
  expect(screen.getByText(/not a live market quote/)).toBeInTheDocument();
});
