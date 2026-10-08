# Nestworth Documentation

This directory is the maintained documentation surface for the current
development line `0.3.7` (unreleased, build 8). It describes the current Wails v3 application and
its product contracts; it does not preserve obsolete implementation branches
or visual prototype bundles.

## Documentation map

| Area | Document | Responsibility |
| --- | --- | --- |
| Product | [Product Vision](product/product-vision.md) | Problem, audience, principles, and durable workflows |
| Product | [Product Roadmap](product/roadmap.md) | Current release outcomes and deferred direction |
| User | [Nestworth skill for Codex](user/nestworth-skill.md) | User-level skill installation, updates, MCP connection, and capability boundaries |
| Design | [Design and UX](design/README.md) | Current screen map, interaction invariants, and maintained design contracts |
| Architecture | [System Overview](architecture/system-overview.md) | Runtime layers, startup, state ownership, and security boundaries |
| Architecture | [Domain Model](architecture/domain-model.md) | Financial entities, validation, and calculation semantics |
| Architecture | [Data and Application Contracts](architecture/data-and-ipc-contracts.md) | SQLite, transactions, serialization, recovery, and providers |
| Design | [Account Container Interaction](design/account-container-interaction.md) | Current Account creation, detail, action, and accessibility behavior |
| Design | [History and Form Defaults](design/history-and-form-defaults-ux.md) | Current history, picker, calculation, and form-state behavior |
| Design | [Backup, Restore, and JSON Export](design/backup-restore-and-json-export.md) | Implemented backup, restore, and JSON export |
| Design | [Visual Analytics and Market History](design/visual-analytics-and-market-history.md) | Implemented chart surfaces and planned historical-series work |
| Design | [Analytics redesign](design/analytics/analytics-redesign-architecture.md) | Return Analysis and Asset Changes kernel, projections, and golden cases |
| Design | [Monthly review and data confidence](design/monthly-review-and-data-confidence.md) | Planned as-of review loop; not in the current release |
| Design | [Available Funds, Term Deposits, and Locked Products](design/available-funds.md) | Implemented liquidity estimates, reservations, explicit product operations, and current limits |
| Development | [Engineering Guide](development/engineering-guide.md) | Setup, code rules, tests, packaging, and documentation maintenance |
| Development | [Local Development and Packaging](development/local-workflow.md) | Clean-checkout setup, Wails dev, bindings, app/DMG builds, and release smoke |
| Development | [Local MCP](development/mcp.md) | Loopback transport, permission modes, implemented query and write tools, and safety boundaries |
| Development | [Wails Version Upgrade](development/wails-version-upgrade.md) | Version synchronization, binding generation, and native/package gates |
| Release | [Release Index](releases/README.md) | Release contract and closeout evidence |
| Release | [v0.3.7 Contract](releases/v0.3.7.md) | Current candidate scope, evidence, and release gates |
| Release | [v0.3.7 Release Notes](releases/v0.3.7-notes.md) | Publication draft; pending release gates |
| Release | [v0.3.6 Contract](releases/v0.3.6.md) | Historical scope, acceptance, and release gates for v0.3.6 |
| QA | [Real MCP and skill acceptance](testing/qa-mcp-skill-acceptance-2026-10-09.md) | Scoped Mac mini client acceptance, source SHA, provenance, and exclusions |
| QA | [QA Archive](qa/README.md) | Selected historical verification reports; current release gates remain in the release contract |

## Source of truth

When documents disagree, use this order:

1. Current code, manifests, database schema, and tests define implemented
   behavior.
2. Architecture documents define stable technical boundaries.
3. The release contract defines the current product scope and acceptance
   criteria.
4. The roadmap records direction and deferred work; it is not evidence that a
   feature exists.

## Status vocabulary

| Status | Meaning |
| --- | --- |
| `Implemented` | Confirmed by current code and relevant tests |
| `In progress` | Partially implemented with remaining checks named |
| `Planned` | Intended future work |
| `Deferred` | Explicitly outside the current release |

Every new document should identify its owner, scope, status, and validation
boundary. Keep one canonical description for each financial rule and link to
it from other documents.

Architecture documents own domain and persistence contracts; design documents
own user-visible flows and interaction state. Maintained documentation is
English. Short-lived reviews, implementation notes, and session artifacts do
not belong in this index.
