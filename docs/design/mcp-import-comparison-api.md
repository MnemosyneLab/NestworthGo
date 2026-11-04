# Read-only market import comparison

`compare_market_data_import` is an authenticated read tool in every MCP mode.
It classifies a proposed batch from one local read-only SQLite snapshot, returns
compact results by default, and retains detailed evidence for on-demand reads
from that same capture. It never fetches a provider, imports/overwrites quotes,
changes source priority, refreshes snapshots, creates a write plan/receipt or
grants subsequent write permission. Source text/URLs are untrusted evidence.

## Request and bounds

`items` accepts 1–500 ordered rows, at most 100 distinct valid targets and
128 KiB serialized comparison input; the existing 1 MiB HTTP body guard remains.
Each row requires unique nonempty `clientItemId` (maximum 128 UTF-8 bytes), one
instrument ID or oriented FX pair, quote kind and an exact decimal value string.
An instrument needs its explicit quote currency. Kinds are instrument `latest`,
`close` (raw unadjusted unit close), `nav` (unit NAV), or FX `latest` and
`daily_reference`. FX means 1 base currency equals value quote currency.

Daily rows need the actual source market/NAV/reference `date`. Latest requires
actual RFC3339 `quotedAt` and rejects daily date. Optional daily quotedAt must
belong to that date in its timezone. Timezone resolution shares the writer's pure
rules: explicit IANA zone, instrument market zone, household history zone, UTC
when no history origin exists. Invalid configured zones fail the capture. Future
or impossible dates, unsupported signs/exponents/NaN/Inf/precision are invalid,
never rounded. Current SQLite writes preserve milliseconds; unsupported
submillisecond latest candidates return unavailable, not truncated missing.

Optional sourceTitle/sourceUrl request matching Agent provenance independently
of numerical equality. URLs are not fetched. `displayScale` (0–12; instrument 4 /
FX 6 by default) labels display tails only; any exact nonzero difference remains
a conflict. Instrument prices permit zero and up to 12 integer/8 fractional
digits; FX is positive, at most 8 integer/12 fractional digits.

`view=summary` is default. `view=full` is an upfront bulk-review mode when the
caller already needs all details. These modes use the same bounded evidence
computation. Comparison initial and continuation pages default to 100 rows,
maximum 100, always constrained by the final 64 KiB JSON-RPC budget. Other bounded
read tools retain their own defaults. Explicit smaller limits remain supported.

```json
{"items":[{"clientItemId":"csv/02","instrumentId":"<instrument UUID>","kind":"nav","currency":"USD","date":"2026-09-28","value":"1.00000001"}],"view":"summary"}
```

## Compact results and delivery completeness

Default `results` rows contain ordinal, clientItemId, status, and optional reason,
duplicateGroup and batchConflict. Absent batchConflict means false. Both ordinal
and client ID preserve CSV order and stable identity. Invalid, unknown and
unavailable rows remain in place. A necessary `same_value_different_source` reason
prevents numeric equality being mistaken for matching proposed provenance.

The envelope includes capture ID, fixed capture/expiration times, content/summary
hashes, scoped counts and signed section cursors. `statusCounts` and
`comparisonComputedFromCompleteEvidence=true` describe all server-computed
classifications, even before all result pages arrive. `resultsDelivery` requires
following the results cursor before claiming every row was delivered.

Detailed sections initially contain **zero delivered rows** in summary mode,
with actual total/returned/hasMore/nextCursor metadata. `evidenceDelivery=deferred`
means evidence is retained but not yet delivered; unread is not empty, missing,
or complete. Pages report their own delivery progress. No mutable global
"client finished reading" flag is stored in the immutable capture.

| Section | Retained evidence and access |
| --- | --- |
| results | Compact ordered projection by default; full input/normalization/match reference in full mode. |
| result_details | Full ordered row projection from the same stored results, available for bulk detail paging in summary mode; no second stored copy. |
| matches | Exact equality/difference counts and relations, provenance result and current single-append impact. |
| evidence | Quote identity, exact original value/currency/FX direction, kind/basis/date/time/source, revision, conversion and active/corrected/withdrawn state, audit links and scoped chart selection. |
| audits | Linked Agent append/correction/retraction IDs, targets/replacements, source title/URL and creation instant. |
| targets | Shared captured source policy and selected current quote/value/source/original direction/date/basis. |

Use `get_read_item` with section=results and id=clientItemId for one row. It returns
the original input/normalization and bounded child pages for that row's target,
match metadata, relations, related evidence and linked audits. Relations have a
separate page entrance rather than an empty array pretending to be complete.
Small details fit in one call; larger sets continue through their signed cursors.
Focused detail explicitly does not claim global evidence delivery.

For all details, follow result_details/matches/evidence/audits/targets cursors from
the original summary capture. Do not recapture merely to switch view: a new
capture can observe later facts/policies. Full mode is useful for upfront bulk
review and avoids sending both compact and full row projections. Not every field
is necessary in default results; normalized input, exact deltas, revisions,
conversion dependencies, current policy and audit provenance serve detailed review
and remain available without duplicating durable/cached facts.

## Classification and identity

| Status | Required scoped proof |
| --- | --- |
| same | Comparable active facts exist and all equal the exact candidate value. |
| conflict | A comparable active fact differs, including when another equals it. |
| historical_only | Comparable facts exist only in corrected/withdrawn history. |
| missing | A known target has no retained comparable fact in the requested date/instant, basis, currency and original direction. |
| unknown_target | Instrument does not exist in the household scope, not a missing price. |
| invalid | Candidate value, kind, currency, date, zone or provenance is invalid. |
| unavailable | Unknown basis/date, conversion dependency, unsupported timestamp precision or reverse-only FX prevents a safe comparison. |

SQL/scan failures, auth/permission errors, timeouts and resource limits never become
missing or partial successful classifications. Missing is local retained absence,
not proof that no provider/deleted/unretained historical value ever existed.
Closures are non-price facts outside this interface.

Repeated business keys retain distinct IDs/order and share a duplicate group.
Different exact proposed values set batchConflict independently of persisted
status; two inconsistent proposals can both be locally missing. Repeated client
IDs fail the request. Daily numeric identity uses market date, but cached
simulation also includes each actual normalized instant and timestamp basis.
Same-day/same-price proposals with different times therefore retain independent
selectionImpact. No input is silently chosen.

Shared strict decimal parsers establish equality. Supported `5`/`5.0000` forms
are format-only equal; exact deltas and display_precision_tail/value_difference
remain visible in details. A small delta does not establish a binary-float cause.
NAV basis agent_unit_nav_v1 is separate from raw-close agent_raw_close_v1 /
yahoo_close_v1. Reverse FX retains its original rate/direction; a rounded
reciprocal never establishes original equality or absence.

Matching active Agent title/URL provenance is independent of numeric equality,
selection and import receipt idempotency. A matching source/value does not prove
uniqueness or authorize another import. Additional conflicting active facts are
retained. Source preferences are reported, not inferred or changed.

## Selection evidence scope

Chart selectors consume original stored observation kind and full ordering fields
(revision/conversion dependencies), not normalized display kind nav. chartSelected
is a nullable boolean. It is known only for a complete requested chart day under
the all-time/all-source selector; inactive facts are definitively false. Latest-only
or adjacent legacy days without complete daily scope return null with
chartSelectionReason=incomplete_day_scope. Null is not unselected. Current quote
selection remains separate and can refer to a different captured instant/date.

Single-append current impact uses the existing selectors purely in memory. It
reports would_select_candidate, current_unchanged, ineligible or unknown. Future-ID
ties, absent FX preference and converted dependencies remain unknown. Historical
valuation and simultaneous batch selection are not simulated. Later writes need
independent authorization and writer validation; snapshot hashes are not write CAS.

## Storage, SQL and snapshot guarantees

Queries include requested effective dates/instants, bounded adjacent-day undated
legacy facts, and directly linked Agent operations even if created later or a
replacement moves dates. Linked outside-scope quote IDs do not imply their values
were captured. Daily quotedAt validates/simulates time but does not add redundant
intraday SQL predicates. Generated OR expressions are balanced, preserving all
bound arguments; legal 500-row batches do not exceed SQLite expression depth.

Retain shared 20,000-row/4 MiB SQL budgets, 4 MiB serialized capture, five-minute
TTL, eight entries/16 MiB cache, two builders and ten-second build timeout. Facts
are stored once. Compact rows, full-row aliases and focused child sections are
bounded read projections. Every final response is capped at 65,536 bytes; oversized
individual details fail explicitly. Page/detail calls read no database, do not
renew TTL, and retain the same hash/permission generation after concurrent writes.
Restart all sections after expiry/eviction/revocation.

Typed ordered inputs are hashed before generic set normalization. The query stores
inputHash/itemCount/view; original inputs appear once in stored result rows. Input
hashes identify proposals; content hashes identify captured bytes, including view,
and confer no write authority.

For 35 targets/233 rows, default compact delivery takes three calls; one common
focused detail adds one. Complete bulk review is more expensive: use the dated QA
record for actual calls/bytes and the summary-to-all-details tradeoff. The writer
still caps separately authorized imports at 100 items; 233 later writes need at
least three atomic batches. No real ledger/CSV or live provider data is used in
acceptance tests, and no latency/p95 claim follows from call-count measurements.
