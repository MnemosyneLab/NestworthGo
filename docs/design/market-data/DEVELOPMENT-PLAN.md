# Market Data Development Plan

Based on the [Product + Technical Design](./NestworthGo-vNext-Market-Data-Product-Technical-Design.md), revised on 2026-09-10. The design document defines the technical contracts; this document tracks development order and progress.

- Overall status: Not started
- Current step: 01
- Last updated: 2026-09-10
- Current blockers: None recorded

## How to Use

Proceed in order. Update each step's status and the development log before starting dependent steps. Use these statuses: **Not started / In progress / Awaiting acceptance / Completed / Blocked**.

Mark implemented work as “Awaiting acceptance” until its acceptance checks pass. Record unexecuted checks as `NOT RUN`; unit tests do not replace database probes or native desktop acceptance. Commit references are for tracking and do not authorize automatic commits or pushes.

## Development Steps

| Step | Work | Completion criteria | Status |
|---|---|---|---|
| 01 Contracts and fixtures | Fix the clock, timezone and expected amounts; define provider timestamp, price adjustment, completeness and FX mappings; prepare sanitized response fixtures. | The first end-to-end scenario has independent expected values; unsupported inputs produce explicit outcomes. | Not started |
| 02 Persistence and API foundation | Observation revisions, canonical indexes, historical modes and bindings, coverage and dirty generations; historical batch interfaces and Tiingo secret storage. | Migration and offline compatibility tests pass; existing latest/manual behavior is preserved; writes and invalidation are atomic. | Not started |
| 03 End-to-end repair | Coverage/gap and opening-anchor planning, cutoff resolver; Tiingo history → persistence → snapshot rebuild → Analytics update, initially using a manual FX fixture. | Real database probes verify delayed quotes, corrections, interruption recovery and idempotency; amounts, quality and provenance agree. | Not started |
| 04 Provider expansion and sync policies | Yahoo history, Frankfurter v2 and Tiingo latest; historical routing, TTL, no-data expiry, recent-correction checks and Force Recheck. | Provider contract tests and controlled live checks pass; splits/dividends, FX and mode changes preserve the agreed financial semantics. | Not started |
| 05 Sync job lifecycle | Plan preview, job queries, progress events, cancellation, duplicate requests, error backoff and database switching. | Reopening a page restores progress; old jobs cannot write to a replacement database; partial success and blockers are reported accurately. | Not started |
| 06 Unified Market Data page | Merge Instruments / FX Rates; preserve management, search, history and manual entry; integrate source settings and sync progress. | Existing capabilities remain available; Wails bindings, frontend tests and amount/quality display checks pass. | Not started |
| 07 Data Health | Local scanning, root-cause grouping, repair preview, Repair All, actions for missing keys/bindings/manual data and Overview indicators. | Opening the page makes no network requests; executable repairs and manual prerequisites are handled separately; repair results are verified. | Not started |
| 08 Integration acceptance and cleanup | Full regression, upgrade/backup restoration, native Wails, localization and keyboard flows, performance; remove obsolete pages and code. | Acceptance under design sections 63 and 66 is complete, with evidence tied to commits/fixtures; unexecuted gates are not marked as passed. | Not started |

Step 03 is the prerequisite for provider expansion and major UI development. Verify migration, precision and invalidation within their owning steps rather than deferring them to final acceptance. Reference acceptance cases MD-01 through MD-17 in design section 66.

## Development Log

Append a row whenever work progresses. A step may have multiple entries; retain failures and unexecuted checks.

| Date | Step | Work completed / Status change | Verification results and evidence paths | Commit (or Uncommitted) | Remaining issues / Next action |
|---|---|---|---|---|---|
| 2026-09-10 | Planning | Development steps established; implementation not started | NOT RUN (planning document only) | Uncommitted | Start step 01 |

## Release Gates

- [ ] Go, Wails/bindings and frontend tests, typecheck, lint, build, migration and backup/restore checks pass.
- [ ] Quantitative database probes pass with no regressions in Analytics precision, residuals or data quality semantics.
- [ ] Native desktop, secret storage, cancellation/database switching and en / zh-CN / zh-TW flows have passed acceptance.
- [ ] Performance is verified on Apple M3 Pro in normal mode; Linux/single-thread results remain non-blocking references.
- [ ] All unresolved issues and unexecuted checks are recorded; final results satisfy design sections 63 and 66.
