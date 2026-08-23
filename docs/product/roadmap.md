# Product Roadmap

## Status and Authority

This roadmap is reviewed for the current Go + Fyne repository. It describes
product dependency order and outcomes, not implementation evidence. Current
code, migrations, tests, and active release contracts remain authoritative.

Detailed v0.1.3–v0.1.5 documents inherited from the Rust/Tauri repository are
kept in the
[unreviewed archive](../legacy/rust-tauri-inherited-unreviewed/README.md). Their
old phase status and technical design are not current plans.

## Strategy

The v0.1 line advances in dependency order:

1. Establish a trustworthy Household balance sheet.
2. Value positions across currencies and Instruments.
3. Remember changes through a simple family timeline and history.
4. Calculate performance and attribution from trustworthy history.
5. Reduce maintenance cost through preparation, recovery, exchange, and search.

Every release preserves local-first operation, exact decimal authority, manual
fallback, append-only financial evidence, and safe database compatibility. A
later release may extend an earlier entity but must not reinterpret existing
Money, Ownership, values, quotes, or archive state.

## Release Sequence

### v0.1.1 — Household Balance Sheet

**Theme:** Build the Household balance sheet.

**Status:** Core implementation complete; public-release closeout pending.

The current Go implementation provides onboarding, Members, Institutions,
Groups, Accounts, exact Ownership, append-only current values, archive/restore,
filters, local persistence, media, settings, and backend-owned Overview totals.

**Exit outcome:** A Household can answer what it owns, owes, and has as current
net worth in one base currency.

See the active [v0.1.1 release contract](../releases/v0.1.1.md).

### v0.1.2 — Multi-Currency and Portfolio

**Theme:** Know what everything is worth.

**Status:** Phases 0–9 implemented; release closeout in progress.

This release adds multi-currency Accounts, Instruments, Holdings, investment
cash, manual quotes, centralized current valuation, and explicit current Instrument/FX
refresh through replaceable Yahoo Finance and Frankfurter adapters. Settings
routes only explicit FX refresh; Instrument bindings remain provider-specific.
Provider failure never blocks startup or complete manual valuation. Historical
market-data backfill is not part of this release.

**Exit outcome:** A Household can value current cash and investment positions in
its base currency while retaining native amounts, provenance, freshness, and
explicit incomplete diagnostics.

See the active [release contract](../releases/v0.1.2.md),
[technical design](../releases/v0.1.2-technical-design.md), and
[implementation plan](../releases/v0.1.2-implementation-plan.md).

### v0.1.3 — Family Timeline and History

**Theme:** Remember what changed.

**Status:** Designed; implementation follows v0.1.2 release closeout.

Add a family-facing Starting point, Record change, Undo/Fix, Timeline, and
net-worth history. The UI asks what happened in ordinary language. Go keeps the
immutable Activity effects, current projections, replay, historical valuation,
and daily cache revisions internally without fabricating trades or cash flows
from existing v0.1.2 state.

Implementation begins only after Phase 10 freezes the actual completed v0.1.2
schema, interfaces, fixtures, query bounds, provider routing, and UI ownership.

See the active [release contract](../releases/v0.1.3.md),
[compatibility baseline](../releases/v0.1.3-baseline.md),
[technical design](../releases/v0.1.3-technical-design.md), and
[implementation plan](../releases/v0.1.3-implementation-plan.md).

### v0.1.4 — Analytics and Performance

**Theme:** Know why wealth changed.

**Status:** Direction only; Go/Fyne design not started.

Intended outcome: derive cost basis, gain, income/fees, currency effects,
time-weighted return, money-weighted return, and attribution from trustworthy
Activity/history evidence. Unavailable inputs remain unavailable rather than
becoming estimates.

Detailed design is intentionally deferred until the Go v0.1.3 change and
history contracts are implemented and verified.

### v0.1.5 — Sustainable Long-Term Use

**Theme:** Keep Nestworth current and recoverable.

**Status:** Direction only; Go/Fyne design not started.

Intended outcome: reduce maintenance cost with review-before-post preparation,
freshness reminders, recoverable backup/restore, controlled import/export,
comparison data, and keyboard-focused search/navigation.

Detailed design is intentionally deferred until the Go persistence, Activity,
history, and analytics boundaries it depends on exist.

## Dependency Rules

- Family change history depends on the implemented v0.1.2 current-state and
  quote contracts.
- History depends on immutable Activities plus a migration-safe origin boundary.
- Performance depends on trustworthy Activity/history evidence and cannot be
  estimated from only initial and current values.
- Automation prepares or proposes facts; it does not silently post them.
- Backup/restore preserves exact database facts and never becomes cloud sync by
  implication.
- Import validates and previews before one atomic commit.
- Optional providers remain infrastructure; normal reads consume persisted
  normalized observations only.

## Deferred Beyond v0.1

- Cloud and multi-device sync
- Direct bank, broker, wallet, or exchange integrations
- Background agents and closed-application refresh
- Tax filing and jurisdiction-specific reports
- Budgeting and expense categorization
- Multi-user accounts, permissions, and remote collaboration
