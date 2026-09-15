import { Hono } from "hono";

import { isAuthorized } from "./auth";
import {
  ApiError,
  ErrorCode,
  ErrorResponse,
  HistoryResponse,
  QuoteResponse,
  SearchResponse,
  UpstreamError,
  WorkerEnv,
} from "./models";
import {
  historySymbol,
  normalizeSymbol,
  quoteSymbol,
  searchSymbols,
  validateSearchQuery,
  YahooClient,
  yahoo,
} from "./yahoo";

const SEARCH_LIMIT_DEFAULT = 10;
const SEARCH_LIMIT_MAX = 20;
const CACHE_TTL = {
  search: 86_400,
  quote: 1_800,
  history: 2_592_000,
} as const;

interface AppOptions {
  yahooClient?: YahooClient;
}

function jsonError(code: ErrorCode, message: string, status: 400 | 401 | 404 | 500 | 502 | 503): Response {
  const body: ErrorResponse = { error: { code, message } };
  return Response.json(body, { status, headers: { "Cache-Control": "no-store" } });
}

function logUpstreamFailure(operation: string, symbol?: string): void {
  console.error("Yahoo Finance request failed", { operation, symbol });
}

function safeErrorResponse(error: unknown, operation: string, symbol?: string): Response {
  if (error instanceof ApiError) {
    return jsonError(error.code, error.message, error.status);
  }
  if (error instanceof UpstreamError || error instanceof Error) {
    logUpstreamFailure(operation, symbol);
    return jsonError("UPSTREAM_ERROR", "Unable to fetch Yahoo Finance data", 502);
  }
  logUpstreamFailure(operation, symbol);
  return jsonError("INTERNAL_ERROR", "An internal error occurred", 500);
}

function cacheKey(request: Request, suffix: string): Request {
  const url = new URL(request.url);
  url.pathname = `/__nestworth_cache/${suffix}`;
  url.search = "";
  return new Request(url, { method: "GET" });
}

function withCacheHeader(response: Response, value: "HIT" | "MISS"): Response {
  const headers = new Headers(response.headers);
  headers.set("X-Cache", value);
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  });
}

async function cachedResponse(
  request: Request,
  suffix: string,
  ttlSeconds: number,
  loader: () => Promise<Response>,
): Promise<Response> {
  const cache = typeof caches === "undefined" ? undefined : (caches as unknown as { default?: Cache }).default;
  if (!cache) return loader();

  const key = cacheKey(request, suffix);
  const cached = await cache.match(key);
  if (cached) return withCacheHeader(cached, "HIT");

  const response = await loader();
  if (!response.ok) return response;

  const cacheable = withCacheHeader(response, "MISS");
  const headers = new Headers(cacheable.headers);
  headers.set("Cache-Control", `public, max-age=0, s-maxage=${ttlSeconds}`);
  const cacheableWithTTL = new Response(cacheable.body, {
    status: cacheable.status,
    statusText: cacheable.statusText,
    headers,
  });
  try {
    await cache.put(key, cacheableWithTTL.clone());
  } catch (error) {
    console.error("Market data cache write failed", error instanceof Error ? error.name : "unknown");
  }
  return cacheableWithTTL;
}

function parseSearchLimit(value: string | undefined): number {
  if (!value) return SEARCH_LIMIT_DEFAULT;
  const limit = Number(value);
  if (!Number.isInteger(limit) || limit < 1 || limit > SEARCH_LIMIT_MAX) {
    throw new ApiError(400, "INVALID_QUERY", `limit must be an integer from 1 to ${SEARCH_LIMIT_MAX}`);
  }
  return limit;
}

export function createApp(options: AppOptions = {}): Hono<{ Bindings: WorkerEnv }> {
  const yahooClient = options.yahooClient ?? yahoo;
  const app = new Hono<{ Bindings: WorkerEnv }>();

  app.use("/v1/*", async (c, next) => {
    if (c.env.API_TOKEN === undefined || c.env.API_TOKEN.trim() === "") {
      return jsonError("AUTH_NOT_CONFIGURED", "API token is not configured", 503);
    }
    if (!(await isAuthorized(c.req.raw, c.env.API_TOKEN))) {
      return jsonError("UNAUTHORIZED", "A valid Bearer token is required", 401);
    }
    await next();
  });

  app.get("/", (c) => c.json({ service: "nestworth-market", status: "ok" }));

  app.get("/v1/search", async (c) => {
    let query: string;
    let limit: number;
    try {
      query = validateSearchQuery(c.req.query("q"));
      limit = parseSearchLimit(c.req.query("limit"));
    } catch (error) {
      return safeErrorResponse(error, "search");
    }

    const key = `search/${encodeURIComponent(query.toLowerCase())}/${limit}`;
    return cachedResponse(c.req.raw, key, CACHE_TTL.search, async () => {
      try {
        const response: SearchResponse = { items: await searchSymbols(yahooClient, query, limit) };
        return Response.json(response, { headers: { "Cache-Control": `public, max-age=0, s-maxage=${CACHE_TTL.search}` } });
      } catch (error) {
        return safeErrorResponse(error, "search");
      }
    });
  });

  app.get("/v1/quote/:symbol", async (c) => {
    let symbol: string;
    try {
      symbol = normalizeSymbol(c.req.param("symbol"));
    } catch (error) {
      return safeErrorResponse(error, "quote", c.req.param("symbol"));
    }

    const key = `quote/${encodeURIComponent(symbol)}`;
    return cachedResponse(c.req.raw, key, CACHE_TTL.quote, async () => {
      try {
        const response: QuoteResponse = await quoteSymbol(yahooClient, symbol);
        return Response.json(response, { headers: { "Cache-Control": `public, max-age=0, s-maxage=${CACHE_TTL.quote}` } });
      } catch (error) {
        return safeErrorResponse(error, "quote", symbol);
      }
    });
  });

  app.get("/v1/history/:symbol", async (c) => {
    let symbol: string;
    try {
      symbol = normalizeSymbol(c.req.param("symbol"));
    } catch (error) {
      return safeErrorResponse(error, "history", c.req.param("symbol"));
    }

    const from = c.req.query("from");
    const to = c.req.query("to");
    const key = `history/${encodeURIComponent(symbol)}/${from ?? ""}/${to ?? ""}`;
    return cachedResponse(c.req.raw, key, CACHE_TTL.history, async () => {
      try {
        const response: HistoryResponse = await historySymbol(yahooClient, symbol, from, to);
        return Response.json(response, { headers: { "Cache-Control": `public, max-age=0, s-maxage=${CACHE_TTL.history}` } });
      } catch (error) {
        return safeErrorResponse(error, "history", symbol);
      }
    });
  });

  app.notFound(() => jsonError("NOT_FOUND", "Route not found", 404));
  app.onError((error) => safeErrorResponse(error, "request"));
  return app;
}

const app = createApp();

export default app;
