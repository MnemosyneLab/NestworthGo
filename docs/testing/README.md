# Testing guide

Start with the [local workflow](../development/local-workflow.md) for the pinned
toolchain and affected checks. Use isolated synthetic databases/settings and fake
providers; follow [native QA isolation](../development/local-workflow.md#isolated-native-development-qa).
Test code and fixtures are the executable regression authority. This guide is a
map of reusable methods, not a record of prior passes.

## Choose the relevant boundary

| Area | Contract and regression entry points |
| --- | --- |
| Account reconciliation | [Matrix](account-reconciliation.md), [UX and liabilities](reconciliation-ux-and-liabilities.md); application, SQLite, MCP and desktop reconciliation suites |
| Historical values and analytics | [Endpoint reconciliation](historical-reconciliation.md), [analytics golden cases](../design/analytics/analytics-redesign-architecture.md#16-golden-cases-engine-gate), [synthetic analytics seed](../../cmd/analytics-qa-seed/README.md), [query cancellation](historical-query-cancellation.md); application historical/analysis and frontend historical-overview/insights tests |
| Investment review | [Daily policy and source integrity](../design/investment-review.md), [dependency/suffix fallback](../design/review-dependency-suffix.md); application review and SQLite integrity tests |
| Cash, products and upcoming actions | [Cash adjustments/Repeat](../user/cash-adjustments-and-repeat.md), [upcoming contract](../design/upcoming-actions.md); preview/commit/receipt, linked-history and frontend navigation regressions |
| Shared controls and localization | [Control contract](../development/frontend-controls.md); actual resource-bundle locale coverage, keyboard/focus, dirty/pending form and stale-response tests |
| Built-in assistant | [Runtime](../architecture/builtin-ai-runtime.md), [provider module](../../providers/einoopenai/README.md); assistant/builtinai/airuntime suites and `providers/einoopenai/check.sh` |
| MCP | [Protocol](../development/mcp.md), [paging](../development/mcp-pagination.md); MCP HTTP permission, schema, wire-budget, revocation and recovery suites; `tools/check-nestworth-skill.py` |
| Mobile | [Access and recovery](../design/mobile-web-access.md); `internal/mobileapi`, frontend mobile tests and `tools/mobile-confirm-reconnect.py` |
| Backup and migration | [Storage compatibility](../architecture/data-and-ipc-contracts.md#database-support-and-storage-recovery), [backup recovery](../development/continuous-backup.md); SQLite, backup and continuous-backup tests |
| Browser integration | [Real server workflow](../development/real-browser-acceptance.md), [retained harnesses](../qa/README.md) |

Completion gates must cover failure as well as success. AI tests use loopback
fake providers and verify clean EOF/terminal validation before tool dispatch,
exact native history replay, cancellation, bounded retention and safe diagnostics.
Provisional text is not executable authority. MCP reads must preserve missingness,
exact values, scope, fixed expiry and complete continuation semantics.
Financial writes must retain atomicity, review/permission boundaries and durable
operation identity across an unknown outcome; never blindly repeat a write.

For analytics acceptance, exercise complete and missing-price/FX scenarios.
Compare household, account and instrument scope, including native/base
currency and cash inclusion. Check that missing data remains distinguishable
from a measured zero in the summary, charts and data tables.

## Evidence and limits

Record the exact SHA, environment, command, fixture and observed result in the
PR or CI artifact. Keep screenshots/logs out of this tree unless they serve a
maintained guide or an indispensable release record.

Linux, jsdom and browser results do not establish native macOS GUI/IPC, VoiceOver,
window/menu lifecycle, OS file dialogs, signing/notarization, physical mobile
browsers or live-provider compatibility. Verify those separately when relevant.
Published release waivers are not passing checks and do not waive future work;
see the [release records](../releases/README.md).

Use affected ordinary tests first. Race, stress and large benchmarks are opt-in
or belong to `perf`; do not reproduce obsolete full-suite commands from old
reports. Documentation-only changes need link, reference and consistency checks,
not a new application benchmark or native acceptance run.
