# Changelog

All notable changes to Nestworth are recorded here.

## [0.3.7] — 2026-10-09

Published as [v0.3.7](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.7)
(build 8) at `4d72b3dfaac34b86f24923760663ef0c11c3ec54`. The release includes
the Apple Silicon DMG/ZIP, `SHA256SUMS`, and the standalone skill 1.6.0 archive
with its SHA-256 file. See the [release contract](docs/releases/v0.3.7.md) for
acceptance scope and remaining GUI/platform boundaries.

### Added

- MCP financial context packages and frozen detail pages/items, with minimal or
  explicitly named disclosure choices; two-state comparisons report right minus
  left and are distinct from returns. A separate coherent comparison-to-
  attribution workflow links compatible captures.
- MCP reads for managed deposits, locked products, product operation history,
  and liquidity routes with actual/forecast evidence, unknowns, fees and
  reservations preserved.
- Guarded MCP preview/commit workflows for completed managed-product lifecycle
  facts, complete revisioned terms/policy edits, and actual locked-product
  valuation observations. All writes use dedicated ledger-write tools; renewal
  and reservation changes remain App workflows.

### Fixed and changed

- Legacy analysis and household/portfolio trends now materialize their
  requested historical snapshot coverage, including earlier sparse ranges,
  without treating a later completion watermark as proof that every earlier
  snapshot exists. Repair is bounded, resumable, and rejects concurrent state
  changes.
- Prevented millisecond clock drift in holding creation and added spacing and
  narrow-window scrolling to product table columns.
- Race CI now shards bounded application and MCP race runs while retaining
  `-race`, all tests, and Go's default package timeout; selector enumeration
  failures fail the shard. This is a CI execution change, not a removal of tests.

### Boundaries and validation

- Skill version is 1.6.0 (`skills/nestworth/VERSION`), included in this release;
  it remains independently versioned from the App. Existing clients must
  install/update it separately.
- PR #47 bounds native dependency installation and is included in main at
  `57be6c18d1d54a98c67fa463f99fea6289fc27e7`; PR-head test/race CI run
  37807183123 passed all jobs. See the
  [v0.3.7 release contract](docs/releases/v0.3.7.md) for detailed evidence.
- The user reports scoped real Codex/MCP/skill acceptance passed on Mac mini
  against source `2cad89e`; see the release contract's QA record and limits.
  Separate v0.3.7 package and synthetic onboarding/quit/restart/persistence
  checks passed. Full Mac GUI regression was deferred by the user; macOS 12
  runtime behavior was not tested. The ad-hoc signed build is not Developer ID
  signed or notarized.

## [0.3.6] — 2026-10-03

Published as [v0.3.6](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.6)
at `64ba10bcf0e1c7bdac56e54b399c0ba9fb901acc`. The release includes the
Apple Silicon DMG/ZIP, `SHA256SUMS`, and the standalone skill archive with its
SHA-256 file. v0.3.7 is now the latest published release; v0.3.6 remains a
published historical release.

### Added

- App-owned continuous SQLite backup to Cloudflare R2, off by default, with
  explicit configuration, connection testing, remote-confirmed backup status,
  manual backup and recovery-point listing.
- Explicit cloud recovery through schema-15 validation, preview, confirmation,
  safety copies and the existing atomic installation/startup journal. Backup
  remains paused after restore; re-enabling creates a fresh stream.
- Opt-in 30/90-day whole-stream retention with read-only previews, verified
  survivors, ownership protection and conditional deletion; cleanup is off by default.
- Bundled bank logos, inherited account icons, and generic/crypto icon coverage.
- Shared secret fields for provider keys and R2 credentials: fixed masks,
  replacement, explicit removal and visibility for newly typed input only.

### Changed

- Advanced synchronized development metadata to v0.3.6 / build 7.
- Embedded Litestream v0.5.17 and matching modernc SQLite v1.49.1; the business
  schema remains 15 and no schema migration is added.
- R2 credentials and backup status live in a separate private local
  `backup_config.db`. Existing provider and AI/MCP settings remain in the
  business database and are included in backup.

### Fixed

- Financial form validation and gain consistency, history-start default times,
  recovery pagination, and current-month empty-state explanations.
- Balance-account reservation recovery, account editing during pending saves,
  edit-dialog dismissal, and annual-interest paid-through date visibility/focus.
- Retention now recognizes the pinned Litestream S3 LTX layout.

### Performance

- Reduced cold in-memory analysis computation and allocations, indexed latest
  account-value reads, and reused market-history inputs within each repair plan.
  Measurements are scoped benchmarks, not whole-app or Mac startup speed claims;
  see the [release contract](docs/releases/v0.3.6.md).

### Boundaries

- Backup runs only while the app runs, on one machine. There is no multi-device
  synchronization, cloud writer election or daemon. Each start creates a fresh
  isolated stream; disabling never deletes remote history.
- Live R2 backup/recovery/retention and latest native Mac GUI acceptance were
  user-reported as passed during preparation. The v0.3.6 Mac artifacts and
  skill bundle were subsequently published with the release. Published v0.3.5
  tags and assets are unchanged. See the [release contract](docs/releases/v0.3.6.md) and
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
