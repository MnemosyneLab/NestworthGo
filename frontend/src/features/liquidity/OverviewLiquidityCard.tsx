import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useLiquidityOverview } from "@/queries/liquidity";
import { completenessLabel, formatKnownOrUnknown, sourceMatchesFilter } from "@/features/liquidity/liquidityDisplay";
import type { LiquidityBucketDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

export function OverviewLiquidityCard({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const overview = useLiquidityOverview({ includeEarlyWithdrawal: false }, isActive);
  const unknown = t("availableFunds.unknownAmount");

  if (overview.isLoading) {
    return (
      <Card data-testid="overview-liquidity-card">
        <CardHeader>
          <CardTitle>{t("availableFunds.pageTitle")}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{t("ui.state.loadingPage")}</p>
        </CardContent>
      </Card>
    );
  }
  if (overview.isError || !overview.data) {
    return (
      <Card data-testid="overview-liquidity-card">
        <CardHeader>
          <CardTitle>{t("availableFunds.pageTitle")}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          <p role="alert" className="text-sm text-destructive">{t("availableFunds.loadError")}</p>
          <Button type="button" size="sm" variant="outline" onClick={() => void overview.refetch()}>{t("common.retryAction")}</Button>
        </CardContent>
      </Card>
    );
  }

  const today = overview.data.buckets?.[0];
  const month = overview.data.buckets?.[2];
  const horizon = (bucket: LiquidityBucketDTO | undefined, title: string, testId: string) => {
    const status = bucket?.status ?? "unavailable";
    const amount = bucket?.fullUnreserved ?? bucket?.knownUnreservedSubtotal;
    const missingRules = bucket ? (overview.data?.sources ?? []).filter(source => sourceMatchesFilter(source, bucket.horizonOn, "needs_rules")).length : 0;
    return <div className="min-w-0 space-y-1" role="group" aria-label={title}>
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm text-muted-foreground">{title}</p>
        <Badge variant={status === "complete" ? "success" : status === "partial" ? "warning" : "secondary"}>{completenessLabel(t, status)}</Badge>
      </div>
      <p className="num break-words text-2xl font-bold tracking-tight" data-testid={testId}>{formatKnownOrUnknown(amount, unknown)}</p>
      <p className="text-xs text-muted-foreground">{t(bucket?.fullUnreserved ? "availableFunds.spendableHint" : amount ? "availableFunds.confirmedSubtotal" : "availableFunds.totalUnknown")}</p>
      {bucket && (missingRules > 0 ? (
        navigation ? <Button type="button" size="sm" variant="link" className="h-auto p-0" onClick={() => navigation.open({ page: "available-funds", focus: { kind: "missing-rules", horizonOn: bucket.horizonOn } })}>{t("availableFunds.missingRulesCount", { count: missingRules })}</Button>
          : <p className="text-xs text-muted-foreground">{t("availableFunds.missingRulesCount", { count: missingRules })}</p>
      ) : <p className="text-xs text-muted-foreground">{t("availableFunds.noMissingRules")}</p>)}
    </div>;
  };
  return (
    <Card className="flex flex-col" data-testid="overview-liquidity-card">
      <CardHeader>
        <CardTitle>{t("availableFunds.pageTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-5">
        <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,10rem),1fr))] gap-4">
          {horizon(today, t("availableFunds.overviewToday"), "overview-liquidity-today")}
          {horizon(month, t("availableFunds.overview30"), "overview-liquidity-30")}
        </div>
        <p className="text-xs text-muted-foreground">{t("availableFunds.current_prices_and_fx")}</p>
        <div className="mt-auto flex justify-end border-t border-border pt-3">
          {navigation && (
            <Button type="button" size="sm" variant="link" className="h-auto p-0" onClick={() => navigation.open({ page: "available-funds" })}>
              {t("availableFunds.openPage")}
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
