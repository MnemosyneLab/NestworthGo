# QA Archive

This index points to selected verification evidence. Reports apply only to the
source revisions and scopes they name; they are not a live status board. Use
the [current published v0.3.10 release](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.10) for published scope and
open acceptance boundaries.

## Product validation

- [v0.3.10 published validation and acceptance limits](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.10) — Published 2026-10-28 with skill 2.0.0; package/archive startup evidence, known race-shard timeout, signing limits and owner-waived GUI gates.

- [v0.3.9 Mac package verification — 2026-10-21](v0.3.9-release-package-2026-10-21.md) — Native package and five-asset integrity checks; owner-waived signing/notarization and deferred native acceptance remain explicit.

- [v0.3.8 Mac package verification — 2026-10-14](v0.3.8-release-package-2026-10-14.md) — Asset/package integrity checks; native acceptance is explicitly owner-deferred.
- [PR101/PR102 post-merge browser acceptance — 2026-10-17–18](post-merge-pr101-pr102-2026-10-17/REPORT.md) — Real Wails server and cloud Chromium against isolated synthetic households; partial pass with a reproducible investment-review replay defect. See the maintained [real-browser workflow](../development/real-browser-acceptance.md).

Older analytics run logs and screenshots were removed after current design,
tests, and later post-merge verification became canonical. They were tied to
obsolete source revisions and did not establish current release or native
acceptance.

## Evidence by feature

These reports retain source revisions, fixtures, scripts/screenshots, measured
results or unresolved acceptance limits that are not replaceable by a current
implementation summary. They are historical evidence, not current bug lists or
instructions to rerun obsolete commands. Use the development guides for today's
toolchain and the report's named revision to reproduce historical results.

| Area | Evidence |
| --- | --- |
| Assistant | [Read-only runtime](builtin-ai-readonly-2026-11-04/README.md), [page guide](builtin-ai-page-guide-2026-11-04/README.md), [sidebar](ai-sidebar-chat-2026-11-05/README.md), [streaming presentation](ai-live-2026-11-05/README.md), [internal page capture](internal-page-content-2026-11-05/README.md) |
| Mobile | [Initial scope](mobile-web-v1-2026-10-30/README.md), [LAN setup](mobile-simple-lan-2026-11-03/README.md), [HTTP boundary](mobile-http-lan-2026-11-03/README.md), [remembered login](mobile-remember-login-2026-11-04/README.md), [confirmation reconnect](mobile-confirm-reconnect-2026-11-04.md), [task summary](mobile-task-summary-2026-11-04/README.md) |
| Navigation and controls | [Desktop history](navigation-history-2026-11-04/README.md), [return entries](navigation-returns-2026-11-04/README.md), [settings tabs](settings-tabs-2026-10-07/README.md), [keyboard boundaries](settings-keyboard-2026-10-08/README.md), [interaction preview](interaction-web-preview-2026-10-16/README.md) |
| Financial UI | [Investment workflows](investment-workflows-2026-10-27/REPORT.md), [return analysis](pr195-return-analysis/verification.md), [review editor](investment-review-daily-2026-10-16/README.md), [review integration](pr98-pr99-main-sync-2026-10-17/README.md), [second review batch](review-batch-two-2026-10-19/README.md), [remaining browser checks](atlas-remaining-2026-11-04/README.md) |
| MCP | [Audit follow-ups](mcp-audit-followups-2026-11-04.md), [compact reads](mcp-compact-reads-2026-11-04.md), [history windows](mcp-history-window-2026-11-04.md), [import comparison](mcp-import-comparison-2026-11-04.md), [target pushdown](mcp-target-pushdown-2026-11-04.md), [pagination](mcp-unified-pagination-2026-11-04.md) |
| CI | [Budget and coverage](ci-five-minute-budget-2026-11-02.md), [frontend readiness](frontend-ci-readiness-2026-11-02.md) |
| Assets | [Bank logos](bank-logos/README.md), [symbols](bank-logos/symbols/README.md), [icon coverage](icon-coverage/README.md) |

Other retained review evidence: [October correctness fixes](../review-2026-10-02.md),
[Insights UX](../reviews/2026-09-19-insights-ux.md),
[September full-app findings](../reviews/2026-09-20-full-app-review.md), and
[localization](../reviews/localization-2026-10-30.md). Their unverified findings
must be rechecked against current source before treating them as open or fixed.
