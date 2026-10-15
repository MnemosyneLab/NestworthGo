import { Lock, Zap } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ActionMenu } from "@/components/ui/action-menu";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { moneyText, resultForHorizon, routeIsEarly, sourceStateLabel, sourceStateTone } from "./liquidityDisplay";
import { sourceName } from "./sourceName";
import type { LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const ZERO = /^0(?:\.0+)?$/;

// Rows collapse into labelled cards when the container (not the window) is narrow.
const cellBase = "px-3 py-3 @max-3xl:flex @max-3xl:items-baseline @max-3xl:justify-between @max-3xl:gap-3 @max-3xl:py-1 @max-3xl:before:text-xs @max-3xl:before:text-muted-foreground @max-3xl:before:content-[attr(data-label)]";

export function SourceTable({ sources, horizonOn, includeEarly, accountNames, onEditRules, onManageReservations, onOpenProduct }: {
  sources: LiquiditySourceDTO[];
  horizonOn: string;
  includeEarly: boolean;
  accountNames: Map<string, string>;
  onEditRules: (source: LiquiditySourceDTO) => void;
  onManageReservations: (source: LiquiditySourceDTO) => void;
  onOpenProduct: (productId: string) => void;
}) {
  const { t } = useTranslation();
  const unknown = t("availableFunds.unknownAmount");
  return <div className="@container">
    <table role="table" className="w-full text-left text-sm @max-3xl:block" data-testid="liquidity-source-table">
      <caption className="sr-only">{t("availableFunds.sourceTable")}</caption>
      <thead className="@max-3xl:sr-only">
        <tr role="row" className="border-b border-border bg-muted/50 text-xs text-muted-foreground">
          <th role="columnheader" className="px-3 py-2.5 font-semibold">{t("availableFunds.asset")}</th>
          <th role="columnheader" className="px-3 py-2.5 text-right font-semibold">{t("availableFunds.currentValue")}</th>
          <th role="columnheader" className="px-3 py-2.5 font-semibold">{t("availableFunds.expectedAccess")}</th>
          <th role="columnheader" className="px-3 py-2.5 text-right font-semibold">{t("availableFunds.availableAfterReserve")}</th>
          <th role="columnheader" className="w-0 px-3 py-2"><span className="sr-only">{t("common.actions")}</span></th>
        </tr>
      </thead>
      <tbody className="@max-3xl:block @max-3xl:divide-y @max-3xl:divide-border">
        {sources.map((source) => {
          const result = resultForHorizon(source, horizonOn);
          const route = result?.selectedRoute ?? (includeEarly ? source.earlyRoute ?? source.normalRoute : source.normalRoute);
          const early = routeIsEarly(route) && !source.excluded;
          const hasReserve = !ZERO.test(source.reservationRequested.amount);
          const unavailableByDate = result?.status === "complete" && !result.selectedRoute;
          const hasFee = !source.excluded && route && (!route.feeNative || !ZERO.test(route.feeNative.amount));
          const shortfall = result?.reserveShortfallNative && !ZERO.test(result.reserveShortfallNative.amount);
          return (
            <tr key={source.sourceKey} role="row" className={cn(
              "border-b border-border align-top transition-colors last:border-0 hover:bg-muted/40",
              "@max-3xl:grid @max-3xl:grid-cols-[1fr_auto] @max-3xl:items-start @max-3xl:gap-x-2 @max-3xl:border-b-0 @max-3xl:p-3",
              early && "bg-warning/8",
            )}>
              <td role="cell" className="px-3 py-3 @max-3xl:min-w-0 @max-3xl:p-0">
                <div className="flex min-w-0 flex-col gap-1 break-words">
                  <span className="font-semibold">{sourceName(t, source)}</span>
                  <span className="text-xs text-muted-foreground">{accountNames.get(source.accountId) ?? source.displayName}</span>
                  <span className="mt-0.5 flex flex-wrap items-center gap-1.5">
                    <Badge variant={sourceStateTone(source)} className="gap-1">
                      {source.displayState === "locked" && !source.excluded && <Lock className="size-3" aria-hidden="true" />}
                      {sourceStateLabel(t, source)}
                    </Badge>
                    {source.dueUnconfirmed && <Badge variant="warning">{t("availableFunds.dueUnconfirmed")}</Badge>}
                  </span>
                </div>
              </td>
              <td role="cell" data-label={t("availableFunds.currentValue")} className={cn(cellBase, "num whitespace-nowrap text-right @max-3xl:order-3 @max-3xl:col-span-2")}>
                <span>{moneyText(source.currentNativeValue, unknown)}</span>
              </td>
              <td role="cell" data-label={t("availableFunds.expectedAccess")} className={cn(cellBase, "@max-3xl:order-4 @max-3xl:col-span-2")}>
                <span className="flex flex-col @max-3xl:items-end">
                  <span className="flex flex-wrap items-center gap-1.5 whitespace-nowrap">
                    {source.excluded ? "—" : route?.receiptOn ?? unknown}
                    {early && <Badge variant="warning" className="gap-1"><Zap className="size-3" aria-hidden="true" />{t("availableFunds.actionEarly")}</Badge>}
                  </span>
                  {hasFee && <span className={cn("mt-1 text-xs", early ? "text-warning-foreground" : "text-muted-foreground")}>{t("availableFunds.cost")}: {moneyText(route.feeNative, unknown)}</span>}
                </span>
              </td>
              <td role="cell" data-label={t("availableFunds.availableAfterReserve")} className={cn(cellBase, "num text-right @max-3xl:order-5 @max-3xl:col-span-2")}>
                <span className="flex flex-col @max-3xl:items-end">
                  {source.excluded || source.dueUnconfirmed ? <span className="text-muted-foreground">—</span>
                    : unavailableByDate ? <span className="inline-flex items-center gap-1 text-xs text-muted-foreground"><Lock className="size-3" aria-hidden="true" />{t("availableFunds.notAvailableByDate")}</span>
                    : result?.unreservedNative ? <span className="whitespace-nowrap text-base font-semibold">{moneyText(result.unreservedNative, unknown)}</span>
                    : <span className="text-muted-foreground">{unknown}</span>}
                  {hasReserve && <details className="mt-1 text-xs text-muted-foreground">
                    <summary className="cursor-pointer">{t("availableFunds.requestedReserve")}: {moneyText(source.reservationRequested, unknown)}</summary>
                    {result?.selectedRoute && <div>{t("availableFunds.appliedReserve")}: {moneyText(result?.appliedReserveNative, unknown)}</div>}
                    {shortfall && <div className="text-warning-foreground">{t("availableFunds.reserveShortfall")}: {moneyText(result?.reserveShortfallNative, unknown)}</div>}
                    {!result?.selectedRoute && <div>{t("availableFunds.reserveNotApplied")}: {moneyText(source.reservationRequested, unknown)}</div>}
                  </details>}
                </span>
              </td>
              <td role="cell" className="px-3 py-2 @max-3xl:order-2 @max-3xl:p-0">
                <div className="flex items-center justify-end gap-2 whitespace-nowrap">
                  {source.displayState === "needs_info" && <Button type="button" size="sm" variant="link" className="px-0" onClick={() => onEditRules(source)}>{t("availableFunds.completeRules")}</Button>}
                  <ActionMenu label={t("connections.actionsFor", { name: `${sourceName(t, source)} · ${source.nativeCurrency}` })} items={[
                    ...(source.displayState === "needs_info" ? [] : [{ id: "rules", label: t("availableFunds.editSource"), onSelect: () => onEditRules(source) }]),
                    { id: "reservations", label: t("availableFunds.manageReservations"), onSelect: () => onManageReservations(source) },
                    ...(source.productId ? [{ id: "product", label: t("availableFunds.products"), onSelect: () => onOpenProduct(source.productId!) }] : []),
                  ]} />
                </div>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  </div>;
}
