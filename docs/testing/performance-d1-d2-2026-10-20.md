# D1/D2 dependency/suffix implementation and validation

Implementation head: `f122578` (see the manifest for full SHA). Base:
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
pass. Native desktop build remains blocked by those dependencies.

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
| Same-day confirmation | 39.766ms | 10.036ms |
| Clock-only | 38.110ms | 82.075ms |
| Quote + FX append (full fallback) | 57.649ms | 72.190ms |
| Quote correction | 52.954ms | 83.958ms |
| Quote-only append | 56.369ms | 88.088ms |

All independent oracles pass. Clock/quote-only append recompute 10 pair-days
and reuse 300. Correction recomputes only the affected instrument's 25/24/23-day
suffix and reuses 275/276/277. Mixed quote+FX append honestly recomputes all 310.
Complete publication/policy replay still contains the full date set. Reduced
financial recomputation **does not establish a latency win**: clock-only is
115.4% slower in this small diagnostic. Extra source/proof/prefix/serialization/
publication work remains; no isolated CPU attribution is claimed.

## Outstanding final validation

Large paired diagnostics and the single final relevant race invocation are
in progress when this draft is created. Results will be appended to this PR.
Thirty-sample p95, isolated per-arm peak RSS, native Apple Silicon latency,
native app/package/restore acceptance and the previously incomplete whole
application race gate remain open. Whole-process VmHWM includes fixture seeding,
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
