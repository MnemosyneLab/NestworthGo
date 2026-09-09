import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { NativeSelect } from "@/components/ui/select";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useCategories, useCategoryDetail } from "@/queries/returnAnalysis";
import type { CategoriesDTO, CategoryDetailDTO, CategoryRowDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { AnalysisSessionState } from "@/stores/analysis";
import type { HistoryNavigationFilters } from "@/app/navigation";
import { useAnalysisProjectionContext } from "@/features/insights/analysisProjectionContext";
import { AvailabilityMarks } from "@/features/insights/CompletenessBanner";
import { analysisReason } from "@/features/insights/analysisText";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";
import { formatAmount } from "@/lib/money";

function amountText(value?: SignedMoneyView | null): string {
  if (!value) return "—";
  const formatted = formatAmount(value.amount, value.currency);
  return value.amount.startsWith("-") || value.amount === "0" ? formatted : `+${formatted}`;
}

function typeLabel(t: (key: string) => string, value: string): string {
  const key = value === "income" ? "income" : value === "spending" ? "spending" : value === "fees" ? "fees" : value === "investment_return" ? "investmentReturn" : "dividendInterestView";
  return t(`insights.${key}`);
}

function categoriesEmpty(data: CategoriesDTO | undefined, t: (key: string) => string) {
  const hasHistory = Boolean(data && (data.rows?.length ?? 0) > 0);
  return <EmptyState title={t(hasHistory ? "insights.historyInsufficient" : "insights.noInvestmentAssets")} description={t(hasHistory ? "insights.historyInsufficientHint" : "insights.noInvestmentAssetsHint")} />;
}

function DetailContent({ data, total, accountNames, instrumentNames, onOpenHistory }: { data: CategoryDetailDTO; total?: SignedMoneyView | null; accountNames: Map<string, string>; instrumentNames: Map<string, string>; onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const childLabel = (child: NonNullable<CategoryDetailDTO["children"]>[number]) => {
    if (child.accountId) return accountNames.get(child.accountId) ?? child.label;
    if (child.instrumentId) return instrumentNames.get(child.instrumentId) ?? child.label;
    return child.key === "cash" ? t("insights.cash") : child.label;
  };
  return <div className="flex flex-col gap-5 overflow-y-auto"><div><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("insights.total")}</p><p className="mt-1 text-2xl font-semibold">{amountText(total)}</p></div>{data.valuationForced && <Badge variant="warning">{t("insights.valuationForced")}</Badge>}{data.status === "partial" && <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{analysisReason(t, data.missingReason) ?? t("insights.partial")}</p>}{data.children && data.children.length > 0 && <div className="flex flex-col gap-2"><h3 className="text-sm font-medium">{t("insights.categoryChildren")}</h3><ul className="flex flex-col gap-1 text-sm">{data.children.map((child) => <li key={child.key} className="flex justify-between gap-3"><span className="truncate">{childLabel(child)}</span><span className="shrink-0">{amountText(child.amount)}</span></li>)}</ul></div>}<div className="flex flex-col gap-2"><h3 className="text-sm font-medium">{t("insights.records")}</h3>{data.activityRefs && data.activityRefs.length > 0 ? <ul className="flex flex-col gap-2 text-sm">{data.activityRefs.map((ref) => <li key={`${ref.date}-${ref.activityId ?? ref.accountId ?? ref.instrumentId ?? ref.amount?.amount ?? "record"}`} className="rounded-md border border-border px-3 py-2"><div className="flex justify-between gap-3"><span>{ref.date}</span><span>{amountText(ref.amount)}</span></div>{onOpenHistory && <Button type="button" variant="link" size="sm" className="mt-1 h-auto px-0" onClick={() => onOpenHistory({ from: ref.date, to: ref.date, accountId: ref.accountId || undefined, instrumentId: ref.instrumentId || undefined })}>{t("insights.viewInHistory")}</Button>}</li>)}</ul> : <p className="text-sm text-muted-foreground">{t("insights.noRecords")}</p>}</div></div>;
}

export function CategoriesTab({ session, onOpenHistory }: { session: AnalysisSessionState; onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const [categoryType, setCategoryType] = useState("spending");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const context = useAnalysisProjectionContext(session);
  const categories = useCategories(context.request, categoryType, context.enabled);
  const detail = useCategoryDetail(context.request, categoryType, selectedKey ?? "", Boolean(selectedKey) && context.enabled);
  const accounts = useAccounts();
  const instruments = useInstruments();
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));
  const labelFor = (row: CategoryRowDTO) => row.accountId ? accountNames.get(row.accountId) ?? row.label : row.instrumentId ? instrumentNames.get(row.instrumentId) ?? row.label : row.key === "cash" ? t("insights.cash") : row.label;
  const data = categories.data;
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
        <label htmlFor="categories-type" className="text-sm font-medium">{t("insights.categoryType")}</label>
        <NativeSelect id="categories-type" value={categoryType} onChange={(event) => { setCategoryType(event.target.value); setSelectedKey(null); }}>
          <option value="income">{t("insights.income")}</option>
          <option value="spending">{t("insights.spending")}</option>
          <option value="fees">{t("insights.fees")}</option>
          <option value="investment_return">{t("insights.investmentReturn")}</option>
          <option value="dividend_interest">{t("insights.dividendInterestView")}</option>
        </NativeSelect>
      </div>
      <p className="max-w-md text-sm text-muted-foreground">{t("insights.categoriesHint")}</p>
    </div>
  );

  let results;
  if (categories.isLoading) results = <LoadingState label={t("insights.loading")} />;
  else if (categories.isError) results = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => categories.refetch()} retryLabel={t("common.retryAction")} />;
  else if (!data?.available) results = categoriesEmpty(data, t);
  else results = (
    <>
      <Card>
        <CardHeader className="flex-row items-center justify-between gap-3">
          <CardTitle>{typeLabel(t, categoryType)}</CardTitle>
          <AvailabilityMarks status={data.status} missingReason={data.missingReason} valuationForced={data.valuationForced} />
        </CardHeader>
        <CardContent>
          <div className="mb-4">
            <p className="text-xs uppercase tracking-wide text-muted-foreground">{t("insights.total")}</p>
            <p className="mt-1 text-2xl font-semibold">{amountText(data.total)}</p>
          </div>
          <ul className="divide-y divide-border" aria-label={t("insights.categories")}>
            {(data.rows ?? []).map((row) => (
              <li key={row.key}>
                <Button type="button" variant="ghost" className="h-auto w-full justify-between gap-4 rounded-md px-3 py-2 text-left" onClick={() => setSelectedKey(row.key)}>
                  <span className="min-w-0 flex-1 truncate">{labelFor(row)}</span>
                  <span className="shrink-0">{amountText(row.amount)}</span>
                </Button>
              </li>
            ))}
          </ul>
          {(data.rows ?? []).length === 0 && <EmptyState title={t("insights.noCategoryData")} description={t("insights.noCategoryDataHint")} />}
        </CardContent>
      </Card>
      <Sheet open={Boolean(selected)} onOpenChange={(open) => { if (!open) setSelectedKey(null); }}>
        <SheetContent side="right">
          <SheetHeader><SheetTitle>{selected ? labelFor(selected) : t("insights.categoryDetail")}</SheetTitle></SheetHeader>
          {detail.isLoading ? <LoadingState label={t("insights.loading")} /> : detail.isError ? <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => detail.refetch()} retryLabel={t("common.retryAction")} /> : detail.data ? <DetailContent data={detail.data} total={selected?.amount} accountNames={accountNames} instrumentNames={instrumentNames} onOpenHistory={onOpenHistory} /> : <EmptyState title={t("insights.noRecords")} />}
        </SheetContent>
      </Sheet>
    </>
  );

  return <div className="flex flex-col gap-4" data-testid="categories">{toolbar}{results}</div>;
}
