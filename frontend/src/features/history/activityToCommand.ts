import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { ChangeCommandKind, type ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";

/** Restore editable facts from persisted historical evidence. Effects carry
 * deltas, so absolute value updates must use their recorded resulting value.
 * Missing legacy observations stay empty and require explicit user input.
 */
export function activityToInitialCommand(activity: ActivityDTO): ChangeCommandRequest {
  const effects = activity.effects ?? [];
  const byRole = (role: string) => effects.find((effect) => effect.role === role);
  const note = activity.note ?? null;

  switch (activity.kind) {
    case "cash_in":
    case "cash_out": {
      const effect = effects[0];
      return {
        kind: activity.kind === "cash_in" ? ChangeCommandKind.ChangeMoneyAdded : ChangeCommandKind.ChangeMoneyRemoved,
        accountId: effect?.accountId ?? "",
        amount: effect?.money?.amount ?? "",
        currency: effect?.money?.currency ?? "USD",
        reason: activity.reason ?? "",
        note,
        added: true,
      };
    }

    case "cash_dividend": {
      const effect = effects[0];
      return {
        kind: ChangeCommandKind.ChangeCashDividend,
        holdingId: activity.dividendDetail?.holdingId ?? "",
        amount: activity.dividendDetail?.amount?.amount ?? effect?.money?.amount ?? "",
        currency: activity.dividendDetail?.amount?.currency ?? effect?.money?.currency ?? "USD",
        note,
        added: true,
      };
    }

    case "value_update": {
      const effect = effects[0];
      const observations = (activity.resulting ?? []).filter(view =>
        view.target === "account_value" && view.accountId && view.amount !== undefined && view.amount !== "" && view.currency &&
        (!effect || (view.accountId === effect.accountId && view.target === effect.target && view.currency === effect.money?.currency)),
      );
      const observation = observations.length === 1 ? observations[0] : undefined;
      return {
        kind: ChangeCommandKind.ChangeValueUpdate,
        accountId: observation?.accountId ?? effect?.accountId ?? "",
        newValue: observation?.amount ?? "",
        newValueCurrency: observation?.currency ?? effect?.money?.currency ?? "USD",
        reason: activity.reason ?? "",
        note,
        added: true,
      };
    }

    case "cash_transfer": {
      const from = byRole("transfer_from");
      const to = byRole("transfer_to");
      return {
        kind: ChangeCommandKind.ChangeCashTransfer,
        fromAccountId: from?.accountId ?? "",
        toAccountId: to?.accountId ?? "",
        sent: from?.money?.amount ?? "",
        sentCurrency: from?.money?.currency ?? "USD",
        received: to?.money?.amount ?? "",
        receivedCurrency: to?.money?.currency ?? "USD",
        fee: byRole("fee")?.money?.amount ?? "",
        feeCurrency: byRole("fee")?.money?.currency ?? from?.money?.currency ?? "USD",
        note,
        added: true,
      };
    }

    case "fx_conversion": {
      const from = byRole("transfer_from");
      const to = byRole("transfer_to");
      const fee = byRole("fee");
      return {
        kind: ChangeCommandKind.ChangeFXConversion,
        accountId: from?.accountId ?? to?.accountId ?? "",
        sold: from?.money?.amount ?? "",
        soldCurrency: from?.money?.currency ?? "USD",
        bought: to?.money?.amount ?? "",
        boughtCurrency: to?.money?.currency ?? "",
        fee: fee?.money?.amount ?? "",
        feeCurrency: fee?.money?.currency ?? "",
        note,
        added: true,
      };
    }

    case "position_transfer": {
      if (effects.length >= 2) {
        const from = byRole("transfer_from");
        const to = byRole("transfer_to");
        return {
          kind: ChangeCommandKind.ChangePositionTransfer,
          fromHoldingId: from?.holdingId ?? "",
          toHoldingId: to?.holdingId ?? "",
          quantity: from?.quantity ?? to?.quantity ?? "",
          note,
          added: true,
        };
      }
      const effect = byRole("quantity") ?? effects[0];
      return {
        kind: ChangeCommandKind.ChangePositionAdjustment,
        holdingId: effect?.holdingId ?? "",
        quantity: effect?.quantity ?? "",
        unitCost: effect?.costUnitPrice ?? "",
        note,
        added: effect?.direction !== "removed",
      };
    }

    case "buy":
    case "sell": {
      const detail = activity.tradeDetail;
      const principal = byRole("principal");
      const fee = byRole("fee");
      return {
        kind: ChangeCommandKind.ChangeTrade,
        settlementAccountId: principal?.accountId ?? "",
        holdingId: detail?.holdingId ?? "",
        instrumentId: detail?.instrumentId ?? "",
        side: detail?.side ?? (activity.kind === "buy" ? "buy" : "sell"),
        quantity: detail?.quantity ?? "",
        gross: detail?.gross?.amount ?? "",
        grossCurrency: detail?.gross?.currency ?? "USD",
        fee: detail?.fee?.amount ?? fee?.money?.amount ?? "",
        feeCurrency: detail?.fee?.currency ?? fee?.money?.currency ?? "",
        note,
        added: true,
      };
    }

    case "debt_draw":
    case "debt_payment": {
      const debt = byRole("debt");
      const principal = byRole("principal");
      const fee = byRole("fee");
      return {
        kind: activity.kind === "debt_draw" ? ChangeCommandKind.ChangeDebtDraw : ChangeCommandKind.ChangeDebtPayment,
        debtAccountId: debt?.accountId ?? "",
        cashAccountId: principal?.accountId ?? "",
        principal: debt?.money?.amount ?? principal?.money?.amount ?? "",
        principalCurrency: debt?.money?.currency ?? principal?.money?.currency ?? "USD",
        interestOrFee: fee?.money?.amount ?? "",
        interestOrFeeCurrency: fee?.money?.currency ?? "",
        note,
        added: true,
      };
    }

    default:
      return emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded, "");
  }
}

export function emptyChangeRequest(kind: ChangeCommandKind, currency: string): ChangeCommandRequest {
  const request: ChangeCommandRequest = {
    kind,
    added: true,
    note: null,
    reason: kind === ChangeCommandKind.ChangeMoneyAdded || kind === ChangeCommandKind.ChangeMoneyRemoved || kind === ChangeCommandKind.ChangeValueUpdate ? "other" : "",
  };

  switch (kind) {
    case ChangeCommandKind.ChangeMoneyAdded:
    case ChangeCommandKind.ChangeMoneyRemoved:
    case ChangeCommandKind.ChangeCashDividend:
      return { ...request, currency };
    case ChangeCommandKind.ChangeValueUpdate:
      return { ...request, newValueCurrency: currency };
    case ChangeCommandKind.ChangeCashTransfer:
      return { ...request, sentCurrency: currency, receivedCurrency: currency };
    case ChangeCommandKind.ChangeFXConversion:
      return { ...request, soldCurrency: currency, boughtCurrency: "", feeCurrency: currency };
    case ChangeCommandKind.ChangeTrade:
      return { ...request, side: "buy", grossCurrency: currency, feeCurrency: currency };
    case ChangeCommandKind.ChangeDebtDraw:
    case ChangeCommandKind.ChangeDebtPayment:
      return { ...request, principalCurrency: currency, interestOrFeeCurrency: currency };
    default:
      return request;
  }
}

/** Explicitly ordinary records only; never infer eligibility from effects. */
export function activityToRepeatDraft(activity: ActivityDTO): ChangeCommandRequest | null {
  if (activity.productContext || activity.dividendDetail || activity.tradeDetail || activity.reversesActivityId || activity.correctionGroupId) return null;
  if (!((activity.kind === "cash_in" && ["income", "interest", "contribution", "gift", "other"].includes(activity.reason)) || (activity.kind === "cash_out" && ["expense", "fee", "tax", "interest", "other"].includes(activity.reason)))) return null;
  const effect = activity.effects?.[0];
  if (activity.effects?.length !== 1 || !effect?.accountId || !effect.money || effect.holdingId) return null;
  return { kind: activity.kind === "cash_in" ? ChangeCommandKind.ChangeMoneyAdded : ChangeCommandKind.ChangeMoneyRemoved, accountId: effect.accountId, amount: effect.money.amount, currency: effect.money.currency, reason: activity.reason, note: activity.note ?? null, added: true };
}
