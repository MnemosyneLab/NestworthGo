# Nestworth

Nestworth is a local-first personal finance desktop application for building
and maintaining a personal or household balance sheet. It is being rebuilt as
a Go + Fyne application.

## Status

The repository is completing the `v0.1.2` Multi-Currency and Portfolio
milestone. Phases 0–9 are implemented and verified: the Go + Fyne shell now
supports local multi-currency Accounts, Instruments, Holdings, exact
valuation, manual and provider quote evidence, explicit Yahoo instrument
refresh, and Settings-routed Yahoo Finance or Frankfurter FX refresh. The
release remains unpublished pending the remaining Phase 10 accessibility,
signing, and distribution checks; its unsigned arm64 `.app` and DMG have been
built and verified locally.

Activity, History, Analytics, Backup/Restore, Import/Export, synchronization,
and background refresh remain deferred to later releases.

## Run

Requirements: Go 1.26 or newer and a desktop platform supported by Fyne.

```bash
cd /Users/waltwang/Developer/walt/Nestworth-go
go run ./cmd/nestworth
```

Build a local binary:

```bash
mkdir -p bin
go build -o bin/nestworth ./cmd/nestworth
./bin/nestworth
```

Create an Apple Silicon macOS application and DMG:

```bash
./scripts/package-macos.sh
```

The artifacts are written to `dist/macos/` as `Nestworth.app` and
`Nestworth-<version>-arm64.dmg`. They use the migrated app and volume icons.
The current artifacts are unsigned; Developer ID signing and notarization are
separate release steps.

## Technology

- Go 1.26
- Fyne v2.8
- SQLite as the local durable business data source, with versioned bootstrap and integrity checks
- Fyne canvas/custom widgets for current balance-sheet summaries and future financial charts
- Go modules and standard Go tooling

The UI renders authoritative application results and must not open SQLite,
construct SQL, or recalculate financial totals. Financial calculations remain
in the Go domain/application layers.

## Project structure

```text
cmd/nestworth/          Application entry point
internal/app/           Application lifecycle and window setup
internal/ui/            Fyne views, widgets, and chart rendering
internal/domain/        Financial entities and invariants
internal/application/   Use cases and orchestration
internal/infrastructure/Repositories, migrations, media, and providers
scripts/                 Build and packaging workflows
assets/                 Brand artwork and native icon resources
docs/                   Product, architecture, and release documentation
```

## Brand and icon resources

The migrated brand resources are catalogued in [assets/README.md](assets/README.md).
The formal brand files are available under `assets/brand/`; native packaging
icons are under `assets/icons/`. Draft artwork is retained under
`assets/brand/drafts/` and is not used by the application yet.

## Documentation

Start with the [documentation index](docs/README.md), especially:

- [System overview](docs/architecture/system-overview.md)
- [Domain model](docs/architecture/domain-model.md)
- [Engineering guide](docs/development/engineering-guide.md)
- [Release documents](docs/releases/README.md)

Active Go release contracts live in [`docs/releases`](docs/releases/README.md).
Uncorrected documents inherited from the Rust/Tauri/React repository are
isolated in the
[`unreviewed archive`](docs/legacy/rust-tauri-inherited-unreviewed/README.md)
and must not be treated as Nestworth-go implementation evidence or plans.

## License

Nestworth is available under the [MIT License](LICENSE).
