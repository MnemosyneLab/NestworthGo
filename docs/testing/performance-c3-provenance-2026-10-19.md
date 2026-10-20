# C3 provenance correction: missing history is not a clean bootstrap

Current source: `7932837`. This supersedes the history-free bootstrap claims in
the earlier [follow-up checkpoint](performance-c3-followup-2026-10-19.md).
**Legacy/restart certification is disabled.** Production remains C2 and the PR
remains a default-off draft; this is not stage-two acceptance.

The [subsequent schema-contract/dispatch checkpoint](performance-c3-dispatch-2026-10-19.md)
fixes a separately reproduced FK cascade writer and removes implicit window
auditing from ordinary C3. Measurements below remain evidence for `7932837`;
use the linked checkpoint for current dispatch costs.

## Independent missing-history regression

The reviewer identified a valid FK-safe deletion: NULL evaluation-state episode
references, delete events, then delete episodes, retaining the prior checkpoint
and acknowledgement mutation receipt. Current episode/event counts become zero.
The rejected bootstrap used those zero counts to grant `MayBootstrap`, performed
full preparation, matched four derived key sets and signed a fresh checkpoint.

The regression first saves the actual acknowledged episode ID/time, receipt and
checkpoint **before deletion**. It proves the old checkpoint recorded nonzero
episode/event counts and the receipt survives unchanged. After restart/key loss,
full preparation of the open case constructs a different, unacknowledged episode.
The resolved case cannot reconstruct its deleted frozen events either. Both
cases previously acquired fresh integrity authority despite that contradiction.
Two full captures taken after deletion would share absent `OpenEpisodes` and
could agree falsely; they are not the historical oracle in these tests.

An additional case erases every derived review table/checkpoint and resets
published control fields while retaining the ack receipt. It shows why the old
"empty namespace" branch also needs independent origin evidence. A same-process
C2 publication/ack followed by erasure of **all** review markers/receipts is
covered too; stored absence cannot reset the private lifecycle receipt.

Full replay recomputes financial daily evidence from current sources. It cannot
recover deleted acknowledgement identity, detection times or all frozen events,
nor prove that an apparently empty namespace never had such history. The
bootstrap candidate is therefore hard-disabled in capture, publication selection
and issuance (`reviewBootstrapProvenanceEnabled=false`). Its structural checks
are retained for review/reproduction, not as sufficient future authorization.

## Initial authority has a separate private origin

Initial certification is limited to a database whose schema this `Open` created.
Reopened, restored and upgraded files never receive this permission, including
ones whose current review tables happen to be empty. A newly initialized schema
records its original physical connection and `data_version` in a reserved
transaction while it has no household or prior review/legacy/receipt history.
The receipt exists only in process memory; SQL/JSON cannot grant or restore it.

Before preparing initial authority, capture requires that private permission,
the original physical connection, unchanged external commit version, empty
derived namespace, no prior publication and no ack/legacy evidence, plus all
normal C1/source/provider/temporal gates. The initial flag is bound by the
repository's source MAC. Any ordinary C2/C3 publication consumes permission
before its first writes under the writer reservation. Acknowledgement handling
and restore/import tracking resets invalidate it too.

Publication must claim that permission atomically and have **unchanged captured
connection counters**, not merely an equal full fingerprint after intervening
commits. An intervening publication followed by complete SQL erasure cannot turn
the older captured initial flag into new authority. Failed/unknown claims retain
full computation/publication with no MAC. Permission loss is conservative; a
failed initial attempt may leave this handle on full fallback permanently.

An already authenticated same-handle chain can still continue after complete
row validation and all existing guards. This is independent of initial origin.
No private key or origin receipt is persisted/exported or reconstructed from
current ledger contents. No general fact cache, sidecar trust store, source
write-trigger optimization, new planner or CI change is introduced.

## Supported states

| State | Integrity authority |
|---|---|
| Fresh schema/current original connection, no prior review handling/publication, all initial gates and atomic claim succeed | One full initial publication may establish authority |
| Valid same-handle signature and complete unchanged protected state | Existing no-op/audited publication gates remain available |
| Existing, reopened, restored, upgraded or replaced-pool namespace | Full fallback; no new integrity authority |
| Deleted episode/event/ack history, even with zero current counts | Full fallback; missing history is not authenticated or claimed repaired |
| Prior C2 publication followed by erasure of all stored markers | Private initial permission remains consumed |
| Unknown/damaged tracking/driver/connection | Original full source/preparation fallback; no authority |

A coherent fresh capture can still use independently verified **source** CAS
without historical integrity authority. It never authorizes no-op/window reuse.
PR121's minimum captured/rechecked temporal frontier and final clock guard stay
unchanged. No race was rerun or timeout increased.

## Validation and measurements

All ordinary `./internal/...` tests pass on `7932837` (application 120.422s,
SQLite 25.963s), with internal vet/build. Focused deletion/genesis cases pass
in 1.312s; the complete certified suite passes (application 14.147s, SQLite
0.351s). The same three deletion cases fail on an immutable archive of exact
`b785d677412cf764482aa6e7e2241c8e30b1e876` plus the regression fixture.

Serial small diagnostics (n=3 medians) pass independent pre-publication full
and durable-output oracles: same-day C2 32.893ms / candidate 3.192ms;
clock-only 36.308ms / 56.466ms; append 60.052ms / 51.913ms; correction
51.252ms / 50.653ms. The distinct history-free restart **fallback** case is
57.138ms / 48.483ms, explicitly requiring no MAC and continued no-op refusal.
The current large serial diagnostic (n=1 individual values) also passes all
independent pre-publication and durable-output oracles:

| Large scenario / n=1 | C2 | Certified |
|---|---:|---:|
| same-day | 6.039s | 28.497ms |
| clock-only | 6.283s | 10.512s |
| append | 9.696s | 8.890s |
| correction | 9.686s | 9.295s |
| restarted full fallback, no authority | 8.232s | 8.661s |

These are serial Linux diagnostics, not 30-sample p95 or isolated per-arm RSS.
The unchanged runtime audit still causes clock-only regression, blocking default
activation. Cumulative `allocatedBytes` is not RSS; whole-process VmHWM includes
fixture seeding, clones, oracles and both arms. No cross-commit controlled speedup
claim is made. Fresh initialization seeding changes setup and may affect physical
layout, so prior iteration timings are not treated as a matched before/after.
The same small/large manifests and exact paired financial IDs/data remain in
force; the harness checks source fingerprint equality and all financial/lifecycle
outputs before publication.

[Before-fix deletion reproduction](artifacts/c3-provenance-2026-10-19/missing-history-before-b785d67.log),
[focused provenance cases](artifacts/c3-provenance-2026-10-19/provenance-focused.log),
[certified tests](artifacts/c3-provenance-2026-10-19/certified.log),
[all internal](artifacts/c3-provenance-2026-10-19/all-internal.log) and
[small raw measurements](artifacts/c3-provenance-2026-10-19/perf-small.log),
[large raw measurements](artifacts/c3-provenance-2026-10-19/perf-large.log) and
[command/result metadata](artifacts/c3-provenance-2026-10-19/validation.json)
retain exact observations. All fixtures/providers are isolated and synthetic.
No race was rerun. No macOS/native or 30-sample p95/per-arm RSS pass is claimed.

The earlier `6861411` history-free bootstrap timings measured a now-rejected
candidate. Its daily/output oracle agreement did not prove that handling history
had never existed. Those numbers cannot justify activation or be described as a
currently supported bootstrap. New restart measurements explicitly require
continued no-op refusal and no MAC after full fallback.

Synthetic performance fixtures now create a fresh schema with the metrics driver
installed from the beginning and seed only unpublished synthetic facts/config.
They do not reopen a snapshot and pretend that it carries origin authority.
The seed helper rejects source review checkpoints, episodes/events, ack receipts
and legacy history. Both arms still share the same synthetic financial IDs and
source facts; all monetary/rule/lifecycle oracle comparisons remain in place.

Remaining gates include an independently reviewed provenance protocol before
existing/restarted ledgers can certify, C3's full runtime-oracle overhead and
changed-path regressions, the earlier incomplete race gate, native Apple Silicon
acceptance and paired 30-sample p95/isolated per-arm RSS. They remain incomplete;
no merge, activation, workflow dispatch or release is performed.

The read-only audit of that runtime cost found a separate proof obligation.
Daily inputs use a fixed civil cutoff (`review_projection_cache.go`), while
current position/eligibility uses capture time plus one nanosecond
(`review_projection_plan.go`). Financial fingerprint equality therefore does
not by itself prove unchanged current position epochs or lifecycle replay
boundaries. A faster path using the existing C2 calculator would need explicit
authenticated target/position/policy continuity and independent future-time,
handling and output-mutation oracles. That path has not been enabled; the
current candidate retains its complete runtime audit. This is an implementation
proof/performance issue, not a claim that D/E or native dependencies are required
to investigate it.
