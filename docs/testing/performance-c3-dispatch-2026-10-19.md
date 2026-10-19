# C3 full fallback dispatch and complete writer contract

Frozen source: `08a5b0245cdf367f62bbd3563d498ff53f604cf3`.
This follows the [private provenance correction](performance-c3-provenance-2026-10-19.md).
Production remains C2. PR129 stays a default-off draft. This checkpoint does not
claim stage-two rollout acceptance or enable legacy/restart certification.

## Exact FK reproduction and fix

The independent reviewer supplied a feasible internal writer missed by the user
trigger allowlist. Before the first certified capture on a fresh synthetic DB,
the fixture recreates the empty `investment_review_episodes` table with its
original columns/constraints/indexes plus a deferred FK from `first_detected_at`
to checkpoint `evaluated_at`, `ON UPDATE CASCADE`. A unique parent index permits
the FK. It rewinds `schema_version` to the verified cookie. There is no added
user trigger, no external connection and no prior registered commit hook.

On an immutable archive of exact `713224c0b219a2aa9ecc068ba6a6f8e26a36b934`
plus only the regression fixture, first publication signs. Advancing the clock
by 1ns allows the first no-op to report success while checkpoint publication
cascades into frozen `first_detected_at` after its seal/counters were prepared.
The log records the actual old/new detection timestamps and success verdict.

`cf8df85` adds an immutable compiled contract for every current table and explicit
index from the four fresh-schema artifacts. Every certification/no-op/source-CAS
verification and legitimate handling reseal verifies that complete contract in
its own transaction, as well as the existing all-trigger/C1/TEMP checks. Full
SQL definitions cover FK actions, deferral, constraints and partial predicates.
Unknown tables, views or explicit indexes also decline authority, conservatively
including benign schema extensions. Even ANALYZE-created unknown schema objects
may force fallback. A schema cookie is only an additional fence. No actual schema,
index, trigger, persistent secret or migration is installed by this change.

The regression now requires no MAC, refused confirmation and byte-identical
projection rows. Separate infrastructure cases cover unchanged valid schema,
an altered table FK with no extra index, altered partial-index predicate, unknown
table/view/index and a `sqliteX...` name that must not evade classification.

## Missed confirmation performs one full preparation

The ordinary `WithCertifiedReviewCandidate()` constructor no longer implicitly
selects the lineage/window experiment. If same-day confirmation cannot succeed,
capture loads all source facts, marks full recomputation, loads no reusable
window and produces no lineage proposal. Preparation runs the original full
planner once; publication retains certified source CAS, provider lease, complete
source fallback, atomic writes, final injected-clock fence and PR121's minimum
captured/rechecked temporal frontier. The independent test oracle remains a
separate pre-publication full capture, not a second production calculation.

Explicitly combining the constructor with
`WithReviewCompletenessLineageShadow(...)` retains the old bounded experiment,
its full runtime oracle and all its publication comparisons. Existing adversarial
bounded tests select that experiment explicitly. No proof requirement was removed
from bounded publication. `WithReviewLineageObserver(...)` observes existing
decisions without enabling it; the measurement harness uses this observer.

New coverage proves ordinary clock-only dispatch contains all historic days,
has no window/lineage/full-oracle candidate, agrees with an independent complete
oracle, retains verified source CAS and subsequently confirms same-day no-op.
The source/projection/config/provider/floor/clock/cancellation/ignored-update/
aborted-update atomicity matrix and future entry/exit/transfer/clear-rebuy
activation matrix now run with both ordinary full dispatch and explicit bounded
experiments. Existing restart/damage/provenance cases remain in the certified
suite. No race rerun or timeout increase is part of this checkpoint.

## Current-source validation and measurements

All ordinary `./internal/...` tests pass on the frozen source (application
120.623s, SQLite 25.796s), with internal vet/build. The complete certified suite
also passes (application 17.290s, SQLite 0.790s). Formatting and diff checks pass.
No real ledger, provider or credentials are used. The retained
small/large harness uses official Go 1.26.1, SQLite 3.53.0, Linux amd64,
`GOMAXPROCS=4`, shared synthetic financial IDs/data, independent pre-publication
full oracles and paired persisted-output comparisons. Fresh candidate fixtures
are initialized through the private-origin path; no reopened clone is certified.

Small n=3 medians, 10 instruments / 1 rule / 30 history days / 310 quotes /
100 synthetic cash activities, with all full and persisted-output oracles passing:

| Scenario | C2 | Ordinary C3 candidate |
|---|---:|---:|
| same-day | 37.283ms | 4.024ms |
| clock-only | 38.279ms | 53.944ms |
| append | 66.702ms | 66.404ms |
| correction | 56.127ms | 59.508ms |
| distinct restarted full fallback, no authority | 62.698ms | 53.690ms |

The small clock-only regression remains real in these diagnostics (+40.9%).
It compares C2's existing window reuse against C3's conservative full calculation
and publication, not two production calculations. No default-activation success
is inferred from warm speedups or from removing the implicit bounded experiment.

Large n=1 individual diagnostics use 100 instruments / 20 rules / 30 history
days / 100,000 quotes / 50,000 synthetic cash activities. Warm is
5.943869408s / 32.193614ms (C2 / ordinary C3). Clock-only is
5.736763340s / 8.281264838s (+44.4%). C3 calculates/presents 62,000 full
pair-days; C2's existing window presents 2,000. SQL executions are 128,242 /
8,242 (C3 / C2), while affected rows are exactly 6,242 in both arms. This
identifies the remaining conservative full-publication cost rather than another
bounded runtime calculation. Clock-only `TotalAlloc` is 3,370,049,032 /
2,008,283,056 bytes (C3 / C2); these are cumulative allocations, not RSS.

The distinct restarted full-fallback pair is 8.214313014s / 8.220574099s,
explicitly requiring no certification and continued no-op refusal.

| Large scenario / n=1 | C2 | Ordinary C3 candidate |
|---|---:|---:|
| same-day | 5.943869408s | 32.193614ms |
| clock-only | 5.736763340s | 8.281264838s |
| append | 10.096223241s | 8.793442522s |
| correction | 9.082050303s | 8.767084731s |
| distinct restarted full fallback, no authority | 8.214313014s | 8.220574099s |

All independent full and durable-output oracles pass; large package elapsed time
is 413.652s. Whole-process VmHWM reaches 1,038,336kB, including fixture generation,
both arms, clones and oracles; it is not an isolated arm's peak. These results
do not establish a controlled cross-commit speedup against older frozen timings.
Raw logs, commands, exact source and SHA-256 are retained in
[the evidence manifest](artifacts/c3-dispatch-2026-10-19/validation.json) and
[checksums](artifacts/c3-dispatch-2026-10-19/SHA256SUMS).

Reproduction (use the same isolated Go cache/module/temp directories recorded
in the accompanying validation manifest):

```sh
NW_C3_CERTIFIED_EVIDENCE=1 NW_C3_PERF_SIZE=small NW_REVIEW_PERF_SAMPLES=3 \
  go test -tags=perf -p=1 ./internal/application \
  -run '^(TestReviewCertifiedPerformanceEvidence|TestReviewCertifiedHistoryFreeBootstrapEvidence)$' \
  -count=1 -timeout=8m -v
```

Large uses `NW_C3_PERF_SIZE=large`, `NW_REVIEW_PERF_SAMPLES=1`, `-timeout=12m`.
These are diagnostic sample counts, not p95 or isolated per-arm RSS acceptance.
Retain unresolved native/performance/review gates and the earlier incomplete
race gate; none is converted to a pass by this dispatch change.

## Implementation boundary versus activation validation

This slice implements ordinary no-op/full dispatch and complete writer-contract
verification. It deliberately implements neither legacy/restart authentication
nor a cheaper clock-only publication plan. Full financial capture is already
permitted by C3 section 5.3; D/E loading is not needed to explain or validate this
fallback. The separate bounded experiment keeps its existing full runtime oracle.

The measured remaining clock-only work is concrete: 120,000 additional conditional
daily-row SQL executions. The prepared full result is retained as authority and
no current ordinary-C3 proof authorizes omitting those prefix publications.
If further clock-only publication reduction is required within C3, the design
decision is whether to add a separately reviewed, authenticated proof of exact
unchanged stored-prefix publication, binding complete row payloads, key sets,
schema/connection/provider/source/time/handling state and atomic revalidation.
Narrow fingerprints/counts alone cannot authorize it. This checkpoint does not
invent that plan, weaken the existing proof or treat a new suffix planner as C3.

Separately, default activation still needs independent correctness review,
acceptable matched performance distributions and per-arm RSS, native Apple
Silicon verification, and completion of the unresolved race gate outside this
task's explicit no-rerun restriction. Linux native preflight still lacks GTK4,
WebKitGTK 6.0 and libsoup 3.0; internal build success is not a native/root-command
build pass. The small/large regression is disclosed rather than accepted here.
No activation, manual CI dispatch, workflow edit, merge or release occurred.
