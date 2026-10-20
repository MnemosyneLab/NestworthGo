import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatKnownOrUnknown } from "@/features/liquidity/liquidityDisplay";
import { Notice } from "@/components/ui/notice";
import { upcomingButton, upcomingNavigation, upcomingTitle, supportedUpcomingAction } from "./upcomingActionPresentation";
import type { UpcomingActionDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

type BadgeVariant = "default" | "secondary" | "success" | "warning" | "destructive";

/** Overdue receipts and blocked items are urgent; other attention items warn; the rest are informational. */
function actionVariant(item: UpcomingActionDTO): BadgeVariant {
  const overdue = (item.daysFromToday ?? 0) < 0 && (item.phase === "receipt_due" || item.phase === "blocked");
  if (overdue) return "destructive";
  if (item.group === "attention") return "warning";
  if (item.group === "eligible") return "success";
  return "default";
}

export function UpcomingActionList({ items, context }: { items: UpcomingActionDTO[]; context: string }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  return <ul className="@container divide-y divide-border" aria-label={t("upcomingActions.title")}>
    {items.map(item => {
      const targets = supportedUpcomingAction(item) ? [item.primaryTarget, ...(item.targets ?? [])].filter((target, index, all) => all.findIndex(other => JSON.stringify(other) === JSON.stringify(target)) === index) : [];
      return <li key={item.id} className="flex min-w-0 flex-wrap items-start justify-between gap-x-4 gap-y-3 py-4">
        <div className="flex min-w-0 flex-1 basis-56 flex-col gap-1.5 break-words">
          <p className="font-semibold">{item.displayName}</p>
          <p><Badge variant={supportedUpcomingAction(item) ? actionVariant(item) : "secondary"} className="max-w-full whitespace-normal">{upcomingTitle(t, item)}</Badge></p>
          <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
            {item.maturityOn && <p className="num">{t("upcomingActions.maturityDate", { date: item.maturityOn })}</p>}
            {item.eligibilityOn && <p className="num">{t("upcomingActions.eligibilityDate", { date: item.eligibilityOn })}</p>}
            {item.sortOn && <p className="num">{item.sortOn}{(item.daysFromToday ?? 0) > 0 && <> · {t("upcomingActions.inDays", { count: item.daysFromToday ?? 0 })}</>}</p>}
            {item.estimatedReceiptOn && <p className="num">{t("upcomingActions.receiptEstimate", { date: item.estimatedReceiptOn })} · {t(`upcomingActions.basis.${item.receiptBasis}`, { defaultValue: t("upcomingActions.basis.unknown") })}</p>}
            {item.phase === "receipt_due" && <p>{t("upcomingActions.receiptNotActual")}</p>}
            {item.referenceOn && <p className="num">{t("upcomingActions.reference", { date: item.referenceOn })}</p>}
            {(item.assumptions ?? []).includes("weekdays_no_holidays") && <p>{t("upcomingActions.weekdays")}</p>}
          </div>
          {(item.reasonCodes ?? []).map(reason => <Notice key={reason} tone="warning" className="py-1.5 text-xs leading-5">{t(`upcomingActions.reasons.${reason}`, { defaultValue: t("upcomingActions.reasons.unsupported_source") })}</Notice>)}
          {(item.reservationLabels ?? []).length > 0 && <details className="rounded-xl border border-border px-3 py-2 text-sm"><summary className="cursor-pointer select-none font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">{t("upcomingActions.reservations", { count: (item.reservationIds ?? []).length })}</summary><ul className="mt-2 flex flex-col gap-1 border-t border-border pt-2 text-muted-foreground">{(item.reservationIds ?? []).map((id, i) => <li key={id}>{item.reservationLabels?.[i]}</li>)}</ul></details>}
        </div>
        <div className="flex w-full min-w-0 max-w-full flex-col items-start gap-2 text-left @lg:w-auto @lg:items-end @lg:text-right">
          <p className="num text-base font-semibold">{t(`upcomingActions.amountRole.${item.amountRole}`, { defaultValue: t("upcomingActions.amountRole.unknown") })}: {formatKnownOrUnknown(item.amount, t("upcomingActions.unknownAmount"))}</p>
          {item.requestedReserve && <p className="num text-xs text-muted-foreground">{t("upcomingActions.requested")}: {formatKnownOrUnknown(item.requestedReserve, t("upcomingActions.unknownAmount"))}</p>}
          {navigation && <div className="flex flex-wrap justify-start gap-2 @lg:justify-end">{targets.map((target, index) => {
            const destination = upcomingNavigation(target, target.kind === "product" ? item.phase : undefined);
            return destination && <Button key={`${target.kind}-${index}`} id={`${context}-${item.id}-${index}`} type="button" variant="outline" size="sm" onClick={() => navigation.open(destination)} aria-label={`${upcomingButton(t, target, target.kind === "product" ? item.phase : undefined)} · ${item.displayName}`}>{upcomingButton(t, target, target.kind === "product" ? item.phase : undefined)}</Button>;
          })}</div>}
        </div>
      </li>;
    })}
  </ul>;
}
