# Nestworth Go

Nestworth is a local-first personal finance desktop application built with Go
and [Fyne](https://fyne.io/).

## Requirements

- Go 1.26 or newer
- A desktop platform supported by Fyne

## Run

```sh
go run ./cmd/nestworth
```

## Project structure

```text
cmd/nestworth/          Application entry point
internal/app/           Application lifecycle and window setup
internal/ui/            Fyne views and widgets
internal/domain/        Financial entities and invariants
internal/application/   Use cases and orchestration
internal/infrastructure/Storage, migrations, and integrations
```

The initial UI is intentionally a small shell. The next implementation steps
are the domain model, SQLite persistence, and the application use cases that
feed the dashboard and charts.
