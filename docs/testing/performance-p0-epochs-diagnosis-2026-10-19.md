# PR111: 64-cycle tail investigation and publication regression

No focused production fix for the original 78.931 ms p95 gap is established.
Keep the baseline default and the original review trigger. This follow-up adds
measurement diagnostics and stronger independent publication assertions only;
production SQL, lifecycle code, schema and defaults are unchanged from `c78344d`.
No C1 work or merge is included. Parent handles remote CI.

## Comparison controls

The original **173.338 → 252.269 ms** arrow compares baseline SQL and bounded SQL
inside the same `a949c01b618bb02b772665644d7a4b030fec634f` test binary/run.
It is not a comparison of the earlier before/after binaries. Both paths used the
same unconditional midnight replay fix from `eb096bc`. SHA-256 of
`review_lifecycle_plan.go` is identical at `eb096bc`, `a949c01` and `c78344d`:
`9013cac3938172a0f162169687d75a0d857a706fb63d8ac1ec5da5f62fdcd93e`.
The baseline/bounded repository flag selects evidence SQL; it does not select
lifecycle behavior.

The fixture generates one synthetic source, including all 64 real full-exit /
re-entry cycles, and creates identical SQLite `VACUUM INTO` clones for each path.
Each measured publication uses a fresh Service and equivalent connection pool.
Financial/capture equality, the independent complete recomputation and every
durable projection/lifecycle/event field remain checked. Both capture order and
publication order use `(sample + offset) % 2`: each path goes first 15 times and
second 15 times. The two worst bounded samples were in **different** slots:
sample 13 first, sample 12 second. A fixed first/second-slot bias alone does not
explain them; counterbalancing does not eliminate all heap/scheduling carryover.

The full-reconcile timer and allocation delta surround only
`Service.ReconcileInvestmentReviewsV2`. Clone creation, service/metric-pool setup,
separately timed capture comparison, full-oracle preparation/comparison, durable
row comparison, log formatting and cleanup are outside that timer. Metrics are
reset before the call and recorded before the durable-row scan. They do not
count oracle SQL as reconciliation SQL. The earlier standalone capture timings
are separate calls; subtracting their p95s from full reconciliation would not
produce a valid publication-stage estimate.

Allocation figures are process-wide Go TotalAlloc/Mallocs deltas, not native
or off-heap allocation/RSS measurements.

Outside-timer work can leave collectible allocations and affect a later timed
GC. In particular, the first publication is preceded by the full oracle; the
second is also preceded by the first publication and durable-row comparison.
Alternating order balances that distinction, but does not create independent
process/heap states. The old run did not record per-call GC, scheduling or
per-query window times, so their contribution to those exact old tails cannot
be reconstructed.

## Original measurements retained

There were 30 pairs, 105 queries returning 1,440 rows and 521 write attempts
with **zero affected rows** on every path/sample. Go allocation counts do not show
a comparable multiplication:

| Original no-op statistic | Baseline | Bounded |
| --- | --- | --- |
| Total median / p95, ms | 103.206 / 173.338 | 107.736 / 252.269 |
| Go TotalAlloc delta, median bytes | 7,389,080 | 7,394,544 |
| Go Mallocs delta, median | 144,404 | 144,436 |
| Query-to-Rows.Close, median, ms | 42.069 | 44.414 |
| Driver Exec, median, ms | 47.643 | 47.905 |

The bounded p95-ranked sample 12 allocated **6,870,816** bytes, less than the
baseline p95-ranked sample 8's **7,390,728** bytes. Its whole-call query / Exec
times were 142.940 / 87.897 ms, versus 104.302 / 45.781 ms for sample 8.
The bounded maximum, sample 13, took 327.705 ms, with 164.190 ms query lifetime
and 117.457 ms Exec time. Exec is the common publication/write path, not the
bounded read query. These point observations locate variation outside just the
evidence query, but do not prove that the read query could not indirectly
trigger a GC/allocator/scheduling slow path. Old per-query/GC data is absent.
See [original extracted samples](performance-p0-epochs-2026-10-19/diagnosis/original-noop-samples.csv).

## New instrumented stage experiment

Immutable source: `3007340b97a3a56743578de2424920a6d4baf24a`.
The opt-in `TestReviewEpochStageDiagnostics` uses the identical E64 configuration
(one account/holding/instrument, four rules, 30 history days, 30 generated quotes,
64 cycles) and 30 no-op pairs. All epoch-boundary and per-sample full/durable
oracles run; none is skipped through subtest filtering. Same Go 1.26.0, SQLite
3.53.0, schema 19, Linux amd64, four-CPU quota/GOMAXPROCS and synthetic tmpfs data
as the original. No build/race/profile jobs ran alongside timing.

A test-only forwarding repository instruments the **actual Service call**:

- Capture includes full financial reads/fingerprinting and reusable evidence SQL.
- Prepare/admission is the interval after capture until publication entry,
  including `PrepareReviewPublication`, provider-key lookup and write admission.
- Publish includes full source recheck/fingerprinting, validations, SQL attempts
  and transaction commit.
- Residual includes household lookup/connection opening, unlock/result work and
  small instrumentation overhead.

Only time reads and O(1) counter snapshots occur inside measured stages; SQL
statement-map copying/sorting and diagnostic logging occur after the call.
Their extra allocations can influence later samples, so this instrumented run
is **separate diagnostic evidence**, not a replacement for the original run.
Window SQL is a subset of capture, SQL query/Exec counters are nested within
stages, and marginal p95s must not be summed.

| Stage/statistic, ms unless stated | Baseline median / p95 | Bounded median / p95 |
| --- | --- | --- |
| Total reconciliation | 88.875 / 99.245 | 91.024 / 100.481 |
| Capture | 21.930 / 24.942 | 23.037 / 24.455 |
| Prepare + write admission | 0.514 / 1.836 | 0.515 / 1.741 |
| Publish | 65.090 / 73.829 | 64.147 / 73.271 |
| Residual | 2.271 / 2.646 | 2.276 / 2.942 |
| Four evidence queries, consumed through Rows.Close | 2.786 / 3.748 | 3.226 / 4.523 |
| Publish driver Exec (included above) | 42.288 / 51.102 | 40.955 / 49.529 |
| GC pause delta bracketing the call | 0.400 / 0.630 | 0.419 / 1.352 |
| Go TotalAlloc delta, bytes | 7,387,984 / 7,393,464 | 7,396,080 / 7,402,288 |
| Go Mallocs delta | 144,403 / 144,421 | 144,434 / 144,461 |

Each call captured and published once; there were no conflict retries. All 60
calls consumed exactly four evidence queries / 12 rows and reused 12 entries.
Capture made 51 queries returning 596 rows; publication made 53 queries returning
843 rows and the same 521 no-effect write attempts. The remaining household
query accounts for the full-call 105 queries / 1,440 rows.

Paired bounded-minus-baseline mean was **+0.558 ms** for evidence SQL (SD 0.729),
**+0.715 ms** for capture (SD 2.251), **-0.244 ms** for publication (SD 12.609),
and **+0.420 ms** total (SD 12.040); bounded was faster in 16/30 total pairs.
The evidence query has a small measured penalty on this small fixture, not an
observed 79 ms direct cost in this run. Publication dominates both paths.

There is measurable slot sensitivity in the common write path: first/second
publication mean was 58.787/67.181 ms baseline and 58.183/67.298 ms bounded.
Both paths still occupy each slot 15 times. GC cycles ranged 2–5 baseline and
2–4 bounded; the largest bracketed stop-the-world pause was 0.655/1.840 ms.
These pauses do not explain the old 79 ms difference, but this does not measure
concurrent GC CPU cost or prove what happened in the old run. No forced GC,
outlier removal, revised budget or alternate default was used.

Recommendation: retain the original activation block. The controls rule out a
mismatched correctness fix and differing SQL/result counts. The large old gap
was not reproduced under instrumentation; common write timing and heap/order
variation are plausible contributors, not a demonstrated root cause. No clear
production patch for that tail is justified. If activation is reconsidered,
retain stage/GC data and obtain controlled target-platform repetitions and
scheduling evidence. Do not skip the common source recheck/write protocol to
make this benchmark pass, and do not start C1 as part of this investigation.

## Stronger independent publication regression

`TestReviewReentryEpisodeFullReplayAcrossMidnight` now clones the acknowledged
snapshot separately for full/baseline/bounded, actually publishes after midnight,
and checks literal expected dates and exact detection/acknowledgement instants
(including the 345 ns acknowledgement component), the new episode ID and all
existing audit-event IDs/keys/payloads/timestamps. Expected revisions are
**2 → 3 → 3 → 4 → 4**: acknowledged, next-day evidence update, repeat, legitimate
recovery, repeat recovery. Event counts are **2 → 2 → 2 → 3 → 3**.

Its local six-day rolling fixture requires all six true votes. A public
backdated cost adjustment recorded after re-entry changes the old October 6
trigger vote from cost 100 to 300, before the new epoch opened. October 7 stays
true after re-entry resets cost to 100. The test requires the corrected October 6
predicate false and latest predicate true, but the six-slot policy false: exactly
five true / one false / zero unknown votes. The test directly requires `recovered`, no current/open episode, one new
resolution event, preserved exact ID/detection/acknowledgement fields (including
inside the resolution payload), and no further revision/event change on repeat.
These expected assertions do not call the shared full-lifecycle oracle. The
other existing five-of-six rolling regression remains unchanged.

## Follow-up validation and reproduction

- Full/durable oracle and repeated-epoch selected tests passed (5.561s at the
  diagnostic source); the selected three-test race run passed (135.347s).
- The final stronger October-6 correction/publication regression passed in all
  three modes (0.351s) and under the race detector (8.837s).
- Application vet, gofmt and staged diff checks passed. No production/API/build
  change requires a new native binding artifact. The original native GTK/WebKit
  blockers remain; parent owns remote CI at the updated PR head.

Timing ran serially before these validation jobs. The diagnostic source precedes
PR112's separately authorized merge; its immutable code/data provenance is
retained. The FX snapshot-bound change is not included in these timings.

```bash
GOMAXPROCS=4 NW_REVIEW_PERF_EVIDENCE=1 go test -count=1 -v \
  ./internal/application -run '^TestReviewEpochStageDiagnostics$'
go test -count=1 ./internal/application \
  -run '^TestReviewReentryEpisodeFullReplayAcrossMidnight$'
go test -race -count=1 -timeout=10m ./internal/application \
  -run '^TestReviewReentryEpisodeFullReplayAcrossMidnight$'
```

[Raw log, sample CSVs, distribution CSV and control hashes](performance-p0-epochs-2026-10-19/diagnosis/)
retain all samples. Median/p95 use nearest rank; SD is sample SD of paired
bounded-minus-baseline differences. Counts are returned rows / driver attempts,
not total visited source rows. All tests use isolated synthetic data; no native
app, real ledger, provider request, cloud backup or credential data is involved.
