# C3 completeness-lineage shadow (2026-10-19)

This is an opt-in diagnostic prerequisite for eventually considering bounded
reuse after a C3 fallback. It grants no reuse or publication authority. Both
measured arms still capture all financial sources, prepare every required date,
recheck full sources and temporal frontiers, and publish the full result.

Base: PR125 merge `2cb6c1cf2f09e22a8935aed612fc2c23c11d51c8`, following
PR123's corrected completeness bootstrap. Its automatic main
[check run](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39250584372)
succeeded on that exact merge. PR119 source checks at
`c3867c626d3b6a0cdbcf669a0047ff8d07d4215f` and PR121 temporal intersection
at `09d6276bb0f0be6ebdbfa4afacfcca843c1e623a` remain ancestors. CI files
and triggers are inherited unchanged from main; no workflow was dispatched.
While validation ran, main advanced to PR126's test-only merge
`8ca69f8baa660553e832f4299daaa0674e272677`. Its push-triggered
[check](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39253876232) run succeeded. An already existing, separately dispatched
[perf run](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39253933293)
also succeeded; it was inspected read-only, never dispatched by this task. It was incorporated unchanged into this branch at
`3861e63864087b909eaa44e8f259c3372f06549b`, before the final full internal
suite, vet and build checks. It removes two unrelated test fixtures; no
production, lineage-test or benchmark sources changed. The paired measurements
and focused race binary use `cdaed04`; provenance records the source per check.

## Capture, preparation, and publication

`WithReviewCompletenessLineageShadow(observer)` returns a separate repository
configuration. Normal initialization does not enable it or C3 confirmation;
`WithFullReviewRecompute()` disables both candidates. The observer emits only
phase and verdict strings. It is synchronous, must be concurrency-safe, and
must not reenter the database. When composed with C3, this observes fallback
and direct-capture publications; a successful existing no-change confirmation
continues to bypass capture.

Within the capture read transaction, the shadow requires an existing
`full-review-projection-v1` certificate, verified schema cookie/PRAGMAs/C1
tracking, matching source token and full historical fingerprint, valid source
floor and policy namespaces, the current clock session, unchanged control,
rules and review metadata, and the exact six-table cardinality manifest.
Disabled/archived rules conservatively decline this first shadow protocol.

An expired certificate is considered only when at least one rule's closed
civil date advances, no rule rolls back, and all other prerequisites hold.
An expired intra-day activation declines. This does not prove arbitrary
transitions safe: a full current publication remains mandatory.

The capture retains narrow daily keys, fingerprints and result truth/reason
for the current rule namespaces, a seal of those rows plus lifecycle metadata,
and a separate bounded window. It does not load every historical daily JSON
payload. `ReusableDailyEvidence` on the actual capture stays empty and
`TemporalFullRecompute` stays true, including direct repository callers.
Unreadable hypothetical window data declines the shadow while preserving the
full path; cancellation remains an error.

The freshly prepared full publication independently enumerates every required
previously closed date. It compares input/result keys, fingerprints, truth and
coverage reasons, and checks typed window payloads plus the trailing policy
anchor. A missing old date cannot pass by substituting another date with the
same row count. An incremental publication or one missing/duplicating historical prefix dates
cannot act as the prefix oracle. Complete new-day and lifecycle agreement is
checked separately by the independent hypothetical-plan/full-oracle comparisons
in the tests and measurements.

Before writes, under the existing SQLite immediate write reservation, the
publisher repeats prerequisite validation, the binding/seal, and bounded
window reads. Changes to floors, derived rows, window payloads or lifecycle
state decline the diagnostic. Existing financial, control, rule and handling
conflicts still reject the actual publication. The final clock/frontier fence
runs **after** all new reads. Publication continues to persist
`min(captured boundary, rechecked boundary)` from PR121. Projection, lifecycle,
control and temporal writes retain their existing atomic transaction.
`lineage_validated_shadow` is emitted only after a successful commit; a failed
or cancelled transaction emits `authoritative_guard_rejected`.

No new durable lineage format, schema, UI, bound service, provider access,
cache, dirty suffix planner, write trigger or CI configuration is introduced.
Repeated synthetic experiments can combine the existing opt-in C3 repository
with this shadow; a default C2 publication continues to clear C3 proof as it
did before this change.

## Correctness boundary

The focused regressions cover single/consecutive/rolling policies; full
publication and hypothetical bounded-plan agreement; missing old history;
same-count key substitution; changed old fingerprints/truth; damaged window
JSON and policy anchors; missing/unknown proof, tracking and temporal policy;
changed rule namespaces/revisions/metadata, handling and providers; broken
journals/triggers/PRAGMAs; source epoch reset, rollback and restart; future cash
and quantity activation at T−1ns/T/T+1ns; concurrent count/fingerprint/window/
lifecycle/floor/source/handling changes; a second connection blocked by the
write reservation; cancellation before entry and at the final clock; exact
temporal-boundary rejection; and injected publication failure after staging.
Rejected writes preserve both projection rows and the temporal certificate.
Successful repair scenarios are compared with an independently opened cloned
database and forced-full recomputation. The persisted-row comparison retains
financial and lifecycle columns and identifiers, normalizing only `computed_at`,
`evaluated_at` and `revised_at`; session-specific temporal certificates are
tested separately by atomicity and interval cases. The retained PR123 bootstrap
and PR121 intersection regressions remain required.

This protocol establishes diagnostic necessary conditions, not a durable
authority to skip work. In particular, the narrow seal does not authenticate
all archived JSON or lifecycle metadata against arbitrary preexisting tampering.
The seal detects changes since capture; the publisher does not execute the
hypothetical new-day/lifecycle plan for a second full runtime comparison.
Scalar fingerprint equality does not independently prove those payloads' integrity. The full publisher's
existing conditional updates are unchanged; this change does not claim to
repair every possible manually damaged payload or remove unexpected archive
keys. Concurrent in-memory provider changes are not separately sampled by the
publisher; the staged provider is compared with the previous published basis.
Those limits, plus reviewed namespace/temporal coverage and stronger independent
agreement evidence, must be resolved before any subsequent reuse authority is
proposed. A validated shadow verdict alone is insufficient.

## Reproduction and results

Final implementation and test source:
`cdaed0476311c46d73fc2156fa54b131c3a2bc9d` (following the initial shadow
implementation `b07e08e6f8fa06b997dd593ef66e0bda13584333`).
Raw logs, provenance and checksums are in
[`performance-c3-lineage-shadow-2026-10-19/`](performance-c3-lineage-shadow-2026-10-19/).
All fixtures are isolated synthetic databases. No app, provider network, real
ledger or credentials are used.

Reproduce the measurements from that source (Go1.26.1, Linux amd64):

```sh
export PATH=/tmp/go/bin:$PATH
export GOCACHE=/tmp/nestworth-c3-cache GOMODCACHE=/tmp/nestworth-c3-mod
export GOPATH=/tmp/nestworth-c3-path TMPDIR=/tmp/nestworth-c3-scratch GOMAXPROCS=4
mkdir -p "$TMPDIR"
NW_C3_LINEAGE_EVIDENCE=1 NW_C3_LINEAGE_SAMPLES=3 \
  go test -tags perf -p 1 ./internal/application \
  -run '^TestReviewCompletenessLineagePerformanceEvidence$' -count=1 -v -timeout=15m
```

`NW_C3_PERF_SIZE=small` or `large` selects one size. Small uses1 account,
10 holdings/instruments,1 rule,30 history days,310 quotes and100 cash activities.
Large uses1 account,100 holdings/instruments,20 rules,30 history days,100,000
quotes and50,000 cash activities. Raw logs include fixture configurations, the fixed clock, generator details
and UUID/clone provenance.
Each arm has a fresh cloned synthetic database, an excluded full C3 bootstrap,
a one-day clock advance, and explicit GC before the timed region. The arms
alternate order. Both time direct capture/prepare/publish; the no-change
confirmation probe, fixture creation, bootstrap and independent oracle checks
are excluded. Correctness checks and measurements run serially, with no
competing Go jobs. These are warm-process diagnostics, not cold or native timings.

Go total allocated bytes are a `runtime.MemStats.TotalAlloc` delta across the
same region, including SQL instrumentation. They are not live heap, peak memory
or RSS. Incidental cleanup `VmHWM` lines cover the whole test lifetime,
including fixture generation, clones and oracles; they are not an arm comparison.
SQL Query-to-Rows.Close aggregates include row consumption and Go work;
they are not pure database execution times. Three samples support raw values
and a median only, with no p95 estimate.

The initial `b07e08e` benchmark safely declined both sizes with
`result_prefix_missing_or_changed`. It exposed a decoding mismatch: SQLite
stores unknown truth as `"unknown"`, while `domain.ReviewUnknown` is empty.
The shadow's read adapter now maps that sentinel explicitly, preserving the
existing full publisher. A three-way true/false/unknown regression fails on
the previous adapter only for unknown, then passes with the fix. The
`before-truth-decode-*` logs retain that initial failure and its passing
pre-fix internal/race checks; they are not final-source performance evidence.
The pre-fix regression log uses `b07e08e` production code plus the newly added
stored-truth test. Unknown facts may be complete historical evidence; they
must not be confused with a missing historical row.

The separately retained `pre-final-retained-race.*` run passed a broader
review selector on the development tree before the final window/shape/test
refinements. Its note identifies that limitation; it is not an immutable-source
acceptance result for the final implementation.

Final paired results, three samples per arm:

| Size | Full fallback median | With lineage shadow median | Actual pair-days, both arms | Hypothetical pair-days, never published |
| --- | ---: | ---: | ---: | ---: |
| Small |63.252ms |96.624ms |310 |10 |
| Large |10.003s |11.698s |62,000 |2,000 |

Every shadow sample emitted `lineage_validated_shadow`. All six paired
persisted projection comparisons and all six hypothetical bounded-plan/full
oracles passed. Prefix key counts were300 and60,000, with separate windows
of10 and6,000 keys. The timed paths both prepared and published all dates;
hypothetical counts are not measured runtime savings.

| Median phase, full → shadow | Small | Large |
| --- | ---: | ---: |
| Capture |23.157 →42.213ms |2.520 →3.251s |
| Prepare |6.965 →7.342ms |1.761 →1.953s |
| Publish |33.272 →46.418ms |5.756 →6.589s |
| Total allocated bytes |13,055,456 →16,972,240 |3,924,949,872 →4,272,388,016 |

These independent phase medians need not sum to the whole-region median.
SQL queries rose117→449 small and614→984 large. Write statements remained
643/128,242 and affected-row counts33/6,242 respectively in both arms.
The additional shadow reads/checks add approximately53% small and17% large
whole-region median cost while preserving full computation. This is diagnostic
overhead, not a speedup estimate for eventual bounded reuse, and supplies no
activation evidence. Raw samples and machine-readable summaries are retained
in `lineage-perf.log` and `measurements.json`.

Final-source validation (full commands and per-check source in
`validation.json`/`provenance.json`):

| Check | Result |
| --- | --- |
| Focused race, `cdaed04` |Application PASS251.171s, including the new shadow cases, all nine PR123 bootstrap repairs and five PR121 interval cases. SQLite/domain helper packages have no matching standalone tests under this selector; their code is exercised through application tests. |
| `go test -p 1 -parallel 4 ./internal/... -count=1 -timeout=10m`, `3861e63` |PASS,239.202s command wall time; application105.094s, SQLite27.091s, MCP35.344s. The previous dense-deadline failure did not recur. |
| `go vet -p 1 ./internal/...` / `go build -p 1 ./internal/...`, `3861e63` |PASS |
| `gofmt -l cmd internal` / `git diff --check` / main CI-file comparison |PASS, no output/diff |
| `go build -p 1 ./cmd/nestworth`, `3861e63` |Failed: pkg-config cannot find GLib/GObject/GIO, GTK4, WebKitGTK6 and libsoup3 development packages. Native build is unavailable, not passed. |

The complete root `go test ./...`/`go vet ./...`, complete repository race
suite, frontend/binding freshness, macOS app/package smoke, native latency/RSS,
accessibility, signing and release checks are not claimed. No native app was
launched; later native QA must retain `com.nestworth.dev` and isolated data.
This task performed no merge to main, release or runtime activation.

## Activation gates remain closed

This shadow does not enable bounded fallback, remove the full source recheck,
or activate C3 in production. A later focused implementation still needs
reviewed complete provenance and invalidation semantics, independent oracle
and concurrency/failure evidence, and paired workloads that separate bootstrap,
warm no-change, clock-only, append, correction and activation. Representative
native Apple Silicon latency/RSS and the plan's proposed >10% regression gate
remain outstanding. Linux diagnostics do not establish cold 3s/warm 500ms or
native acceptance. Merging this shadow would not authorize activation/release.
