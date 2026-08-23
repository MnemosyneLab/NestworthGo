# Nestworth Documentation

This directory contains the product, architecture, engineering, and release
documentation for Nestworth. The current repository is completing the
`v0.1.2` Multi-Currency and Portfolio milestone: Go/Fyne views sit above a
local SQLite business database with authoritative valuation, explicit Yahoo
instrument refresh, and Settings-routed Yahoo Finance or Frankfurter FX
refresh. Activity, history, analytics, and recovery automation remain future
work.

## Documentation map

| Document | Responsibility |
| --- | --- |
| [Product Vision](product/product-vision.md) | Product problem, audience, principles, workflows, and boundaries |
| [Product Roadmap](product/roadmap.md) | Release sequence, outcomes, dependencies, and deferred capabilities |
| [v0.1.0 UI MVP](product/v0.1.0-ui-mvp.md) | Implemented UI scope, preference contract, and acceptance evidence |
| [System Overview](architecture/system-overview.md) | Go/Fyne runtime, application layers, startup, and security boundaries |
| [Domain Model](architecture/domain-model.md) | Business entities, financial semantics, validation, and calculations |
| [Data and Application Contracts](architecture/data-and-ipc-contracts.md) | Implemented SQLite schema, transactions, serialization, errors, provider refresh, and media rules |
| [Engineering Guide](development/engineering-guide.md) | Go workflow, repository conventions, tests, and release checks |
| [Release documents](releases/README.md) | Active Go release contracts plus the separately labeled inherited archive |

The filename `data-and-ipc-contracts.md` is retained for link stability from
the source documentation. The Go + Fyne application has no Tauri IPC layer;
the current equivalent is the typed boundary between Fyne views, application
use cases, and domain results.

## Source of truth

When sources disagree, use this order:

1. Current code, manifests, migrations, and tests define implemented behavior.
2. Explicit decisions in current architecture and domain documents define the
   intended contracts for work that has not yet been implemented.
3. Release contracts define product scope and acceptance criteria.
4. The roadmap expresses direction, not implementation evidence.

The documentation does not replace executable validation. The current
repository checks are `go test ./...`, targeted `go test -race`, `go vet ./...`,
`go build ./cmd/nestworth`, formatting drift, and `git diff --check`; release
packaging and accessibility checks remain Phase 10 work.

## Status vocabulary

| Status | Meaning |
| --- | --- |
| `Implemented` | Confirmed in the current repository by code and relevant tests |
| `In progress` | Partially executed; remaining acceptance checks are named explicitly |
| `Planned` | Intended for a future implementation phase |
| `Historical` | Kept only in the unreviewed inherited archive for reference |
| `Deferred` | Intentionally outside the active scope |

Do not use `Implemented` for an aspiration, a schema placeholder, or a copied
release plan. If evidence is incomplete, use `Planned` and state the missing
acceptance condition.

## Maintenance rules

- Keep product intent independent from framework details.
- Keep Go/Fyne implementation guidance in the architecture and engineering docs.
- Give each financial rule one canonical home and link to it elsewhere.
- Keep unreviewed inherited documents in
  `legacy/rust-tauri-inherited-unreviewed`; move only a rewritten and
  revalidated Go/Fyne contract into `releases`.
- Do not claim SQLite, charts, or financial calculations are implemented until
  code and tests prove them.
- Verify relative links, run `git diff --check`, and run repository checks before
  committing documentation changes.
