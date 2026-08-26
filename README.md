# Nestworth

Nestworth is a local-first personal finance desktop application for building
and maintaining a personal or household balance sheet. The desktop shell is a
[Wails v3](https://v3.wails.io/) application: a Go backend plus a React
TypeScript frontend.

## Status

The repository implements the v0.1.4 Cost Basis and Gain milestone on a Wails
v3 shell. Local multi-currency portfolios, immutable change effects, History
Origin, replay, historical snapshots, average-cost capture and replay,
realized/unrealized gain, currency decomposition, Investments and Analytics
views, and the associated integrity/privacy regression suites remain in Go.
The Fyne UI was retired at Wails cutover. Public distribution remains pending
manual accessibility review, Developer ID signing, notarization, and artifact
retention. Packaging parity (unsigned macOS `.app` / DMG via the Wails
Taskfile) is the next migration phase.

Backup/Restore, Import/Export, synchronization, and background refresh remain
deferred to later releases.

## Run

Requirements: Go 1.26 or newer, Node.js with pnpm, and a desktop platform
supported by Wails v3 (primary target: macOS on Apple Silicon).

```bash
cd /Users/waltwang/Developer/walt/Nestworth-go
wails3 dev
```

Build a local binary after the frontend production bundle exists:

```bash
cd frontend && pnpm run build && cd ..
mkdir -p bin
go build -o bin/nestworth ./cmd/nestworth
./bin/nestworth
```

Create an Apple Silicon macOS application via the Wails Taskfile (Phase 7
packaging parity; unsigned, same as the former Fyne packager):

```bash
wails3 build
```

## Technology

- Go 1.26
- Wails v3 (`v3.0.0-beta.12`)
- React + TypeScript + Vite frontend
- SQLite as the local durable business data source, with versioned bootstrap and integrity checks
- Apache ECharts for net-worth trend rendering
- Go modules and standard Go tooling, plus pnpm for the frontend

The UI renders authoritative application results and must not open SQLite,
construct SQL, or recalculate financial totals. Financial calculations remain
in the Go domain/application layers. Values cross the Wails boundary as DTOs
from `internal/wailsapi`.

## Project structure

```text
cmd/nestworth/          Wails v3 application entry point
internal/wailsapi/      Bound Go services and DTOs for the Wails IPC boundary
internal/domain/        Financial entities and invariants
internal/application/   Use cases and orchestration
internal/infrastructure/Repositories, migrations, media, and providers
frontend/               React + TypeScript UI
build/                  Wails Taskfile packaging assets
assets/                 Brand artwork and native icon resources
docs/                   Product, architecture, and release documentation
```

## Brand and icon resources

The migrated brand resources are catalogued in [assets/README.md](assets/README.md).
The formal brand files are available under `assets/brand/`; native packaging
icons are under `assets/icons/`. Draft artwork is retained under
`assets/brand/drafts/` and is not used by the application yet.
