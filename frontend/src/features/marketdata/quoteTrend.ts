import type { TFunction } from "i18next";
import type { TrendChartProps } from "@/components/charts/TrendChart";
import { quoteObservationLabel, quoteSourceLabel } from "@/lib/quoteEvidencePresentation";
import { formatAmount, formatQuotePrice } from "@/lib/money";
import { formatQuoteAsOf } from "@/lib/time";
import type { QuoteSeriesPointDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote/models";

/** A stored observation; the holding performance read model adds an explicit discontinuity flag. */
export type QuoteTrendPoint = QuoteSeriesPointDTO & { breakBefore?: boolean };
export type QuoteTrendSeries = { points: QuoteTrendPoint[] | null; observations: QuoteTrendPoint[] | null };

/**
 * Maps one local quote series to TrendChart props. Both the market-data sheet
 * and the holding performance panel use it so evidence labels, the data table
 * and currency handling cannot drift apart.
 */
export function quoteTrendProps(input: {
  series: QuoteTrendSeries | undefined;
  kind: "instrument" | "fx";
  /** Daily-only series: plot each point on its civil effective date, the date the figures use. */
  daily?: boolean;
  /** Instrument quote currency; undefined for FX rates. */
  currency?: string;
  priceSuffix?: string;
  color: string;
  timezone: string | undefined;
  language: string;
  t: TFunction;
}): Pick<TrendChartProps, "dates" | "series" | "currency" | "valueFormatter" | "extraTableColumns" | "extraTableRows"> {
  const { series, kind, daily = false, currency, priceSuffix = "", color, timezone, language, t } = input;
  const points = series?.points ?? [];
  const observations = series?.observations ?? [];
  const label = kind === "fx" ? t("charts.rate") : t("charts.price");
  const sourceLabel = (sourceKind: string, key?: string) => quoteSourceLabel(t, sourceKind, key);
  const observationLabel = (point: Parameters<typeof quoteObservationLabel>[1]) => quoteObservationLabel(t, point);
  return {
    dates: points.map((point) => (daily || point.timestampBasis === "date_label") && point.effectiveDate ? point.effectiveDate : point.quotedAt),
    series: [{
      key: "value",
      name: label,
      color,
      values: points.map((point) => currency && point.currency && point.currency !== currency ? null : point.value),
      pointMeta: points.map((point, index) => ({
        breakBefore: point.breakBefore ?? (index > 0 && points[index - 1].priceBasis !== point.priceBasis),
        sourceLabel: sourceLabel(point.sourceKind, point.sourceKey),
        delayed: point.delayed,
        observationLabel: observationLabel(point),
        observationKind: point.sourceKind === "manual" ? "manual" : point.observationKind,
        effectiveDate: point.effectiveDate,
      })),
    }],
    currency: currency ?? "",
    valueFormatter: (value) => (value ? (currency ? formatQuotePrice(value, currency) + priceSuffix : formatAmount(value)) : t("accounts.noValue")),
    extraTableColumns: [t("charts.quotedAt"), label, t("quoteDetails.type"), t("quoteDetails.effectiveDate"), t("charts.source"), t("charts.delayed")],
    extraTableRows: observations.map((item) => [
      formatQuoteAsOf(item, timezone, language),
      currency ? formatQuotePrice(item.value, item.currency || currency) + priceSuffix : formatAmount(item.value),
      observationLabel(item),
      item.effectiveDate || "—",
      sourceLabel(item.sourceKind, item.sourceKey),
      item.delayed ? t("charts.delayed") : t("charts.onTime"),
    ]),
  };
}
