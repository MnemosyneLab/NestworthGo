import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { CompositionChart } from "@/components/charts/CompositionChart";
import { TrendChart } from "@/components/charts/TrendChart";
import { RangeToggle } from "@/components/charts/RangeToggle";
import { chartTheme } from "@/components/charts/chartTheme";
import { usePortfolio, usePortfolioTrend } from "@/queries/portfolio";
import { useCatalog } from "@/queries/catalog";
import { formatAmount, sortByCanonicalDesc } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import { EntityIcon } from "@/components/icons/EntityIcon";

/**
 * PortfolioPage is the independent household portfolio view. Totals,
 * allocation, and included accounts come from PortfolioService.Portfolio;
 * this page does not convert FX or treat missing quotes as zero.
 */
export function PortfolioPage({ onOpenAccount }: { onOpenAccount?: (accountId: string) => void } = {}) {
  const { t } = useTranslation();
  const catalog = useCatalog();
  const portfolio = usePortfolio();
  const ranges = catalog.data?.trendRanges ?? ["30d", "1y", "all"];
  const [range, setRange] = useState("30d");
  const trend = usePortfolioTrend(range);
  const theme = chartTheme();

  if (portfolio.isLoading) {
    return <LoadingState label={t("portfolio.loading")} />;
  }
  if (portfolio.isError || !portfolio.data) {
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
  const total = data.valuedSubtotal ? formatAmount(data.valuedSubtotal.amount, data.valuedSubtotal.currency) : t("accounts.noValue");
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
      <PageHeader title={t("portfolio.pageTitle")} description={t("portfolio.pageDescription")} />
      <p className="text-sm text-muted-foreground">{t("portfolio.holdingsOnlyNote")}</p>
      {accounts.length === 0 ? (
        <EmptyState title={t("portfolio.emptyTitle")} description={t("portfolio.emptyDescription")} />
      ) : (
        <>
          <Card>
            <CardHeader className="flex flex-row items-start justify-between gap-3">
              <CardTitle>{t("charts.valuedSubtotal")}</CardTitle>
              <Badge variant={data.complete ? "success" : "warning"}>
                {data.complete ? t("overview.healthy") : t("overview.needsAttention")}
              </Badge>
            </CardHeader>
            <CardContent>
              <p className="text-3xl font-semibold tracking-tight" data-testid="portfolio-total">
                {total}
              </p>
              {!data.complete && (
                <p className="mt-2 text-sm text-warning-foreground">
                  {t("overview.totalsExcludeMissing")} {t("portfolio.missingQuotes", { count: missingCount })}
                </p>
              )}
            </CardContent>
          </Card>

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
              <RangeToggle ranges={ranges} value={range} onChange={setRange} label={t("analytics.range")} />
              {trend.isError ? (
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
                    values: trendPoints.map((point) => point.valuedSubtotal?.amount),
                  }]}
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
                      <EntityIcon iconKey={valuation.account.iconKey} kind="account" className="size-5 text-primary" />
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
