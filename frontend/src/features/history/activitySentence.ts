import { displayEnum } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import type { ActivityDTO, ActivityEffectDTO, EndpointViewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type ActivityWithResulting = ActivityDTO & { resulting?: EndpointViewDTO[] | null };

function activityResultingAmount(activity: ActivityWithResulting): string {
  const endpoint = activity.resulting?.[0];
  if (!endpoint?.amount) {
    return "";
  }
  return formatAmount(endpoint.amount, endpoint.currency);
}

type Translator = (key: string, options?: Record<string, unknown>) => string;

function moneyLabel(effect: ActivityEffectDTO | undefined): string {
  if (!effect?.money) {
    return "";
  }
  return formatAmount(effect.money.amount, effect.money.currency);
}

function byRole(effects: ActivityEffectDTO[], role: string): ActivityEffectDTO | undefined {
  return effects.find((effect) => effect.role === role);
}

/**
 * activitySentence turns a recorded Activity into one user-language line,
 * e.g. "Added $1,000.00 to Checking". Internal kind codes stay out of the
 * sentence; missing names fall back to localized placeholders or the kind
 * label when there is not enough effect data to form a sentence.
 */
export function activitySentence(
  t: Translator,
  activity: ActivityDTO,
  accounts: Map<string, string>,
  instruments: Map<string, string>,
): string {
  const effects = activity.effects ?? [];
  const accountName = (effect: ActivityEffectDTO | undefined) =>
    (effect?.accountId ? accounts.get(effect.accountId) : undefined) || t("history.unknownAccount");
  const instrumentName = (id: string | null | undefined) =>
    (id ? instruments.get(id) : undefined) || t("history.unknownInstrument");

  const core = coreSentence(t, activity, effects, accountName, instrumentName);
  const fee = feeLabel(activity, effects);
  const sentence = fee && !activity.reversesActivityId ? t("history.sentence.withFee", { sentence: core, fee }) : core;
  const skipReason = activity.kind === "buy" || activity.kind === "sell";
  const reason = activity.reason ? displayEnum(t, "history.reason", activity.reason) : "";
  if (!skipReason && reason && activity.reason !== "other") {
    return t("history.sentence.withReason", { sentence, reason });
  }
  return sentence;
}

function feeLabel(activity: ActivityDTO, effects: ActivityEffectDTO[]): string {
  if ((activity.kind === "buy" || activity.kind === "sell") && activity.tradeDetail?.fee) {
    return formatAmount(activity.tradeDetail.fee.amount, activity.tradeDetail.fee.currency);
  }
  if (activity.kind === "fx_conversion" || activity.kind === "debt_payment" || activity.kind === "cash_transfer") {
    return moneyLabel(byRole(effects, "fee"));
  }
  return "";
}

function coreSentence(
  t: Translator,
  activity: ActivityDTO,
  effects: ActivityEffectDTO[],
  accountName: (effect: ActivityEffectDTO | undefined) => string,
  instrumentName: (id: string | null | undefined) => string,
): string {
  if (activity.reversesActivityId) {
    return t("history.sentence.reversal");
  }

  const firstMoney = effects.find((effect) => effect.money) ?? effects[0];

  switch (activity.kind) {
    case "cash_in": {
      const amount = moneyLabel(firstMoney);
      if (!amount) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      return t("history.sentence.added", { amount, account: accountName(firstMoney) });
    }
    case "cash_out": {
      const amount = moneyLabel(firstMoney);
      if (!amount) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      return t("history.sentence.removed", { amount, account: accountName(firstMoney) });
    }
    case "cash_transfer": {
      const from = byRole(effects, "transfer_from");
      const to = byRole(effects, "transfer_to");
      const amount = moneyLabel(from) || moneyLabel(to);
      if (!amount) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      return t("history.sentence.transferred", { amount, from: accountName(from), to: accountName(to) });
    }
    case "fx_conversion": {
      const from = byRole(effects, "transfer_from");
      const to = byRole(effects, "transfer_to");
      const sold = moneyLabel(from);
      const bought = moneyLabel(to);
      if (!sold || !bought) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      return t("history.sentence.converted", { sold, bought, account: accountName(from ?? to) });
    }
    case "value_update": {
      const resulting = activityResultingAmount(activity);
      if (resulting) {
        return t("history.sentence.updatedValue", { amount: resulting, account: accountName(firstMoney) });
      }
      const amount = moneyLabel(firstMoney);
      if (!amount) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      const canonical = firstMoney?.money?.amount ?? "";
      const increased = !canonical.startsWith("-");
      const delta = formatAmount(canonical.startsWith("-") ? canonical.slice(1) : canonical, firstMoney?.money?.currency);
      const key = increased ? "history.sentence.updatedValueIncreased" : "history.sentence.updatedValueDecreased";
      return t(key, { delta, account: accountName(firstMoney) });
    }
    case "buy":
    case "sell": {
      const detail = activity.tradeDetail;
      const principal = byRole(effects, "principal");
      const amount = detail?.gross ? formatAmount(detail.gross.amount, detail.gross.currency) : moneyLabel(principal);
      const quantity = detail?.quantity ? formatAmount(detail.quantity) : "";
      const instrument = instrumentName(detail?.instrumentId ?? principal?.instrumentId);
      const account = accountName(principal ?? effects.find((effect) => effect.accountId));
      if (!amount || !quantity) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      const key = detail?.side === "sell" || activity.kind === "sell" ? "history.sentence.sold" : "history.sentence.bought";
      return t(key, { quantity, instrument, amount, account });
    }
    case "position_transfer": {
      if (effects.length >= 2) {
        const from = byRole(effects, "transfer_from");
        const to = byRole(effects, "transfer_to");
        const quantity = from?.quantity || to?.quantity ? formatAmount(from?.quantity ?? to?.quantity ?? "") : "";
        if (!quantity) {
          return displayEnum(t, "history.kind", activity.kind);
        }
        const fromLabel = from?.accountId ? accountName(from) : instrumentName(from?.instrumentId);
        const toLabel = to?.accountId ? accountName(to) : instrumentName(to?.instrumentId);
        return t("history.sentence.movedPosition", { quantity, from: fromLabel, to: toLabel });
      }
      const effect = byRole(effects, "quantity") ?? effects[0];
      const quantity = effect?.quantity ? formatAmount(effect.quantity) : "";
      if (!quantity) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      const key = effect?.direction === "removed" ? "history.sentence.removedQuantity" : "history.sentence.addedQuantity";
      return t(key, { quantity, instrument: instrumentName(effect?.instrumentId) });
    }
    case "debt_draw": {
      const amount = moneyLabel(byRole(effects, "debt") ?? firstMoney);
      if (!amount) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      return t("history.sentence.drewDebt", { amount, account: accountName(byRole(effects, "debt") ?? firstMoney) });
    }
    case "debt_payment": {
      const amount = moneyLabel(byRole(effects, "debt") ?? firstMoney);
      if (!amount) {
        return displayEnum(t, "history.kind", activity.kind);
      }
      return t("history.sentence.paidDebt", { amount, account: accountName(byRole(effects, "debt") ?? firstMoney) });
    }
    default:
      return displayEnum(t, "history.kind", activity.kind);
  }
}
