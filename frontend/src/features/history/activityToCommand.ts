import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

/**
 * activityToInitialCommand reconstructs a best-effort ChangeCommandRequest
 * ("kind" + field map + "added" flag) from a previously recorded
 * ActivityDTO, for the Fix flow's "pre-fill the Record change form with
 * the original change" UX.
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
export function activityToInitialCommand(activity: ActivityDTO): { kind: string; fields: Record<string, string>; added: boolean } {
  const effects = activity.effects ?? [];
  const byRole = (role: string) => effects.find((effect) => effect.role === role);

  switch (activity.kind) {
    case "cash_in":
    case "cash_out": {
      const effect = effects[0];
      return {
        kind: activity.kind === "cash_in" ? "money_added" : "money_removed",
        fields: {
          accountId: effect?.accountId ?? "",
          amount: effect?.money?.amount ?? "",
          currency: effect?.money?.currency ?? "USD",
          reason: activity.reason ?? "",
          note: activity.note ?? "",
        },
        added: true,
      };
    }

    case "value_update": {
      const effect = effects[0];
      return {
        kind: "value_update",
        fields: {
          accountId: effect?.accountId ?? "",
          newValue: effect?.money?.amount ?? "",
          newValueCurrency: effect?.money?.currency ?? "USD",
          reason: activity.reason ?? "",
          note: activity.note ?? "",
        },
        added: true,
      };
    }

    case "cash_transfer": {
      const from = byRole("transfer_from");
      const to = byRole("transfer_to");
      return {
        kind: "cash_transfer",
        fields: {
          fromAccountId: from?.accountId ?? "",
          toAccountId: to?.accountId ?? "",
          sent: from?.money?.amount ?? "",
          sentCurrency: from?.money?.currency ?? "USD",
          received: to?.money?.amount ?? "",
          receivedCurrency: to?.money?.currency ?? "USD",
          note: activity.note ?? "",
        },
        added: true,
      };
    }

    case "fx_conversion": {
      const from = byRole("transfer_from");
      const to = byRole("transfer_to");
      const fee = byRole("fee");
      return {
        kind: "fx_conversion",
        fields: {
          accountId: from?.accountId ?? to?.accountId ?? "",
          sold: from?.money?.amount ?? "",
          soldCurrency: from?.money?.currency ?? "USD",
          bought: to?.money?.amount ?? "",
          boughtCurrency: to?.money?.currency ?? "USD",
          fee: fee?.money?.amount ?? "",
          feeCurrency: fee?.money?.currency ?? "",
          note: activity.note ?? "",
        },
        added: true,
      };
    }

    case "position_transfer": {
      // The same ActivityKind backs two different commands: a two-holding
      // transfer (two effects, transfer_from/transfer_to) and a single-
      // holding adjustment (one effect, role "quantity"). Distinguish by
      // effect count, mirroring domain's buildPositionTransfer vs.
      // buildPositionAdjustment.
      if (effects.length >= 2) {
        const from = byRole("transfer_from");
        const to = byRole("transfer_to");
        return {
          kind: "position_transfer",
          fields: {
            fromHoldingId: from?.holdingId ?? "",
            toHoldingId: to?.holdingId ?? "",
            quantity: from?.quantity ?? to?.quantity ?? "",
            note: activity.note ?? "",
          },
          added: true,
        };
      }
      const effect = byRole("quantity") ?? effects[0];
      return {
        kind: "position_adjustment",
        fields: {
          holdingId: effect?.holdingId ?? "",
          quantity: effect?.quantity ?? "",
          unitCost: effect?.costUnitPrice ?? "",
          note: activity.note ?? "",
        },
        added: effect?.direction !== "removed",
      };
    }

    case "buy":
    case "sell": {
      const detail = activity.tradeDetail;
      const principal = byRole("principal");
      const fee = byRole("fee");
      return {
        kind: "trade",
        fields: {
          settlementAccountId: principal?.accountId ?? "",
          instrumentId: detail?.instrumentId ?? "",
          side: detail?.side ?? (activity.kind === "buy" ? "buy" : "sell"),
          quantity: detail?.quantity ?? "",
          gross: detail?.gross?.amount ?? "",
          grossCurrency: detail?.gross?.currency ?? "USD",
          fee: detail?.fee?.amount ?? fee?.money?.amount ?? "",
          feeCurrency: detail?.fee?.currency ?? fee?.money?.currency ?? "",
          note: activity.note ?? "",
        },
        added: true,
      };
    }

    case "debt_draw":
    case "debt_payment": {
      const debt = byRole("debt");
      const principal = byRole("principal");
      const fee = byRole("fee");
      return {
        kind: activity.kind,
        fields: {
          debtAccountId: debt?.accountId ?? "",
          cashAccountId: principal?.accountId ?? "",
          principal: debt?.money?.amount ?? principal?.money?.amount ?? "",
          principalCurrency: debt?.money?.currency ?? principal?.money?.currency ?? "USD",
          interestOrFee: fee?.money?.amount ?? "",
          interestOrFeeCurrency: fee?.money?.currency ?? "",
          note: activity.note ?? "",
        },
        added: true,
      };
    }

    default:
      return { kind: "money_added", fields: { currency: "USD" }, added: true };
  }
}
