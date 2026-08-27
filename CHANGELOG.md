# Changelog

All notable changes to Nestworth are recorded here. The project has not
published a public distribution yet.

## [0.2.1] — Unreleased

### Changed

- Replaced Account primary/secondary categories with `account_type`,
  `balance_sheet_role`, and `tracking_mode` on a breaking SQLite schema `7`.
- Overview now reports component-level `assetsByType` and `liabilitiesByType`
  instead of account-level category totals.
- Portfolio inclusion is a whole-account `include_in_portfolio` switch.
- Existing schema `6` databases are rejected without migration or rewrite;
  a new database must be created.

### Current capability

- Local Household onboarding, Accounts, Directory, Investments, Market Data,
  History, Analytics, and Settings surfaces are available in the desktop UI.
- Go remains authoritative for financial validation, persistence, valuation,
  replay, cost basis, gains, and provider routing.
- Frontend tests cover page behavior, localization, error handling, and
  keyboard-only navigation paths.

### Deferred

- Backup/Restore, Import/Export, synchronization, direct financial
  integrations, background refresh, signing, notarization, and public
  artifact publication remain outside this release closeout.

## [0.2.0] — Superseded unreleased line

`0.2.0` was the prior unreleased Wails v3 closeout line. It was never
published and is superseded by `0.2.1`.
