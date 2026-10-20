# Nestworth Documentation

This directory documents current `main`. The latest published release is
v0.3.8 (build 9, 2026-10-14); `main` has advanced beyond that release. The
current version/build and implementation status come from source and tests,
while release records describe only published artifacts.

## Documentation map

| Area | Document | Responsibility |
| --- | --- | --- |
| Product | [Product Vision](product/product-vision.md) | Problem, audience, principles, and durable workflows |
| Product | [Product Roadmap](product/roadmap.md) | Current release outcomes and deferred direction |
| Product | [Feature brainstorm](product/feature-brainstorm-2026-09-20.md) | Discussion only; no commitment or feasibility review |
| User | [Historical panorama](user/historical-overview.md) | Read-only A/B categories, evidence and return navigation |
| User | [Cash adjustments and Repeat](user/cash-adjustments-and-repeat.md) | Preview, confirm, edit, and recovery guidance for ordinary cash workflows |
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
| Development | [Development Guide](development/README.md) | Current setup, build, packaging, MCP, backup, and validation procedures |
| Development | [Local Development and Packaging](development/local-workflow.md) | Clean-checkout setup, Wails dev, bindings, app/DMG builds, and release smoke |
| Development | [Real Browser Acceptance](development/real-browser-acceptance.md) | Server-mode launch, isolated synthetic fixtures, cloud-browser evidence, and native-only boundaries |
| Development | [Local MCP](development/mcp.md) | Loopback transport, permission modes, implemented query and write tools, and safety boundaries |
| Development | [Wails Version Upgrade](development/wails-version-upgrade.md) | Version synchronization, binding generation, and native/package gates |
| Release | [Release Index](releases/README.md) | Current published release, historical release contracts, and closeout evidence |
| Release | [v0.3.8 Release Record](releases/v0.3.8.md) | Published scope, source/tag identity, package verification, and acceptance boundaries |
| Release | [v0.3.8 Release Notes](releases/v0.3.8-notes.md) | Published release summary, compatibility, and package/acceptance status |
| Release | [v0.3.7 Contract](releases/v0.3.7.md) | Published release scope, acceptance evidence, and remaining GUI/distribution boundaries |
| Release | [v0.3.7 Release Notes](releases/v0.3.7-notes.md) | Published release summary |
| Release | [v0.3.6 Contract](releases/v0.3.6.md) | Historical scope, acceptance, and release gates for v0.3.6 |
| QA | [Real MCP and skill acceptance](testing/qa-mcp-skill-acceptance-2026-10-09.md) | Scoped Mac mini client acceptance, source SHA, provenance, and exclusions |
| QA | [v0.3.7 package acceptance](testing/qa-v0.3.7-release-package-2026-10-09.md) | Published asset digests and scoped synthetic package checks |
| QA | [v0.3.8 Mac package verification](qa/v0.3.8-release-package-2026-10-14.md) | Published arm64 asset hashes, non-interactive integrity checks, signature status, and owner-deferred acceptance |
| QA | [PR101/PR102 post-merge browser acceptance](qa/post-merge-pr101-pr102-2026-10-17/REPORT.md) | Real backend and cloud-browser results for the merged interaction/review changes; includes one reproducible P2 replay defect |
| Testing | [Testing and Acceptance](testing/README.md) | Reusable validation methods and dated evidence boundaries |
| QA | [QA Archive](qa/README.md) | Selected verification evidence, including unresolved acceptance boundaries |

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
own user-visible flows and interaction state. Maintained product and engineering contracts use English. Existing Chinese
proposal and test-plan material keeps its established language; it is not
current status or a release commitment.

## Engineering and verification

- [Development guides](development/README.md) — setup, packaging, MCP, backup,
  and current engineering procedures.
- [Architecture](architecture/system-overview.md) — runtime, domain model,
  persistence, data, and IPC contracts.
- [Reusable testing guide](testing/account-reconciliation.md) — synthetic-data
  safety and validation methods; dated reports apply only to their named source.
- [Selected QA evidence](qa/README.md) — retained reports with distinct scope or
  unresolved acceptance boundaries.
- [Release records](releases/README.md) — compatibility and published scope.
- [Performance optimization report](development/performance-optimization-report.md)
  — implementation choices and limits of historical measurements.

This is the navigation entry point. Area indexes link maintained contracts;
dated QA and review records are retained only when they provide distinct
evidence or an unresolved gate. Current status comes from source, tests, and the
current release record.

Dated documents under [reviews](reviews/) record historical findings tied to
their named source revisions; they are not current issue trackers.
