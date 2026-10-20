# H2b: selected-account snapshot components

Baseline: `c14308c16a60746f11f566fd914e5cf5d552a64a` (E1 reverted;
PR147 chart date optimization retained). The initial PR150 commit
`42ad3958055c1006ce7cbf525ba17dd5e301fc16` restricted this path to activity-free
households. The follow-up removes that restriction after tracing the actual
classifier and daily bridge dependencies. E1/E2 remain abandoned. No schema,
proof machinery, review/D planner or historical replay changes are introduced.

## Normal path and dependency boundary

Asset Changes and Return Analysis Wails projections call application projections
sharing `AnalysisService` and its bounded memo. On a miss, `loadInputs` reads the
full archived-inclusive portfolio and origin, maintains closed days, loads full
activities and stored snapshots, loads full FX, constructs the immutable origin
opening when needed, plans H2a quote IDs, uses G's complete quote batches and
resolves snapshot source evidence. H1, G and H2a are existing optimizations, not
new savings here.

The normal SQLite path now hydrates selected-account stored components for
**explicit single-account scope, including households with activities**. Activity
count is not an admission condition. Full captured activities, their normalized
and recorded effects, archived-inclusive account/ownership/holding/instrument
metadata, FX facts/preferences and H2a's activity quote dependencies remain.
No current-account filter is used as proof of historical ownership or closure.

The dependency trace supporting this boundary is:

- `resolveAnalysisUniverse` matches account scope before adding known components.
  Unknown account dates are a household-wide exception, protected by admission.
- `classifyActivity` builds every endpoint from complete effects and full account,
  holding and instrument metadata. This preserves internal/external, counterparty,
  correction and cost attribution without consulting outside snapshot values.
- Snapshot-dependent `effectAmount` and `isCarryingValueRemoval` are called for
  components inside the selected universe. Their start/end item lookups use that
  selected component key. Quote inference and carrying-value removal therefore
  retain their required selected historical observations.
- Outside cash return projections use `effectAmountForComponent`: money is
  converted using full FX and the selected owner currency; a non-money leg is
  passed an empty snapshot in both full and narrowed paths. This path never
  obtains an outside account's snapshot value.
- Daily bridges iterate selected valuation components. Pre-creation checks inspect
  the selected account's historical items and full activities. Household headers,
  including completeness/counts/totals, remain unchanged.

The optional `AnalysisSnapshotRepository` is default-enabled in normal SQLite
analysis. One read transaction covers unchanged latest headers, distinct raw
historical account IDs and item hydration. Every raw account ID must resolve
against the captured full portfolio (including archived accounts); unknown
historical metadata or an unknown selected account triggers full hydration in
that transaction. Other scopes and adapters without the port retain the original
loader. Cancellation, read/scan errors and commit errors still propagate.
Separate existing activity/portfolio/FX read boundaries are unchanged; this is
not a new whole-service transaction. Removing the activity EXISTS check does
not weaken the transaction around snapshot admission and hydration.

Full origin opening construction and maintenance remain unchanged, including
unrelated opening components. Selected historical rows, source quote/preference
IDs, zero positions and valid holdings are retained. Snapshot-referenced evidence
continues through the existing evidence adapter; selected source evidence and
household closure facts remain available. No dependency graph engine is needed.

## Synthetic differential evidence

`TestAnalysisAccountSnapshotLoading` compares actual full and scoped service
loaders backed by SQLite, with G batching in both arms. The full oracle wrapper
hides only optional snapshot loading. The fixture has 14 accounts, 3 stored days,
selected cash, nonzero CNY and zero USD holdings, an outside brokerage/holding,
FX, an unrelated archived account and `America/New_York` history.

The test compares entire ordinary and investment outputs (financial decimals,
attribution, sources, coverage and unknown reasons), native/base outcomes and
errors, unchanged activities/portfolio/FX, durable headers and exact selected
snapshot items. It asserts actual decoded-row reduction and one historical-ID
admission SELECT. It also exercises normal service native-to-base fallback for
both ordinary and investment analysis.

Positive cases include origin/later openings, future activity, selected outgoing
and incoming transfers with fees, corrections to each direction, in-kind position
transfers in both directions and an unpriced carrying-value removal (asserted
at -5). Outside cash trade/fee/dividend attribution is persisted through the real
adapter: normal domain previews have only their cash endpoint redirected, with
endpoint projections recomputed by `ApplyEffects` before atomic persistence.
This is synthetic historical evidence, not a new public command workflow; current
commands normally settle on the holding's account. The associated outside fees
are asserted as two -1 amounts and the USD 2 dividend converts to CNY 14 using
full FX. No kernel-only substitute input is used.

Full fallbacks remain tested for household scope, unknown selection and absent
historical account metadata, both before and after activities exist. The latter
explicitly asserts preserved unknown-account dates. Transfer/correction/future
activity fallback assertions from the initial slice have been replaced with
positive activation and result-equality assertions.

## Small measurements and limits

Linux amd64, Go 1.26.0; synthetic DB, prebuilt snapshots, same process/open handle,
no memo/ensurer in the paired loaders. Two ordered full-then-scoped observations
from the expanded fixture after transfers, corrections, position changes and
outside cash attribution; other checks were not running concurrently:

| From / To | Stored item rows decoded | Whole loader SELECTs | Full / scoped load |
| --- | ---: | ---: | ---: |
| Aug 2 / Aug 3 | 53 → 9 | 35 → 35 | 5.96 / 5.85 ms |
| Aug 1 / Aug 3 (origin opening) | 53 → 9 | 77 → 77 | 12.58 / 9.68 ms |

This is 44 fewer financial component rows decoded (83%) in an activity-bearing
household. One extra distinct-account admission SELECT is offset here by avoiding
an unrelated snapshot FX-evidence SELECT. Other fixtures can pay one net extra
SELECT. The admission query still scans the requested snapshot keys, returning
small account IDs; unrelated SQL work is not entirely eliminated. Existing indexes
are reused. Component rows are scalar, not JSON: savings are scans, ID/currency/
decimal parsing and allocation. Full activities and full origin work remain costs.

The short timings show no consistent speed improvement for this small fixture.
They have order/cache noise and do not establish end-to-end latency, p95,
allocation totals, RSS, native Mac performance or deployment. The delivered
benefit is less unrelated component hydration in normal activity-bearing account
analysis. One-account ledgers can pay admission overhead with no row savings.

## Validation and CI cost

Follow-up checks passed:

- Application analysis/reconciliation and SQLite analysis tests:
  `go test ./internal/application ./internal/infrastructure/sqlite -run
  '^(TestAnalysis|TestReconciliationRemoval)' -count=1` (5.233s / 0.602s).
- Wails analysis package tests (0.220s), affected-package vet and package builds.
- Focused expanded differential regression, formatting and diff checks.

The expanded regression is 0.666 seconds in the final focused run, compared with
0.32 seconds for the initial slice: about 0.35 seconds extra test work, with no new
CI job or dependency. Parent owns CI run/elapsed-time review; local durations do
not establish hosted CI cost. The previous PR150 [check run](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39411678821)
at `42ad395` succeeded in 5m44s elapsed (creation through final update, including
setup/queue), providing a baseline for follow-up CI review. The previous initial-slice full affected package
suite passed at the earlier head and is not represented as a new full-suite run.
No repeated full suites, race or broad benchmarks were run for the follow-up.
No real ledger or frontend changes. Native desktop packaging remains unverified;
GTK4/WebKitGTK6/libsoup3 prerequisites are absent and no system installation was
attempted. The PR records the exact final head and check outcomes.
