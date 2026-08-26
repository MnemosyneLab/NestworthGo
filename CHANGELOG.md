# Changelog

All notable changes to Nestworth are recorded here. The project has not
published a public distribution yet.

## [0.2.0] — Unreleased

### Changed

- Standardized the desktop application on Wails v3 with `cmd/nestworth` as the
  only application entry point.
- Synchronized Go, frontend, and native packaging metadata to `0.2.0`.
- Reorganized the repository documentation around current product, design,
  architecture, development, and release contracts.
- Removed obsolete migration notes, prototype bundles, and unmaintained
  implementation records from the source tree.

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
