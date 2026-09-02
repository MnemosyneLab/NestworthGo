# Nestworth-go Technical Review

Review date: 2026-09-02  
Baseline: current checkout (`schema` 9, Wails v3 `v3.0.0-beta.12`, Go 1.26, shopspring/decimal, modernc SQLite).  
Method: read the domain, application, SQLite, Wails, and frontend implementations. Architecture docs were used as contracts, then checked against code. A second pass merged additional evidence from focused persistence, domain, architecture, and frontend explorations.

Issue types used below: **actual bug**, **architectural problem**, **technical debt**, **missing tests**, **optional improvement**.

## Executive Summary

Nestworth-go is a serious local-first ledger, not a UI prototype. The important contracts are real in code: typed money, one Household, immutable Activities, History Origin as a cutover, missing quotes treated as incomplete rather than zero, Wails adapters that do not own finance, and SQLite reads that load a portfolio snapshot in a bounded set of queries.

The remaining risk is concentrated in a few financial and durability paths, not in naming or component style.

The live Overview path is the most trustworthy valuation. Historical daily snapshots and net-worth trends do not yet share that same contract: they can include Accounts the user excluded from net worth, they round before aggregating, and they cannot represent a negative net worth. Average-cost gain is internally consistent but rounds unit cost to two decimals and leaves buy commissions out of cost. Realized/dividend FX also requires a stored preference that live valuation will invent. Opening an existing v9 database can rebuild the `accounts` table with foreign keys off; backup inspect/restore then rejects the unrepaired CHECK dialect. Directory writes and quote refresh are not on the backup exclusive gate. Investments issues one full-snapshot `AccountGain` per brokerage Account.

The application is in a good place to harden those paths before adding FIFO lots, returns, or more providers. Do not treat the current Overview numbers as proof that History, Analytics trends, or gain views match them.

## Critical / High Priority Findings

No issue in this checkout is a silent overwrite of posted Activities. The high-priority items are wrong or failing historical totals, cost-basis and FX-preference mismatch, a schema-rewrite / restore dialect split, an incomplete write gate, and chatty Investments IPC.

### H1. Historical snapshots ignore `include_in_net_worth`

- **Severity:** High
- **Type:** actual bug
- **Location:** `internal/application/historical_snapshot.go` (aggregation around the valued-account loop); contrast `internal/application/service.go` Overview filter
- **Problem:** Live Overview skips Accounts with `IncludeInNetWorth == false`. `BuildDailyValuationSnapshot` sums every non-archived valued Account, including excluded ones. Historical replay *does* copy the flag onto the reconstructed Account (`internal/application/historical_replay.go`), but the snapshot builder never reads it.
- **Why it matters:** History, Analytics net-worth trend, and Overview can disagree after a user unchecks “include in net worth”. That is a balance-sheet error, not a display quirk.
- **Recommended fix:** Apply the same include/archive rules as Overview, using `exactBaseAmount` (see H3). Add a regression that excludes an Account and asserts closed-day net worth matches Overview for the same reconstructed state.

### H2. Negative net worth cannot be stored or trended

- **Severity:** High
- **Type:** actual bug
- **Location:** `internal/application/historical_snapshot.go` (`NewMoney(assets.Sub(liabilities), …)`); `internal/application/trend.go` (`NewMoney(current.NetWorth, …)`); `internal/domain/model.go` `NewMoney`
- **Problem:** Overview net worth is a signed `decimal.Decimal` (`assets - liabilities`) and the Overview DTO can emit a leading minus. Daily snapshots store `net_worth_amount` as `Money`. `NewMoney` rejects negatives. A Household with liabilities larger than assets fails snapshot persistence and fails appending today’s trend point.
- **Why it matters:** Credit-card / loan-heavy Households are valid product states. Snapshot rebuild and Analytics then fail instead of showing a signed net worth.
- **Recommended fix:** Persist snapshot and trend net worth as `SignedMoney` (or a dedicated signed column). Keep assets and liabilities as non-negative `Money`. Add a test with assets 50 / liabilities 80.

### H3. Historical totals round per Account, then sum

- **Severity:** High
- **Type:** actual bug
- **Location:** `internal/application/historical_snapshot.go` (`account.BaseValue.Amount`); contrast `internal/application/service.go` `exactBaseAmount`
- **Problem:** Overview aggregates `BaseAmountExact`. Daily snapshots add the already-rounded `MoneyView` string on `AccountValuation.BaseValue`.
- **Why it matters:** The domain contract says closed-day totals come from the same valuation path and that aggregates must not be rebuilt from rounded strings. Multi-holding FX Accounts will drift from Overview by rounding remainder.
- **Recommended fix:** Sum `BaseAmountExact` (or `valuedAccount.baseExact`) and round once at the snapshot Money boundary.

### H4. Opening v9 rewrites `accounts` with foreign keys off

- **Severity:** High
- **Type:** reliability risk (crash / interruption)
- **Location:** `internal/infrastructure/sqlite/schema_repair.go` `repairV9CashOnHandHoldingsCheck`; called from `database.go` `Open` before `verifySchema`
- **Problem:** The approved exception to “no auto-migrate” rebuilds `accounts` via `accounts_new` / `DROP` / `RENAME`, with `PRAGMA foreign_keys = OFF` outside the transaction. Existing tests cover SQL-string replacement, not a live table rewrite, crash mid-swap, or `PRAGMA foreign_key_check` afterward.
- **Why it matters:** A crash between `DROP TABLE accounts` and restore of FKs/indexes can fail the next open with `integrity_failed` and no in-app migration back. This is the one write that runs on every existing v9 file at startup.
- **Recommended fix:** Keep the CHECK widen, but do it inside one immediate transaction, re-enable FKs, run `foreign_key_check`, and add a fixture test that opens a pre-repair v9 file, interrupts after `DROP`, and proves Open either rolls back or refuses to boot with a recoverable status. Do not add a second silent rewrite.

### H5. Average cost is rounded to two decimals

- **Severity:** High (crypto / high-precision instruments)
- **Type:** financial correctness
- **Location:** `internal/domain/cost_basis.go` `blendCost` (`RoundBank(2)`); `internal/domain/cost_basis_test.go` golden `93.33` / `340.02`
- **Problem:** `UnitPrice` allows eight fractional digits. Replay normalizes average cost to two places on every blend. Buys, transfers, and later sells all inherit that truncated average.
- **Why it matters:** Brokerage stocks in USD/CNY are close enough. Crypto, metals, and fractional FX-quoted instruments are not. Realized gain is then computed from the truncated average.
- **Recommended fix:** Keep full `UnitPrice` precision through replay. Round only when crossing a Money DTO. Re-golden tests against the eight-decimal contract, not cash-like two-decimal prices.

### H6. Buy commissions are not in cost basis

- **Severity:** High
- **Type:** financial correctness (product rule vs tax/cost convention)
- **Location:** `internal/domain/change.go` trade builder (unit price = gross / quantity; fee is a separate cash effect); `internal/domain/cost_basis.go` Buy uses `UnitPrice`, Sell subtracts fee from realized gain
- **Problem:** A buy of 10 shares for 1000 plus a 5 fee records average cost 100, not 100.5. Sell fees reduce realized gain; buy fees do not increase basis. Cash is correct (fee leaves the Account). Gain is biased upward.
- **Why it matters:** Users who record commissions will see optimistic unrealized/realized gain. This is defensible only if the product explicitly treats fees as period expense and never as basis.
- **Recommended fix:** Decide the rule in the domain contract, then implement it. If fees capitalize, blend `(gross + buyFee) / quantity` (or store an explicit unit cost). If they do not, document that on Investments/Analytics and keep tests that lock the choice.

### H7. Gain views drop holdings whose Instrument is archived

- **Severity:** High
- **Type:** actual bug
- **Location:** `internal/application/gain_service.go` (`instrument.ArchivedAt != nil { continue }`); contrast `internal/application/valuation.go` `valueAccount`
- **Problem:** Valuation still prices an active Holding whose Instrument is archived. GainService skips those holdings entirely, so Account/Investments gain can omit cost, value, and realized history that Overview still shows.
- **Why it matters:** Archive is reversible identity, not delete. Gain and valuation must use the same retained-reference rule.
- **Recommended fix:** Value archived Instruments that still have active Holdings. Omit only archived Holdings. Add a test that archives the Instrument and still returns HoldingGain.

### H8. Unrepaired v9 backups fail restore verify

- **Severity:** High
- **Type:** actual bug
- **Location:** `internal/infrastructure/sqlite/readonly.go` `OpenReadOnlyForVerify`; contrast `database.go` `Open` which calls `repairV9CashOnHandHoldingsCheck` before `verifySchema`
- **Problem:** Live open widens the `cash_on_hand` CHECK, then verifies. Read-only backup inspect requires the *widened* CHECK and never runs repair. A v9 file that has not been opened by the post-repair binary (older backup, copy, `VACUUM INTO` before first new-app launch) opens live after repair and fails restore/inspect.
- **Why it matters:** The product can read the user’s database and refuse to restore the same bytes. Two physical v9 dialects share `user_version = 9`.
- **Recommended fix:** Accept both CHECK forms in verify, or run the lossless widen in the read-only path, or bump `user_version` to 10 after rewrite. Add a test that `OpenReadOnlyForVerify` succeeds on an unrepaired v9 fixture.

### H9. Unclosed `Rows` can stall the only SQLite connection

- **Severity:** High
- **Type:** actual bug
- **Location:** `internal/infrastructure/sqlite/activity_repository.go` `ListActivities` (scan error returns without `rows.Close()`); same pattern in `snapshot_repository.go` `ListDailyValuationSnapshots` and `observation_repository.go` `listAccountStateObservationsQuery`. Pool is `SetMaxOpenConns(1)`.
- **Problem:** `ListActivityPage` closes on scan error. `ListActivities` does not. One unparseable activity/observation pins the writer connection until GC.
- **Why it matters:** The UI looks hung on every later query. History Overview still calls `ListActivities`.
- **Recommended fix:** `defer rows.Close()` on every query. Grep for `QueryContext` without defer. Add a test that a corrupt activity row still lets a subsequent query succeed.

### H10. Startup collapses schema failures into generic `unavailable`

- **Severity:** High
- **Type:** actual bug (contract)
- **Location:** `cmd/nestworth/main.go` (Open error always becomes `domain.ErrUnavailable`); statuses in `database.go` (`legacy_database`, `unsupported_future_database`, `integrity_failed`)
- **Problem:** `sqlite.Open` already distinguishes older, newer, and corrupt files. `main` throws that away. Blocked startup cannot tell the user to keep the file vs create a new database.
- **Why it matters:** A recoverable legacy file looks like “could not open.” Users can overwrite it.
- **Recommended fix:** Map `BootstrapError.Status` to the documented startup codes. Keep the path in slog only. Update `BlockedStartupPage` copy per code.

### H11. Backup exclusive lock does not cover directory or refresh writes

- **Severity:** High
- **Type:** actual bug (concurrency)
- **Location:** `internal/application/exclusive.go`; `CreateMember` in `service.go` (no `changeMu`); `internal/wailsapi/data/data.go` holds `changeMu` only around `SnapshotTo`
- **Problem:** `BeginExclusiveOperation` is a CAS flag used by backup/CSV/restore. Account/history mutations take `changeMu`. `CreateMember` / institution / group / icon setters and quote persist do neither.
- **Why it matters:** With `MaxOpenConns(1)` and `busy_timeout=5000`, a directory write during `VACUUM INTO` can hang then surface as generic `internal`. The snapshot can also miss concurrent directory/quote rows.
- **Recommended fix:** One write gate for every mutation, including directory and refresh persist. Prefer `defer Unlock`. Test that `CreateMember` during `SnapshotTo` waits or returns `ErrBackupRestoreBusy`.

### H12. Investments issues one full-snapshot `AccountGain` per Account

- **Severity:** High
- **Type:** performance / architectural
- **Location:** `frontend/src/queries/analytics.ts` `useHoldingGainsByAccounts`; `internal/application/gain_service.go` `AccountGain` (`ReadPortfolioSnapshot` then filter)
- **Problem:** n brokerage Accounts ⇒ n household snapshots and n cost-basis replays. `HoldingsByAccounts` already batches; gains do not.
- **Why it matters:** This is the worst frontend-driven query pattern in the app. It will dominate after a handful of Holdings Accounts.
- **Recommended fix:** One `AccountGains(accountIDs)` / household gain snapshot that reads once.

### H13. Realized/dividend FX requires a stored preference; live valuation does not

- **Severity:** High
- **Type:** actual bug
- **Location:** `internal/application/gain_service.go` `gainFXRateAtOrBefore`; contrast `internal/application/valuation.go` `convert`
- **Problem:** `convert` synthesizes a provider preference when none is stored and still converts. Realized/dividend conversion returns unavailable if `findFXPreference` is nil, even when a usable quote exists.
- **Why it matters:** Overview can be complete while Investments realized gain and Analytics dividends show missing FX for the same pair.
- **Recommended fix:** Share one quote-selection helper (implicit provider default + inverse). Test: USD holding, CNY base, quote present, no `fx_preferences` row.

## Domain & Financial Model

There is no separate `Position` entity. A Holding is the position: one active Instrument per Holdings Account, current quantity, archive flag, inherited ownership. Cash is an append-only per-currency observation. That model matches realistic brokerage, bank, crypto-exchange, and simple liability Accounts. Shorts, margin, pending/recurring Activity, FIFO lots, wash sales, staking, and unknown-basis lots are deferred in the domain document and are also absent from code.

**What is solid**

- Household singleton, immutable base currency, typed IDs, closed Account type/role/tracking table in both domain and `schema.sql`.
- Money, Quantity, UnitPrice, FxRate, SignedMoney are string-parsed at boundaries. JSON number decoders in Yahoo/Frankfurter use `UseNumber()`.
- Missing instrument or FX quotes mark the component unavailable and exclude it from totals. Identity FX (native == base) needs no quote.
- Activities are kind + typed effects, not editable rows. Reversal posts the inverse; correction is reversal + replacement in one transaction.
- `include_in_portfolio` is persisted and shown in the Account form, but Portfolio totals are “holding components on active non-liability Accounts”. That match is documented in `frontend/src/queries/portfolio.ts`. `include_in_liquid_assets` is persisted and replayed and never consumed by valuation.

**Gaps versus the domain document**

| Document claim | Code |
| --- | --- |
| Activity kinds such as Opening Adjustment, Deposit, Fee, Income | Stored kinds are `cash_in`, `cash_out`, `value_update`, `buy`, … Reasons carry `fee` / `income`. |
| `COST_BASIS_DECLARATION` / `LotRef` | No such table. Starting cost lives on origin components; acquisition cost on effects / trade details. |
| Direct and inverse FX produce the same rounded Money | Conversion uses multiply or divide; inverse rate uses `RoundBank(12)` in quote series. Worth a dedicated equality test on the valuation path. |

### Additional domain findings

**M1. Ownership 10,000 bps is application-only**  
- **Severity:** Medium  
- **Type:** architectural / persistence  
- **Location:** `internal/domain/model.go` `ParseOwnership`; `schema.sql` `account_ownership.share_bps`  
- **Problem:** SQL checks each share in `1..=10000` but does not require the Account total to be 10,000. Direct SQLite writes can create an invalid allocation.  
- **Recommended fix:** Keep app validation; add a repository/integrity test. A trigger is optional.

**M2. Quantity and money are unconstrained TEXT**  
- **Severity:** Medium  
- **Type:** persistence  
- **Location:** `schema.sql` `holdings.quantity`, `account_values.amount`, `account_cash_values.amount`  
- **Problem:** Canonical decimal syntax is enforced in Go parsers, not in CHECK clauses. A bug or future importer can persist `-1` quantity or non-canonical amounts.  
- **Recommended fix:** Add CHECKs that match the Go regexes, or a `PRAGMA`/`verify` scan of money columns in `verifySchema`.

**M3. Trade unit price uses `Round(8)`, not banker's rounding**  
- **Severity:** Medium  
- **Type:** financial correctness  
- **Location:** `internal/domain/change.go` (`Gross / Quantity`).Round(8)  
- **Problem:** Valuation and Money use `RoundBank`. Trade implied price uses half-away-from-zero at eight decimals.  
- **Recommended fix:** Use `RoundBank(8)` so midpoint cases match the rest of the book.

**M11. Archiving a Holding drops realized-gain history**  
- **Severity:** Medium  
- **Type:** actual bug  
- **Location:** `internal/infrastructure/sqlite/cost_basis_repository.go` (`archived_at IS NULL`); `internal/application/gain_service.go` `RealizedGainInRange`  
- **Problem:** Distinct from H7 (archived *Instrument*). Sold-then-archived Holdings vanish from realized-gain reports.  
- **Recommended fix:** Load cost-basis events without the archive filter when aggregating realized events.

## Architecture

Layering matches `docs/architecture/system-overview.md`.

- `internal/domain` does not import Wails, SQL, or OS APIs.
- `internal/application` owns use cases, `changeMu`, valuation, gain, history, CSV, refresh.
- `internal/infrastructure/sqlite` implements the repository port.
- `internal/wailsapi/*` maps commands to DTOs and `apierror.Wrap`.
- `cmd/nestworth` is the only package that imports `github.com/wailsapp/wails/v3` besides native dialogs.

The repository port is already split by context (`DirectoryRepository`, `AccountRepository`, `HistoryRepository`, …) even though `sqlite.Repository` is one type. That is the right direction.

**M4. `application.Service` is still the god facade**  
- **Severity:** Medium  
- **Type:** architectural / technical debt  
- **Location:** `internal/application/service.go` plus `change_service.go`, `history*.go`, `portfolio.go`, `csv.go`, `refresh.go`, `exclusive.go` — on the order of 80 exported methods  
- **Problem:** Tests and Wails adapters all depend on one concrete type. Valuation and Gain are separate structs but are reached through Service. File split helped; the type did not shrink.  
- **Why it matters:** New history/import features keep attaching to the same mutex, clock, and repository.  
- **Recommended fix:** Do not rewrite. Extract write vs read facades when the next large feature lands (for example CSV + restore already have `exclusive` / `LockWrites`). Keep one composition root in `cmd/nestworth`.

**M5. Domain document and stored Activity taxonomy have drifted**  
- **Severity:** Medium  
- **Type:** technical debt  
- **Location:** `docs/architecture/domain-model.md` vs `internal/domain/change.go`  
- **Recommended fix:** Treat stored kinds as canonical. Update the domain document to the `cash_in` / `value_update` vocabulary, and keep UI copy as a presentation map.

**M13. Application imports `internal/infrastructure/csvcodec`**  
- **Severity:** Medium  
- **Type:** architectural  
- **Location:** `internal/application/csv.go`, `csv_plan.go`  
- **Problem:** Production application code takes `csvcodec.Table` on its API. Domain already has `CSVImportBatch`.  
- **Recommended fix:** An application-owned table port; infrastructure implements it.

**M14. `wailsapi/data` and `recovery` own file and DB lifecycle**  
- **Severity:** Medium  
- **Type:** architectural  
- **Location:** `internal/wailsapi/data/data.go`, `internal/wailsapi/recovery/recovery.go`  
- **Problem:** Household/history adapters only call `application.Service`. Data/recovery hold `*sqlite.DB`, run ZIP/CSV sessions, and `go quit.Quit()`.  
- **Recommended fix:** Move orchestration behind application ports; keep Wails as DTO + `apierror.Wrap`.

## Persistence

SQLite setup is deliberate: `_txlock=immediate`, `foreign_keys=1`, `busy_timeout=5000`, `MaxOpenConns(1)`, WAL for on-disk files, `PRAGMA integrity_check` in `verifySchema`, latest-observation indexes, unique active holding per (account, instrument), Household `singleton_key = 1`.

`ReadPortfolioSnapshot` loads household, origin, directory, accounts, instruments, holdings, cash, quotes, and FX in one read-only transaction. Query count is bounded by entity type, not by holding count.

Older non-empty databases are rejected (`StatusLegacyDatabase`). Future `user_version` is rejected. Fresh files apply `schema.sql` in a transaction. That is a coherent compatibility story for a breaking schema-9 generation.

Backup/restore is more mature than typical hobby desktop apps: `VACUUM INTO` snapshots, 0600 packaged members, restore journal states, startup `ReconcileOnStartup`, exclusive lock, WAL checkpoint, close-then-swap.

**M6. Live database file mode is not forced to 0600**  
- **Severity:** Medium  
- **Type:** security / privacy  
- **Location:** `internal/infrastructure/sqlite/database.go` `Open` (directory `0700`); contrast `SnapshotTo` / backup `Chmod(0600)`  
- **Problem:** The live `nestworth.db` inherits umask. On a shared Mac user account that can be world-readable.  
- **Recommended fix:** `chmod 0600` after create/open, including WAL/SHM if present.

**M7. Repair tests do not open a real pre-repair database**  
- **Severity:** Medium  
- **Type:** missing tests  
- **Location:** `internal/infrastructure/sqlite/schema_repair_test.go`  
- **Recommended fix:** See H4 and H8. Add `testdata/` with the old CHECK and assert both `Open` and `OpenReadOnlyForVerify`.

**M15. Malformed institution/group IDs are dropped on read**  
- **Severity:** Medium  
- **Type:** actual bug  
- **Location:** `internal/infrastructure/sqlite/repository.go` `parseInstitutionID` / `parseGroupID`  
- **Problem:** Parse failure becomes `nil` (unassigned), not `ErrIntegrity`.  
- **Recommended fix:** Return the parse error from `scanAccountRecord`.

**M16. Schema verify uses `CAST(quantity AS REAL)`**  
- **Severity:** Medium  
- **Type:** actual bug vs invariant  
- **Location:** `internal/infrastructure/sqlite/schema_verify.go`; `cost_basis_repository.go`  
- **Problem:** Domain forbids binary float for quantities. Used as a `> 0` predicate on Open.  
- **Recommended fix:** Compare canonical TEXT, or parse with `ParseQuantity`.

**M17. Activity list hydrates per row**  
- **Severity:** Medium  
- **Type:** architectural / performance  
- **Location:** `internal/infrastructure/sqlite/activity_repository.go` (`activityEffects` + `attachActivityDetails` per activity)  
- **Problem:** Contradicts “must not query once per result row.” Snapshot batch path already does the right thing.  
- **Recommended fix:** `WHERE activity_id IN (...)` for effects and trade/dividend details. Index `activity_effect_id` on cash/value projections.

## Wails Integration

Wails does not leak into domain or application. Bindings are generated and gitignored; `task setup` regenerates them. Adapters parse IDs, map DTOs, and wrap errors. History timezone is taken from Origin, not from the frontend payload.

Startup is careful: if SQLite cannot open, only App, Catalog, and Recovery are registered so the UI can render `BlockedStartupPage` instead of raw “service not found”. Refresh completion is a typed event with a lazy emitter so `wailsapi/marketdata` does not import Wails.

`WireError` JSON-in-`Error()` is an unusual but documented v3 constraint; frontend `parseWailsError` treats non-JSON as generic `internal`.

**M8. Overview page fans out extra IPC**  
- **Severity:** Medium  
- **Type:** performance / Wails  
- **Location:** `frontend/src/features/overview/OverviewPage.tsx` (`useOverview`, `useAccounts`, `useHoldingsByAccounts`, `useInstruments`, `useHistoryOrigin`, `useListActivities`, `useSettings`)  
- **Problem:** Overview DTO already has totals, missing inputs, and breakdowns. Extra list calls reload holdings/instruments for chrome and a recent-activity strip. Each call is another snapshot-ish read under `changeMu` contention.  
- **Recommended fix:** Extend Overview (or a small `HomeDTO`) with activity headlines and account-name map. Keep TanStack Query; reduce the number of bound methods per paint.

**M9. Wails v3 is still beta**  
- **Severity:** Medium  
- **Type:** technical debt / optional  
- **Location:** `go.mod` `v3.0.0-beta.12`  
- **Problem:** Binding, event, and error-propagation behavior can still change.  
- **Recommended fix:** Pin, run `task check` after upgrades, and keep `apierror` tests as the IPC contract.

**M18. History UI ignores the page cursor**  
- **Severity:** Medium  
- **Type:** architectural / missing tests  
- **Location:** `frontend/src/features/history/HistoryPage.tsx` (`limit: 50`, no `afterId`)  
- **Problem:** Backend returns `hasMore` + `next`. The page never sends the cursor. Older activity is silently dropped.  
- **Recommended fix:** Load-more using `activities.data.next`. Test that `hasMore: true` shows a control.

## Backend

Write serialization (`changeMu`) plus SQLite single-connection is the right concurrency model for ledger mutations. RecordChange reloads state rather than trusting a preview. Clocks are injectable. Domain errors are stable codes. `apierror.Wrap` strips unexpected Go errors to `internal`.

Logging does not include balances, notes, quantities, or SQL rows. It does log the full database path (`cmd/nestworth/main.go`) and, at Debug, provider transport errors that often contain URLs/symbols. That violates the architecture “no paths/symbols in logs” rule.

`LockWrites` / `UnlockWrites` are a raw mutex expose (H11). `os.Exit(1)` on `app.Run()` error skips `defer database.Close()`. Onboarding commits the Household, then writes default institution/group in follow-up transactions.

No process-wide mutable finance globals. `init` only registers the refresh event type.

## Frontend

Feature folders, TanStack Query, Zod forms, and `callService` are a maintainable desktop UI. `frontend/src/lib/money.ts` formats and rounds with bigint/banker's rounding for display and form helpers. Charts convert strings to `number` only for pixel geometry.

`CalculateAmounts` fills one side of a form from quantity/price or FX. Go still validates on commit. That is allowed helper math, not a second ledger.

Loading, error, and empty states exist via `PageState`. i18n coverage tests exist. Frontend tests cover pages, history copy, invalidation, and money (34 test files). Visited pages stay mounted (`App.tsx` `hidden=`), so query observers keep running. Restore/CSV confirm tokens are returned to the webview with no TTL.

**M10. Ownership percent uses IEEE `Number`**  
- **Severity:** Low  
- **Type:** actual bug (edge) / optional  
- **Location:** `frontend/src/features/accounts/accountCatalog.ts` (`Math.round(Number(percentages[index]) * 100)`)  
- **Problem:** Basis points are integers in Go. A long percent string can round differently than `PercentToBasisPoints`.  
- **Recommended fix:** Reuse the bigint helper in `money.ts` (`roundedRational`) or send percents as strings and let Go convert.

## Reliability & Security

**Strengths:** immutable ledger, preview ≠ commit, reversal/fix, WAL, immediate transactions, restore journal, CSV import in one transaction, blocked startup without business writes.

**Remaining reliability**

- H1–H3 can persist wrong historical facts (snapshots are append-only revisions; a later rebuild can correct them if dirty-from is set, but the stored revision is still wrong until rebuilt).
- H4 and H8 are the main crash-safety / restore holes on ordinary launch.
- H9 can freeze the process after one bad row.
- H11 can race backup with directory/refresh writes.
- Idempotency: archive-of-already-archived is handled. RecordChange is not idempotent by client nonce; a double submit can post twice. Medium for a desktop app; worth a UI disable + mutex (already have mutex).

**Trust boundary:** the webview is treated as untrusted for finance. Household ID and timezone are server-derived. File import goes through native dialogs and CSV planning, not arbitrary SQL.

**Privacy:** local DB, no account registration, provider calls only on explicit refresh. Live DB mode (M6) is the main file-permission gap. Settings JSON is 0600.

## Performance

Current scale (personal Household, hundreds to low thousands of Activities) is within the snapshot + single-connection design.

Bottlenecks when history grows:

- Investments N+1 `AccountGain` (H12) is the first to bite.
- Gain replay walks per-holding cost events on read; AccountGain loops holdings sequentially.
- Daily snapshot rebuild replays origin + activities per closed day (batch cache exists and should stay on the hot path).
- Overview extra IPC (M8); History over-fetches holdings for labels.
- Activity list hydrates per row (M17). The page also never uses `hasMore` (M18).

SQLite latest-quote indexes are appropriate. Do not add per-row queries in repository list methods.

## Testing & Engineering

**Strengths:** 68 Go `*_test.go` files under `internal`, covering domain decimal/cost basis, change commands, valuation, historical snapshots, concurrency of history writes, schema verify/integrity, backup journal, Wails mapping, CSV. Frontend Vitest coverage of pages and money is better than most Wails apps.

**Gaps**

- No GitHub Actions workflow. `task check` is the suite (gofmt, `go test ./...`, vet, build, pnpm lint/typecheck/test). CI is local-only.
- No test that liabilities > assets.
- No test that excluded Accounts disappear from daily snapshots.
- No live v9 CHECK-rewrite integration test.
- No test that archived Instrument still appears in GainService (H7).
- No test that unrepaired v9 passes `OpenReadOnlyForVerify` (H8).
- No test that `CreateMember` during backup is busy or waits (H11).
- No History `hasMore` UI test (M18). Restore dialog tests are almost absent.
- Native GUI / packaged `.app` remains a separate unrun gate (as in prior work on this repo).

## Technical Debt

1. `application.Service` surface area (M4).
2. Documented-but-unused `include_in_portfolio` / `include_in_liquid_assets` semantics.
3. Domain-doc Activity names vs stored kinds (M5).
4. `CostBasisDeclaration` in the ER diagram with no table.
5. Wails v3 beta pin (M9).
6. Average-cost two-decimal convention baked into goldens (H5).
7. Settings salvage that resets invalid fields rather than failing closed (acceptable for prefs, not for the ledger).

None of these should block fixing H1–H3, H7–H13.

## Recommended Roadmap

### 1. Fix immediately

1. Filter historical snapshot aggregates with `IncludeInNetWorth` (H1).
2. Allow signed snapshot/trend net worth (H2).
3. Aggregate historical totals from exact base amounts (H3).
4. Stop skipping archived Instruments in GainService (H7).
5. Map bootstrap statuses to distinct blocked-startup codes (H10).
6. `defer rows.Close()` on every SQLite query (H9).
7. Share FX quote selection between valuation and realized/dividend (H13).
8. Add regressions for those items.

### 2. Fix before the next major feature

1. Close the v9 CHECK rewrite crash window; make restore verify accept unrepaired v9 (H4, H8, M7).
2. Put directory and refresh persist on the same write gate as backup (H11).
3. Batch Investments `AccountGain` (H12).
4. Decide and implement buy-fee cost basis (H6); re-golden average cost precision (H5).
5. History load-more (M18); `chmod 0600` live DB (M6); trade `RoundBank(8)` (M3).
6. Wire `task check` into CI.

### 3. Medium-term refactoring

1. Split read vs write application facades without changing Wails method names (M4).
2. Collapse Overview IPC into one DTO (M8); batch activity list hydration (M17).
3. Align domain-model.md with stored Activity kinds and drop the phantom declaration table (M5).
4. SQL CHECKs or verify-time scans for money/quantity/ownership totals (M1, M2).
5. Keep realized history for archived Holdings (M11); stop silent ID drops (M15).
6. Either implement liquid-asset totals or hide `include_in_liquid_assets` from the UI.
7. Move CSV/backup orchestration out of `wailsapi` (M13, M14).

### 4. Optional improvements

1. FIFO / tax lots, unknown basis, TWR/MWR (already deferred).
2. Client nonce / idempotency keys on RecordChange.
3. Encrypt-at-rest or macOS Data Protection for `nestworth.db`.
4. Multi-hop FX (explicitly out of scope today).
5. Wails v3 stable upgrade when it exists (M9).
6. Replace frontend ownership `Number` conversion (M10).
7. Log database basename, not the home-directory path. Expire restore/CSV tokens.

---

This review did not start the native app or exercise the GUI. Automated tests were not re-run in this pass; claims about missing tests come from reading the test files named above.
