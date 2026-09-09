import { useState } from "react";
import { useTranslation } from "react-i18next";
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
import { analysisReason } from "@/features/insights/analysisText";
import { compareCanonical, divideCanonical, formatAmount, multiplyCanonical } from "@/lib/money";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";

const returnComponentKeys: Record<string, string> = {
  price_change: "priceChange",
  fx_impact: "fxImpact",
  dividend_interest: "dividendInterest",
  investment_fee: "investmentFee",
  cash_fx_impact: "cashFxImpact",
  residual: "residual",
};

function amountText(value?: SignedMoneyView | null): string {
  if (!value) return "—";
  const formatted = formatAmount(value.amount, value.currency);
  return value.amount.startsWith("-") || value.amount === "0" ? formatted : `+${formatted}`;
}

function rateText(value: string | null | undefined): string {
  if (value == null) return "—";
  const numeric = Number(value) * 100;
  if (!Number.isFinite(numeric)) return "—";
  const rounded = numeric.toFixed(2).replace(/\.00$/, "").replace(/(\.\d)0$/, "$1");
  return numeric > 0 ? `+${rounded}%` : `${rounded}%`;
}

function coverageMark(ratedDays: number, totalDays: number) {
  return ratedDays < totalDays ? <sup className="ml-0.5 text-warning-foreground">◇</sup> : null;
}

function returnTypeLabel(t: (key: string) => string, value: string): string {
  const key = value === "total_return" ? "totalReturn" : value === "realized" ? "realizedGain" : value === "unrealized" ? "unrealizedGain" : "dividendInterestView";
  return t(`insights.${key}`);
}

function componentLabel(t: (key: string) => string, key: string): string {
  const name = returnComponentKeys[key];
  return name ? t(`insights.components.${name}`) : key;
}

function contributionRowLabel(t: (key: string) => string, groupBy: string, key: string, label: string, accountNames: Map<string, string>, instrumentNames: Map<string, string>): string {
  if (groupBy === "account") return accountNames.get(key) ?? label;
  if (groupBy === "instrument") return instrumentNames.get(key) ?? label;
  return key === "cash" ? t("insights.cash") : key;
}

function contributionEmpty(data: ContributionDTO | undefined, returnType: string, t: (key: string) => string) {
  if (returnType === "unrealized") return <EmptyState title={t("insights.unrealizedUnavailable")} description={analysisReason(t, data?.missingReason) ?? t("insights.unrealizedUnavailableHint")} />;
  return <EmptyState title={t("insights.noContributionData")} description={analysisReason(t, data?.missingReason) ?? t("insights.noContributionDataHint")} />;
}

function absoluteAmount(value: string): string {
  return value.startsWith("-") ? value.slice(1) : value;
}

function contributionBarWidth(row: ContributionRowDTO, rows: ContributionRowDTO[]): string {
  const amount = row.amount?.amount;
  if (!amount) return "0";
  const maximum = rows.reduce((current, candidate) => {
    const candidateAmount = candidate.amount?.amount;
    if (!candidateAmount) return current;
    return compareCanonical(absoluteAmount(candidateAmount), current) > 0 ? absoluteAmount(candidateAmount) : current;
  }, "0");
  if (maximum === "0") return "0";
  return multiplyCanonical(divideCanonical(absoluteAmount(amount), maximum, 6), "100", 2);
}

function ContributionRow({ row, rows, label, showRate, onSelect }: { row: ContributionRowDTO; rows: ContributionRowDTO[]; label: string; showRate: boolean; onSelect: () => void }) {
  const { t } = useTranslation();
  const partial = row.status === "partial" || row.ratedDays < row.totalDays;
  return <li><Button type="button" variant="ghost" className="h-auto w-full justify-between gap-4 rounded-md px-3 py-2 text-left" onClick={onSelect}><span className="flex min-w-0 flex-1 items-center gap-2"><span className="h-1.5 min-w-8 max-w-40 flex-1 overflow-hidden rounded-full bg-muted" aria-hidden="true"><span className="block h-full rounded-full bg-primary/60" style={{ width: `${contributionBarWidth(row, rows)}%` }} /></span><span className="min-w-0 truncate">{label}</span>{partial && <Badge variant="warning">{t("insights.partial")}</Badge>}</span><span className="flex shrink-0 items-center gap-4"><span>{row.amount ? amountText(row.amount) : t("insights.unavailableAmount")}</span>{showRate && <span className="w-20 text-right text-muted-foreground">{rateText(row.rate)}{coverageMark(row.ratedDays, row.totalDays)}</span>}</span></Button></li>;
}

function compositionLabel(t: (key: string) => string, item: NonNullable<ContributionItemDTO["components"]>[number], accountNames: Map<string, string>, instrumentNames: Map<string, string>): string {
  if (returnComponentKeys[item.key]) return componentLabel(t, item.key);
  if (item.instrumentId) return instrumentNames.get(item.instrumentId) ?? item.key;
  if (item.accountId) return accountNames.get(item.accountId) ?? item.key;
  return item.key === "cash" ? t("insights.cash") : item.key;
}

function ItemDetail({ data, returnType, accountNames, instrumentNames, onOpenHistory }: { data: ContributionItemDTO; returnType: string; accountNames: Map<string, string>; instrumentNames: Map<string, string>; onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const showRate = returnType === "total_return" || (returnType === "unrealized" && data.rate != null);
  const openHistory = () => {
    if (!onOpenHistory) return;
    onOpenHistory({ from: data.historyHint.from, to: data.historyHint.to, accountId: data.historyHint.accountId || undefined, instrumentId: data.historyHint.instrumentId || undefined, kinds: data.historyHint.kinds && data.historyHint.kinds.length > 0 ? data.historyHint.kinds : undefined });
  };
  const historyNarrowed = Boolean(data.historyHint.accountId || data.historyHint.instrumentId);
  return <div className="flex flex-col gap-5 overflow-y-auto"><div><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("insights.contribution")}</p><p className="mt-1 text-2xl font-semibold">{amountText(data.amount)}</p>{showRate && <p className="text-sm text-muted-foreground">{rateText(data.rate)}{coverageMark(data.ratedDays, data.totalDays)}</p>}</div>{data.valuationForced && <Badge variant="warning">{t("insights.valuationForced")}</Badge>}{data.status === "partial" && <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{analysisReason(t, data.missingReason) ?? t("insights.partial")}</p>}<p className="text-sm text-muted-foreground">{t("insights.contributionIndependent")}</p><DetailList title={t("insights.composition")} values={data.components} labelFor={(item) => compositionLabel(t, item, accountNames, instrumentNames)} /><DetailList title={t("insights.byAccount")} values={data.byAccount} labelFor={(item) => accountNames.get(item.accountId || item.key) ?? (item.key === "cash" ? t("insights.cash") : item.key)} />{onOpenHistory && <div className="flex flex-col gap-2">{!historyNarrowed && (data.byAccount?.length ?? 0) > 1 && <p className="text-sm text-muted-foreground">{t("insights.historyRangeWider")}</p>}<Button type="button" variant="outline" onClick={openHistory}>{t("insights.viewInHistory")}</Button></div>}</div>;
}

function DetailList({ title, values, labelFor }: { title: string; values: ContributionItemDTO["components"]; labelFor: (item: NonNullable<ContributionItemDTO["components"]>[number]) => string }) {
  if (!values || values.length === 0) return null;
  return <div className="flex flex-col gap-2"><h3 className="text-sm font-medium">{title}</h3><ul className="flex flex-col gap-1 text-sm">{values.map((item) => <li key={item.key} className="flex justify-between gap-3"><span className="truncate">{labelFor(item)}</span><span className="shrink-0">{amountText(item.amount)}</span></li>)}</ul></div>;
}

export function ContributionTab({ session, onOpenHistory }: { session: AnalysisSessionState; onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const [returnType, setReturnType] = useState<string>(session.contributionReturnType || "total_return");
  const [groupBy, setGroupBy] = useState("instrument");
  const [ordering, setOrdering] = useState("amount_desc");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const context = useAnalysisProjectionContext(session);
  const contribution = useContribution(context.request, returnType, groupBy, ordering, context.enabled);
  const item = useContributionItem(context.request, returnType, groupBy, selectedKey ?? "", Boolean(selectedKey) && context.enabled);
  const accounts = useAccounts();
  const instruments = useInstruments();
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));
  const data = contribution.data;
  const showRate = returnType === "total_return" || (returnType === "unrealized" && (data?.rows ?? []).some((row) => row.rate != null));
  const labelFor = (row: { key: string; label: string }) => contributionRowLabel(t, groupBy, row.key, row.label, accountNames, instrumentNames);
  const selected = (data?.rows ?? []).find((row) => row.key === selectedKey) ?? null;
  if (selectedKey && data && !selected) setSelectedKey(null);

  if (context.origin.isLoading) return <LoadingState label={t("insights.loading")} />;
  if (context.origin.isError) return <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => context.origin.refetch()} retryLabel={t("common.retryAction")} />;
  if (!context.origin.data) return <EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} />;
  if (!context.scopeReady) return <EmptyState title={t("insights.scopeRequired")} description={t("insights.scopeRequiredHint")} />;
  if (!context.rangeAvailable) return <EmptyState title={t("insights.historyInsufficient")} description={t("insights.historyInsufficientHint")} />;

  const toolbar = (
    <div className="flex flex-wrap items-end gap-3 rounded-xl border border-border bg-card/60 p-4">
      <div className="flex flex-col gap-1.5">
        <label htmlFor="contribution-return-type" className="text-sm font-medium">{t("insights.returnType")}</label>
        <NativeSelect id="contribution-return-type" value={returnType} onChange={(event) => {
          const next = event.target.value;
          setReturnType(next);
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
        <NativeSelect id="contribution-group-by" value={groupBy} onChange={(event) => { setGroupBy(event.target.value); setSelectedKey(null); }}>
          <option value="instrument">{t("insights.byInstrument")}</option>
          <option value="account">{t("insights.byAccount")}</option>
          {returnType !== "realized" && <option value="currency">{t("insights.byCurrency")}</option>}
          {returnType !== "realized" && <option value="asset_class">{t("insights.byAssetClass")}</option>}
        </NativeSelect>
      </div>
      <div className="flex flex-col gap-1.5">
        <label htmlFor="contribution-sort" className="text-sm font-medium">{t("insights.sort")}</label>
        <NativeSelect id="contribution-sort" value={ordering} onChange={(event) => setOrdering(event.target.value)}>
          <option value="amount_desc">{t("insights.sortAmountDesc")}</option>
          <option value="amount_asc">{t("insights.sortAmountAsc")}</option>
          {showRate && <option value="rate_desc">{t("insights.sortRateDesc")}</option>}
          {showRate && <option value="rate_asc">{t("insights.sortRateAsc")}</option>}
          <option value="name_asc">{t("insights.sortNameAsc")}</option>
        </NativeSelect>
      </div>
      <p className="max-w-md text-sm text-muted-foreground">{t("insights.contributionIndependent")}</p>
    </div>
  );

  let results;
  if (contribution.isLoading) results = <LoadingState label={t("insights.loading")} />;
  else if (contribution.isError) results = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => contribution.refetch()} retryLabel={t("common.retryAction")} />;
  else if (!data?.available) results = contributionEmpty(data, returnType, t);
  else results = (
    <>
      <Card>
        <CardHeader className="flex-row items-center justify-between gap-3">
          <CardTitle>{returnTypeLabel(t, returnType)}</CardTitle>
          <AvailabilityMarks status={data.status} missingReason={data.missingReason} valuationForced={data.valuationForced} ratedDays={data.ratedDays} totalDays={data.totalDays} />
        </CardHeader>
        <CardContent>
          <ul className="divide-y divide-border" aria-label={t("insights.contribution")}>
            {(data.rows ?? []).map((row) => <ContributionRow key={row.key} row={row} rows={data.rows ?? []} label={labelFor(row)} showRate={showRate} onSelect={() => setSelectedKey(row.key)} />)}
          </ul>
          {(data.rows ?? []).length === 0 && <EmptyState title={t("insights.noContributionData")} description={t("insights.noContributionDataHint")} />}
        </CardContent>
      </Card>
      <Sheet open={Boolean(selected)} onOpenChange={(open) => { if (!open) setSelectedKey(null); }}>
        <SheetContent side="right">
          <SheetHeader><SheetTitle>{selected ? labelFor(selected) : t("insights.contributionDetail")}</SheetTitle></SheetHeader>
          {item.isLoading ? <LoadingState label={t("insights.loading")} /> : item.isError ? <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => item.refetch()} retryLabel={t("common.retryAction")} /> : item.data ? <ItemDetail data={item.data} returnType={returnType} accountNames={accountNames} instrumentNames={instrumentNames} onOpenHistory={onOpenHistory} /> : <EmptyState title={t("insights.noContributionData")} />}
        </SheetContent>
      </Sheet>
    </>
  );

  return <div className="flex flex-col gap-4" data-testid="contribution">{toolbar}{results}</div>;
}
