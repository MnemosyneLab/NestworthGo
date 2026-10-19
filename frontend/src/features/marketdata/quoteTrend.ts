import type { TFunction } from "i18next";
import type { TrendChartProps, TrendSeries } from "@/components/charts/TrendChart";
import { quoteObservationLabel, quoteSourceLabel } from "@/lib/quoteEvidencePresentation";
import { formatAmount, formatQuotePrice } from "@/lib/money";
import { formatCivilDate, formatQuoteAsOf } from "@/lib/time";
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
  colors?: string[];
  timezone: string | undefined;
  language: string;
  t: TFunction;
}): Pick<TrendChartProps, "dates" | "series" | "currency" | "valueFormatter" | "extraTableColumns" | "extraTableRows"> {
  const { series, kind, daily = false, currency, priceSuffix = "", color, timezone, language, t } = input;
  const coordinate = (point: QuoteTrendPoint) => (daily || point.timestampBasis === "date_label") && point.effectiveDate ? point.effectiveDate : point.quotedAt;
  // Date labels and instants can sort differently after projection onto the axis.
  const points = daily ? series?.points ?? [] : [...(series?.points ?? [])].sort((a, b) => Date.parse(coordinate(a)) - Date.parse(coordinate(b)));
  const observations = series?.observations ?? [];
  const label = kind === "fx" ? t("charts.rate") : t("charts.price");
  const sourceLabel = (sourceKind: string, key?: string) => quoteSourceLabel(t, sourceKind, key);
  const observationLabel = (point: Parameters<typeof quoteObservationLabel>[1]) => quoteObservationLabel(t, point);
  const values = points.map((point) => currency && point.currency && point.currency !== currency ? null : point.value);
  const pointMeta = points.map((point, index) => ({
    breakBefore: point.breakBefore ?? (daily && index > 0 && points[index - 1].priceBasis !== point.priceBasis),
    sourceLabel: sourceLabel(point.sourceKind, point.sourceKey),
    delayed: point.delayed,
    observationLabel: observationLabel(point),
    observationKind: point.sourceKind === "manual" ? "manual" : point.observationKind,
    effectiveDate: point.effectiveDate,
  }));
  const chartSeries: TrendSeries[] = [];
  if (daily) {
    chartSeries.push({ key: "value", name: label, color, values, pointMeta });
  } else {
    // Sources describe provenance, not separate trends. Connect the same price
    // type across sources; intraday/latest facts remain independent dots.
    const groups = new Map<string, number[]>();
    const priceType = (point: QuoteTrendPoint) => point.priceBasis === "agent_unit_nav_v1" ? "nav" : point.observationKind || "unknown";
    points.forEach((point, index) => {
      const key = JSON.stringify([point.currency, priceType(point)]);
      const indices = groups.get(key) ?? [];
      indices.push(index);
      groups.set(key, indices);
    });
    const names = new Map<string, number>();
    for (const [key, indices] of groups) {
      const included = new Set(indices);
      const first = points[indices[0]];
      const name = t(`quoteDetails.kinds.${priceType(first)}`, { defaultValue: t("quoteDetails.kinds.unknown") });
      const ordinal = (names.get(name) ?? 0) + 1;
      names.set(name, ordinal);
      chartSeries.push({
        key, name: ordinal === 1 ? name : `${name} (${ordinal})`,
        color: input.colors?.[chartSeries.length % input.colors.length] ?? color, values,
        pointMeta: pointMeta.map((meta, index) => included.has(index) ? meta : undefined),
        pointIndices: indices,
        presentation: first.observationKind === "realtime" || first.observationKind === "latest" ? "points" : "line",
      });
    }
  }
  return {
    dates: points.map(coordinate),
    series: chartSeries,
    currency: currency ?? "",
    valueFormatter: (value) => (value ? (currency ? formatQuotePrice(value, currency) + priceSuffix : formatAmount(value)) : t("accounts.noValue")),
    extraTableColumns: [t("charts.quotedAt"), label, t("quoteDetails.type"), t("quoteDetails.effectiveDate"), t("charts.source"), t("charts.delayed")],
    extraTableRows: observations.map((item) => [
      formatQuoteAsOf(item, timezone, language),
      currency ? formatQuotePrice(item.value, item.currency || currency) + priceSuffix : formatAmount(item.value),
      observationLabel(item),
      item.effectiveDate ? formatCivilDate(item.effectiveDate, language) : "—",
      sourceLabel(item.sourceKind, item.sourceKey),
      item.delayed ? t("charts.delayed") : t("charts.onTime"),
    ]),
  };
}
