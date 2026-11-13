# Performance optimization report

## Source and implementation status

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

## Maintained correctness oracles

The full-source review oracle uses supported certified capture, discards every
reuse input and computes all days independently. In-flight publication
comparisons use the coherent captured source. Durable-row comparisons use an
independently reopened source clone. Whole-household account hydration remains
a test oracle. Production passes an explicit selected-account scope.

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

## Performance evidence

The prefix memo avoids some repeated decoding and argument preparation. It has
cold-allocation and retained-heap costs and still performs source reads,
policy/lifecycle replay and proof validation. Use controlled samples and report
elapsed time, allocation and RSS separately. Record native Mac measurements
separately from Linux measurements.

## Maintained checks

Retain certified context/schema/DDL/FK/trigger/mutation/genesis/missing-history,
publication equality, dependency/FX, memo ownership/generation and full-oracle
regressions. Keep activity hydration chunk/cancellation/snapshot coverage,
analysis quote/account/snapshot/result comparisons, JSON exact-byte/stream tests,
and frontend freshness/hidden-read/nested session/draft/guard regressions.

The ordinary suite includes small memo guard and epoch/lifecycle regressions.
`TestReviewCaptureMACBindsEligibilityAndMemoReceipt` binds eligibility and
ownership fields. `TestReviewReusableWindowEquivalent` retains an independent
query oracle for supported C2 callers.

`TestReviewDefaultPerformance` is the perf-tagged default-path smoke check with
full financial and durable-result comparisons. The extended dependency oracle
and repeated epoch check run through the perf workflow. Tagged JSON checks
compare maintained synthetic integrity outputs directly. Compare generic float
compatibility separately from integrity-bearing bytes.
