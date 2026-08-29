import { displayEnum } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { activitySentence } from "@/features/history/activitySentence";
import type { ActivityDTO, ActivityEffectDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type Translator = (key: string, options?: Record<string, unknown>) => string;

function moneyText(amount?: string, currency?: string): string {
  if (!amount) {
    return "";
  }
  return formatAmount(amount, currency);
}

function effectLine(t: Translator, effect: ActivityEffectDTO, accounts: Map<string, string>, instruments: Map<string, string>): string {
  const account = effect.accountId ? (accounts.get(effect.accountId) ?? t("history.unknownAccount")) : "";
  const instrument = effect.instrumentId ? (instruments.get(effect.instrumentId) ?? t("history.unknownInstrument")) : "";
  const amount = effect.money ? moneyText(effect.money.amount, effect.money.currency) : "";
  const quantity = effect.quantity ? formatAmount(effect.quantity) : "";
  return [effect.role, effect.direction, account, instrument, amount, quantity].filter(Boolean).join(" · ");
}

function ActivityDetailBody({
  activity,
  timezone,
  accounts,
  instruments,
  t,
}: {
  activity: ActivityDTO;
  timezone?: string;
  accounts: Map<string, string>;
  instruments: Map<string, string>;
  t: Translator;
}) {
  const sentence = activitySentence(t, activity, accounts, instruments);
  const detail = activity.tradeDetail;
  const effects = activity.effects ?? [];
  const byRole = (role: string) => effects.find((effect) => effect.role === role);
  const from = byRole("transfer_from");
  const to = byRole("transfer_to");
  const fee = byRole("fee");

  return (
    <div className="flex flex-col gap-4 text-sm">
      <p>
        <span className="text-muted-foreground">{t("history.detailKind")}</span>
        <span className="ml-2">{displayEnum(t, "history.kind", activity.kind)}</span>
      </p>
      <p>
        <span className="text-muted-foreground">{t("history.detailLocalDateTime")}</span>
        <span className="ml-2">{formatTimestamp(activity.effectiveAt, timezone)}</span>
      </p>
      {timezone && (
        <p>
          <span className="text-muted-foreground">{t("history.detailTimezone")}</span>
          <span className="ml-2">{timezone}</span>
        </p>
      )}
      <p>
        <span className="text-muted-foreground">{t("history.detailSentence")}</span>
        <span className="ml-2">{sentence}</span>
      </p>
      {(activity.kind === "buy" || activity.kind === "sell") && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailAccount")}: {accounts.get(byRole("principal")?.accountId ?? "") ?? t("history.unknownAccount")}</p>
          <p>{t("history.detailInstrument")}: {(detail?.instrumentId ? instruments.get(detail.instrumentId) : undefined) ?? t("history.unknownInstrument")}</p>
          <p>{t("history.detailSide")}: {displayEnum(t, "history", detail?.side ?? activity.kind)}</p>
          {detail?.quantity && <p>{t("history.detailQuantity")}: {formatAmount(detail.quantity)}</p>}
          {detail?.unitPrice && <p>{t("history.detailUnitPrice")}: {formatAmount(detail.unitPrice, detail.gross?.currency)}</p>}
          {detail?.gross && <p>{t("history.detailGross")}: {moneyText(detail.gross.amount, detail.gross.currency)}</p>}
          {detail?.fee && <p>{t("history.fee")}: {moneyText(detail.fee.amount, detail.fee.currency)}</p>}
        </div>
      )}
      {activity.kind === "cash_transfer" && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailFrom")}: {accounts.get(from?.accountId ?? "") ?? t("history.unknownAccount")} {from?.money ? moneyText(from.money.amount, from.money.currency) : ""}</p>
          <p>{t("history.detailTo")}: {accounts.get(to?.accountId ?? "") ?? t("history.unknownAccount")} {to?.money ? moneyText(to.money.amount, to.money.currency) : ""}</p>
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
      {activity.kind === "value_update" && (
        <div className="flex flex-col gap-1 rounded-md border border-border p-3">
          <p>{t("history.detailAccount")}: {accounts.get(effects[0]?.accountId ?? "") ?? t("history.unknownAccount")}</p>
          {effects[0]?.money && <p>{t("history.detailDelta")}: {moneyText(effects[0].money.amount, effects[0].money.currency)}</p>}
          {activity.reason && <p>{t("history.reasonLabel")}: {displayEnum(t, "history.reason", activity.reason)}</p>}
        </div>
      )}
      {activity.kind !== "buy" && activity.kind !== "sell" && activity.kind !== "cash_transfer" && activity.kind !== "fx_conversion" && activity.kind !== "value_update" && (
        <ul className="flex flex-col gap-1 rounded-md border border-border p-3">
          {effects.map((effect, index) => (
            <li key={effect.id || index}>{effectLine(t, effect, accounts, instruments)}</li>
          ))}
        </ul>
      )}
      {activity.note && (
        <p>
          <span className="text-muted-foreground">{t("history.detailNote")}</span>
          <span className="ml-2">{activity.note}</span>
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {activity.reversesActivityId && <Badge variant="secondary">{t("history.reversal")}</Badge>}
        {activity.correctionGroupId && !activity.reversesActivityId && <Badge variant="secondary">{t("history.corrected")}</Badge>}
      </div>
    </div>
  );
}

export function ActivityDetailSheet({
  activity,
  timezone,
  accounts,
  instruments,
  onClose,
  t,
}: {
  activity: ActivityDTO | null;
  timezone?: string;
  accounts: Map<string, string>;
  instruments: Map<string, string>;
  onClose: () => void;
  t: Translator;
}) {
  return (
    <Sheet open={activity !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t("common.details")}</SheetTitle>
        </SheetHeader>
        <div className="overflow-y-auto">
          {activity && (
            <ActivityDetailBody activity={activity} timezone={timezone} accounts={accounts} instruments={instruments} t={t} />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
