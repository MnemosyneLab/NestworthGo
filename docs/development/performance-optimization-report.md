# Performance optimization report

## Source and implementation status

Audit and cleanup base: `dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d`.
The cleanup branch is in progress; its reviewed final head and CI results belong
in the pull request. This report does not claim that pending runtime/harness
cleanup is finished.

Normal desktop GUI/MCP startup always enables certified no-change, dependency
suffix preparation and same-handle prefix memo. The two environment rollback
switches have been removed. Bare repositories retain the non-certified C2
compatibility path and baseline key-first window SQL. B bounded production
selection and the constant-false bootstrap implementation have been removed.
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

## Rejected work and remaining cleanup

The old counting-only no-change experiment and independent lineage/runtime shadow
are rejected designs. Their runtime/test dependency chains still require cleanup
on this branch; existing tests and helpers are retained until their useful safety
regressions have been migrated. Full-recompute and constructor-only test controls,
observation fields and remaining profile/multi-arm harnesses also remain
pending. Eight standalone old no-change/RSS/lineage/bootstrap/dispatch/memo/epoch
and historical CPU-profile harness files have been removed. Removing those dependencies must preserve the complete independent
financial/lifecycle oracle rather than only compare the latest day.

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

`TestReviewCaptureMACBindsEligibilityAndMemoReceipt` binds current eligibility and
ownership fields. `TestReviewReusableWindowEquivalent` retains a small independent
query oracle for supported C2 callers; obsolete B timing/EXPLAIN/probe matrices
are removed from that test. The nested reconciliation App test has an individual
10-second integration budget, justified by main's 5.087s and prior PR's 4.634s
against the old 5-second limit; assertions and global timeouts are unchanged.

Final exact-head ordinary CI and the remaining harness migration are pending.
Perf selectors are synchronized with retained tests; a preflight checks both
zero matches and individual deleted alternatives before starting expensive runs. Race stays in the daily/manual perf workflow and
the schedule cost gate remains intact. No race reruns or broad matrices were run
for this cleanup. Native packaging, launch, signing and deployment are unverified.
