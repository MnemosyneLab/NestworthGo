import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Button } from "@/components/ui/button";
import { formatKnownOrUnknown } from "@/features/liquidity/liquidityDisplay";
import { upcomingButton, upcomingNavigation, upcomingTitle, supportedUpcomingAction } from "./upcomingActionPresentation";
import type { UpcomingActionDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

export function UpcomingActionList({ items, context }: { items: UpcomingActionDTO[]; context: string }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  return <ul className="divide-y divide-border" aria-label={t("upcomingActions.title")}>
    {items.map(item => {
      const target = supportedUpcomingAction(item) ? upcomingNavigation(item.primaryTarget) : null;
      return <li key={item.id} className="flex min-w-0 flex-wrap items-start justify-between gap-3 py-4">
        <div className="min-w-0 flex-1 basis-56 space-y-1 break-words">
          <p className="font-medium">{item.displayName}</p>
          <p className="text-sm">{upcomingTitle(t, item)}</p>
          {item.sortOn && <p className="text-sm text-muted-foreground">{item.sortOn}{(item.daysFromToday ?? 0) > 0 && <> · {t("upcomingActions.inDays", { days: item.daysFromToday })}</>}</p>}
          {item.estimatedReceiptOn && <p className="text-sm text-muted-foreground">{t("upcomingActions.receiptEstimate", { date: item.estimatedReceiptOn })} · {t(`upcomingActions.basis.${item.receiptBasis}`, { defaultValue: t("upcomingActions.basis.unknown") })}</p>}
          {item.referenceOn && <p className="text-sm text-muted-foreground">{t("upcomingActions.reference", { date: item.referenceOn })}</p>}
          <p className="text-sm tabular-nums">{t(`upcomingActions.amountRole.${item.amountRole}`, { defaultValue: t("upcomingActions.amountRole.unknown") })}: {formatKnownOrUnknown(item.amount, t("upcomingActions.unknownAmount"))}</p>
          {item.requestedReserve && <p className="text-sm tabular-nums">{t("upcomingActions.requested")}: {formatKnownOrUnknown(item.requestedReserve, t("upcomingActions.unknownAmount"))}</p>}
          {(item.reservationLabels ?? []).length > 0 && <details className="text-sm"><summary className="cursor-pointer">{t("upcomingActions.reservations", { count: (item.reservationIds ?? []).length })}</summary><ul>{(item.reservationIds ?? []).map((id, i) => <li key={id}>{item.reservationLabels?.[i]}</li>)}</ul></details>}
          {(item.reasonCodes ?? []).map(reason => <p key={reason} className="text-sm text-warning-foreground">{t(`upcomingActions.reasons.${reason}`, { defaultValue: t("upcomingActions.reasons.unsupported_source") })}</p>)}
          {(item.assumptions ?? []).includes("weekdays_no_holidays") && <p className="text-sm text-muted-foreground">{t("upcomingActions.weekdays")}</p>}
        </div>
        {target && navigation && <Button id={`${context}-${item.id}`} type="button" variant="outline" size="sm" onClick={() => navigation.open(target)} aria-label={`${upcomingButton(t, item.primaryTarget, item.phase)} · ${item.displayName}`}>{upcomingButton(t, item.primaryTarget, item.phase)}</Button>}
      </li>;
    })}
  </ul>;
}
