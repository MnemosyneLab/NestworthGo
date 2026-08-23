# System Overview

## Current baseline

Nestworth is a local-first desktop application written in Go with Fyne. The
repository contains the v0.1.1 backend foundation, the v0.1.2 portfolio line,
and the implemented v0.1.3 history milestone through Phase 10: typed domain
contracts, SQLite bootstrap and migrations through schema `4`, onboarding,
multi-currency Accounts, Instruments, Holdings, immutable Activities, replay,
historical snapshots, History, Analytics, exact valuation, and explicit
Yahoo/Frankfurter refresh routing. Fyne renders application results; it does
not open SQLite, call HTTP, or recalculate financial totals.

The initial platform targets macOS on Apple Silicon. Fyne keeps the option of
supporting other desktop platforms later without introducing a second UI stack.
Core browsing and editing must remain usable without registration or a network
connection.

## Application layers

```mermaid
flowchart LR
    UI["Fyne views and widgets"] --> App["Application use cases"]
    App --> Domain["Domain model and invariants"]
    App --> Ports["Repository and provider ports"]
    Ports --> Infra["SQLite and platform infrastructure"]
    Infra --> DB[("Local SQLite database")]
```

### Fyne UI

Fyne owns presentation, interaction state, layout, accessibility affordances,
and chart rendering. Views call application use cases and render returned view
models once those layers exist. They must not open SQLite, construct SQL, or
recalculate authoritative financial totals. Chart code maps returned values to
pixels; it does not derive gain, return, allocation, FX, or net-worth totals.
The current Overview is a live application result; the former visual Preview dashboard is retained only for presentation-only controller tests.

### Application layer

Application use cases coordinate validation, authorization to the active
Household, transactions, repository queries, domain construction, and view
model assembly. Overview and portfolio summaries are application-level reads
because they combine several repositories under one consistent snapshot.

The application layer is the only place allowed to coordinate a multi-entity
mutation. Provider calls remain behind interfaces and are never required for
startup, onboarding, ordinary reads, or valuation reads. Only explicit user
refresh actions may invoke a configured provider.

### Domain

The domain owns identifiers, money, quantities, currencies, ownership,
timestamps, account lifecycle, instruments, holdings, quotes, Activities,
History Origin, lots, and financial sign rules. It must not import Fyne, SQL
drivers, or operating-system APIs.

### Infrastructure

Infrastructure owns the database path, connection options, migrations,
integrity checks, media normalization, and platform integration. It implements
interfaces defined toward the application/domain layers and does not decide
product-level validation or presentation.

## Dependency rules

- Dependencies point inward toward the domain.
- Fyne views call application interfaces, never repositories directly.
- Only infrastructure opens or mutates business data.
- Financial formulas execute in Go application/domain code and cross into UI as
  results.
- A multi-row mutation is one application transaction.
- A summary composed from multiple collections uses one read snapshot.
- List implementations use bounded queries and must not query once per result row.
- Raw SQL errors and sensitive local data never reach user-facing UI models.

Stable business semantics are defined in the [domain model](domain-model.md). Current persistence and application behavior are defined in [data and application contracts](data-and-ipc-contracts.md).

## Startup and bootstrap

```mermaid
flowchart TD
    Launch["Launch Fyne application"] --> Create["Create app and main window"]
    Create --> Init["Initialize application state"]
    Init --> Inspect["Inspect local database compatibility"]
    Inspect -->|"New or supported"| Open["Open, migrate, and verify SQLite"]
    Inspect -->|"Unsupported or corrupt"| Blocked["Show safe startup error"]
    Open --> Bootstrap["Load settings and active Household"]
    Bootstrap -->|"No Household"| Onboarding["Show onboarding"]
    Bootstrap -->|"Household exists"| Overview["Show overview"]
    Blocked --> Error["Keep business writes unavailable"]
```

The compatibility inspection, migration, schema verification, and blocked-startup paths are implemented in `internal/infrastructure/sqlite`. Supported older databases receive a consistent pre-migration snapshot; unsupported future versions are rejected before schema writes. The application then bootstraps the active Household, opens onboarding when needed, and renders the live Overview.

## State ownership

| State | Owner |
| --- | --- |
| Durable business data | SQLite through infrastructure repositories |
| Financial calculations | Go domain/application services |
| Navigation and filters | Fyne view state |
| Form input | Feature-owned Fyne widgets and validation models |
| Language, appearance, and FX route | Local JSON settings plus UI theme adapter and application provider selection |
| Chart geometry | UI-only rendering model derived from authoritative results |

The UI must not optimistically invent financial totals. After a mutation it
reloads the authoritative application result.

## Technology decisions

| Area | Choice | Boundary |
| --- | --- | --- |
| Language | Go 1.26 | Application, domain, and infrastructure code |
| Persistence | SQLite v1 | Local durable source of truth |
| Decimal arithmetic | shopspring/decimal-backed domain Money | No binary floating point for financial values |
| Charts | Fyne canvas/custom widgets | Rendering only; no financial calculations |
| Module/build | Go modules and standard Go tooling | `go run`, `go test`, `go vet`, `go build` |

Dependency versions are owned by `go.mod` and `go.sum`, not duplicated here.

## Privacy and security boundaries

- No account registration or required internet connection for core operation.
- Business data remains local and is not sent to Fyne or external providers.
- Provider integrations are explicit adapters with safe failure behavior and
  no startup dependency. v0.1.2 ships Yahoo current quotes and an FX-only
  Frankfurter adapter.
- Logs must not include balances, notes, quantities, account names, instrument
  symbols, quote values, raw legs, credentials, image bytes, or database rows.
- User-facing errors expose stable safe codes; detailed diagnostics stay local.
- Imported media is decoded, bounded, normalized, and stored as Household-scoped
  data rather than exposing arbitrary filesystem paths.

## Evolution rules

Later releases may add providers, imports, or sync, but must preserve these
boundaries:

- The local store remains usable when integrations fail.
- Provider implementations stay behind application interfaces.
- One valuation path supplies current summaries; history reconstructs from the
  History Origin and ordered Activities.
- Analytics remain a read-only interpretation of the ledger.
- Historical facts are appended or explicitly corrected, never silently edited.
- Compatibility checks occur before migration or business writes.
