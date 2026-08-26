# Wails v3 Migration Documents

This directory contains the implementation plan for replacing the Fyne
desktop shell with a [Wails v3](https://v3.wails.io/) application: a Go
backend (reusing `internal/domain`, `internal/application`, and
`internal/infrastructure`) plus a React + TypeScript frontend, as selected
in [`prototype/nestworth-wails-frontend-stack.md`](../../prototype/nestworth-wails-frontend-stack.md)
and informed by [`prototype/功能现状与交互设计说明.md`](../../prototype/功能现状与交互设计说明.md).

**Status:** Phases 0–6 `Implemented on 2026-08-26`. The canonical
`cmd/nestworth` entry point is the Wails application; Fyne has been
removed. Phases 7–8 (packaging/distribution parity and release closeout)
remain `Planned`. Rollback is now "revert the cutover commit(s)," not
"resume a parallel Fyne build."

## Documents

| Document | Owns |
| --- | --- |
| [Migration plan](wails-v3-migration-plan.md) | Purpose, goals, non-goals, scope boundaries, locked decisions, and acceptance criteria — the "contract" for the migration |
| [Technical design](wails-v3-technical-design.md) | Target architecture, project layout, Go service/binding design, the serialization and error contract across the Wails IPC boundary, frontend architecture, build/packaging, and testing strategy |
| [Implementation plan](wails-v3-implementation-plan.md) | Phased delivery order, deliverables, required checks, exit checks, risk register, and rollback plan |

## Reading order

1. Read the [migration plan](wails-v3-migration-plan.md) first for *why* and
   *what is in/out of scope*.
2. Read the [technical design](wails-v3-technical-design.md) for *how* the
   Go/Wails/React boundary is built.
3. Read the [implementation plan](wails-v3-implementation-plan.md) for
   *delivery order* and remaining Phase 7–8 work.

## Relationship to existing documents

- The [System Overview](../architecture/system-overview.md),
  [Domain Model](../architecture/domain-model.md), and
  [Data and Application Contracts](../architecture/data-and-ipc-contracts.md)
  describe the **currently implemented** Go + Wails application. They were
  updated at Phase 6 cutover.
- The [product vision](../product/product-vision.md) and
  [roadmap](../product/roadmap.md) are unaffected: this is a frontend/runtime
  migration, not a change in product scope.
