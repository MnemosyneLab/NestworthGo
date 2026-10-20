# Review dependency invalidation and suffix preparation (D1/D2)

Stage three follows section 6 of the [approved optimization plan](performance-optimization-plan.zh-CN.md).
C3 publication equality is the prerequisite, not stage three. Normal desktop
initialization selects `WithReviewDependencySuffix`; GUI and MCP use that same
repository/service. `NESTWORTH_REVIEW_FULL_RECOMPUTE=1` selects the existing
complete-source/full-preparation path for the process. No schema migration,
ledger setting, shared-fact layout, analysis batching, or history truncation is
introduced. Explicitly selecting the old lineage shadow retains its full runtime
oracle and excludes D2 selection.

## Scope and dependency proof

D1 consumes a contiguous interval of C1 journal revisions within its retained
floor and unchanged tracking/epoch/resolver/evaluator/metric namespace. It takes
the union of OLD and NEW instrument/effective-date scopes for instrument quote
insert/update/delete. The earliest effective-date label starts an unbounded
suffix for that instrument in every rule; this covers same-day replacement,
source candidate loss, and later price-lookback baselines. Quote selection uses
`EffectiveDate`, not polling time or the last N true votes. Ambiguous/malformed
scopes, non-quote changes, rule/configuration/handling changes, incompatible
calendars, damaged tracking, and missing prefix slots use full recomputation.
Generation-only `history_snapshot_state` updates may accompany quote writes;
they are ignored only after the separate non-quote fingerprint agrees.

Journal scope alone is insufficient historical authority. New authenticated
full publications sign two additional descriptions in the existing temporal
certificate:

* The complete exact non-quote historical fingerprint (all batch fields except
  instrument quote arrays and non-economic input generation). This detects
  activation of previously excluded activities even alongside quote DML.
* Exact quote-candidate fingerprints grouped by instrument/effective date,
  sorted by quote ID, including private decimal scalars, source priority,
  revision, eligibility metadata, dates, and every other captured field.

Capture requires the existing same-handle MAC and complete projection seal to
be authentic. It independently compares the old and current complete quote
bucket sets. Every differing OLD/NEW bucket must be covered by D1's dirty
frontier in every rule. Unexplained differences force full preparation; a
count-preserving or incomplete journal scope cannot hide a price change.
These additional signed fields do not establish authority by themselves.
They are never trusted after key loss, reopen, restore, or upgrade. Existing
private initial-publication permission, missing-history restrictions, exact
compiled schema/trigger/FK contracts, physical connection/counters, and provider
lease remain unchanged. No lost event or acknowledgement history is invented.

## D2 computation and publication

In the coherent capture transaction, load current-version complete daily
input/result pairs from origin through the earlier of the previous latest
closed day and dirty day minus one. The universe includes explicit/current
targets, evaluated instruments, and open episodes. Validate contiguous civil
slots, complete keys, input/result fingerprint linkage, typed dates/instruments,
and provider continuity. Any gap or decode mismatch discards the entire proposed
prefix and retains full preparation. False and unknown slots are retained.

Bind that prefix and its dirty plan to the existing private source-capture MAC
using both JSON and exact private scalar fingerprints, including selected FX
rates. D2 reuses only those authenticated values. Every remaining day through
the latest closed day uses the existing authoritative financial calculator and
predicate evaluator with the complete captured history and lookback candidates.
There is no early stopping. A date/instrument move invalidates both scopes.
Unrelated instruments reuse their complete previous financial daily evidence.

The first implementation deliberately uses **origin as the policy/lifecycle
checkpoint**: policy windows and position/episode lifecycle replay still run over
the full date sequence, using authenticated prefix votes and newly computed
suffix votes. This retains full consecutive streaks, rolling three-valued votes,
epochs, historical recovery/first-hit boundaries, current-time eligibility,
suppression expiry, frozen trigger evidence, and proven acknowledgement
association. It saves financial input/predicate recomputation, not all O(history)
work. A compact resumable lifecycle checkpoint is deferred until separately
proved. No financial history or frozen event is dropped to create that shortcut.

The resulting complete publication retains all original epoch, episode,
handling, state, control, checkpoint, temporal-frontier, cancellation and atomic
write work. C3's exact actual-argument equality comparison omits only zero-effect
daily UPSERTs. Unrelated daily results produce no SQL writes. Full source
capture, complete row sealing, prefix reads/decoding, quote seal comparison,
full policy replay and exact SQL image reads remain real costs.

A suffix publication requires the exact authenticated source/control/checkpoint
and physical connection CAS plus the pinned provider lease under the write
reservation. Authority loss returns a conflict before any write; it cannot
silently publish reused values through C2. The service recaptures/retries under
its existing limit. Captures lacking suffix authority select full preparation
up front. Provider discontinuity uses full preparation. Direct/disabled
publishers reject suffix stages. The minimum captured/rechecked temporal fence
and final injected-clock guard remain intact.

## Rollback and acceptance

Use the process-local environment switch above, then explicit reconcile to
atomically republish full output; no downgrade or data cleanup is needed.
Existing/restarted/restored ledgers remain full fallback and cannot acquire
certified historical reuse authority. Enabling the dispatcher does not assert
stage-two or native performance acceptance. Linux ordinary tests, paired
synthetic diagnostics and scoped race checks must be reported separately from
30-sample p95, isolated per-arm RSS, and native Apple Silicon acceptance.
