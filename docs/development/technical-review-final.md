# Nestworth-go Technical Review: Final Remediation Plan

Review date: 2026-09-02  
Baseline: current checkout, schema 9, Go 1.26, Wails v3 `v3.0.0-beta.12`, `shopspring/decimal` 1.4.0, and `modernc.org/sqlite` 1.44.3.

This document is the implementation plan produced after checking
`writing-block.md`, `technical-review.md`, and `technical-review-gpt.md`
against the current repository. It replaces neither source review; it
deduplicates them, corrects overstatements, chooses a solution where a review
left a policy decision open, and gives an ordered path to close every issue.

## Executive Summary

The repository has a sound base: finance values cross boundaries as decimal
strings, the activity ledger is append-only, current valuation retains exact
intermediate values, SQLite writes are transactional, Wails is outside the
domain layer, and backup/restore already has package validation and journaled
replacement.

The release-blocking gap is consistency between those good foundations and a
small set of secondary paths:

1. Historical net-worth snapshots do not apply `IncludeInNetWorth`, force a
   signed result through non-negative `Money`, and aggregate rounded values.
2. Position transfer ignores destination-quantity overflow and can commit a
   zero resulting quantity.
3. Gain calculations use different archive and FX-selection rules from live
   valuation. Average cost also discards supported unit-price precision.
4. Backup inspection rejects one v9 schema shape that normal startup accepts,
   and the advertised exclusive-operation gate does not cover every writer.
5. Several SQLite readers do not close `Rows` on scan errors, while startup
   hides actionable database states behind generic `unavailable`.
6. Analytics and list screens contain avoidable repeated reads and one
   incomplete History pagination flow.

No critical remote-security vulnerability was established. The highest risk
is wrong, missing, or unavailable financial output and recoverability behavior,
not code style.

## Verification Performed

The following checks passed on the review baseline:

- `env GOCACHE=/tmp/nestworth-final-review-test go test ./...`
- `env GOCACHE=/tmp/nestworth-final-review-race go test -race ./...`
- `env GOCACHE=/tmp/nestworth-final-review-vet go vet ./...`
- `env GOCACHE=/tmp/nestworth-final-review-build go build ./cmd/nestworth`
- `gofmt -l cmd internal` returned no files.
- `pnpm run build` in `frontend/` passed; Vite warned that the main minified
  JavaScript chunk is about 1.68 MB.
- `pnpm run lint` in `frontend/` passed.
- `pnpm run typecheck` in `frontend/` passed.
- `pnpm run test` in `frontend/` passed: 34 files, 278 tests.

The repository's canonical `task check` could not be invoked on the review
machine because the `task` executable was unavailable. Its constituent checks
were run directly as listed above. The Go linker emitted deployment-target
warnings but all commands exited successfully.

Not run: the native Wails application, browser walkthroughs, a packaged `.app`
or DMG, signing/notarization, native dialogs, real restart after Restore, and a
native accessibility pass. These remain manual release gates.

## Decisions This Plan Freezes

Implementers should not reopen these decisions inside individual fixes. If a
product owner wants a different accounting policy, record that change in an
ADR and update the tests before implementation.

| Area | Final rule |
| --- | --- |
| Net worth | Assets and liabilities remain non-negative `Money`; net worth and its trend start/end/change use `SignedMoney`. |
| Precision | Preserve checked exact decimals through quantity, price, FX, and aggregation. Round with midpoint-nearest-even only at a Money/wire display boundary. |
| Snapshot storage | Store an exact canonical base-amount string for each item. Existing positive schema-9 values remain readable. Rebuild old rounded revisions append-only; do not delete or rewrite historical Activities. |
| Average cost | Preserve the full supported `UnitPrice` precision of eight fractional digits. Do not round average cost to two decimals after each blend. |
| Fees | Report gains net of both acquisition and disposal fees. Add a buy fee to acquisition basis; subtract a sell fee from proceeds. Trade price itself remains price-only. |
| Archives | Archiving identity metadata never erases an active position or historical gain. Current totals exclude archived Holdings; period analytics still read their immutable realized/dividend history. |
| FX | One selector serves live, snapshot, realized-gain, and dividend paths. It prefers the applicable stored preference and otherwise uses the configured/default provider key; it selects the latest stored direct or inverse quote at or before the cutoff and never performs network I/O on a read. Missing evidence remains explicit. |
| Portfolio flag | `include_in_portfolio` remains a persisted compatibility field but is intentionally ignored by the current holdings-only Portfolio metric. Do not wire it back into valuation. |
| Backup | `VACUUM INTO` remains the consistent database snapshot mechanism. The application-wide write gate guarantees package-level quiescence and clean busy behavior; provider network requests are never performed while that gate is held. |
| User data | Never reset, delete, or silently replace a user's database to make a fix pass. Schema/derived-data changes must be backward-readable and rebuild derived snapshots append-only. |

## Corrected Conclusions From the Source Reviews

These corrections prevent implementation effort from targeting problems that
the current code does not actually have.

- The v9 account CHECK repair already runs its DDL in a transaction. Because
  the DSN uses `_txlock=immediate`, it is already an immediate transaction.
  Normal `Open` also calls `verifySchema` immediately afterward, and that runs
  `PRAGMA foreign_key_check`. The remaining work is compatibility, connection-
  scoped cleanup, and failure-fixture coverage—not evidence of an existing
  non-atomic `DROP`/rename corruption bug.
- `VACUUM INTO` produces a consistent SQLite snapshot even if an unrelated
  writer is attempted concurrently. The backup problem is that the documented
  exclusive gate is not honored by all mutation paths, which can cause waits or
  busy failures and can make the database and separately read settings describe
  different instants. Do not replace `VACUUM INTO` with raw file copying.
- `include_in_portfolio` being ignored is an intentional current contract, and
  the current create/edit forms no longer expose its checkbox. Preserve the
  compatibility field until a deliberate schema cleanup.
- Buy-fee treatment was unspecified, not a proven implementation bug. This
  plan resolves the ambiguity by choosing net-of-fees gain and requiring tests.
- Wails beta status, the broad application facade, hidden mounted pages, and
  advanced brokerage features are risks or debt, not current financial bugs.

## Prioritized Issue Register

| ID | Priority | Type | Verified problem | Primary locations | Status |
| --- | --- | --- | --- | --- | --- |
| F1 | P0 | Actual bug | Historical snapshots ignore `IncludeInNetWorth`. | `internal/application/historical_snapshot.go`, `service.go` | Resolved |
| F2 | P0 | Actual bug | Negative net worth cannot be saved or appended as today's trend point. | `internal/domain/history.go`, `historical_snapshot.go`, `trend.go`, Wails DTOs | Resolved |
| F3 | P0 | Actual bug | Historical account and portfolio totals sum rounded Money values. | `historical_snapshot.go`, `trend.go`, `snapshot_repository.go` | Resolved |
| F4 | P0 | Actual data-integrity bug | `buildPositionTransfer` discards `NewQuantity` errors; commit consumes the invalid endpoint. | `internal/domain/change.go`, `change_repository.go` | Resolved |
| F5 | P0 | Actual consistency bug | Gain paths drop active holdings whose Instrument is archived and drop realized history for archived Holdings. | `internal/application/gain_service.go`, `internal/infrastructure/sqlite/cost_basis_repository.go` | Resolved |
| F6 | P0 | Actual consistency bug | Historical gain/dividend FX requires an explicit preference while live valuation can use the default provider. | `gain_service.go`, `valuation.go` | Resolved |
| F7 | P0 | Actual compatibility bug | Read-only backup verification rejects the accepted pre-repair v9 account CHECK shape. | `sqlite/readonly.go`, `schema_repair.go`, `schema_verify.go` | Resolved |
| F8 | P0 | Actual resource bug | Some query loops return on scan error before closing `Rows` on a one-connection pool. | `activity_repository.go`, `observation_repository.go`, `snapshot_repository.go` | Resolved |
| F9 | P0 | Actual recovery-contract bug | Known bootstrap states are collapsed to generic `unavailable`. | `cmd/nestworth/main.go`, `sqlite/database.go`, Wails App startup DTO/UI | Resolved |
| F10 | P0 | Actual concurrency/design bug | Backup/import/restore exclusivity does not cover directory, onboarding, settings, and refresh persistence writers. | `application/exclusive.go`, `service.go`, `refresh.go`, Wails data/recovery adapters | Resolved |
| F11 | P1 | Financial correctness | Average cost rounds to two decimals despite an eight-decimal `UnitPrice` contract. | `internal/domain/cost_basis.go` | Resolved |
| F12 | P1 | Accounting policy gap | Buy fees reduce cash but are not included in acquisition basis. | `internal/domain/change.go`, `cost_basis.go` | Resolved |
| F13 | P1 | Performance/API | Investments performs one full-snapshot `AccountGain` call per Account; Overview also fans out. | `application/gain_service.go`, `frontend/src/queries/analytics.ts`, `OverviewPage.tsx` | Resolved |
| F14 | P1 | Actual UX bug | History returns a cursor but the UI never requests the next page. | `HistoryPage.tsx`, `frontend/src/queries/history.ts` | Open |
| F15 | P1 | Integrity/debt | Optional directory IDs can be silently converted to nil; schema verification casts exact quantities through `REAL`; aggregate ownership and snapshot provenance are not fully checked at persistence boundaries. | `sqlite/repository.go`, `schema_verify.go`, snapshot/ownership writers | Resolved |
| F16 | P1 | Privacy/hardening | Live database mode is not normalized, startup logs full paths, and pending Restore packages have no TTL/count bound. | `sqlite/database.go`, `cmd/nestworth/main.go`, `wailsapi/recovery/recovery.go` | Resolved |
| F17 | P1 | Performance | Activity/snapshot list hydration is per parent row; hidden pages can keep queries active. | SQLite list repositories, `frontend/src/App.tsx` | Open |
| F18 | P2 | Architecture debt | `application.Service` is a broad facade; application imports CSV infrastructure; Wails data/recovery adapters own persistence lifecycle. | `internal/application`, `internal/wailsapi/data`, `recovery` | Open |
| F19 | P2 | Contract/documentation debt | Activity taxonomy and cost-basis entities in architecture docs do not match stored code; `include_in_liquid_assets` has no metric consumer. | `docs/architecture`, account UI/contracts | Open |
| F20 | P2 | Reliability/release debt | Activity mutations have no request idempotency key; there is no CI workflow; native/package gates are manual; Wails is beta. | History commands, `.github`, build files | Open |
| F21 | P2 | Frontend precision/performance | Ownership conversion uses JavaScript `Number`; the production bundle has one large main chunk. | `accountCatalog.ts`, frontend route/import structure | Open |

## Implementation Sequence

Use the work packages below in order. Each package should be independently
reviewable and should leave the full automated suite green. Do not combine the
P0 financial changes with the later architecture refactor.

### Work Package 0 — Add Red Tests and Golden Fixtures

**Covers:** F1–F12 before behavior changes.

1. Add minimal fixtures that reproduce each P0 defect without changing
   production code.
2. Add a pre-repair v9 SQLite fixture containing realistic linked rows, not only
   a string-replacement unit test. Never derive it from a user's database.
3. Add a generated multi-account, multi-year fixture for later query-count and
   snapshot benchmarks.
4. Record the current expected failures in the PR description; do not weaken
   existing tests to make the new cases pass.

Required tests:

- Excluded asset and liability Accounts do not enter a daily snapshot.
- A 50-asset / 80-liability Household saves, reloads, and trends `-30`.
- Several sub-cent exact components agree between live Overview, persisted
  snapshot, reload, NetWorthTrend, and PortfolioTrend.
- A destination Holding at maximum quantity plus a transfer is rejected and no
  Activity, effect, or quantity observation is written.
- Active Holding + archived Instrument remains in current gain output.
- Sold then archived Holding remains in period realized gain.
- FX quote present, no explicit preference: Overview and gain/dividend agree.
- Both accepted v9 CHECK forms pass backup inspect and normal open.
- A forced scan error is followed by a successful query on the same one-
  connection database.
- Backup concurrent with each writer either obtains one coherent package or
  returns `backup_restore_busy`; it never times out as generic `internal`.

### Work Package 1 — Unify Historical and Live Valuation

**Covers:** F1–F3.

**Progress:** F1, F2, and F3 resolved.

Primary files:

- `internal/domain/history.go`
- `internal/application/historical_snapshot.go`
- `internal/application/trend.go`
- `internal/infrastructure/sqlite/snapshot_repository.go`
- `internal/wailsapi/history/history.go`
- `internal/wailsapi/portfolio/portfolio.go`
- corresponding frontend trend types and tests

Implementation steps:

1. Extract one account-eligibility helper used by both Overview and historical
   aggregation. It must reject archived Accounts and Accounts with
   `IncludeInNetWorth == false`; liability sign remains an aggregation concern.
2. Change snapshot net worth and trend point net worth from `*Money` to
   `*SignedMoney`. Change trend `Start` and `End` to signed values as well;
   `Change` remains signed. Assets and liabilities remain `*Money`.
3. In `BuildDailyValuationSnapshot`, sum `BaseAmountExact`, not
   `BaseValue.Amount`. Round once when constructing assets/liabilities/net worth.
4. Preserve each available component's canonical exact base amount in snapshot
   persistence. The least disruptive schema-9-compatible approach is:

   - treat the existing snapshot-item `base_amount` TEXT as an exact canonical
     calculation value on new writes;
   - expose a rounded `MoneyView` only when mapping to Wails DTOs;
   - validate the stored string with a bounded exact-decimal parser rather than
     `ParseMoney`;
   - keep old four-decimal rows readable because they are a valid subset.

5. Version snapshot content hashes, for example `v2:<sha256>`. When the latest
   closed-day revision has an old/unversioned hash, mark history dirty from the
   Origin and append corrected revisions. Do not update or delete old revisions.
6. Make `portfolioPointFromSnapshot` sum the stored exact strings and round once.
7. Update hash inputs, persistence helpers, Wails mappings, generated bindings,
   frontend trend parsing, and chart tests for negative values.

Acceptance criteria:

- Live and historical totals use identical eligibility and precision rules.
- Net worth may be negative everywhere it is displayed, persisted, or compared.
- Old schema-9 databases open without a schema reset; corrected derived
  snapshots are appended and Activities remain byte-for-byte untouched.
- Missing valuation components remain omitted and explicitly incomplete; they
  are never converted to zero.

### Work Package 2 — Close Transfer and Gain Correctness Gaps

**Covers:** F4–F6.

**Progress:** F4, F5, and F6 resolved.

Implementation steps:

1. In `buildPositionTransfer`, return the `NewQuantity` error for both source and
   destination results. Do not construct effects or endpoint views after an
   overflow. Keep commit atomic.
2. Introduce one participation policy for valuation/gain code:

   - an active Holding is calculated even if its Instrument identity is archived;
   - an archived Holding is omitted from current position totals;
   - immutable realized/dividend period events remain reportable after the
     Holding or Instrument is archived.

3. Remove `instrument.ArchivedAt != nil` skips from current gain paths where an
   active Holding still references the Instrument.
4. Add a cost-event repository option that includes archived Holdings for
   historical realized-gain reads. Do not globally remove archive filtering from
   current-position queries.
5. Extract a quote-selection function returning rate plus evidence/source. Use
   it from `ValuationService`, historical snapshots, `RealizedGainInRange`,
   acquisition FX, and dividend income.
6. Apply explicit preference-at-cutoff when present; otherwise use the service's
   configured/default provider key. Select stored observations only. Preserve
   direct/inverse behavior and missing-input diagnostics.

Acceptance criteria:

- Transfer overflow returns `decimal_overflow` (or the established typed code)
  and persists no rows.
- Archive/restore never changes historical totals solely by hiding identity.
- The same pair, cutoff, stored quotes, and preference state chooses the same FX
  observation in live valuation, snapshots, realized gains, and dividends.

### Work Package 3 — Correct Average Cost and Fee Policy

**Covers:** F11–F12 and rounding-policy debt.

**Progress:** F11 and F12 resolved.

Implementation steps:

1. Replace `RoundBank(2)` in `blendCost` with construction at the supported
   `UnitPrice` precision. If division produces more than eight fractional digits,
   apply `RoundBank(8)` once.
2. Keep trade `UnitPrice` price-only. Add a distinct fee-adjusted acquisition
   unit-cost value to the cost-basis event constructed for a buy:
   `(gross + buy fee) / quantity`, with checked decimal arithmetic.
3. Keep sell realized gain as proceeds minus remaining basis minus sell fee.
4. Ensure reversals and corrections reverse the same fee-adjusted basis facts;
   do not special-case them in the read model.
5. Replace generic `Round(8)` in implied trade-price calculation with the named
   banker's-rounding helper used by the accounting policy.
6. Update `domain-model.md` with the chosen net-of-fees rule and precision table.

Required cases:

- repeated buys at `1.23456789` and another midpoint price;
- buy fee only, sell fee only, and both fees;
- partial sale, transfer after a fee-bearing buy, reversal, and correction;
- crypto-scale fractional quantity and price;
- independent hand-calculated expected basis, proceeds, realized, and
  unrealized gain—not values copied from the implementation.

### Work Package 4 — Make SQLite Verification and Readers Fail Closed

**Covers:** F7–F8 and the integrity part of F15.

**Progress:** F7, F8, and F15 resolved.

Implementation steps:

1. Refactor schema verification so the account CHECK has two named accepted v9
   forms: the original balance-only `cash_on_hand` form and the widened
   balance-or-holdings form. Keep every other table/index/FK check exact.
2. `OpenReadOnlyForVerify` must accept either known form without writing,
   creating directories, applying repair, or enabling WAL. Report any other
   shape as `backup_integrity_failed`.
3. Normal writable `Open` may retain the lossless CHECK widening. Perform the
   PRAGMA and transaction work on a dedicated `*sql.Conn`, restore
   `foreign_keys=ON` with a defer/finalizer path, then run the existing full
   `verifySchema` before exposing the DB.
4. Test rollback/failure injection around create/copy/drop/rename/index/commit.
   The expected result is either the original valid table or the fully verified
   widened table.
5. Add `defer rows.Close()` immediately after every successful `QueryContext`.
   Audit the repository with `rg -n "QueryContext\\(" internal/infrastructure/sqlite`.
   Return `rows.Err()` after iteration and preserve a meaningful close error
   where applicable.
6. Replace `CAST(quantity AS REAL)` integrity checks with domain parsing of the
   bounded canonical text values. Never introduce float semantics in verification.
7. Make non-empty malformed institution/group IDs return a typed integrity
   error from account scanning. `nil` is reserved for SQL NULL/empty only.
8. Validate 10,000 ownership basis points in each repository transaction that
   replaces ownership and in startup verification. A trigger is optional; do
   not add one if it makes multi-row replacement temporarily invalid.
9. Extend verify-time scans to canonical money, quantity, price, and FX text
   columns that are not safely expressible as SQLite CHECK constraints.
10. Validate every non-null snapshot provenance identifier against its expected
    observation table and Household. If observation retention will ever delete
    source rows, make snapshot evidence self-contained before adding retention;
    a dangling provenance string is not acceptable audit evidence.

Acceptance criteria:

- Both supported v9 forms pass read-only backup inspection; an unknown form fails.
- Read-only verification produces no files and changes no bytes.
- Repair failure never exposes a partially rebuilt table.
- Corrupt IDs or decimals fail as integrity errors rather than appearing absent.
- A scan error cannot pin the single database connection.

### Work Package 5 — Preserve Actionable Startup Recovery

**Covers:** F9.

**Progress:** F9 resolved.

Implementation steps:

1. Add stable safe domain/wire codes for at least:
   `database_upgrade_required`, `database_from_newer_version`,
   `database_integrity_failed`, and `database_unavailable`.
2. Map `sqlite.BootstrapError.Status` in `cmd/nestworth/main.go` without sending
   its path or driver error to the webview. Keep full technical detail only in
   local diagnostics after applying the logging rules in Work Package 7.
3. Extend the App startup DTO to include safe found/supported schema versions
   where useful. Do not expose the absolute database path.
4. Give `BlockedStartupPage` distinct recovery copy and actions:

   - older schema: keep the file and restore/import through a supported path;
   - newer schema: open with a compatible/newer app;
   - integrity failure: preserve the file and Restore from backup;
   - unavailable: check permissions/disk and retry.

5. Keep RecoveryService and Catalog available in every blocked state.
6. Replace `os.Exit(1)` after `app.Run()` with a return path that executes DB and
   future cleanup defers.

Acceptance criteria:

- Unit tests cover every bootstrap status through the Wails DTO.
- A native smoke test proves each blocked page can open Restore.
- No user-facing error contains a filesystem path, SQL, or driver text.

### Work Package 6 — Replace the Partial Exclusive Flag With One Write Gate

**Covers:** F10 and backup-related architecture concerns.

**Progress:** F10 resolved. `WriteCoordinator` is the single write gate; exclusive backup/restore/CSV and ordinary mutations share it.

The current atomic `exclusive` flag is advisory: ordinary writers do not check
it. Replace it with one application-owned coordinator rather than adding
scattered `if exclusive` checks.

Implementation steps:

1. Add a `WriteCoordinator` with two operations:

   - `WithWrite(ctx, fn)`: for the persistence portion of every ordinary mutation;
   - `WithExclusive(ctx, kind, fn)`: for backup, Restore, and CSV commit.

2. Keep `changeMu` only for serializing ledger read-modify-write operations.
   Define and document lock order: exclusive/write coordinator first, then
   `changeMu`, then SQLite transaction. Every path must follow that order.
3. Route Account, Holding, Instrument, directory, icon, onboarding, history,
   settings, and default-directory writes through `WithWrite`.
4. Refresh performs provider network I/O without a write lock. Only its final
   revalidation/persistence step enters `WithWrite`. Starting exclusive work
   cancels active refreshes and prevents a late result from persisting.
5. Backup enters exclusive mode, cancels/waits for refresh, snapshots the DB,
   reads settings, verifies, hashes, and packages before leaving exclusive mode.
   Do not hold a raw mutex across a save-file dialog.
6. Restore enters exclusive mode, checkpoints while the session is open, closes,
   runs the journaled file-group swap, and quits. Keep the existing rule that a
   closed session requires restart even after a restore error.
7. CSV preview stays read-only. CSV commit enters exclusive mode and revalidates
   its token/fingerprint inside the write transaction.
8. Return `backup_restore_busy` promptly for conflicting operations rather than
   allowing a five-second SQLite busy timeout to become `internal`.

Concurrency matrix to test:

| Exclusive operation | Concurrent writer | Required result |
| --- | --- | --- |
| Backup | activity, directory, icon, settings | writer waits safely or receives typed busy; backup verifies |
| Backup | refresh fetch already running | fetch is cancelled or its persist is rejected; backup verifies |
| Restore | any mutation | mutation is rejected before DB close |
| CSV commit | activity or directory write | exactly one side obtains the gate; no partial import |
| Ordinary writes | ordinary writes | ledger operations remain serialized; no deadlock under `-race` |

### Work Package 7 — Tighten Privacy, File Modes, and Token Lifecycle

**Covers:** F16 and security/privacy review items.

**Progress:** F16 resolved. Live DB/WAL/SHM are forced to `0600`; logs omit paths and provider URLs; Restore/CSV previews expire, cap, and Restore stages a `0600` file instead of keeping the package in memory.

Implementation steps:

1. After validating that the path is the app-owned live database, enforce `0600`
   on the main DB. Apply restrictive modes to app-owned WAL/SHM files when they
   exist; keep the containing directory `0700`.
2. Replace logged database paths with a safe category or basename. Sanitize
   provider errors so URLs/symbols are not logged. Never log balances, notes,
   quantities, quote values, raw effects, credentials, or database rows.
3. Add `createdAt` to pending Restore and CSV sessions, use an injected clock,
   expire them after a documented short TTL, and cap the number and total bytes
   retained. Remove expired entries eagerly on inspect/confirm and on shutdown.
4. Pending Restore currently retains the complete in-memory backup package,
   including database bytes. Prefer storing an app-owned `0600` staged file plus
   fingerprint and deleting it on confirm, expiry, or shutdown.
5. Keep tokens random, single-use, and bound to fingerprint/path metadata.

Acceptance criteria:

- Permission tests cover new and pre-existing permissive files.
- Log-capture tests reject forbidden fields and paths.
- Expired, reused, and evicted tokens fail with stable codes and leave no staged files.

### Work Package 8 — Batch Analytics and List Hydration

**Covers:** F13 and the repository portion of F17.

**Progress:** F13 resolved. `AccountGains` reads one portfolio snapshot and shares one cost-basis replay; Investments uses a single TanStack query. Overview returns labels, history-started, and bounded recent-activity headlines from that same use case so the page no longer fans out to accounts/instruments/holdings/history APIs. F17 repository hydration is batched: a 50-row Activity page and a 365-day snapshot range use a bounded query count (`IN (...)` child lookups on existing indexes). Hidden-page query lifecycle remains Work Package 9.

Implementation steps:

1. Add `AccountGains(accountIDs)` or a Household gain snapshot endpoint. It must
   call `ReadPortfolioSnapshot` once, build one cost-basis replay context, and
   return a map/list keyed by Account ID.
2. Change `useHoldingGainsByAccounts` to one TanStack query. Invalidate it on the
   same Activity, Holding, quote, FX-preference, and archive mutations as current
   gain queries.
3. Create one Overview/Home read model from one repository snapshot. Include the
   current totals, breakdowns, missing inputs, names needed for labels, and the
   bounded recent-activity headlines required by that screen. Keep smaller APIs
   for detail screens.
4. Batch Activity effects/trade/dividend details by page IDs. Batch daily-
   snapshot items by snapshot IDs. Preserve deterministic ordering.
5. Add useful indexes for batched child lookups only after `EXPLAIN QUERY PLAN`
   proves they are needed.
6. Add query-count assertions and benchmarks using the generated realistic fixture.
7. Measure lazy closed-day snapshot creation separately from pure trend reads.
   Keep it cancellable and bounded, acquire the write coordinator only for the
   append transaction, and expose a clear calculating state if it remains lazy.

Acceptance criteria:

- Account gain work is O(entity types + events), not one household snapshot per Account.
- One Overview render observes one consistent snapshot.
- Query count for a 50-row Activity page and a 365-day snapshot range is bounded,
  not linear in parent row count.

### Work Package 9 — Finish History Pagination and Frontend Lifecycle

**Covers:** F14, frontend part of F17, and F21.

Implementation steps:

1. Convert History paging to `useInfiniteQuery` (or equivalent), passing the
   backend `next` cursor as `afterId`. Render a Load more action while `hasMore`
   is true; keep loaded pages stable after a mutation invalidation.
2. Test more than 50 Activities with identical timestamps/secondary sort keys to
   prove no duplicate or skipped row.
3. Decide per page whether state must survive navigation. Unmount recoverable
   pages or explicitly disable polling/effects while their container is hidden.
   Keep server state in TanStack Query rather than mounted DOM.
4. Replace ownership percent `Number` conversion with the existing exact string/
   bigint rational helper and keep Go validation authoritative.
5. Split large chart/analytics routes with dynamic imports. Record bundle sizes
   before and after; do not introduce a loading flash on the default Overview.

Acceptance criteria:

- Every historical Activity is reachable from the UI.
- Hidden pages perform no provider refresh and no unnecessary periodic IPC.
- Ownership basis points match Go for midpoint and long-decimal input cases.
- The Vite large-chunk warning is removed or a measured, documented exception.

### Work Package 10 — Clarify Module Boundaries Without a Rewrite

**Covers:** F18.

Do this only after P0 behavior is stable.

1. Split interfaces by use case while retaining one composition root:
   `DirectoryService`, `LedgerService`, `ValuationService`, `HistoryService`,
   `ImportExportService`, and `RecoveryService` are suitable boundaries.
2. Keep existing Wails method names/DTOs during the first extraction to avoid a
   simultaneous frontend migration.
3. Move CSV table/codec types behind an application-owned port; infrastructure
   implements parsing/encoding.
4. Move backup packaging, Restore orchestration, database lifecycle, and process
   restart decisions behind application ports. Wails adapters should select a
   file, map DTOs, and call a use case.
5. Make `Bootstrap` read-only. Move idempotent default-directory creation into
   onboarding or one explicitly gated initialization use case/transaction.
6. Keep domain packages independent of Wails, SQL, filesystem, HTTP, and UI types.

Acceptance criteria:

- Package dependency tests or an import-lint check enforce the direction.
- Backup/Restore and CSV orchestration can be integration-tested without Wails.
- No generic framework or service locator is introduced.

### Work Package 11 — Align Product Contracts and Release Engineering

**Covers:** F19–F20 and remaining optional/debt items.

1. Update `docs/architecture/domain-model.md` so displayed Activity labels are
   clearly mapped to stored kinds such as `cash_in`, `cash_out`, and
   `value_update`. Remove or mark deferred the nonexistent
   `COST_BASIS_DECLARATION` table and `LotRef` identity.
2. Document `include_in_portfolio` as persisted-but-unused and decide whether
   `include_in_liquid_assets` gets a named metric. Until such a metric ships,
   label it as reserved or hide it from active forms; do not imply it changes a
   visible total.
3. Add a client-generated mutation/request ID to Activity commands and a unique
   persistence constraint. A retry with the same ID and same payload returns the
   original result; the same ID with different payload returns conflict.
4. Add CI that runs formatting, Go tests, race (possibly a separate job), vet,
   Go build, frontend build/lint/typecheck/test, and `git diff --check`.
5. Pin Wails versions. Upgrade only in a dedicated change with regenerated
   bindings, full automated gates, and native/package smoke tests.
6. Keep FIFO/tax lots, shorts, margin, corporate actions, wash sales, staking,
   multi-hop FX, TWR/MWR, and encryption-at-rest as separately scoped product
   work. They are not required to close this review.
7. Keep preference salvage deliberately separate from ledger recovery. Resetting
   an invalid appearance/date-format preference to a safe default is acceptable;
   silently repairing or dropping invalid financial rows is not.

## Pull Request / Delivery Order

Use small PRs in this dependency order:

1. **Regression fixtures only:** Work Package 0.
2. **Historical signed/exact valuation:** Work Package 1.
3. **Transfer, archive, and FX consistency:** Work Package 2.
4. **Average-cost and fee policy:** Work Package 3.
5. **SQLite verification/read lifetime:** Work Package 4.
6. **Startup recovery contract:** Work Package 5.
7. **Global write coordinator:** Work Package 6.
8. **Privacy, permissions, token lifecycle:** Work Package 7.
9. **Batch APIs and repository hydration:** Work Package 8.
10. **History/front-end lifecycle:** Work Package 9.
11. **Boundary refactor:** Work Package 10.
12. **Documentation, idempotency, and CI:** Work Package 11.

Do not begin the boundary refactor before packages 1–7 are merged; otherwise
financial fixes and file-lifecycle moves will obscure each other's review.

## Definition of Done for Every Work Package

1. Add a failing regression before changing production behavior.
2. Preserve current user-database rows and immutable Activity history.
3. Return a stable `domain.ErrorCode` for expected failures; unexpected details
   stay out of the webview.
4. Run:

   ```sh
   test -z "$(gofmt -l cmd internal)"
   env GOCACHE=/tmp/nestworth-go-check go test ./...
   env GOCACHE=/tmp/nestworth-go-race go test -race ./...
   env GOCACHE=/tmp/nestworth-go-vet go vet ./...
   env GOCACHE=/tmp/nestworth-go-build go build ./cmd/nestworth
   cd frontend
   pnpm run build
   pnpm run lint
   pnpm run typecheck
   pnpm run test
   ```

5. Run `git diff --check` and verify that generated/temporary artifacts are not
   committed.
6. Update architecture and IPC documentation in the same PR when a contract,
   DTO, persistence meaning, error code, or accounting rule changes.
7. State native/manual checks as run or not run. Never infer native behavior from
   Vitest or Go tests.

## Final Release Gate

After all P0 and P1 packages are complete, perform a native packaged-app pass
using a disposable database and sanitized fixtures:

- onboarding and ordinary Account/Holding/Activity flows;
- liability-heavy negative net worth in Overview and History charts;
- excluded Account parity between Overview and closed-day snapshots;
- archived Instrument/Holding gain visibility rules;
- more than 50 History Activities and Load more;
- manual/provider quote and direct/inverse FX parity;
- backup during refresh and attempted writes;
- inspect and Restore both accepted v9 shapes;
- forced failed Restore followed by journal rollback on restart;
- blocked startup for older, newer, corrupt, and inaccessible databases;
- file permissions, no sensitive logs, native dialogs, quit/restart;
- keyboard navigation, focus restoration, screen-reader labels, and chart fallback text;
- packaged `.app`/DMG, signing, notarization, and clean-machine launch if this is
  a distribution build.

Release historical analytics only when F1–F6 and F11–F12 pass. Call
backup/Restore production-safe only when F7–F10 and F16 pass, including the
native restart/rollback cases.

## Source-Review Crosswalk

This crosswalk shows that every material source-review item has an owner in the
plan.

| Source-review issue | Final disposition |
| --- | --- |
| Historical inclusion, signed net worth, rounded totals | F1–F3; Work Package 1 |
| Position-transfer overflow | F4; Work Package 2 |
| Average-cost precision, buy fee, mixed rounding | F11–F12; Work Package 3 |
| Archived Instrument/Holding gain history | F5; Work Package 2 |
| v9 repair and read-only verifier mismatch | F7 plus corrected conclusion; Work Package 4 |
| Unclosed rows | F8; Work Package 4 |
| Startup error collapse and `os.Exit` cleanup | F9; Work Package 5 |
| Partial backup/write gate and bootstrap mutation | F10; Work Packages 6 and 10 |
| FX-selection asymmetry | F6; Work Package 2 |
| Account-gain and Overview IPC fan-out | F13; Work Package 8 |
| Activity/snapshot per-row hydration | F17; Work Package 8 |
| History cursor ignored | F14; Work Package 9 |
| Ownership sum, canonical numeric TEXT, `REAL` cast, malformed IDs, snapshot provenance | F15; Work Package 4 |
| DB mode, log privacy, pending-token lifecycle | F16; Work Package 7 |
| Broad service facade, CSV dependency, Wails lifecycle ownership | F18; Work Package 10 |
| Activity/domain-doc drift and unused inclusion flags | F19; Work Package 11 |
| Hidden mounted pages, JS ownership precision, bundle size | F17/F21; Work Package 9 |
| No mutation idempotency, no CI, Wails beta/native gates | F20; Work Package 11 and Final Release Gate |
| Lazy snapshot writes during trend reads, settings salvage | Bounded/explicit in Work Package 8; preference-only salvage retained in Work Package 11 |
| Deferred lots/returns/providers/encryption ideas | Explicitly out of review-remediation scope; separately planned product work |

## Final Assessment

Nestworth-go does not need a rewrite. It needs a disciplined sequence of
localized fixes that makes every read model obey the same financial rules,
makes recovery states explicit, and turns the current advisory exclusivity into
a real application-wide write boundary. The work is complete only when the
cross-path regression tests and native recovery gates pass; green unit tests
alone are not sufficient evidence for financial or Restore correctness.
