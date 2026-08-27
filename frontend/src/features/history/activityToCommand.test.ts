import { describe, expect, it } from "vitest";
import { ChangeCommandKind } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { activityToInitialCommand, emptyChangeRequest } from "@/features/history/activityToCommand";

/**
 * ACTIVITY_TO_COMMAND is the inverse of history.ChangeCommandRequest.ToCommand.
 * Keep this table in lockstep with internal/wailsapi/history/command.go.
 * Reversal is History-only and has no command kind.
 */
const ACTIVITY_TO_COMMAND: Record<string, ChangeCommandKind> = {
  cash_in: ChangeCommandKind.ChangeMoneyAdded,
  cash_out: ChangeCommandKind.ChangeMoneyRemoved,
  cash_transfer: ChangeCommandKind.ChangeCashTransfer,
  fx_conversion: ChangeCommandKind.ChangeFXConversion,
  buy: ChangeCommandKind.ChangeTrade,
  sell: ChangeCommandKind.ChangeTrade,
  value_update: ChangeCommandKind.ChangeValueUpdate,
  debt_draw: ChangeCommandKind.ChangeDebtDraw,
  debt_payment: ChangeCommandKind.ChangeDebtPayment,
};

describe("activityToInitialCommand kind mapping", () => {
  it.each(Object.entries(ACTIVITY_TO_COMMAND))("maps %s to %s", (activityKind, commandKind) => {
    const activity = {
      id: "a1",
      kind: activityKind,
      effects: [
        { role: "principal", accountId: "acc-1", money: { amount: "1", currency: "USD" } },
        { role: "transfer_from", accountId: "acc-1", holdingId: "h1", money: { amount: "1", currency: "USD" }, quantity: "1" },
        { role: "transfer_to", accountId: "acc-2", holdingId: "h2", money: { amount: "1", currency: "USD" }, quantity: "1" },
        { role: "debt", accountId: "debt-1", money: { amount: "1", currency: "USD" } },
        { role: "quantity", holdingId: "h1", quantity: "1" },
      ],
      tradeDetail: { holdingId: "h1", instrumentId: "i1", side: activityKind === "sell" ? "sell" : "buy", quantity: "1", gross: { amount: "1", currency: "USD" } },
    } as ActivityDTO;

    expect(activityToInitialCommand(activity).kind).toBe(commandKind);
    if (commandKind === ChangeCommandKind.ChangeTrade) {
      expect(activityToInitialCommand(activity).holdingId).toBe("h1");
    }
  });

  it("maps a two-sided position_transfer activity to ChangePositionTransfer", () => {
    const activity = {
      id: "a1",
      kind: "position_transfer",
      effects: [
        { role: "transfer_from", holdingId: "h1", quantity: "2" },
        { role: "transfer_to", holdingId: "h2", quantity: "2" },
      ],
    } as ActivityDTO;
    expect(activityToInitialCommand(activity).kind).toBe(ChangeCommandKind.ChangePositionTransfer);
  });

  it("maps a one-sided position_transfer activity to ChangePositionAdjustment", () => {
    const activity = {
      id: "a1",
      kind: "position_transfer",
      effects: [{ role: "quantity", holdingId: "h1", quantity: "2", direction: "removed" }],
    } as ActivityDTO;
    expect(activityToInitialCommand(activity).kind).toBe(ChangeCommandKind.ChangePositionAdjustment);
  });
});

describe("emptyChangeRequest", () => {
  it("stores the trade currencies that the form displays by default", () => {
    expect(emptyChangeRequest(ChangeCommandKind.ChangeTrade, "USD")).toEqual(
      expect.objectContaining({ side: "buy", grossCurrency: "USD", feeCurrency: "USD" }),
    );
  });

  it("uses the provided household or account currency instead of CNY", () => {
    expect(emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded, "SGD").currency).toBe("SGD");
    expect(emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded, "USD").currency).toBe("USD");
    expect(emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded, "")).not.toEqual(
      expect.objectContaining({ currency: "CNY" }),
    );
  });
});
