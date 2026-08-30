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
});
