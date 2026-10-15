import { expect, it } from "vitest";
import type { TFunction } from "i18next";
import { quoteTrendProps, type QuoteTrendPoint } from "./quoteTrend";

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
