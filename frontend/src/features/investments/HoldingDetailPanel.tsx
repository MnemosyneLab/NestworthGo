import { useId } from "react";
import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Button } from "@/components/ui/button";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { useInstruments } from "@/queries/investments";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { displayEnum } from "@/lib/display";
import { PricePerformancePanel } from "./PricePerformancePanel";

type Props = {
  instrumentId: string;
  /** Account name or the all-accounts label; names the region for assistive technology. */
  scope: string;
};

/**
 * Shared read-only body for an expanded holding row. The row above already
 * shows quantity, cost, value and gain, so this panel adds only price
 * performance and instrument facts. Hosts mount it for expanded, active rows.
 */
export function HoldingDetailPanel({ instrumentId, scope }: Props) {
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

  return (
    <div role="region" aria-labelledby={headingId} className="flex min-w-0 flex-col gap-4 rounded-xl bg-muted/20 p-3 sm:p-4">
      <h3 id={headingId} className="sr-only">{instrument ? instrumentDisplayLabel(instrument) : t("portfolio.unknownInstrument")} · {scope}</h3>
      {instrument?.managedProductId && navigation && <Button type="button" variant="outline" className="self-start" onClick={() => navigation.open({ page: "available-funds", productId: instrument.managedProductId })}>{t("availableFunds.openPage")}</Button>}
      {instrument && !instrument.managedProductId && <PricePerformancePanel instrument={instrument} />}
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
