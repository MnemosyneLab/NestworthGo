import { FundsComparison } from "./FundsComparison";
import { EarlyWithdrawalToggle } from "./EarlyWithdrawalToggle";
import { SourceTable } from "./SourceTable";
import { CalendarClock, TriangleAlert } from "lucide-react";
import { InfoPopover } from "@/components/ui/info-popover";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { DatePicker } from "@/components/ui/date-picker";
import { ReservationManager } from "./ReservationManager";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import { useLiquidityOverview } from "@/queries/liquidity";
import { PolicySheet, ProductDetailSheet, ProductFormSheet } from "@/features/liquidity/ProductSheets";
import { canHoldProducts } from "@/features/liquidity/productPolicy";
import {
  bucketForHorizon,
  formatKnownOrUnknown,
  resultForHorizon,
  sourceFilterCounts,
  sourceMatchesFilter,
  type SourceFilter,
} from "@/features/liquidity/liquidityDisplay";
import type { LiquidityFocus } from "@/app/navigation";
import type { LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

export function AvailableFundsPage({
  productId,
  accountId,
  focus,
  navigationRequestId = 0,
  isActive = true,
}: {
  productId?: string;
  accountId?: string;
  focus?: LiquidityFocus;
  navigationRequestId?: number;
  isActive?: boolean;
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
  const [receiptRequestId, setReceiptRequestId] = useState<number>();
  const overview = useLiquidityOverview({
    customHorizonOn: customDate || null,
    includeEarlyWithdrawal: includeEarly,
  }, isActive);
  const { refetch: refetchOverview } = overview;
  const accounts = useAccounts({});
  const [focusReservationId, setFocusReservationId] = useState<string>();
  const [navigationMessage, setNavigationMessage] = useState<string>();
  // Back restores older request IDs; every consumed request stays consumed while retained.
  const consumedRequests = useRef(new Set<number>());
  const navigationTransition = useRef<{ id: number; active: boolean }>();
  const [navigationRetry, setNavigationRetry] = useState(0);
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
  useEffect(() => {
    const previous = navigationTransition.current;
    if (previous?.active && (!isActive || previous.id !== navigationRequestId)) {
      // Leaving or superseding a pending navigation cancels it for subsequent Back.
      consumedRequests.current.add(previous.id);
    }
    navigationTransition.current = { id: navigationRequestId, active: isActive };
  }, [isActive, navigationRequestId]);
  useEffect(() => {
    if (!isActive || consumedRequests.current.has(navigationRequestId) || (!focus && !productId && !accountId)) return;
    let cancelled = false;
    void refetchOverview().then(result => {
      if (cancelled) return;
      setPolicySource(null); setManageReservations(false); setReservationSource(null); setProductFormAccountId(null); setDetailProductId(null); setReceiptRequestId(undefined); setFocusReservationId(undefined);
      if (result.isError || !result.data) { setNavigationMessage(t("upcomingActions.loadError")); return; }
      consumedRequests.current.add(navigationRequestId);
      setNavigationMessage(undefined);
      const target = focus ?? (productId ? { kind: "product" as const, productId } : undefined);
      const freshSources = result.data.sources ?? [];
      const sameSource = (source: LiquiditySourceDTO, ref: LiquiditySourceDTO["sourceRef"]) => source.sourceRef.kind === ref.kind && source.sourceRef.accountId === ref.accountId && source.sourceRef.holdingId === ref.holdingId && (ref.kind !== "account_cash" || source.sourceRef.currency === ref.currency);
      if (target?.kind === "product" || target?.kind === "receipt") {
        const exists = freshSources.some(source => source.productId === target.productId);
        if (target.kind === "product" || exists) setDetailProductId(target.productId);
        if (target.kind === "receipt" && exists) setReceiptRequestId(navigationRequestId);
        if (!exists) setNavigationMessage(t("upcomingActions.stateChanged"));
      } else if (target?.kind === "policy") {
        const source = freshSources.find(source => sameSource(source, target.sourceRef));
        if (source) setPolicySource(source); else setNavigationMessage(t("upcomingActions.targetMissing"));
      } else if (target?.kind === "reservations") {
        const source = target.sourceRef ? freshSources.find(source => sameSource(source, target.sourceRef!)) : undefined;
        setReservationSource(source ?? null); setFocusReservationId(target.reservationId); setManageReservations(true);
        if (target.sourceRef && !source) setNavigationMessage(t("upcomingActions.stateChanged"));
      } else if (accountId) setProductFormAccountId(accountId);
    });
    return () => { cancelled = true; };
  }, [focus, navigationRequestId, isActive, productId, accountId, refetchOverview, navigationRetry, t]);
  const retryOverview = () => {
    if (isActive && (focus || productId || accountId) && !consumedRequests.current.has(navigationRequestId)) {
      setNavigationRetry(value => value + 1);
    } else { void refetchOverview(); }
  };
  const [previousIsActive, setPreviousIsActive] = useState(isActive);
  // Adjust retained sheet state during the page transition, before displaying it again.
  if (previousIsActive !== isActive) {
    setPreviousIsActive(isActive);
    if (!isActive) {
      setPolicySource(null); setManageReservations(false); setChooseAccount(false);
      setProductFormAccountId(null); setDetailProductId(null); setReceiptRequestId(undefined); setFocusReservationId(undefined);
    }
  }
  const unknown = t("availableFunds.unknownAmount");
  const filterLabels: Record<SourceFilter, string> = {
    all: t("availableFunds.filterAll"), available: t("availableFunds.filterAvailable"), locked: t("availableFunds.filterLocked"),
    needs_info: t("availableFunds.filterNeedsInfo"), excluded: t("availableFunds.filterExcluded"),
  };
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
          onRetry={retryOverview}
          retryLabel={t("common.retryAction")}
        />
      </>
    );
  }

  const today = buckets[0];
  const week = buckets[1];
  const month = buckets[2];
  const custom = buckets.find((bucket) => customDate && bucket.horizonOn === customDate);
  const counts = sourceFilterCounts(sources, horizonOn);
  const filterOptions = (["all", "available", "locked", "needs_info", "excluded"] as const)
    .filter((value) => value === "all" || value === filter || counts[value] > 0)
    .map((value) => ({
      value,
      label: <>{filterLabels[value]}{" "}<span className="ml-0.5 tabular-nums opacity-70">{counts[value]}</span></>,
    }));
  const openReservations = (source: LiquiditySourceDTO | null) => { setReservationSource(source); setManageReservations(true); };
  const notes = <>
    <p>{t("availableFunds.description")}</p>
    <p>{t("availableFunds.assetsOnly")}</p>
    {(data.assumptions ?? []).map((assumption) => <p key={assumption}>{t(`availableFunds.${assumption}`, { defaultValue: t("availableFunds.missingInputs") })}</p>)}
  </>;

  return (
    <div className="flex flex-col gap-5" aria-busy={overview.isFetching} data-testid="available-funds-page">
      {pageChrome}

      {sources.length === 0 ? (
        <EmptyState title={t("availableFunds.emptyTitle")} description={t("availableFunds.emptyDescription")} />
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-3">
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <CalendarClock className="size-4" aria-hidden="true" />
              {t("availableFunds.asOf", { date: data.localDate, timezone: data.timezone })}
            </p>
            <div className="flex flex-wrap items-center gap-3">
              <div className="flex items-center gap-2">
                <Label htmlFor="custom-horizon" className="text-sm font-medium">{t("availableFunds.customDate")}</Label>
                <div className="w-44">
                  <DatePicker clearable allowFuture min={data.localDate} id="custom-horizon" value={customDate} onChange={(next) => { setCustomDate(next); setSelectedHorizon(next || undefined); }} />
                </div>
              </div>
              <EarlyWithdrawalToggle checked={includeEarly} onChange={setIncludeEarly} />
            </div>
          </div>
          <FundsComparison
            horizons={[
              ...(today ? [{ label: t("availableFunds.today"), bucket: today }] : []),
              ...(week ? [{ label: t("availableFunds.within7"), bucket: week }] : []),
              ...(month ? [{ label: t("availableFunds.within30"), bucket: month }] : []),
              ...(custom && ![today, week, month].some(bucket => bucket?.horizonOn === custom.horizonOn) ? [{ label: t("availableFunds.customDate"), bucket: custom }] : []),
            ]}
            selected={horizonOn} onSelect={setSelectedHorizon} sources={sources}
            baseCurrency={data.baseCurrency} includeEarly={includeEarly} busy={overview.isPlaceholderData} notes={notes}
          />
          {overview.isPlaceholderData && <p role="status" className="text-sm text-muted-foreground">{t("availableFunds.updatingEstimate")}</p>}
          {allExcluded && <EmptyState title={t("availableFunds.allExcludedTitle")} description={t("availableFunds.assetsOnly")} />}
          {!allExcluded && allLocked && <EmptyState title={t("availableFunds.allLockedTitle")} />}

          <section className="flex flex-col gap-3" aria-label={t("availableFunds.assets")}>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <h2 className="font-semibold">{t("availableFunds.assetsByDate", { date: horizonOn })}</h2>
                <InfoPopover label={t("availableFunds.aboutAmounts")}>{t("availableFunds.nativeCurrencyNote")}</InfoPopover>
              </div>
              <Button type="button" size="sm" variant="outline" onClick={() => openReservations(null)}>{t("availableFunds.manageReservations")}</Button>
            </div>
            {selectedBucket?.status !== "complete" && !allExcluded && (
              <div role="status" className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-warning/40 bg-warning/12 px-3 py-2 text-sm text-warning-foreground">
                <span className="flex items-center gap-2"><TriangleAlert className="size-4 shrink-0" aria-hidden="true" />{t("availableFunds.needsInfoTitle")}</span>
                {filter !== "needs_info" && counts.needs_info > 0 && <Button type="button" size="sm" variant="outline" onClick={() => setFilter("needs_info")}>{t("availableFunds.showNeedsInfo")}</Button>}
              </div>
            )}
            <SegmentedControl label={t("availableFunds.assets")} options={filterOptions} value={filter} onChange={(value) => setFilter(value as SourceFilter)} className="self-start" />
            {visibleSources.length === 0 ? (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
                <span>{t("availableFunds.noSources")}</span>
                <Button type="button" size="sm" variant="outline" onClick={() => setFilter("all")}>{t("availableFunds.showAllSources")}</Button>
              </div>
            ) : (
              <div className="overflow-hidden rounded-2xl border border-border bg-card">
                <SourceTable
                  sources={visibleSources} horizonOn={horizonOn} includeEarly={includeEarly} accountNames={accountNames}
                  onEditRules={setPolicySource} onManageReservations={openReservations} onOpenProduct={setDetailProductId}
                />
              </div>
            )}
          </section>
        </>
      )}

      {navigationMessage && <p role="status">{navigationMessage}</p>}
      {(data.unresolvedReservations ?? []).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{t("availableFunds.unresolvedReservations")}</CardTitle>
          </CardHeader>
          <CardContent className="text-sm">
            {(data.unresolvedReservations ?? []).map((item) => (
              <p key={item.reservation.id}>{item.reservation.label}: {formatKnownOrUnknown(item.reservation.amount, unknown)} · {t("availableFunds.unresolvedSource")} <Button variant="link" onClick={() => openReservations(null)}>{t("availableFunds.manageReservations")}</Button></p>
            ))}
          </CardContent>
        </Card>
      )}

      <PolicySheet key={policySource?.sourceKey ?? "policy"} source={policySource} open={isActive && Boolean(policySource)} onOpenChange={(open) => { if (!open) setPolicySource(null); }} />
      {manageReservations && <ReservationManager key={reservationSource?.sourceKey ?? "reservation"} source={reservationSource} focusReservationId={focusReservationId} sources={sources} accountNames={Object.fromEntries(accountNames)} open={isActive && manageReservations} onOpenChange={(open) => { setManageReservations(open); if (!open) setReservationSource(null); }} />}
      <Sheet open={isActive && chooseAccount} onOpenChange={setChooseAccount}><SheetContent><SheetHeader><SheetTitle>{t("availableFunds.chooseAccount")}</SheetTitle></SheetHeader>
        {eligibleAccounts.map(({ account }) => <Button key={account.id} variant="outline" onClick={() => { setChooseAccount(false); setProductFormAccountId(account.id); }}>{account.name}</Button>)}
      </SheetContent></Sheet>
      {productFormAccountId && (
        <ProductFormSheet
          key={productFormAccountId}
          accountName={accountNames.get(productFormAccountId)}
          accountId={productFormAccountId}
          currency={(accounts.data ?? []).find((record) => record.account.id === productFormAccountId)?.account.defaultCurrency ?? data.baseCurrency}
          open={isActive && Boolean(productFormAccountId)}
          onOpenChange={(open) => { if (!open) setProductFormAccountId(null); }}
        />
      )}
      <ProductDetailSheet key={detailProductId ?? "product"} productId={detailProductId} receiptRequestId={receiptRequestId} open={isActive && Boolean(detailProductId)} onOpenChange={(open) => { if (!open) { setDetailProductId(null); setReceiptRequestId(undefined); } }} />
    </div>
  );
}
