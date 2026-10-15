import { useId } from "react";
import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Button } from "@/components/ui/button";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { QuoteHistoryContent } from "@/features/marketdata/QuoteHistorySheet";
import { useInstruments, useQuotePerformance } from "@/queries/investments";
import { formatSignedMoney, formatAmount, formatDecimalPercent } from "@/lib/money";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { displayEnum } from "@/lib/display";
import type { HoldingAmountsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

function PricePerformance({ instrumentId }: { instrumentId: string }) {
  const { t } = useTranslation();
  const performance = useQuotePerformance(instrumentId);
  if (performance.isLoading) return <LoadingState label={t("ui.state.loadingPage")} />;
  if (performance.isError) return <ErrorState title={t("marketData.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void performance.refetch()} retryLabel={t("common.retryAction")} />;
  const items = performance.data?.items ?? [];
  const reason = performance.data?.unavailableReason;
  const unavailableLabel = reason === "incomparable_daily_prices" ? t("holdingDetail.unavailable.incomparable_daily_prices")
    : reason === "unsupported_instrument" ? t("holdingDetail.unavailable.unsupported_instrument")
    : t("holdingDetail.unavailable.insufficient_daily_history");
  if (!items.length) return <section aria-label={t("holdingDetail.performance")}>
    <h4 className="font-medium">{t("holdingDetail.performance")}</h4>
    <p className="text-sm text-muted-foreground">{unavailableLabel}</p>
  </section>;
  return (
    <section aria-label={t("holdingDetail.performance")}>
      <h4 className="font-medium">{t("holdingDetail.performance")}</h4>
      <p className="text-xs text-muted-foreground">{t("holdingDetail.priceDisclaimer")}</p>
      <div className="mt-2 flex flex-wrap gap-3">
        {items.map(item => (
          <div key={item.windowDays} className="rounded-lg border border-border p-3">
            <p>{t("holdingDetail.window", { days: item.windowDays })} · {t(item.annualizedPercent != null ? "holdingDetail.annualized" : "holdingDetail.priceChange")}</p>
            <p className="num">{formatDecimalPercent(item.annualizedPercent ?? item.periodPercent)}</p>
            {item.annualizedPercent != null && <p className="text-xs text-muted-foreground">{t("holdingDetail.priceChange")}: {formatDecimalPercent(item.periodPercent)}</p>}
            <p className="text-xs">{item.baselineOn} → {item.latestOn} · {t("holdingDetail.days", { days: item.elapsedDays })}</p>
            <p className="text-xs">{t("holdingDetail.asOf", { date: item.latestOn })}</p>
          </div>
        ))}
      </div>
    </section>
  );
}

type Props = {
  instrumentId: string;
  amounts?: HoldingAmountsDTO;
  scope: string;
  amountsLoading?: boolean;
  amountsError?: boolean;
  onRetry?: () => void;
};

/** Shared read-only body. Hosts mount only for expanded, active rows. */
export function HoldingDetailPanel({ instrumentId, amounts, scope, amountsLoading = false, amountsError = false, onRetry }: Props) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const headingId = useId();
  const instruments = useInstruments(true);
  const instrument = instruments.data?.find(item => item.id === instrumentId);
  const attributes = instrument ? [
    [t("accounts.instrumentType"), displayEnum(t, "enum", instrument.type)],
    [t("history.currency"), instrument.quoteCurrency],
    [t("portfolio.countryCode"), instrument.countryCode],
    [t("portfolio.marketCode"), instrument.marketCode],
    [t("portfolio.symbol"), instrument.symbol],
    ["ISIN", instrument.isin],
  ].filter(([, value]) => value) : [];
  const amountKeys = ["totalCost", "currentValue", "unrealizedGain"] as const;
  const quantity = amounts ? formatAmount(amounts.quantity) : t("accounts.noValue");

  return (
    <div role="region" aria-labelledby={headingId} className="flex min-w-0 flex-col gap-4 rounded-lg bg-muted/20 p-4">
      <h3 id={headingId} className="font-semibold">{instrument ? instrumentDisplayLabel(instrument) : t("portfolio.unknownInstrument")} · {scope}</h3>
      {amountsLoading ? <LoadingState label={t("ui.state.loadingPage")} /> : amountsError ? (
        <ErrorState title={t("portfolio.loadError")} description={t("ui.state.errorDescription")} onRetry={onRetry} retryLabel={t("common.retryAction")} />
      ) : (
        <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-3">
          {amountKeys.map((key, index) => {
            const value = amounts?.[key];
            const reason = key === "totalCost" ? amounts?.costMissingReason : key === "currentValue" ? amounts?.valueMissingReason : amounts?.gainMissingReason;
            return <div key={key}>
              <dt className="text-muted-foreground">{t(["portfolio.cost", "portfolio.currentValue", "portfolio.unrealizedGain"][index])}</dt>
              <dd className="num text-base font-semibold">{value ? (key === "unrealizedGain" ? formatSignedMoney : formatAmount)(value.amount, value.currency) : t("accounts.noValue")}</dd>
              {!value && reason && <dd className="text-xs text-muted-foreground">{displayEnum(t, "portfolio.missingReason", reason)}</dd>}
            </div>;
          })}
        </dl>
      )}
      <p className="text-xs text-muted-foreground">{t("portfolio.quantity")}: {instrument?.managedProductId && amounts ? t("availableFunds.contractQuantity", { quantity }) : quantity}</p>
      {instrument?.managedProductId && navigation && <Button type="button" variant="outline" onClick={() => navigation.open({ page: "available-funds", productId: instrument.managedProductId })}>{t("availableFunds.openPage")}</Button>}
      {instrument && !instrument.managedProductId && <><PricePerformance instrumentId={instrumentId} /><QuoteHistoryContent target={{ kind: "instrument", instrument }} /></>}
      {instruments.isLoading && <LoadingState label={t("ui.state.loadingPage")} />}
      {instruments.isError ? (
        <ErrorState title={t("portfolio.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void instruments.refetch()} retryLabel={t("common.retryAction")} />
      ) : (
        <dl className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-3">
          {attributes.map(([label, value]) => <div key={label}><dt className="text-muted-foreground">{label}</dt><dd className="break-words">{value}</dd></div>)}
        </dl>
      )}
      {instrument?.note && <details className="text-sm text-muted-foreground">
        <summary className="cursor-pointer rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">{t("accounts.note")}</summary>
        <p className="mt-2 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{instrument.note}</p>
      </details>}
    </div>
  );
}
