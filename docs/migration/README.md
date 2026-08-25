# Wails v3 Migration Documents

This directory contains the implementation plan for replacing the Fyne
desktop shell with a [Wails v3](https://v3.wails.io/) application: a Go
backend (reusing the current `internal/domain`, `internal/application`, and
`internal/infrastructure` packages) plus a React + TypeScript frontend, as
selected in [`prototype/nestworth-wails-frontend-stack.md`](../../prototype/nestworth-wails-frontend-stack.md)
and informed by [`prototype/功能现状与交互设计说明.md`](../../prototype/功能现状与交互设计说明.md).

**Status:** `Planned`. No Wails code exists in the repository yet. Nothing in
this directory describes shipped behavior; see
[status vocabulary](../README.md#status-vocabulary) for what `Planned` means
in this repository's documentation convention.

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
   new Go/Wails/React boundary is built, including the concrete
   serialization problem this migration must solve (see
   [Serialization and error contract](wails-v3-technical-design.md#5-serialization-and-error-contract-across-the-wails-boundary)).
3. Read the [implementation plan](wails-v3-implementation-plan.md) for
   *delivery order*, phase-by-phase deliverables, and the risk register.

## Relationship to existing documents

- The current [System Overview](../architecture/system-overview.md),
  [Domain Model](../architecture/domain-model.md), and
  [Data and Application Contracts](../architecture/data-and-ipc-contracts.md)
  describe the **currently implemented** Go + Fyne application. They remain
  authoritative until the phases in the
  [implementation plan](wails-v3-implementation-plan.md) actually change the
  shipped code; this migration plan does not retroactively change their
  `Implemented` status.
- Once the migration begins shipping, each phase must update the affected
  architecture/engineering documents in the same change, per the
  [documentation maintenance rules](../README.md#maintenance-rules), and this
  README must be updated to point at the new authoritative documents instead
  of describing a plan.
- The [product vision](../product/product-vision.md) and
  [roadmap](../product/roadmap.md) are unaffected: this is a frontend/runtime
  migration, not a change in product scope. No v0.1.x release contract is
  reopened by this plan; see
  [Non-goals](wails-v3-migration-plan.md#3-non-goals).
