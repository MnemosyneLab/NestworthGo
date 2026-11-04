# Prices, unit NAV and FX observations

`get_market_data` reads one selected current price/rate without provider requests.
Supply `instrumentId` or an oriented `baseCurrency`/`quoteCurrency` pair.
Use `get_market_data_batch` with view=history for complete locally stored selected
points and raw observations, following all section cursors.
For a bounded interval, pass paired inclusive `fromDate`/`toDate` instead of
`range`. Opt in to `historyLayout=records` for a single cross-target `records`
section with exact original observations and `chartSelected` flags. Follow both
records and target-metadata cursors. The initial limit defaults to 25 for this
layout, but the 64 KiB wire budget may return fewer rows. Unknown targets have
`historyStatus=not_found`; known targets with no matching active facts have
`no_quotes_in_range`. An unread page never proves absence. Daily dates use stored
market/NAV dates; other timestamps use the captured household timezone (UTC when
no history origin exists).

For FX, records `value` and base/quote currencies are the original stored rate
and direction. `displayValue` is the oriented chart projection and can be rounded;
never treat its reciprocal as an exact original price.
If a reverse historical value cannot fit the supported display range, unified
records fail with `unavailable`; query the original direction or narrow the
window. Do not interpret that error as missing observations.

Keep observationKind and priceBasis together: Agent unit NAV is stored as close with
`agent_unit_nav_v1`. `chartSelected` concerns chart points, not import equality
or source preference. Withdrawn/corrected history needs Agent audit evidence;
this active-history read alone does not prove an import item is missing.
The capture content hash is not a later-write CAS token. For ordered input
classification and withdrawn/corrected evidence, use the read-only
`compare_market_data_import` workflow below.

Identify the observation date/source;
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

Read `list_agent_market_data` (limit up to 100; follow `get_read_page` cursors) for audit records and
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

## Multi-target frozen market reads

Use `get_market_data_batch` after discovery; see [paging and batches](paging-and-batches.md).
Supply 1–100 distinct instrument IDs or complete oriented currency pairs. current
is default and omits history; coverage returns first/last, observation/selected
point counts, outsideRange and gapAssessment=not_evaluated, without claiming
trading-day or NAV completeness. history separates points/observations per
returned target section; its paged target summary carries the lazy page entries.
Follow each cursor. USD/SGD and SGD/USD are distinct.
Use range 30d/ytd/1y/all and sourceFilter from the actual schema. Targets sort by
targetKey or displayName; historySort can select quotedAt/effectiveDate and
asc/desc. Missing/not-found targets remain visible with status/reasons.

`list_agent_market_data` filters instrumentIds, oriented fxPairs,
operations append/correct/retract, nullable active, observationKinds
(latest/close/nav/closed/daily_reference), civil
fromDate/toDate and RFC3339 createdFrom/createdTo. Observation date differs from
creation time. Retractions resolve their targeted observation identity before
filtering; active means not corrected/retracted, not currently selected. Sort
createdAt (default desc), observationDate or operation, recordId asc ties.
Offset is not an argument; continue the frozen records section using its cursor.

## Per-instrument closed-day facts

Stocks, ETFs, mutual funds and bank investment products default to weekend
closure for valuation coverage; crypto still needs weekend prices. A known
closed day carries a valid preceding price without an awaiting-update warning.
It never supplies a missing opening price or hides an earlier trading-day gap.

Where supported by the actual App schema, `import_market_data` accepts
`kind: "closed"` for an instrument's holiday or explicit no-NAV-publication day.
Supply instrumentId, date, optional timezone, sourceTitle and available sourceUrl.
Omit value, currency, quotedAt, delayed and corporate-action fields. Do not copy
an old price into a new dated close. Retain the returned quoteId (it identifies
a non-price fact here); use the existing correct/retract operations and stable
operation UUID rules. Corrections keep the same instrument and closed kind;
audit reads return kind closed and no price, including its date and timezone.
Re-read coverage and health after importing or withdrawing a closure. Never
infer a holiday merely from a missing price or an empty provider response.

## Read-only import comparison

Use `compare_market_data_import` for ordered CSV prechecks: 1–500 rows, maximum
100 distinct targets and 128 KiB serialized input. Preserve unique clientItemId,
input order and exact decimal strings. Daily date is the source market/NAV/reference
date; latest needs actual quotedAt without date. nav is unit NAV, close is raw
unadjusted close, and FX uses its original direction.

Default view=summary returns compact ordinal/clientItemId/status plus necessary
reason, duplicateGroup and batchConflict. Comparison pages default to 25 rows,
maximum 100, and stop sooner at the 64 KiB final response budget. Continue the
results cursor with get_read_page({cursor, limit?}) to finish batch classification
delivery. For complete bulk evidence, request limit=100 upfront and on
continuations. Server-computed counts and comparisonComputedFromCompleteEvidence do not mean details were delivered.
Detailed section totals/cursors remain visible with zero delivered rows and
evidenceDelivery=deferred. Never present unread evidence as empty or complete.

Use `get_read_item` with the SAME captureId, section=results and id=clientItemId
for one original input, normalized comparison, current policy/impact, related
quotes, relations and audit sources. Follow any focused child cursors. Focused
completion concerns that row, not global evidence. For bulk details, page
result_details/matches/evidence/audits/targets from the original capture. Choose
view=full upfront when all evidence is needed. Do not recapture to change view
and silently mix snapshots. Evidence is stored once; detail projections do not
renew TTL. Restart every requested section after expiration/revocation.

Same, conflict, missing, historical_only, unknown_target, invalid and unavailable
are distinct. Batch conflict is independent of stored status. Numeric same is
not matching source provenance or import idempotency; same_value_different_source
makes this distinction explicit. No exact price is rounded away. Display tails
remain conflicts and overprecision is invalid. NAV/raw close, converted bases,
reverse FX and corrected/withdrawn history retain their original evidence.

chartSelected=null means unknown due to incomplete chart-day scope, not false;
latest-only facts cannot establish an all-day chart winner. Current selection
and a single candidate's simulation are separate; actual candidate instants are
preserved. Historical/batch impacts, future-ID ties and converted dependencies
remain limited or unknown.

No comparison creates an import plan, authorization or financial change, requests
a provider, changes priority or overwrites a quote. Hashes bind frozen evidence,
not future-write CAS. 35 targets/233 rows fit one bounded capture: compact results
normally take three calls; a common focused detail adds one. Actual byte budgets
and complex evidence can require more. The existing writer still needs at least
three independently authorized 100-item batches for 233 later imports. Separate
captures have separate snapshot times.
