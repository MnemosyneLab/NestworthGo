# Changelog

All notable changes to Nestworth are recorded here.

## [0.3.6] — Unreleased

### Added

- App-owned continuous SQLite backup to Cloudflare R2, off by default, with
  explicit configuration, connection testing, remote-confirmed backup status,
  manual backup and recovery-point listing.
- Explicit cloud recovery through schema-15 validation, preview, confirmation,
  safety copies and the existing atomic installation/startup journal. Backup
  remains paused after restore; re-enabling creates a fresh stream.
- Shared secret fields for provider keys and R2 credentials: fixed masks,
  replacement, explicit removal and visibility for newly typed input only.

### Changed

- Advanced synchronized development metadata to v0.3.6 / build 7.
- Embedded Litestream v0.5.17 and matching modernc SQLite v1.49.1; the business
  schema remains 15 and no schema migration is added.
- R2 credentials and backup status live in a separate private local
  `backup_config.db`. Existing provider and AI/MCP settings remain in the
  business database and are included in backup.

### Boundaries

- Backup runs only while the app runs, on one machine. There is no multi-device
  synchronization, cloud writer election or daemon. Each start creates a fresh
  isolated stream; disabling never deletes remote history.
- Live R2 and native Mac acceptance remain pending. Published v0.3.5 tags and
  assets are unchanged. See the [release contract](docs/releases/v0.3.6.md) and
  [backup implementation notes](docs/development/continuous-backup.md).

## [0.3.5] — 2026-09-30

### Added

- Household Overview and Data Health improvements, including clearer household
  trends, breakdown navigation, missing-input actions, and repair progress.
- Available Funds estimates with source-specific access rules and reservations,
  plus managed term deposits and locked products. Product operations include
  previews, explicit receipts/redemptions, renewal, and guarded grouped undo.
- Local MCP read, directory-maintenance, and ledger-writing modes. The tools
  cover account and directory queries, ledger entries and atomic batches,
  position import/transfer, contribution analysis, reconciliation, historical
  corrections, data-health repair, and Agent-supplied market data.
- Agent-supplied instrument and FX observations as a local quote source. The
  App does not query an Agent endpoint.

### Changed

- Upgraded the Wails v3 desktop shell and @wailsio/runtime to v3.0.0-beta.26.
- Advanced synchronized application metadata to v0.3.5 / build 6.
- SQLite schema 15 is the only schema accepted for existing databases. This
  line does not migrate the schema-11 database from v0.3.4.
- Removed the Cloudflare Worker market-data provider. Quote and history refresh
  remains explicit and uses the configured local provider adapters.
- The macOS release task now prepares a metadata-preserving arm64 ZIP, DMG
  checksums, and the standalone Nestworth skill bundle plus its checksum.

### Compatibility and boundaries

- New databases use schema 15. Existing databases or backup files with older,
  unversioned, or future schemas are rejected without migration or recreation.
  Preserve the original database and backup; do not delete them to resolve an
  incompatibility. JSON export is not a restorable backup.
- The MCP listener is local loopback only. It does not provide a cloud relay or
  place brokerage orders. Direct bank/brokerage integration, synchronization,
  cloud backup (including Litestream/R2), and background refresh are not
  implemented in this release line.
- v0.3.5 was published at commit `0a4639deed01198233284d8baf184f23954d8b45`.
  Its [release contract](docs/releases/v0.3.5.md) retains the historical gate record.

## [0.3.4] — 2026-09-20

### Changed

- Separated FX conversion spread from holding-period FX changes in Asset Changes
  and Return Analysis, with consistent trend totals and cash/account breakdowns.
- Replaced CSV import/export with a versioned JSON export for external apps and
  scripts. Includes archived entities, full business and market history, and a
  current balance/holding/cost summary with explicit missing-value states.
  Removed CSV import and mapping; backup and restore remain the recovery path.
- Asset Changes opens with daily Asset Trend. Trend and Change Drivers show shared
  backend-calculated period values and percentage change, with explicit unavailable
  reasons and a cash-flow-versus-return explanation.
- Simplified insights controls, showed effective trend dates, unified Return Trend
  date shortcuts, and preserved the active tab when resetting filters.
- Upgraded the Wails v3 desktop shell and `@wailsio/runtime` to `v3.0.0-beta.23`.
- Added cross-account holdings grouped by instrument, expandable account details,
  native-currency cost/value/gain totals, and an optional zero-position filter.
  Missing FX no longer hides valid native amounts in this view.
- The holdings creation form accepts an explicit unit cost after history starts,
  allowing positions without a saved current price to be recorded.
- Combined Portfolio and Holdings into one Portfolio page with Overview and Holdings tabs.
- Advanced synchronized application metadata to `v0.3.4` / build `5`.
- Reorganized Settings into appearance/language, market connections, backup/data,
  diagnostics, About, and reset sections, with section navigation and a shared
  preference save bar; credentials retain independent save actions.

### Upgrade and verification

- SQLite schema remains `11`. Schema `9` and `10` databases still upgrade offline
  to `11`. Preserve the original database and backup before upgrading, then create
  a fresh backup after a successful open.
- Current checks, artifact details, and outstanding manual/distribution gates are
  recorded in the [v0.3.4 release contract](docs/releases/v0.3.4.md).

## [0.3.3] — 2026-09-19

### Added

- Yahoo instrument search.
- CoinGecko crypto search, latest prices, daily history, and local Demo-key settings.
- Gold and silver templates with grams/troy-ounce units, backend currency conversion,
  conversion provenance, and CSV unit metadata. Yahoo futures prices are reference estimates.
- Quote-history observation details, configurable local diagnostics, and named
  repair previews with current queries and completed results.

### Changed

- Synchronized application metadata at `v0.3.3` / build `4` and Wails `v3.0.0-beta.21`.
- Moved manual FX entry into a header action and side panel.
- Replaced the Yahoo adapter with go-yfinance; existing provider bindings remain explicit.
- Unified instrument history around first effective ownership, with a seven-day
  lead-in and creation-date fallback for never-held instruments.
- Reused fresh successful quote checks during normal repair, including after restart;
  explicit force refresh remains available.
- Improved Asset Changes contribution rows and historical date-range defaults.
- Advanced SQLite to schema `11`, preserving existing custom-metal semantics.

### Fixed

- Backdated valuation, historical coverage, and analytics repair regressions.
- Stale job failures appearing as current health findings and redundant repair work
  for inferred internal equity-market closures.
- Release-test fixtures and assertions that still expected pre-metal schema or
  settings without the resolved diagnostic log path.

### Upgrade and verification

- Schema `9` and `10` databases upgrade offline to `11`. Older-schema backup files
  cannot be restored directly through the schema-11 restore flow; preserve the
  original database and backup before upgrading, then create a fresh backup.
- Current checks, artifact details, and outstanding manual/distribution gates are
  recorded in the [v0.3.3 release contract](docs/releases/v0.3.3.md).

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
