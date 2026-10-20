# H2b: activity-free account snapshot components

Baseline: `c14308c16a60746f11f566fd914e5cf5d552a64a` (E1 reverted;
PR147 chart date optimization retained). E1/E2 are abandoned. This change
adds no schema, proof machinery, review/D planner changes or replay algorithm.

## Normal path and dependency boundary

Asset Changes and Return Analysis Wails projections call the application
projections, which share `AnalysisService` and its existing bounded memo.
On a miss, `loadInputs` reads the full archived-inclusive portfolio and origin,
maintains closed days, loads activities and stored snapshots, loads full FX,
constructs the immutable origin opening when needed, plans H2a quote IDs and
loads G's complete quote batches, then resolves snapshot source evidence.
H1's existing account hydration, G's batching and H2a's quote scoping are not
new savings here.

Classification precedes output filtering. Activity transfer counterparties,
normalized and recorded correction effects, cost origins and historical holding
metadata cannot be discarded using current account membership. H2b therefore
admits **only explicit single-account scope with no captured activities**, and
SQLite additionally requires **no household activity at all**, including future
activities. All activity-bearing households retain full component hydration.
This is deliberately a small delivered subset, with no dependency graph engine.

The optional `AnalysisSnapshotRepository` is used by the normal SQLite service
without a flag. Adapters without it keep the original loader. One read
transaction covers the activity existence check, unchanged latest snapshot
headers, raw distinct historical account IDs, and item hydration. Every raw
historical account ID must exist in the captured full portfolio, including
archived accounts; unknown IDs or an unknown selected account fall back to full
hydration in that same transaction. SQL/read/scan errors propagate, rather than
being interpreted as successful admission. Captured activities are loaded before
this read, so an activity appearing later cannot introduce new classification
inputs after the admission decision. Other input families retain their existing
separate read boundaries; this does not introduce a whole-service transaction.

For admitted input, only stored component rows belonging to the selected account
are decoded. With no activities, classification has no counterparty, correction
or cost effects; universe construction already filters known unrelated items
by account. Unknown-account detection is protected by admission. Day calculations
and pre-creation checks use the selected components. Household snapshot headers
(including completeness, counts and totals) remain unchanged. All portfolio
records, FX, needed quote candidates and evidence remain available. Full origin
construction/maintenance remains unchanged, including its unrelated components.
Opening dates, timezone, zero positions and valid historical holdings are retained.

## Small synthetic evidence

`TestAnalysisAccountSnapshotLoading` uses the real service loader and SQLite
adapter, with a wrapper hiding only optional snapshot loading as the full oracle.
The fixture has 13 accounts, 3 stored days, a selected cash component, a nonzero
CNY holding, a zero USD holding, FX, an unrelated archived account and
`America/New_York` history. It compares entire ordinary and investment results,
including source/coverage/unknown fields and exact financial values. It also
checks unchanged portfolio/activity/FX inputs and durable snapshot headers.
Both a later opening date and the origin-date opening are exercised.

Fallback comparisons cover household scope, an unknown selected account,
historical account metadata absent from the captured portfolio (with an explicit
unknown-account-date assertion), a future transfer, an in-range transfer with
fee, and its correction. Captured activity cases never invoke the narrow port;
the future activity case invokes it and is rejected within SQLite. The test
uses synthetic databases only and takes well under a second in the focused run.

Linux amd64, Go 1.26.0; snapshots prebuilt, same process and open database,
no memo/ensurer during paired loading. Driver metrics include SQL consumption
through row close. Two short ordered full-then-narrow observations from one
final focused run after package checks completed (not randomized or
independently cold):

| From / To | Stored item rows decoded | Whole loader SELECTs | Full / narrow load |
| --- | ---: | ---: | ---: |
| Aug 2 / Aug 3 | 45 → 9 | 29 → 31 | 13.91 / 4.17 ms |
| Aug 1 / Aug 3 (origin opening) | 45 → 9 | 71 → 73 | 12.46 / 10.98 ms |

The new saving is 36 fewer stored financial component rows decoded (80%) in
this fixture. The two extra SELECTs are activity admission and distinct historical
account IDs. The latter still scans the requested snapshot keys, returning 13
small account IDs; this is not a claim of avoiding all unrelated SQL work.
Existing snapshot/account indexes are reused. There is no JSON component decode
here: savings are row scan, ID/currency/decimal parsing and item allocation.
Quote reductions remain H2a's work. The ordered timings have cache/order noise
and do not establish end-to-end speed, p95, allocation totals, RSS, native Mac
performance or production deployment. Activity-heavy ledgers gain nothing here;
one-account ledgers can pay two extra checks without reducing item rows.

## Validation boundary

Passed: `go test ./internal/application ./internal/infrastructure/sqlite
./internal/wailsapi/analysis` (application 162.356s, SQLite 38.274s, Wails analysis
0.261s); final focused differential test (0.322s); vet and builds of those three
packages; gofmt and diff checks. The package suite ran once; final focused
verification includes subsequent test assertions and the normal quote-batch port
in both arms. No race, broad benchmark
matrix, repeated full suites, real ledger, frontend changes or native desktop
packaging. Linux desktop build prerequisites (GTK4, WebKitGTK 6 and libsoup 3)
are absent; no system dependency installation was attempted. Parent owns CI monitoring and elapsed-time review. The draft PR records
the exact tested head and final check outcomes.
