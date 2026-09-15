import YahooFinance from "yahoo-finance2";

import {
  ApiError,
  HistoryPrice,
  HistoryResponse,
  QuoteResponse,
  SearchItem,
} from "./models";

export interface YahooClient {
  search(query: string): Promise<unknown>;
  quote(symbol: string): Promise<unknown>;
  chart(
    symbol: string,
    options: { interval: "1d"; period1: Date; period2: Date },
  ): Promise<unknown>;
}

export const yahoo: YahooClient = new YahooFinance({
  suppressNotices: ["yahooSurvey"],
}) as unknown as YahooClient;

const SYMBOL_PATTERN = /^[A-Za-z0-9^][A-Za-z0-9._^=-]{0,31}$/;
const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;
const MAX_SEARCH_LENGTH = 80;
const MAX_HISTORY_DAYS = 366 * 20;

type RecordValue = Record<string, unknown>;

function isRecord(value: unknown): value is RecordValue {
  return typeof value === "object" && value !== null;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function dateValue(value: unknown): Date | undefined {
  const date = value instanceof Date ? value : new Date(typeof value === "number" ? value * 1000 : String(value));
  return Number.isNaN(date.getTime()) ? undefined : date;
}

function isoDateTime(value: unknown): string | null {
  const date = dateValue(value);
  return date ? date.toISOString() : null;
}

function marketDate(value: unknown): string | undefined {
  const date = dateValue(value);
  return date ? date.toISOString().slice(0, 10) : undefined;
}

function requiredSymbol(symbol: string): string {
  const normalized = symbol.trim().toUpperCase();
  if (!SYMBOL_PATTERN.test(normalized)) {
    throw new ApiError(400, "INVALID_SYMBOL", "symbol is invalid");
  }
  return normalized;
}

export function normalizeSymbol(symbol: string): string {
  return requiredSymbol(symbol);
}

export function validateSearchQuery(query: string | undefined): string {
  const normalized = query?.trim();
  if (!normalized) {
    throw new ApiError(400, "QUERY_REQUIRED", "query parameter q is required");
  }
  if (normalized.length > MAX_SEARCH_LENGTH) {
    throw new ApiError(400, "INVALID_QUERY", "query is too long");
  }
  return normalized;
}

export function parseHistoryDate(value: string | undefined, field: "from" | "to"): Date {
  if (!value || !DATE_PATTERN.test(value)) {
    throw new ApiError(400, "INVALID_DATE", `${field} must use YYYY-MM-DD`);
  }

  const [, yearText, monthText, dayText] = DATE_PATTERN.exec(value)!;
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    throw new ApiError(400, "INVALID_DATE", `${field} is not a calendar date`);
  }
  return date;
}

export function parseHistoryRange(fromValue: string | undefined, toValue: string | undefined): { from: Date; to: Date } {
  const from = parseHistoryDate(fromValue, "from");
  const to = parseHistoryDate(toValue, "to");
  const days = (to.getTime() - from.getTime()) / 86_400_000;
  if (days < 0) {
    throw new ApiError(400, "INVALID_DATE_RANGE", "from must not be after to");
  }
  if (days > MAX_HISTORY_DAYS) {
    throw new ApiError(400, "INVALID_DATE_RANGE", "history range is too large");
  }
  return { from, to };
}

function searchQuotes(result: unknown): unknown[] {
  if (!isRecord(result) || !Array.isArray(result.quotes)) return [];
  return result.quotes;
}

export function mapSearch(result: unknown, limit: number): SearchItem[] {
  const items: SearchItem[] = [];
  const seen = new Set<string>();

  for (const value of searchQuotes(result)) {
    if (!isRecord(value)) continue;
    const symbol = stringValue(value.symbol)?.toUpperCase();
    if (!symbol || seen.has(symbol)) continue;
    seen.add(symbol);
    items.push({
      symbol,
      name: stringValue(value.longname) ?? stringValue(value.shortname) ?? symbol,
      exchange: stringValue(value.exchDisp) ?? stringValue(value.fullExchangeName) ?? stringValue(value.exchange) ?? null,
      type: stringValue(value.quoteType)?.toUpperCase() ?? null,
    });
    if (items.length >= limit) break;
  }

  return items;
}

function quoteValue(result: unknown): RecordValue {
  if (!isRecord(result)) throw new Error("quote payload is not an object");
  return result;
}

function selectedPrice(quote: RecordValue): { price: number; asOf: string | null } {
  const state = stringValue(quote.marketState)?.toUpperCase();
  const regular = numberValue(quote.regularMarketPrice);
  const pre = numberValue(quote.preMarketPrice);
  const post = numberValue(quote.postMarketPrice);

  if (state === "PRE" && pre !== undefined) {
    return { price: pre, asOf: isoDateTime(quote.preMarketTime ?? quote.regularMarketTime) };
  }
  if (state === "POST" && post !== undefined) {
    return { price: post, asOf: isoDateTime(quote.postMarketTime ?? quote.regularMarketTime) };
  }
  if (regular !== undefined) {
    return { price: regular, asOf: isoDateTime(quote.regularMarketTime) };
  }
  if (post !== undefined) {
    return { price: post, asOf: isoDateTime(quote.postMarketTime) };
  }
  if (pre !== undefined) {
    return { price: pre, asOf: isoDateTime(quote.preMarketTime) };
  }

  throw new Error("quote payload has no usable price");
}

export function mapQuote(result: unknown, requestedSymbol: string): QuoteResponse {
  const quote = quoteValue(result);
  const selected = selectedPrice(quote);
  return {
    symbol: stringValue(quote.symbol)?.toUpperCase() ?? requestedSymbol,
    name: stringValue(quote.longName) ?? stringValue(quote.shortName) ?? null,
    currency: stringValue(quote.currency)?.toUpperCase() ?? null,
    exchange: stringValue(quote.fullExchangeName) ?? stringValue(quote.exchange) ?? null,
    price: selected.price,
    asOf: selected.asOf,
    marketState: stringValue(quote.marketState)?.toUpperCase() ?? null,
    source: "yahoo",
  };
}

function chartPayload(result: unknown): { meta: RecordValue; quotes: unknown[] } {
  if (!isRecord(result) || !Array.isArray(result.quotes)) {
    throw new Error("chart payload is malformed");
  }
  return {
    meta: isRecord(result.meta) ? result.meta : {},
    quotes: result.quotes,
  };
}

export function mapHistory(result: unknown, requestedSymbol: string): HistoryResponse {
  const chart = chartPayload(result);
  const prices: HistoryPrice[] = [];
  const seen = new Set<string>();

  for (const value of chart.quotes) {
    if (!isRecord(value)) continue;
    const date = marketDate(value.date);
    const close = numberValue(value.close);
    if (!date || close === undefined || seen.has(date)) continue;
    seen.add(date);
    prices.push({ date, close });
  }

  prices.sort((left, right) => left.date.localeCompare(right.date));
  return {
    symbol: stringValue(chart.meta.symbol)?.toUpperCase() ?? requestedSymbol,
    currency: stringValue(chart.meta.currency)?.toUpperCase() ?? null,
    interval: "1d",
    prices,
  };
}

export async function searchSymbols(client: YahooClient, query: string, limit: number): Promise<SearchItem[]> {
  return mapSearch(await client.search(query), limit);
}

export async function quoteSymbol(client: YahooClient, symbol: string): Promise<QuoteResponse> {
  const normalized = requiredSymbol(symbol);
  return mapQuote(await client.quote(normalized), normalized);
}

export async function historySymbol(
  client: YahooClient,
  symbol: string,
  fromValue: string | undefined,
  toValue: string | undefined,
): Promise<HistoryResponse> {
  const normalized = requiredSymbol(symbol);
  const { from, to } = parseHistoryRange(fromValue, toValue);
  const inclusiveEnd = new Date(to.getTime() + 86_400_000);
  return mapHistory(
    await client.chart(normalized, { interval: "1d", period1: from, period2: inclusiveEnd }),
    normalized,
  );
}
