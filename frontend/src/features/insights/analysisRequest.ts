import type { AnalysisQueryRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis/models";
import type { AnalysisSessionState } from "@/stores/analysis";
import { lastClosedDate, parseYmd, ymd, ymdInTimeZone } from "@/features/insights/calendar";

export type ReturnTrendRange = "30d" | "ytd" | "1y" | "3y" | "all" | "custom";

export function analysisRequest(state: AnalysisSessionState, from: string, to: string): AnalysisQueryRequest {
  const scopeKind = state.scope === "account" ? "account" : state.scope === "instrument" ? "instrument" : "household";
  const request: AnalysisQueryRequest = {
    scopeKind,
    from,
    to,
    valuation: state.valuation,
    basis: "investment",
    includeCash: state.includeCash,
  };
  if (state.scopeId && scopeKind !== "household") {
    request.scopeId = state.scopeId;
  }
  const filters = state.moreFilters;
  if (typeof filters.accountId === "string" && filters.accountId) request.accountId = filters.accountId;
  if (typeof filters.currency === "string" && filters.currency) request.currency = filters.currency;
  if (typeof filters.assetClass === "string" && filters.assetClass) request.assetClass = filters.assetClass;
  if (typeof filters.instrumentId === "string" && filters.instrumentId) request.instrumentId = filters.instrumentId;
  if (typeof filters.memberId === "string" && filters.memberId) request.memberId = filters.memberId;
  return request;
}

export function originLocalDate(startedAt: string, timeZone?: string): string {
  const originInstant = new Date(startedAt);
  return Number.isNaN(originInstant.getTime()) ? startedAt.slice(0, 10) : ymdInTimeZone(originInstant, timeZone);
}

export function effectiveRange(state: Pick<AnalysisSessionState, "from" | "to">, month: string, timeZone?: string, startedAt?: string): { from: string; to: string } {
  const [year, value] = month.split("-").map(Number);
  const monthStartValue = `${month}-01`;
  const end = new Date(year, value, 0);
  const monthEndValue = `${year}-${String(value).padStart(2, "0")}-${String(end.getDate()).padStart(2, "0")}`;
  return clampAnalyzableRange({
    from: state.from || monthStartValue,
    to: state.to || monthEndValue,
  }, startedAt, timeZone);
}

export function yearRange(year: number, timeZone?: string, startedAt?: string): { from: string; to: string } {
  return periodRange({ from: "", to: "" }, `${year}-01-01`, `${year}-12-31`, timeZone, startedAt);
}

function addDays(value: string, amount: number): string {
  const date = parseYmd(value);
  date.setDate(date.getDate() + amount);
  return ymd(date);
}

function maxDate(left: string, right: string): string {
  return left > right ? left : right;
}

/** Named ranges for Return Trend. The end is always the latest closed
 * Origin-local day, and the start is clamped to History Origin so a named
 * range never asks the engine for dates before the available history. */
export function returnTrendRange(range: Exclude<ReturnTrendRange, "custom">, startedAt: string, timeZone?: string): { from: string; to: string } {
  const to = lastClosedDate(timeZone);
  const originDate = originLocalDate(startedAt, timeZone);
  const start = range === "all"
    ? originDate
    : range === "ytd"
      ? `${to.slice(0, 4)}-01-01`
      : addDays(to, range === "30d" ? -29 : range === "1y" ? -364 : -1094);
  return { from: maxDate(start, originDate), to };
}

export function activeReturnTrendRange(state: Pick<AnalysisSessionState, "from" | "to">, startedAt: string, timeZone?: string): ReturnTrendRange | null {
  if (!state.from || !state.to) return null;
  for (const range of ["30d", "ytd", "1y", "3y", "all"] as const) {
    const named = returnTrendRange(range, startedAt, timeZone);
    if (named.from === state.from && named.to === state.to) return range;
  }
  return "custom";
}

/**
 * Intersects a display period with the session's explicit date filter. The
 * visible-month request intentionally keeps the full query range so the 2b
 * cursor contract can preserve its period summary; year cards, however, are
 * independent periods and must use this intersection.
 */
export function periodRange(state: Pick<AnalysisSessionState, "from" | "to">, from: string, to: string, timeZone?: string, startedAt?: string): { from: string; to: string } {
  const periodFrom = state.from && state.from > from ? state.from : from;
  const periodTo = state.to && state.to < to ? state.to : to;
  return clampAnalyzableRange({ from: periodFrom, to: periodTo }, startedAt, timeZone);
}

function clampAnalyzableRange(range: { from: string; to: string }, startedAt?: string, timeZone?: string): { from: string; to: string } {
  const originDate = startedAt ? originLocalDate(startedAt, timeZone) : "";
  return {
    from: originDate && range.from < originDate ? originDate : range.from,
    to: clampToClosedDate(range.to, timeZone),
  };
}

function clampToClosedDate(to: string, timeZone?: string): string {
  const closed = lastClosedDate(timeZone);
  return to > closed ? closed : to;
}
