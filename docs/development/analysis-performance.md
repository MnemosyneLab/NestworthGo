# Cold analysis performance investigation

Measured on 2026-10-01, against main `aa9cfb91673e90618633e5b63e3f335dea5ebc7e`.

## What the budget measures

`TestAnalysisPerformanceBudgets/cold-3y` times only `ComputeAnalysis`, the pure
in-memory analysis kernel. Input construction happens before the timer. It does
not time desktop startup, SQL, snapshot maintenance, service memo population, or
frontend rendering. The race build explicitly skips this subtest; a race pass
is not evidence that the cold budget passed.

The unchanged synthetic fixture has 500 USD balance components, each valued at
100, with no activities or quotes. The range is 2023-01-01 through 2025-12-31:
1,096 days, 548,000 result component-days, and 1,097 snapshots including the
opening day. Every benchmark iteration computes the entire result without a
service memo. All experiments used synthetic inputs and temporary test data.

## History and attribution

- `435499f` (2026-09-04) introduced the 3Y/500 benchmark with these dates and values.
- `a66f7f9` (2026-09-07) added the 3-second cold budget assertion. The fixture
  helper extraction did not change the timed workload.
- `f7b9080` (2026-09-10) added an exact attribution map to each component-day,
  adding 548,000 empty maps in this fixture. Its added activity scan can also
  matter for populated ledgers, but this fixture has no activities.
- `7b529a0` (2026-09-13) raised the Linux CI assertion from 3 to 10 seconds,
  without changing the fixture. Its recorded 4.10-second Linux run and untested
  Apple M3 Pro target are not comparable measurements across machines.
- Since that change, the fixture and core decimal operations have stayed the
  same. Later holding/quote and pre-creation-account paths do not execute for
  these fully populated base-currency balances. A later completeness pass adds
  only one traversal of the 1,096 daily results.

Same-machine measurements of `7b529a0` and current main overlap and have nearly
identical allocation counts. They do not support a claim of steadily increasing
cost since the budget was relaxed. The reported 10.183-second and 10.559-second
CI failures show a thin margin on those runners, not a longitudinal trend.
Growing real ledgers can still increase work: the kernel materializes each day
and component, and some activity/missing-value paths scan inputs repeatedly.
Those paths and actual application startup require separate representative
measurements; this optimization does not claim to resolve them.

## Profile and implementation

Go CPU and allocation profiles identify repeated decimal work and component-key
construction, rather than I/O. In the baseline profile, cumulative sampled CPU
was 24.6% in `NewSignedMoney` (18.0% in `RoundBank`), 9.7% in
`residualTolerance`, and 10.0% in `ComponentID.Key`. Background GC accounted for
27.0%. These are CPU sample shares, with overlapping call stacks, not additive
wall-time percentages. Whole-process profiles include fixture construction;
benchmark time and allocation metrics below exclude it.

The patch removes redundant work:

1. `NewSignedMoney` uses `Round(4)` when the decimal exponent is already at least
   -4. There can be no half-even tie in this case. This keeps the exact value
   and the original -4 result exponent while avoiding `RoundBank`'s remainder
   and tie checks. Finer scales still use `RoundBank(4)`. Currency validation
   and the overflow check before rounding stay in place.
2. Component keys use a fixed four-element scratch array instead of growing a
   slice, and each day reuses one key for its three map lookups. Key strings and
   identity rules are unchanged.
3. Residual tolerance uses exact decimal constants `2 × 10^-places` and
   `1 × 10^-9`, replacing repeated multiplication and float-to-decimal parsing.

No cache contracts, financial formulas, fixture sizes, budgets, or tests are
removed or relaxed.

## Repeated measurements

Environment: Linux amd64, Go 1.26.0, AMD EPYC 9V74, five visible virtual CPUs,
`GOMAXPROCS=5`, cgroup quota 4 CPUs and memory limit 16 GiB, default Go GC
settings. Dependency compilation was outside benchmark timing. Runs were
serial, with no concurrent tests, profiling, or builds. Each set used five
single-iteration samples; profiling was a separate run.

```sh
go test ./internal/application -run '^$' \
  -bench '^BenchmarkAnalysisColdComputeUpperBound$' \
  -benchtime=1x -count=5 -benchmem
```

| Revision | Seconds, all five samples | Median seconds | Median B/op | Median allocs/op |
| --- | --- | ---: | ---: | ---: |
| `7b529a0` | 5.168703, 4.868269, 4.935377, 4.882117, 5.375293 | 4.935377 | 3,236,455,360 | 92,717,358 |
| `aa9cfb9` | 4.926400, 4.705801, 4.931990, 5.435139, 4.855937 | 4.926400 | 3,236,446,728 | 92,717,173 |
| Optimized | 3.370403, 3.217514, 3.398980, 3.200456, 3.150460 | 3.217514 | 2,147,954,736 | 60,354,704 |

Median wall time fell 34.7%, allocated bytes 33.6%, and allocation count 34.9%.
Allocated bytes are cumulative allocation traffic, not peak resident memory.
This is useful additional margin against the unchanged Linux 10-second budget;
it does not establish a native Mac startup time or the 3-second M3 Pro target.

Profiles can be reproduced separately:

```sh
go test ./internal/application -run '^$' \
  -bench '^BenchmarkAnalysisColdComputeUpperBound$' -benchtime=1x \
  -cpuprofile=/tmp/analysis.cpu -memprofile=/tmp/analysis.mem \
  -o /tmp/analysis.test
go tool pprof -top -cum /tmp/analysis.test /tmp/analysis.cpu
go tool pprof -top -alloc_space /tmp/analysis.test /tmp/analysis.mem
```

## Correctness evidence

Regression tests compare the amount and exponent against the original
`RoundBank(4)` operation across exponents -16 through 8, positive and negative
half-even ties, exact values, and values close to the money limit. Separate
checks cover overflow before rounding, currency normalization/rejection, all
component-key branches, and residual tolerances around both currency floors.

A temporary differential probe computed the full 548,000-day result on main
and on the patch. It serialized every component-day and the remaining period
result in order, sorting map keys and including both the value and exponent of
every decimal (including `SignedMoney`'s private amount and currency). Both
SHA-256 digests were:

```text
c02fbef5a1d5f4f4f456343998843b3c3135c2e140f701f4d5834228e89b8732
```

The digest probe was outside timing runs and was not added to the regular test
suite. The existing analysis correctness, snapshot invalidation, FX, holding,
attribution, and boundary tests remain the broader regression coverage.
