# Changelog

All notable changes to Nestworth are recorded here.

## [0.3.2] — 2026-09-14

### Added

- Added provider-neutral historical market data with coverage/day-status
  tracking, correction-safe observations, resumable repair, and historical
  snapshot rebuilds.
- Added the unified Market Data workflow and Data Health center, including
  local gap detection, repair planning, progress, cancellation, and explicit
  incomplete-data states.
- Added historical provider support for Tiingo, Yahoo instrument data, and
  Frankfurter FX data, with deterministic fixtures and fail-closed persistence
  for malformed or unsupported responses.

### Changed

- Advanced synchronized application metadata to `v0.3.2` / build `3`.
- Extended valuation, analytics, settings, Wails services, and frontend query
  invalidation to consume historical market-data coverage and repair state.

### Verification

- The v0.3.2 automated and local packaging evidence is recorded in the
  [release contract](docs/releases/v0.3.2.md).
- Native Wails desktop smoke, real-provider HTTP checks, Apple M3 Pro
  performance, keyboard/accessibility review, Developer ID signing, and
  notarization remain explicitly reported gates for this first publication.

## [0.3.1] — Unreleased

### Added

- Replaced the Insights Analysis dashboard with Return Analysis and Asset
  Changes: scoped universes, effect classification, signed Asset Changes
  identity, path-aware Price/FX, Modified Dietz linking, memoized Wails
  projections, six tabs, shared filters, and History deep-links.
- Added interest to the writable money-in and value-update reason catalogs.

### Verification

- Analysis kernel and Insights automated suites pass, together with the full
  Go and frontend test runs.
- Synchronized application metadata as `v0.3.1` / build `2`.
- Wails desktop smoke, real-household review, keyboard/accessibility,
  signing, and notarization remain named unexecuted gates.

## [0.3.0] — Superseded unreleased line

### Added

- Local `.nestworth-backup` backup/restore with checksum, schema, and
  SQLite verification, a journaled file-group swap, and quit-and-relaunch.
- Restore from blocked startup when the current database cannot be opened.
- Accounts and Holdings CSV export/import with mapping, preview, and an
  all-or-nothing create-only commit.

### Current capability

- Local Household onboarding, Accounts, Directory, Investments, Market Data,
  History, Return Analysis, Asset Changes, Settings, backup/restore, and CSV
  portability.
- Go remains authoritative for financial validation, persistence, valuation,
  replay, cost basis, gains, and provider routing.
- Frontend tests cover page behavior, localization, error handling, and
  keyboard-only navigation paths.

### Deferred

- Encrypted or cloud backup, synchronization, direct financial integrations,
  background refresh, signing, notarization, and public artifact publication
  remain outside this release closeout.

## [0.2.1] — Superseded unreleased line

`0.2.1` replaced Account categories with `account_type`,
`balance_sheet_role`, and `tracking_mode` on schema `9`. It was never
published and is superseded by `0.3.0`.

## [0.2.0] — Superseded unreleased line

`0.2.0` was the prior unreleased Wails v3 closeout line. It was never
published and is superseded by `0.2.1`.
