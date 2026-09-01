import { displayEnum } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { activitySentence } from "@/features/history/activitySentence";
import type { ActivityDTO, ActivityEffectDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type Translator = (key: string, options?: Record<string, unknown>) => string;

const SPECIALIZED_KINDS = new Set([
  "buy", "sell", "cash_in", "cash_out", "cash_dividend", "cash_transfer", "fx_conversion",
  "position_transfer", "value_update", "debt_draw", "debt_payment", "reversal",
]);

function moneyText(amount?: string, currency?: string): string {
  return amount ? formatAmount(amount, currency) : "";
}

function effectLine(
  t: Translator,
  effect: ActivityEffectDTO,
  accounts: Map<string, string>,
  instruments: Map<string, string>,
  holdings: Map<string, string>,
): string {
  const target = effect.holdingId
    ? (holdings.get(effect.holdingId) ?? t("history.unknownHolding"))
    : effect.accountId
      ? (accounts.get(effect.accountId) ?? t("history.unknownAccount"))
      : effect.instrumentId
        ? (instruments.get(effect.instrumentId) ?? t("history.unknownInstrument"))
        : "";
  const direction = effect.direction === "added" ? "Added" : effect.direction === "removed" ? "Removed" : "";
  const amount = effect.money
    ? `${t(direction ? `history.detailAmount${direction}` : "history.amount")}: ${moneyText(effect.money.amount, effect.money.currency)}`
    : "";
  const quantity = effect.quantity
    ? `${t(direction ? `history.detailQuantity${direction}` : "history.detailQuantity")}: ${formatAmount(effect.quantity)}`
    : "";
  const itemDirection = !amount && !quantity && direction ? t(`history.detailItem${direction}`) : "";
  return [target, amount, quantity, itemDirection].filter(Boolean).join(" · ") || t("history.detailUnknownImpact");
}

function accountName(t: Translator, effect: ActivityEffectDTO | undefined, accounts: Map<string, string>): string {
  return (effect?.accountId ? accounts.get(effect.accountId) : undefined) ?? t("history.unknownAccount");
}

function holdingName(t: Translator, effect: ActivityEffectDTO | undefined, holdings: Map<string, string>): string {
  return (effect?.holdingId ? holdings.get(effect.holdingId) : undefined) ?? t("history.unknownHolding");
}

function ActivityDetailBody({
  activity, originalActivity, originalActivityLoading, timezone, accounts, instruments, holdings, t,
}: {
  activity: ActivityDTO;
  originalActivity?: ActivityDTO;
  originalActivityLoading?: boolean;
  timezone?: string;
  accounts: Map<string, string>;
  instruments: Map<string, string>;
  holdings: Map<string, string>;
  t: Translator;
}) {
  const sentence = activitySentence(t, activity, accounts, instruments, holdings);
  const detail = activity.tradeDetail;
  const effects = activity.effects ?? [];
  const byRole = (role: string) => effects.find((effect) => effect.role === role);
  const from = byRole("transfer_from");
  const to = byRole("transfer_to");
  const fee = byRole("fee");
  const debt = byRole("debt");
  const principal = byRole("principal");
  const quantityEffect = byRole("quantity") ?? effects[0];
  const firstMoney = effects.find((effect) => effect.money);

  return (
    <div className="flex flex-col gap-4 text-sm">
      <p><span className="text-muted-foreground">{t("history.detailKind")}</span><span className="ml-2">{displayEnum(t, "history.kind", activity.kind)}</span></p>
      <p><span className="text-muted-foreground">{t("history.detailLocalDateTime")}</span><span className="ml-2">{formatTimestamp(activity.effectiveAt, timezone)}</span></p>
      {timezone && <p><span className="text-muted-foreground">{t("history.detailTimezone")}</span><span className="ml-2">{timezone}</span></p>}
      <p><span className="text-muted-foreground">{t("history.detailSentence")}</span><span className="ml-2">{sentence}</span></p>

      {(activity.kind === "buy" || activity.kind === "sell") && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailAccount")}: {accountName(t, principal, accounts)}</p>
          <p>{t("history.detailInstrument")}: {(detail?.instrumentId ? instruments.get(detail.instrumentId) : undefined) ?? t("history.unknownInstrument")}</p>
          <p>{t("history.detailSide")}: {displayEnum(t, "history", detail?.side ?? activity.kind)}</p>
          {detail?.quantity && <p>{t("history.detailQuantity")}: {formatAmount(detail.quantity)}</p>}
          {detail?.unitPrice && <p>{t("history.detailUnitPrice")}: {formatAmount(detail.unitPrice, detail.gross?.currency)}</p>}
          {detail?.gross && <p>{t("history.detailGross")}: {moneyText(detail.gross.amount, detail.gross.currency)}</p>}
          {detail?.fee && <p>{t("history.fee")}: {moneyText(detail.fee.amount, detail.fee.currency)}</p>}
        </div>
      )}
      {(activity.kind === "cash_in" || activity.kind === "cash_out") && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailAccount")}: {accountName(t, firstMoney, accounts)}</p>
          {firstMoney?.money && <p>{t("history.amount")}: {moneyText(firstMoney.money.amount, firstMoney.money.currency)}</p>}
          {activity.reason && <p>{t("history.reasonLabel")}: {displayEnum(t, "history.reason", activity.reason)}</p>}
        </div>
      )}
      {activity.kind === "cash_dividend" && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailAccount")}: {accountName(t, effects[0], accounts)}</p>
          <p>{t("history.detailInstrument")}: {(activity.dividendDetail?.instrumentId ? instruments.get(activity.dividendDetail.instrumentId) : undefined) ?? t("history.unknownInstrument")}</p>
          {activity.dividendDetail?.amount && <p>{t("history.amount")}: {moneyText(activity.dividendDetail.amount.amount, activity.dividendDetail.amount.currency)}</p>}
        </div>
      )}
      {activity.kind === "cash_transfer" && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailFrom")}: {accountName(t, from, accounts)} {from?.money ? moneyText(from.money.amount, from.money.currency) : ""}</p>
          <p>{t("history.detailTo")}: {accountName(t, to, accounts)} {to?.money ? moneyText(to.money.amount, to.money.currency) : ""}</p>
          {fee?.money && <p>{t("history.fee")}: {moneyText(fee.money.amount, fee.money.currency)}</p>}
        </div>
      )}
      {activity.kind === "fx_conversion" && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailSold")}: {from?.money ? moneyText(from.money.amount, from.money.currency) : ""}</p>
          <p>{t("history.detailBought")}: {to?.money ? moneyText(to.money.amount, to.money.currency) : ""}</p>
          {activity.transactionFxRate && <p>{t("history.detailRate")}: {activity.transactionFxRate}</p>}
          {fee?.money && <p>{t("history.fee")}: {moneyText(fee.money.amount, fee.money.currency)}</p>}
        </div>
      )}
      {activity.kind === "position_transfer" && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailChangeType")}: {t(from && to ? "history.detailPositionTransfer" : "history.detailPositionAdjustment")}</p>
          {from && to ? (
            <>
              <p>{t("history.detailFromHolding")}: {holdingName(t, from, holdings)}</p>
              <p>{t("history.detailToHolding")}: {holdingName(t, to, holdings)}</p>
              {(from.quantity || to.quantity) && <p>{t("history.detailQuantity")}: {formatAmount(from.quantity ?? to.quantity ?? "")}</p>}
            </>
          ) : (
            <>
              <p>{t("history.detailHolding")}: {holdingName(t, quantityEffect, holdings)}</p>
              <p>{t("history.detailAdjustment")}: {t(quantityEffect?.direction === "removed" ? "history.detailQuantityRemoved" : "history.detailQuantityAdded")}</p>
              {quantityEffect?.quantity && <p>{t("history.detailQuantity")}: {formatAmount(quantityEffect.quantity)}</p>}
            </>
          )}
        </div>
      )}
      {(activity.kind === "debt_draw" || activity.kind === "debt_payment") && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailDebtAccount")}: {accountName(t, debt, accounts)}</p>
          <p>{t("history.detailCashAccount")}: {accountName(t, principal, accounts)}</p>
          {principal?.money && <p>{t("history.detailPrincipal")}: {moneyText(principal.money.amount, principal.money.currency)}</p>}
          {fee?.money && <p>{t("history.detailInterestOrFee")}: {moneyText(fee.money.amount, fee.money.currency)}</p>}
        </div>
      )}
      {activity.kind === "value_update" && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailAccount")}: {accountName(t, effects[0], accounts)}</p>
          {effects[0]?.money && <p>{t("history.detailDelta")}: {moneyText(effects[0].money.amount, effects[0].money.currency)}</p>}
          {activity.reason && <p>{t("history.reasonLabel")}: {displayEnum(t, "history.reason", activity.reason)}</p>}
        </div>
      )}
      {activity.kind === "reversal" && (
        <div className="flex flex-col gap-2 rounded-md border border-border p-3">
          <p>{t("history.detailOriginalActivity")}: {originalActivityLoading ? t("history.detailOriginalActivityLoading") : originalActivity ? activitySentence(t, originalActivity, accounts, instruments, holdings) : t("history.detailOriginalActivityUnavailable")}</p>
          <p>{t("history.detailReversalDirection")}: {t("history.detailOppositeDirection")}</p>
          <div>
            <p>{t("history.detailAffectedItems")}</p>
            <ul className="mt-1 flex list-disc flex-col gap-1 pl-5">
              {effects.map((effect, index) => <li key={effect.id || index}>{effectLine(t, effect, accounts, instruments, holdings)}</li>)}
            </ul>
          </div>
        </div>
      )}
      {!SPECIALIZED_KINDS.has(activity.kind) && (
        <ul className="flex list-disc flex-col gap-1 rounded-md border border-border p-3 pl-8">
          {effects.map((effect, index) => <li key={effect.id || index}>{effectLine(t, effect, accounts, instruments, holdings)}</li>)}
        </ul>
      )}
      {activity.note && <p><span className="text-muted-foreground">{t("history.detailNote")}</span><span className="ml-2">{activity.note}</span></p>}
      <div className="flex flex-wrap gap-2">
        {activity.reversesActivityId && <Badge variant="secondary">{t("history.reversal")}</Badge>}
        {activity.correctionGroupId && !activity.reversesActivityId && <Badge variant="secondary">{t("history.corrected")}</Badge>}
      </div>
    </div>
  );
}

export function ActivityDetailSheet({
  activity, originalActivity, originalActivityLoading, timezone, accounts, instruments, holdings = new Map(), onClose, t,
}: {
  activity: ActivityDTO | null;
  originalActivity?: ActivityDTO;
  originalActivityLoading?: boolean;
  timezone?: string;
  accounts: Map<string, string>;
  instruments: Map<string, string>;
  holdings?: Map<string, string>;
  onClose: () => void;
  t: Translator;
}) {
  return (
    <Sheet open={activity !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent>
        <SheetHeader><SheetTitle>{t("common.details")}</SheetTitle></SheetHeader>
        <div className="overflow-y-auto">
          {activity && <ActivityDetailBody activity={activity} originalActivity={originalActivity} originalActivityLoading={originalActivityLoading} timezone={timezone} accounts={accounts} instruments={instruments} holdings={holdings} t={t} />}
        </div>
      </SheetContent>
    </Sheet>
  );
}
