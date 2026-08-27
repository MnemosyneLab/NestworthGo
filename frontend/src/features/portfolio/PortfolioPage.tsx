import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { usePortfolio } from "@/queries/portfolio";
import { formatAmount, formatPercent } from "@/lib/money";
import { displayEnum } from "@/lib/display";

/**
 * PortfolioPage is the independent household portfolio view. Totals,
 * allocation, and included accounts come from PortfolioService.Portfolio;
 * this page does not convert FX or treat missing quotes as zero.
 */
export function PortfolioPage({ onOpenAccount }: { onOpenAccount?: (accountId: string) => void } = {}) {
  const { t } = useTranslation();
  const portfolio = usePortfolio();

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
  const accounts = data.accounts ?? [];
  const allocation = data.byInstrumentType ?? [];

  return (
    <div className="flex flex-col gap-6" data-testid="portfolio-page">
      <PageHeader title={t("portfolio.pageTitle")} description={t("portfolio.pageDescription")} />
      <p className="text-sm text-muted-foreground">{t("portfolio.wholeAccountNote")}</p>
      {accounts.length === 0 ? (
        <EmptyState title={t("portfolio.emptyTitle")} description={t("portfolio.emptyDescription")} />
      ) : (
        <>
          <Card>
            <CardHeader className="flex flex-row items-start justify-between gap-3">
              <CardTitle>{t("portfolio.pageTitle")}</CardTitle>
              <Badge variant={data.complete ? "success" : "warning"}>
                {data.complete ? t("overview.healthy") : t("overview.needsAttention")}
              </Badge>
            </CardHeader>
            <CardContent>
              <p className="text-3xl font-semibold tracking-tight" data-testid="portfolio-total">
                {total}
              </p>
              {!data.complete && (
                <p className="mt-2 text-sm text-warning-foreground">{t("overview.totalsExcludeMissing")}</p>
              )}
            </CardContent>
          </Card>

          {allocation.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>{t("portfolio.allocation")}</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-2">
                {allocation.map((item) => (
                  <div key={item.key} className="flex items-center justify-between text-sm">
                    <span>{displayEnum(t, "enum", item.key)}</span>
                    <span className="flex items-center gap-2 text-muted-foreground">
                      <span>{formatAmount(item.amount.amount, item.amount.currency || currency)}</span>
                      <Badge variant="secondary">{formatPercent(item.shareBps)}</Badge>
                    </span>
                  </div>
                ))}
              </CardContent>
            </Card>
          )}

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
