import type { AnalysisQueryRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis/models";
import type { AnalysisSessionState } from "@/stores/analysis";
import { lastClosedDate } from "@/features/insights/calendar";

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

export function effectiveRange(state: Pick<AnalysisSessionState, "from" | "to">, month: string, timeZone?: string): { from: string; to: string } {
  const [year, value] = month.split("-").map(Number);
  const monthStartValue = `${month}-01`;
  const end = new Date(year, value, 0);
  const monthEndValue = `${year}-${String(value).padStart(2, "0")}-${String(end.getDate()).padStart(2, "0")}`;
  return {
    from: state.from || monthStartValue,
    to: clampToClosedDate(state.to || monthEndValue, timeZone),
  };
}

export function yearRange(year: number, timeZone?: string): { from: string; to: string } {
  return periodRange({ from: "", to: "" }, `${year}-01-01`, `${year}-12-31`, timeZone);
}

/**
 * Intersects a display period with the session's explicit date filter. The
 * visible-month request intentionally keeps the full query range so the 2b
 * cursor contract can preserve its period summary; year cards, however, are
 * independent periods and must use this intersection.
 */
export function periodRange(state: Pick<AnalysisSessionState, "from" | "to">, from: string, to: string, timeZone?: string): { from: string; to: string } {
  const periodFrom = state.from && state.from > from ? state.from : from;
  const periodTo = state.to && state.to < to ? state.to : to;
  return { from: periodFrom, to: clampToClosedDate(periodTo, timeZone) };
}

function clampToClosedDate(to: string, timeZone?: string): string {
  const closed = lastClosedDate(timeZone);
  return to > closed ? closed : to;
}
