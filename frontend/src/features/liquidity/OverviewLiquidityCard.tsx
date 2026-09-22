import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useLiquidityOverview } from "@/queries/liquidity";
import { completenessLabel, formatKnownOrUnknown } from "@/features/liquidity/liquidityDisplay";

export function OverviewLiquidityCard() {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const overview = useLiquidityOverview({ includeEarlyWithdrawal: false });
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
  const status = today?.status ?? "unavailable";
  return (
    <Card className="flex flex-col" data-testid="overview-liquidity-card">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>{t("availableFunds.pageTitle")}</CardTitle>
        <Badge variant={status === "complete" ? "success" : status === "partial" ? "warning" : "destructive"}>
          {completenessLabel(t, status)}
        </Badge>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-5">
        <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,10rem),1fr))] gap-4">
          <div>
            <p className="text-sm text-muted-foreground">{t("availableFunds.overviewToday")}</p>
            <p className="break-words text-xl font-semibold" data-testid="overview-liquidity-today">
              {formatKnownOrUnknown(today?.fullUnreserved ?? today?.knownUnreservedSubtotal, unknown)}
            </p>
            {!today?.fullUnreserved && <p className="mt-1 text-xs text-muted-foreground">{t("availableFunds.incompleteAmount")}</p>}
          </div>
          <div>
            <p className="text-sm text-muted-foreground">{t("availableFunds.overview30")}</p>
            <p className="break-words text-xl font-semibold" data-testid="overview-liquidity-30">
              {formatKnownOrUnknown(month?.fullUnreserved ?? month?.knownUnreservedSubtotal, unknown)}
            </p>
            {!month?.fullUnreserved && <p className="mt-1 text-xs text-muted-foreground">{t("availableFunds.incompleteAmount")}</p>}
          </div>
        </div>
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
