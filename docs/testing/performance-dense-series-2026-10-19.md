# Dense historical series: shared endpoint request IDs

**Deferred prototype checkpoint.** Parent has paused this independent investigation to prioritize C2b. The distribution/profile results are completed evidence; final whole-suite/race validation and draft review for this prototype have not been completed. No PR or merge is requested by this preservation commit.

Independent of C2, based on main `8058bf6e421cc291460a346125ec4fb0a1087926` (CI pause declaration included). Production implementation: `a613acbe55c2d24740786ad3aeedee11c5936305`. No schema/index/cache, financial formula, input budget or runtime deadline changes.

## Profile and change

Repeated ordinary CI failures reached the unchanged10s `TestHistoricalOverviewDenseEntrySingleAndComparison/series-endpoint-equivalence` deadline. Profiling only `HistoricalOverviewSeries`, after synthetic setup, attributed3.63 CPU seconds (56.81% of6.39 sampled CPU seconds) to `attachActivityResulting`. This is a single diagnostic profile, not a latency distribution.

The four endpoint UNION arms previously bound the same IDs four times. Historical hydration therefore used256 activities per chunk (up to1024 parameters), repeating child queries for each chunk. A shared `requested_activity(id)` VALUES CTE binds each ID once; the historical loader can use900 IDs with every statement at most900 parameters. It still hydrates effects, trade/dividend/product details and **all four resulting endpoint kinds**, then performs monetary replay once over the full economic order. No endpoint values are inferred or discarded.

The second single profile attributed1.39 sampled CPU seconds to endpoint hydration. Its total call elapsed3.304s versus5.602s in the first profile. Those two profiled samples only identify the hotspot; the separate distribution below supports the latency comparison. SQLite VM steps/table visits were not counted, so query counts are not presented as physical row-visit counts.

## Controlled distribution

Thirty samples **per path**, alternating order, one immutable synthetic database with identical IDs/values. Separate immutable test executables were built before measurement. Each sample starts a fresh process/SQLite connection, verifies the same readonly database before the timer, performs GC, then measures one full31-day service call through endpoint construction and all `Rows.Next`/`Scan`/`Close` consumption. Fixture creation, verification, GC, process startup and correctness checks are outside the reported call time. There is no concurrent build/test/race/profile during the distribution.

This is a warm-OS-data, fresh-process/connection experiment, not controlled cold pages or steady warm-process latency. Verification itself touches database data before timing. Linux x86_64, Xeon Platinum8573C, CPU quota4, GOMAXPROCS4, Go1.26.0, modernc SQLite3.53.0; isolated `/workspace` storage. Native Apple Silicon performance is unmeasured.

Fixture:100 accounts,9 instruments,900 holdings plus100 cash leaves (1000 leaves),9000 quote backfills plus9 baseline quotes,25000 activities,86433 candidate facts,13513678 counted input bytes,31 points,1100 endpoint rows. Fixed clock2026-09-01T12:00:00Z; range2026-08-01..31. Source construction is the existing `seriesBenchmarkFixture` without reduced scale. The before production code is main176bfb0 (identical financial code to8058bf6); the after production patch is the implementation above. Both use the identical committed opt-in harness.

| Full service call | Before | Shared IDs |
| --- | ---: | ---: |
| n | 30 | 30 |
| Median | 6.077017s | 3.595013s |
| Nearest-rank p95 (sorted sample29) | 6.899943s | 4.355965s |
| Min / max | 5.539521s / 8.079960s | 3.260884s / 4.407574s |
| QueryContext/QueryRow count | 623 | 273 |
| Median total allocated bytes | 1112076436 | 1107274852 |
| Median allocation count | 7832672.5 | 7817188 |

Median elapsed decreased40.8%, p95 decreased36.9% on this host/workload. Allocated bytes are cumulative Go allocation, **not** live heap or peak RSS. Every sample kept the same fact/byte/leaf/point counts. No result is a native UI timing or proof of canonical CI success.

## Correctness and retained limits

The old separately bound UNION remains a test-only SQL oracle. All endpoint fields, including decimal text/currency/quantity/cost, compare exactly. Coverage includes all four endpoint arms and an absent ID; a concurrent writer between chunks under one read snapshot; cancellation and a900-parameter maximum; and economic replay with backdated correction and absolute observations spanning the new900-ID boundaries. Existing dense single/comparison/series equality, readonly/no-write checks, cancellation and fact/byte/leaf/date/runtime budgets remain unchanged.

The new chunk-boundary fixtures use901/1801 rows so they continue to exercise multiple chunks. The full mandatory non-race fixture remains1000 leaves/25000 activities. Existing race-specific scale/deadline behavior is unchanged. Source revision/journal write overhead is outside this read optimization and remains visible.

Raw30+30 logs, profiler summaries, sample JSON, runner, binary/source-database hashes and checksums are in [the evidence directory](performance-dense-series-2026-10-19). No database is committed. Final local validation and intentional absence of a new CI run are recorded with draft delivery. Native build/packaging acceptance remains separate.

To reproduce, build the baseline and candidate with the same `historical_series_profile_test.go`, set `NW_SERIES_CPU_EVIDENCE_DIR` and an explicitly isolated `NW_SERIES_EVIDENCE_DB`, and run `TestHistoricalSeriesCPUProfileEvidence` alone. The stored runner records alternating fresh-process samples. Set `NW_SERIES_CPU_PROFILE=1` only for separate diagnostic profiling, not the distribution.
