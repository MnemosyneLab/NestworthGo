import { FundsComparison } from "./FundsComparison";
import { ActionMenu } from "@/components/ui/action-menu";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { DatePicker } from "@/components/ui/date-picker";
import { sourceName } from "./sourceName";
import { ReservationManager } from "./ReservationManager";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { PageChrome } from "@/components/layout/PageChrome";
import { PageIntro } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import { useLiquidityOverview } from "@/queries/liquidity";
import { PolicySheet, ProductDetailSheet, ProductFormSheet,  moneyText } from "@/features/liquidity/ProductSheets";
import { canHoldProducts } from "@/features/liquidity/productPolicy";
import {
  bucketForHorizon,
  formatKnownOrUnknown,
  remindersFor,
  resultForHorizon,
  sourceMatchesFilter,
  sourceStateLabel,
  type SourceFilter,
} from "@/features/liquidity/liquidityDisplay";
import type { LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

export function AvailableFundsPage({
  productId,
  accountId,
}: {
  productId?: string;
  accountId?: string;
} = {}) {
  const { t } = useTranslation();
  const [includeEarly, setIncludeEarly] = useState(false);
  const [customDate, setCustomDate] = useState("");
  const [filter, setFilter] = useState<SourceFilter>("all");
  const [policySource, setPolicySource] = useState<LiquiditySourceDTO | null>(null);
  const [manageReservations,setManageReservations] = useState(false);
  const [reservationSource, setReservationSource] = useState<LiquiditySourceDTO | null>(null);
  const [chooseAccount, setChooseAccount] = useState(false);
  const [productFormAccountId, setProductFormAccountId] = useState<string | null>(accountId ?? null);
  const [detailProductId, setDetailProductId] = useState<string | null>(productId ?? null);
  const overview = useLiquidityOverview({
    customHorizonOn: customDate || null,
    includeEarlyWithdrawal: includeEarly,
  });
  const accounts = useAccounts({});
  const data = overview.data;
  const buckets = data?.buckets ?? [];
  const [selectedHorizon, setSelectedHorizon] = useState<string>();
  const horizonOn = selectedHorizon && buckets.some((bucket) => bucket.horizonOn === selectedHorizon)
    ? selectedHorizon
    : buckets[0]?.horizonOn ?? "";
  const sources = data?.sources ?? [];
  const visibleSources = sources.filter((source) => sourceMatchesFilter(source, horizonOn, filter));
  const accountNames = useMemo(
    () => new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name])),
    [accounts.data],
  );
  const eligibleAccounts = (accounts.data ?? []).filter((record) => canHoldProducts(record.account));
  const reminders = data ? remindersFor(data, t) : [];
  const unknown = t("availableFunds.unknownAmount");
  const allExcluded = sources.length > 0 && sources.every((source) => source.excluded);
  const selectedBucket = data ? bucketForHorizon(data, horizonOn) : undefined;
  const allLocked = sources.some((source) => !source.excluded) && sources.every((source) => source.excluded || (resultForHorizon(source, horizonOn)?.status === "complete" && !resultForHorizon(source, horizonOn)?.selectedRoute));

  const pageChrome = (
    <PageChrome
      pageId="available-funds"
      title={t("availableFunds.pageTitle")}
      actions={
        <>
          {eligibleAccounts.length > 0 && (
            <Button type="button" onClick={() => { if (eligibleAccounts.length === 1) setProductFormAccountId(eligibleAccounts[0].account.id); else setChooseAccount(true); }}>
              {t("availableFunds.addProduct")}
            </Button>
          )}
        </>
      }
    />
  );

  if (overview.isLoading) {
    return <>{pageChrome}<LoadingState label={t("ui.state.loadingPage")} /></>;
  }
  if (overview.isError || !data) {
    return (
      <>
        {pageChrome}
        <ErrorState
          title={t("availableFunds.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => overview.refetch()}
          retryLabel={t("common.retryAction")}
        />
      </>
    );
  }

  const today = buckets[0];
  const week = buckets[1];
  const month = buckets[2];
  const custom = buckets.find((bucket) => customDate && bucket.horizonOn === customDate);

  return (
    <div className="flex flex-col gap-5" aria-busy={overview.isFetching} data-testid="available-funds-page">
      {pageChrome}
      <PageIntro
        description={t("availableFunds.description")}
        status={
          <p className="text-sm text-muted-foreground">
            {t("availableFunds.asOf", { date: data.localDate, timezone: data.timezone })}
          </p>
        }
      />

      {sources.length === 0 ? (
        <EmptyState title={t("availableFunds.emptyTitle")} description={t("availableFunds.emptyDescription")} />
      ) : (
        <>
          <div className="flex flex-wrap items-end gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="custom-horizon">{t("availableFunds.customDate")}</Label>
              <DatePicker clearable allowFuture min={data.localDate} id="custom-horizon" value={customDate} onChange={(next) => { setCustomDate(next); setSelectedHorizon(next || undefined); }} />
            </div>
            <label className="flex min-h-9 items-center gap-2 text-sm">
              <input type="checkbox" checked={includeEarly} onChange={(event) => setIncludeEarly(event.target.checked)} />
              {t("availableFunds.considerEarly")}
            </label>
            <Button type="button" size="sm" variant="outline" className="ml-auto" onClick={() => { setReservationSource(null); setManageReservations(true); }}>{t("availableFunds.manageReservations")}</Button>
          </div>
          <FundsComparison
            horizons={[
              ...(today ? [{ label: t("availableFunds.today"), bucket: today }] : []),
              ...(week ? [{ label: t("availableFunds.within7"), bucket: week }] : []),
              ...(month ? [{ label: t("availableFunds.within30"), bucket: month }] : []),
              ...(custom && ![today, week, month].some(bucket => bucket?.horizonOn === custom.horizonOn) ? [{ label: t("availableFunds.customDate"), bucket: custom }] : []),
            ]}
            selected={horizonOn} onSelect={setSelectedHorizon} sources={sources}
            baseCurrency={data.baseCurrency}
          />
          {overview.isPlaceholderData && <p role="status" className="text-sm text-muted-foreground">{t("availableFunds.updatingEstimate")}</p>}
          {allExcluded && <EmptyState title={t("availableFunds.allExcludedTitle")} description={t("availableFunds.assetsOnly")} />}
          {!allExcluded && allLocked && <EmptyState title={t("availableFunds.allLockedTitle")} />}
          {selectedBucket?.status !== "complete" && !allExcluded && (
            <p role="status" className="text-sm text-warning-foreground">{t("availableFunds.needsInfoTitle")}</p>
          )}

          {reminders.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>{t("availableFunds.upcoming")}</CardTitle>
              </CardHeader>
              <CardContent>
                <ul className="flex flex-col gap-2 text-sm">
                  {reminders.map((reminder) => (
                    <li key={reminder.id}>
                      {reminder.productId ? (
                        <Button type="button" variant="link" className="h-auto p-0" onClick={() => setDetailProductId(reminder.productId ?? null)}>
                          {reminder.text}
                        </Button>
                      ) : reminder.text}
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          )}

          <div className="flex flex-wrap gap-2" role="group" aria-label={t("availableFunds.assets")}>
            {(["all", "available", "locked", "needs_info", "excluded"] as const).map((value) => (
              <Button key={value} type="button" size="sm" variant={filter === value ? "default" : "outline"} aria-pressed={filter === value} onClick={() => setFilter(value)}>
                {value === "all" && t("availableFunds.filterAll")}
                {value === "available" && t("availableFunds.filterAvailable")}
                {value === "locked" && t("availableFunds.filterLocked")}
                {value === "needs_info" && t("availableFunds.filterNeedsInfo")}
                {value === "excluded" && t("availableFunds.filterExcluded")}
              </Button>
            ))}
          </div>

          <p className="text-sm text-muted-foreground">{t("availableFunds.sourceHorizon", { date: horizonOn })}</p>
          <div className="overflow-x-auto rounded-2xl border border-border">
            <table className="w-full min-w-[38rem] text-left text-sm" data-testid="liquidity-source-table">
              <caption className="sr-only">{t("availableFunds.sourceTable")}</caption>
              <thead>
                <tr className="border-b border-border bg-muted/50 text-muted-foreground">
                  <th className="px-3 py-2.5 font-semibold">{t("availableFunds.asset")}</th>
                  <th className="px-3 py-2.5 text-right font-semibold">{t("availableFunds.currentValue")}</th>
                  <th className="px-3 py-2.5 font-semibold">{t("availableFunds.expectedAccess")}</th>
                  <th className="px-3 py-2.5 text-right font-semibold">{t("availableFunds.availableAfterReserve")}</th>
                  <th className="w-0 px-3 py-2"><span className="sr-only">{t("common.actions")}</span></th>
                </tr>
              </thead>
              <tbody>
                {visibleSources.length === 0 ? (
                  <tr>
                    <td className="px-3 py-6 text-muted-foreground" colSpan={5}>{t("availableFunds.noSources")}</td>
                  </tr>
                ) : visibleSources.map((source) => {
                  const result = resultForHorizon(source, horizonOn);
                  const route = result?.selectedRoute ?? (includeEarly ? source.earlyRoute ?? source.normalRoute : source.normalRoute);
                  const hasReserve = !/^0(?:\.0+)?$/.test(source.reservationRequested.amount);
                  const unavailableByDate = result?.status === "complete" && !result.selectedRoute;
                  return (
                    <tr key={source.sourceKey} className="border-b border-border align-top transition-colors last:border-0 hover:bg-muted/40">
                      <td className="px-3 py-2">
                        <div className="flex max-w-64 flex-col gap-1 break-words">
                          <span className="font-medium">{sourceName(t, source)}</span>
                          <span className="text-xs text-muted-foreground">{accountNames.get(source.accountId) ?? source.displayName}</span>
                          <span className="text-xs text-muted-foreground">{sourceStateLabel(t, source)}</span>
                          {source.dueUnconfirmed && <Badge variant="warning">{t("availableFunds.dueUnconfirmed")}</Badge>}
                        </div>
                      </td>
                      <td className="num whitespace-nowrap px-3 py-2 text-right">{moneyText(source.currentNativeValue, unknown)}</td>
                      <td className="px-3 py-2">
                        <p className="whitespace-nowrap">{source.excluded ? "—" : route?.receiptOn ?? unknown}</p>
                        {!source.excluded && route && (!route.feeNative || !/^0(?:\.0+)?$/.test(route.feeNative.amount)) && <p className="mt-1 text-xs text-muted-foreground">{t("availableFunds.cost")}: {moneyText(route.feeNative, unknown)}</p>}
                      </td>
                      <td className="num px-3 py-2 text-right">
                        <p className={unavailableByDate || source.excluded ? "text-xs text-muted-foreground" : "whitespace-nowrap font-medium"}>
                          {source.excluded ? t("availableFunds.filterExcluded") : source.dueUnconfirmed ? t("availableFunds.dueUnconfirmed") : unavailableByDate ? t("availableFunds.notAvailableByDate") : moneyText(result?.unreservedNative, unknown)}
                        </p>
                        {hasReserve && <details className="mt-1 text-xs text-muted-foreground">
                          <summary className="cursor-pointer">{t("availableFunds.requestedReserve")}: {moneyText(source.reservationRequested, unknown)}</summary>
                          {result?.selectedRoute && <div>{t("availableFunds.appliedReserve")}: {moneyText(result?.appliedReserveNative, unknown)}</div>}
                        {result?.reserveShortfallNative && !/^0(?:\.0+)?$/.test(result.reserveShortfallNative.amount) && (
                          <div className="text-xs text-warning-foreground">{t("availableFunds.reserveShortfall")}: {moneyText(result.reserveShortfallNative, unknown)}</div>
                        )}
                        {!result?.selectedRoute && (
                          <div className="text-xs text-muted-foreground">{t("availableFunds.reserveNotApplied")}: {moneyText(source.reservationRequested, unknown)}</div>
                        )}
                        </details>}
                      </td>
                      <td className="px-3 py-2">
                        <div className="flex items-center justify-end gap-2 whitespace-nowrap">
                          {source.displayState === "needs_info" ? (
                            <Button type="button" size="sm" variant="link" className="px-0" onClick={() => setPolicySource(source)}>{t("availableFunds.completeRules")}</Button>
                          ) : null}
                          <ActionMenu label={t("connections.actionsFor", { name: `${sourceName(t, source)} · ${source.nativeCurrency}` })} items={[
                            ...(source.displayState === "needs_info" ? [] : [{ id: "rules", label: t("availableFunds.editSource"), onSelect: () => setPolicySource(source) }]),
                            { id: "reservations", label: t("availableFunds.manageReservations"), onSelect: () => { setReservationSource(source); setManageReservations(true); } },
                            ...(source.productId ? [{ id: "product", label: t("availableFunds.products"), onSelect: () => setDetailProductId(source.productId) }] : []),
                          ]} />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {(data.unresolvedReservations ?? []).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{t("availableFunds.unresolvedReservations")}</CardTitle>
          </CardHeader>
          <CardContent className="text-sm">
            {(data.unresolvedReservations ?? []).map((item) => (
              <p key={item.reservation.id}>{item.reservation.label}: {formatKnownOrUnknown(item.reservation.amount, unknown)} · {t("availableFunds.unresolvedSource")} <Button variant="link" onClick={() => { setReservationSource(null); setManageReservations(true); }}>{t("availableFunds.manageReservations")}</Button></p>
            ))}
          </CardContent>
        </Card>
      )}

      <details className="rounded-lg border border-border p-4 text-sm text-muted-foreground">
          <summary className="cursor-pointer font-medium">{t("availableFunds.assumptions")}</summary>
          <div className="mt-3 flex flex-col gap-2">
            <p>{t("availableFunds.assetsOnly")}</p>
            {(data.assumptions ?? []).map((assumption) => <p key={assumption}>{t(`availableFunds.${assumption}`, { defaultValue: t("availableFunds.missingInputs") })}</p>)}
          </div>
      </details>

      <PolicySheet key={policySource?.sourceKey ?? "policy"} source={policySource} open={Boolean(policySource)} onOpenChange={(open) => { if (!open) setPolicySource(null); }} />
      {manageReservations && <ReservationManager key={reservationSource?.sourceKey ?? "reservation"} source={reservationSource} sources={sources} accountNames={Object.fromEntries(accountNames)} open={manageReservations} onOpenChange={(open) => { setManageReservations(open); if (!open) setReservationSource(null); }} />}
      <Sheet open={chooseAccount} onOpenChange={setChooseAccount}><SheetContent><SheetHeader><SheetTitle>{t("availableFunds.chooseAccount")}</SheetTitle></SheetHeader>
        {eligibleAccounts.map(({ account }) => <Button key={account.id} variant="outline" onClick={() => { setChooseAccount(false); setProductFormAccountId(account.id); }}>{account.name}</Button>)}
      </SheetContent></Sheet>
      {productFormAccountId && (
        <ProductFormSheet
          key={productFormAccountId}
          accountName={accountNames.get(productFormAccountId)}
          accountId={productFormAccountId}
          currency={(accounts.data ?? []).find((record) => record.account.id === productFormAccountId)?.account.defaultCurrency ?? data.baseCurrency}
          open={Boolean(productFormAccountId)}
          onOpenChange={(open) => { if (!open) setProductFormAccountId(null); }}
        />
      )}
      <ProductDetailSheet key={detailProductId ?? "product"} productId={detailProductId} open={Boolean(detailProductId)} onOpenChange={(open) => { if (!open) setDetailProductId(null); }} />
    </div>
  );
}
