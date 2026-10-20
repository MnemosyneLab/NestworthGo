# Direct Agent FX correction effective-date movement

Base: `c14308c16a60746f11f566fd914e5cf5d552a64a` (E1 reverted; chart PR147
preserved). Linux amd64, Go1.26.2; isolated synthetic SQLite databases only.

## Verified premises and scope

`Service.ImportAgentMarketData` calls the production batch repository. Its FX
correction validates the same direct pair, inserts a new quote with supersedes,
and inserts correction provenance; it does not update the old quote.
`selectHistoricalFXQuoteAtMarketDate` admits Agent daily-reference candidates
only when `EffectiveDate <= marketDate`. The dependency planner already touches
both replacement and withdrawn target dates and takes their minimum.

This change removes only the planner's same-date restriction and advances
`ReviewFXDescriptionVersion` to `direct-agent-import-fx-v3`. Old description
versions fail closed. Full OLD/NEW candidate inventories, complete Agent audit
inventory, non-FX invariant and independent journal associations remain required.
All existing publication MAC/CAS, exact bytes/SQL images, row seals, final-clock,
provider and physical-handle fences remain unchanged. No prefix memo, selector,
analysis, replay, schema or journal-format changes.

The existing normal dependency-suffix dispatcher admits this extension by
default only when its complete proof passes. Full recomputation remains available
through `NESTWORTH_REVIEW_FULL_RECOMPUTE=1`. Manual/provider/latest, inverse/cross,
pair/policy/preference changes, mixed changes, complex intermediate corrections,
missing/damaged proof and restart/restore retain full fallback.

## Bounded correctness evidence

Production-import tests use the existing 11-day Oct1–11 fixture, with an
acknowledged episode before correction. The independent full-reference database
performs full recomputation; comparison covers persisted daily financial and
predicate evidence, position epochs, evaluation state, episodes/events and
acknowledgement association, plus daily timestamps. Review publication also
preserves the complete imported Agent audit records.

| Correction | Rule zone | Reused days | Recomputed days | Explicit financial assertions |
| --- | --- | ---: | ---: | --- |
| Oct8 → Oct7 | UTC | 6 | 5 | Oct7 and Oct8 use replacement rate3 |
| Oct8 → Oct9 | Asia/Shanghai | 7 | 4 | Oct8 returns to baseline rate1; Oct9 uses rate3 |
| Oct8 → Oct1 / Sep30 | UTC | 0 | 11 | No reusable prefix; full persisted oracle equality |

Independent negative checks cover a plausible NEW journal date falsely moved
later, forged old target association, forged OLD date claims, supersedes versus
provenance disagreement, retained prior-provenance edits, missing proof, mixed
changes and intermediate correction chains. Existing raw-mutation, restart,
selection/preference, seal/journal damage fallbacks are retained. The final-clock
provenance mutation test now exercises a date-moving correction and verifies
atomic rejection without review writes.

Checks (non-race):

- `go test ./internal/domain ./internal/application -run 'TestReviewFX|TestReviewDependencyFX' -count=1 -v`
- Final changed cases: `go test ./internal/application -run 'TestReviewDependencyFXProductionImportFullOracle/(move-|intermediate-chain)|TestReviewDependencyFXProvenanceFinalFence' -count=1 -v`
- `go vet ./internal/domain ./internal/application`
- `gofmt` on changed Go files; `git diff --check`.

Computation reduction is 6/11 or7/11 financial/predicate days versus full
recomputation. This is a work-count observation, not a latency claim. Complete
source capture, prefix validation, full lifecycle replay and publication fences
remain real work. No race, broad performance matrix, repeated full suite, native
macOS or deployment acceptance was run. The draft PR records the exact head;
CI monitoring belongs to the coordinator.
