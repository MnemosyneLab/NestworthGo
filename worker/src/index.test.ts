import { describe, expect, it } from "vitest";

import { createApp } from "./index";
import { YahooClient } from "./yahoo";

const env = { API_TOKEN: "test-token" };

function request(path: string, authorization = "Bearer test-token"): Request {
  return new Request(`https://worker.test${path}`, {
    headers: authorization ? { Authorization: authorization } : undefined,
  });
}

async function json(response: Response): Promise<Record<string, unknown>> {
  return (await response.json()) as Record<string, unknown>;
}

class FakeYahoo implements YahooClient {
  readonly calls: Array<{ operation: string; symbol?: string; options?: unknown }> = [];
  searchResult: unknown = {
    quotes: [
      { symbol: "D05.SI", longname: "DBS Group Holdings Ltd", exchDisp: "SES", quoteType: "EQUITY" },
      { symbol: "D05.SI", shortname: "Duplicate" },
      { symbol: "QQQ", longname: "Invesco QQQ Trust", exchDisp: "NASDAQ", quoteType: "ETF" },
    ],
  };
  quoteResult: unknown = {
    symbol: "D05.SI",
    longName: "DBS Group Holdings Ltd",
    currency: "SGD",
    fullExchangeName: "Singapore Exchange",
    regularMarketPrice: 54.32,
    regularMarketTime: 1_757_812_500,
    marketState: "REGULAR",
  };
  chartResult: unknown = {
    meta: { symbol: "D05.SI", currency: "SGD" },
    quotes: [
      { date: new Date("2026-09-11T00:00:00Z"), close: 53.82 },
      { date: new Date("2026-09-12T00:00:00Z"), close: null },
      { date: new Date("2026-09-14T00:00:00Z"), close: 54.1 },
    ],
    events: {},
  };

  async search(_query: string): Promise<unknown> {
    this.calls.push({ operation: "search" });
    return this.searchResult;
  }

  async quote(symbol: string): Promise<unknown> {
    this.calls.push({ operation: "quote", symbol });
    return this.quoteResult;
  }

  async chart(symbol: string, options: { interval: "1d"; period1: Date; period2: Date }): Promise<unknown> {
    this.calls.push({ operation: "chart", symbol, options });
    return this.chartResult;
  }
}

describe("Nestworth market Worker", () => {
  it("requires a Bearer token before reaching Yahoo", async () => {
    const fake = new FakeYahoo();
    const app = createApp({ yahooClient: fake });
    const response = await app.fetch(request("/v1/search?q=DBS", ""), env);

    expect(response.status).toBe(401);
    expect(await json(response)).toEqual({
      error: { code: "UNAUTHORIZED", message: "A valid Bearer token is required" },
    });
    expect(fake.calls).toHaveLength(0);
  });

  it("rejects a missing token configuration", async () => {
    const response = await createApp({ yahooClient: new FakeYahoo() }).fetch(request("/v1/search?q=DBS"), {});
    expect(response.status).toBe(503);
    expect(await json(response)).toEqual({
      error: { code: "AUTH_NOT_CONFIGURED", message: "API token is not configured" },
    });
  });

  it("maps search, quote, and history into the public contract", async () => {
    const fake = new FakeYahoo();
    const app = createApp({ yahooClient: fake });

    const search = await app.fetch(request("/v1/search?q=DBS&limit=2"), env);
    expect(search.status).toBe(200);
    expect(await json(search)).toEqual({
      items: [
        { symbol: "D05.SI", name: "DBS Group Holdings Ltd", exchange: "SES", type: "EQUITY" },
        { symbol: "QQQ", name: "Invesco QQQ Trust", exchange: "NASDAQ", type: "ETF" },
      ],
    });

    const quote = await app.fetch(request("/v1/quote/D05.SI"), env);
    expect(quote.status).toBe(200);
    expect(await json(quote)).toMatchObject({ symbol: "D05.SI", currency: "SGD", price: 54.32, source: "yahoo" });

    const history = await app.fetch(request("/v1/history/D05.SI?from=2026-09-11&to=2026-09-14"), env);
    expect(history.status).toBe(200);
    expect(await json(history)).toEqual({
      symbol: "D05.SI",
      currency: "SGD",
      interval: "1d",
      prices: [
        { date: "2026-09-11", close: 53.82 },
        { date: "2026-09-14", close: 54.1 },
      ],
    });
    expect(fake.calls.map((call) => call.operation)).toEqual(["search", "quote", "chart"]);
  });

  it("validates history ranges and never calls Yahoo for invalid input", async () => {
    const fake = new FakeYahoo();
    const response = await createApp({ yahooClient: fake }).fetch(
      request("/v1/history/D05.SI?from=2026-09-15&to=2026-09-14"),
      env,
    );
    expect(response.status).toBe(400);
    expect(await json(response)).toEqual({
      error: { code: "INVALID_DATE_RANGE", message: "from must not be after to" },
    });
    expect(fake.calls).toHaveLength(0);
  });
});
