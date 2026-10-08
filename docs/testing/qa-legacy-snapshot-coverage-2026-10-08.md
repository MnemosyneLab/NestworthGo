# Legacy analysis snapshot coverage — synthetic verification

Base: `c4c157171ebbf47f09f9d4e20d0970caf11b4261` (PR #40), 2026-10-08.
All ledgers are disposable SQLite fixtures. No live ledger, provider, R2, or
publication/deployment was used.

## Reproduction before the fix

Start history on Aug 1, UTC, with complete synthetic cash facts, no snapshots,
`dirty_from=NULL`, and no completion watermark. Advance the clock to Aug 12.
Run a late request, followed by Aug 1–2 legacy analysis or a custom trend.

| First request | Rows after first request | Subsequent Aug 1–2 analysis | Net worth / portfolio trend |
| --- | --- | --- | --- |
| Attribution Aug 10→11 | Aug 10, 11 | `unavailable`, both daily returns null | Both dates `missing`, null amounts; net worth `missing_boundary` |
| Legacy analysis Aug 10–11 | Aug 9, 10, 11 | `unavailable`, both daily returns null | Same missing dates and null amounts |

All six combinations failed to materialize Aug 1 and 2. Watermark Aug 11
incorrectly caused the old helper to skip the earlier request. A later dirty
marker could also skip absent dates before it. The old hash migration expanded
any stale row into a full origin→yesterday rebuild.

## Change and contracts

Use PR #40's actual-row/dirty-range coverage planner for both old analysis and
trends. Retain the analysis caller's inclusive period and predecessor (or origin
opening); trends maintain only their displayed closed dates. Rebuild contiguous
missing, dirty, old-hash, or old-resolver-policy days in at most 31-day chunks.
Unrequested stale rows do not expand the request. Existing per-day saves keep
the watermark monotonic and advance only a matching dirty prefix. Range
completion is permitted only when the requested range covered that prefix.
A retained earlier dirty prefix can still span later rows already rebuilt at
its current generation. Repeated later requests reuse those rows after checking
actual presence, current hash/policy, and positive generation provenance from a
generation-aware repository. Unknown-generation dirty rows still rebuild.

The old helper now holds the existing serial, reentrant coordinator gate
throughout planning/build/completion. Pure coverage reads release the gate
without incrementing the write revision; nested write entry points mark the
permit and invalidate previews even for equal-hash metadata writes or partial
rebuilds. Existing outer WithWrite/exclusive behavior remains unchanged. Batch/save generation guards remain in use; a final
state-generation check also rejects a revision during a clean/no-op coverage
read or between batches. A concurrent client can cause `conflict` rather than
silently acknowledge stale coverage; the remaining dirty range is resumable.
No valuation, rounding, replay, schema, or public API contract changed.

## Regression evidence

`legacy_snapshot_coverage_test.go` covers both orders for late attribution/old
analysis followed by old analysis/net worth trend/portfolio trend; exact sparse
date sets; repeats with zero extra builder saves (including equal-hash saves);
repeats under a retained earlier dirty prefix; partly present intervals and
interior holes; in-window old hash/policy and an
out-of-window old hash; bounded dirty prefix/tail and an unbounded quote-repair
tail; manual quote correction and effective-dated source preference revisions;
41 days in two chunks; external source mutation after coverage read, after batch
read, and between chunks, followed by successful resume.

Financial controls use 100 CNY cash plus ten synthetic fund units with daily
prices 5, 6, …: Aug 1–2 investment return and closed-date trend/attribution change
are 10 CNY. Correcting Aug 2 to 7 produces 20 CNY across all three paths and
retains attribution's precision identities. Aug 10–11 legacy analysis includes
the predecessor and returns 20 CNY; endpoint trend/attribution change is 10 CNY.
These are their existing, distinct period contracts.

Havana Nov 1's repeated left midnight remains supported for history-origin-day
legacy analysis and trends (their next midnights resolve). Attribution retains
`historical_boundary_unsupported`; its stricter preflight was not copied into
the legacy entry points. Existing 31-day, historical, analysis, and attribution
suites remain part of the verification.

## Guarded-preview regression found in review

Preserved reviewed head: `de522081cad3f9662f85aa162ea90b7afdc29f94` (CI
37751445891 passed both jobs). The initial coverage refactor acquired WithWrite
for pure maintenance, and releaseWrite incremented the preview revision even
when no rows changed. A pinned synthetic cash-only ledger with all closes Aug
1–11 materialized and clean reproduced this against the exact reviewed head.

| Reader after guarded cash +1 preview | Base c4c1571 database changes / commit | Reviewed de52208 database changes / commit | Fixed result |
| --- | --- | --- | --- |
| NetWorthTrend Aug 10–11 | 0 / success | 0 / stale_preview | 0 / success |
| PortfolioTrend Aug 10–11 | 0 / success | 0 / stale_preview | 0 / success |
| Uncached direct Analyze Aug 10–11 | 0 / success | 0 / stale_preview | 0 / success |

All commits used a fresh valid mutation UUID, 64 hex-character payload hash,
and the original token. Additional tests cover real Wails AssetChange and
ReturnCalendar warm reads; missing-day writes (3 database changes) rejecting
the old token; equal-hash metadata writes rejecting the old token without
appending physical revisions; and a coverage-read barrier that fences a queued
guarded writer, whose original token remains valid when that pure read finishes.
MCP analyze_period's existing outer WithWrite behavior is unchanged.

## Additional independent-review findings

Review of the initial refactor identified two further reachable correctness
regressions, also reproduced on the preview-fixed head `6f91436`:

* A real `CommitInstrumentHistory` coverage commit at Aug 6 creates dirty Aug
  2–5. Materialize Aug 6–11, then record ordinary income +50 effective Aug 3 at
  Aug 12. Before the propagation fix, Aug 10 stayed 240 (expected 290), including
  after rebuilding Aug 2–11; Aug 5→6 changed −40 (expected +10). The ordinary
  mutation increased generation but inherited the earlier dirty upper bound.
  The same transaction now widens an existing bound to the last closed household
  day and retains NULL for already-unbounded work. The financial regression now
  reads 290 / +10 / 290; legacy investment return and attribution identities pass.
  This reproducer uses the production history-commit entry point, not fabricated
  SQL markers or a live provider.
* Attribution Aug 10–11 followed by the Oct 3 thirty-day trend leaves 31 rows and
  **32** missing closes (Aug 1–9 and Aug 12–Sep 3) out of 63 closed history days.
  Previously health was healthy with zero issues, repair estimated zero, and
  repair did nothing. Health/preview now inspect actual rows for holes and old
  hash/policy, without building them. Repair fills exactly those 32 holes in two
  bounded builder batches; already-present rows are retained, then health is
  healthy and repeat repair builds zero days. The state-only helper is renamed
  `dirtySnapshotRange` and reports only pending invalidation. No application
  consumer uses the completion watermark to prove contiguous coverage.

A separate complete-valuation SQL-marker fixture retains the original Aug 2–5
state shape and verifies 290 / +10, including repeat reads without extra saves.

The fully asynchronous cancelled-sync path uses StartMarketDataSync, a synthetic
Tiingo adapter and the real history persister. At Aug 6, CancelSyncJob runs after
the commit and before snapshot rebuilding: dirty Aug 1–5, generation 1, no
watermark. At Aug 16, Aug 6–11 snapshots store subtotal 150 at generation 1 but
are incomplete because later closes are missing. Ordinary income +50 effective
Aug 3 previously left subtotal 150/gen1 on pinned 6f91436; base c4c1571 returned
200/gen2 under the same flow. The fix returns 200/gen2 both before and after an
Aug 1–11 request and retains dirty Aug 12–15. The trend still has null change and
missing_boundary. This evidence is distinct from the complete-valuation −40
case; no incomplete amount is presented as a valid trend delta. Data Health
collapses missing-day work behind missing-input prerequisites instead of
advertising it as immediately executable.

Cross-chunk publication remains intentionally resumable, not range-atomic.
A mutation after the first 31 saves leaves 41 rows of mixed generations and
returns conflict: Aug 2 is still 160 while Sep 10 is already 551. Durable dirty
state makes health report outdated work. An eight-case test exercises Analyze,
net worth trend, portfolio trend and attribution, both retrying on the same
Service and closing/reopening SQLite. Each reader repairs its required coverage;
all 41 dates are checked for exact net worth (Aug 2=161, Sep 10=551), completeness
and affected-date generation. Analysis component totals and investment return
400, net worth change 401, instrument-only portfolio values 60/450, and attribution
precision identities are verified. Uncovered pending tails remain durable.

A separate repair-loop race was confirmed on 0cc2e68. Materialize Aug 1, record
cash +1 effective Aug 2, then inject another ordinary +1 effective Aug 3 after
the second batch is loaded. The old retry loop rebuilt only that later chunk
at generation 2 and incorrectly completed the earlier dirty prefix: Aug 3
remained 171/gen1 (expected 172/gen2), dirty was NULL, and health was healthy.
Repair now fences every chunk against the invocation's initial generation and
returns conflict on revision instead of retrying a fixed later chunk. The
unrepaired Aug 3 prefix remains durable and health reports outdated work. Both
same-Service retry and SQLite close/reopen repair all 63 amounts and affected
generations; the only remaining unbounded dirty cursor is Oct 3's open day.
Existing interrupted-batch recovery remains verified. No range-atomic rewrite
or schema/API expansion was added.

The extra health diagnosis performs a latest-row scan and date-label iteration;
its large-history cost was not separately benchmarked. Ordinary-mutation bound
propagation adds no history rebuild to the write itself.

## Performance

`legacy_snapshot_coverage_benchmark_test.go` measures a cash-only, 62-closed-day
ledger and the Aug 1–2 net worth trend. Before/after runs used Go 1.26.0, Linux
amd64, GOMAXPROCS=5, the same machine, no competing test run, `-benchtime=20x
-count=3`; medians of three samples are shown. Setup is excluded from timing.
Warm samples first materialize all history; cold samples start with no rows.

| Case | Base ns/op | Fixed ns/op | Base B/op | Fixed B/op | Base allocations | Fixed allocations |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Warm bounded trend | 2,970,243 | 852,050 | 739,307 | 58,003 | 25,618 | 1,660 |
| Cold bounded trend | 26,624,496 | 5,027,233 | 1,702,263 | 165,592 | 51,148 | 4,213 |

Warm time fell about 71%; cold time about 81%. Cold reconstruction narrows from
62 days to 2. This is a synthetic coverage benchmark, not a production latency
claim. Large ledgers, all-time trend latency, native UI latency, live providers,
and R2 were not measured. The shared planner adds one state read to attribution
for the final generation check; attribution latency was not separately measured.

## Validation scope

Local focused regression tests, the application suite, formatting/diff checks,
user-skill validation, and 20 installer tests pass. Full default local tests hit
missing GTK/WebKit/libsoup dependencies in `cmd/nestworth`; native CI installs
those dependencies. Local all-package headless tests/vet and internal-package
race tests are run serially, with all existing performance-budget tests enabled.
Final native CI and exact commit are reported on the draft PR and in the handoff,
without changing CI, skipping tests, or relaxing thresholds.
