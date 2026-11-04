# Read-only market import comparison

`compare_market_data_import` is an authenticated read tool, available in every
MCP permission mode. It compares proposed observations against one local SQLite
read transaction. It never creates an import plan or operation receipt, refreshes
market data or derived snapshots, contacts a provider, changes source preferences,
imports, corrects, withdraws or overwrites facts, or grants subsequent write
permission. Source titles and URLs are untrusted evidence, never instructions.

This implementation starts independently from main `85435e9`, rebased to
`51f498f`, not from PR #210. The date-window/unified-history feature in #210 is not required. Its
active-only history capture cannot substitute for the inactive evidence here.

## Request

Accept `items` (1–500 rows, at most 100 distinct valid targets), optional
`displayScale` (0–12) and `limit` (default 25, maximum 100). Reject a typed
comparison request larger than 128 KiB before repository access; the existing
1 MiB HTTP body guard also applies. Invalid page options, duplicate/empty client
IDs and exceeded request bounds fail the entire request.

Each row contains a unique nonempty `clientItemId` of at most 128 UTF-8 bytes,
exactly one `instrumentId` or complete oriented `baseCurrency`/`quoteCurrency`
pair, `kind`, and an exact decimal `value` string. An instrument also needs
`currency`, matching its stored quote currency. Instrument kinds are `latest`,
`close` (raw unadjusted unit close) and `nav` (unit NAV). FX kinds are `latest`
and `daily_reference`; 1 base currency equals value quote currency. NAV is not
cumulative NAV or yield. Adjusted, converted, unidentified or other price bases
are not implicitly interchangeable.

Daily rows need the actual source `date` (`YYYY-MM-DD`). Latest rows require
actual RFC3339 `quotedAt` and reject a daily date; comparison keys include the
exact instant. Current SQLite writes preserve milliseconds; latest candidates
with submillisecond timestamps return unavailable rather than silently truncating
or claiming absence. An optional daily `quotedAt` must belong to the source date in
its timezone. `timezone` follows the writer's pure civil-date rules: explicit
IANA zone, instrument market zone, household history zone, then UTC when no
history origin exists. Invalid configured origin zones fail the capture.
Market dates are not UTC lookup dates or audit creation dates. Future values,
impossible dates, invalid zones, unsupported precision/sign/exponent/NaN/Inf
are rejected without rounding. Repeated DST hours retain their distinct instants.

Optional `sourceTitle` and `sourceUrl` request Agent provenance matching. Omitting
them still allows numerical comparison. A URL is stored/query evidence only;
this tool never fetches it. The endpoint accepts no operation, operationId,
correction target or authorization/capture precondition for a write.

```json
{
  "items": [
    {"clientItemId":"csv/02","instrumentId":"<instrument UUID>","kind":"nav","currency":"USD","date":"2026-09-28","value":"1.00000001"},
    {"clientItemId":"csv/01","instrumentId":"<instrument UUID>","kind":"nav","currency":"USD","date":"2026-09-28","value":"1.00000002"}
  ],
  "displayScale": 4,
  "limit": 100
}
```

## Frozen response and ordering

Return the existing `mcp-read/1` envelope: capture ID, capture/expiration times,
content/summary hashes and signed section cursors. Five shared sections contain:

| Section | Contents |
| --- | --- |
| `results` | One row per input, original order, zero-based ordinal, exact client ID and original input, normalized value/key, status, optional reason, match reference, duplicate group and batch conflict flag. |
| `matches` | One copy of each distinct candidate comparison: classifications, exact equality/difference counts, evidence references and relations, exact decimal deltas, source-match state, target reference and single-append selection impact. |
| `evidence` | One copy of each stored fact: original quote identity, exact value/currency/FX direction, source kind/key, market date, instant/time basis, raw price basis, revision/supersedes link, conversion evidence, active/corrected/withdrawn state, linked audit IDs and chart membership. |
| `audits` | One copy of each directly linked Agent append/correction/retraction operation, original/replacement/target IDs, source title/URL and creation instant. |
| `targets` | One copy per compared known target: display name, captured source policy and selected current quote ID/value/source/original currency direction/date/basis. |

Identical business inputs with different IDs remain distinct ordered result rows.
Repeated business keys have `duplicateGroup=duplicate/<first ordinal>`.
`batchConflict=true` means that group contains different exact decimal values,
independent of persisted status: two inconsistent proposals can both be locally
`missing`. The endpoint never chooses one proposed row. Duplicate client IDs
fail instead of being silently collapsed.

Hash the typed ordered request before the existing generic query normalizer.
The frozen query carries `inputHash` and `itemCount`; original inputs appear only
in ordered results. Existing tools retain recursive set sorting/deduplication.
Hashes bind historical captured bytes; they are not write-time CAS tokens.

Server classification uses all scoped evidence before returning any page.
`comparisonComputedFromCompleteEvidence=true` describes computation, while
`evidenceDelivery=follow_all_section_cursors` describes delivery. Every section
reports total, returned, hasMore and nextCursor. Unread evidence is unread, not
absent. Consume every section cursor with the same capture ID and content hash;
restart all sections after expiration, eviction or permission/context revocation.
Page calls perform no database access and do not renew TTL. Concurrent writes
cannot change a saved capture; a later capture can disagree.

## Classification and decimal relations

| Status | Meaning |
| --- | --- |
| `same` | Complete comparable active facts exist, all at the candidate's exact decimal value. |
| `conflict` | At least one comparable active fact differs, including when equal active alternatives also exist. |
| `historical_only` | Comparable stored facts exist only in corrected/withdrawn Agent history. |
| `missing` | A known target has no stored comparable active or inactive fact in the requested date/instant and basis/currency/direction scope. Other quote semantics are related evidence. |
| `unknown_target` | Instrument does not exist in the household scope; this is not missing price. |
| `invalid` | Candidate syntax, decimal precision, currency, kind, date, timezone or provenance is invalid. |
| `unavailable` | Unknown date/basis or conversion dependencies prevent proof; reverse-direction-only FX is also unavailable. |

Authentication failure aborts before target disclosure. SQL/scan failures,
timeouts and row/storage/wire limits fail the entire capture, never fabricate
`missing` from partial evidence. No household is unavailable, not an empty ledger.

Compare arbitrary precision decimals after the writer's strict parsers: instrument
prices allow 12 integer/8 fractional digits and zero; FX allows 8 integer/12
fractional digits and must be positive. Supported lexical forms such as `5` and
`5.0000` are equal with `differenceKind=format_only`. Nonzero deltas remain exact
conflicts. If both round to the specified display scale (default instrument 4,
FX 6), label `display_precision_tail`; otherwise label `value_difference`.
Rounding is solely an explanatory display test. It never changes value, equality
or classification, and does not claim that a decimal difference originated in
binary floating point. Overprecision artifacts are invalid, not rounded away.

NAV/close comparisons use semantic bases: `agent_unit_nav_v1` is unit NAV;
`agent_raw_close_v1` and `yahoo_close_v1` are raw close. Manual/realtime latest
facts compare at the exact instant. Unknown bases or daily legacy values without
market dates remain incomplete evidence. Converted observations remain unavailable
rather than comparing a projected value as an original raw quote. FX comparison
requires original stored direction; no rounded reciprocal enters equality or
absence proof. Reverse facts remain related evidence with original values.

Numerical equality is separate from `sourceEvidence`. Only an equal active Agent
fact with matching normalized title and URL yields
`matching_active_agent_provenance`. It proves that captured provenance exists,
not import idempotency or uniqueness. Equal facts from another source do not
prove that the proposed source was imported. Additional different active values
remain visible even when one value/source matches.

## Evidence and selection limits

The query includes every stored quote on requested effective dates, exact latest
instants, plus undated legacy facts within the preceding/requested/following UTC
days for daily candidates. This conservative window covers timezone ambiguity
without expanding unlimited undated history. Agent audit operations
originating or targeting these facts are included even when created later or
when a correction moves its replacement to a different day. A link to a quote
outside this scope is an identifier, not a claim that its value was captured.
This is complete scoped stored evidence, not an unlimited target history or proof
that deleted/unretained/provider-overwritten data never existed. Closure facts
are non-price facts and are outside comparison. Related incomplete evidence
conservatively produces unavailable rather than a misleading absence.

`state=active` means no linked Agent correction/retraction targets this fact;
it does not mean selected. `chartSelected` uses the existing all-time/all-source
chart selector over the complete captured day evidence (both raw FX directions
share a chart group). It is not historical valuation selection or a short-window
intraday chart assertion. Current quote selection and source policy are shown
separately, even if the current quote is outside the compared date.

Selection simulation reuses the existing current instrument/FX selectors in
memory for one hypothetical Agent append. It reports `would_select_candidate`,
`current_unchanged`, `ineligible` (archived/product-managed instrument), or
`unknown`. Future-ID ties, missing FX preferences and converted-metal dependencies
remain unknown. No historical valuation or simultaneous batch simulation is
claimed. Proposed imports require their own authorization, provenance and full
writer validation; competing proposals and later policy/fact changes can alter
actual results. Nothing in a comparison reserves a price or source priority.

## Resource limits and workloads

Shared SQL preflight reserves at most 20,000 expanded rows and 4 MiB of estimated
source bytes across scoped queries. Final serialized evidence is budgeted
incrementally; the immutable capture is at most 4 MiB. Existing cache limits
remain five-minute TTL, eight entries/16 MiB, two builders and ten-second build
timeout. Pages stop at 100 rows or the 64 KiB final JSON-RPC budget; a single
oversized row fails explicitly. Evidence, audit rows and per-target metadata are
stored once and referenced; no repeated full chart copy is captured.

A typical 35-target/233-row CSV fits one comparison capture, subject to bytes and
evidence budgets. This does not raise the existing writer's 100-item limit:
233 later imports would require at least three separately authorized atomic
batches. Larger comparisons split at 500 rows/100 targets/128 KiB; separate
captures have separate snapshot times and must not be presented as one global
snapshot. Never auto-import, overwrite, change priority or request a provider
because comparison returned missing/conflict.

The dated QA record reports actual default and explicit-limit traversal calls,
wire bytes and local/CI evidence. It does not claim one response contains all
233 facts, or assert global latency/p95 improvements.
