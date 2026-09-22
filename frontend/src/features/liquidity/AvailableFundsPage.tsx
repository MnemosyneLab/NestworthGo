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
import { NativeSelect } from "@/components/ui/select";
import { PageChrome } from "@/components/layout/PageChrome";
import { PageIntro } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import { useLiquidityOverview } from "@/queries/liquidity";
import { PolicySheet, ProductDetailSheet, ProductFormSheet,  moneyText } from "@/features/liquidity/ProductSheets";
import { canHoldProducts } from "@/features/liquidity/productPolicy";
import {
  actionRequiredLabel,
  bucketForHorizon,
  cashVersusProceeds,
  formatKnownOrUnknown,
  remindersFor,
  resultForHorizon,
  sourceMatchesFilter,
  sourceStateLabel,
  type CurrencyMode,
  type SourceFilter,
} from "@/features/liquidity/liquidityDisplay";
import type { LiquidityBucketDTO, LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

function HorizonCard({
  label,
  bucket,
  selected,
  onSelect,
  unknown,
  sources,
  currencyMode,
}: {
  label: string;
  bucket: LiquidityBucketDTO;
  selected: boolean;
  onSelect: () => void;
  unknown: string;
  sources: LiquiditySourceDTO[];
  currencyMode: CurrencyMode;
}) {
  const { t } = useTranslation();
  const incomplete = currencyMode === "native"
    ? !(bucket.nativeCurrencyGroups?.length) || bucket.nativeCurrencyGroups.some((group) => !group.fullAvailable || !group.fullUnreserved)
    : !bucket.fullAvailable || !bucket.fullUnreserved;
  const available = currencyMode === "native"
    ? (bucket.nativeCurrencyGroups ?? []).map((group) => formatKnownOrUnknown(group.fullAvailable ?? group.knownAvailableSubtotal, unknown)).join(" · ") || unknown
    : formatKnownOrUnknown(bucket.fullAvailable ?? bucket.knownAvailableSubtotal, unknown);
  const reserved = currencyMode === "native"
    ? (bucket.nativeCurrencyGroups ?? []).map((group) => formatKnownOrUnknown(group.appliedReserveSubtotal, unknown)).join(" · ") || unknown
    : formatKnownOrUnknown(bucket.appliedReserveSubtotal, unknown);
  const unreserved = currencyMode === "native"
    ? (bucket.nativeCurrencyGroups ?? []).map((group) => formatKnownOrUnknown(group.fullUnreserved ?? group.knownUnreservedSubtotal, unknown)).join(" · ") || unknown
    : formatKnownOrUnknown(bucket.fullUnreserved ?? bucket.knownUnreservedSubtotal, unknown);
  const breakdown = cashVersusProceeds(sources, bucket.horizonOn, unknown, bucket.fullAvailable?.currency ?? bucket.knownAvailableSubtotal?.currency ?? "USD", currencyMode);
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onSelect}
      className={`rounded-xl border p-4 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary ${selected ? "border-primary bg-primary/5" : "border-border bg-card"}`}
      data-testid={`horizon-${bucket.horizonOn}`}
    >
      <p className="text-sm font-medium">{label}</p>
      <p className="mt-2 text-2xl font-semibold" data-testid={`horizon-available-${bucket.horizonOn}`}>{available}</p>
      <p className="mt-1 text-sm text-muted-foreground">{t("availableFunds.reserved")}: {reserved}</p>
      <p className="text-sm text-muted-foreground">{t("availableFunds.unreserved")}: {unreserved}</p>
      {incomplete && <p className="mt-2 text-xs text-muted-foreground">{t("availableFunds.incompleteAmount")}</p>}
      <p className="mt-3 text-xs text-muted-foreground">{t("availableFunds.cashAccessible")}: {breakdown.cash}</p>
      <p className="text-xs text-muted-foreground">{t("availableFunds.estimatedProceeds")}: {breakdown.proceeds}</p>
    </button>
  );
}

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
  const [currencyMode, setCurrencyMode] = useState<CurrencyMode>("base");
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
  const allLocked = Boolean(selectedBucket && selectedBucket.status === "complete" && !selectedBucket.fullAvailable && !selectedBucket.knownAvailableSubtotal && sources.some((source) => !source.excluded));

  const pageChrome = (
    <PageChrome
      pageId="available-funds"
      title={t("availableFunds.pageTitle")}
      actions={
        <>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={includeEarly}
              onChange={(event) => setIncludeEarly(event.target.checked)}
            />
            {t("availableFunds.considerEarly")}
          </label>
          <Button type="button" size="sm" variant="outline" onClick={() => { setReservationSource(null); setManageReservations(true); }}>{t("availableFunds.manageReservations")}</Button>
          {eligibleAccounts.length > 0 && (
            <Button type="button" size="sm" onClick={() => { if (eligibleAccounts.length === 1) setProductFormAccountId(eligibleAccounts[0].account.id); else setChooseAccount(true); }}>
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
    <div className="flex flex-col gap-6" data-testid="available-funds-page">
      {pageChrome}
      <PageIntro
        description={t("availableFunds.description")}
        status={
          <p className="text-sm text-muted-foreground">
            {t("availableFunds.asOf", { date: data.localDate, timezone: data.timezone })}
          </p>
        }
      />
      <p className="text-sm text-muted-foreground">{t("availableFunds.cumulativeHint")}</p>
      <p className="text-sm text-muted-foreground">{t("availableFunds.assetsOnly")}</p>

      {sources.length === 0 ? (
        <EmptyState title={t("availableFunds.emptyTitle")} description={t("availableFunds.emptyDescription")} />
      ) : (
        <>
          <div className="grid gap-3 md:grid-cols-3" role="group" aria-label={t("availableFunds.selectHorizon")}>
            {today && <HorizonCard label={t("availableFunds.today")} bucket={today} selected={horizonOn === today.horizonOn} onSelect={() => setSelectedHorizon(today.horizonOn)} unknown={unknown} sources={sources} currencyMode={currencyMode} />}
            {week && <HorizonCard label={t("availableFunds.within7")} bucket={week} selected={horizonOn === week.horizonOn} onSelect={() => setSelectedHorizon(week.horizonOn)} unknown={unknown} sources={sources} currencyMode={currencyMode} />}
            {month && <HorizonCard label={t("availableFunds.within30")} bucket={month} selected={horizonOn === month.horizonOn} onSelect={() => setSelectedHorizon(month.horizonOn)} unknown={unknown} sources={sources} currencyMode={currencyMode} />}
          </div>
          {custom && (
            <HorizonCard label={t("availableFunds.customDate")} bucket={custom} selected={horizonOn === custom.horizonOn} onSelect={() => setSelectedHorizon(custom.horizonOn)} unknown={unknown} sources={sources} currencyMode={currencyMode} />
          )}
          <div className="flex flex-wrap items-end gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="custom-horizon">{t("availableFunds.customDate")}</Label>
              <DatePicker clearable allowFuture id="custom-horizon" value={customDate} onChange={(next) => { setCustomDate(next); setSelectedHorizon(next || undefined); }} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="currency-mode">{t("availableFunds.currencyDisplay")}</Label>
              <NativeSelect id="currency-mode" value={currencyMode} onChange={(event) => setCurrencyMode(event.target.value as CurrencyMode)}>
                <option value="base">{t("availableFunds.base")}</option>
                <option value="native">{t("availableFunds.native")}</option>
              </NativeSelect>
            </div>
          </div>
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

          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full min-w-[56rem] text-left text-sm" data-testid="liquidity-source-table">
              <caption className="sr-only">{t("availableFunds.sourceTable")}</caption>
              <thead>
                <tr className="border-b border-border bg-muted/40 text-muted-foreground">
                  <th className="px-3 py-2 font-medium">{t("availableFunds.account")}</th>
                  <th className="px-3 py-2 font-medium">{t("availableFunds.asset")}</th>
                  <th className="px-3 py-2 font-medium">{t("availableFunds.currentValue")}</th>
                  <th className="px-3 py-2 font-medium">{t("availableFunds.expectedAccess")}</th>
                  <th className="px-3 py-2 font-medium">{t("availableFunds.cost")}</th>
                  <th className="px-3 py-2 font-medium">{t("availableFunds.reserved")}</th>
                  <th className="px-3 py-2 font-medium">{t("availableFunds.unreserved")}</th>
                  <th className="px-3 py-2 font-medium">{t("availableFunds.nextAction")}</th>
                </tr>
              </thead>
              <tbody>
                {visibleSources.length === 0 ? (
                  <tr>
                    <td className="px-3 py-6 text-muted-foreground" colSpan={8}>{t("availableFunds.noSources")}</td>
                  </tr>
                ) : visibleSources.map((source) => {
                  const result = resultForHorizon(source, horizonOn);
                  const route = result?.selectedRoute ?? (includeEarly ? source.earlyRoute : source.normalRoute);
                  return (
                    <tr key={source.sourceKey} className="border-b border-border last:border-0">
                      <td className="px-3 py-2">{accountNames.get(source.accountId) ?? source.displayName}</td>
                      <td className="px-3 py-2">
                        <div className="flex flex-col gap-1">
                          <span>{sourceName(t, source)}</span>
                          <span className="text-xs text-muted-foreground">{sourceStateLabel(t, source)}</span>
                          {source.dueUnconfirmed && <Badge variant="warning">{t("availableFunds.dueUnconfirmed")}</Badge>}
                        </div>
                      </td>
                      <td className="px-3 py-2">{moneyText(source.currentNativeValue, unknown)}</td>
                      <td className="px-3 py-2">{route?.receiptOn ?? t("availableFunds.unknownAmount")}</td>
                      <td className="px-3 py-2">{moneyText(route?.feeNative, unknown)}</td>
                      <td className="px-3 py-2">
                        <div>{t("availableFunds.appliedReserve")}: {moneyText(result?.appliedReserveNative, unknown)}</div>
                        <div className="text-xs text-muted-foreground">{t("availableFunds.requestedReserve")}: {moneyText(source.reservationRequested, unknown)}</div>
                        {result?.reserveShortfallNative && !/^0(?:\.0+)?$/.test(result.reserveShortfallNative.amount) && (
                          <div className="text-xs text-amber-700 dark:text-amber-400">{t("availableFunds.reserveShortfall")}: {moneyText(result.reserveShortfallNative, unknown)}</div>
                        )}
                        {!result?.selectedRoute && source.reservationRequested && !/^0(?:\.0+)?$/.test(source.reservationRequested.amount) && (
                          <div className="text-xs text-muted-foreground">{t("availableFunds.reserveNotApplied")}: {moneyText(source.reservationRequested, unknown)}</div>
                        )}
                      </td>
                      <td className="px-3 py-2">{source.dueUnconfirmed ? unknown : moneyText(result?.unreservedNative, unknown)}</td>
                      <td className="px-3 py-2">
                        <div className="flex items-center justify-between gap-2 whitespace-nowrap">
                          {source.displayState === "needs_info" ? (
                            <Button type="button" size="sm" variant="link" className="px-0" onClick={() => setPolicySource(source)}>{t("availableFunds.completeRules")}</Button>
                          ) : source.productId ? (
                            <Button type="button" size="sm" variant="link" className="px-0" onClick={() => setDetailProductId(source.productId)}>{t("availableFunds.products")}</Button>
                          ) : <span className="text-sm text-muted-foreground">{actionRequiredLabel(t, route?.actionRequired)}</span>}
                          <ActionMenu label={t("connections.actionsFor", { name: `${sourceName(t, source)} · ${source.nativeCurrency}` })} items={[
                            ...(source.displayState === "needs_info" ? [] : [{ id: "rules", label: t("availableFunds.editSource"), onSelect: () => setPolicySource(source) }]),
                            { id: "reservations", label: t("availableFunds.manageReservations"), onSelect: () => { setReservationSource(source); setManageReservations(true); } },
                            ...(source.productId && source.displayState === "needs_info" ? [{ id: "product", label: t("availableFunds.products"), onSelect: () => setDetailProductId(source.productId) }] : []),
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

      {(data.assumptions ?? []).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{t("availableFunds.assumptions")}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-1 text-sm text-muted-foreground">
            {(data.assumptions ?? []).map((assumption) => <p key={assumption}>{t(`availableFunds.${assumption}`, { defaultValue: t("availableFunds.missingInputs") })}</p>)}
          </CardContent>
        </Card>
      )}

      <PolicySheet key={policySource?.sourceKey ?? "policy"} source={policySource} open={Boolean(policySource)} onOpenChange={(open) => { if (!open) setPolicySource(null); }} />
      <ReservationManager key={reservationSource?.sourceKey ?? "reservation"} source={reservationSource} sources={sources} open={manageReservations} onOpenChange={(open) => { setManageReservations(open); if (!open) setReservationSource(null); }} />
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
