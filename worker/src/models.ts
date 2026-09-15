export interface WorkerEnv {
  API_TOKEN?: string;
}

export interface SearchItem {
  symbol: string;
  name: string;
  exchange: string | null;
  type: string | null;
}

export interface SearchResponse {
  items: SearchItem[];
}

export interface QuoteResponse {
  symbol: string;
  name: string | null;
  currency: string | null;
  exchange: string | null;
  price: number;
  asOf: string | null;
  marketState: string | null;
  source: "yahoo";
}

export interface HistoryPrice {
  date: string;
  close: number;
}

export interface HistoryResponse {
  symbol: string;
  currency: string | null;
  interval: "1d";
  prices: HistoryPrice[];
}

export type ErrorCode =
  | "AUTH_NOT_CONFIGURED"
  | "UNAUTHORIZED"
  | "NOT_FOUND"
  | "QUERY_REQUIRED"
  | "INVALID_QUERY"
  | "INVALID_SYMBOL"
  | "INVALID_DATE"
  | "INVALID_DATE_RANGE"
  | "UPSTREAM_ERROR"
  | "INTERNAL_ERROR";

export interface ErrorResponse {
  error: {
    code: ErrorCode;
    message: string;
  };
}

export class ApiError extends Error {
  constructor(
    public readonly status: 400 | 401 | 404 | 500 | 502 | 503,
    public readonly code: ErrorCode,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export class UpstreamError extends Error {
  constructor() {
    super("Yahoo Finance request failed");
    this.name = "UpstreamError";
  }
}
