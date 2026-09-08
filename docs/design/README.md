# Design and UX

The current product surface is the React application under `frontend/src`.
The navigation model is defined by
[`frontend/src/app/navigation.ts`](../../frontend/src/app/navigation.ts), and
the startup/onboarding gates are defined by
[`frontend/src/App.tsx`](../../frontend/src/App.tsx).

## Maintained contracts

- [Account Container Interaction](account-container-interaction.md) — Account creation, detail views, actions, state handling, and accessibility behavior.
- [History and related-form defaults](history-and-form-defaults-ux.md) — Current history, picker, calculation, quote, timezone, and form-state behavior.
- [Backup, restore, and CSV portability](backup-restore-and-csv-portability.md) — Implemented local backup, restore, Accounts CSV, and Holdings CSV workflows.
- [Visual analytics and market history](visual-analytics-and-market-history.md) — Implemented chart and local quote-history surfaces, with external historical-series work still planned.
- [Analytics redesign architecture](analytics/analytics-redesign-architecture.md) — Replace the Analysis dashboard with Return Analysis and Asset Changes. Product, wireframe, and [development plan](analytics/analytics-development-plan.md) companions live in the same folder. Phase 6 cutover is implemented; Wails desktop smoke is still pending.

## Current screen map

The desktop shell exposes these top-level destinations:

- Overview;
- Accounts;
- Investments;
- Market Data;
- Directory, containing Members, Institutions, and Groups;
- History;
- Return Analysis;
- Asset Changes;
- Settings.

An empty local database opens onboarding. A database that cannot be opened or
verified opens the blocked-startup state and keeps business calls unavailable.

## Interaction invariants

- Financial totals, valuation, gain, replay, and validation come from Go
  application services.
- The frontend owns layout, navigation, form state, accessibility behavior,
  localization, and chart rendering.
- Every feature needs loading, empty, error, and unavailable states where the
  underlying query can produce them.
- User-triggered provider refresh is explicit and must not be required for
  startup or ordinary local reads.
- Mutations reload authoritative data instead of inventing optimistic
  financial totals.
- New interactive controls need keyboard coverage and localized labels.

## Design change rules

Use the current code and tests as the baseline for design changes. A change to
layout, wording, or interaction details must state whether it is implemented,
in progress, planned, or deferred. Store durable decisions in the product,
architecture, or release document that owns them; standalone prototypes are
not product contracts.
