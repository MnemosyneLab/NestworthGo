# Wails v3 Migration Plan

## 1. Purpose and Status

**Status:** `Planned`. This document defines the contract for replacing the
Fyne desktop shell (`internal/ui`, `internal/app`) with a
[Wails v3](https://v3.wails.io/) application built from a React + TypeScript
frontend and the existing Go backend. It does not describe shipped behavior.

Wails v3 is the framework selected by
[`prototype/nestworth-wails-frontend-stack.md`](../../prototype/nestworth-wails-frontend-stack.md):
Go stays the backend and the sole financial authority; React + TypeScript,
Vite, Tailwind CSS, shadcn/ui, TanStack Query/Table, Zustand, React Hook
Form + Zod, Apache ECharts, and i18next become the presentation stack. The
[interaction design brief](../../prototype/功能现状与交互设计说明.md) is the
product/UX reference for what the current application already does and
which flows a redesign must preserve or deliberately improve.

This document owns **why** the migration happens and **what is in and out of
scope**. The [technical design](wails-v3-technical-design.md) owns **how**
the Go/Wails/React boundary is built. The
[implementation plan](wails-v3-implementation-plan.md) owns **delivery
order**.

## 2. Motivation

The current Fyne shell (`internal/ui`, ~7,300 lines across 21 files) is the
only reason the product cannot express the high-density tables, multi-step
forms, and chart-heavy views the
[frontend stack decision](../../prototype/nestworth-wails-frontend-stack.md)
identifies as required (§1: "UI 表达能力明显高于 Fyne"). The Go backend —
`internal/domain`, `internal/application`, and `internal/infrastructure` —
is not the problem: it already owns every financial calculation, is
already covered by the test suite described in the
[engineering guide](../development/engineering-guide.md), and already
returns results shaped as explicit, presentation-agnostic structs per the
[data and application contracts](../architecture/data-and-ipc-contracts.md).
The migration's job is to give that backend a materially more capable UI
without touching its financial correctness guarantees.

## 3. Non-Goals

This migration explicitly does **not**:

- Change any financial calculation, rounding rule, or domain invariant in
  `internal/domain` or `internal/application`. Every value the new frontend
  displays must trace to the same Go computation the current Fyne UI
  already calls.
- Change the SQLite schema, migration state machine, or on-disk database
  format owned by `internal/infrastructure/sqlite`. Schema `6` remains
  current; the compatibility rules in
  [data and application contracts](../architecture/data-and-ipc-contracts.md#migration-compatibility-state-machine)
  are unaffected.
- Reopen or renegotiate the scope of any v0.1.1–v0.1.4 release contract in
  [`docs/releases`](../releases/README.md). Feature parity with those
  contracts is a **requirement** of this migration (§7), not an opportunity
  to redesign product scope.
- Decide new product scope beyond what
  [`docs/product/roadmap.md`](../product/roadmap.md) already directs (for
  example, v0.1.5's review-before-post, freshness reminders, or
  backup/restore). Those remain separate product decisions with their own
  design documents.
- Add a second application framework as a long-term fallback. Fyne is
  retired once this migration completes; see §9 (Rollback).
- Adopt Next.js, SSR, Redux Toolkit, Ant Design, Material UI, AG Grid, or
  Axios. The [frontend stack decision](../../prototype/nestworth-wails-frontend-stack.md#15-不推荐的方案)
  already rejected these for this product.

## 4. Goals

1. Replace `internal/ui` (Fyne views/widgets) and the Fyne-specific parts of
   `internal/app` (window/lifecycle wiring) with a Wails v3 application and
   a React + TypeScript frontend, while reusing `internal/domain`,
   `internal/application`, and `internal/infrastructure` verbatim.
2. Expose the existing `internal/application.Service` capability surface to
   the frontend through a small number of Wails **services** — thin Go
   adapters that translate domain results into explicit, wire-safe DTOs and
   translate `*domain.Error` into a stable, structured error contract (see
   [technical design §5](wails-v3-technical-design.md#5-serialization-and-error-contract-across-the-wails-boundary)).
   No new business logic is added in the adapter layer.
3. Reach and verify feature parity with every currently implemented Fyne
   page and workflow (Onboarding, Overview, Accounts, Investments, Market
   Data, Members, Institutions, Groups, History/Timeline, Record
   change/Undo/Fix, Analytics, Settings) before Fyne is removed.
4. Preserve every local-first, financial-correctness, and privacy boundary
   already stated in the
   [system overview](../architecture/system-overview.md#privacy-and-security-boundaries):
   no account registration, no required network connection for core
   operation, provider calls remain explicit and never block startup, and
   sensitive data (balances, notes, quantities, credentials, raw SQL,
   database rows) never crosses into a user-facing error or log.
5. Ship on the same primary target (macOS Apple Silicon) with an equivalent
   or better packaging/signing story, while keeping the Windows/Linux door
   open the way Fyne did (system overview: "Fyne keeps the option of
   supporting other desktop platforms later").
6. Improve, not regress, the existing UI rules from the
   [engineering guide](../development/engineering-guide.md#fyne-ui-rules):
   the UI still must not open SQLite, construct SQL, or recalculate a
   financial result; long-running work still must not block the UI; every
   interactive feature still needs a keyboard path.

## 5. Scope Boundaries: What Is Reused vs. Replaced

| Layer | Disposition | Notes |
| --- | --- | --- |
| `internal/domain` | **Reused, unmodified logic** | No behavior change. See [technical design §4](wails-v3-technical-design.md#4-a-required-domain-side-fix-json-marshaling-for-value-types) for one required but behavior-preserving addition (JSON marshaling on value types already used only as an implementation detail today). |
| `internal/application` | **Reused verbatim** | Every method in the current `Service` surface (`Bootstrap`, `CreateAccount`, `RecordChange`, `HoldingGain`, `RefreshAll`, ...) keeps its existing signature and tests. Wails services call it exactly as Fyne's `Controller` does today. |
| `internal/infrastructure/sqlite` | **Reused, unmodified** | Same schema, same compatibility state machine, same repository interface. |
| `internal/infrastructure/marketdata` | **Reused, unmodified** | Same Yahoo/Frankfurter adapters and `MarketDataRegistryPort`. |
| `internal/infrastructure/media` | **Reused, unmodified** | Same image normalization; the picker UI moves to a Wails dialog (§6). |
| `internal/settings` | **Reused, unmodified persistence** | Same JSON file, same schema-versioned validate/salvage logic. Only the caller (Wails main instead of Fyne `App`) changes. |
| `internal/i18n` (catalog) | **Ported, not reused as Go code** | The catalog **content** (English/简体中文/正體中文 strings and error-code mapping) moves to i18next resources; the Go package's runtime role (formatting inside Fyne widgets) is retired. See [technical design §8](wails-v3-technical-design.md#8-internationalization). |
| `internal/ui` | **Replaced** | Fyne views, widgets, chart rendering, and the `Controller` are replaced by the React frontend. Retired at the end of the migration (§9), not deleted early. |
| `internal/app` | **Replaced** | Fyne app/window lifecycle wiring is replaced by a Wails `main.go` that wires the same `application.Service`, `settings.Store`, and `sqlite.DB` construction. |
| `internal/version` | **Reused, unmodified** | Same version/build metadata; exposed to the frontend through a small `AppService` (technical design §6). |
| `internal/ui/image_picker_darwin.go` / `image_picker_other.go` | **Replaced** | Native Cocoa `NSOpenPanel` code is replaced by Wails's cross-platform `app.Dialog.OpenFile()`, removing macOS-only cgo from this concern. |
| `scripts/package-macos.sh` | **Replaced** | Fyne's packager is replaced by the Wails v3 Taskfile-based build (`wails3 build` / `task darwin:package`); see [technical design §11](wails-v3-technical-design.md#11-build-and-packaging). |
| Application data (SQLite file, settings JSON) | **Unaffected** | Existing user data opens exactly as before; no migration of user data is part of this plan. |

## 6. Product/UX Continuity Requirement

The [interaction design brief](../../prototype/功能现状与交互设计说明.md)
explicitly invites a redesign of navigation and information architecture
(§7, §10.6: "设计师可以推翻当前导航结构"). This migration plan does **not**
require the new frontend to reproduce the current Fyne navigation
pixel-for-pixel. It **does** require that:

- Every currently implemented capability listed in the interaction brief
  §5 (v0.1.1–v0.1.4 "当前已实现的核心能力") remains reachable and correct
  after the migration; a page can move, merge, or be restructured, but a
  capability cannot silently disappear.
- Every state the brief requires in §9 (data states, operation states,
  visual/platform states) is designed for, not just the happy path.
- The brief's §4 principles (Household first, Local first, Financial
  correctness first, Progressive complexity, Explainable data) hold in the
  new UI exactly as they hold in the current one.
- Native image/file pickers, window chrome, and keyboard behavior remain
  desktop-appropriate; this is a desktop application inside a webview, not
  a web page.

Any information-architecture decision from the interaction brief's §13 open
questions must be resolved and recorded (for example, in a follow-up product
document) before or during Phase 5 of the
[implementation plan](wails-v3-implementation-plan.md#phase-5-frontend-feature-parity),
not left implicit in component code.

## 7. Locked Decisions

| Decision | Contract |
| --- | --- |
| Desktop framework | Wails v3 (`github.com/wailsapp/wails/v3`), not v2, not Tauri, not Electron |
| Backend language | Go, reusing `internal/domain` / `internal/application` / `internal/infrastructure` without behavior change |
| Frontend stack | React + TypeScript + Vite + Tailwind + shadcn/ui + TanStack Query/Table + Zustand + React Hook Form + Zod + Apache ECharts + i18next, per the [frontend stack decision](../../prototype/nestworth-wails-frontend-stack.md) |
| IPC boundary | Wails v3 generated bindings only; no REST/HTTP API, no GraphQL, no second IPC mechanism |
| Financial authority | Unchanged: Go computes every financial value; the frontend only formats and lays out results, exactly as [data and application contracts](../architecture/data-and-ipc-contracts.md#serialization-and-view-models) already requires of any UI |
| Serialization | Every value crossing the Wails boundary uses the same rules already defined for the Fyne boundary (typed IDs as lowercase hyphenated UUID strings, RFC 3339 millisecond timestamps, three-letter currency codes, canonical decimal strings, explicit optional fields, stable error codes) — see [technical design §5](wails-v3-technical-design.md#5-serialization-and-error-contract-across-the-wails-boundary) |
| Error contract | `*domain.Error{Code, Field, Message}` crosses the boundary as structured JSON, not a plain error string; the frontend translates by `Code`/`Field`, never by parsing English message text |
| Database/settings location | Unchanged: `os.UserConfigDir()/Nestworth/{nestworth.db,settings.json}`, same `NESTWORTH_DATABASE_PATH` / `NESTWORTH_SETTINGS_PATH` overrides for isolated testing |
| Primary platform | macOS Apple Silicon first, matching the current target; Windows/Linux remain an explicit later option, not required for this migration's completion |
| Fyne removal timing | Fyne code is removed only after the parity checklist in §8 passes on the new frontend, per the strangler-style rollout in the [implementation plan](wails-v3-implementation-plan.md) |
| App identity | Bundle ID `com.nestworth.app`, app name `Nestworth`, and `internal/version` metadata are preserved; this is a runtime swap, not a rebrand |

## 8. Acceptance Criteria (Release Parity Checklist)

The migration is **complete** only when all of the following hold, verified
against a fresh database and against the sanitized fixtures already used by
the Go test suite:

- Every capability in the release contracts
  [v0.1.1](../releases/v0.1.1.md), [v0.1.2](../releases/v0.1.2.md),
  [v0.1.3](../releases/v0.1.3.md), and [v0.1.4](../releases/v0.1.4.md) is
  reachable in the Wails/React application and produces the same Go-computed
  values the Fyne application produces for the same fixture.
- Onboarding, Overview, Accounts (create/update/archive/ownership), Members,
  Institutions, Groups, Instruments/Holdings, manual and provider-refreshed
  quotes/FX, History/Timeline, Starting Point, Record change, Undo, Fix,
  Investments (cost/gain), Analytics (realized gain by period, currency
  decomposition), and Settings all work end to end through the new frontend.
- Every "unavailable"/"incomplete"/"stale"/"manual" state the Go backend can
  produce is visibly and correctly represented; the frontend never invents,
  hides, or zeroes a value the backend marked unavailable.
- Provider refresh (Yahoo Instrument quotes, Frankfurter FX) remains
  explicit, cancellable, and never required for startup or any read.
- English, Simplified Chinese, and Traditional Chinese are all available in
  the new frontend with complete key coverage (no missing-key fallback to
  raw keys in a shipped build).
- Light, Dark, and System appearance all render correctly, matching the
  existing accent system's intent (even if implemented with new tokens).
- Window size persistence, application icon, and native menu/dock behavior
  work equivalently to the current Fyne shell.
- Keyboard-only completion is possible for onboarding, account creation,
  record-change, and settings, matching the bar the current
  [engineering guide](../development/engineering-guide.md#fyne-ui-rules)
  already sets ("Add keyboard and accessibility behavior with each
  interactive feature").
- `go test ./...`, `go test -race ./...`, and `go vet ./...` pass unchanged
  for `internal/domain`, `internal/application`, and
  `internal/infrastructure`; no test in those packages is deleted or
  weakened to make the migration pass.
- The new Wails service/adapter layer has its own test coverage for
  serialization (round-trip of every DTO) and error mapping (every
  `domain.ErrorCode` reaches the frontend with its code and field intact).
- A macOS Apple Silicon build produces a launchable, unsigned `.app`
  (signing/notarization remain separate distribution gates, exactly as they
  are today per the [v0.1.4 release contract](../releases/v0.1.4.md#release-acceptance)).
- All architecture, engineering, and release documents that described the
  Fyne shell are updated in the same change that retires it (README,
  [system overview](../architecture/system-overview.md),
  [engineering guide](../development/engineering-guide.md)); no active
  document keeps describing Fyne as current once it is removed.

## 9. Rollback

Because `internal/domain`, `internal/application`, and
`internal/infrastructure` are not modified by this migration (only added to,
per [technical design §4](wails-v3-technical-design.md#4-a-required-domain-side-fix-json-marshaling-for-value-types)),
the Fyne shell keeps building and running throughout the migration until the
implementation plan's cutover phase. This gives a cheap rollback path at any
point before cutover: stop the Wails work, keep shipping the Fyne
`cmd/nestworth`. After cutover (Fyne code removed), rollback means reverting
the cutover commit(s) on version control; there is no data-format rollback
concern because the database and settings file are untouched by this
migration.

## 10. Compatibility Promises

- No existing SQLite database or settings file requires migration or
  conversion to work with the Wails application.
- No `internal/domain` or `internal/application` behavior changes; a test
  that passes against the current Fyne-era code must still pass, unmodified,
  against the Wails-era code, because it exercises the same packages.
- A later release may still add new product capability (v0.1.5 and beyond)
  on top of the Wails/React frontend; this plan does not foreclose or
  predetermine that design.
- This plan may be revised if Wails v3 ships a breaking change before the
  migration completes; because v3 is beta software (see
  [technical design §2](wails-v3-technical-design.md#2-a-note-on-wails-v3-maturity)),
  the [implementation plan](wails-v3-implementation-plan.md#risk-register)
  tracks this explicitly as a risk, not an assumption.
