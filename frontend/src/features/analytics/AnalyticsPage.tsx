import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/select";
import { TrendChart } from "@/components/charts/TrendChart";
import { SignedBarChart } from "@/components/charts/SignedBarChart";
import { RangeToggle } from "@/components/charts/RangeToggle";
import { chartTheme } from "@/components/charts/chartTheme";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { useRealizedGain, useDividendIncome, useNetWorthTrend, type AnalyticsRange, type AnalyticsTrendRange, portfolioScope } from "@/queries/analytics";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";
import { useDailySnapshotState } from "@/queries/history";
import { useCatalog } from "@/queries/catalog";
import { formatAmount } from "@/lib/money";
import { cn } from "@/lib/utils";

/** Analytics separates trend context from realized-gain detail and exposes
 * the same chart data in an accessible table disclosure. Snapshot rebuilds
 * stay in the Go trend read path, which chunks the 31-day backend limit. */
export function AnalyticsPage() {
  const { t } = useTranslation();
  const catalog = useCatalog();
  const ranges = catalog.data?.trendRanges ?? [];
  const [range, setRange] = useState<AnalyticsRange>({ kind: "trend", value: "30d" });
  const [customFrom, setCustomFrom] = useState("");
  const [customTo, setCustomTo] = useState("");
  const [scopeKind, setScopeKind] = useState<"portfolio" | "account" | "instrument">("portfolio");
  const [scopeId, setScopeId] = useState("");
  const accounts = useAccounts();
  const instruments = useInstruments();
  const scope = scopeKind === "account" && scopeId ? { accountId: scopeId } : scopeKind === "instrument" && scopeId ? { instrumentId: scopeId } : portfolioScope;
  const snapshotState = useDailySnapshotState();
  const [gainDimension, setGainDimension] = useState<"instrument" | "account">("instrument");
  const realizedGain = useRealizedGain(range, scope);
  const dividendIncome = useDividendIncome(range, scope);
  const netWorthTrend = useNetWorthTrend(range);
  const theme = chartTheme();
  const pageChrome = <PageChrome pageId="analytics" title={t("nav.analytics")} />;

  if (realizedGain.isLoading || dividendIncome.isLoading || netWorthTrend.isLoading) {
    return <>{pageChrome}<LoadingState label={t("analytics.loading")} /></>;
  }

  if (realizedGain.isError || dividendIncome.isError) {
    return (
      <>
        {pageChrome}
        <ErrorState
          title={t("analytics.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => {
            void realizedGain.refetch();
            void dividendIncome.refetch();
            void netWorthTrend.refetch();
          }}
          retryLabel={t("common.retryAction")}
        />
      </>
    );
  }

  const points = netWorthTrend.data?.points ?? [];
  const gainData = realizedGain.data;
  const incomeData = dividendIncome.data;
  const gainGroups = gainDimension === "instrument" ? (gainData?.byInstrument ?? []) : (gainData?.byAccount ?? []);
  const incomeGroups = incomeData?.byInstrument ?? [];

  return (
    <div className="flex flex-col gap-6">
      {pageChrome}
      <PageIntro description={t("analytics.description")} />
      {snapshotState.data && <p role="status" className="text-sm text-muted-foreground">{snapshotState.data.dirtyFrom ? t("analytics.snapshotNeedsRepair", { date: snapshotState.data.dirtyFrom }) : snapshotState.data.lastCompletedClosedOn ? t("analytics.snapshotHealthyThrough", { date: snapshotState.data.lastCompletedClosedOn }) : t("analytics.snapshotNotBuilt")}</p>}

      <div className="flex flex-wrap items-end gap-3">
        <RangeToggle
          ranges={ranges}
          value={range.kind === "trend" ? range.value : ""}
          onChange={(value) => {
            setRange({ kind: "trend", value: value as AnalyticsTrendRange });
          }}
          label={t("analytics.range")}
        />
        <div className="flex flex-col gap-1.5">
          <label htmlFor="analytics-scope" className="text-sm font-medium">{t("analytics.scope")}</label>
          <NativeSelect id="analytics-scope" value={scopeKind} onChange={(event) => { setScopeKind(event.target.value as typeof scopeKind); setScopeId(""); }}>
            <option value="portfolio">{t("analytics.scopePortfolio")}</option>
            <option value="account">{t("analytics.scopeAccount")}</option>
            <option value="instrument">{t("analytics.scopeInstrument")}</option>
          </NativeSelect>
        </div>
        {scopeKind !== "portfolio" && (
          <div className="flex flex-col gap-1.5">
            <label htmlFor="analytics-scope-id" className="text-sm font-medium">{t("analytics.scopeSelection")}</label>
            <NativeSelect id="analytics-scope-id" value={scopeId} onChange={(event) => setScopeId(event.target.value)}>
              <option value="">{t("common.selectOption")}</option>
              {(scopeKind === "account" ? accounts.data ?? [] : instruments.data ?? []).map((item) => {
                const id = "account" in item ? item.account.id : item.id;
                const label = "account" in item ? item.account.name : item.name;
                return <option key={id} value={id}>{label}</option>;
              })}
            </NativeSelect>
          </div>
        )}
      </div>
      <div className="flex flex-wrap items-end gap-2 rounded-md border border-border bg-muted/30 p-3">
        <div className="flex flex-col gap-1.5"><label htmlFor="analytics-from" className="text-sm font-medium">{t("analytics.customFrom")}</label><input id="analytics-from" type="date" value={customFrom} onChange={(event) => setCustomFrom(event.target.value)} className="h-9 rounded-md border border-input bg-background px-3 text-sm" /></div>
        <div className="flex flex-col gap-1.5"><label htmlFor="analytics-to" className="text-sm font-medium">{t("analytics.customTo")}</label><input id="analytics-to" type="date" value={customTo} onChange={(event) => setCustomTo(event.target.value)} className="h-9 rounded-md border border-input bg-background px-3 text-sm" /></div>
        <Button type="button" variant="outline" disabled={!customFrom || !customTo || customFrom > customTo} onClick={() => setRange({ kind: "custom", from: customFrom, to: customTo })}>{t("analytics.applyCustomRange")}</Button>
        {range.kind === "custom" && <span className="text-sm text-muted-foreground">{t("analytics.customRangeActive")}</span>}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("analytics.wealthTrend")}</CardTitle>
        </CardHeader>
        <CardContent>
          {range.kind === "custom" ? <p className="text-sm text-muted-foreground">{t("analytics.customWealthUnavailable")}</p> : netWorthTrend.isError ? (
            <ErrorState
              title={t("analytics.loadError")}
              description={t("ui.state.errorDescription")}
              onRetry={() => {
                void netWorthTrend.refetch();
              }}
              retryLabel={t("common.retryAction")}
            />
          ) : (
            <TrendChart
              ariaLabel={t("analytics.wealthTrend")}
              summary={t("analytics.wealthSummary")}
              dates={points.map((point) => point.localDate)}
              series={[
                { key: "netWorth", name: t("charts.netWorth"), color: theme.primary, values: points.map((point) => point.netWorth?.amount) },
                { key: "assets", name: t("charts.assets"), color: theme.success, values: points.map((point) => point.assets?.amount) },
                { key: "liabilities", name: t("charts.liabilities"), color: theme.destructive, values: points.map((point) => point.liabilities?.amount) },
              ]}
              currency={netWorthTrend.data?.currency || "USD"}
              height={320}
              emptyTitle={t("analytics.trendEmpty")}
              extraTableColumns={[t("charts.date"), t("charts.netWorth"), t("charts.assets"), t("charts.liabilities"), t("charts.incomplete")]}
              extraTableRows={points.map((point) => [
                point.localDate,
                point.netWorth ? formatAmount(point.netWorth.amount, point.netWorth.currency) : t("accounts.noValue"),
                point.assets ? formatAmount(point.assets.amount, point.assets.currency) : t("accounts.noValue"),
                point.liabilities ? formatAmount(point.liabilities.amount, point.liabilities.currency) : t("accounts.noValue"),
                point.complete ? t("charts.complete") : t("charts.incomplete"),
              ])}
            />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("analytics.periodSummary")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <SummaryItem label={t("analytics.periodStart")} value={netWorthTrend.data?.start ? formatAmount(netWorthTrend.data.start.amount, netWorthTrend.data.start.currency) : t("accounts.noValue")} />
          <SummaryItem label={t("analytics.periodEnd")} value={netWorthTrend.data?.end ? formatAmount(netWorthTrend.data.end.amount, netWorthTrend.data.end.currency) : t("accounts.noValue")} />
          <SummaryItem label={t("analytics.periodChange")} value={netWorthTrend.data?.change ? formatAmount(netWorthTrend.data.change.amount, netWorthTrend.data.change.currency) : t("accounts.noValue")} />
          <SummaryItem label={t("analytics.realizedGain")} value={gainData?.total ? formatAmount(gainData.total.amount, gainData.total.currency) : t("accounts.noValue")} warning={gainData && !gainData.available} />
          <SummaryItem label={t("analytics.dividendIncome")} value={incomeData?.total ? formatAmount(incomeData.total.amount, incomeData.total.currency) : t("accounts.noValue")} warning={incomeData && !incomeData.available} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className={cn(gainData && !gainData.available && "text-warning")}>
            {t("analytics.realizedGain")} {gainData && !gainData.available && `(${t("analytics.statusPartial")})`}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap gap-2" role="group" aria-label={t("analytics.realizedGain")}>
            <Button type="button" size="sm" variant={gainDimension === "instrument" ? "default" : "outline"} onClick={() => setGainDimension("instrument")}>
              {t("analytics.byInstrument")}
            </Button>
            <Button type="button" size="sm" variant={gainDimension === "account" ? "default" : "outline"} onClick={() => setGainDimension("account")}>
              {t("analytics.byAccount")}
            </Button>
          </div>
          <SignedBarChart
            ariaLabel={t("analytics.realizedGain")}
            summary={t("analytics.realizedGain")}
            items={gainGroups.map((group) => ({
              key: group.key,
              label: group.label,
              amount: group.gain.amount,
              currency: group.gain.currency,
              available: group.available,
            }))}
            emptyLabel={t("analytics.empty")}
            allowNegative
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className={cn(incomeData && !incomeData.available && "text-warning")}>
            {t("analytics.dividendIncome")} {incomeData && !incomeData.available && `(${t("analytics.statusPartial")})`}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <SignedBarChart
            ariaLabel={t("analytics.dividendIncome")}
            summary={t("analytics.dividendIncome")}
            items={incomeGroups.map((group) => ({
              key: group.key,
              label: group.label,
              amount: group.gain.amount,
              currency: group.gain.currency,
              available: group.available,
            }))}
            emptyLabel={t("analytics.empty")}
            allowNegative={false}
          />
        </CardContent>
      </Card>
    </div>
  );
}

function SummaryItem({ label, value, warning }: { label: string; value: string; warning?: boolean }) {
  return (
    <div>
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className={cn("mt-1 text-lg font-semibold", warning && "text-warning")}>{value}</p>
    </div>
  );
}
