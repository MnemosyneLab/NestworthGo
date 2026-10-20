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

In the coherent capture transaction, use request-table chunks of at most 100
instruments (206 SQL parameters) per rule to load current-version complete daily
input/result pairs from origin through the earlier of the previous latest
closed day and dirty day minus one. The universe includes explicit/current
targets, evaluated instruments, and open episodes. Validate contiguous civil
slots, complete keys, input/result fingerprint linkage, typed dates/instruments,
and provider continuity. Any gap or decode mismatch discards the entire proposed
prefix and retains full preparation. False and unknown slots are retained.

Bind the prefix/dirty-plan digest and prefix presence to the existing private
source-capture MAC. The digest covers JSON and exact private scalars, including
selected FX rates. Capture hashes it once; routine source CAS and signing
authenticate the bound digest without repeatedly hashing the complete prefix.
After the final injected clock and before the first write, publication recomputes
the typed digest and source MAC, rejecting prefix or digest drift atomically.
D2 reuses only those authenticated values. Every remaining day through
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

Correction/fallback publications remain complete. Clock-only publications may
omit exactly attested daily output under the contract below. All retain the
original epoch, episode,
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

## Coherent source-description reuse

The source-capture MAC also binds the nonquote fingerprint and complete quote
bucket descriptions used by this suffix capture. If the exact source token and
complete historical fingerprint equal the previous authenticated publication,
the descriptions are deterministic and reused inside this verified capture.
For changed source, compute descriptions once, independently compare changed
quote buckets against the contiguous journal and bind them before signing.
The final pre-write MAC check detects description drift. Physical connection
counters, full row sealing and every existing CAS/lease/time fence still apply.
This shares no persistent financial facts and grants no historical authority.

See [follow-up evidence](../testing/performance-d1-d2-followup-2026-10-20.md) for
measured remaining overhead and the required separate suffix-publication proof
before omitting daily outputs instead of comparing their complete SQL arguments.

## Clock-only exact output attestation

On the unchanged-source path only (identical complete authoritative fingerprint
and exact source token), the coherent prefix reader selects all13 input and9
result SQL payload columns. It decodes financial evidence and invokes the **same
canonical argument builder** used by publication. Attestation requires exact
typed equality of every key, scalar, NULL, coverage/reason and JSON byte,
including the complete selected-FX codec. Computed/evaluated/revised timestamps
are excluded from the argument image because an equal payload makes the existing
UPSERT WHERE false and preserves all three. Serialization mismatch leaves the
row unattested and emits it through the normal C3 comparator. Quote corrections
and other source changes keep their previous reader/comparator path.

Only `OutputAttested` membership and authenticated typed evidence cross the
capture boundary, not SQL images. The existing suffix digest and source MAC bind
these fields. Preparation still computes every policy window and lifecycle from
origin, but omits the attested daily output from its publication. Daily policy
windows themselves were never daily SQL payload columns; latest/frozen policy
and lifecycle output remains fully staged.

Before comparison and again after the final injected clock, the writer checks
an exact unique partition of every expected origin-to-latest daily key into
retained attested output or emitted days. It derives eligible scopes from the
capture; missing/duplicate scopes, gaps, overlap, duplicate days, extra dates or
changed target exclusion reject atomically. The original rule/version/timezone
and final latest-day checks remain. Existing C3 CAS, authenticated protected-row
seal, physical counters, unchanged pinned provider lease and temporal fences
protect the attestation through the writer reservation. Final prefix digest/MAC
verification and complete post-publication sealing remain mandatory.

For an attested publication, transaction-local equality reads begin at the
earliest emitted date, retaining a conservative superset of all attempted daily
keys. A missing image executes the original UPSERT; consume-once duplicate-key
semantics are unchanged. Without any attested output, the full equality reader
is unchanged. This is a bounded D2 publication proof, not a lifecycle checkpoint,
E/shared facts, a new historical trust authority or a fingerprint substitution
for exact SQL argument comparison.


Normal GUI/MCP startup now also enables the bounded same-handle prefix memo
following explicit user approval. `NESTWORTH_REVIEW_PREFIX_MEMO_DISABLE=1`
disables that memo while retaining D1/D2; the full-recompute switch above takes
precedence. See [the memo contract](review-prefix-memo-experiment.md) for limits,
authority requirements and the measured cold/memory tradeoff.

## FX extension boundary

FX mutations still require full recomputation. The current signed non-quote
fingerprint includes FX, while the candidate inventory and OLD/NEW coverage proof
above describe instrument quotes only. See the [D-FX boundary evidence](../testing/performance-d-fx-boundary-2026-10-20.md)
for synthetic full-oracle fallback coverage and the missing authentication proof
that blocks a narrow direct-pair extension. Unchanged-source prefixes may retain
already authenticated selected FX; this does not authorize FX-source mutations.
