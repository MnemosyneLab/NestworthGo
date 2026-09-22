import { displayEnum } from "@/lib/display";
import { addCanonical, formatAmount } from "@/lib/money";
import { moneyText } from "@/features/liquidity/ProductSheets";
import type {
  BucketResultDTO,
  LiquidityBucketDTO,
  LiquidityOverviewDTO,
  LiquiditySourceDTO,
} from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import type { MoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export type SourceFilter = "all" | "available" | "locked" | "needs_info" | "excluded";
export type CurrencyMode = "base" | "native";

export function addCivilDays(isoDate: string, days: number): string {
  const [year, month, day] = isoDate.split("-").map(Number);
  const date = new Date(Date.UTC(year, month - 1, day + days));
  return date.toISOString().slice(0, 10);
}

type Translator = (key: string, options?: Record<string, unknown>) => string;

export function completenessLabel(t: Translator, status: string): string {
  if (status === "complete") {
    return t("availableFunds.complete");
  }
  if (status === "partial") {
    return t("availableFunds.partial");
  }
  return t("availableFunds.unavailable");
}

export function actionRequiredLabel(t: Translator, action: string | undefined): string {
  switch (action) {
    case "redeem":
      return t("availableFunds.actionRedeem");
    case "sell":
      return t("availableFunds.actionSell");
    case "withdraw":
      return t("availableFunds.actionWithdraw");
    case "early_withdrawal":
      return t("availableFunds.actionEarly");
    case "none":
    case "":
    case undefined:
      return t("availableFunds.actionNone");
    default:
      return displayEnum(t, "availableFunds", action);
  }
}

export function sourceStateLabel(t: Translator, source: LiquiditySourceDTO): string {
  if (source.excluded) {
    return t("availableFunds.filterExcluded");
  }
  switch (source.displayState) {
    case "due_unconfirmed":
      return t("availableFunds.stateDue");
    case "locked":
      return t("availableFunds.stateLocked");
    case "redeemable":
      return t("availableFunds.stateRedeemable");
    case "settled":
      return t("availableFunds.stateSettled");
    case "cancelled":
      return t("availableFunds.stateCancelled");
    default:
      return displayEnum(t, "availableFunds", source.displayState);
  }
}

export function bucketForHorizon(overview: LiquidityOverviewDTO, horizonOn: string): LiquidityBucketDTO | undefined {
  return (overview.buckets ?? []).find((bucket) => bucket.horizonOn === horizonOn);
}

export function resultForHorizon(source: LiquiditySourceDTO, horizonOn: string): BucketResultDTO | undefined {
  return (source.bucketResults ?? []).find((result) => result.horizonOn === horizonOn);
}

export function displayMoney(value: MoneyView | null | undefined, unknown: string): string {
  return moneyText(value, unknown);
}

export function formatKnownOrUnknown(value: MoneyView | null | undefined, unknown: string): string {
  if (!value) {
    return unknown;
  }
  return formatAmount(value.amount, value.currency);
}

export function sourceMatchesFilter(source: LiquiditySourceDTO, horizonOn: string, filter: SourceFilter): boolean {
  const result = resultForHorizon(source, horizonOn);
  const needsInfo = result?.status === "partial" || result?.status === "unavailable" || Boolean((source.reasons ?? []).length);
  switch (filter) {
    case "available":
      return !source.excluded && Boolean(result?.selectedRoute) && result?.status !== "unavailable";
    case "locked":
      return !source.excluded && (source.displayState === "locked" || !result?.selectedRoute);
    case "needs_info":
      return needsInfo;
    case "excluded":
      return source.excluded;
    default:
      return true;
  }
}

function accumulateGroup(
  current: MoneyView | null,
  next: MoneyView,
): { value: MoneyView | null; mixed: boolean } {
  if (!current) {
    return { value: next, mixed: false };
  }
  if (current.currency !== next.currency) {
    return { value: current, mixed: true };
  }
  return { value: { amount: addCanonical(current.amount, next.amount), currency: current.currency }, mixed: false };
}

export function cashVersusProceeds(
  sources: LiquiditySourceDTO[],
  horizonOn: string,
  unknown: string,
  zeroCurrency: string,
): { cash: string; proceeds: string } {
  let cashKnown = true;
  let proceedsKnown = true;
  let cashAmount: MoneyView | null = null;
  let proceedsAmount: MoneyView | null = null;
  for (const source of sources) {
    if (source.excluded) {
      continue;
    }
    const result = resultForHorizon(source, horizonOn);
    const action = result?.selectedRoute?.actionRequired ?? source.normalRoute?.actionRequired;
    const isCash = action ? action === "none" || action === "withdraw" : source.sourceRef.kind !== "holding";
    if (!result || result.status !== "complete") {
      if (isCash) cashKnown = false;
      else proceedsKnown = false;
    }
    if (!result?.selectedRoute) continue;
    const net = result.netBase ?? result.netNative;
    if (!net) {
      if (isCash) {
        cashKnown = false;
      } else {
        proceedsKnown = false;
      }
      continue;
    }
    if (isCash) {
      const grouped = accumulateGroup(cashAmount, net);
      cashAmount = grouped.value;
      if (grouped.mixed) {
        cashKnown = false;
      }
    } else {
      const grouped = accumulateGroup(proceedsAmount, net);
      proceedsAmount = grouped.value;
      if (grouped.mixed) {
        proceedsKnown = false;
      }
    }
  }
  return {
    cash: cashKnown ? formatKnownOrUnknown(cashAmount ?? { amount: "0", currency: zeroCurrency }, unknown) : unknown,
    proceeds: proceedsKnown ? formatKnownOrUnknown(proceedsAmount ?? { amount: "0", currency: zeroCurrency }, unknown) : unknown,
  };
}

export type Reminder = { id: string; productId?: string; text: string };

export function remindersFor(overview: LiquidityOverviewDTO, t: Translator): Reminder[] {
  const localDate = overview.localDate;
  const until = addCivilDays(localDate, 7);
  const reminders: Reminder[] = [];
  for (const source of overview.sources ?? []) {
    if (source.dueUnconfirmed) {
      reminders.push({
        id: `${source.sourceKey}-due`,
        productId: source.productId ?? undefined,
        text: t("availableFunds.reminderDue", { name: source.displayName }),
      });
      continue;
    }
    const unlockOn = source.policy?.unlockOn;
    if (!unlockOn || unlockOn < localDate || unlockOn > until) {
      continue;
    }
    reminders.push({
      id: `${source.sourceKey}-unlock`,
      productId: source.productId ?? undefined,
      text: source.productId
        ? t("availableFunds.reminderMaturity", { name: source.displayName })
        : t("availableFunds.reminderUnlock", { name: source.displayName }),
    });
  }
  return reminders;
}
