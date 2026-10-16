# Daily investment review implementation checkpoint

## Source and baseline

The authoritative implementation design is Library item
`libfile_da0cbd81b3c08191a4bdc7c9b6bee201`, current version 2, design 1.2,
114254 bytes, SHA256
`acddcc6e6eb82806c1d67156dd89d2e4e9b63af0a5e337df56a228ce1c34e56e`.
It was materialized and read in full. This checkpoint starts at main commit
`d23c01f4f9b59b64b7f83d4b246a8d6b37252f91`, including PR97.

## Implemented checkpoint

`internal/domain/review_daily_policy.go` adds a pure, currently unconnected
daily-policy kernel. It contains three-valued AND/OR, trigger-policy validation
and normalization, and date-keyed single/consecutive/rolling window evaluation.
Missing days are unknown. The window ends at the caller-supplied latest closed
civil date, never at the last available quote. Each date contributes at most
one vote; revisions replace that date's evidence. A sufficient known rolling
hit count wins even when other dates are false or unknown. The reported
consecutive count is the trailing true suffix within this policy's window.

This is **not** the complete rule engine or a user-visible feature. The caller
must establish the fixed rule timezone, closed date, real historical daily
predicate and positive-quantity gate. No current holding is used to construct
history here. No application wiring, schema change, legacy conversion, or
publication is included. The legacy evaluator remains unchanged.

Synthetic domain tests cover the three-valued truth tables, same-day repeated
evaluation, window expiry without new quotes, late replacements, parameter
normalization, unknown dates and exclusion of today's preview. An exhaustive
three-day oracle compares uncertain policies with every binary completion.
These tests establish only the pure kernel's behavior, not historical replay,
instance identity, persistence, or native UI acceptance.

## Remaining implementation sequence

1. Typed/versioned RuleSpec, bounded AST, strict decode and semantic hashing;
   decimal thresholds, canonical four-place Money comparison, daily evidence
   and the shared performance resolver with independent legacy admission.
2. Pure historical inputs with proven quantity/cost/metadata coverage, forward
   checkpoints, authoritative effects and atomic position epochs. FX admission
   must keep date-labelled references separate from timestamped observations.
3. Schema19, verified recovery copies, explicit 15–19 verification/upgrade and
   backup chains, lossless legacy isolation and mutation receipts. No schema
   upgrade should be shipped before this entire compatibility gate passes.
4. Bounded reconcile, authoritative source-token revalidation, complete atomic
   publication, cancellation, corrections, stable episodes and ack/suppression
   preservation. U→T without intervening proven policy F reuses the episode.
5. Wails/MCP read/write separation, grouped pagination, coverage, exclusive
   backup gates and legacy adapters. General JSON export stays format v2.
6. Dedicated review UI, compact localized actions, rule editor/preview,
   suppression management, focus regression and scheduling. Complete the
   design's acceptance matrix and performance fixtures before readiness.

## Parallel frontend ownership

The review task may change only review integration in
`UpcomingActionsPage.tsx`, `overview/OverviewPage.tsx`, and the backward-compatible
optional disabled/pending API of `ui/action-menu.tsx`. None is changed in this
checkpoint. Global navigation, App, shared asset details and public filter
context remain with the overall interaction task.

Proposed minimum navigation contract for that task:

```ts
{ page: "upcoming-actions"; tab?: "recent" | "investment-review" }
```

App should pass this context to the retained page; review integration will
select the matching Tab and route the single PageChrome refresh to the active
Tab's explicit local reconcile command. Missing tab preserves the recent
default. Navigation context must remain available for back navigation.

## Verification boundary

All future tests use synthetic or isolated inputs. Local application identity
remains `com.nestworth.dev` / `Nestworth`; no separate QA identity is permitted.
No real ledger has been opened. Native macOS Wails, VoiceOver, IME, restart,
backup restore and packaging checks remain unverified. Domain checks on Linux
do not satisfy those gates. This checkpoint is suitable only for draft review;
it is not ready for merge or release.
