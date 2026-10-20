# C3 follow-up: implementation boundaries and trigger integrity

**Superseded bootstrap claim:** the [missing-history correction](performance-c3-provenance-2026-10-19.md)
disproves the history-free bootstrap below. Legacy/restart certification is now
hard-disabled; only a private, controlled fresh initialization may establish
initial authority. This document retains the earlier rejected experiment and
frozen measurements for provenance, not as activation evidence.
The [subsequent schema/dispatch checkpoint](performance-c3-dispatch-2026-10-19.md)
fixes a reproduced FK cascade writer and removes implicit window auditing from
ordinary C3; use its current-source measurements for that dispatch.

PR129 remains an intermediate, default-off delivery. This follow-up stays on
`perf/c3-runtime-window-oracle` and the same PR. No race rerun, timeout increase,
CI change, workflow dispatch, activation, merge to main or release is performed.
The earlier global application race timeout remains an incomplete gate.

## Actual stage-two boundary

[Design section 5.3](../design/performance-optimization-plan.zh-CN.md#53-分阶段接入与-cas)
limits initial C3 acceleration to same-day no-op and publication recheck. A full
financial capture on clock-only calls is permitted until D/E. C3 does not
require a new dirty/suffix planner, shared financial fact storage or complete
clock-only incremental computation. Existing fixed-window correctness remains
relevant; optional full/bounded runtime diagnostics must not be mistaken for a
mandatory C3 exit requirement. Default activation and native p95/RSS acceptance
are separate gates from implementing and reviewing the C3 path.

This follow-up reduces repeated C3 verification and provides a non-laundering
bootstrap for a restricted existing/restarted state space. States whose frozen
history cannot be independently authenticated retain explicit full fallback.
This is an independently reviewable checkpoint, **not completion of stage-two
rollout acceptance**. No D/E/G/H work or new incremental planner is included.

## Reproduced integrity finding and fix

The review's two trigger scenarios were reproduced using ordinary tests on an
immutable archive of exact head `0aa192f4a292eee6d17c8781ad615795369ebb12`, plus the
new regression fixture. No existing branch was edited for reproduction.

* An unknown permanent trigger installed before verified Open corrupted an old
  daily JSON row during a control self-update. Both the first and second no-op
  were accepted by the old code, retaining integrity authority.
* A TEMP trigger on review control updates changed a retained **resolved**
  episode's opening event. Legitimate ack, rule and suppression commands each
  re-signed the corruption. Dropping the TEMP trigger before the next capture
  did not remove the laundered authority. The permanent-trigger variants also
  reproduced. Full preparation does not rewrite this retained event, so the
  fixture cannot hide the defect by repairing a current daily row.

The certified verifier now checks an exact immutable contract for **all**
permanent triggers: financial source and reconciliation trigger names/bodies,
with no unrecognized trigger allowed. A schema cookie recorded by Open is not
sufficient. The compiled contract copies the existing source DDL map rather
than mutating that shared registry. TEMP object rejection remains.

Both before/after handling hooks verify schema, source tracking and the complete
writer contract before preserving/reissuing authority. If those assumptions
are unknown, the legitimate handling mutation may proceed, but cannot preserve
certified authority. A changed protected seal then prevents recertification,
even after the fault trigger has been dropped.

[Before-fix reproduction](artifacts/c3-followup-2026-10-19/trigger-before-0aa192f.log)
fails with both accepted no-ops and all six laundering cases.
[Focused after-fix tests](artifacts/c3-followup-2026-10-19/trigger-focused-after.log)
pass. The ordinary [certified suite](artifacts/c3-followup-2026-10-19/certified-unit-after-trigger-fix.log)
also passes (application 14.839s, SQLite 0.114s). No race was run for this change.
These are synthetic fixtures with a synthetic provider key and no provider
network or real ledger/credential access.

## Complete proofs with less repeated work

`eb61000` changes the complete ordered row seal to an injective binary encoding:
table/row/type tags, arities and byte lengths preserve every SQLite value and
boundary without JSON-escaping large serialized evidence again. A reusable
single-row buffer replaces per-row JSON allocations. This is not a ledger cache;
the full seal still reads every protected column when continuity is unknown.
The row encoding was introduced with `certified-full-review-v2`; the complete
protocol is now `certified-full-review-v3` after the commit fence below. Old
markers grant no authority. Unit cases distinguish null/empty/text/blob/integer/float/bool,
ambiguous tuples and embedded zero bytes, and reject unknown driver types.

Capture may retain the previous full seal only when its private signature is
valid and the signed counters equal the same physical connection's counters
sampled **before** the first source read. Publication checks counters before
rescanning protected rows: exact equality proves the captured complete snapshot
unchanged; changed/unknown counters still require the complete seal and original
full financial recheck. Rollbacks, metadata rewinds and other connections remain
covered by the existing counter/source tests. C1's complete tracking state and
exact permanent/TEMP writer contract are still verified in each transaction.

The all-trigger contract already verifies each financial trigger body, so its
same-transaction C1 verification avoids repeating those individual trigger
queries. Certified capture now performs this verification once before lineage;
publication similarly reuses only a result obtained within its own transaction.
Standalone C1/Open/shadow callers retain the original complete verifier. No
schema-cookie or process-wide tracking-verification cache is introduced.

## Same-connection DDL fence

Review of the counter premise found another real gap: `total_changes()` does
not count some DDL, and a writer can restore the writable schema cookie.
The previous same-connection counters could therefore remain equal across a
DDL commit. An immutable archive of `605156b`, plus the new regression fixture,
reproduces all three effects: accepting a no-op after DDL, selecting source CAS
after DDL between capture/publication, and signing lost retained resolved-event
history after its table was dropped/recreated with the original DDL/indexes.
The fixture asserts that both old counters and the restored cookie are equal;
it does not simulate this by editing the certificate. A rolled-back DDL
transaction leaves the unchanged projection authoritative.

`6861411` adds a process-private, saturating atomic commit sequence on the actual
physical connection, using the pinned driver's `RegisterCommitHook` capability.
It complements `total_changes()` (including rolled-back DML) and `data_version`
(other-connection commits). SQL cannot rewind this Go counter. It is sampled
before source reads and signed into capture/publication continuity. Mandatory
publication writes predict exactly one COMMIT, and the callback increments only
that atomic counter and returns success. It issues no SQL or connection calls,
as required by [SQLite's commit-hook contract](https://www.sqlite.org/c3ref/commit_hook.html).
A callback before a failed commit can only conservatively invalidate continuity.

The candidate owns this driver's single commit-hook slot; repository audit found
no other hook registration. Unknown driver capability, physical identity or a
saturated sequence selects full source/publication with no integrity authority.
The benchmark decorator forwards the real capability instead of hiding it.
Normal shutdown unregisters the current live callback; the callback captures
only its atomic counter, not a DB handle or financial data. Default C2 never
installs a hook. This is a correctness fence, not source write-trigger tuning.

Handling transactions can use another pooled physical connection, so they now
preserve their verified full row seal while clearing connection continuity.
They already require a full reconciliation via `CertifiedNeedsReconcile`; only
that publication may establish a new connection proof. They do not manufacture
an identity for a different pooled transaction.

[Before-fix DDL regression](artifacts/c3-followup-2026-10-19/ddl-before-605156b.log)
fails three cases; [after-fix regression](artifacts/c3-followup-2026-10-19/ddl-after.log)
passes all four (including rollback). The ordinary certified suite also covers
unknown identity, missing driver hook and saturated commit counter, all retaining
successful full publication without a MAC. No race was rerun.

## Rejected provenance bootstrap (historical experiment)

`27ad1b9` permits a new handle to establish authority only by **complete
replacement from full preparation**, subject to all these gates:

* Verified tracking, source capture signature, stable physical connection,
  explicit provider lease and all ordinary source/configuration/temporal CAS
  checks succeed under the publication write reservation.
* No episode or episode event exists, including resolved episodes. Every rule
  is an enabled, unarchived daily rule with exactly one condition version (1).
  Legacy/extra rules, disabled control and `needs_repair` state decline.
* Preparation is full, with no reusable window or bounded/full-oracle stage.
  The bootstrap flag is signed in the source proof and authorizes no reuse or
  no-op before the replacement commits.
* Inputs, results, epochs and evaluation state write **every** semantic column
  from that full preparation, including scalar columns that ordinary conditional
  UPSERTs might leave unchanged. Input/result timestamps are regenerated;
  evaluation publication counters advance normally; `dirty_from` is cleared.
* Under the same reservation, the exact four derived key sets match the full
  publication: no extra policy, timezone, condition, target or epoch row may be
  retained and certified. Unknown extra rows are retained with no authority;
  no deletion/reset is used to make this check pass.

Only then may the new private key sign the resulting full projection in the same
atomic control/checkpoint publication. A new handle first rejects the old MAC;
it never imports authority or a secret from the ledger. Subsequent same-day
confirmation is allowed only after this full replacement commits. Newly opened
episodes generated by that fresh full preparation have known provenance.

Existing/restarted ledgers with open/resolved/frozen history or historical rule
versions remain full fallback. The original facts needed to independently replay
frozen historical evidence can be absent after corrections; preserving old JSON
and hashing it cannot solve that problem. Supporting those states would require
a separately reviewed replay/provenance protocol. This PR deliberately grants
them no shortcut and rewrites no frozen history.

Ordinary tests compare the proposed stage with a separate full preparation
**before** publication, exercise legacy and lost-key restart, repair scalar/JSON
damage in the recomputable namespace, reject extra keys/unsupported rules and
retained resolved-event corruption, and verify source/projection/checkpoint
conflicts plus rollback after the first daily write. The restart fixture checks
that an old valid certificate declines before the new full publication.

## Validation and measurements

Frozen performance/full-internal code and harness:
`6861411f69c914a99e9690fa5facf549bf030d5d`. Subsequent `da6d675` only bounds
optional shutdown cleanup: it skips a busy pool and uses a short acquisition
deadline, so an idle leased connection cannot block `DB.Close`. A focused close
regression and the ordinary certified suite cover that guard; reconcile hot
paths and the measured harness are unchanged.
Main remains `326688ce86d35216a9fe55a3bee63bcbb379683d`; PR119 and PR121's exact
merge commits remain ancestors. PR128 and main's workflows are unchanged.
PR121's minimum captured/rechecked temporal boundary is preserved.

All `./internal/...` ordinary tests passed on `6861411` (application 119.398s,
SQLite 25.367s), as did internal vet/build. After the shutdown guard, certified
ordinary tests pass again (application 11.251s, SQLite 0.319s); vet/build are
checked on that guard too. The focused new DDL/legacy/restart/concurrency cases
are included. No race or native build pass is implied by these commands.

The final serial Linux diagnostics use small **n=3 medians** and large **n=1
individual values**, rather than presenting a single large sample as a p95.
Every pair passes its pre-publication independent full oracle and durable
financial/lifecycle comparison. The normal C2 reference is not forced full;
on a new handle its own restart fence requires full preparation naturally.

| Size / n | Scenario | C2 | Certified |
|---|---|---:|---:|
| Small / 3 | same-day | 33.483ms | 2.915ms |
| Small / 3 | clock-only | 38.739ms | 52.588ms |
| Small / 3 | append | 52.943ms | 53.087ms |
| Small / 3 | correction | 58.908ms | 56.246ms |
| Small / 3 | history-free restart bootstrap | 54.862ms | 66.423ms |
| Large / 1 | same-day | 5.839s | 34.318ms |
| Large / 1 | clock-only | 6.427s | 11.322s |
| Large / 1 | append | 10.433s | 11.228s |
| Large / 1 | correction | 9.892s | 10.351s |
| Large / 1 | history-free restart bootstrap | 8.373s | 8.694s |

Small has 10 instruments, 1 rule, 310 quotes and 100 cash activities; large has
100 instruments, 20 rules, 100,000 quotes and 50,000 cash activities. Both have
30 history days. The bootstrap fixture substitutes first-version no-hit rules
before creating its C2 projection; it is a distinct workload with no lifecycle
history, not a timing comparison against the triggered fixture's bootstrap.
It validates exact durable rows, allowing only the documented additive exact-FX
codec and a checked evaluation publication-counter advance (1 -> 2), plus the
existing timestamp normalization. No financial, rule, policy or evidence fields
are discarded. Warm cases assert zero pair-days and no financial/evidence JSON
reads. Changed paths still check the independent full oracle before publication;
current future-activation correctness is covered by ordinary tests, not a newly
claimed activation latency measurement.

Each log reports SQL executions/returned rows, affected rows, reserved write
time, pair-days and allocations. `allocatedBytes` is cumulative Go allocation,
not RSS. The logged VmHWM includes fixture/clones/oracles and both arms, so it
cannot establish an isolated per-arm RSS result. Original `eb61000` and
`605156b` iteration logs are explicitly named `exploratory-*`; they predate the
commit-sequence correction and cannot justify activation. Cross-commit timing
differences are not a controlled speedup claim.

Reproduce from a separate archive of the frozen source, using Go 1.26.1 and the
same writable cache/module settings recorded in `validation.json`:

```sh
NW_C3_CERTIFIED_EVIDENCE=1 NW_C3_PERF_SIZE=small NW_REVIEW_PERF_SAMPLES=3 \
  go test -tags=perf -p 1 ./internal/application \
  -run '^(TestReviewCertifiedPerformanceEvidence|TestReviewCertifiedHistoryFreeBootstrapEvidence)$' \
  -count=1 -timeout=8m -v
NW_C3_CERTIFIED_EVIDENCE=1 NW_C3_PERF_SIZE=large NW_REVIEW_PERF_SAMPLES=1 \
  go test -tags=perf -p 1 ./internal/application \
  -run '^(TestReviewCertifiedPerformanceEvidence|TestReviewCertifiedHistoryFreeBootstrapEvidence)$' \
  -count=1 -timeout=12m -v
```

[Raw small](artifacts/c3-followup-2026-10-19/perf-small.log),
[raw large](artifacts/c3-followup-2026-10-19/perf-large.log),
[all internal](artifacts/c3-followup-2026-10-19/all-internal.log),
[close/certified checks](artifacts/c3-followup-2026-10-19/close-certified.log),
[validation metadata](artifacts/c3-followup-2026-10-19/validation.json) and checksums
retain source, command and result provenance. Large n=1 is diagnostic only;
neither these results nor the earlier n=3 runs meet native 30-sample acceptance.

## Remaining gates

* **Default activation:** blocked. Production uses C2; no UI/MCP setting or
  default initialization calls the certified candidate. Independent review of
  the trigger, source, temporal, provider, physical-connection, provenance and
  atomic-publication proofs is still required.
* **Performance:** the same-day path improves substantially, but the candidate
  still computes a complete runtime oracle before bounded publication. This is
  additional work on clock-only calls. The full audit has not been removed on
  the strength of favorable no-op timings. An equally strong, reviewed proof
  that avoids that work remains an implementation/performance follow-up within
  C3; it does not authorize a D/E planner. Other measured regressions must also
  be explained before activation.
* **Coverage:** history-free restart/legacy bootstrap is implemented; frozen
  episode/event history and older condition namespaces explicitly decline.
  Broadening that supported space requires independent provenance, not a
  persisted checksum or self-comparison after publication.
* **Race:** the single earlier `53dbd18` run exhausted the application package's
  global 20m deadline; domain/SQLite passed and no data-race warning was seen.
  Overall race acceptance is incomplete. These follow-up changes have ordinary
  coverage only. No race rerun or timeout increase was performed.
* **Native acceptance:** paired 30-sample p95, isolated per-arm RSS and native
  Apple Silicon UI/end-to-end checks remain unmet. Prior Linux native compile
  lacked GTK/WebKit/GLib/libsoup development dependencies; no native pass is
  claimed. Required Mac identity remains `com.nestworth.dev` / `Nestworth.app`
  with explicitly isolated synthetic DB/settings paths.

These gates distinguish the bounded C3 implementation checkpoint from stage-two
acceptance. No merge, activation, CI dispatch or release was performed.
