# Changelog

All notable changes to Nestworth are recorded here. The project has not
published a public distribution yet.

## [0.3.2] — Unreleased

### Changed

- Advanced synchronized application metadata to `v0.3.2` / build `3`.

### Verification

- The previous `0.3.1` capability and validation scope carry forward.
- Wails desktop smoke, real-household review, keyboard/accessibility,
  signing, and notarization remain named unexecuted gates.

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
