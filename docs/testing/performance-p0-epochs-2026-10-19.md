# P0 PR-B follow-up: repeated epochs and small-ledger activation evidence

Later PR111 review: [64-cycle tail diagnosis and stronger publication regression](performance-p0-epochs-diagnosis-2026-10-19.md). The original distributions below remain unchanged.

Contract: [approved plan](../design/performance-optimization-plan.zh-CN.md),
PR-A/PR-B and §5.4. Base: PR110 squash merge
`3b0902e893cd3eff8a5a15a341c1e7385590d00a` (latest main, Wails beta.32).
PR110 was merged only after checking its exact approved head
`ec58baf87dc48b2be7afb54ea3f44371a7c2f0d4`, successful
[CI39196603950](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39196603950),
clean mergeability and no blocking reviews; normal squash controls, no bypass.

## Changes and correctness findings

The original bounded query correlated `MAX(consecutive_count)` with every
returned day. The production state primary key ends in `position_epoch_id`,
so retained closed epochs were repeatedly scanned. A materialized per-instrument
aggregate computes the maximum once, still anchored to the selected latest
stored date. No index, schema, financial arithmetic, public interface or
publication protocol changes.

Real command-path repeated full exits and re-entry exposed a pre-existing full
replay defect: same-day re-entry can establish an episode using yesterday's
closed votes; a later replay started at the epoch opening date, omitted that
established trigger and rejected the open identity. Full replay now includes
the existing trigger interval. The independent full oracle was not weakened.
The initial failing gate is retained as evidence, followed by its passing run.

The fixture publishes both the zero-position boundary and each re-entry, compares
both query captures and checks their prepared publications against independently
prepared full source,
and compares every durable projection/lifecycle/event field after publication
on separate clones. Existing episode/event IDs and acknowledgements stay exact;
only newly generated IDs are normalized by logical identity. Ordinary CI runs
eight full cycles, followed by no-op, clock-only, append and historical correction.
The opt-in distribution adds 64 real cycles (65 position epochs, 260 evaluation
states for four rules). Epoch operations and oracle/cloning are outside timed
samples; this does not claim the cost of writing/replaying those cycles improved.

## Measurement protocol

Before query change: `eb096bc2a55fcda31f498aa6a6ed3e4e8d341db6`
(test harness plus lifecycle fix, original bounded SQL). After query change:
`a949c01b618bb02b772665644d7a4b030fec634f`. Measurements use Go
1.26.0, SQLite 3.53.0, schema 19, Linux amd64, Xeon Platinum 8573C, four CPU quota,
GOMAXPROCS=4, 16 GiB memory and tmpfs synthetic databases. No provider requests,
real ledger, settings or cloud backups. All formal runs execute serially without
race, build or profiling work alongside them.

Each scenario's 30 paired samples starts from an identical `VACUUM INTO`
snapshot, alternating baseline/candidate order. Publication uses separate clones
and fresh Service/SQLite connections. Fixture UUIDs are generated once per run;
separate before/after runs share deterministic arithmetic/dates, not identical
UUIDs. Baseline/candidate comparisons within each run are exact shared snapshots.
"Warm" means a durable projection already exists, not controlled OS page caches.
Cold initial projection is n=1 per fixture and is not a cold distribution.

Reported p95 is nearest rank (sorted sample 29 of 30), with no outlier removal.
Microquery timings include QueryContext, typed decoding and Rows.Close. End-to-end
timings include capture, source fingerprinting, planning and atomic publication
with full source recheck. SQL row counts mean rows returned, not visits; the
scalar probe counts evaluation-state predicate evaluations only, not all VDBE
steps or all source work. Peak RSS is the whole process including fixture,
clones and oracle, not allocations per operation. Raw logs retain allocations,
SQL executions, writes/affected rows, selected rows, EXPLAIN and complete timings.

## Query stress (2 instruments, 4 input versions, 2,048 states per instrument)

This reduced SQL fixture uses the production primary keys and no extra state
index; it is separate from the real command-path lifecycle fixture below.
Timing uses seven stored slots per instrument. The calendar spans intentionally
contain missing dates, including Apia 2011-12-30; they are not counts of stored slots.

| History span (days) | Before baseline median/p95 ms | Before candidate median/p95 ms | After baseline median/p95 ms | After candidate median/p95 ms |
| --- | --- | --- | --- | --- |
| 30 | 5.677 / 6.186 | 11.329 / 14.028 | 5.658 / 6.861 | 5.029 / 5.573 |
| 365 | 8.014 / 9.050 | 11.239 / 13.520 | 7.973 / 8.662 | 4.866 / 5.704 |
| 1095 | 13.190 / 17.821 | 10.975 / 13.711 | 13.343 / 15.877 | 4.943 / 6.457 |

Each cell is n=30; both median and p95 use nearest rank. Epoch predicate
evaluations fall from 28,672 to 4,096 for 14 returned days. The date-key
predicate counts remain 216/2,688/8,080 baseline versus 50 bounded for
30/365/1,095 days. These are separate probes, not a count of all query work.
State work remains linear in retained epochs once per requested instrument;
old input policies can still add date-key visits. No history-independent total
capture claim is made. The retained per-instrument prepared alternative is
also in the raw logs; it is not selected adaptively by workload.

## End-to-end paired distributions after the query change

All fixtures use one account, 30 days and no cash-only/mixed background activity.
S1 has ten holdings/instruments, 300 generated plus ten baseline quotes and
1 or 20 rules. E64 has one holding/instrument, four rules, 30 generated plus
one baseline quote, and 128 real position adjustments over 64 complete cycles.
All also include synthetic FX references; configuration manifests and actual
epoch/state counts are in the logs.

| Workload | Scenario | Baseline median/p95 ms | Candidate median/p95 ms | p95 difference ms (%) |
| --- | --- | --- | --- | --- |
| small-r1 | no-op | 45.094 / 59.460 | 44.696 / 57.004 | -2.456 (-4.1%) |
| small-r1 | clock-only | 45.382 / 50.101 | 45.602 / 51.117 | +1.016 (+2.0%) |
| small-r1 | append | 66.982 / 73.324 | 66.341 / 75.673 | +2.349 (+3.2%) |
| small-r1 | correction | 64.583 / 80.852 | 64.748 / 74.948 | -5.904 (-7.3%) |
| small-r20 | no-op | 171.269 / 185.211 | 140.297 / 157.800 | -27.411 (-14.8%) |
| small-r20 | clock-only | 230.611 / 291.093 | 193.118 / 250.485 | -40.608 (-14.0%) |
| small-r20 | append | 882.557 / 1532.922 | 900.661 / 1770.799 | +237.877 (+15.5%) |
| small-r20 | correction | 815.560 / 1256.327 | 835.089 / 1411.954 | +155.627 (+12.4%) |
| real-epochs64 | no-op | 103.206 / 173.338 | 107.736 / 252.269 | +78.931 (+45.5%) |
| real-epochs64 | clock-only | 107.974 / 220.285 | 105.176 / 174.809 | -45.476 (-20.6%) |
| real-epochs64 | append | 126.211 / 363.125 | 128.963 / 297.374 | -65.751 (-18.1%) |
| real-epochs64 | correction | 113.345 / 172.848 | 116.625 / 214.460 | +41.613 (+24.1%) |

n=30 per scenario/path; 120 repetitions of four fixed scenarios per workload,
not 120 distinct correctness cases. Before/after distributions for both capture
and full reconciliation, all raw samples, single initial baseline cold projection
times and whole-process RSS are retained in [raw evidence](performance-p0-epochs-2026-10-19/)
and its [summary CSV](performance-p0-epochs-2026-10-19/distributions.csv).
Append and correction capture no reusable evidence and execute the same full
computation on both paths. Timing differences there are not query speedups.
No p95s are added or multiplied across phases/rules.

The R20 append (+15.5%, +237.877 ms) and correction (+12.4%, +155.627 ms)
p95 regressions are **retained review triggers**, not waived
samples. Across the 30 append paired full reconciliations, candidate-minus-baseline
mean was -2.211 ms, sample SD 449.002 ms; the candidate was
faster in 16/30 pairs. Its two slowest samples were 1,927.249 and 1,770.799 ms,
with 921.190 and 1,016.249 ms inside driver Exec, respectively. Both still used
179 queries, returned 1,646 rows and executed 12,835 writes with 645 affected
rows. Neither loaded reusable evidence. These observations locate substantial
variation in the common publication/write path; they do not prove a stable
query-caused regression or dismiss the observed tail. All samples remain in
both the raw logs and the p95 calculation. Additional isolated/native profiling
is required before treating this gate as closed. No repeated run was used to
replace the distribution or obtain a more favorable p95. Correction paired
candidate-minus-baseline mean was +65.458 ms, sample SD
501.926 ms; candidate was faster in 14/30 pairs, also with no reusable query.

E64 also retains material no-op (+45.5%, +78.931 ms) and correction
(+24.1%, +41.613 ms) p95 review triggers. No-op capture alone was
24.989/37.272 ms baseline median/p95 versus 26.786/51.731 ms candidate.
Its paired full-reconcile mean difference was +14.518 ms, sample SD 56.462 ms,
with candidate faster in 14/30 pairs. Unlike the mutation controls, no-op
does execute the different evidence queries; this result cannot be cleared by
pointing to their common fallback. E64 correction had no reuse, mean paired
difference +3.915 ms, sample SD 29.761 ms, candidate faster in 12/30 pairs.
The reduced SQL improvement therefore does not establish an epoch-workload
end-to-end tail improvement. Preserve the baseline until this is investigated
and reviewed.

## Rollout decision and remaining stages

Keep `NewRepository` on the original baseline query. Linux evidence supports
reviewing the improved bounded candidate, and the small one-rule workload should
be judged using absolute milliseconds as well as percentages. It does not supply
the outstanding target-platform native latency/RSS evidence or the complete
workload matrix, and the material R20 mutation and E64 tail review triggers above
remain open. No adaptive small-ledger branch, new threshold, budget waiver,
or automatic default activation is introduced. The per-instrument alternative
is faster in this two-instrument/high-epoch reduced fixture; changing request
shape needs its broader workload evidence, not a decision from this microquery
alone. The grouped chunk aggregate still has epoch-dependent work/temp grouping;
its predicate-count improvement is not a constant-memory or total-I/O proof.

No new 3s cold / 500ms warm claim is made. Initial cold figures are single baseline
fixture-preconditioning samples; there is no separate candidate cold distribution
or controlled OS-cold/startup measurement here. Whole-process paired-run RSS cannot
isolate each query path's peak. Earlier reference R30/R365 timings in the PR110
report remain historical evidence at their documented source, not new results.

The next dependency stage remains **C1 tracking schema**: complete source/writer,
OLD/NEW, REPLACE/UPSERT, FK, restore/import/epoch and verifier audit before any
schema/journal implementation. C2 shadow-token and C3 CAS/no-op remain separate
reviewed stages. §5.4 temporal closure, future-effective activation without DML,
clock rollback, concurrent writes, crash/restore/restart, timezone/cutoff and
later scoped GUI/MCP parity gates remain blocking for those stages. No source
recheck is removed and no no-op shortcut or temporal certificate is enabled.
F4/F15 and credentials remain outside this change. Stop this focused draft for
parent review; no merge or release authorization applies to it.

## Reproduction

Use the source commits above with Go 1.26 and a writable module/build cache.
Do not run race/build/profile jobs alongside these measurements. All data is
created under `testing.T.TempDir`; no application launch or real ledger is used.

```bash
export GOMAXPROCS=4 NW_REVIEW_PERF_EVIDENCE=1
NW_REVIEW_PERF_EPOCHS=2048 go test -count=1 -v ./internal/infrastructure/sqlite \
  -run '^TestReviewReusableWindowEquivalent$'
export NW_REVIEW_PERF_ACCOUNTS=1 NW_REVIEW_PERF_INSTRUMENTS=10 \
  NW_REVIEW_PERF_HOLDINGS=10 NW_REVIEW_PERF_DAYS=30 \
  NW_REVIEW_PERF_QUOTES=300 NW_REVIEW_PERF_ACTIVITIES=0
for rules in 1 20; do
  NW_REVIEW_PERF_RULES="$rules" go test -count=1 -v ./internal/application \
    -run '^TestReviewPairedPerformanceEvidence$'
done
NW_REVIEW_PERF_INSTRUMENTS=1 NW_REVIEW_PERF_HOLDINGS=1 \
NW_REVIEW_PERF_QUOTES=30 NW_REVIEW_PERF_RULES=4 \
NW_REVIEW_PERF_EPOCH_CYCLES=64 go test -count=1 -v ./internal/application \
  -run '^TestReviewPairedPerformanceEvidence$'
```

## Validation at the final Go source

| Check | Result |
| --- | --- |
| `go test -count=1 -p=1 ./...` | Exit 1 solely because native `cmd/nestworth` cannot build without GTK4/WebKitGTK6 headers. Tests in all 38 other tested packages passed; application 127.858s. No unit assertion failed. |
| `go test -race -count=1 -timeout=10m -p=1 ./internal/application ./internal/infrastructure/sqlite -run Review` | Passed: application 238.266s, SQLite 27.958s, no race reports. Includes names containing Review, broader than PR110's prefix-only local scope. |
| `go vet ./internal/...`, `go build ./internal/...` | Passed. |
| Full vet / `go build ./cmd/nestworth` | Blocked by the same missing native headers. |
| gofmt and `git diff --check` | Passed. |
| Frozen frontend install and lint, pnpm 12.2.1 / Node 24.19.0 | Passed. No frontend or lockfile changes. |
| Wails beta.32 binding generation | Exit 1: GTK4/WebKitGTK6 pkg-config failure. Dependent binding freshness, frontend typecheck/test/build were not run locally after that failure. CI must complete them with native dependencies installed. |

See the raw validation logs and `validation-status.log`; their nonzero native
exits are not reported as successful checks. Full remaining race partitions
and native-dependent checks belong to the exact-head CI run linked in the draft
PR, which the parent monitors. No CI success is inferred before completion.
No native app was launched here. Native Mac acceptance still requires the
established `com.nestworth.dev` identity and isolated synthetic database/settings.
