import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import i18n from "@/i18n";
import { ActivityDetailSheet } from "./ActivityDetailSheet";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function t(key: string, options?: Record<string, unknown>) {
  return i18n.t(key, options as Record<string, string>) as string;
}

describe("ActivityDetailSheet", () => {
  it("renders cash dividend account, instrument, and amount", async () => {
    const activity = {
      id: "div-1",
      kind: "cash_dividend",
      reason: "income",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      dividendDetail: { holdingId: "h1", instrumentId: "i1", amount: { amount: "25", currency: "USD" } },
      effects: [{ accountId: "acc-1", money: { amount: "25", currency: "USD" } }],
    } as unknown as ActivityDTO;

    render(
      <ActivityDetailSheet
        activity={activity}
        timezone="UTC"
        accounts={new Map([["acc-1", "Brokerage"]])}
        instruments={new Map([["i1", "NVIDIA"]])}
        onClose={() => undefined}
        t={t}
      />,
    );

    expect(await screen.findByText("Cash dividend")).toBeInTheDocument();
    expect(screen.getByText("Dividend $25.00 from NVIDIA into Brokerage")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Account: Brokerage")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Instrument: NVIDIA")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Amount: $25.00")).toBeInTheDocument();
    expect(screen.queryByText(/unknown instrument/i)).not.toBeInTheDocument();
  });

  it("keeps the archived instrument name supplied by History", async () => {
    const activity = {
      id: "div-archived",
      kind: "cash_dividend",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      dividendDetail: { holdingId: "h1", instrumentId: "old-1", amount: { amount: "8", currency: "USD" } },
      effects: [{ accountId: "acc-1", money: { amount: "8", currency: "USD" } }],
    } as unknown as ActivityDTO;

    render(
      <ActivityDetailSheet
        activity={activity}
        timezone="UTC"
        accounts={new Map([["acc-1", "Brokerage"]])}
        instruments={new Map([["old-1", "Archived Fund"]])}
        onClose={() => undefined}
        t={t}
      />,
    );

    expect(await screen.findByText((_, node) => node?.textContent === "Instrument: Archived Fund")).toBeInTheDocument();
    expect(screen.queryByText(/unknown instrument/i)).not.toBeInTheDocument();
  });

  it("renders cash changes without exposing effect role or direction", async () => {
    const activity = {
      id: "cash-1",
      kind: "cash_in",
      reason: "income",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      effects: [{ role: "amount", direction: "added", accountId: "acc-1", money: { amount: "1000", currency: "USD" } }],
    } as unknown as ActivityDTO;

    render(<ActivityDetailSheet activity={activity} accounts={new Map([["acc-1", "Checking"]])} instruments={new Map()} onClose={() => undefined} t={t} />);

    expect(await screen.findByText((_, node) => node?.textContent === "Account: Checking")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Amount: $1,000.00")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Reason: Income")).toBeInTheDocument();
    expect(screen.queryByText(/amount · added/i)).not.toBeInTheDocument();
  });

  it("distinguishes a position transfer by its source and destination holdings", async () => {
    const activity = {
      id: "position-1",
      kind: "position_transfer",
      reason: "other",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      effects: [
        { role: "transfer_from", direction: "removed", holdingId: "holding-1", instrumentId: "i1", quantity: "2" },
        { role: "transfer_to", direction: "added", holdingId: "holding-2", instrumentId: "i1", quantity: "2" },
      ],
    } as unknown as ActivityDTO;

    render(
      <ActivityDetailSheet
        activity={activity}
        accounts={new Map()}
        instruments={new Map([["i1", "NVIDIA"]])}
        holdings={new Map([["holding-1", "Brokerage · NVIDIA"], ["holding-2", "Retirement · NVIDIA"]])}
        onClose={() => undefined}
        t={t}
      />,
    );

    expect(await screen.findByText("Moved 2 from Brokerage · NVIDIA to Retirement · NVIDIA")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Source holding: Brokerage · NVIDIA")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Destination holding: Retirement · NVIDIA")).toBeInTheDocument();
    expect(screen.queryByText(/transfer_from|transfer_to|removed|added/)).not.toBeInTheDocument();
  });

  it("labels a single position effect as a quantity adjustment", async () => {
    const activity = {
      id: "adjustment-1",
      kind: "position_transfer",
      reason: "reconciliation",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      effects: [{ role: "quantity", direction: "removed", holdingId: "holding-1", instrumentId: "i1", quantity: "3" }],
    } as unknown as ActivityDTO;

    render(<ActivityDetailSheet activity={activity} accounts={new Map()} instruments={new Map([["i1", "NVIDIA"]])} holdings={new Map([["holding-1", "Brokerage · NVIDIA"]])} onClose={() => undefined} t={t} />);

    expect(await screen.findByText((_, node) => node?.textContent === "Change type: Quantity adjustment")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Holding: Brokerage · NVIDIA")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Adjustment: Quantity removed")).toBeInTheDocument();
  });

  it("renders debt payment business roles and its fee", async () => {
    const activity = {
      id: "debt-1",
      kind: "debt_payment",
      reason: "principal",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      effects: [
        { role: "debt", direction: "removed", accountId: "debt", money: { amount: "500", currency: "USD" } },
        { role: "principal", direction: "removed", accountId: "cash", money: { amount: "500", currency: "USD" } },
        { role: "fee", direction: "removed", accountId: "cash", money: { amount: "5", currency: "USD" } },
      ],
    } as unknown as ActivityDTO;

    render(<ActivityDetailSheet activity={activity} accounts={new Map([["debt", "Credit card"], ["cash", "Checking"]])} instruments={new Map()} onClose={() => undefined} t={t} />);

    expect(await screen.findByText((_, node) => node?.textContent === "Debt account: Credit card")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Cash account: Checking")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Principal: $500.00")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Interest or fee: $5.00")).toBeInTheDocument();
    expect(screen.queryByText(/principal · removed|debt · removed|fee · removed/)).not.toBeInTheDocument();
  });

  it("renders both debt draw effects as debt account, cash account, and principal", async () => {
    const activity = {
      id: "draw-1",
      kind: "debt_draw",
      reason: "principal",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      effects: [
        { role: "debt", direction: "added", accountId: "debt", money: { amount: "10000", currency: "CNY" } },
        { role: "principal", direction: "added", accountId: "cash", money: { amount: "10000", currency: "CNY" } },
      ],
    } as unknown as ActivityDTO;

    render(<ActivityDetailSheet activity={activity} accounts={new Map([["debt", "Credit card"], ["cash", "CMB"]])} instruments={new Map()} onClose={() => undefined} t={t} />);

    expect(await screen.findByText((_, node) => node?.textContent === "Debt account: Credit card")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Cash account: CMB")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Principal: CN¥10,000.00")).toBeInTheDocument();
    expect(screen.queryByText(/debt · added|principal · added/)).not.toBeInTheDocument();
  });

  it("renders a reversal with the original summary and user-facing impacts", async () => {
    const originalActivity = {
      id: "cash-1",
      kind: "cash_in",
      reason: "income",
      effectiveAt: "2026-01-14T12:00:00.000Z",
      effectiveLocalDate: "2026-01-14",
      effects: [{ role: "amount", direction: "added", accountId: "acc-1", money: { amount: "100", currency: "USD" } }],
    } as unknown as ActivityDTO;
    const activity = {
      id: "reversal-1",
      kind: "reversal",
      reversesActivityId: "cash-1",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      effects: [{ role: "amount", direction: "removed", accountId: "acc-1", money: { amount: "100", currency: "USD" } }],
    } as unknown as ActivityDTO;

    render(<ActivityDetailSheet activity={activity} originalActivity={originalActivity} accounts={new Map([["acc-1", "Checking"]])} instruments={new Map()} onClose={() => undefined} t={t} />);

    expect(await screen.findByText((_, node) => node?.textContent === "Original activity: Added $100.00 to Checking (Income)")).toBeInTheDocument();
    expect(screen.getByText((_, node) => node?.textContent === "Reversal direction: Opposite of the original activity")).toBeInTheDocument();
    expect(screen.getByText("Checking · Amount removed: $100.00")).toBeInTheDocument();
    expect(screen.queryByText(/amount · removed/i)).not.toBeInTheDocument();
  });

  it("keeps the unknown-kind fallback free of raw role and direction codes", async () => {
    const activity = {
      id: "future-1",
      kind: "future_kind",
      effectiveAt: "2026-01-15T12:00:00.000Z",
      effectiveLocalDate: "2026-01-15",
      effects: [{ role: "internal_role", direction: "added", accountId: "acc-1", money: { amount: "12", currency: "USD" } }],
    } as unknown as ActivityDTO;

    render(<ActivityDetailSheet activity={activity} accounts={new Map([["acc-1", "Checking"]])} instruments={new Map()} onClose={() => undefined} t={t} />);

    expect(await screen.findByText("Checking · Amount added: $12.00")).toBeInTheDocument();
    expect(screen.queryByText(/internal_role/)).not.toBeInTheDocument();
  });
});
