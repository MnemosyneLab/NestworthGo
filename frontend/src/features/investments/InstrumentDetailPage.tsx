import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import type { InstrumentDetailContext } from "@/app/navigation";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAccounts } from "@/queries/accounts";
import { useInstrumentHoldingGroups } from "@/queries/analytics";
import { useCurrentInstrumentQuote, useInstruments } from "@/queries/investments";
import { useSettings } from "@/queries/settings";
import { displayEnum } from "@/lib/display";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { formatAmount, formatQuotePrice, formatSignedMoney } from "@/lib/money";
import { formatQuoteAsOf } from "@/lib/time";
import { QuoteHistoryContent } from "@/features/marketdata/QuoteHistorySheet";
import { AnalyzeMenu } from "@/features/insights/AnalyzeMenu";
import { PerformanceReturns } from "./PerformanceReturns";
import { PricePerformancePanel } from "./PricePerformancePanel";
import type { HoldingAmountsDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function HoldingFigures({ amounts }: { amounts: HoldingAmountsDTO }) {
  const { t } = useTranslation();
  const fields = [
    [t("portfolio.quantity"), formatAmount(amounts.quantity)],
    [t("portfolio.cost"), amounts.totalCost ? formatAmount(amounts.totalCost.amount, amounts.totalCost.currency) : t("accounts.noValue")],
    [t("portfolio.currentValue"), amounts.currentValue ? formatAmount(amounts.currentValue.amount, amounts.currentValue.currency) : t("accounts.noValue")],
    [t("portfolio.unrealizedGain"), amounts.unrealizedGain ? formatSignedMoney(amounts.unrealizedGain.amount, amounts.unrealizedGain.currency) : t("accounts.noValue")],
  ];
  return <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
    {fields.map(([label, value]) => <div key={label} className="rounded-xl bg-muted/40 p-3"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="num mt-1 font-semibold">{value}</dd></div>)}
  </dl>;
}

function ManagedProductEntry({ productId }: { productId: string }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  return <div className="flex flex-col items-start gap-3 rounded-2xl border border-border bg-card p-4">
    <p className="text-sm text-muted-foreground">{t("availableFunds.managedInstrumentHint")}</p>
    {navigation && <Button type="button" variant="outline" onClick={() => navigation.open({ page: "available-funds", productId })}>{t("availableFunds.openPage")}</Button>}
  </div>;
}

function HoldingProjection({ instrument, holding, accountId, from, to }: {
  instrument: InstrumentDTO;
  holding?: InstrumentHoldingsDTO;
  accountId?: string;
  from?: string;
  to?: string;
}) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const accounts = useAccounts({ includeArchived: true });
  const accountName = accountId ? accounts.data?.find((record) => record.account.id === accountId)?.account.name : undefined;
  const member = accountId ? (holding?.holdings ?? []).find((item) => item.accountId === accountId) : undefined;
  const amounts = accountId ? member?.amounts : holding?.amounts;
  const scope = accountId ? t("instrumentDetail.accountScope", { name: accountName ?? member?.accountName ?? accountId }) : t("instrumentDetail.allAccounts");

  if (instrument.managedProductId) return <ManagedProductEntry productId={instrument.managedProductId} />;

  return <section aria-label={t("instrumentDetail.holdings")} className="flex flex-col gap-4">
    <p className="text-sm text-muted-foreground">{t("instrumentDetail.personalNote")}</p>
    <p className="text-sm font-medium">{scope}</p>
    {amounts ? <HoldingFigures amounts={amounts} /> : <EmptyState title={t("instrumentDetail.noHolding")} />}
    {!accountId && (holding?.holdings ?? []).length > 1 && <ul className="flex flex-col gap-2" aria-label={t("portfolio.accounts")}>
      {(holding?.holdings ?? []).map((item) => <li key={item.holdingId} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-border px-3 py-2 text-sm">
        <span>{item.accountName}</span>
        <span className="num">{formatAmount(item.amounts.quantity)}</span>
        {navigation && <Button type="button" variant="link" size="sm" onClick={() => navigation.open({ page: "instrument-detail", instrumentId: instrument.id, accountId: item.accountId, section: "holdings", from, to })}>{t("investmentAssets.openInstrument", { name: item.accountName })}</Button>}
      </li>)}
    </ul>}
    <div className="flex flex-wrap items-center gap-2">
      <AnalyzeMenu instrumentId={instrument.id} accountId={accountId} label={instrumentDisplayLabel(instrument)} from={from} to={to} inline includeHistory={false} />
      {navigation && <Button id="instrument-detail-related-transactions" type="button" variant="outline" size="sm" onClick={() => navigation.open({ page: "history", filters: { instrumentId: instrument.id, accountId, from, to } })}>{t("instrumentDetail.transactions")}</Button>}
    </div>
  </section>;
}

/** One read-only destination for account and household holdings, market performance, and source data. */
export function InstrumentDetailPage({ context, onContextChange, active = true }: {
  context: InstrumentDetailContext;
  onContextChange: (context: InstrumentDetailContext) => void;
  active?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const navigation = useObjectNavigation();
  const instruments = useInstruments(true);
  const groups = useInstrumentHoldingGroups(true, context.accountId ?? "");
  const quote = useCurrentInstrumentQuote(context.instrumentId);
  const settings = useSettings();
  const instrument = instruments.data?.find((item) => item.id === context.instrumentId);
  const holding = (groups.data ?? []).flatMap((group) => group.instruments ?? []).find((item) => item.instrumentId === context.instrumentId);
  const section = context.section ?? "holdings";
  const focus = { ...context.healthFocus, instrumentId: context.instrumentId,
    label: context.healthFocus?.label ?? (instrument ? instrumentDisplayLabel(instrument) : undefined),
    rangeStart: context.from ?? context.healthFocus?.rangeStart,
    rangeEnd: context.to ?? context.healthFocus?.rangeEnd };

  if (instruments.isLoading) return <LoadingState label={t("ui.state.loadingPage")} />;
  if (instruments.isError) return <ErrorState title={t("portfolio.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void instruments.refetch()} retryLabel={t("common.retryAction")} />;
  if (!instrument) return <EmptyState title={t("instrumentDetail.unavailable")}
    action={navigation && <Button variant="outline" onClick={() => navigation.open({ page: "portfolio", view: "instruments" })}>{t("investmentAssets.instruments")}</Button>} />;

  const label = instrumentDisplayLabel(instrument);
  const attributes = [
    [t("accounts.instrumentType"), displayEnum(t, "enum", instrument.type)],
    [t("history.currency"), instrument.quoteCurrency],
    [t("portfolio.symbol"), instrument.symbol],
    [t("portfolio.marketCode"), instrument.marketCode],
    [t("portfolio.countryCode"), instrument.countryCode],
    ["ISIN", instrument.isin],
  ].filter(([, value]) => value);

  return <div className="flex min-w-0 flex-col gap-5" data-testid="instrument-detail-page">
    <PageChrome pageId="instrument-detail" enabled={active} title={label} />
    <div className="flex flex-wrap items-center gap-2">
      <h2 className="text-xl font-semibold">{label}</h2>
      {instrument.archivedAt && <Badge variant="secondary">{t("instrumentDetail.archived")}</Badge>}
    </div>
    <dl className="grid grid-cols-2 gap-x-4 gap-y-2 rounded-2xl border border-border bg-card p-4 text-sm sm:grid-cols-3">
      {attributes.map(([name, value]) => <div key={name}><dt className="text-xs text-muted-foreground">{name}</dt><dd className="break-words">{value}</dd></div>)}
    </dl>
    {instrument.note && <details className="text-sm"><summary className="cursor-pointer">{t("accounts.note")}</summary><p className="mt-2 whitespace-pre-wrap break-words">{instrument.note}</p></details>}
    <Tabs value={section} onValueChange={(value) => onContextChange({ ...context, section: value as InstrumentDetailContext["section"] })}>
      <TabsList aria-label={t("instrumentDetail.title")}>
        <TabsTrigger value="holdings">{t("instrumentDetail.holdings")}</TabsTrigger>
        <TabsTrigger value="performance">{t("instrumentDetail.performance")}</TabsTrigger>
        <TabsTrigger value="data">{t("instrumentDetail.data")}</TabsTrigger>
      </TabsList>
      <TabsContent value="holdings">
        {groups.isLoading ? <LoadingState label={t("portfolio.loading")} /> : groups.isError ? <ErrorState title={t("portfolio.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void groups.refetch()} retryLabel={t("common.retryAction")} />
          : <HoldingProjection instrument={instrument} holding={holding} accountId={context.accountId} from={context.from} to={context.to} />}
      </TabsContent>
      <TabsContent value="performance" className="flex flex-col gap-4">
        {instrument.managedProductId ? <ManagedProductEntry productId={instrument.managedProductId} /> : <>
          <p className="text-sm text-muted-foreground">{t("instrumentDetail.marketNote")}</p>
          <PricePerformancePanel instrument={instrument} />
          <PerformanceReturns instrumentId={instrument.id} />
        </>}
      </TabsContent>
      <TabsContent value="data" className="flex flex-col gap-4">
        {instrument.managedProductId ? <ManagedProductEntry productId={instrument.managedProductId} /> : <>
        <p className="text-sm text-muted-foreground">{t("instrumentDetail.dataNote")}</p>
        <section className="rounded-2xl border border-border bg-card p-4" aria-label={t("instrumentDetail.currentQuote")}>
          <h3 className="text-sm font-medium">{t("instrumentDetail.currentQuote")}</h3>
          {quote.isLoading ? <LoadingState label={t("ui.state.loadingPage")} /> : quote.isError ? <ErrorState title={t("portfolio.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void quote.refetch()} retryLabel={t("common.retryAction")} />
            : <p className="num mt-2 text-xl font-semibold">{quote.data ? formatQuotePrice(quote.data.unitPrice, quote.data.currency) : t("portfolio.noCurrentPrice")}</p>}
          {quote.data && <p className="text-xs text-muted-foreground">{t("portfolio.quotedAsOf", { time: formatQuoteAsOf(quote.data, settings.data?.timezone, i18n.language) })} · {t("instrumentDetail.quoteSource")}: {displayEnum(t, "portfolio", quote.data.sourceKind)}{quote.data.sourceKey && quote.data.sourceKey !== quote.data.sourceKind ? ` · ${quote.data.sourceKey}` : ""}</p>}
        </section>
        <QuoteHistoryContent target={{ kind: "instrument", instrument }} focus={focus}
          onRangeChange={({ from, to }) => onContextChange({ ...context, from, to,
            healthFocus: { ...context.healthFocus, rangeStart: from, rangeEnd: to } })} />
        {navigation && <Button id="instrument-detail-maintain" type="button" variant="outline" className="self-start" onClick={() => navigation.open({ page: "market-data", focus: { ...focus, action: "instrument_editor" } })}>{t("instrumentDetail.maintain")}</Button>}
        </>}
      </TabsContent>
    </Tabs>
  </div>;
}
