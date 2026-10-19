# Engineering Guide

## Current baseline

Nestworth `0.3.8` is the current release, published 2026-10-14, for this Go
1.26 module, with a Wails v3 desktop shell and a React + TypeScript frontend.
The Go domain and application layers own
financial validation, persistence, valuation, replay, cost basis, gains, and
provider routing. The frontend owns presentation, interaction state,
localization, accessibility, and chart rendering.

## Prerequisites

- Go 1.26 or newer;
- Node.js with pnpm;
- Wails CLI `v3.0.0-beta.32`;
- macOS 27.0 or newer on Apple Silicon, with a macOS 27 SDK, for the primary
  desktop and packaging target.
- Linux compile of `./cmd/nestworth` also needs `gcc`, `pkg-config`,
  `libgtk-4-dev`, `libwebkitgtk-6.0-dev`, and `libsoup-3.0-dev`.

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

`frontend/bindings/` contains only generated, gitignored files. After a clean
checkout, generate it with `wails3 task setup` or `wails3 task
generate:bindings`. The direct frontend `dev`, `build`, `typecheck`, and `test`
scripts regenerate it before importing bindings. Run `generate:bindings`
before `check:bindings`: the check requires existing output and compares it
with fresh generator output. The Wails build tasks generate bindings before
building the frontend.

Run the normal automated checks with a writable cache:

```bash
GOCACHE=/tmp/nestworth-go-0.3.8 go test ./...
GOCACHE=/tmp/nestworth-go-0.3.8 go test -race ./...
GOCACHE=/tmp/nestworth-go-0.3.8 go vet ./...
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

The expected outputs are `dist/macos/Nestworth.app`, the versioned arm64
DMG and ZIP, `dist/macos/SHA256SUMS`, and the standalone skill bundle with
its checksum. Use isolated database and settings paths for a launch smoke;
never point tests at real financial data.

## Repository layout

```text
cmd/nestworth/          Wails process entry point
internal/wailsapi/      Bound services and wire DTOs
internal/application/   Use cases, transactions, and read models
internal/domain/        Entities, value types, invariants, and calculations
internal/infrastructure/SQLite repositories and providers
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

See the [shared-control contract](frontend-controls.md) for component discovery,
date semantics, interaction conventions, enforced boundaries and exceptions.

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
foreign keys and checks structural integrity. New databases use schema 20.
Verified v15–19 databases receive additive upgrades in order: short names (16),
reconciliation (17), reconciliation evidence/revisions (18), investment review
(19), and shadow financial source revision/journal metadata (20). Each step
retains a verified read-only recovery copy. Schema20 keeps the full financial
fingerprint/recheck algorithm authoritative; see the
[source coverage, recovery and rollback contract](../testing/performance-c1-source-coverage-2026-10-19.md).
No other legacy migration is supported.
See [reconciliation storage](account-reconciliation.md#schema-18-backup-and-export). Other older, unversioned, future, or invalid databases remain blocked.
See the [narrow storage upgrade](investment-short-names.md#narrow-storage-upgrade)
for verification, transaction, recovery-copy and v15/v16 backup boundaries. Missing current tables and old
constraints fail verification rather than being repaired. Durable settings and
provider credentials live in SQLite; the separate versioned JSON file stores
presentation preferences.

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

GitHub Actions workflow `.github/workflows/check.yml` contains the automated
gates: gofmt, `go test ./...`, a separate race job, vet, `go build
./cmd/nestworth`, frontend build/lint/typecheck/test, and `git diff --check`.
Those gates run as parallel jobs; the aggregate check names remain `test` and
`race`. Automatic `push` and `pull_request` triggers are paused; see
[ci-pause.md](ci-pause.md). The workflow file itself is manual dispatch only
until that pause is lifted. `.github/workflows/perf.yml` runs the full-scale
profiles (`NESTWORTH_PERF=1`) only on a Sunday 18:00 UTC schedule and on
manual dispatch. Native `.app`/DMG packaging, signing, and notarization stay
manual. Wails v3 is pinned at `v3.0.0-beta.32` (`@wailsio/runtime` at
`3.0.0-beta.32`); upgrade those versions only in a dedicated change.

## Documentation and change gate

Update the owning architecture or product document when a stable contract
changes. Maintained documentation is English. Before committing:

```bash
gofmt -l cmd internal
GOCACHE=/tmp/nestworth-go-0.3.8 go test ./...
GOCACHE=/tmp/nestworth-go-0.3.8 go vet ./...
go build ./cmd/nestworth
git diff --check
```

Record unexecuted manual accessibility, signing, notarization, or publication
checks explicitly; passing unit tests does not prove those gates.
