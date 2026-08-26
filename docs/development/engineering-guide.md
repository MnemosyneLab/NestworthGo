# Engineering Guide

## Current status

The repository is a Go 1.26 module with a Wails v3 desktop shell (React +
TypeScript frontend) and the v0.1.4 Cost Basis and Gain implementation. The
quality gate covers exact domain values, SQLite bootstrap and compatibility,
onboarding, portfolio valuation, immutable change effects, replay, historical
snapshots, average-cost gain/decomposition reads, localization, frontend page
tests, and net-worth trends. Public distribution still needs manual
accessibility, signing, and notarization checks.

## Prerequisites

- Go 1.26 or newer
- Node.js with pnpm
- macOS on Apple Silicon for the primary desktop target
- Wails v3 CLI (`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12`)

Dependency versions are defined by `go.mod` and `frontend/package.json`.

## Setup and daily commands

From the repository root:

```bash
go mod download
cd frontend && pnpm install && cd ..
wails3 dev
```

Run the available checks:

```bash
go test ./...
go vet ./...
gofmt -w cmd internal
go build ./cmd/nestworth
cd frontend && pnpm run lint && pnpm run typecheck && pnpm run test
```

For a distributable local binary:

```bash
cd frontend && pnpm run build && cd ..
mkdir -p bin
go build -o bin/nestworth ./cmd/nestworth
./bin/nestworth
```

On Apple Silicon macOS, build the unsigned `.app` and UDZO DMG:

```bash
wails3 task darwin:package:release
```

Output is `dist/macos/Nestworth.app` and
`dist/macos/Nestworth-0.1.4-arm64.dmg`. Developer ID signing, notarization,
and manual accessibility review remain separate distribution gates.

Do not launch destructive reset flows against the
only copy of real financial data. Tests and smoke checks must use temporary or
explicitly isolated application-data directories.

## Repository layout

```text
cmd/nestworth/          Wails v3 process entry point
internal/wailsapi/      Bound services and DTOs for the Wails IPC boundary
internal/domain/        Financial entities and invariants
internal/application/   Use cases and orchestration
internal/infrastructure/Repositories, migrations, media, and providers
frontend/               React + TypeScript UI
build/                  Wails Taskfile packaging assets
docs/                   Product, architecture, and release contracts
```

Keep packages behind narrow interfaces. The UI should depend on application
view models, the application layer should depend on domain and ports, and
infrastructure should implement ports rather than becoming the business layer.

## Financial implementation rules

- Never use `float32` or `float64` for persisted values or financial formulas.
- Keep money, quantity, FX, ownership, and return values as exact decimals.
- Persist canonical decimal strings or an equivalent lossless representation.
- Keep current valuation, Activity posting, historical valuation, and analytics
  as distinct authorities with explicit tests.
- Missing quotes produce incomplete/unavailable results; never substitute zero.
- Multi-row writes validate and commit atomically.
- Posted Activities are immutable; correction and reversal append linked records.
- Recompute derived lots and analytics from authoritative facts rather than
  storing duplicate financial truth.

The [domain model](../architecture/domain-model.md) is the canonical home for
business semantics.

## Frontend UI rules

- Pages render `internal/wailsapi` DTOs and do not open SQLite or construct SQL.
- Long-running reads and provider calls run in Go; the UI consumes Promises and events.
- Chart components own scales, axes, and tooltips, but never compute a financial result.
- Keep reusable visual primitives in `frontend/src/components`; keep financial decisions in Go.
- Add keyboard and accessibility behavior with each interactive feature.

## UI preferences

The current MVP stores presentation preferences at
`<os.UserConfigDir>/Nestworth/settings.json`. The file is schema-versioned,
written with a same-directory temporary file followed by sync and atomic rename,
and created with mode `0600`. Invalid settings fall back to defaults without
opening a future business database. A syntactically valid but unavailable FX
provider is rejected by the application registry and reset to Yahoo at startup.

The supported choices are System/Light/Dark appearance, Nestworth/Ocean/
Forest/Amber/Rose accents, System/English/简体中文/正體中文 language, IANA
timezone, Monday/Sunday week start, ISO/day-first/month-first/localized dates,
24-hour/12-hour time, CNY/USD/SGD/EUR/JPY/HKD/TWD/GBP/AUD display currency,
dot/comma decimal separators, comma/dot/space/apostrophe/no grouping,
0/2/4 decimal places, and the production `frankfurter` FX provider.
Presentation preferences affect display; Frankfurter affects only explicit
user-triggered FX refresh.

## Persistence and schema generations

The current SQLite runtime is implemented in `internal/infrastructure/sqlite`:

1. Inspect schema compatibility before application writes.
2. Reject non-empty older databases without migration or partial writes.
3. Block unsupported future versions without persistent writes.
4. Enable foreign keys and a bounded busy timeout on every connection.
5. Verify schema shape, integrity, and foreign-key consistency on startup.
6. Keep the current schema structural; enforce cross-row business rules in Go transactions.
7. Test create, reopen, integrity, and zero-write failure behavior.

Current persistence and serialization contracts live in [data and application contracts](../architecture/data-and-ipc-contracts.md).

## Testing strategy

| Layer | Required evidence |
| --- | --- |
| Domain | Table-driven validation and calculation tests, including decimal edges |
| Application | Use-case tests with fake repositories and transaction outcomes |
| Infrastructure | Migration, query ordering, integrity, and repository tests |
| UI | Component tests for page state, keyboard-only flows, and locale coverage |
| Integration | Isolated SQLite database plus application-level flows |
| Release | `go test`, `go vet`, format check, frontend lint/typecheck/test, build, and isolated launch smoke test |

Every bug fix should add a regression test at the lowest layer that captures
the violated contract. Tests must use sanitized deterministic data and must not
depend on live market providers.

## Documentation and commit gate

Update architecture/domain docs in the same change as a stable contract. Before
committing documentation or code:

```bash
gofmt -l cmd internal
go test ./...
go vet ./...
go build ./cmd/nestworth
cd frontend && pnpm run lint && pnpm run typecheck && pnpm run test
git diff --check
```

Unsigned arm64 macOS `.app`/DMG packaging is `wails3 task darwin:package:release`.
The checked-in release metadata is v0.1.4/build 1. Signing, notarization,
and manual accessibility review remain separate distribution gates.

For an isolated desktop smoke, set `NESTWORTH_DATABASE_PATH` and
`NESTWORTH_SETTINGS_PATH` to files under a temporary task directory. The app
uses those paths only when explicitly provided; normal launches continue to
use the platform application-data locations.
