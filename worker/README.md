# Nestworth Market Worker

This is a standalone Cloudflare Worker that provides Nestworth with Bearer-token-protected Yahoo Finance market-data endpoints. The Worker returns only the normalized fields used by Nestworth and does not expose raw `yahoo-finance2` responses or upstream exceptions.

## Endpoints

Every `/v1/*` request must include:

```http
Authorization: Bearer <API_TOKEN>
```

### Instrument search

```text
GET /v1/search?q=DBS&limit=10
```

Response:

```json
{
  "items": [
    {
      "symbol": "D05.SI",
      "name": "DBS Group Holdings Ltd",
      "exchange": "SES",
      "type": "EQUITY"
    }
  ]
}
```

`limit` is optional. It accepts values from 1 to 20 and defaults to 10.

### Latest price

```text
GET /v1/quote/D05.SI
```

The Worker selects a regular, pre-market, or post-market price based on Yahoo's `marketState`. If no supported price is available, it returns `502 UPSTREAM_ERROR` instead of converting the missing price to zero.

```json
{
  "symbol": "D05.SI",
  "name": "DBS Group Holdings Ltd",
  "currency": "SGD",
  "exchange": "Singapore Exchange",
  "price": 54.32,
  "asOf": "2026-09-14T09:15:00.000Z",
  "marketState": "REGULAR",
  "source": "yahoo"
}
```

### Daily historical prices

```text
GET /v1/history/D05.SI?from=2025-01-01&to=2026-09-14
```

Both `from` and `to` are required and must use `YYYY-MM-DD` format. `to` is inclusive. The first version provides daily `close` prices only. Exchange holidays are not filled with synthetic prices, and Yahoo rows without a close are filtered out. A single request may cover at most 20 years.

```json
{
  "symbol": "D05.SI",
  "currency": "SGD",
  "interval": "1d",
  "prices": [
    { "date": "2026-09-11", "close": 53.82 },
    { "date": "2026-09-14", "close": 54.10 }
  ]
}
```

Errors use a consistent shape:

```json
{
  "error": {
    "code": "UPSTREAM_ERROR",
    "message": "Unable to fetch Yahoo Finance data"
  }
}
```

## Local development

Node.js and npm are required. Install dependencies and create a local secret:

```bash
cd /Users/waltwang/Developer/walt/Nestworth-go/worker
npm install
cp .dev.vars.example .dev.vars
# Edit .dev.vars and set an API_TOKEN for local use only.
npm run dev
```

Test the local Worker:

```bash
curl -H 'Authorization: Bearer replace-with-a-long-random-local-token' \
  'http://localhost:8787/v1/search?q=DBS'

curl -H 'Authorization: Bearer replace-with-a-long-random-local-token' \
  'http://localhost:8787/v1/quote/D05.SI'

curl -H 'Authorization: Bearer replace-with-a-long-random-local-token' \
  'http://localhost:8787/v1/history/D05.SI?from=2025-01-01&to=2026-09-14'
```

Run the local checks:

```bash
npm test
npm run typecheck
npx wrangler deploy --dry-run
```

## Deployment

After logging in to Cloudflare, set the production secret and deploy:

```bash
npx wrangler login
npx wrangler secret put API_TOKEN
npm run deploy
```

## Enabling the Worker in Nestworth

After deployment, open Nestworth Settings → Market-data providers, enter the Worker URL, and save the Worker token. When creating or editing an investment instrument, set Quote source to `Provider` and select `worker` as the Provider key. Future latest-price and daily-history synchronization for that instrument will use this Worker. Existing `yahoo_finance` and `tiingo` bindings are not replaced.

The Worker provides instrument quotes and history only; it does not provide FX rates. The Worker URL and token are stored in local settings. The token is sent to the Worker only at request time and is not included in Wails DTOs or the household database.

The Worker uses the Cloudflare Cache API: search responses are cached for 24 hours, quote responses for 30 minutes, and history responses for 30 days. Only successful responses are cached, and authentication runs before cache lookup. The cache is an edge cache, not Nestworth's long-term history store; Nestworth should still persist historical prices in its local database.

This Worker depends on Yahoo Finance's unofficial API. Yahoo availability, response fields, and access policies may change, so perform a real Yahoo request and Cloudflare deployment check before production use.
