import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { ChangeText } from "@/components/ui/change-text";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { ContributionRanking, rankedContributions } from "./ContributionRanking";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { NativeSelect } from "@/components/ui/select";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useContribution, useContributionItem } from "@/queries/returnAnalysis";
import type { ContributionDTO, ContributionItemDTO, ContributionRowDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { AnalysisSessionState } from "@/stores/analysis";
import type { HistoryNavigationFilters } from "@/app/navigation";
import { useAnalysisProjectionContext } from "@/features/insights/analysisProjectionContext";
import { AvailabilityMarks } from "@/features/insights/CompletenessBanner";
import { analysisCashLabel, analysisReason } from "@/features/insights/analysisText";
import { compareCanonical, formatSignedMoney, formatDecimalRatioPercent } from "@/lib/money";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";
import { instrumentDisplayLabels } from "@/lib/instrumentDisplay";

const returnComponentKeys: Record<string, string> = {
  price_change: "priceChange",
  fx_impact: "fxImpact",
  fx_conversion_spread: "fxConversionSpread",
  dividend_interest: "dividendInterest",
  investment_fee: "investmentFee",
  cash_fx_impact: "cashFxImpact",
  residual: "residual",
};

function amountText(value?: SignedMoneyView | null): string {
  if (!value) return "—";
  return formatSignedMoney(value.amount, value.currency);
}

function rateText(value: string | null | undefined): string {
  return formatDecimalRatioPercent(value, 2, true);
}

function coverageMark(ratedDays: number, totalDays: number) {
  return ratedDays < totalDays ? <sup className="ml-0.5 text-warning-foreground">◇</sup> : null;
}

function componentLabel(t: (key: string) => string, key: string): string {
  const name = returnComponentKeys[key];
  return name ? t(`insights.components.${name}`) : key;
}

function contributionRowLabel(t: (key: string) => string, groupBy: string, key: string, label: string, accountNames: Map<string, string>, instrumentNames: Map<string, string>): string {
  if (groupBy === "account") return accountNames.get(key) ?? label;
  if (groupBy === "instrument") return analysisCashLabel(t, key) ?? instrumentNames.get(key) ?? label;
  return key === "cash" ? t("insights.cash") : key;
}

function contributionEmpty(data: ContributionDTO | undefined, returnType: string, t: (key: string) => string) {
  if (returnType === "unrealized") return <EmptyState title={t("insights.unrealizedUnavailable")} description={analysisReason(t, data?.missingReason) ?? t("insights.unrealizedUnavailableHint")} />;
  return <EmptyState title={t("insights.noContributionData")} description={analysisReason(t, data?.missingReason) ?? t("insights.noContributionDataHint")} />;
}

function ContributionRow({ row, label, showRate, onSelect }: { row: ContributionRowDTO; label: string; showRate: boolean; onSelect: () => void }) {
  const { t } = useTranslation();
  const reason = row.status !== "ok" ? analysisReason(t, row.missingReason) : undefined;
  const direction = row.amount ? compareCanonical(row.amount.amount, "0") : 0;
  return <li><Button type="button" variant="ghost" className="h-auto w-full justify-between gap-4 rounded-md px-2 py-2 text-left" onClick={onSelect}>
    <span className="min-w-0 flex-1"><span className="block truncate">{label}</span>{reason && <span className="block text-xs text-warning-foreground">{reason}</span>}{row.status === "partial" && <Badge variant="warning">{t("insights.partial")}</Badge>}{row.status === "unavailable" && <Badge variant="warning">{t("insights.status.unavailable")}</Badge>}</span>
    <span className="flex shrink-0 items-center gap-4 num">{row.amount ? <ChangeText direction={direction > 0 ? "up" : direction < 0 ? "down" : "flat"}>{amountText(row.amount)}</ChangeText> : <span className="text-muted-foreground">{t("insights.unavailableAmount")}</span>}{showRate && <span className="w-20 text-right text-muted-foreground">{rateText(row.rate)}{coverageMark(row.ratedDays, row.totalDays)}</span>}</span>
  </Button></li>;
}

function compositionLabel(t: (key: string) => string, item: NonNullable<ContributionItemDTO["components"]>[number], accountNames: Map<string, string>, instrumentNames: Map<string, string>): string {
  if (returnComponentKeys[item.key]) return componentLabel(t, item.key);
  if (item.instrumentId) return instrumentNames.get(item.instrumentId) ?? t("history.unknownInstrument");
  if (item.accountId) return accountNames.get(item.accountId) ?? t("history.unknownAccount");
  return item.key === "cash" ? t("insights.cash") : t("insights.unattributedResidual");
}

function ItemDetail({ data, returnType, accountNames, instrumentNames, showHistoryFilterWarning, onOpenHistory }: { data: ContributionItemDTO; returnType: string; accountNames: Map<string, string>; instrumentNames: Map<string, string>; showHistoryFilterWarning: boolean; onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const showRate = returnType === "total_return" || (returnType === "unrealized" && data.rate != null);
  const openHistory = () => {
    if (!onOpenHistory) return;
    onOpenHistory({ from: data.historyHint.from, to: data.historyHint.to, accountId: data.historyHint.accountId || undefined, instrumentId: data.historyHint.instrumentId || undefined, kinds: data.historyHint.kinds && data.historyHint.kinds.length > 0 ? data.historyHint.kinds : undefined });
  };
  const historyNarrowed = Boolean(data.historyHint.accountId || data.historyHint.instrumentId);
  return <div className="flex flex-col gap-5 overflow-y-auto"><div><p className="text-sm text-muted-foreground">{t("insights.contribution")}</p><p className="mt-1 text-2xl font-semibold">{data.amount ? amountText(data.amount) : t("insights.unavailableAmount")}</p>{showRate && <p className="text-sm text-muted-foreground">{rateText(data.rate)}{coverageMark(data.ratedDays, data.totalDays)}</p>}</div>{data.valuationForced && <Badge variant="warning">{t("insights.valuationForced")}</Badge>}{(data.status === "partial" || data.status === "unavailable") && <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{analysisReason(t, data.missingReason) ?? t(data.status === "unavailable" ? "insights.unavailableAmount" : "insights.partial")}</p>}<p className="text-sm text-muted-foreground">{t("insights.contributionIndependent")}</p><DetailList title={t("insights.composition")} values={data.components} labelFor={(item) => compositionLabel(t, item, accountNames, instrumentNames)} /><DetailList title={t("insights.byAccount")} values={data.byAccount} labelFor={(item) => accountNames.get(item.accountId || item.key) ?? (item.key === "cash" ? t("insights.cash") : t("insights.unattributedResidual"))} />{onOpenHistory && <div className="flex flex-col gap-2">{showHistoryFilterWarning && <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{t("insights.historyContributionFilterWider")}</p>}{!historyNarrowed && (data.byAccount?.length ?? 0) > 1 && <p className="text-sm text-muted-foreground">{t("insights.historyRangeWider")}</p>}<Button type="button" variant="outline" onClick={openHistory}>{t("insights.viewInHistory")}</Button></div>}</div>;
}

function DetailList({ title, values, labelFor }: { title: string; values: ContributionItemDTO["components"]; labelFor: (item: NonNullable<ContributionItemDTO["components"]>[number]) => string }) {
  const { t } = useTranslation();
  if (!values || values.length === 0) return null;
  return <div className="flex flex-col gap-2"><h3 className="text-sm font-medium">{title}</h3><ul className="flex flex-col gap-1 text-sm">{values.map((item) => <li key={item.key} className="flex justify-between gap-3"><span className="truncate">{labelFor(item)}</span><span className="shrink-0">{item.amount ? amountText(item.amount) : t("insights.unavailableAmount")}</span></li>)}</ul></div>;
}

export function ContributionTab({ session, onOpenHistory }: { session: AnalysisSessionState; onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const returnType = session.contributionReturnType;
  const [groupBy, setGroupBy] = useState("instrument");
  const [ordering, setOrdering] = useState("amount_desc");
  const [direction, setDirection] = useState("all");
  const [limit, setLimit] = useState("10");
  const [search, setSearch] = useState("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const effectiveGroupBy = returnType === "realized" && (groupBy === "currency" || groupBy === "asset_class") ? "instrument" : groupBy;
  const effectiveOrdering = returnType !== "total_return" && (ordering === "rate_desc" || ordering === "rate_asc") ? "amount_desc" : ordering;
  const context = useAnalysisProjectionContext(session);
  const contribution = useContribution(context.request, returnType, effectiveGroupBy, effectiveOrdering, context.enabled);
  const item = useContributionItem(context.request, returnType, effectiveGroupBy, selectedKey ?? "", Boolean(selectedKey) && context.enabled);
  const accounts = useAccounts();
  const instruments = useInstruments();
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = instrumentDisplayLabels(instruments.data ?? []);
  const data = contribution.data;
  const showHistoryFilterWarning = [session.moreFilters.currency, session.moreFilters.assetClass].some((value) => typeof value === "string" && value.trim().length > 0);
  const showRate = returnType === "total_return";
  const labelFor = (row: { key: string; label: string }) => contributionRowLabel(t, effectiveGroupBy, row.key, row.label, accountNames, instrumentNames);
  const ranked = rankedContributions(data?.rows ?? [], direction, limit);
  const details = (data?.rows ?? []).filter((row) => labelFor(row).toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()));
  const selected = (data?.rows ?? []).find((row) => row.key === selectedKey) ?? null;
  if (selectedKey && data && !selected) setSelectedKey(null);

  if (context.origin.isLoading) return <LoadingState label={t("insights.loading")} />;
  if (context.origin.isError) return <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => context.origin.refetch()} retryLabel={t("common.retryAction")} />;
  if (!context.origin.data) return <EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} />;
  if (!context.scopeReady) return <EmptyState title={t("insights.scopeRequired")} description={t("insights.scopeRequiredHint")} />;
  if (!context.rangeAvailable) return <EmptyState title={t(context.awaitingClose ? "insights.currentMonthUnclosed" : "insights.historyInsufficient")} description={t(context.awaitingClose ? "insights.currentMonthUnclosedHint" : "insights.historyInsufficientHint")} />;

  const sortControl = (
      <div className="flex flex-col gap-1.5">
        <label htmlFor="contribution-sort" className="sr-only">{t("insights.sort")}</label>
        <NativeSelect id="contribution-sort" value={effectiveOrdering} onChange={(event) => setOrdering(event.target.value)}>
          <option value="amount_desc">{t("insights.sortAmountDesc")}</option>
          <option value="amount_asc">{t("insights.sortAmountAsc")}</option>
          {showRate && <option value="rate_desc">{t("insights.sortRateDesc")}</option>}
          {showRate && <option value="rate_asc">{t("insights.sortRateAsc")}</option>}
          <option value="name_asc">{t("insights.sortNameAsc")}</option>
        </NativeSelect>
      </div>
  );

  const toolbar = (
    <div className="flex flex-wrap items-end gap-3 rounded-xl border border-border bg-card/60 px-4 py-2">
      <div className="flex flex-col gap-1.5">
        <label htmlFor="contribution-return-type" className="text-sm font-medium">{t("insights.returnType")}</label>
        <NativeSelect id="contribution-return-type" value={returnType} onChange={(event) => {
          const next = event.target.value;
          session.setFilters({ returnType: next as AnalysisSessionState["contributionReturnType"] });
          setSelectedKey(null);
          if (next === "realized" && (groupBy === "currency" || groupBy === "asset_class")) setGroupBy("instrument");
          if (next !== "total_return" && (ordering === "rate_desc" || ordering === "rate_asc")) setOrdering("amount_desc");
        }}>
          <option value="total_return">{t("insights.totalReturn")}</option>
          <option value="realized">{t("insights.realizedGain")}</option>
          <option value="dividend_interest">{t("insights.dividendInterestView")}</option>
        </NativeSelect>
      </div>
      <div className="flex flex-col gap-1.5">
        <label htmlFor="contribution-group-by" className="text-sm font-medium">{t("insights.groupBy")}</label>
        <NativeSelect id="contribution-group-by" value={effectiveGroupBy} onChange={(event) => { setGroupBy(event.target.value); setSelectedKey(null); }}>
          <option value="instrument">{t("insights.byInstrument")}</option>
          <option value="account">{t("insights.byAccount")}</option>
          {returnType !== "realized" && <option value="currency">{t("insights.byCurrency")}</option>}
          {returnType !== "realized" && <option value="asset_class">{t("insights.byAssetClass")}</option>}
        </NativeSelect>
      </div>
      {!data?.available && sortControl}
      <p className="ml-auto max-w-md text-xs text-muted-foreground">{t("insights.contributionIndependent")}</p>
    </div>
  );

  let results;
  if (contribution.isLoading) results = <LoadingState label={t("insights.loading")} />;
  else if (contribution.isError) results = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => contribution.refetch()} retryLabel={t("common.retryAction")} />;
  else if (!data?.available) results = contributionEmpty(data, returnType, t);
  else results = (
    <>
      <div className="grid gap-3 sm:grid-cols-3" aria-label={t("insights.contributionSummary")}>
        {([["total", "selectedContributionTotal"], ["positive", "positiveContributionTotal"], ["negative", "negativeContributionTotal"]] as const).map(([key, title]) => {
          const amount = data.summary?.[key];
          const sign = amount ? compareCanonical(amount.amount, "0") : 0;
          return <Card key={key}><CardContent className="pt-4 pb-4"><p className="text-sm text-muted-foreground">{t(`insights.${title}`)}</p>{amount ? <ChangeText direction={sign > 0 ? "up" : sign < 0 ? "down" : "flat"} className="mt-1 text-xl font-semibold num">{amountText(amount)}</ChangeText> : <p className="mt-1 text-xl font-semibold text-muted-foreground">{t("insights.unavailableAmount")}</p>}{data.summary?.amountStatus === "partial" && <p className="mt-1 text-xs text-warning-foreground">{t("insights.knownContributionSubtotal")}</p>}</CardContent></Card>;
        })}
      </div>
      <Card>
        <CardHeader className="flex-row flex-wrap items-center justify-between gap-3">
          <div><CardTitle>{t("insights.mainContributions")}</CardTitle><p className="mt-1 text-xs text-muted-foreground">{t("insights.absoluteContributionOrder")}</p></div>
          <div className="flex flex-wrap items-center gap-3"><SegmentedControl label={t("insights.contributionDirection")} value={direction} onChange={setDirection} options={[{ value: "all", label: t("insights.largestImpact") }, { value: "positive", label: t("insights.positiveContributions") }, { value: "negative", label: t("insights.negativeContributions") }]} /><NativeSelect aria-label={t("insights.contributionLimit")} value={limit} onChange={(event) => setLimit(event.target.value)}><option value="10">{t("insights.showContributionCount", { count: 10 })}</option><option value="20">{t("insights.showContributionCount", { count: 20 })}</option><option value="all">{t("common.all")}</option></NativeSelect></div>
        </CardHeader>
        <CardContent>
          <AvailabilityMarks status={data.status} missingReason={data.missingReason} valuationForced={data.valuationForced} ratedDays={data.ratedDays} totalDays={data.totalDays} />
          <ContributionRanking rows={ranked} labelFor={labelFor} label={t("insights.mainContributions")} onSelect={setSelectedKey} />
          {ranked.length === 0 && <EmptyState size="compact" title={t("insights.noContributionData")} />}
          <p className="mt-3 text-xs text-muted-foreground" role="status">{t("insights.contributionDisplayScope", { shown: ranked.length, total: (data.rows ?? []).length, hidden: (data.rows ?? []).length - ranked.length })}</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="flex-row flex-wrap items-end justify-between gap-3"><CardTitle>{t("insights.allContributionDetails")}</CardTitle><div className="flex flex-wrap items-end gap-3"><Input aria-label={t("insights.searchContributions")} placeholder={t("insights.searchContributions")} value={search} onChange={(event) => setSearch(event.target.value)} className="w-52" />
          {sortControl}
        </div></CardHeader>
        <CardContent><div className="flex justify-between gap-4 border-b border-border pb-2 text-xs text-muted-foreground"><span>{t(`insights.${effectiveGroupBy === "instrument" ? "instrument" : effectiveGroupBy === "account" ? "byAccount" : effectiveGroupBy === "currency" ? "currency" : "assetClass"}`)}</span><span>{t("insights.contribution")}{showRate && ` / ${t("insights.returnRate")}`}</span></div><ul className="divide-y divide-border" aria-label={t("insights.contribution")}>{details.map((row) => <ContributionRow key={row.key} row={row} label={labelFor(row)} showRate={showRate} onSelect={() => setSelectedKey(row.key)} />)}</ul>{details.length === 0 && <EmptyState size="compact" title={t("insights.noContributionData")} />}</CardContent>
      </Card>
      <Sheet open={Boolean(selected)} onOpenChange={(open) => { if (!open) setSelectedKey(null); }}>
        <SheetContent side="right">
          <SheetHeader><SheetTitle>{selected ? labelFor(selected) : t("insights.contributionDetail")}</SheetTitle></SheetHeader>
          {item.isLoading ? <LoadingState label={t("insights.loading")} /> : item.isError ? <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => item.refetch()} retryLabel={t("common.retryAction")} /> : item.data ? <ItemDetail data={item.data} returnType={returnType} accountNames={accountNames} instrumentNames={instrumentNames} showHistoryFilterWarning={showHistoryFilterWarning} onOpenHistory={onOpenHistory ? filters => { setSelectedKey(null); onOpenHistory(filters); } : undefined} /> : <EmptyState title={t("insights.noContributionData")} />}
        </SheetContent>
      </Sheet>
    </>
  );

  return <div className="flex flex-col gap-4" data-testid="contribution" aria-busy={contribution.isFetching}>{toolbar}{results}{contribution.isPlaceholderData && <span className="sr-only" role="status">{t("insights.loading")}</span>}</div>;
}
