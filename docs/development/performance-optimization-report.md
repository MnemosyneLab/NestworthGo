# Performance optimization report

## Source and implementation status

Audit and cleanup base: `dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d`.
Implementation: [cleanup PR #154](https://github.com/MnemosyneLab/NestworthGo/pull/154).
Runtime/harness consolidation is `b35f3ce`; oracle isolation is `f2733ff`.
The PR records their full immutable SHAs, final documentation head and exact-head
ordinary CI as the delivery record. The
first validated checkpoint was `826449a2815e387e485b54851eaae271bbc9c1f5`;
`ccfacd5711f1431d6a0522930b89e443c1cb9678` exposed a legacy-fixture migration
error in ordinary CI, corrected by seeding genuinely uncertified C2 history.
The assertion that legacy history never gains integrity authority is preserved.

Normal desktop GUI/MCP startup always enables certified no-change, dependency
suffix preparation and same-handle prefix memo. The two environment rollback
switches have been removed. Bare repositories retain the non-certified C2
compatibility path and baseline key-first window SQL. B bounded production
selection and the constant-false bootstrap implementation have been removed.
The memo experiment alias, boolean argument and observation-only mode are also
removed; callers select the single normal default initializer without an on/off argument.
Complete preparation and recomputation remain normal safety fallbacks.

## Shipped improvements

- Financial source revision/journal tracking and temporal publication fences.
- Authenticated same-handle no-change confirmation, daily-prefix suffix reuse,
  exact SQL-image publication equality and bounded private prefix memo.
- Shared-ID activity hydration, batched quote reads, scoped account/snapshot
  hydration, period/presence/quote indexes and request-local lookup reuse.
- Frontend page visibility and refresh invalidation that avoid hidden reads.
- Sonic through the first-party JSON wrapper with exact-byte/stream safeguards.

These describe implemented/default behavior, not deployment or native acceptance.
No schema or financial policy change is part of this cleanup.

## Rejected work and removed experiments

Removed the old counting-only no-change experiment, independent lineage/runtime
shadow, constant-false bootstrap, unused B bounded candidate, rollout constructors
and environment overrides. Bare `NewRepository` retains reachable non-certified
C2 baseline SQL; desktop initialization remains certified, suffix-enabled and
memo-enabled. Business controls and automatic safety fallbacks are unchanged.

Full-source preparation and quote-loading oracles now live in tests. The review
oracle uses supported certified capture to bypass C2 cached-window decoding,
then discards every reuse input before independently computing all days.
In-flight publication comparisons recompute from the coherent captured source
without recapturing on the candidate handle; durable-row comparisons additionally
use an independently reopened source clone. Small
regressions cover valid poisoned and malformed input/result JSON. Query counters
and fault injection belong to test drivers. Removed memo observation counters
from both capture-MAC producer and verifier; eligibility/receipt tampering tests
retain the security boundary.

Retired the default-off/on-off matrices, profile/RSS/stage probes, artifact
snapshot generators, repeated paired/full-pipeline and write-amplification
harnesses. Shared small fixtures, exact durable-row/day-set/lifecycle comparators,
crash/CAS faults, memo/source ownership and cancellation regressions remain.
Whole-household account hydration is now a test oracle; production always passes
an explicit selected-account scope. Exactly 722 audited historical artifacts
(690 files in 24 directories and 32 top-level reports) were removed, plus the
four separately audited obsolete plan/memo/codec/QA documents. Durable protocol
details live in existing maintenance guides, not additional process reports.

Legacy/restart bootstrap cannot establish absent historical handling provenance.
B bounded is not enabled. E1 was reverted in `c14308c`; existing request-local
caches remain. Agent instrument append was rejected because reduced computed-day
counts did not deliver better measured elapsed time. E2 and further SQL work are
optional future investigations, not rollout gates or cleanup scope.

## Correctness contracts

The [source/temporal protocol](../design/investment-review.md#financial-source-tracking-maintenance)
and [suffix/memo protocol](../design/review-dependency-suffix.md) retain durable
dependency, invalidation, ownership and publication requirements.

- Preserve exact financial values, source ownership, dates and missing-data
  semantics, including PR144 incomplete-header full hydration.
- Incomplete snapshots, unknown dependencies, journal gaps/floors, invalid
  provenance, MAC/certificate/CAS/row-seal failures, provider/clock changes,
  new handles and memo capacity/generation changes retain conservative fallback.
- Never infer missing episode/event/ack history from current empty tables.
- Source capture MAC encodes the complete capture proof. Bootstrap removal
  changes its in-memory encoded shape in both producer and verifier. Capture
  proofs/private signing keys are not persisted or exported; stored temporal
  certificate encoding and schema remain unchanged.
- Keep complete day sets, episode/event/ack identities, timestamps, cancellation,
  crash recovery, exact FX and lossless codec comparisons.
- Page active/enabled controls, business configuration, backup, security flags
  and isolated database paths remain business behavior.

## Limited measured evidence

A historical matched Linux synthetic dense-hydration sample compared baseline
`47c31ca88f1527f550e06f0c43d8f8854f3c0b08` with candidate
`f2fee931fa3d5041311468534ccb196332696067`: ten alternating pairs had median
service-call time 5.222s versus 2.902s and median counted SQL calls 623 versus
273. Both returned 86,433 facts, 13,513,678 bytes, 1,000 leaves and 31 points.
Host: Linux x86_64 AMD EPYC 9V74, CPU quota 4, Go 1.26.0, modernc SQLite
3.53.0, GOMAXPROCS=4. Fresh processes/connections used warm OS file data;
fixture creation, startup and GC were outside the timer. The ten-sample
nearest-rank p95 is merely the observed maximum, not a stable tail estimate.

Prefix memo reduced repeated warm decode/argument work but has cold-allocation
and retained-heap costs. It still decodes/copies owned values and preserves
source reads, policy/lifecycle replay and proof validation. Allocation counters
are not RSS. No unified end-to-end latency, RSS, p95 or native Mac result is
established by these observations or by the latest hidden-read/lookup changes.

Historical Sonic validation on darwin/arm64 compared baseline
`8a6bdbcefadb9e965280b6bd1bde7bedf13dc102`: 60 JSON fixtures and 13 synthetic
integrity outputs matched, including certificate/fingerprint/receipt/HMAC bytes.
That record reports Go 1.26.4 full non-race tests, vet and native compilation;
it does not report app launch acceptance or a codec speedup measurement.

The removed raw logs/profiles and original stage reports remain recoverable in
[the immutable audit tree](https://github.com/MnemosyneLab/NestworthGo/tree/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d/docs/testing).
They are historical evidence, not current validation.

## Maintained checks

Retain certified context/schema/DDL/FK/trigger/mutation/genesis/missing-history,
publication equality, dependency/FX, memo ownership/generation and full-oracle
regressions. Keep activity hydration chunk/cancellation/snapshot coverage,
analysis quote/account/snapshot/result comparisons, JSON exact-byte/stream tests,
and frontend freshness/hidden-read/nested session/draft/guard regressions.

The small memo guard regressions now run without perf/environment gating:
stale/cancel/evict/source/provider/ack/suppression/missing-row/clock/reopen/rule,
exact-FX alias isolation and future activation. The capacity regression checks
payload overflow without constructing a 32MiB string. Together the focused
application cases took 4.711s and SQLite capacity coverage 0.044s on this Linux
executor; these are test costs, not application performance samples.

`TestReviewCaptureMACBindsEligibilityAndMemoReceipt` binds current eligibility and
ownership fields. `TestReviewReusableWindowEquivalent` retains a small independent
query oracle for supported C2 callers; obsolete B timing/EXPLAIN/probe matrices
are removed from that test. The nested reconciliation App test has an individual
10-second integration budget, justified by main's 5.087s and prior PR's 4.634s
against the old 5-second limit; assertions and global timeouts are unchanged.

`TestReviewDefaultPerformance` is a small perf-tagged default-path smoke check
with full financial and durable-result comparisons. The extended dependency
oracle and eight-cycle epoch check remain opt-in through the perf workflow.
Normal checks keep only the small epoch/lifecycle regression. Tagged JSON checks
compare six maintained synthetic integrity outputs directly without generating
repository snapshots. On this Linux Go 1.26.0 executor Sonic emits native `-0`
byte parity; the older platform's documented `0` exception remains explicitly
accepted for generic floats, outside strict integrity-output comparisons.

Final exact-head ordinary CI is required in the PR. Perf selectors are synchronized with retained tests; a preflight checks both
zero matches and individual deleted alternatives before starting expensive runs. Race stays in the daily/manual perf workflow and
the schedule cost gate remains intact. No race reruns or broad matrices were run
for this cleanup. Native packaging, launch, signing and deployment are unverified.
