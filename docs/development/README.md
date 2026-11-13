# Development Documentation

Use this index for current engineering contracts, repeatable workflows, and
scoped implementation evidence. Repository-level rules live in
[`AGENTS.md`](../../AGENTS.md); the [engineering guide](engineering-guide.md)
sets package boundaries and validation expectations.

## Build and acceptance

- [Local Development and Packaging](local-workflow.md) — setup, Wails tasks,
  generated bindings, frontend checks, packaging, and isolated native QA.
- [Real Browser Acceptance](real-browser-acceptance.md) — server-mode launch,
  synthetic fixtures, browser evidence, and native-only limits.
- [Wails Version Upgrade](wails-version-upgrade.md) — coordinated runtime,
  CLI, bindings, build, and native/package validation.
- [Frontend Controls](frontend-controls.md) — shared components, interaction
  rules, accessibility, and enforced checks, including holdings.

## Architecture and feature contracts

- [Local MCP](mcp.md) and [MCP Products](mcp-products.md) — current contract:
  permissions, tools, read and write flows, recovery, and budgets. Stage
  history is in [mcp-changelog.md](mcp-changelog.md).
- [MCP Pagination](mcp-pagination.md) — frozen captures, continuations and retained budgets.
- [Financial Context](financial-context.md) — MCP context authorization and
  disclosure limits.
- [Continuous Backup](continuous-backup.md) — opt-in backup and recovery.
- [Account Reconciliation](account-reconciliation.md) and
  [Historical Reconciliation](historical-reconciliation.md) — transaction,
  evidence, and recovery contracts.
- [Investment Short Names](investment-short-names.md) — current short-name feature
  migration evidence and its current compatibility boundaries.
- [Bank Logos](bank-logos.md) and [Icon Catalog](icon-catalog.md) — asset
  ownership and licensing references.

## Performance evidence

- [Performance Optimization Report](performance-optimization-report.md) —
  implementation choices and limits of historical measurements.
- [Analysis Performance](analysis-performance.md) and
  [Account Value Read Performance](account-value-read-performance.md) — scoped
  implementation evidence, not whole-app performance claims.

Dated implementation records document only their named source revision. Use
current source, tests, and the release record for present behavior and
acceptance status.
