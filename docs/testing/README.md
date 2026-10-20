# Testing and Acceptance Evidence

Use synthetic databases and isolated settings paths for local validation. Follow
[repository QA isolation rules](../../AGENTS.md) and the
[development workflow](../development/local-workflow.md). Dated evidence applies
only to its named source revision; it does not establish current CI or native
acceptance unless the report explicitly says so.

## Reusable methods and matrices

- [Account reconciliation](account-reconciliation.md) — synthetic regression
  matrix for session, financial-link, recovery, and migration boundaries.
- [Historical reconciliation](historical-reconciliation.md) — accounting and
  replay evidence for historical changes.
- [Analytics test plan](nestworth-analytics-test-plan.md) — reusable Return
  Analysis and Asset Changes coverage matrix; shipped behavior follows current
  architecture and code.

## Dated acceptance evidence

These records document a specific feature or review. Treat pending native or
external-client checks as open until the owning release record confirms them.

- [Investment review daily acceptance — 2026-10-16](investment-review-daily-acceptance-2026-10-16.md)
- [Upcoming actions QA](qa-upcoming-actions.md)
- [Real MCP and skill acceptance — v0.3.7](qa-mcp-skill-acceptance-2026-10-09.md)
- [v0.3.7 package acceptance](qa-v0.3.7-release-package-2026-10-09.md)
- [Historical panorama P1](historical-panorama-p1.md) and [P2a](historical-panorama-p2a.md)
- [Historical query cancellation](historical-query-cancellation.md)
- [Account composition](qa-account-composition.md)
- [Cash adjustments and Repeat](qa-cash-adjustments-and-repeat.md)
- [First increment polish](qa-first-increment-polish.md)
- [Homepage refinement](qa-homepage-refinement.md)
- [Legacy snapshot coverage](qa-legacy-snapshot-coverage-2026-10-08.md)
- [Delayed NAV display](delayed-nav-display-2026-10-13.md)
- [PR91 merge and binding errors](pr91-merge-and-binding-errors.md)
- [Shared-control audit](shared-control-audit-2026-10-13.md)
- [Account and liquidity QA fixes](qa-account-liquidity-2026-10-02.md)

The current post-merge browser result and release package evidence are indexed
under [QA](../qa/README.md). Do not infer acceptance for a later source revision
from these older records.
