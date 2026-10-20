# Dense historical series: current-main validation

This follow-up supersedes the deferred-status paragraph in the
[2026-10-19 prototype record](performance-dense-series-2026-10-19.md). The
optimization is proposed in [PR #133](https://github.com/MnemosyneLab/NestworthGo/pull/133).
Baseline source is current `main` at `47c31ca88f1527f550e06f0c43d8f8854f3c0b08`;
candidate source is `f2fee931fa3d5041311468534ccb196332696067` (before this
evidence-only follow-up commit).

## Scope and correctness

The four-arm resulting-endpoint union binds every activity ID once through a
`requested_activity` VALUES CTE. Hydration batches are 900 IDs. Each of the
five hydration statements binds at most those 900 IDs and has no additional
household parameter; review hydration also binds only the activity IDs. This
is below SQLite's historical 999 host-parameter ceiling, including the older
SQLite limit, while the project's bundled modernc SQLite is 3.53.0. The
implementation does not alter financial formulas, source selection, lifecycle
or read-transaction boundaries.

Regression coverage retains the old four-IN-set query as an independent oracle
and compares all endpoint fields, including exact decimal text. It exercises
all four union arms, an absent ID, the 900/901 and 1800/1801 chunk edges,
cancellation, one read snapshot across a concurrent writer, tied activity
ordering, and replay of absolute observations across chunks. A full SQLite
package test and the dense application entry/replay tests passed locally.

## Matched local performance sample

The user requested ten repetitions. We analyzed the first ten alternating
baseline/candidate pairs from the source-verified 14-pair local run; the
remaining four completed pairs are retained but excluded from these summary
statistics. Earlier incomplete/invalid pilot logs are also retained in the
evidence folder and explicitly marked as non-evidence. See
[`current-main-paired-final3`](performance-dense-series-2026-10-20/current-main-paired-final3)
for the raw logs, all 14 samples, and the exact analyzed ten-pair subset.

Both test binaries were built from separate worktrees: baseline `47c31ca`,
candidate `f2fee93`. The profile test source was identical in both. Their
SHA-256 digests are recorded in `provenance.json`. A same-database source check
confirmed 623 counted SQL calls for baseline and 273 for candidate, with the
same 86,433 facts, 13,513,678 bytes, 1,000 leaves and 31 output points.

| Timed full service call | Current main | Candidate |
| --- | ---: | ---: |
| n | 10 | 10 |
| Median | 5.222 s | 2.902 s |
| Nearest-rank p95 (sample 9) | 5.397 s | 3.067 s |
| Min / max | 5.020 / 5.552 s | 2.707 / 3.153 s |
| Median counted SQL calls | 623 | 273 |
| Median allocated bytes | 1,112,066,212 | 1,107,275,008 |

Median latency improved 44.4% and nearest-rank p95 improved 43.2% in this
matched run. All ten paired samples had identical fact, byte, leaf and point
counts. The elapsed timer covers the full 31-day service call and result
consumption; fixture creation, read-only verification, process startup and GC
are outside it. This is an isolated Linux synthetic-database result with warm
OS file data and fresh processes/connections, not a native macOS measurement or
a cold-storage claim. The previous 30-pair Linux result is historical evidence
from a different host and is not used for these figures.

The host was Linux x86_64 on an AMD EPYC 9V74, CPU quota 4, Go 1.26.0, modernc
SQLite 3.53.0, and GOMAXPROCS 4. The synthetic database SHA-256 is recorded in
`provenance.json`; no ledger or provider credentials were used.

## PR checks and timing review

The pushed candidate check run `39356706774` completed successfully in 5m12s
(02:02:07–02:07:19 UTC). Recent comparable runs were PR #130 run `39355709532`
at 4m47s and PR #129 run `39350454742` at 5m16s. The current run is 25s longer
than #130 and 4s shorter than #129; this is not a material overall increase.
The longest relevant Go job was 196s versus 120s for #130, 171s for #129, and
214s for #128 (`39277492480`), within the recent observed range. These job
durations include setup and runner variance. GitHub exposed the Go timing
artifacts, but downloading them returned HTTP 403 and `gh run view --log`
returned no step log, so an exact test-step attribution is unavailable.

The added replay boundary test took 4.716s locally. The 30-pair performance
experiment remains a local opt-in activity; it is not run by ordinary PR
checks. The opt-in profile test skips immediately without its explicit
environment variables. Race and scheduled perf remain in the repository's
separate daily perf workflow; no workflow was edited or dispatched for this
task. Native macOS build/packaging and UI acceptance remain unmeasured here.

## Reproduction

Build the profile test binary in each worktree using Go 1.26.0 and the same
`historical_series_profile_test.go`, then run the committed
[`run_paired.py`](performance-dense-series-2026-10-20/run_paired.py) with the
two binary paths and fresh isolated evidence/database paths. It defaults to
ten alternating pairs. See `provenance.json` for the exact binaries, fixture,
host and commands used for this run.
