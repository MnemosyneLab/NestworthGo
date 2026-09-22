import { displayEnum } from "@/lib/display";
import { addCanonical, formatAmount } from "@/lib/money";
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
  return productStateLabel(t, source.displayState);
}

export function productStateLabel(t: Translator, state: string): string {
  switch (state) {
    case "needs_info":
      return t("availableFunds.filterNeedsInfo");
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
      return displayEnum(t, "availableFunds", state);
  }
}

export function bucketForHorizon(overview: LiquidityOverviewDTO, horizonOn: string): LiquidityBucketDTO | undefined {
  return (overview.buckets ?? []).find((bucket) => bucket.horizonOn === horizonOn);
}

export function resultForHorizon(source: LiquiditySourceDTO, horizonOn: string): BucketResultDTO | undefined {
  return (source.bucketResults ?? []).find((result) => result.horizonOn === horizonOn);
}

export function displayMoney(value: MoneyView | null | undefined, unknown: string): string {
  return formatKnownOrUnknown(value, unknown);
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
      return !source.excluded && (source.displayState === "locked" || (!needsInfo && !result?.selectedRoute));
    case "needs_info":
      return needsInfo;
    case "excluded":
      return source.excluded;
    default:
      return true;
  }
}

export function cashVersusProceeds(
  sources: LiquiditySourceDTO[],
  horizonOn: string,
  unknown: string,
  zeroCurrency: string,
  currencyMode: CurrencyMode = "base",
): { cash: string; proceeds: string } {
  const cash = { known: true, amounts: new Map<string, string>() };
  const proceeds = { known: true, amounts: new Map<string, string>() };
  for (const source of sources) {
    if (source.excluded) continue;
    const result = resultForHorizon(source, horizonOn);
    const action = result?.selectedRoute?.actionRequired ?? source.normalRoute?.actionRequired;
    const isCash = action ? action === "none" || action === "withdraw" : source.sourceRef.kind !== "holding";
    const group = isCash ? cash : proceeds;
    // Partial results may also have unknown alternative routes. Do not infer
    // completeness from the presence of one known native amount.
    if (!result || result.status !== "complete") group.known = false;
    if (!result?.selectedRoute) continue;
    const net = currencyMode === "native" ? result.netNative : result.netBase;
    if (!net) {
      group.known = false;
      continue;
    }
    group.amounts.set(net.currency, addCanonical(group.amounts.get(net.currency) ?? "0", net.amount));
  }
  const render = (group: typeof cash) => {
    if (!group.known) return unknown;
    if (group.amounts.size === 0) {
      const currencies = currencyMode === "native"
        ? [...new Set(sources.filter((source) => !source.excluded).map((source) => source.nativeCurrency).filter(Boolean))].sort()
        : [zeroCurrency];
      return (currencies.length ? currencies : [zeroCurrency]).map((currency) => formatAmount("0", currency)).join(" · ");
    }
    return [...group.amounts].sort(([a], [b]) => a.localeCompare(b))
      .map(([currency, amount]) => formatAmount(amount, currency)).join(" · ");
  };
  return { cash: render(cash), proceeds: render(proceeds) };
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

export function moneyText(value: MoneyView | null | undefined, unknown: string): string {
  return formatKnownOrUnknown(value, unknown);
}
