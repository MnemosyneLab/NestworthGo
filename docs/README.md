# Nestworth Documentation

This directory contains the product, architecture, engineering, and release
documentation for Nestworth. The current repository implements the v0.1.4
Cost Basis and Gain milestone: a Wails v3 (Go + React) shell sits above a
local SQLite schema-6 business database with append-only Activities,
historical valuation snapshots, average-cost replay, realized/unrealized gain,
currency decomposition, Investments, and Analytics. Provider refresh remains
explicit; ordinary history and gain reads use only local data.

## Documentation map

| Document | Responsibility |
| --- | --- |
| [Product Vision](product/product-vision.md) | Product problem, audience, principles, workflows, and boundaries |
| [Product Roadmap](product/roadmap.md) | Release sequence, outcomes, dependencies, and deferred capabilities |
| [v0.1.0 UI MVP](product/v0.1.0-ui-mvp.md) | Implemented UI scope, preference contract, and acceptance evidence |
| [System Overview](architecture/system-overview.md) | Wails v3 runtime, application layers, startup, and security boundaries |
| [Domain Model](architecture/domain-model.md) | Business entities, financial semantics, validation, and calculations |
| [Data and Application Contracts](architecture/data-and-ipc-contracts.md) | Implemented SQLite schema, transactions, serialization, errors, provider refresh, and media rules |
| [Engineering Guide](development/engineering-guide.md) | Go workflow, repository conventions, tests, and release checks |
| [Release documents](releases/README.md) | Active Go release contracts plus the separately labeled inherited archive |
| [v0.1.4 implementation plan](releases/v0.1.4-implementation-plan.md) | Phase commits, shipped scope, and verification evidence |
| [Wails v3 migration documents](migration/README.md) | Frontend/runtime migration from Fyne to Wails v3 + React; Phases 0–8 implemented; public-distribution gates remain |

The filename `data-and-ipc-contracts.md` is retained for link stability from
the source documentation. The Wails v3 application has a real IPC boundary
(`internal/wailsapi` DTOs); SQLite, domain, and application contracts below
that boundary are unchanged.

## Source of truth

When sources disagree, use this order:

1. Current code, manifests, migrations, and tests define implemented behavior.
2. Explicit decisions in current architecture and domain documents define the
   intended contracts for work that has not yet been implemented.
3. Release contracts define product scope and acceptance criteria.
4. The roadmap expresses direction, not implementation evidence.

The documentation does not replace executable validation. The current
repository checks are `go test ./...`, `go test -race ./...`, `go vet ./...`,
`go build ./cmd/nestworth`, formatting drift, `git diff --check`, and the
isolated unsigned arm64 packaging smoke. Manual keyboard/VoiceOver review,
signing, notarization, and artifact retention remain named distribution
follow-ups.

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
- Keep Wails/React implementation guidance in the architecture and engineering docs.
- Give each financial rule one canonical home and link to it elsewhere.
- Keep unreviewed inherited documents in
  `legacy/rust-tauri-inherited-unreviewed`; move only a rewritten and
  revalidated Go contract into `releases`.
- Do not claim SQLite, charts, or financial calculations are implemented until
  code and tests prove them.
- Verify relative links, run `git diff --check`, and run repository checks before
  committing documentation changes.
