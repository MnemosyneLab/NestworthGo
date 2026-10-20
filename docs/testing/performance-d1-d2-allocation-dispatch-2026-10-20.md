# PR131 allocation cleanup and dispatch assessment

Baseline production `dbf0f2df7034b1318b2badff46d5dff590a36367`; final production
`140278e42ed60fc9257612b7eb19a3f41cb8e03e`, retaining user merge057 and
main3d202a2 / PR133. This compares PR131 implementations. **C2/C3 controls here
are explicit opt-in harness configurations, not shipped currentmain defaults.**
PR131 enablement remains separate and unmerged. No cumulative pre-wave
comparison was run; that extra request was withdrawn and assigned to Luna.

## Proof-neutral changes

Publication Days capacity now uses the actual emitted count. Retained output is
counted during the existing daily planning traversal, so no second history walk
is added. It is allocation accounting only: the unchanged publication loop still
checks each authenticated prefix entry, and all SQL attestation, exact coverage,
source/control/checkpoint CAS, physical counters, provider lease, time guards,
final typed-prefix authentication and post-publication full sealing remain.
The count increments at most once per appended planned day; subtraction cannot
produce a negative capacity on a successful plan.

The22 scan destinations and their pointers are allocated once per query instead
of once per row. Each complete SQL image is compared immediately; only decoded
owned evidence and immutable strings leave the iteration. No tuple/scratch slice
is retained in the evidence map or capture. Existing multi-row full/durable
oracles cover the reuse, including exact FX and all three daily timestamps.

Focused SQLite/application tests passed .063/7.374s; final internal vet/build
passed. Selector diagnostic passed .163s. No new default-CI test was added:
selector and optional backfill diagnostics are behind the perf tag and explicit
environment opt-in. No race, workflow edit/dispatch, merge, release, real ledger,
settings or provider-credential operation occurred. Normal-check duration and
per-test comparison are recorded in the PR after the final check.

## Supported selectors and negative routing result

`WithBoundedReviewWindow()` changes query strategy; on a D2 repository it leaves
C3 enabled. Capture still forces certified full preparation unless the existing
lineage/runtime-full-oracle contract permits a bounded proposal. The certified
preparer explicitly constructs the complete full oracle before that proposal;
the selector does not provide the cheap standalone C2 path.

A fresh `NewRepository(db).WithBoundedReviewWindow()` selects cheap ordinary C2.
The opt-in synthetic diagnostic uses only those supported APIs: bounded capture
is selected and its stage matches independent full recomputation, but publication
clears the certified MAC. Returning to D2 then has MayCertify=false and no suffix.
That transition forfeits the existing historical-reuse chain and cannot be
silently repaired under the legacy/restart restrictions. Ordinary C2 also has a
different input codec selection; retaining the exact-FX contract cannot be
assumed by switching constructors.

The small full-C3 control also has different D2 source-description support. Its
n=1 clock/correction/backfill results (46.770/50.257/50.143ms) do not establish a
state-preserving dispatcher or a scale-independent crossover. No guessed size or
dirty-day threshold is introduced. A certified bounded clock-publication
contract that retains historical/lifecycle authority and D2 source descriptions
across transitions would require an explicit larger design decision. This report
does not claim every possible D2-only improvement is exhausted.

**No scenario dispatcher is implemented.** The current small diagnostics provide
no D2-over-C2 benefit for corrections/backfills to justify such a policy. Keep
the enabled safe path and existing full fallback. Rollback remains
`NESTWORTH_REVIEW_FULL_RECOMPUTE=1` before initialization, then explicit reconcile.

## Matched small diagnostics

Same saved unpublished synthetic source facts/UUIDs, separate baseline/candidate
binaries, fresh genesis clones earning independent authority, serial runs,
Go1.26.1/SQLite3.53.0/Linux amd64/GOMAXPROCS4/four-CPU quota. Setup and full/durable
oracles are outside timed reconciliation. Source:10 instruments/holdings,1rule,
30 historical days,310 quotes,100 cash activities. Backfill inserts a new close
observation for one instrument at origin+15days with a later captured timestamp.

Final n=3 matched pairs per scenario; initial and superseded small experiments
are retained and labelled. There were10 matched repetitions total including the
initial diagnostic and two intermediate allocation experiments, no large runs,
no p95/isolated RSS/native acceptance claim. The first implementation added a
second counting pass; final production folds it into existing planning instead.

| Final median, milliseconds | Baseline D2 | Cleanup D2 | Cleanup paired bounded C2 |
|---|---:|---:|---:|
| Clock-only | 59.827786 | 59.458721 | 35.879946 |
| Quote correction | 70.320954 | 74.634303 | 51.546766 |
| Quote backfill | 70.628005 | 68.263886 | 50.867048 |

Clock total allocation median16,395,168→16,148,160 bytes; allocation count
151,595→151,287. Small TotalAlloc values vary with buffer/pool/GC behavior;
this is not peak RSS. The structural allocation reductions and observed clock
count reduction do not establish a stable latency win. Correction/backfill
medians vary in opposite directions; no gains are inferred from reused days.
All independent plan/full and durable-output oracles passed. The ratios and raw
samples are available rather than collapsed into an overall weighted percentage.

[Manifest, complete raw evidence and SHA256SUMS](artifacts/d1-d2-allocation-dispatch-2026-10-20/).
Remaining limitations include the unaccepted end-to-end regression, representative
scale-dependent routing evidence, p95, isolated per-arm RSS and native Mac QA.
Linux native dependency limitations are unchanged; normal CI and native Mac
acceptance remain separate. Atlas review is independent and was not awaited.
