# C3a opt-in no-change confirmation candidate

Base: main `09d6276bb0f0be6ebdbfa4afacfcca843c1e623a` (PR121), with PR119
`c3867c626d3b6a0cdbcf669a0047ff8d07d4215f` as an ancestor. Both merge SHAs and
remote main were checked through GitHub and Git. New branch:
`perf/c3-no-change-review`. Reviewed/selectively reused the prior task's stopped
handoff `b007566969326e335c0064b8b55d20baa5518e91`; its branch was not changed.
Implementation and measurement harness: `5db64934d1ea4364c3f8b6b508d382be71cf992c`.
Measurement head `4b188bc` adds only an outstanding-full-stage concurrency test.
Post-measurement correction `dfbd16c` tightens only failed-tracking guards:
schema-cookie/PRAGMA or C1 verifier failure now requests full preparation without
rolling-window reuse. The measured valid confirmation and ordinary changed-data
paths are unchanged. Final focused unit/race checks cover that correction; the
earlier broad run is not relabelled as a rerun of this later head.
Before delivery main advanced to `1040f7b736b6dd5baac3d3deea855a8dc8e0c38a`
(user-owned PR122 CI/performance-test scheduling). It was merged unchanged into
this branch as `e1fb2852e11e1b12cfd1aa93721d071a298d6d65`, preserving the original
measurement commits. The PR diff contains no workflow/CI-script edits. No
production Go behavior changed in that main advance; the only changed helper
used by this evidence adds an empty-input guard to `durationPercentile`, unused
by the nonempty30-sample distributions. Post-sync integration checks are
recorded separately rather than relabelling prior measurements.

## Scope and activation gates

**Disabled by default.** Only explicit internal
`WithReviewNoChangeCandidate()` opts a repository in. There is no environment,
GUI, MCP or startup activation switch. `WithFullReviewRecompute()` disables it.
The normal service and changed-source publication retain full financial source
capture, exact fingerprint, preparation and full publication recheck. No dirty
suffix planning, cache, source-trigger optimization, schema migration or CI
change is included. User-owned PR118 workflow configuration is inherited intact;
no workflow is manually dispatched. No merge/release is authorized here.

The candidate attempts a short confirmation under the existing driver’s
`BEGIN IMMEDIATE` reservation, shared by GUI/MCP explicit refresh. Every gate
must hold in the same transaction:

1. Current schema, verified-open schema cookie, FK and recursive-trigger
   PRAGMAs; exact C1 trigger/table definitions and bounded journal continuity.
   DDL after open, including trigger drop/recreate, disables reuse for that handle.
   Failed tracking verification also discards reusable daily evidence on fallback.
2. Known source household/epoch/revision and tracking/resolver/evaluator/metric
   namespaces; consumed revision at/above the current journal floor. Pruning
   through the consumed revision is safe when the source token is unchanged.
3. A complete publication and a C3 proof minted by full publication, unchanged
   control/config/handling state, complete rule revision set, raw rule/version/
   target/suppression metadata, open-episode identity/revision/ack metadata and
   FX provider selection. C2-only checkpoints cannot authorize confirmation.
4. C2’s finite temporal interval and session/rollback generation cover capture
   and final logical as-of, and every rule’s closed date agrees. No frontier is
   extended. Passing a future source boundary, suppression/calendar boundary,
   restart or observed rollback takes the full path.
5. A persisted six-table row-count manifest agrees with current projection
   cardinalities. Missing daily input/result/evaluation rows force preparation
   with no reusable evidence, rather than trusting a surviving rolling window.
6. The final injected clock/provider check succeeds; control and checkpoint
   CAS each affect exactly one row. A conflict retries the whole operation at
   most three times. Cancellation/SQL failure rolls back the entire transaction.

Successful confirmation changes checkpoint as-of/JSON only, plus a control
self-update for checked CAS. Daily inputs/results, epochs, episodes, events,
acknowledgements and evaluation states remain unchanged. The original finite
end is retained. Source edits remain protected by the immediate reservation
until commit. A later source/time change makes subsequent reads stale as in C2.

PR121’s full-publication correction is preserved: the checkpoint end remains
`min(captured boundary, rechecked boundary)`, with a copy of the capture.
No-change is possible only with verified unchanged source tracking; its interval
is never extended and it does not repeat the uncut full-source frontier scan.

The count manifest checks completeness, not arbitrary same-cardinality manual
edits to derived evidence. As in C1, tracking is not a security boundary against
deliberate metadata/PRAGMA manipulation. It does not replace SQLite integrity
verification, verified recovery or independent financial/lifecycle oracles.
Counts scan historical indexes; this candidate makes no constant-work claim.

Default activation remains a separate reviewed decision under
[plan §5](../design/performance-optimization-plan.zh-CN.md#5-事务版本和变更日志pr-c1c2c3依赖-a).
Manual dependency/coverage review, representative native Apple Silicon latency
and peak-RSS comparisons, native build/acceptance and complete validation are
not established by Linux measurements. No 3s cold/500ms warm product acceptance
or release readiness is claimed. Disabling the candidate returns to C2 without
downgrading schema or deleting financial/handling data.

## Correctness evidence

The focused synthetic tests exercise default/forced-full behavior; unchanged
projection rows and cloned full recomputation; source/config/state/rule/episode/
provider changes; missing/unknown proofs/policies; journal gaps and floor pruning;
trigger drop/recreate and PRAGMAs; rollback/restart/epoch reset; exit/entry/clear-
rebuy/transfer at T−1ns/T/T+1ns with acknowledgement preservation; final-boundary
and final-provider rejection; cancellation; actual second-connection exclusion
then successful write after commit; CAS ignore/abort and bounded retry;
subprocess exit93 after checkpoint update before commit and recovery; missing
projection rows/full repair; rolled-back source DML; response-loss retry and
suppression expiry; and rejection of a full stage superseded by confirmation.
Existing C2 temporal/publication/source tests remain intact,
including the PR121 earlier/later-frontier regression and calendar/DST coverage.

The test oracle clones the authoritative synthetic SQLite snapshot, starts a
new DB session and forces full recomputation. It compares every projection/
lifecycle column, normalizing only computation timestamps and row insertion
order. Financial decimals, coverage, evidence, episode identity and ack remain.
Performance measurements additionally use the existing full daily typed oracle.
MCP/Wails tests run both the default path and the opt-in candidate, count actual
confirmations and compare refresh metadata and expired alert/coverage responses.

Validation results and measurements are recorded below after execution. Raw
commands, environment, logs and checksums live in
[`performance-c3-no-change-2026-10-19/`](performance-c3-no-change-2026-10-19/).

Current validation results:

- Focused C3 units passed7.002s. The first expanded unit run failed two
  assertions comparing restored daily rows in insertion order; the comparator
  now sorts canonical rows, preserving every semantic column. The original
  failure log is retained, followed by the passing final log.
- Full `go test -p 1 ./...` completed with38 successful package results,
  including application107.897s, SQLite24.207s, MCP32.771s, backup2.990s and
  continuous backup23.160s. The existing dense deadlines passed in this run.
  The command is **not green**: `cmd/nestworth` cannot compile without the
  GTK/WebKit/GLib/libsoup development dependencies. No backend assertion failed.
- MCP/Wails default and candidate parity passed under race13.710s.
- Focused domain/SQLite/application source, temporal, publication and C3 race
  suite passed1.017s /3.784s /383.319s, including the PR121 frontier fix. No race
  detector report or assertion failure occurred.
- After the failed-tracking fallback correction, final C3 units passed7.496s
  and the damaged-tracking/outstanding-stage race checks passed15.356s. Final
  internal vet/build, formatting and whitespace checks passed. The valid-path
  performance code did not change; measurements were not rerun or relabelled.
- After syncing latest main, C3 plus shared fixture configuration units passed
  7.989s and candidate MCP/Wails parity passed0.248s. No workflow was dispatched.
- `go vet ./internal/...`, `go build ./internal/...`, gofmt and whitespace
  checks passed. Full vet and the canonical desktop build fail on the same
  missing native packages. No dependency installation or desktop launch was
  attempted. Frontend/bindings/native Mac acceptance were not run; no bound
  service signatures, wire DTOs, frontend files or generated artifacts changed.
- Full `go test -race ./...` was not run. The focused race results are separate
  evidence. Unit/race/build validation jobs overlapped and their runtimes are
  correctness evidence, never performance samples.

## Results

All60 invocation pairs (30 per fixture) and every diagnostic full oracle passed.
The opt-in evidence test completed337.489s, serially after validation exited.
Machine: Linux x86_64 AMD EPYC9V74, five exposed CPUs, four-CPU cgroup quota,
GOMAXPROCS4; Go1.26.1 / SQLite3.53.0; ephemeral container storage (medium unknown).
Both arms use the **bounded C2 evidence window** to isolate C3; this is not a
timing claim about the production-default P0 baseline-window configuration.

| Warm no-change, n=30/arm | C2 bounded median | Candidate median | C2 bounded p95 | Candidate p95 |
| --- | ---: | ---: | ---: | ---: |
| Small | 34.015ms | 9.264ms | 40.786ms | 12.650ms |
| Large/R30 | 5.151s | 38.008ms | 5.560s | 47.880ms |

| Resource per call | Small C2 → candidate | Large C2 → candidate |
| --- | ---: | ---: |
| Median allocated bytes (decimal MB) | 8.124 → 1.324 | 1,943.045 → 2.567 |
| Median allocation count | 137,429 → 6,662 | 29,655,966 → 36,557 |
| SQL queries / returned rows, sample1 | 108 /2,186 → 170 /324 | 385 /512,911 → 170 /2,681 |
| Write SQL attempts / affected rows, sample1 | 22 /1 → 2 /2 | 4,002 /1 → 2 /2 |
| Median publication reservation-to-finish | 17.080 → 9.173ms | 2,565.185 → 37.928ms |

The candidate repeats the complete tracking verifier, so small-fixture query
count increases. It reads no quote/evidence JSON, and issues no daily/lifecycle
writes. Two matched metadata rows include the control self-CAS; this is not a
claim of fewer physical changed pages or zero WAL. Returned rows do not count
the historical index entries scanned for the completeness manifest.

The final candidate storage diagnostic is2,810,632 bytes small /244,183,816 bytes
large for DB+WAL+SHM, including60000 inputs and60000 results in the large fixture.
These are complete store sizes, not C3-only growth. Cumulative process high-water
RSS was55,816KiB after small and779,084KiB after large including setup/clones/
oracles. These cannot establish per-arm peak RSS or the10% native rollout gate.

| Separate diagnostic, n=1 | Small | Large/R30 |
| --- | ---: | ---: |
| Clock-only | 45.842ms;10 pair-days | 5.846s;2,000 pair-days |
| Historical quote correction | 57.573ms;300 pair-days | 10.214s;60,000 pair-days |
| Market append | 57.389ms;310 pair-days | 10.885s;62,000 pair-days |

Large fresh projection took9.384s (n=1, warm process/pages, not OS/native cold).
The existing full path remains expensive for changes; this PR claims no suffix,
activation, cold-start or clock-only acceleration. These are controlled Linux
fixture observations, not production latency estimates or native SLA acceptance.
[`summary.json`](performance-c3-no-change-2026-10-19/summary.json) contains exact
nanoseconds/allocation summaries; its standard-library parser is reproducible.

## Measurement method

`NW_C3_PERF_EVIDENCE=1` enables `TestReviewNoChangePerformanceEvidence` only.
It runs serially after validation, using deterministic arithmetic fixtures,
fixed injected clocks and UUID-identical `VACUUM INTO` clones. The baseline
keeps C2 full capture/recheck with the bounded evidence window; the candidate
uses the same window for fallback. Both refresh their independent DB sessions
before timing. Thirty paired samples alternate execution order, advance by
nanoseconds inside one valid interval and include invocation through commit.
Setup, cloning, session refresh, oracles and diagnostic reads are outside samples.
This is warm no-change, not cold process/OS startup or clock-only work.

Small: 1 account, 10 instruments/holdings, 1 rule, 30 days, 310 generated quotes
plus 10 manual seeds, 100 cash activities and 120 FX rows. Large: historical R30,
1 account, 100 instruments/holdings, 20 rules (2,000 pairs), 30 days, 100,000
generated quotes plus 100 manual seeds, 50,000 cash activities and 120 FX rows.
Generated arithmetic fixtures use no provider network or real credentials/data.

Logs retain every duration, allocation bytes/count, emitted pair-days, SQL
executions/rows consumed/affected rows and Query-to-Rows.Close time (including
consumption, not pure SQL). The test-only driver now preserves production
`recursive_triggers=1` and measures successful non-readonly BeginTx reservation
through Commit/Rollback, excluding lock acquisition wait. Count-query returned
rows do not represent index rows visited. First-sample SQL consumers and DB/WAL/
SHM sizes are retained. Allocation totals are process-counter deltas, not peak
live heap/RSS. Process high-water RSS is cumulative and cannot establish a
paired arm-specific RSS regression gate. Nearest-rank p95 is sorted sample29.

Fresh-projection, clock-only, quote-correction and append diagnostics are logged
separately at n=1 per fixture, with a full oracle, and have no p95 claim. Temporal
activation is covered by deterministic correctness tests, not these warm timings.
Native identity remains `com.nestworth.dev` if later native tests launch an app;
no native app was launched in this task.
