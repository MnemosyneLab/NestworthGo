import { expect, it } from "vitest";
import type { TFunction } from "i18next";
import { quoteTrendProps, type QuoteTrendPoint } from "./quoteTrend";
import { trendQualitySegments } from "@/components/charts/TrendChart";

const t = ((key: string) => key) as unknown as TFunction;
const point = (overrides: Partial<QuoteTrendPoint>): QuoteTrendPoint => ({
  quotedAt: "2026-10-14T15:30:00Z", effectiveDate: "2026-10-14", timestampBasis: "instant", value: "1", currency: "USD",
  sourceKind: "provider", sourceKey: "", priceBasis: "unit_price", observationKind: "close", delayed: false, ...overrides,
}) as QuoteTrendPoint;
const base = { kind: "instrument" as const, currency: "USD", color: "blue", timezone: "UTC", language: "en", t };
const meta = (points: QuoteTrendPoint[], daily = false) => quoteTrendProps({ ...base, daily, series: { points, observations: points } }).series[0].pointMeta ?? [];

it("plots daily series on their civil effective date and raw history on the quote instant", () => {
  const points = [point({})];
  expect(quoteTrendProps({ ...base, daily: true, series: { points, observations: points } }).dates).toEqual(["2026-10-14"]);
  expect(quoteTrendProps({ ...base, series: { points, observations: points } }).dates).toEqual(["2026-10-14T15:30:00Z"]);
});

it("prefers the backend discontinuity flag and falls back to a price-basis change", () => {
  const points = [point({}), point({ breakBefore: true }), point({ priceBasis: "unit_nav" })];
  expect(meta(points, true).map((entry) => entry?.breakBefore)).toEqual([false, true, true]);
  const explicit = [point({}), point({ priceBasis: "unit_nav", breakBefore: false })];
  expect(meta(explicit, true).map((entry) => entry?.breakBefore)).toEqual([false, false]);
});

it("connects Yahoo and Agent closes while keeping interleaved intraday quotes as independent dots", () => {
  const points = [
    point({ quotedAt: "2026-10-12T09:00:00Z", value: "5.50699997", priceBasis: "yahoo_close_v1" }),
    point({ quotedAt: "2026-10-13T09:00:00Z", value: "5.5170002", priceBasis: "yahoo_close_v1" }),
    point({ quotedAt: "2026-10-14T03:09:00Z", value: "5.517", observationKind: "realtime", priceBasis: "unit_price" }),
    point({ quotedAt: "2026-10-14T09:00:00Z", value: "5.49700022", priceBasis: "yahoo_close_v1" }),
    point({ quotedAt: "2026-10-15T16:00:00Z", effectiveDate: "2026-10-15", timestampBasis: "date_label", value: "5.461", sourceKind: "agent", priceBasis: "agent_raw_close_v1" }),
    point({ quotedAt: "2026-10-16T06:05:00Z", value: "5.511", observationKind: "realtime", priceBasis: "unit_price" }),
    point({ quotedAt: "2026-10-16T09:00:00Z", value: "5.50199986", priceBasis: "yahoo_close_v1" }),
  ];
  const observations = [...points].reverse();
  const props = quoteTrendProps({ ...base, colors: ["red", "blue", "green"], series: { points, observations } });
  expect(props.series).toHaveLength(2);
  expect(props.series.map(series => trendQualitySegments(series).map(segment => segment.indices))).toEqual([[[0, 1, 3, 4, 6]], [[2, 5]]]);
  expect(props.series.map(series => series.presentation)).toEqual(["line", "points"]);
  expect(props.series.map(series => series.color)).toEqual(["red", "blue"]);
  expect(props.extraTableRows).toHaveLength(observations.length);
  expect(props.extraTableRows?.[0][1]).toContain("5.50199986");
  expect(points[0].value).toBe("5.50699997");
});

it("sorts projected date labels chronologically and preserves explicit gaps and currency boundaries", () => {
  const points = [
    point({ quotedAt: "2026-10-15T01:00:00Z", effectiveDate: "2026-10-15" }),
    point({ quotedAt: "2026-10-15T16:00:00Z", effectiveDate: "2026-10-15", timestampBasis: "date_label" }),
    point({ quotedAt: "2026-10-16T01:00:00Z", breakBefore: true }),
    point({ quotedAt: "2026-10-17T01:00:00Z" }),
    point({ quotedAt: "2026-10-18T01:00:00Z", currency: "SGD" }),
  ];
  const props = quoteTrendProps({ ...base, series: { points, observations: points } });
  expect(props.dates.slice(0, 2)).toEqual(["2026-10-15", "2026-10-15T01:00:00Z"]);
  expect(props.series).toHaveLength(2);
  expect(trendQualitySegments(props.series[0]).map(segment => segment.indices)).toEqual([[0, 1], [2, 3]]);
  expect(props.series[1].values[4]).toBeNull();
});

it("renders latest FX quotes as dots independently of daily reference rates", () => {
  const points = [point({ observationKind: "daily_reference" }), point({ observationKind: "latest" })];
  const props = quoteTrendProps({ ...base, kind: "fx", currency: undefined, series: { points, observations: points } });
  expect(props.series.map(series => series.presentation)).toEqual(["line", "points"]);
});

it("distinguishes NAV from a daily close while keeping the line label independent of its first source", () => {
  const points = [point({ sourceKind: "manual" }), point({ priceBasis: "agent_unit_nav_v1" }), point({})];
  const props = quoteTrendProps({ ...base, series: { points, observations: points } });
  expect(props.series.map(series => series.name)).toEqual(["quoteDetails.kinds.close", "quoteDetails.kinds.nav"]);
  expect(props.series.map(series => series.pointIndices)).toEqual([[0, 2], [1]]);
});

it.each([
  ["en", "Oct 15, 2026", /5:00\s*PM/],
  ["zh-CN", "2026年10月15日", /17:00/],
  ["zh-TW", "2026年10月15日", /下午5:00/],
])("localizes both date columns in %s while preserving observation precision", (language, date, time) => {
  const points = [
    point({ quotedAt: "2026-10-15T09:00:00Z", effectiveDate: "2026-10-15", timestampBasis: "session_close", sourceKey: "yahoo_finance" }),
    point({ quotedAt: "2026-10-14T16:00:00Z", effectiveDate: "2026-10-15", timestampBasis: "date_label", sourceKind: "agent" }),
    point({ quotedAt: "2026-10-15T06:05:00Z", effectiveDate: "", observationKind: "realtime" }),
  ];
  for (const kind of ["instrument", "fx"] as const) {
    const props = quoteTrendProps({ ...base, kind, language, timezone: "Asia/Singapore", series: { points, observations: points } });
    const rows = props.extraTableRows!;
    expect(rows[0][0]).toContain(date);
    expect(rows[0][0]).toMatch(time);
    expect(rows[0][3]).toBe(date);
    expect(rows[1][0]).toBe(date);
    expect(rows[1][3]).toBe(date);
    expect(rows[2][3]).toBe("—");
  }
  expect(points[0].effectiveDate).toBe("2026-10-15");
});
