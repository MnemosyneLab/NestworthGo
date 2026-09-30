# Prices, unit NAV and FX observations

`get_market_data` reads selected current price/rate and local history without
fetching a provider. Use `instrumentId`, or both oriented `baseCurrency` and
`quoteCurrency`, plus optional `range` (`30d`, `ytd`, `1y`, `all`, or a supported
explicit `YYYY-MM-DD:YYYY-MM-DD` range). Identify the observation date/source;
a carried NAV is not a newly published NAV.

For “补上昨天的基金净值”, obtain the actual issuer/market evidence from the
user's document or the client's available research tools. Verify identity,
currency, unit, price basis and source date. If research is unavailable, request
the values/evidence; do not fabricate them. The App does not call an Agent to
fetch data. A source URL is provenance only and is not fetched by the App.

`import_market_data` needs ledger permission and 1–100 `items`, atomically saved
with one operation UUID. An ordinary instrument allows `latest`, raw unadjusted
`close`, or unit `nav`. FX allows `latest` or `daily_reference`, with positive
rate meaning **1 baseCurrency = value quoteCurrency**. Instrument price zero is
valid evidence; missing/unknown is not zero. Do not enter cumulative NAV,
annualized yield or an adjusted close as raw unit price.

Example issuer unit NAV:

<!-- example: fund-nav -->
```json
{"operationId":"${operationId}","input":{"items":[{"instrumentId":"${instrumentId}","currency":"CNY","value":"1.0197","kind":"nav","date":"2026-09-28","timezone":"Asia/Shanghai","sourceTitle":"Issuer unit NAV publication","sourceUrl":"https://example.com/nav"}]}}
```

Example dated FX reference:

<!-- example: fx-reference -->
```json
{"operationId":"${operationId}","input":{"items":[{"baseCurrency":"USD","quoteCurrency":"CNY","value":"7","kind":"daily_reference","date":"2026-09-28","timezone":"Asia/Shanghai","sourceTitle":"Bank reference rate publication"}]}}
```

`latest` requires the actual RFC3339 `quotedAt`, not lookup/import time. Daily
items require the actual market/NAV/reference date; an optional actual timestamp
must agree with that date in the supplied timezone. Without a timestamp the App
uses a date-label anchor, not an assertion that a quote occurred at midnight.
Future observations are rejected. Always preserve source title and available
URL. Optional daily `splitFactor` / per-unit `dividendCash` is metadata, not a
ledger split/dividend transaction.

## Preference, correction and withdrawal

Read `list_agent_market_data` (offset/limit up to 100) to find audit records and
actual quote IDs. `active` means eligible, not necessarily selected.
`operation:"correct"` requires an existing Agent `targetQuoteId` and the full
replacement observation. `operation:"retract"` accepts only targetQuoteId,
operation and provenance; omit the old value/identity/date fields. These tools
cannot retract provider/manual facts.

<!-- example: withdraw-quote -->
```json
{"operationId":"${operationId}","input":{"items":[{"operation":"retract","targetQuoteId":"${quoteId}","sourceTitle":"Issuer withdrew the observation"}]}}
```

Instrument `quoteSource` is set through create/update; `set_fx_source` takes a
mutation envelope whose input has `currencyA`, `currencyB`, `source` (different
field names from quote import/query). For example:

<!-- example: fx-source -->
```json
{"operationId":"${operationId}","input":{"currencyA":"USD","currencyB":"CNY","source":"agent"}}
```

`agent` is Agent-only, while manual/provider preferences can accept supplied
Agent overlays. Current daily values resolve by effective date, Agent winning
the same date, before comparing with the latest realtime timestamp. Valid exact
daily Agent dates suppress routine provider fetches; missing/stale inputs still
need their configured supply. Do not claim every new import will be selected.

After import inspect `currentValuationStatus` and `snapshotStatus`, then current
quotes and health. A `pending` derived update needs repair; a completed rebuild
does not prove data completeness. Retraction restores eligible fallback facts;
metal prices dependent on FX are repriced, or unavailable without fallback FX.
App-managed product quotes require the App's own product workflow.
