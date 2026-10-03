# Nestworth

Nestworth is a local-first personal finance desktop application for building
and maintaining a personal or household balance sheet. The current development
line is `0.3.6` (build `7`, unreleased): a Wails v3 desktop shell with a Go backend and a
React + TypeScript frontend.

## Current scope

Nestworth provides a local household balance sheet with:

- Household Overview, net-worth history, account and ownership breakdowns, and
  actionable Data Health for missing valuation or market-history inputs;
- Members, Institutions, Groups, Accounts, Instruments, Holdings, multi-currency
  cash, and a local activity ledger with History and correction/reversal flows;
- Average-cost basis, realized/unrealized gain, currency decomposition,
  Return Analysis, Asset Changes, and contribution detail;
- Available Funds estimates with source-specific access rules and reservations,
  plus managed term deposits and locked products with explicit valuation,
  receipt, redemption, renewal, and undo workflows;
- Local price/history sources for Yahoo, Tiingo, CoinGecko and Agent-supplied
  observations, plus Frankfurter FX; external refresh is user-triggered;
- Local SQLite backup/restore and versioned JSON export for external tools;
  JSON export is not a restorable backup;
- Optional continuous SQLite backup to Cloudflare R2 while the app runs, with
  remote-confirmed status and explicit recovery; see the
  [backup notes](docs/development/continuous-backup.md) before enabling;
- A loopback MCP interface with read-only, directory-maintenance, and
  ledger-writing permission modes, and an optional Nestworth skill for Codex.

Core browsing and editing work locally without registration. The app does not
place brokerage orders. Multi-device synchronization, direct bank/brokerage
integrations and automatic market-data refresh remain outside this release.
Continuous backup and whole-stream history cleanup are off by default. Optional
30/90-day cleanup protects the current stream and two verified sealed survivors;
it does not bound storage for a long-running stream. Live R2 and latest Mac GUI
acceptance are user-reported as passed; fresh release packaging remains pending.

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

Build and verify the local arm64 macOS application, DMG, ZIP, checksum
manifest, and standalone user-skill bundle:

```bash
wails3 task package:release
```

The default outputs are `dist/macos/Nestworth.app`,
`dist/macos/Nestworth-0.3.6-arm64.dmg`,
`dist/macos/Nestworth-0.3.6-arm64.zip`, `dist/macos/SHA256SUMS`,
`dist/skills/nestworth-skill.tar.gz`, and its `.sha256` file. The app is
ad-hoc signed for local launch. Developer ID signing, notarization, artifact
retention, packaged-artifact acceptance, and manual accessibility review remain separate
distribution gates.

## Technology

- Go 1.26 and Go modules;
- Wails v3 `v3.0.0-beta.26`;
- React, TypeScript, Vite, Tailwind CSS, TanStack Query/Table, Zustand,
  React Hook Form, Zod, i18next, and Apache ECharts;
- SQLite as the local durable source of financial truth;
- `shopspring/decimal` for exact financial arithmetic;
- Yahoo, Tiingo, and CoinGecko for instrument data;
- Frankfurter for FX refresh.

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

## Use with Codex

The optional [Nestworth skill](docs/user/nestworth-skill.md) guides a local Codex
client through the App's MCP for accounts, investments, recording transactions,
reconciliation, data health and analysis. Install/update from a checkout with
`bash tools/install-nestworth-skill.sh`, or use a standalone release skill bundle.
This does not enable MCP or grant permission; connect through the App's
**Settings → AI / MCP** separately. Maintainers can build the independent bundle
with `python3 tools/package-nestworth-skill.py`; no skill is installed by a build.

## Database support

The app creates new databases with SQLite schema 15 and opens existing
databases only when they already use schema 15. The schema-11 database from
v0.3.4 is not automatically upgraded; older, unversioned, or future schemas are
rejected without migration, deletion, or recreation. Preserve the original
database and backup. A restore accepts only a current-schema backup that passes
read-only verification. JSON export is a structured external copy, not a
restorable backup. CSV import and export are no longer available.
