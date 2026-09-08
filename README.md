# Nestworth

Nestworth is a local-first personal finance desktop application for building
and maintaining a personal or household balance sheet. The current release
line is `0.3.1`: a Wails v3 desktop shell with a Go backend and a React +
TypeScript frontend.

## Current scope

The application currently provides:

- Household onboarding and a local SQLite business database;
- Accounts, Members, Institutions, Groups, Instruments, and Holdings;
- Multi-currency valuation with explicit user-triggered market-data refresh;
- Immutable financial changes, History Origin, replay, snapshots, and History;
- Average-cost basis, realized/unrealized gain, currency decomposition,
  Return Analysis, and Asset Changes;
- Settings for language, appearance, display formats, window state, and FX
  provider selection;
- Local backup/restore and Accounts/Holdings CSV import/export.

Core browsing and editing are local and do not require registration or a
network connection. Synchronization, direct financial integrations, and
background refresh remain deferred.

## Run locally

Requirements: Go 1.26 or newer, Node.js with pnpm, and a desktop platform
supported by Wails v3. The primary target is macOS on Apple Silicon.

```bash
wails3 task setup
wails3 task dev
```

`wails3 task setup` installs dependencies and regenerates the ignored Wails
TypeScript bindings. `wails3 task dev` starts the Go rebuild loop, Vite, and
the desktop shell. For the complete local development, validation, `.app`, and
DMG workflow, see the [local development and packaging guide](docs/development/local-workflow.md).

Build the canonical desktop binary:

```bash
wails3 task build
```

Build and verify the local macOS application and arm64 DMG:

```bash
wails3 task package:release
```

The release output is `dist/macos/Nestworth.app` and
`dist/macos/Nestworth-0.3.1-arm64.dmg`. Signing, notarization, artifact
retention, and manual accessibility review remain distribution gates.

## Technology

- Go 1.26 and Go modules;
- Wails v3 `v3.0.0-beta.16`;
- React, TypeScript, Vite, Tailwind CSS, TanStack Query/Table, Zustand,
  React Hook Form, Zod, i18next, and Apache ECharts;
- SQLite as the local durable source of financial truth;
- `shopspring/decimal` for exact financial arithmetic;
- Yahoo for instrument quotes and Frankfurter for FX refresh.

The frontend renders authoritative DTOs returned by `internal/wailsapi`. It
does not open SQLite, construct SQL, call providers, or recalculate financial
totals.

## Repository map

```text
cmd/nestworth/          Wails application entry point
internal/wailsapi/      Bound Go services and wire DTOs
internal/application/   Use cases and orchestration
internal/domain/        Financial entities and invariants
internal/infrastructure/SQLite and provider adapters
frontend/               React + TypeScript application
build/                  Wails build and packaging assets
assets/                 Brand artwork and native icon resources
docs/                   Product, design, architecture, development, and release docs
testdata/               Sanitized deterministic compatibility fixtures
```

Read the [documentation index](docs/README.md) for the maintained project
contracts.
