import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { WaterfallChart } from "@/features/insights/WaterfallChart";
import { useAssetChange, useAssetDriverDetail } from "@/queries/returnAnalysis";
import type { AnalysisQueryRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis/models";
import type { AssetChangeDTO, AssetChangeGroupDTO, AssetChangeRowDTO, AssetDriverDetailDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { formatAmount } from "@/lib/money";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";
import type { AnalysisNavigationContext, HistoryNavigationFilters } from "@/app/navigation";
import type { AnalysisSessionState } from "@/stores/analysis";

const RETURN_DRIVER_BUCKETS = new Set(["price_change", "dividend_interest", "fx_impact", "fee"]);

function contributionReturnType(bucket: string): NonNullable<AnalysisNavigationContext["returnType"]> {
  return bucket === "dividend_interest" ? "dividend_interest" : "total_return";
}

function isReturnDriver(bucket?: string): boolean {
  return Boolean(bucket && RETURN_DRIVER_BUCKETS.has(bucket));
}

function amountText(value?: SignedMoneyView | null): string {
  if (!value) return "—";
  const formatted = formatAmount(value.amount, value.currency);
  return value.amount.startsWith("-") || value.amount === "0" ? formatted : `+${formatted}`;
}

function driverLabel(t: (key: string) => string, bucket: string, fallback: string): string {
  const key = ({
    external_flow: "externalFlows",
    income: "income",
    spending: "spending",
    dividend_interest: "components.dividendInterest",
    price_change: "components.priceChange",
    fx_impact: "components.fxImpact",
    fee: "components.investmentFee",
    liability_impact: "liabilityImpact",
    adjustment: "adjustments",
    residual: "components.residual",
  } as Record<string, string>)[bucket];
  return key ? t(`insights.${key}`) : fallback;
}

function groupLabel(t: (key: string) => string, key: string, fallback: string): string {
  const translation = ({ cash: "cashFlows", market: "market", other: "other", residual: "residualGroup" } as Record<string, string>)[key];
  return translation ? t(`insights.${translation}`) : fallback;
}

function SummaryCard({ data, scope }: { data: AssetChangeDTO; scope: "portfolio" | "account" | "instrument" }) {
  const { t } = useTranslation();
  const partial = data.status === "partial";
  const title = scope === "account" ? t("insights.accountValueChange") : scope === "instrument" ? t("insights.investmentAssetChange") : t("insights.netWorthChange");
  return (
    <Card>
      <CardHeader className="gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>{title}</CardTitle>
          <div className="flex flex-wrap items-center gap-2">
            {data.valuationForced && <Badge variant="warning">{t("insights.valuationForced")}</Badge>}
            {partial && <Badge variant="warning">{t("insights.partial")}</Badge>}
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          <SummaryValue label={t("insights.beginningValue")} value={amountText(data.summary.beginningValue)} />
          <SummaryValue label={t("insights.endingValue")} value={amountText(data.summary.endingValue)} />
          <SummaryValue label={t("insights.change")} value={amountText(data.summary.change)} emphasis />
        </div>
      </CardHeader>
      {partial && data.missingReason && <CardContent><p className="text-sm text-warning-foreground">{data.missingReason}</p></CardContent>}
    </Card>
  );
}

function SummaryValue({ label, value, emphasis = false }: { label: string; value: string; emphasis?: boolean }) {
  return <div><p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p><p className={emphasis ? "mt-1 text-2xl font-semibold" : "mt-1 font-medium"}>{value}</p></div>;
}

function DriverButton({ row, onSelect }: { row: AssetChangeRowDTO; onSelect: (row: AssetChangeRowDTO) => void }) {
  const { t } = useTranslation();
  const residual = row.bucket === "residual";
  return (
    <li>
      <Button type="button" variant="ghost" className={`h-auto w-full justify-between rounded-md px-3 py-2 text-left ${residual ? "border border-warning/40 bg-warning/5 hover:bg-warning/10" : ""}`} onClick={() => onSelect(row)}>
        <span className="truncate">{row.label}</span>
        <span className={row.amount?.amount.startsWith("-") ? "shrink-0 text-gain-negative" : "shrink-0 text-gain-positive"}>{amountText(row.amount) || t("insights.unavailableAmount")}</span>
      </Button>
    </li>
  );
}

function AttributionGroup({ group, onSelect }: { group: AssetChangeGroupDTO; onSelect: (row: AssetChangeRowDTO) => void }) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3 py-4">
        <CardTitle>{group.label}</CardTitle>
        <span className="font-semibold">{amountText(group.amount) || t("insights.unavailableAmount")}</span>
      </CardHeader>
      <CardContent className="pt-0">
        <ul className="divide-y divide-border" aria-label={group.label}>
          {(group.rows ?? []).map((row) => <DriverButton key={row.key} row={row} onSelect={onSelect} />)}
        </ul>
      </CardContent>
    </Card>
  );
}

function DimensionList({ title, rows, labelFor }: { title: string; rows: AssetDriverDetailDTO["byInstrument"]; labelFor?: (row: NonNullable<AssetDriverDetailDTO["byInstrument"]>[number]) => string }) {
  if (!rows || rows.length === 0) return null;
  return <div className="flex flex-col gap-2"><h3 className="text-sm font-medium">{title}</h3><ul className="flex flex-col gap-1 text-sm">{rows.map((row) => <li key={row.key} className="flex justify-between gap-3"><span className="truncate">{labelFor?.(row) ?? row.label}</span><span className="shrink-0">{amountText(row.amount)}</span></li>)}</ul></div>;
}

function residualLabel(item: NonNullable<AssetDriverDetailDTO["residualDetails"]>[number], accountNames: Map<string, string>, instrumentNames: Map<string, string>): string {
  const dimensions = [
    item.accountId ? accountNames.get(item.accountId) ?? item.accountId : "",
    item.instrumentId ? instrumentNames.get(item.instrumentId) ?? item.instrumentId : "",
  ].filter(Boolean);
  return dimensions.length > 0 ? dimensions.join(" · ") : item.componentKey;
}

function DriverDetailSheet({ request, session, selected, onClose, accountNames, instrumentNames, onOpenHistory, onOpenReturnAnalysis }: { request: AnalysisQueryRequest; session: AnalysisSessionState; selected: AssetChangeRowDTO | null; onClose: () => void; accountNames: Map<string, string>; instrumentNames: Map<string, string>; onOpenHistory?: (filters: HistoryNavigationFilters) => void; onOpenReturnAnalysis?: (analysis: AnalysisNavigationContext) => void }) {
  const { t } = useTranslation();
  const detail = useAssetDriverDetail(request, selected?.bucket ?? "", Boolean(selected));
  const isResidual = selected?.bucket === "residual";
  return (
    <Sheet open={Boolean(selected)} onOpenChange={(open) => { if (!open) onClose(); }}>
      <SheetContent side="right">
        <SheetHeader>
          <SheetTitle>{selected?.label ?? t("insights.driverDetails")}</SheetTitle>
          <p className="text-sm text-muted-foreground">{request.from} — {request.to}</p>
        </SheetHeader>
        {detail.isLoading ? <LoadingState label={t("insights.loading")} /> : detail.isError ? <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => detail.refetch()} retryLabel={t("common.retryAction")} /> : detail.data ? <DriverDetailContent data={detail.data} request={request} session={session} selected={selected} isResidual={isResidual} accountNames={accountNames} instrumentNames={instrumentNames} onOpenHistory={onOpenHistory} onOpenReturnAnalysis={onOpenReturnAnalysis} /> : <EmptyState title={t("insights.noDriverDetails")} />}
      </SheetContent>
    </Sheet>
  );
}

function DriverDetailContent({ data, request, session, selected, isResidual, accountNames, instrumentNames, onOpenHistory, onOpenReturnAnalysis }: { data: AssetDriverDetailDTO; request: AnalysisQueryRequest; session: AnalysisSessionState; selected: AssetChangeRowDTO | null; isResidual: boolean; accountNames: Map<string, string>; instrumentNames: Map<string, string>; onOpenHistory?: (filters: HistoryNavigationFilters) => void; onOpenReturnAnalysis?: (analysis: AnalysisNavigationContext) => void }) {
  const { t } = useTranslation();
  const openReturnAnalysis = onOpenReturnAnalysis && isReturnDriver(selected?.bucket);
  return (
    <div className="flex flex-col gap-5 overflow-y-auto">
      <div>
        <p className="text-xs uppercase tracking-wide text-muted-foreground">{t("insights.total")}</p>
        <p className="mt-1 text-2xl font-semibold">{amountText(selected?.amount)}</p>
      </div>
      {data.valuationForced && <Badge variant="warning">{t("insights.valuationForced")}</Badge>}
      {data.status === "partial" && <div className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground"><AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />{data.missingReason ?? t("insights.partial")}</div>}
      {isResidual && data.residualDetails && data.residualDetails.length > 0 ? (
        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-medium">{t("insights.componentDays")}</h3>
          <ul className="flex flex-col gap-2 text-sm">{data.residualDetails.map((item) => <li key={`${item.date}-${item.componentKey}`} className="rounded-md border border-warning/40 bg-warning/5 px-3 py-2"><div className="flex justify-between gap-3"><span className="truncate">{residualLabel(item, accountNames, instrumentNames)}</span><span className="shrink-0">{amountText(item.amount)}</span></div><p className="mt-1 text-xs text-muted-foreground">{item.date}</p><p className="mt-1 truncate text-xs text-muted-foreground">{item.componentKey}</p>{onOpenHistory && <Button type="button" variant="link" size="sm" className="mt-1 h-auto px-0" onClick={() => onOpenHistory({ from: item.date, to: item.date, accountId: item.accountId || undefined, instrumentId: item.instrumentId || undefined })}>{t("insights.viewInHistory")}</Button>}</li>)}</ul>
        </div>
      ) : null}
      <DimensionList title={t("insights.byInstrument")} rows={data.byInstrument} labelFor={(row) => row.instrumentId ? instrumentNames.get(row.instrumentId) ?? row.label : row.label} />
      <DimensionList title={t("insights.byAccount")} rows={data.byAccount} labelFor={(row) => row.accountId ? accountNames.get(row.accountId) ?? row.label : row.label} />
      {!isResidual && (!data.byInstrument?.length && !data.byAccount?.length) && <EmptyState title={t("insights.noDriverDetails")} />}
      {openReturnAnalysis && selected && (
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenReturnAnalysis?.({
            scope: session.scope,
            scopeId: session.scopeId,
            valuation: session.valuation,
            includeCash: session.includeCash,
            from: request.from,
            to: request.to,
            moreFilters: session.moreFilters,
            returnType: contributionReturnType(selected.bucket),
          })}
        >
          {t("insights.openReturnAnalysis")}
        </Button>
      )}
    </div>
  );
}

export function ChangeDriversTab({ request, session, scope, onOpenHistory, onOpenReturnAnalysis }: { request: AnalysisQueryRequest; session: AnalysisSessionState; scope: "portfolio" | "account" | "instrument"; onOpenHistory?: (filters: HistoryNavigationFilters) => void; onOpenReturnAnalysis?: (analysis: AnalysisNavigationContext) => void }) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<AssetChangeRowDTO | null>(null);
  const data = useAssetChange(request);
  const accounts = useAccounts();
  const instruments = useInstruments();
  if (data.isLoading) return <LoadingState label={t("insights.loading")} />;
  if (data.isError) return <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => data.refetch()} retryLabel={t("common.retryAction")} />;
  if (!data.data?.available) return <EmptyState title={t("insights.noAssetChangeData")} description={data.data?.missingReason ?? t("insights.noAssetChangeDataHint")} />;
  const result = data.data;
  const rows = (result.waterfall ?? []).map((row) => ({ ...row, label: driverLabel(t, row.bucket, row.label) }));
  const groups = (result.groups ?? []).map((group) => ({ ...group, label: groupLabel(t, group.key, group.label), rows: (group.rows ?? []).map((row) => ({ ...row, label: driverLabel(t, row.bucket, row.label) })) }));
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));
  return (
    <div className="flex flex-col gap-4" data-testid="change-drivers">
      <SummaryCard data={result} scope={scope} />
      <WaterfallChart summary={result.summary} rows={rows} onSelect={(key) => { const row = rows.find((candidate) => candidate.key === key); if (row) setSelected(row); }} labels={{ beginning: t("insights.beginningValue"), ending: t("insights.endingValue"), amount: t("insights.amount"), empty: t("insights.noAssetChangeData") }} />
      <section className="flex flex-col gap-3" aria-label={t("insights.attribution")}>
        <h2 className="text-lg font-semibold">{t("insights.attribution")}</h2>
        <div className="grid gap-3 lg:grid-cols-2">{groups.map((group) => <AttributionGroup key={group.key} group={group} onSelect={setSelected} />)}</div>
      </section>
      <DriverDetailSheet request={request} session={session} selected={selected} accountNames={accountNames} instrumentNames={instrumentNames} onOpenHistory={onOpenHistory} onOpenReturnAnalysis={onOpenReturnAnalysis} onClose={() => setSelected(null)} />
    </div>
  );
}
