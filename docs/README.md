# Nestworth Documentation

This directory is the maintained documentation surface for release `0.2.0`.
It describes the current Wails v3 application and its product contracts; it
does not preserve obsolete implementation branches or visual prototype
bundles.

## Documentation map

| Area | Document | Responsibility |
| --- | --- | --- |
| Product | [Product Vision](product/product-vision.md) | Problem, audience, principles, and durable workflows |
| Product | [Product Roadmap](product/roadmap.md) | Current release outcomes and deferred direction |
| Design | [Design and UX](design/README.md) | Current screen map, interaction invariants, and design handoff rules |
| Architecture | [System Overview](architecture/system-overview.md) | Runtime layers, startup, state ownership, and security boundaries |
| Architecture | [Domain Model](architecture/domain-model.md) | Financial entities, validation, and calculation semantics |
| Architecture | [Data and Application Contracts](architecture/data-and-ipc-contracts.md) | SQLite, transactions, serialization, errors, media, and providers |
| Development | [Engineering Guide](development/engineering-guide.md) | Setup, code rules, tests, packaging, and documentation maintenance |
| Release | [Release Index](releases/README.md) | Release contract and closeout evidence |
| Release | [v0.2.0 Contract](releases/v0.2.0.md) | Scope, acceptance, and release gates for the current line |

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
