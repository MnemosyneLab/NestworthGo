# Wails v3 Frontend Navigation Decisions

**Status:** `Implemented on 2026-08-26` (decisions shipped with the Wails
frontend). This is the short design note the
[implementation plan Phase 3](wails-v3-implementation-plan.md#phase-3--frontend-foundation)
required before frontend feature work began: it resolved the navigation-affecting
open questions from the
[interaction design brief §13](../../prototype/功能现状与交互设计说明.md#13-交给设计师前需要确认的问题)
so they were not left implicit in component code. This note does not
reopen product scope (per the
[migration plan's non-goals](wails-v3-migration-plan.md#3-non-goals)); it
only fixes the information architecture for the pages the migration
already committed to shipping.

## Decisions

| # | Question (brief §13) | Decision | Rationale |
| --- | --- | --- | --- |
| 1 | macOS-only first, or Windows/Linux too? | macOS Apple Silicon first; Windows/Linux remain optional/deferred. | Already locked by the [migration plan §7](wails-v3-migration-plan.md#7-locked-decisions); restated here for completeness, not re-decided. |
| 2 | Default landing page: Overview or Accounts? | **Overview.** | Matches the then-current Fyne shell's default (`internal/ui/shell.go`'s `Controller` started at `PageOverview`); this migration is a runtime swap, not a product change, and nothing in the interaction brief argues for changing the entry point. |
| 3 | Is Activity a separate entry from History? | **No — Activity is a view within History**, not a separate top-level nav item. | The then-current Fyne shell already treated them as one concept (`PageActivity` was a source-compatible alias for `PageHistory`); the new frontend keeps the Timeline/Activity feed, Starting Point, Record change, Undo, and Fix flows together under one "History" nav entry. |
| 4 | Do Members/Institutions/Groups stay top-level, or become Accounts filters? | **Stay reachable as dedicated management screens, grouped under one "Directory" section in the sidebar** (three tabs/routes under a shared parent, not three separate top-level nav rows, and not collapsed into Accounts' filter UI). | `DirectoryService` (Phase 1) already treats Members/Institutions/Groups as one API surface with the same CRUD/archive/icon shape; grouping them in the nav mirrors that without removing any of the three as independently manageable entities the release contracts require. |

## Resulting top-level navigation

```text
Overview        (default landing page)
Accounts
Investments
Market Data
Directory        — Members | Institutions | Groups (grouped, see decision 4)
History          — Timeline/Activity, Starting Point, Record change, Undo, Fix (see decision 3)
Analytics
Settings
```

This is the same set of reachable capabilities the then-current Fyne shell
exposed (`PageOverview`, `PageAccounts`, `PageInvestments`,
`PageMarketData`, `PageMembers`/`PageInstitutions`/`PageGroups`,
`PageHistory`, `PageAnalytics`, `PageSettings`); only the Directory
grouping changes the navigation shape, and no capability listed in the
[migration plan's acceptance criteria](wails-v3-migration-plan.md#8-acceptance-criteria-release-parity-checklist)
is removed or hidden by it.

## Open questions not resolved here

Questions 5–12 in the interaction brief's §13 (household composition
assumptions, minimal Account creation fields, Record change interaction
style, "Archive" wording, status-label Chinese phrasing, Overview
drill-down, dashboard customization, and keyboard/VoiceOver priority) are
**page-level or copy-level** decisions, not navigation-structure
decisions. They are deferred to the Phase 4/5 page implementations that
actually need them, each recorded as an inline decision in that page's
implementation (for example, in a component doc comment or a PR
description), rather than pre-decided here in the abstract.
