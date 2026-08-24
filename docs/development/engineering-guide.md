# Engineering Guide

## Current status

The repository is a Go 1.26 module with a Fyne v2.8 desktop shell and the
v0.1.4 Cost Basis and Gain implementation through Phase 9. The quality gate
covers exact domain values, SQLite bootstrap and compatibility, onboarding,
portfolio valuation, immutable change effects, replay, historical snapshots,
average-cost gain/decomposition reads, localization, live Fyne page
transitions, and net-worth trends. Public distribution still needs manual
accessibility, signing, and notarization checks.

## Prerequisites

- Go 1.26 or newer
- macOS on Apple Silicon for the primary desktop target
- Fyne platform prerequisites documented by the target operating system

Dependency versions are defined by `go.mod`. Do not add a second package
manager or a frontend build system without updating the architecture decision.

## Setup and daily commands

From the repository root:

```bash
go mod download
go run ./cmd/nestworth
```

Run the available checks:

```bash
go test ./...
go vet ./...
gofmt -w cmd internal
go build ./cmd/nestworth
```

For a distributable local binary:

```bash
mkdir -p bin
go build -o bin/nestworth ./cmd/nestworth
./bin/nestworth
```

On Apple Silicon macOS, build the `.app` and `.dmg` together:

```bash
./scripts/package-macos.sh
```
The script builds `darwin/arm64`, invokes the Fyne 2.8 packager, stamps the
Bundle ID `com.nestworth.app`, name `Nestworth`, version `0.1.4`, and build `1`,
then creates an unsigned UDZO DMG with an Applications shortcut. Override the
release metadata for a local release build with `NESTWORTH_VERSION` and
`NESTWORTH_BUILD`.
source and preserves `assets/icons/icon.icns` in both the app bundle and DMG
volume.

Do not launch future database migrations or destructive reset flows against the
only copy of real financial data. Tests and smoke checks must use temporary or
explicitly isolated application-data directories.

## Repository layout

```text
cmd/nestworth/          Process entry point
internal/app/           Application lifecycle and window setup
internal/ui/            Fyne views, widgets, and chart rendering
internal/domain/        Financial entities and invariants
internal/application/   Use cases and orchestration
internal/infrastructure/Repositories, migrations, media, and providers
docs/                   Product, architecture, and release contracts
scripts/                Build and macOS packaging workflows
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

## Fyne UI rules

- Views render application results and do not open SQLite or construct SQL.
- Long-running reads and provider calls must not block the Fyne event loop.
- UI callbacks should hand work to an application use case and marshal the
  result back to the UI safely.
- Chart widgets own scales, axes, hit testing, and tooltips, but never compute
  a financial result.
- Keep reusable visual primitives in `internal/ui`; keep financial decisions
  out of renderers.
- Add keyboard and accessibility behavior with each interactive feature.
- Keep the v0.1.0 Preview values visibly labelled and never promote them to
  authoritative application results.

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
0/2/4 decimal places, and the registered `yahoo_finance` or `frankfurter`
FX provider. Presentation preferences affect display; the FX provider affects
only explicit user-triggered FX refresh.

## Persistence and migrations

The v0.1.1 SQLite runtime is implemented in `internal/infrastructure/sqlite`:

1. Inspect schema compatibility before application writes.
2. Snapshot supported older databases with a SQLite-consistent view before migration.
3. Block unsupported future versions without persistent writes.
4. Enable foreign keys and a bounded busy timeout on every connection.
5. Verify schema shape, integrity, and foreign-key consistency on startup.
6. Keep migration SQL structural; enforce cross-row business rules in Go transactions.
7. Test migration, reopen, integrity, and zero-write failure behavior.

Current persistence and serialization contracts live in [data and application contracts](../architecture/data-and-ipc-contracts.md).

## Testing strategy

| Layer | Required evidence |
| --- | --- |
| Domain | Table-driven validation and calculation tests, including decimal edges |
| Application | Use-case tests with fake repositories and transaction outcomes |
| Infrastructure | Migration, query ordering, integrity, and repository tests |
| UI | Widget/model tests for state transitions and chart geometry |
| Integration | Isolated SQLite database plus application-level flows |
| Release | `go test`, `go vet`, format check, build, and isolated launch smoke test |

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
git diff --check
```

The current repository includes an unsigned arm64 macOS packaging workflow.
The checked-in release metadata is v0.1.4/build 1. The package workflow
verifies the app and DMG in an isolated build output. Signing, notarization,
and manual accessibility review remain separate distribution gates.

For an isolated desktop smoke, set `NESTWORTH_DATABASE_PATH` and
`NESTWORTH_SETTINGS_PATH` to files under a temporary task directory. The app
uses those paths only when explicitly provided; normal launches continue to
use the platform application-data locations.
