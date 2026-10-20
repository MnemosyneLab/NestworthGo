# Clock-only profile and compact origin baselines

This is a bounded follow-up to merged PR129, based on main
`784eacc8396885b95ae9b02bb4b8e4572d342208`. PR129's final head was
`641fa4a8b10cb416f0f4563ab89a3ea94d127177`; their trees are identical.
The user's stop preserved a perf-only profiler commit and two uncommitted files.
Work resumed only to validate and finish those changes if useful. It does not
resume broad stage-two work or implement the separate D1/D2/enablement task.

## Scope and correctness argument

The profile isolates a large repeated allocation in
`projectReviewHistoricalDayFromPortfolioWithQuoteSeries`: every daily input
builds an origin map containing complete `HistoryOriginComponent` values.
Consumers only read membership, `Quantity` and `UnitCost`.

The change stores a private `reviewOriginCostBaseline` with those two pointer
fields. It preserves the same filter, keys, last-matching-entry overwrite and
nil/missing behavior. The old complete component already contained these same
pointers; no pointer/value ownership changes. The map is still rebuilt for each
projection. Every daily input and cost replay is still computed. There is no
new cache, persistent fact/authority protocol, dependency/suffix planner,
changed financial formula or reduced preparation scope.

The only production file changed is `review_historical_inputs.go`:
the local map value and the private recursive `reviewCostLot` map parameter.
All three call sites are in that file. Regression coverage in
`review_historical_inputs_test.go` checks missing versus present-nil baselines,
nil cost, duplicate last cost, unrelated component kind and nil holding ID.
Existing cost/reversal/transfer/coverage and certified temporal/publication
tests remain the ordinary correctness oracles.

No repository/schema/connection/source/provider/clock/publication guard changed.
Production C3 remains default-off. Nothing here changes D1/D2 dispatch or enables
the C3 candidate; those belong to the separate task.

## Profile method

`review_clock_profile_performance_test.go` is perf-build-only. It reproduces the
existing synthetic matched clock-only fixture and exact-FX bootstrap codec.
The C3 arm starts from a fresh private-origin fixture; financial IDs/data match
the C2 arm within each pair. Full preparation is checked independently before
profiling; paired durable output is compared after publication.

CPU profiling starts after fixture/bootstrap/oracle work and covers one actual
`ReconcileInvestmentReviewsV2` call per arm. A repository wrapper embeds the real
repository so all optional capabilities remain available. It records capture,
publish and certified no-op durations and CPU labels without production hooks.
The residual is **outside repository calls**, including preparation and other
service work; it is not labelled pure preparation or causal attribution.

Allocation sampling is set once by `-memprofilerate=65536`. Cumulative alloc
profiles are taken before and after the call, with two GCs at both boundaries;
`pprof -sample_index=alloc_space -base before after` removes setup/oracle stacks.
The delta includes small profiling instrumentation allocations. MemStats byte
counts cover only the measured reconcile. CPU totals may exceed wall time
because multiple threads run. Profile times are diagnostic, not rollout samples.

The initial profile used local commit `79f67df`; after the user's merge it was
carried to main-based commit `7b269be`. Both have tree
`7cabe1d16a0720ce64c0c2ce1606bf92c8d44d5b`. The after profile used that same tree
plus the preserved patch SHA-256
`de788a048a60c92d338c681b6a2c0d3a97800489c469d870e8d10792fe11f2ab`.
The final commit/check manifest records the immutable shipped source.

## Concrete profile findings

Large fixture: 100 instruments, 20 rules, 30 history days, 100,000 quotes,
50,000 synthetic cash activities; official Go 1.26.1, SQLite 3.53.0, Linux amd64,
`GOMAXPROCS=4`. Both before and after pass the full/durable oracles.

| Profiled wall time | Before C2 | Before C3 | After C2 | After C3 |
|---|---:|---:|---:|---:|
| reconcile | 5.535s | 7.237s | 5.993s | 7.391s |
| capture | 2.373s | 2.204s | 2.659s | 2.335s |
| publication | 2.970s | 3.246s | 3.114s | 3.401s |
| outside repository | 0.191s | 1.783s | 0.221s | 1.649s |
| C3 confirmation | — | 0.004s | — | 0.005s |

The initial profile supports the earlier timing-partition hypothesis: preparation
accounts for 1.75s sampled CPU, compared with 0.19s for C2. The origin assignment
line accounts for about 540ms cumulative CPU and 1,278.91MB sampled flat allocation.
After narrowing the value, the line is about 270ms and 325.15MB; full preparation
is 1.66s sampled CPU. These samples overlap other cumulative stacks and GC and
must not be added as independent wall costs.

| Cumulative allocation for one measured call | Before | After |
|---|---:|---:|
| C2 MemStats TotalAlloc delta | 2,009,120,736 bytes | 1,977,709,872 bytes |
| C3 MemStats TotalAlloc delta | 3,524,081,984 bytes | 2,549,901,776 bytes |
| C3 sampled preparation allocation | 1,802.14MB | 871.69MB |

C3 cumulative allocation drops 974,180,208 bytes, about **27.6%**, without changing
62,000 prepared pair-days, 8,242 executed statements or 6,242 affected rows.
C2 still prepares 2,000 pair-days. Allocation is not retained memory or RSS.
Whole-process VmHWM includes seeding, both arms and oracles (before 1,212,356kB,
after 1,233,804kB); no isolated per-arm peak reduction is claimed.

Absolute profiled latency is not demonstrated improved: capture/publication
times of unchanged work also vary. The local allocation benefit is concrete;
the profile alone does not show the clock-only regression resolved.

## Final ordinary checks and non-profiled measurements

Final production/test change: `484efcdaa008f56518df356f70674646d9163ab6`.
The profiler is separate commit `7b269be`; the production change can be integrated
without it. Resumed focused historical/certified/equality tests pass (19.963s),
followed by the full application (124.762s) and domain (0.031s) suites.
Internal vet/build, perf-build application vet, formatting and diff checks pass.
The interrupted pre-resume command is not counted as a successful check.

Matched non-profiled small n=3 medians, all full/durable oracles passing:

| Scenario | C2 | C3 |
|---|---:|---:|
| same-day | 36.951ms | 4.954ms |
| clock-only | 33.679ms | 44.653ms |
| append | 61.092ms | 52.524ms |
| correction | 52.567ms | 51.848ms |

Matched non-profiled large clock-only n=1 also passes every oracle:
C2 5.730892144s / C3 6.975385390s, still **+21.7%** for C3. Allocated bytes
are 1,977,640,496 / 2,550,354,928; C2/C3 prepare 2,000 / 62,000 pair-days.
Both execute 8,242 statements and affect 6,242 rows. Process-wide VmHWM is
961,044kB including both arms/setup/oracles; it is not per-arm RSS acceptance.
Large package elapsed time is 101.935s; small is 5.886s.

These results establish useful allocation reduction and unchanged output.
They do not establish 30-sample p95, native behavior, isolated RSS, or a controlled
before/after latency improvement. All before/after profiles and unprofiled logs
are separately identified in the
[manifest](artifacts/c3-origin-profile-2026-10-20/validation.json) and covered by
[checksums](artifacts/c3-origin-profile-2026-10-20/SHA256SUMS).

For the separate third-stage task, the reusable production changes are only:

- `internal/application/review_historical_inputs.go`: narrow local map values
  and the private recursive `reviewCostLot` argument; public signatures unchanged.
- `internal/application/review_historical_inputs_test.go`: six semantic edge cases
  plus the standard `slices` import.

The perf-only profiler is an independent optional integration. No dependency
planner, suffix selection, persisted fact reuse or activation change is included.

## Reproduction and acceptance boundary

```sh
NW_C3_PROFILE_DIR=/tmp/c3-origin-profile-large NW_C3_PERF_SIZE=large \
  go test -tags=perf -p=1 ./internal/application \
  -run '^TestReviewClockOnlyCPUAllocationProfile$' \
  -count=1 -timeout=12m -memprofilerate=65536 -v
go tool pprof -top -cum /tmp/c3-origin-profile-large/large-certified-cpu.pb.gz
go tool pprof -sample_index=alloc_space \
  -base /tmp/c3-origin-profile-large/large-certified-alloc-before.pb.gz \
  -top -cum /tmp/c3-origin-profile-large/large-certified-alloc-after.pb.gz
```

Use the isolated PATH/cache/module/temp settings in the evidence manifest.
Small profiler runs validate instrumentation only; their CPU sample counts are
too low for hotspot conclusions. Non-profiled matched measurements are recorded
separately. No data from a real household or live provider is used.

Non-profiled reproduction uses `NW_C3_CERTIFIED_EVIDENCE=1` and the existing
`TestReviewCertifiedPerformanceEvidence` harness: small size, n=3, `-timeout=8m`;
large size, n=1, run filter
`^TestReviewCertifiedPerformanceEvidence$/^large$/^clock-only$`, `-timeout=12m`.
Both use `-tags=perf -p=1 -count=1 -v` and the same isolated environment.

This follow-up does not complete stage-two acceptance or authorize activation.
Independent review, appropriate matched distributions, isolated per-arm RSS,
native Apple Silicon validation and the previously incomplete final race gate
remain separate. No intermediate race, timeout increase, CI edit/dispatch,
activation, merge or release occurs here. Further computational reuse belongs
to the separately authorized D1/D2 task, not this patch.
