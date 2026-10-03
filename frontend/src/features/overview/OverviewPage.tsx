import type * as React from "react";
import { useObjectNavigation } from "@/app/NavigationContext";
import type { HealthFocus } from "@/app/navigation";
import { NetWorthTrendCard } from "./NetWorthTrendCard";
import { useMarketDataHealth } from "@/queries/marketdata";
import { useTranslation } from "react-i18next";
import { useOverview } from "@/queries/portfolio";
import { useSettings } from "@/queries/settings";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { MoneyText } from "@/components/ui/money-text";
import { IconTile } from "@/components/ui/icon-tile";
import { ShareBar } from "@/components/ui/share-bar";
import { TONE_CLASSES, toneForKey, type Tone } from "@/lib/tone";
import { activityStyle } from "@/lib/activityStyle";
import { cn } from "@/lib/utils";
import { CheckCircle2, TrendingUp, TrendingDown, ArrowLeftRight, Wallet, History, PencilLine, HeartPulse, CircleDollarSign, type LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { formatAmount, formatPercent, isPositiveCanonical } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { formatTimestamp } from "@/lib/time";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { activitySentence } from "@/features/history/activitySentence";
import { CompositionChart } from "@/components/charts/CompositionChart";
import { DataHealthIndicator } from "@/features/data-health/DataHealthIndicator";
import { OverviewLiquidityCard } from "@/features/liquidity/OverviewLiquidityCard";
import { healthIssueLabel } from "@/features/data-health/healthIssueLabel";
import type { ActivityDTO, BreakdownDTO, MissingInputDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type OverviewMissingInput = MissingInputDTO & { accountName?: string; quoteSource?: string };
type OverviewLabel = { id: string; name: string };
type OverviewInstrumentLabel = OverviewLabel & { quoteSource?: string; symbol?: string };
type OverviewHoldingLabel = OverviewLabel & { accountId: string; instrumentId: string };

type OverviewReadModel = {
  currency: string;
  accountCount: number;
  complete: boolean;
  missingInputs?: OverviewMissingInput[];
  assets: string;
  liabilities: string;
  netWorth: string;
  assetsByType?: BreakdownDTO[];
  liabilitiesByType?: BreakdownDTO[];
  byMember?: BreakdownDTO[];
  byInstitution?: BreakdownDTO[];
  byGroup?: BreakdownDTO[];
  byAccountType?: BreakdownDTO[];
  historyStarted?: boolean;
  recentActivities?: ActivityDTO[];
  accountLabels?: OverviewLabel[];
  instrumentLabels?: OverviewInstrumentLabel[];
  holdingLabels?: OverviewHoldingLabel[];
};

function BreakdownList({
  title,
  description,
  items,
  currency, onSelect,
}: {
  title: string;
  description?: string;
  items: BreakdownDTO[];
  currency: string;
  onSelect?: (item: BreakdownDTO) => void;
}) {
  if (items.length === 0) {
    return null;
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <p className="text-sm font-normal text-muted-foreground">{description}</p>}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {items.map((item) => {
          const tone = toneForKey(item.key || item.label);
          return (
            <div key={item.key} className="relative flex flex-col gap-1.5 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                <span className="flex min-w-0 flex-1 basis-28 items-center gap-2">
                  <span aria-hidden="true" className={cn("size-2.5 shrink-0 rounded-full", TONE_CLASSES[tone].dot)} />
                  {onSelect ? (
                    <button
                      type="button"
                      className="truncate text-left font-semibold text-foreground transition-colors after:absolute after:-inset-1 after:rounded-lg hover:text-primary focus-visible:outline-none focus-visible:after:ring-2 focus-visible:after:ring-primary/50"
                      onClick={() => onSelect(item)}
                    >
                      {item.label}
                    </button>
                  ) : (
                    <span className="truncate font-semibold text-foreground">{item.label}</span>
                  )}
                </span>
                <span className="ml-auto flex shrink-0 items-center gap-3">
                  <span className="num text-xs text-muted-foreground">{formatPercent(item.shareBps)}</span>
                  <MoneyText>{formatAmount(item.amount, currency)}</MoneyText>
                </span>
              </div>
              <ShareBar shareBps={item.shareBps} tone={tone} />
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}

/** One actionable line in the merged to-do card. */
function TodoRow({ icon, tone, text, actionLabel, onAction }: { icon: LucideIcon; tone: Tone; text: React.ReactNode; actionLabel?: string; onAction?: () => void }) {
  return (
    <li className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <span className="flex min-w-0 items-center gap-3">
        <IconTile icon={icon} tone={tone} size="sm" />
        <span className="text-sm text-foreground">{text}</span>
      </span>
      {actionLabel && onAction && (
        <Button type="button" size="sm" variant="soft" className="shrink-0 self-start sm:self-auto" onClick={onAction}>
          {actionLabel}
        </Button>
      )}
    </li>
  );
}

function countMissing(items: OverviewMissingInput[], kind: string): number {
  return items.filter((item) => item.kind === kind).length;
}

function missingItemLabel(
  t: (key: string) => string,
  item: OverviewMissingInput,
  accountNames: Map<string, string>,
): string {
  if (item.kind === "instrument_price") {
    return instrumentDisplayLabel({ name: item.instrumentName, symbol: item.instrumentSymbol }, t("overview.unknownInstrument"));
  }
  if (item.kind === "fx_rate" && (item.baseCurrency || item.quoteCurrency)) {
    return `${item.baseCurrency}/${item.quoteCurrency}`;
  }
  return item.accountName || accountNames.get(item.accountId) || t("overview.unknownAccount");
}

function formatUpdatedAt(timestamp: number, language: string, timezone?: string): string {
  return formatTimestamp(timestamp, timezone, language);
}

export function OverviewPage({
  onAddAccount,
  onOpenAccounts,
  onOpenHistory,
  onOpenMarketData,
  onOpenDataHealth,
}: {
  onAddAccount?: () => void;
  onOpenAccounts?: () => void;
  onOpenHistory?: () => void;
  onOpenMarketData?: () => void;
  onOpenDataHealth?: () => void;
} = {}) {
  const { t, i18n } = useTranslation();
  const navigation = useObjectNavigation();
  const settings = useSettings();
  const overview = useOverview();
  const health = useMarketDataHealth();
  const healthKnown = Boolean(health.data) && !health.isError && !health.isPending;
  const healthIssueCount = health.data?.issueCount ?? 0;
  const pageChrome = <PageChrome pageId="overview" title={t("nav.overview")} actions={navigation && <Button variant="outline" onClick={() => navigation.open({ page: "historical-overview" })}>{t("historicalOverview.title")}</Button>} />;

  if (overview.isLoading) {
    return <>{pageChrome}<LoadingState label={t("ui.state.loadingPage")} /></>;
  }
  if (overview.isError || !overview.data) {
    return (
      <>
        {pageChrome}
        <ErrorState
          title={t("overview.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => overview.refetch()}
          retryLabel={t("common.retryAction")}
        />
      </>
    );
  }

  const data = overview.data as OverviewReadModel;
  const currency = data.currency || "USD";
  const missing = data.missingInputs ?? [];
  const updatedLabel =
    overview.dataUpdatedAt > 0 ? t("overview.updatedAt", { time: formatUpdatedAt(overview.dataUpdatedAt, i18n.language, settings.data?.timezone) }) : undefined;
  const healthBadge = data.complete ? (
    <Badge variant="success">{t("review.currentValuationComplete")}</Badge>
  ) : (
    <Badge variant="warning">{t("overview.needsAttention")}</Badge>
  );
  const headerStatus = (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="secondary">{t("overview.accountCount", { count: data.accountCount })}</Badge>
      {healthBadge}
      <DataHealthIndicator onOpen={onOpenDataHealth} />
      {updatedLabel && <p className="text-sm text-muted-foreground">{updatedLabel}</p>}
    </div>
  );

  if (data.accountCount === 0) {
    return (
      <div className="flex flex-col gap-6" data-testid="overview-page">
        {pageChrome}
        <PageIntro description={t("overview.description")} status={headerStatus} />
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
  const accountNames = new Map((data.accountLabels ?? []).map((label) => [label.id, label.name]));
  const instrumentNames = new Map((data.instrumentLabels ?? []).map((label) => [label.id, instrumentDisplayLabel(label, label.name)]));
  const quoteSourceByInstrument = new Map((data.instrumentLabels ?? []).map((label) => [label.id, label.quoteSource]));
  const holdingNames = new Map((data.holdingLabels ?? []).map((label) => [label.id, label.name]));
  const missingPriceSource = (item: OverviewMissingInput) => item.quoteSource || (item.instrumentId ? quoteSourceByInstrument.get(item.instrumentId) : undefined);
  const missingManualPrices = missing.filter((item) => {
    if (item.kind !== "instrument_price") {
      return false;
    }
    const source = missingPriceSource(item);
    return source !== "provider" && source !== "agent";
  }).length;
  const missingAgentPrices = missing.filter((item) => item.kind === "instrument_price" && missingPriceSource(item) === "agent").length;
  const missingProviderPrices = missingPrices - missingManualPrices - missingAgentPrices;
  const recent = data.recentActivities ?? [];
  const hasLiabilities = isPositiveCanonical(data.liabilities);
  const showLiabilityComposition = hasLiabilities && (data.liabilitiesByType ?? []).length > 0;
  const showAssetComposition = (data.assetsByType ?? []).length > 0;
  const historyReady = data.historyStarted;
  const historyNotStarted = !data.historyStarted;
  const noNextSteps = missingManualPrices === 0 && missingAgentPrices === 0 && missingProviderPrices === 0 && missingValues === 0 && missingFx === 0 && !historyNotStarted && !(historyReady && recent.length === 0) && healthKnown && healthIssueCount === 0;
  const missingFocus = (item: OverviewMissingInput): HealthFocus => ({
    instrumentId: item.instrumentId ?? undefined, accountId: item.kind === "account_value" ? item.accountId : undefined,
    currencyA: item.kind === "fx_rate" ? item.baseCurrency : undefined, currencyB: item.kind === "fx_rate" ? item.quoteCurrency : undefined,
    label: missingItemLabel(t, item, accountNames),
    kind: item.kind === "instrument_price" && missingPriceSource(item) === "agent" ? "missing_agent_price" : undefined,
    action: item.kind === "account_value" ? "account_value" : missingPriceSource(item) === "agent" ? "none" : missingPriceSource(item) === "manual" ? "manual_entry" : "repair",
  });

  const todoRows: React.ReactNode[] = [];
  if (!healthKnown) {
    todoRows.push(
      <TodoRow key="health-state" icon={HeartPulse} tone={8} text={t(health.isError ? "review.healthFailed" : "review.healthLoading")}
        actionLabel={health.isError ? t("common.retryAction") : undefined} onAction={() => void health.refetch()} />,
    );
  }
  const listedIssues = navigation ? (health.data?.issues ?? []).filter((issue) => !issue.collapsed).slice(0, 3) : [];
  const accountNameRecord = Object.fromEntries(accountNames);
  listedIssues.forEach((issue) => {
    todoRows.push(
      <TodoRow key={issue.id} icon={HeartPulse} tone={3} text={`${healthIssueLabel(t, issue, accountNameRecord)} · ${displayEnum(t, "dataHealth.kind", issue.kind)}`}
        actionLabel={t("connections.viewGap")} onAction={() => navigation?.openHealth(issue)} />,
    );
  });
  if (healthKnown && healthIssueCount > 0 && listedIssues.length === 0) {
    todoRows.push(
      <TodoRow key="health-issues" icon={HeartPulse} tone={3} text={t("dataHealth.indicatorIssues", { count: healthIssueCount })}
        actionLabel={onOpenDataHealth ? t("dataHealth.fixInDataHealth") : undefined} onAction={onOpenDataHealth} />,
    );
  }
  if (!navigation) {
    if (missingManualPrices > 0) {
      todoRows.push(<TodoRow key="manual-prices" icon={PencilLine} tone={6} text={t("overview.setManualPricesNext", { count: missingManualPrices })} actionLabel={t("overview.openMarketData")} onAction={onOpenMarketData} />);
    }
    if (missingAgentPrices > 0) {
      todoRows.push(<TodoRow key="agent-prices" icon={PencilLine} tone={6} text={t("overview.setAgentPricesNext", { count: missingAgentPrices })} actionLabel={t("overview.openMarketData")} onAction={onOpenMarketData} />);
    }
    if (missingProviderPrices > 0) {
      todoRows.push(<TodoRow key="provider-prices" icon={TrendingUp} tone={1} text={t("overview.refreshPricesNext", { count: missingProviderPrices })} actionLabel={t("overview.openMarketData")} onAction={onOpenMarketData} />);
    }
    if (missingFx > 0) {
      todoRows.push(<TodoRow key="fx" icon={ArrowLeftRight} tone={8} text={t("overview.refreshFxNext", { count: missingFx })} actionLabel={t("overview.openMarketData")} onAction={onOpenMarketData} />);
    }
    if (missingValues > 0) {
      todoRows.push(<TodoRow key="values" icon={Wallet} tone={5} text={t("overview.reviewValuesNext", { count: missingValues })} actionLabel={t("overview.openAccounts")} onAction={onOpenAccounts} />);
    }
  }
  if (historyNotStarted) {
    todoRows.push(<TodoRow key="start-history" icon={History} tone={6} text={t("overview.startHistoryNext")} actionLabel={t("overview.openHistory")} onAction={onOpenHistory} />);
  }
  if (historyReady && recent.length === 0) {
    todoRows.push(<TodoRow key="record-change" icon={CircleDollarSign} tone={2} text={t("overview.recordChangeNext")} actionLabel={t("overview.openHistory")} onAction={onOpenHistory} />);
  }

  // Recent activity grouped by local date, newest group first as delivered by the API.
  const activityGroups: Array<{ date: string; items: ActivityDTO[] }> = [];
  for (const activity of recent) {
    const last = activityGroups[activityGroups.length - 1];
    if (last && last.date === activity.effectiveLocalDate) last.items.push(activity);
    else activityGroups.push({ date: activity.effectiveLocalDate, items: [activity] });
  }

  return (
    <div className="flex flex-col gap-6" data-testid="overview-page">
      {pageChrome}
      <PageIntro description={t("overview.description")} status={headerStatus} />

      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="relative overflow-hidden bg-gradient-to-br from-primary/10 via-card to-card">
          <CardHeader>
            <CardTitle className="text-sm font-semibold text-muted-foreground">{t("overview.netWorth")}</CardTitle>
          </CardHeader>
          <CardContent className="@container">
            <p className="num text-4xl font-extrabold tracking-tight text-foreground sm:text-5xl" data-testid="overview-net-worth">
              {formatAmount(data.netWorth, currency)}
            </p>
            {!data.complete && (
              <p className="mt-2 text-sm text-warning-foreground">{t("overview.totalsExcludeMissing")}</p>
            )}
            <div className="mt-6 grid grid-cols-1 gap-3 @md:grid-cols-2">
              <div className="flex items-center gap-3 rounded-2xl bg-success/10 p-3">
                <IconTile icon={TrendingUp} tone={2} />
                <div className="min-w-0">
                  <p className="text-xs font-medium text-muted-foreground">{t("overview.assets")}</p>
                  <p className="num truncate text-lg font-bold text-success" data-testid="overview-assets">
                    {formatAmount(data.assets, currency)}
                  </p>
                </div>
              </div>
              <div className={cn("flex items-center gap-3 rounded-2xl p-3", hasLiabilities ? "bg-destructive/10" : "bg-muted/70")}>
                <IconTile icon={TrendingDown} tone={hasLiabilities ? 3 : 8} />
                <div className="min-w-0">
                  <p className="text-xs font-medium text-muted-foreground">{t("overview.liabilities")}</p>
                  <p className={cn("num truncate text-lg font-bold", hasLiabilities ? "text-destructive" : "text-foreground")} data-testid="overview-liabilities">
                    {formatAmount(data.liabilities, currency)}
                  </p>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <OverviewLiquidityCard />
      </div>

      {data.complete && noNextSteps ? (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-2xl border border-success/25 bg-success/10 px-5 py-4 text-sm">
          <CheckCircle2 className="size-5 text-success" aria-hidden="true" />
          <p className="font-semibold text-foreground">{t("overview.dataHealthComplete")}</p>
          <p className="text-muted-foreground">{t("overview.noNextSteps")}</p>
        </div>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>{t("overview.nextStepsTitle")}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {data.complete ? (
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <CheckCircle2 className="size-4 text-success" aria-hidden="true" />
                {t("overview.dataHealthComplete")}
              </p>
            ) : (
              <div role="alert" className="flex flex-col gap-3 rounded-xl bg-warning/12 p-4 text-sm">
                <p className="font-semibold text-warning-foreground">{t("overview.dataHealthIncomplete", { count: missing.length })}</p>
                {missing.length > 0 && (
                  <ul className="list-inside list-disc text-muted-foreground">
                    {missing.map((item, index) => (
                      <li key={`${item.kind}-${item.accountId}-${index}`}>
                        {navigation ? <Button variant="link" className="h-auto p-0 text-left" onClick={() => navigation.openHealth(missingFocus(item))}>{missingItemLabel(t, item, accountNames)} · {displayEnum(t, "overview.missingKind", item.kind)}</Button> : t("overview.missingInput", {
                          label: `${missingItemLabel(t, item, accountNames)} (${displayEnum(t, "overview.missingKind", item.kind)})`,
                        })}
                      </li>
                    ))}
                  </ul>
                )}
                {onOpenDataHealth && (
                  <Button type="button" size="sm" variant="outline" className="self-start" onClick={onOpenDataHealth}>
                    {t("dataHealth.fixInDataHealth")}
                  </Button>
                )}
              </div>
            )}
            {noNextSteps ? (
              <p className="text-sm text-muted-foreground">{t("overview.noNextSteps")}</p>
            ) : todoRows.length > 0 ? (
              <ul className="flex flex-col gap-3">{todoRows}</ul>
            ) : null}
            {listedIssues.length > 0 && onOpenDataHealth && (
              <Button type="button" size="sm" variant="ghost" className="self-start text-primary" onClick={onOpenDataHealth}>
                {t("dataHealth.fixInDataHealth")}
                {healthIssueCount > listedIssues.length && <span className="num text-muted-foreground">({healthIssueCount})</span>}
              </Button>
            )}
          </CardContent>
        </Card>
      )}

      <NetWorthTrendCard />

      <div className="grid grid-cols-1 gap-4">
        {(showAssetComposition || showLiabilityComposition) && (
          <div className={cn("grid grid-cols-1 gap-4", showAssetComposition && showLiabilityComposition && "xl:grid-cols-2")}>
            {showAssetComposition && (
              <Card>
                <CardHeader>
                  <CardTitle>{t("overview.composition")}</CardTitle>
                  <p className="text-sm font-normal text-muted-foreground">{t("overview.compositionHint")}</p>
                </CardHeader>
                <CardContent>
                  <CompositionChart
                    title={t("overview.composition")}
                    items={(data.assetsByType ?? []).map((item) => ({
                      key: item.key,
                      label: displayEnum(t, "enum", item.key),
                      amount: item.amount,
                      shareBps: item.shareBps,
                    }))}
                    currency={currency}
                    centerValue={data.assets}
                    centerCaption={t("overview.valuedSubtotal")}
                    ariaLabel={t("overview.composition")}
                    summary={t("overview.compositionHint")}
                    height={280}
                    complete={data.complete}
                    missingCount={missing.length}
                  />
                </CardContent>
              </Card>
            )}
            {showLiabilityComposition && (
              <Card>
                <CardHeader>
                  <CardTitle>{t("overview.liabilitiesComposition")}</CardTitle>
                </CardHeader>
                <CardContent>
                  <CompositionChart
                    title={t("overview.liabilitiesComposition")}
                    items={(data.liabilitiesByType ?? []).map((item) => ({
                      key: item.key,
                      label: displayEnum(t, "enum", item.key),
                      amount: item.amount,
                      shareBps: item.shareBps,
                    }))}
                    currency={currency}
                    centerValue={data.liabilities}
                    centerCaption={t("overview.valuedSubtotal")}
                    ariaLabel={t("overview.liabilitiesComposition")}
                    summary={t("overview.liabilitiesComposition")}
                    height={280}
                  />
                </CardContent>
              </Card>
            )}
          </div>
        )}
        <p className="text-sm text-muted-foreground">{t("overview.dimensionHint")}</p>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <BreakdownList
            title={t("overview.byInstitution")}
            onSelect={navigation ? item => navigation.open({ page: "accounts", filter: { dimension: "institutionId", value: item.key || "unassigned", label: item.label } }) : undefined}
            items={(data.byInstitution ?? []).map((item) => ({
              ...item,
              label: !item.key || item.key === "unassigned" ? t("overview.unassignedInstitution") : item.label,
            }))}
            currency={currency}
          />
          <BreakdownList
            title={t("overview.byAccountType")}
            onSelect={navigation ? item => navigation.open({ page: "accounts", filter: { dimension: "accountType", value: item.key, label: item.label } }) : undefined}
            description={t("overview.byAccountTypeHint")}
            items={(data.byAccountType ?? []).map((item) => ({ ...item, label: displayEnum(t, "enum", item.key) }))}
            currency={currency}
          />
          <BreakdownList title={t("overview.byMember")} onSelect={navigation ? item => navigation.open({ page: "accounts", filter: { dimension: "memberId", value: item.key, label: item.label } }) : undefined} items={data.byMember ?? []} currency={currency} />
          <BreakdownList title={t("overview.byGroup")} onSelect={navigation ? item => navigation.open({ page: "accounts", filter: { dimension: "groupId", value: item.key, label: item.label } }) : undefined} items={(data.byGroup ?? []).map(item => ({ ...item, label: !item.key || item.key === "unassigned" ? t("connections.noGroup") : item.label }))} currency={currency} />
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
          {recent.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("overview.noRecentActivity")}</p>
          ) : (
            <div className="flex flex-col gap-5" data-testid="overview-recent-activity">
              {activityGroups.map((group) => (
                <section key={group.date} className="flex flex-col gap-3">
                  <h3 className="num text-xs font-semibold text-muted-foreground">
                    <time dateTime={group.items[0].effectiveAt}>{group.date}</time>
                  </h3>
                  <ul className="flex flex-col gap-3">
                    {group.items.map((activity) => {
                      const style = activityStyle(activity.kind);
                      return (
                        <li key={activity.id} className="flex items-center gap-3 text-sm">
                          <IconTile icon={style.icon} tone={style.tone} size="sm" />
                          <p className="text-foreground">{activitySentence(t, activity, accountNames, instrumentNames, holdingNames)}</p>
                        </li>
                      );
                    })}
                  </ul>
                </section>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
