import { ValuationQualitySummary } from "@/components/valuation/ValuationQualitySummary";
import { fullDisplayAmount, valuationDetails, valuationTableAmount, valuationLabel, hasDisplay } from "@/components/valuation/valuationQuality";
import { useState } from "react";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { HoldingsTab } from "@/features/investments/HoldingsTab";
import { InstrumentManagement } from "@/features/investments/InstrumentManagement";
import type { PortfolioView } from "@/app/navigation";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { CompositionChart } from "@/components/charts/CompositionChart";
import { TrendChart } from "@/components/charts/TrendChart";
import { chartTheme, useThemeVersion } from "@/components/charts/chartTheme";
import { usePortfolio, usePortfolioTrend } from "@/queries/portfolio";
import { DateRangeControl } from "@/components/charts/DateRangeControl";
import { useTrendDateRange } from "@/components/charts/useTrendDateRange";
import { formatAmount, sortByCanonicalDesc } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import { AccountIcon } from "@/components/icons/AccountIcon";

/**
 * PortfolioOverview renders the household portfolio summary. Totals,
 * allocation, and included accounts come from PortfolioService.Portfolio;
 * this page does not convert FX or treat missing quotes as zero.
 */
function PortfolioOverview({ onOpenAccount, active = true }: { onOpenAccount?: (accountId: string) => void; active?: boolean } = {}) {
  const { t } = useTranslation();
  const portfolio = usePortfolio();
  const range = useTrendDateRange();
  const trend = usePortfolioTrend(range.queryRange, active);
  useThemeVersion();
  const theme = chartTheme();

  if (portfolio.isLoading) {
    return <LoadingState label={t("portfolio.loading")} />;
  }
  if (!portfolio.data) {
    return (
      <ErrorState
        title={t("portfolio.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => portfolio.refetch()}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const data = portfolio.data;
  const currency = data.currency || "USD";
  const accounts = sortByCanonicalDesc(
    data.accounts ?? [],
    (valuation) => valuation.baseValue?.amount,
    (left, right) => left.account.name.localeCompare(right.account.name),
  );
  const allocation = data.byInstrumentType ?? [];
  const missingCount = (data.missingInputs ?? []).length;
  const trendPoints = trend.data?.points ?? [];

  return (
    <div className="flex flex-col gap-6" data-testid="portfolio-page">
 {portfolio.isFetching && <p role="status">{t("valuation.updating")}</p>}
 {portfolio.isError && <p role="alert">{t("valuation.priorResult")} <Button variant="link" onClick={() => void portfolio.refetch()}>{t("common.retryAction")}</Button></p>}
      {accounts.length === 0 ? (
        <EmptyState title={t("portfolio.emptyTitle")} description={t("portfolio.emptyDescription")} />
      ) : (
        <>
          {allocation.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>{t("portfolio.allocation")}</CardTitle>
              </CardHeader>
              <CardContent>
                <CompositionChart
                  title={t("portfolio.allocation")}
                  items={allocation.map((item) => ({
                    key: item.key,
                    label: displayEnum(t, "enum", item.key),
                    amount: item.amount.amount,
                    shareBps: item.shareBps,
                  }))}
                  currency={currency}
                  centerValue={data.valuedSubtotal?.amount ?? "0"}
                  centerCaption={t("charts.valuedSubtotal")}
                  ariaLabel={t("portfolio.allocation")}
                  summary={t("portfolio.allocation")}
                  height={320}
                  complete={data.complete}
                  missingCount={missingCount}
                />
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader>
              <CardTitle>{t("portfolio.trend")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <DateRangeControl {...range} />
 {trend.isFetching && trend.data && <p role="status">{t("valuation.updating")}</p>}
 {trend.isError && trend.data && <p role="alert">{t("valuation.priorResult")} <Button variant="link" onClick={() => void trend.refetch()}>{t("common.retryAction")}</Button></p>}
              {trend.isError && !trend.data ? (
                <ErrorState
                  title={t("portfolio.loadError")}
                  description={t("ui.state.errorDescription")}
                  onRetry={() => void trend.refetch()}
                  retryLabel={t("common.retryAction")}
                />
              ) : trend.isLoading ? (
                <LoadingState label={t("portfolio.loading")} />
              ) : (
                <TrendChart
                  ariaLabel={t("portfolio.trend")}
                  summary={t("portfolio.trendSummary")}
                  dates={trendPoints.map((point) => point.localDate)}
                  series={[{
                    key: "valuedSubtotal",
                    name: t("charts.valuedSubtotal"),
                    color: theme.primary,
                    values: trendPoints.map((point) => fullDisplayAmount(point.displayValuation,point.complete ? point.valuedSubtotal?.amount : null)),
 pointMeta: trendPoints.map(point => ({provisional:point.current || point.displayValuation?.quality?.coverage === "provisional",qualityLabel:valuationDetails(t,point.displayValuation,point.current)})),
                  }]}
                  extraTableColumns={[t("charts.date"),t("charts.valuedSubtotal"),t("valuation.knownSubtotal"),t("connections.coverage")]}
 extraTableRows={trendPoints.map(point=>[point.localDate,valuationTableAmount(t,point.displayValuation,point.valuedSubtotal),point.displayValuation?.knownSubtotal ? formatAmount(point.displayValuation.knownSubtotal.amount,point.displayValuation.knownSubtotal.currency) : t("accounts.noValue"),valuationDetails(t,point.displayValuation,point.current)])}
 currency={trend.data?.currency || currency}
                  height={320}
                  emptyTitle={t("charts.insufficientHistory")}
                />
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("portfolio.accounts")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              {accounts.map((valuation) => {
                const amount = valuation.baseValue
                  ? formatAmount(valuation.baseValue.amount, valuation.baseValue.currency)
                  : t("accounts.noValue");
                return (
                  <div key={valuation.account.id} className="flex items-center justify-between gap-3 text-sm">
                    <div className="flex min-w-0 items-center gap-2">
                      <AccountIcon account={valuation.account} className="size-8 text-primary" />
                      <div className="flex min-w-0 flex-col gap-1">
                      {onOpenAccount ? (
                        <Button
                          type="button"
                          variant="link"
                          className="h-auto justify-start p-0 text-sm"
                          onClick={() => onOpenAccount(valuation.account.id)}
                        >
                          {valuation.account.name}
                        </Button>
                      ) : (
                        <span className="font-medium">{valuation.account.name}</span>
                      )}
                      <span className="text-xs text-muted-foreground">
                        {displayEnum(t, "enum", valuation.account.accountType)}
                        {valuation.institutionName ? ` · ${valuation.institutionName}` : ""}
                      </span>
                      </div>
                    </div>
                    <span className="flex items-center gap-2">
                      {!valuation.complete && <Badge variant="warning">{t("accounts.partialValuation")}</Badge>}
                      <span>{amount}</span>
                    </span>
                  </div>
                );
              })}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}

/** Browse owned holdings first; saved instruments and allocation are secondary tasks. */
export function PortfolioPage({ onOpenAccount, view, onViewChange, active = true, holdingDraftRequest }: {
  onOpenAccount?: (accountId: string) => void;
  view?: PortfolioView;
  onViewChange?: (view: PortfolioView) => void;
  active?: boolean;
  holdingDraftRequest?: { requestId: number; accountId: string };
} = {}) {
  const { t } = useTranslation();
  const portfolio = usePortfolio();
  const [localView, setLocalView] = useState<PortfolioView>("holdings");
  const [visited, setVisited] = useState<PortfolioView[]>(["holdings"]);
  const tab = view ?? localView;
  const select = (next: string) => {
    const selected = next as PortfolioView;
    if (view === undefined) setLocalView(selected);
    onViewChange?.(selected);
    setVisited((previous) => previous.includes(selected) ? previous : [...previous, selected]);
  };
  const summary = portfolio.data;
  return (
    <div className="flex flex-col gap-4">
      <PageChrome pageId="portfolio" enabled={active} title={t("portfolio.pageTitle")} />
      {summary && <section aria-label={t("investmentAssets.summary")} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-2xl border border-border bg-card px-4 py-3">
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">{t("investmentAssets.summary")}</p>
          <p className="num text-xl font-semibold" data-testid="portfolio-total">{valuationTableAmount(t, summary.displayValuation, summary.valuedSubtotal)}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={hasDisplay(summary.displayValuation) ? "secondary" : summary.complete ? "success" : "warning"}>
            {hasDisplay(summary.displayValuation) ? valuationLabel(t, summary.displayValuation, true) : summary.complete ? t("overview.healthy") : t("overview.needsAttention")}
          </Badge>
          <ValuationQualitySummary value={summary.displayValuation} current />
        </div>
        {!summary.complete && <p className="basis-full text-xs text-warning-foreground">{t("overview.totalsExcludeMissing")} {t("portfolio.missingQuotes", { count: (summary.missingInputs ?? []).length })}</p>}
      </section>}
      <Tabs value={tab} onValueChange={select}>
        <TabsList aria-label={t("portfolio.pageTitle")}>
          <TabsTrigger value="holdings">{t("investmentAssets.holdings")}</TabsTrigger>
          <TabsTrigger value="instruments">{t("investmentAssets.instruments")}</TabsTrigger>
          <TabsTrigger value="allocation">{t("investmentAssets.allocation")}</TabsTrigger>
        </TabsList>
        <TabsContent value="holdings" keepMounted>
          <HoldingsTab onOpenAccount={onOpenAccount} active={active && tab === "holdings"} draftRequest={holdingDraftRequest} />
        </TabsContent>
        <TabsContent value="instruments" keepMounted>
          {(tab === "instruments" || visited.includes("instruments")) && <div className="flex flex-col gap-3">
            <InstrumentManagement pageId="portfolio" active={active && tab === "instruments"} enableSearch browseAll groupTestIdPrefix="investment-assets-group" />
          </div>}
        </TabsContent>
        <TabsContent value="allocation" keepMounted>
          {(tab === "allocation" || visited.includes("allocation")) && <PortfolioOverview onOpenAccount={onOpenAccount} active={active && tab === "allocation"} />}
        </TabsContent>
      </Tabs>
    </div>
  );
}
