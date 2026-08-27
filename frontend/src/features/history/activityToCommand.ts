import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { ChangeCommandKind, type ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";

/**
 * activityToInitialCommand reconstructs a best-effort ChangeCommandRequest
 * from a previously recorded ActivityDTO, for the Fix flow's "pre-fill the
 * Record change form with the original change" UX.
 *
 * This is intentionally best-effort, not a lossless inverse of
 * history.ChangeCommandRequest.ToCommand: domain.Activity only stores the
 * *effects* a change produced, not the original command, so some
 * distinctions the command made (e.g. which side of a symmetric transfer
 * the user entered first) cannot always be recovered byte-for-byte. Every
 * field this function fills in is still checked by RecordChange/FixChange
 * server-side, so a wrong guess here can never corrupt data — the user
 * sees exactly what they are about to submit before confirming.
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

    case "value_update": {
      const effect = effects[0];
      return {
        kind: ChangeCommandKind.ChangeValueUpdate,
        accountId: effect?.accountId ?? "",
        newValue: effect?.money?.amount ?? "",
        newValueCurrency: effect?.money?.currency ?? "USD",
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
        boughtCurrency: to?.money?.currency ?? "USD",
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
      return { ...request, currency };
    case ChangeCommandKind.ChangeValueUpdate:
      return { ...request, newValueCurrency: currency };
    case ChangeCommandKind.ChangeCashTransfer:
      return { ...request, sentCurrency: currency, receivedCurrency: currency };
    case ChangeCommandKind.ChangeFXConversion:
      return { ...request, soldCurrency: currency, boughtCurrency: currency, feeCurrency: currency };
    case ChangeCommandKind.ChangeTrade:
      return { ...request, side: "buy", grossCurrency: currency, feeCurrency: currency };
    case ChangeCommandKind.ChangeDebtDraw:
    case ChangeCommandKind.ChangeDebtPayment:
      return { ...request, principalCurrency: currency, interestOrFeeCurrency: currency };
    default:
      return request;
  }
}
