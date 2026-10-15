import { quoteTrendProps } from "./quoteTrend";
import type { HealthFocus } from "@/app/navigation";
import { metalPriceSuffix } from "@/lib/preciousMetals";
import { type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { TrendChart } from "@/components/charts/TrendChart";
import { SourceFilterToggle } from "@/components/charts/RangeToggle";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";
import { useInstrumentQuoteSeries, useFXQuoteSeries } from "@/queries/investments";
import { DateRangeControl } from "@/components/charts/DateRangeControl";
import { useTrendDateRange } from "@/components/charts/useTrendDateRange";
import { useSettings } from "@/queries/settings";
import { instrumentDisplayLabel, instrumentSecondaryName } from "@/lib/instrumentDisplay";
import { chartTheme, useThemeVersion } from "@/components/charts/chartTheme";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export type QuoteHistoryTarget =
  | { kind: "instrument"; instrument: InstrumentDTO }
  | { kind: "fx"; currencyA: string; currencyB: string };

export function QuoteHistorySheet({
  target, focus,
  onClose,
}: {
  target: QuoteHistoryTarget;
  focus?: HealthFocus;
  onClose: () => void;
}) {
  return <QuoteHistoryContent target={target} focus={focus} render={(body,title) => <Sheet open onOpenChange={open => { if (!open) onClose(); }}><SheetContent className="max-w-xl overflow-y-auto" side="right"><SheetHeader><SheetTitle>{title}</SheetTitle><SheetDescription>{target.kind === "instrument" ? instrumentSecondaryName(target.instrument) : title}</SheetDescription></SheetHeader>{body}</SheetContent></Sheet>} />;
}

export function QuoteHistoryContent({ target, focus, render }: { target: QuoteHistoryTarget; focus?: HealthFocus; render?: (body: ReactNode, title: string) => ReactNode }) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const range = useTrendDateRange(focus?.rangeStart && focus.rangeEnd ? { from: focus.rangeStart, to: focus.rangeEnd } : undefined, true);
  const [sourceFilter, setSourceFilter] = useState("all");
  const [swapped, setSwapped] = useState(false);
  const instrumentId = target.kind === "instrument" ? target.instrument.id : "";
  const fxBase = target.kind === "fx" ? (swapped ? target.currencyB : target.currencyA) : "";
  const fxQuote = target.kind === "fx" ? (swapped ? target.currencyA : target.currencyB) : "";
  const instrumentSeries = useInstrumentQuoteSeries(instrumentId, range.queryRange, sourceFilter, target.kind === "instrument");
  const fxSeries = useFXQuoteSeries(fxBase, fxQuote, range.queryRange, sourceFilter, target.kind === "fx");
  const series = target.kind === "instrument" ? instrumentSeries : fxSeries;
  useThemeVersion();
  const theme = chartTheme();

  const title = target.kind === "instrument"
    ? t("marketData.instrumentHistory", { name: instrumentDisplayLabel(target.instrument, target.instrument.name) })
    : t("marketData.fxHistory", { base: fxBase, quote: fxQuote });

  const observations = series.data?.observations ?? [];
  const currency = target.kind === "instrument" ? target.instrument.quoteCurrency : undefined;
  const priceSuffix = target.kind === "instrument" ? metalPriceSuffix(target.instrument, t) : "";
  const emptyRange = !series.isLoading && !series.isError && observations.length === 0;
  const hasFactsOutsideRange = emptyRange && series.data?.outsideRange === true;

  const body = (
    <>
        {target.kind === "instrument" && target.instrument.metalTemplate && <p className="mt-3 text-xs text-muted-foreground">{t("metals.referenceDisclaimer")}</p>}
        {target.kind === "instrument" && target.instrument.providerKey === "coingecko" && <p className="mt-3 text-xs text-muted-foreground">{t("portfolio.coinGeckoNotice")} · <a href="https://www.coingecko.com/en/api" target="_blank" rel="noreferrer" className="underline">{t("portfolio.coinGeckoAttribution")}</a></p>}
        <div className="flex flex-col gap-4">
          <DateRangeControl {...range} />
          <SourceFilterToggle value={sourceFilter} onChange={setSourceFilter} />
          <p className="text-sm text-muted-foreground">{t("quoteDetails.help")}</p>
          {currency && observations.some(item => item.currency && item.currency !== currency) && <p className="text-xs text-muted-foreground">{t("holdingDetail.otherCurrencies")}</p>}
          {target.kind === "fx" && (
            <>
              <p className="text-sm text-muted-foreground">{t("marketData.fxDailyReference")}</p>
              <Button type="button" variant="outline" size="sm" onClick={() => setSwapped((current) => !current)}>
                {t("charts.swapDirection")}
              </Button>
            </>
          )}
          {series.isLoading ? (
            <LoadingState label={t("ui.state.loadingPage")} />
          ) : series.isError ? (
            <ErrorState
              title={t("marketData.loadError")}
              description={t("ui.state.errorDescription")}
              onRetry={() => void series.refetch()}
              retryLabel={t("common.retryAction")}
            />
          ) : emptyRange ? (
            <EmptyState
              title={hasFactsOutsideRange ? t("charts.rangeEmpty") : t("charts.noLocalHistory")}
              action={hasFactsOutsideRange ? (
                <Button type="button" variant="outline" size="sm" onClick={() => range.onChange({ from: range.min, to: range.max })}>
                  {t("charts.showAllRange")}
                </Button>
              ) : undefined}
            />
          ) : (
            <TrendChart
              ariaLabel={title}
              summary={title}
              {...quoteTrendProps({ series: series.data, kind: target.kind, currency, priceSuffix, color: theme.primary, timezone: settings.data?.timezone, language: i18n.language, t })}
              height={300}
              emptyTitle={t("charts.insufficientHistory")}
            />
          )}
        </div>
    </>
  );
  return render ? render(body,title) : body;
}
