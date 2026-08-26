import { useTranslation } from "react-i18next";
import { useOverview } from "@/queries/portfolio";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";
import { useHistoryOrigin, useListActivities } from "@/queries/history";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatAmount, formatPercent } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { activitySentence } from "@/features/history/activitySentence";
import type { BreakdownDTO, MissingInputDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function BreakdownList({ title, items, currency }: { title: string; items: BreakdownDTO[]; currency: string }) {
  if (items.length === 0) {
    return null;
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {items.map((item) => (
          <div key={item.key} className="flex items-center justify-between text-sm">
            <span className="text-foreground">{item.label}</span>
            <span className="flex items-center gap-2 text-muted-foreground">
              <span>{formatAmount(item.amount, currency)}</span>
              <Badge variant="secondary">{formatPercent(item.shareBps)}</Badge>
            </span>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function countMissing(items: MissingInputDTO[], kind: string): number {
  return items.filter((item) => item.kind === kind).length;
}

function formatUpdatedAt(timestamp: number, language: string): string {
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short" }).format(new Date(timestamp));
}

export function OverviewPage({
  onAddAccount,
  onOpenAccounts,
  onOpenHistory,
  onOpenMarketData,
}: {
  onAddAccount?: () => void;
  onOpenAccounts?: () => void;
  onOpenHistory?: () => void;
  onOpenMarketData?: () => void;
} = {}) {
  const { t, i18n } = useTranslation();
  const overview = useOverview();
  const origin = useHistoryOrigin();
  const activities = useListActivities(5);
  const accounts = useAccounts({});
  const instruments = useInstruments();

  if (overview.isLoading) {
    return <LoadingState label={t("ui.state.loadingPage")} />;
  }
  if (overview.isError || !overview.data) {
    return (
      <ErrorState
        title={t("overview.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => overview.refetch()}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const data = overview.data;
  const currency = data.currency || "USD";
  const missing = data.missingInputs ?? [];
  const updatedLabel =
    overview.dataUpdatedAt > 0 ? t("overview.updatedAt", { time: formatUpdatedAt(overview.dataUpdatedAt, i18n.language) }) : undefined;
  const healthBadge = data.complete ? (
    <Badge variant="success">{t("overview.healthy")}</Badge>
  ) : (
    <Badge variant="warning">{t("overview.needsAttention")}</Badge>
  );
  const headerStatus = (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="secondary">{t("overview.accountCount", { count: data.accountCount })}</Badge>
      {healthBadge}
      {updatedLabel && <p className="text-sm text-muted-foreground">{updatedLabel}</p>}
    </div>
  );

  if (data.accountCount === 0) {
    return (
      <div className="flex flex-col gap-6" data-testid="overview-page">
        <PageHeader title={t("nav.overview")} description={t("overview.description")} status={headerStatus} />
        <EmptyState
          title={t("overview.emptyTitle")}
          description={t("overview.emptyDescription")}
          action={
            onAddAccount ? (
              <Button type="button" onClick={onAddAccount}>
                {t("overview.addAccount")}
              </Button>
            ) : undefined
          }
        />
      </div>
    );
  }

  const missingPrices = countMissing(missing, "instrument_price");
  const missingValues = countMissing(missing, "account_value");
  const missingFx = countMissing(missing, "fx_rate");
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));
  const recent = activities.data ?? [];
  const historyReady = !origin.isLoading && !origin.isError && Boolean(origin.data);
  const historyNotStarted = !origin.isLoading && !origin.isError && !origin.data;

  return (
    <div className="flex flex-col gap-6" data-testid="overview-page">
      <PageHeader title={t("nav.overview")} description={t("overview.description")} status={headerStatus} />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1.35fr)_minmax(20rem,0.9fr)]">
        <Card>
          <CardHeader>
            <CardTitle>{t("overview.netWorth")}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-4xl font-semibold tracking-tight" data-testid="overview-net-worth">
              {formatAmount(data.netWorth, currency)}
            </p>
            <div className="mt-5 grid grid-cols-2 gap-4">
              <div>
                <p className="text-sm text-muted-foreground">{t("overview.assets")}</p>
                <p className="mt-1 text-xl font-semibold text-success" data-testid="overview-assets">
                  {formatAmount(data.assets, currency)}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">{t("overview.liabilities")}</p>
                <p className="mt-1 text-xl font-semibold text-destructive" data-testid="overview-liabilities">
                  {formatAmount(data.liabilities, currency)}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>{t("overview.dataHealthTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3 text-sm">
              {data.complete ? (
                <p className="text-muted-foreground">{t("overview.dataHealthComplete")}</p>
              ) : (
                <div role="alert" className="flex flex-col gap-3">
                  <p className="text-warning-foreground">{t("overview.dataHealthIncomplete", { count: missing.length })}</p>
                  {missing.length > 0 && (
                    <ul className="list-inside list-disc text-muted-foreground">
                      {missing.map((item, index) => (
                        <li key={`${item.kind}-${item.accountId}-${index}`}>
                          {t("overview.missingInput", {
                            label: `${item.instrumentName || item.instrumentSymbol || t("overview.unknownAccount")} (${displayEnum(t, "overview.missingKind", item.kind)})`,
                          })}
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("overview.nextStepsTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {missingPrices === 0 &&
              missingValues === 0 &&
              missingFx === 0 &&
              !historyNotStarted &&
              !(historyReady && !activities.isLoading && !activities.isError && recent.length === 0) ? (
                <p className="text-sm text-muted-foreground">{t("overview.noNextSteps")}</p>
              ) : (
                <ul className="flex flex-col gap-3">
                  {missingPrices > 0 && (
                    <li className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                      <p className="text-sm text-foreground">{t("overview.refreshPricesNext", { count: missingPrices })}</p>
                      {onOpenMarketData && (
                        <Button type="button" size="sm" variant="outline" onClick={onOpenMarketData}>
                          {t("overview.openMarketData")}
                        </Button>
                      )}
                    </li>
                  )}
                  {missingFx > 0 && (
                    <li className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                      <p className="text-sm text-foreground">{t("overview.refreshFxNext", { count: missingFx })}</p>
                      {onOpenMarketData && (
                        <Button type="button" size="sm" variant="outline" onClick={onOpenMarketData}>
                          {t("overview.openMarketData")}
                        </Button>
                      )}
                    </li>
                  )}
                  {missingValues > 0 && (
                    <li className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                      <p className="text-sm text-foreground">{t("overview.reviewValuesNext", { count: missingValues })}</p>
                      {onOpenAccounts && (
                        <Button type="button" size="sm" variant="outline" onClick={onOpenAccounts}>
                          {t("overview.openAccounts")}
                        </Button>
                      )}
                    </li>
                  )}
                  {historyNotStarted && (
                    <li className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                      <p className="text-sm text-foreground">{t("overview.startHistoryNext")}</p>
                      {onOpenHistory && (
                        <Button type="button" size="sm" variant="outline" onClick={onOpenHistory}>
                          {t("overview.openHistory")}
                        </Button>
                      )}
                    </li>
                  )}
                  {historyReady && !activities.isLoading && !activities.isError && recent.length === 0 && (
                    <li className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                      <p className="text-sm text-foreground">{t("overview.recordChangeNext")}</p>
                      {onOpenHistory && (
                        <Button type="button" size="sm" variant="outline" onClick={onOpenHistory}>
                          {t("overview.openHistory")}
                        </Button>
                      )}
                    </li>
                  )}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <CardTitle>{t("overview.recentActivity")}</CardTitle>
          {onOpenHistory && (
            <Button type="button" size="sm" variant="ghost" onClick={onOpenHistory}>
              {t("overview.viewAllActivity")}
            </Button>
          )}
        </CardHeader>
        <CardContent>
          {activities.isError ? (
            <ErrorState
              title={t("history.loadError")}
              description={t("ui.state.errorDescription")}
              onRetry={() => activities.refetch()}
              retryLabel={t("common.retryAction")}
            />
          ) : activities.isLoading ? (
            <LoadingState label={t("history.loading")} />
          ) : recent.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("overview.noRecentActivity")}</p>
          ) : (
            <ul className="flex flex-col gap-3" data-testid="overview-recent-activity">
              {recent.map((activity) => (
                <li key={activity.id} className="flex flex-col gap-1 border-b border-border pb-3 text-sm last:border-0 last:pb-0">
                  <p className="text-foreground">{activitySentence(t, activity, accountNames, instrumentNames)}</p>
                  <time className="text-xs text-muted-foreground" dateTime={activity.effectiveAt}>
                    {activity.effectiveLocalDate}
                  </time>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <BreakdownList title={t("overview.byCategory")} items={(data.byCategory ?? []).map((item) => ({ ...item, label: displayEnum(t, "enum", item.key) }))} currency={currency} />
        <BreakdownList title={t("overview.byMember")} items={data.byMember ?? []} currency={currency} />
        <BreakdownList title={t("overview.byInstitution")} items={data.byInstitution ?? []} currency={currency} />
        <BreakdownList title={t("overview.byGroup")} items={data.byGroup ?? []} currency={currency} />
      </div>
    </div>
  );
}
