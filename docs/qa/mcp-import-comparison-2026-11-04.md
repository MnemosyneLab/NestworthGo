# MCP import comparison QA — 2026-11-04

[Task #211](https://github.com/MnemosyneLab/NestworthGo/issues/211),
[draft PR #212](https://github.com/MnemosyneLab/NestworthGo/pull/212).
Isolated cloud Linux amd64 / Go 1.27.3, real modernc SQLite temporary databases
and loopback MCP HTTP/SDK. No real ledger/CSV or external market data was used.
The endpoint is read-only; setup writes populate synthetic fixtures only.

## Validated review findings

Each regression first failed against the published implementation, then passed
with the fix. The original assertions/oracles were retained.

1. **Daily simulation cache identity.** Persist noon NAV=2; propose same-day/same-
   price NAV=1 at 09:00 and 15:00. Individually the existing selector reports
   current_unchanged / would_select_candidate; the old batch reused the first
   result for both. Cache identity now includes actual normalized instant and
   timestamp basis. Numeric duplicate groups still use the daily business key.
   Both input orders are tested.
2. **Original chart ordering.** Earlier Agent unit NAV versus later provider raw
   close must resolve as daily source evidence, not normalized nav-as-realtime.
   Converted provider facts also require raw observation time and revision.
   Real quote-series reads independently establish the expected selected IDs.
   Comparison now uses original stored observation kinds and complete ordering
   fields, preserving revision/conversion evidence.
3. **Incomplete chart scope.** Latest facts at 09:00 and 15:00, requesting only
   09:00, previously claimed the captured point was the all-day chart winner.
   It now returns chartSelected=null / incomplete_day_scope. Adding a daily
   request supplies the complete day and yields known false/true flags. Current
   quote selection remains separate. Inactive facts are definitively false.
4. **SQLite expression depth.** A legal 500-row daily+quotedAt request failed in
   the project's modernc driver with `Expression tree is too large (maximum depth
   1000)`. Daily instants no longer duplicate date predicates, and generated OR
   trees are balanced without changing arguments, precision or row/byte limits.
   Both 500 daily timestamps and 500 latest instants are regression fixtures.

No selector policy was invented, financial write was used for comparison, or
precision/storage ceiling was raised to pass these tests.

## Default summary and on-demand evidence

Default view=summary emits compact ordinal/clientItemId/status plus necessary
reason, duplicateGroup and batchConflict. Detailed original inputs, normalized
keys, deltas, source records, chart/current selection and audits remain retained
once in the same frozen capture. Default comparison page size is 100, maximum
100, with the 65,536-byte final-wire cap still authoritative; explicit 25 is tested.

`comparisonComputedFromCompleteEvidence` and statusCounts describe server
computation. Delivered result pages and deferred detail sections are separate:
detail page metadata initially reports returned=0 with signed continuation cursors.
Following only results establishes batch classification delivery, not full evidence
delivery. `get_read_item` section=results id=clientItemId returns bounded focused
children from the same capture; a focused item never claims global delivery.

`result_details` and focused sections are virtual projections, not additional
stored evidence. The cache still stores only five physical sections. Tests verify
original decimal lexemes, order, unique evidence identities, fresh decode targets
(no stale source URL/reason leakage), frozen retries after a later correction,
unchanged hash/expiry, bounded child pagination, cursor tampering/section binding,
revocation and expiry. Small focused details fit in one HTTP call. Large relations
are independently paged rather than embedded in an oversized match record.

## Measured 35-target / 233-row workload

233 exact proposed rows match 233 synthetic Agent NAV records across 35 targets.
Setup imports use three existing 100-item writer batches and are excluded from
read metrics. The SDK returns structured JSON and JSON text; measured bytes include
both, escaping and the complete JSON-RPC response.

| Journey | Calls | Total actual response bytes | Maximum actual | Maximum reserved |
| --- | ---: | ---: | ---: | ---: |
| Default compact results, 100 rows | 3 | 38,986 | 19,544 | 20,424 |
| Compact results, explicit 25 | 10 | 51,248 | 10,391 | 11,270 |
| One common focused detail after summary | +1 | 6,924–6,925 | under 7 KiB | under 8 KiB |
| Default summary then every detail section | 19 | 885,962 | 64,521 | 65,400 |
| Upfront full review, default 100 | 15 | 849,937 | 64,520 | 65,398 |
| Upfront full review, explicit 25 | 40 | 893,792 | 64,394 | 65,272 |

Minor ID/cursor/page-boundary variation changes bytes slightly between runs.
Frozen summary/full captures measured 386,148/386,151 bytes, below 4 MiB. Complete
traversals retain 233 full input rows, 233 matches, 233 original facts, 233 audits
and 35 target rows. The summary-to-all-details total excludes the separate focused
item experiment. No missing, duplicate or reordered identities are accepted.

The main tradeoff is explicit: routine precheck needs three calls and one selected
detail adds one; a caller needing every field should choose full upfront to avoid
sending both compact and full row projections (19 versus 15 calls). Fields such as
original input/normalization, exact delta, revision/conversion, source policy and
audit provenance are useful for detailed review but unnecessary in each default
status row. Minimal delivery changes neither the computed classification nor the
retained evidence or later-write authorization.

The earlier full-only version needed 16 calls; metadata-first packing reduced it
to 15. Existing history traversal needs 73 calls but supplies neither comparison
classification nor withdrawn absence proof. These workloads have different evidence
content. No latency/p95, universal call count or one-response claim is made.
The writer remains limited to 100 items per separately authorized batch.

## Bounds, non-effects and checks

Existing tests retain exact decimal zero/tiny/max/tails/overprecision/format-only
coverage; different-source equality and simultaneous conflicts; NAV/raw-close and
original/reverse FX; date/market-day/DST/UTC fallback/invalid zones; correction and
withdrawal; ordered duplicates; unknown/invalid/unavailable versus scoped missing;
concurrent atomic snapshot generations; auth/permission failures; SQL errors,
20,001 rows, source bytes and escaped-JSON 4 MiB failure. query_only, total_changes,
portfolio equality and forbidden provider fixtures verify no database/provider
side effects. Snapshot/input hashes are not future-write CAS.

Application/MCP/SQLite review regressions and the full internal Go suite passed
(application 192.184s, SQLite 38.201s, MCP 80.808s in this cloud run). Internal
vet, formatting and whitespace checks passed. All 136 tooling tests passed in
123.078s; 73 current skill examples/references, the distribution version guard,
package checksum and all 16 archive/source comparisons passed. Exact-head native
CI links/status are recorded separately in the PR; these local timings are test
durations, not product latency claims.
Skill 2.3.0 remains the already-increased version in this unmerged PR, above main's
2.2.0; updated guidance describes compact versus detail delivery and unknown chart
selection. Published assets and installed user skills are unchanged. No frontend
implementation or generated bindings are changed.

Native desktop compilation locally requires unavailable GTK4/WebKitGTK6/libsoup/
glib/gio development packages. Linux tests do not establish native GUI, packaging,
signing or notarization acceptance. Only the exact new head's CI proves its native
checks; older green heads remain separate evidence.
