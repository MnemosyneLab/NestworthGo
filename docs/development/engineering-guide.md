# Engineering Guide

## Current baseline

Nestworth `0.2.1` is a Go 1.26 module with a Wails v3 desktop shell and a
React + TypeScript frontend. The Go domain and application layers own
financial validation, persistence, valuation, replay, cost basis, gains, and
provider routing. The frontend owns presentation, interaction state,
localization, accessibility, and chart rendering.

## Prerequisites

- Go 1.26 or newer;
- Node.js with pnpm;
- Wails CLI `v3.0.0-beta.12`;
- macOS on Apple Silicon for the primary desktop and packaging target.

Dependency versions are defined by `go.mod`, `go.sum`,
`frontend/package.json`, and `frontend/pnpm-lock.yaml`.

## Setup and daily commands

For the complete copy/paste workflow, see the [Local Development and Packaging
guide](local-workflow.md). The short form is:

From the repository root:

```bash
wails3 task setup
wails3 task dev
```

`frontend/bindings/` is gitignored. After a clean checkout, generate it
with `wails3 task setup` or `wails3 task generate:bindings`; direct frontend
`dev`, `build`, `typecheck`, and `test` scripts also generate it when the
expected binding file is missing.

Run the normal automated checks with a writable cache:

```bash
GOCACHE=/tmp/nestworth-go-0.2.1 go test ./...
GOCACHE=/tmp/nestworth-go-0.2.1 go test -race ./...
GOCACHE=/tmp/nestworth-go-0.2.1 go vet ./...
gofmt -l cmd internal
go build ./cmd/nestworth
cd frontend && pnpm run lint && pnpm run typecheck && pnpm run test
```

Generate Wails bindings after changing a bound Go service:

```bash
wails3 task generate:bindings
```

Bindings and frontend bundles are generated artifacts. Do not hand-edit them
or commit them.

Build and verify the primary-target package:

```bash
wails3 task package:release
```

The expected output is `dist/macos/Nestworth.app` and
`dist/macos/Nestworth-0.2.1-arm64.dmg`. Use isolated database and settings
paths for a launch smoke; never point tests at real financial data.

## Repository layout

```text
cmd/nestworth/          Wails process entry point
internal/wailsapi/      Bound services and wire DTOs
internal/application/   Use cases, transactions, and read models
internal/domain/        Entities, value types, invariants, and calculations
internal/infrastructure/SQLite repositories, media, and providers
internal/settings/      Presentation preferences and provider selection
frontend/               React application and generated binding consumer
build/                  Wails Taskfiles and platform packaging metadata
assets/                 Brand and native icon sources
docs/                   Maintained product and engineering contracts
testdata/               Sanitized deterministic compatibility fixtures
```

Keep dependencies pointed inward. The application layer defines orchestration
and ports; infrastructure implements them. Bound services adapt application
results to stable wire DTOs without embedding business rules.

## Financial implementation rules

- Never use binary floating point for persisted values or financial formulas.
- Preserve exact decimal money, quantities, rates, and percentages.
- Missing quotes yield incomplete or unavailable results; never substitute
  zero.
- Multi-row mutations validate and commit atomically.
- Posted financial changes are immutable; correction and reversal operations
  append linked records.
- Derived gain and analytics values are recomputed from authoritative facts.
- Keep current valuation, historical valuation, provider refresh, and replay
  as distinct authorities with explicit tests.

## Frontend rules

- Pages call generated Wails services through the query/mutation helpers.
- Pages do not open SQLite, construct SQL, call providers, or recalculate
  financial totals.
- Query states must distinguish loading, empty, error, unavailable, and stale
  data where applicable.
- Charts map authoritative values to pixels; they do not derive those values.
- Every interactive feature adds keyboard and localized-label coverage.
- After a mutation, invalidate or reload the authoritative query.

## Persistence and settings

The current SQLite schema is verified before business writes. Startup enables
foreign keys, checks structural integrity, and blocks unsupported future or
invalid databases without partial writes. Settings live separately in a
schema-versioned JSON file and contain presentation preferences plus explicit
FX-provider routing.

## Validation strategy

| Layer | Required evidence |
| --- | --- |
| Domain | Decimal edges, invariants, and deterministic calculations |
| Application | Use-case validation, transaction outcomes, and read models |
| Infrastructure | Schema, repository ordering, integrity, and provider bounds |
| Frontend | Page state, localization, error handling, and keyboard flows |
| Integration | Isolated SQLite data plus application-level workflows |
| Release | Build metadata, package smoke, and named manual gates |

Use sanitized deterministic fixtures. Provider tests use fakes and must not
depend on live network responses.

## Documentation and change gate

Update the owning architecture or product document when a stable contract
changes. Before committing:

```bash
gofmt -l cmd internal
GOCACHE=/tmp/nestworth-go-0.2.1 go test ./...
GOCACHE=/tmp/nestworth-go-0.2.1 go vet ./...
go build ./cmd/nestworth
git diff --check
```

Record unexecuted manual accessibility, signing, notarization, or publication
checks explicitly; passing unit tests does not prove those gates.
