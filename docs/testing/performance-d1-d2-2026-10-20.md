# D1/D2 dependency/suffix implementation and validation

Historical pre-PR133 measurements. See [integrated clock-omission evidence](performance-d1-d2-clock-omission-2026-10-20.md) for current source, proof and matched separate-build results.

This report preserves the initial checkpoint. See [the follow-up evidence](performance-d1-d2-followup-2026-10-20.md) for current production results, CI timing and current race policy.

Implementation head: `647371973483574cd66eac39213ddae4ae119f57` (see the manifest for full SHA). Base:
`784eacc8396885b95ae9b02bb4b8e4572d342208`, PR129 merge. This is the
third high-level stage, D1/D2, not another interpretation of C3 as stage three.
The user's explicit activation authorization supersedes prior default-off
instructions. It does not establish stage-two acceptance or performance gates.

## Implemented behavior

[Design and proof](../design/review-dependency-suffix.md) describes quote
OLD/NEW dependency invalidation, exact authenticated prefix reuse and the final
pre-write digest fence. Normal desktop initialization selects
`WithReviewDependencySuffix`; GUI and MCP share that service. Rollback is
`NESTWORTH_REVIEW_FULL_RECOMPUTE=1` before launch, followed by explicit reconcile.
No ledger setting is modified. No schema migration, shared persistent financial
facts, G/H optimization, release, workflow edit/dispatch or merge is included.

Supported reuse is deliberately bounded to authenticated same-handle chains,
unchanged rule/handling/provider namespaces, and quote-only changes with complete
journal coverage and independent signed exact quote-bucket validation.
Clock advancement and quote insert/correction/backfill/delete/date/instrument
moves reuse the unaffected financial prefix. Policy and lifecycle still replay
from origin with the full three-valued daily sequence. This is not a compact
resumable lifecycle checkpoint or early-stopping implementation.

Non-quote/unknown dependencies (including FX, activities/corrections/transfers,
ownership, provider preferences/bindings), changed rules/handling, malformed
scopes, missing slots, journal gaps/floors, rollback and incompatible temporal
namespaces retain full preparation. Existing/reopened/restored/legacy files
cannot acquire certified historical authority. A stale staged suffix fails
atomically rather than being published through an uncertified fallback.

Separately reviewed production/regression commit
[`484efcdaa008f56518df356f70674646d9163ab6`](https://github.com/MnemosyneLab/NestworthGo/commit/484efcdaa008f56518df356f70674646d9163ab6)
was cherry-picked as `9f276ba` after parent review cleared it. It only replaces
per-day complete origin component map values with the two consumed pointer
fields; no profiler or other PR130 commits were imported. Final paired arms
both include that fix. PR130 CI completion is not asserted here.

## Correctness and ordinary checks

Independent forced-full captures rebuild inputs without prefix/checkpoint reuse.
Durable-output oracles compare all financial/daily/lifecycle/frozen payloads and
all three daily timestamps. Every existing episode/event identity and ack remains
exact; only newly allocated UUIDs map through complete semantic keys. The full
oracle uses the same additive lossless FX serializer while disabling certification
and incremental computation. Exact private FX rates are independently tested.

Coverage includes single/consecutive/rolling policies, unrelated instruments,
lookback corrections beyond the rolling window, quote append/backfill/delete/
instrument/date moves, acknowledged corrections, suppression expiry, DST,
activation of future positions, source rollback, changed rules, FX fallback,
missing rows, reopen/key loss and damaged scope metadata. Publication cases
include concurrent external source edits, provider ABA, handling/control changes,
floor changes, canceled context, final-clock cancellation, prefix/digest drift in
the final callback, disabled/direct publishers and failed checkpoint writes.
No new unsafe schema-forgery or security experiments were introduced.

The broad serial `GOMAXPROCS=4 go test -p=1 ./...` invocation passed every
internal package on the initial D1/D2 implementation, including application
141.031s and SQLite 33.044s. The command overall **failed** compiling
`cmd/nestworth`, because Linux lacks GLib/GTK4/WebKitGTK6/libsoup development
packages. A prior overlapping broad run failed the unrelated analysis cold
budget at 10.636s against its unchanged 10s limit; the serial application suite
passed that same test. No budget or workflow was changed.

After compact-map integration and the final prefix-digest change, the complete
focused certified/D1/D2/domain/SQLite matrix passed (application 34.091s,
SQLite 0.799s, domain 0.004s), with integrated historical-input/cost tests
6.086s and internal vet/build. The full broad suite was not rerun after that
integration; the exact scope of each log is retained. Formatting and diff checks
pass. After the final prefix batching change, the complete D1/D2 oracle matrix
passed again (12.156s), and internal vet/build passed.

[CI run 39353736515](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39353736515)
passed every check, including all three Go shards, both frontend shards, meta
and aggregate `test`, on exact code commit `647371973483574cd66eac39213ddae4ae119f57`.
This supplies broad integrated-head validation without duplicating it locally.
Local native desktop build remains blocked by the dependencies above; passing
Linux CI is not native Apple Silicon runtime/package acceptance.

## Final small paired diagnostics

Official Go 1.26.1, SQLite 3.53.0, Linux amd64, GOMAXPROCS=4, four-core CPU
quota. Synthetic fixture: 1 account, 10 instruments/holdings, 1 rule,
30 history days, 310 generated quotes, 100 cash activities. Three serial paired
samples alternate arm order, using identical source IDs/data and independent
pre-publication full/durable oracles. No other build/test/race runs concurrently.
Correction samples intentionally use different dirty dates (25/24/23 affected
pair-days); these are diagnostic medians, not an identical-workload p95.

| Scenario / median n=3 | C2 | Enabled D1/D2 |
|---|---:|---:|
| Same-day confirmation | 35.208ms | 9.033ms |
| Clock-only | 40.497ms | 80.361ms |
| Quote + FX append (full fallback) | 59.567ms | 74.958ms |
| Quote correction | 56.382ms | 79.590ms |
| Quote-only append | 56.345ms | 83.355ms |

All independent oracles pass. Clock/quote-only append recompute 10 pair-days
and reuse 300. Correction recomputes only the affected instrument's 25/24/23-day
suffix and reuses 275/276/277. Mixed quote+FX append honestly recomputes all 310.
Complete publication/policy replay still contains the full date set. Reduced
financial recomputation **does not establish a latency win**: clock-only is
98.4% slower in this small diagnostic. Extra source/proof/prefix/serialization/
publication work remains; no isolated CPU attribution is claimed.

## Final large paired diagnostics

The final serial run passes all independent full/durable oracles on the same
code commit. Fixture: 1 account, 100 instruments/holdings, 20 rules, 30 history
days, 100,000 generated quotes and 50,000 cash activities. These are individual
n=1 observations, not p95 or native acceptance.

| Scenario / n=1 | C2 | Enabled D1/D2 | Recomputed / reused pair-days |
|---|---:|---:|---:|
| Clock-only | 5.890865s | 10.241296s | 2,000 / 60,000 |
| Quote + FX append (full fallback) | 9.710322s | 8.880526s | 62,000 / 0 |
| Quote correction | 9.897613s | 10.391025s | 500 / 59,500 |
| Quote-only append | 10.398464s | 10.892903s | 2,000 / 60,000 |

Clock-only remains **73.9% slower**. Batching reduces the initial implementation's
2,647 queries to 667, versus C2's 625, but does not establish an elapsed-time
improvement. It returns 714,382 rows versus C2's 513,151; both execute 8,242
statements and affect exactly 6,242 rows. D2 clock-only cumulative allocation is
4,060,713,464 bytes versus C2's 1,977,624,568. These are total allocations, not
peak live heap or RSS. Correction executes 4,056 statements versus C2's 124,002,
affecting the same 156 rows; it still regresses about 5.0% in this diagnostic.
Quote-only append affects the same 6,342 rows in both arms.

Full source capture, independent quote-bucket checks, typed prefix decoding and
hashing, full policy/lifecycle replay, complete stage serialization and exact SQL
publication comparison all remain costs. These timings/counts do not isolate
CPU causality. Reduced recomputation and write elision are not accepted as an
end-to-end performance win. The draft remains open for that unresolved issue.

The final large package took 391.418s including fixture generation, clones,
bootstrap, all independent oracles and both timed arms. Whole-process VmHWM was
1,122,536kB, explicitly not isolated per-arm RSS. An earlier superseded large run
was stopped after prefix batching was implemented; a targeted check briefly
overlapped that old run, so its later timing samples are not final evidence.
The final small/large logs here were collected serially with no concurrent
build/test/race process. The earlier small log is retained under its source SHA;
no controlled cross-commit speedup is inferred from it.

## Final race and remaining acceptance

The single final relevant race invocation completed with **NOT PASS**:
domain 1.023s and SQLite 24.981s passed; application timed out at 600.032s
(the unchanged 10-minute budget) during
`TestReviewDependencySuffixFullDurableOracle/rolling/quote-and-quantity`.
There were 184 completed test/subtest pass events across the invocation and
no `WARNING: DATA RACE` message. This is not an application race pass or a
repository-wide race pass. The application timeout remains an uncompleted local/scheduled performance check; under the later user instruction it is not a normal PR readiness gate.
No intermediate/second race was run, no timeout increased, and no workflow
edited or manually dispatched. An executor reconnect lost the session registry,
but the original Go and application processes continued; complete JSON package
events confirm the final timeout outcome. Validation was not restarted.

The exact command, code SHA, package results and complete raw JSON log are in
[the manifest](artifacts/d1-d2-2026-10-20/validation.json) and
[race-final.jsonl](artifacts/d1-d2-2026-10-20/race-final.jsonl).
[SHA-256 checksums](artifacts/d1-d2-2026-10-20/SHA256SUMS) cover retained artifacts.

Thirty-sample p95, isolated per-arm peak RSS, native Apple Silicon latency,
native app/package/restore acceptance and the previously incomplete whole
application race validation remain open; race is not a normal PR readiness gate. Whole-process VmHWM includes fixture seeding,
both arms and oracles, and allocatedBytes is cumulative allocation, not RSS.
This change is enabled in code as requested; neither the regression nor missing
acceptance is converted to a pass.

Reproduce final small evidence with isolated synthetic temporary databases:

```sh
NW_D2_SUFFIX_EVIDENCE=1 NW_C3_PERF_SIZE=small NW_REVIEW_PERF_SAMPLES=3 \
GOMAXPROCS=4 GOPATH=/tmp/nestworth-go GOCACHE=/tmp/nestworth-cache \
/tmp/go/bin/go test -tags=perf -p=1 ./internal/application \
  -run '^TestReviewDependencySuffixPerformanceEvidence$' -count=1 -timeout=8m -v
```

Large uses `NW_C3_PERF_SIZE=large`, `NW_REVIEW_PERF_SAMPLES=1`, the selected
`clock-only|append|correction|quote-append` subtests, and the existing 12m
large diagnostic budget. Logs are under
[artifacts/d1-d2-2026-10-20](artifacts/d1-d2-2026-10-20/).
