# Review dependency invalidation and suffix preparation (D1/D2)

The [performance report](../development/performance-optimization-report.md) records
the shipped choices. Certified publication equality is a prerequisite. Normal
desktop initialization selects `WithDefaultReviewOptimizations`; GUI and MCP
use that same repository/service. Ineligible evidence uses the existing complete-source/
full-preparation fallback automatically. No schema migration,
ledger setting, shared-fact layout, analysis batching, or history truncation is
introduced. The abandoned independent lineage/runtime shadow is removed.

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

See [follow-up evidence](https://github.com/MnemosyneLab/NestworthGo/blob/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d/docs/testing/performance-d1-d2-followup-2026-10-20.md) for
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
with automatic capacity, ownership and generation fallbacks. Startup has no
environment switches for algorithm selection. See [the memo contract](../development/performance-optimization-report.md) for limits,
authority requirements and the measured cold/memory tradeoff.

## Direct reference FX import extension

Changed FX has a separately versioned non-FX invariant, complete authenticated
candidate inventories from both captured FX collections, and a complete sealed
household Agent provenance inventory. The original non-quote fingerprint still
includes FX and remains the instrument-quote guard; no provenance table is
ignored. Descriptions are derived inside coherent capture before source-MAC
signing and copied into an authentic publication certificate. Missing/legacy
versions cannot authorize changed-FX reuse.

The scope is a production `ImportAgentMarketData` append, replacement correction
(including effective-date movement) or retraction for one direct pair of Agent
daily-reference candidates with unchanged source, policy, timestamp basis and
preferences. `direct-agent-import-fx-v3` rejects older description proofs, requiring
full rebuilding before these semantics can authorize reuse. Every prior provenance
row remains identical. Each added FX record and active-candidate addition/withdrawal must
independently match the sealed inventories and journal IDs, with supersedes/target
association. Quote-only raw UPDATE/DELETE is full fallback. Candidate/provenance
coverage includes inactive targets and unrelated instrument records; their changes
are never broadly exempted.

For a correction, independently validated OLD target and NEW replacement dates
both contribute: the frontier is `min(old date, new date)`. A later replacement
removes the old candidate on its original date; the prior baseline applies until
the replacement becomes eligible. Origin-or-before movement cannot reuse a
nonempty prefix. No intermediate correction-chain proof is added.

Every known rule/instrument is invalidated from the earliest affected date through
latest closed day, retaining full origin policy/lifecycle replay and every existing
MAC, sealing, CAS, physical connection, provider and clock fence. Unsupported
sources/routing, pair/policy/preference changes, unrelated/mixed writes, incomplete proof
and restart/restore remain full preparation. See the [production import proof and
full-oracle evidence](https://github.com/MnemosyneLab/NestworthGo/blob/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d/docs/testing/performance-d-fx-boundary-2026-10-20.md) for exact
eligibility and demonstrated application-path computation savings.

See [effective-date movement evidence](https://github.com/MnemosyneLab/NestworthGo/blob/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d/docs/testing/performance-d-fx-date-movement-2026-10-20.md) for the bounded synthetic cases and limits.


## Same-handle prefix memo ownership

Private representation: immutable strings encoding complete typed input via the
existing lossless exact-FX codec, coverage, and daily result, plus exact key and
fingerprint/value metadata. These are typed-evidence encodings, not SQL tuples.
Only entries whose22-column SQL image exactly matched canonical arguments may
enter the memo. On reuse, authenticate the private content digest and envelope
before decoding fresh owned public values. No caller map, slice, decimal storage
or scan destination is retained. Decode/copy remains O(history), as do complete
source capture/fingerprint, policy/position/lifecycle replay, final typed-prefix
digest/MAC, unique retained/emitted partition and full protected-row sealing.

Bind each snapshot to this DB handle/key, household, exact committed predecessor
certificate digest and row seal, complete source fingerprint and exact token,
provider selection/revision, clock/proof/codec generation, and exact key/scope
membership. Every scope covers a contiguous prefix from its captured origin.
Full source reads remain mandatory because future events can change membership
without a journal write. Provider/rule/handling/source/clock/reopen changes miss
conservatively. Miss/eviction never grants or destroys certification authority.

Seed private candidates while the coherent reader still owns decoded data,
before exposing the capture. Carry only an authenticated receipt in the capture.
Candidate data stays private. After successful certified commit, promote only the
matching private candidate/generation and rebind its unchanged attested content
to the exact new committed certificate/seal. The unique partition guarantees
those retained keys were not written. Newly emitted UPSERT rows are NOT cached:
the next capture reads/attests the uncovered tail with the existing comparator.
SQL success alone is not canonical equality. Serialization mismatches stop that
scope's cached coverage, retaining ordinary comparison.

One committed and one pending immutable snapshot per DB handle. At most65536
entries and32MiB encoded payload per snapshot; at most64MiB encoded payload plus
bounded entry/scope metadata across both. These are eviction limits, not routing
thresholds or an RSS bound. Snapshot replacement is mutex-protected; readers use
immutable snapshots. A private generation observed inside the SQL transaction fences its postcommit
hook; stale hooks and stale/evicted receipts cannot replace newer state. No
private mutex spans SQLite commit. Failed/cancelled/CAS-
rejected/rolled-back publication cannot advance or promote it. No promotion reads
caller-owned evidence. Source/proof failure uses existing fallback; no authority
repair. Default D2 remains available independently of memo availability.


Certified no-op publication only rebinds existing immutable memo contents after
successful checkpoint CAS. It requires the same source, seal, provider and clock
and seeds no new entries. Generation is sampled before financial reads; lookup,
eviction and pending installation are fenced against newer committed snapshots.
Memo misses and eviction never grant or remove certification authority.
