# Nestworth-go Technical Review

Review date: 2026-09-02<br>
Review scope: the full repository, including the domain model, application services, SQLite persistence, backup/restore, Wails adapters, frontend, tests, documentation, and local engineering workflow.

This is an independent review of the current checkout. `docs/development/technical-review.md` was used as a reference for areas worth re-checking, but the findings below are based on the current implementation and the validation run in this review.

## Executive Summary

Nestworth-go has a strong foundation for a local-first personal-finance application. The domain layer uses typed identifiers and exact decimal values, activities are treated as an immutable ledger with reversal support, the application layer separates business rules from Wails and SQLite, backup archives have meaningful path and size checks, and the automated Go and frontend suites are currently green.

The main release risk is not basic engineering hygiene; it is the difference between the live valuation path and the historical analytics/persistence path. Historical snapshots currently aggregate accounts without applying `IncludeInNetWorth`, cannot represent a valid negative net worth, and aggregate already-rounded monetary values. Those issues can make a historical chart disagree with the current overview or fail for a liability-heavy household. Cost-basis precision, transaction fees, archived instruments, and position-transfer overflow create additional ways for financially meaningful results to diverge from user expectations.

No critical security exploit was established in this pass. There are, however, several high-severity correctness and data-integrity findings that should be addressed before treating historical analytics and advanced investment accounting as authoritative.

### Priority summary

| Priority | Scope | Theme |
| --- | --- | --- |
| High | H1–H14 | Historical valuation, cost basis, archival semantics, schema/backup compatibility, resource handling, concurrency, and analytics scaling |
| Medium | Several findings | Database invariant enforcement, architecture boundaries, read-path behavior, permissions, pagination, and observability |
| Low / debt | Several findings | Explicit product limits, frontend precision ownership, Wails beta lifecycle, and documentation/CI gaps |

## Validation and Review Boundaries

The following automated checks were run from the repository root unless noted otherwise:

- `env GOCACHE=/tmp/nestworth-gpt-review go test ./...` — passed.
- `env GOCACHE=/tmp/nestworth-gpt-review go test -race ./...` — passed.
- `env GOCACHE=/tmp/nestworth-gpt-review go vet ./...` — passed.
- `env GOCACHE=/tmp/nestworth-gpt-review go build ./cmd/nestworth` — passed.
- `gofmt -l cmd internal` — no output.
- `pnpm run test` from `frontend/` — 34 test files and 278 tests passed.
- `pnpm run typecheck` from `frontend/` — passed.
- `pnpm run lint` from `frontend/` — passed.

The Go build and test commands emitted local macOS deployment-target linker warnings, but exited successfully. I did not start the Wails application, open a browser, perform a native GUI walkthrough, build/package a distributable app, or validate signing/notarization/accessibility with the native shell. The frontend build script was not used as a substitute for those checks.

## Critical / High Priority Findings

### H1 — Historical snapshots ignore `IncludeInNetWorth`

Severity: High<br>
Type: Actual financial correctness bug<br>
Location: `internal/application/historical_snapshot.go:73-131`; account-state reconstruction in `internal/application/historical_replay.go:155-200`

The replay layer reconstructs account state, including `IncludeInNetWorth`, but `BuildDailyValuationSnapshot` iterates over every valued account and adds its base value to assets or liabilities without checking that flag. The live overview applies the flag before calculating totals in `internal/application/service.go:990-1008`.

This makes a historical net-worth point disagree with the live overview whenever an account is excluded from net worth. It is especially easy to encounter after a user changes an account’s inclusion setting because the historical replay has the necessary state but the snapshot aggregator drops it.

Recommended fix:

- Centralize net-worth eligibility in one valuation/aggregation function used by both live and historical paths.
- Apply the reconstructed account state before adding any historical asset or liability amount.
- Add a regression test with an account that changes `IncludeInNetWorth` over time and assert that the historical total changes at the correct date.

### H2 — Negative net worth cannot be represented by historical snapshots

Severity: High<br>
Type: Actual financial correctness bug<br>
Location: `internal/application/historical_snapshot.go:121-131`; `internal/application/trend.go:33-48`; `internal/domain/model.go:303-370`

`Money` intentionally rejects negative values, while net worth is a signed aggregate. Historical snapshot construction calls `NewMoney(assets.Sub(liabilities), ...)`; the trend reader repeats the same assumption when converting stored totals. A household whose liabilities exceed its assets therefore cannot produce a valid historical snapshot or trend point.

The live overview already calculates assets and liabilities separately and exposes their signed difference. The historical path should preserve the same contract rather than forcing a signed result through a non-negative money type.

Recommended fix:

- Represent net worth as `SignedMoney`, or as a dedicated signed total that retains the existing currency and precision rules.
- Keep assets and liabilities non-negative individually; only the net result should be signed.
- Add tests for liability-only and liability-greater-than-asset households through snapshot save, reload, and `NetWorthTrend`.

### H3 — Historical totals are built from rounded values

Severity: High<br>
Type: Actual precision/correctness bug<br>
Location: `internal/application/historical_snapshot.go:73-119`; `internal/application/trend.go:202-232`; exact live aggregation in `internal/application/valuation.go:90-180,720-732`

The live valuation path retains exact decimal component amounts and rounds only when creating the money-facing view. Historical snapshot construction instead parses each `AccountValuation.BaseValue` and each component `BaseAmount`, both of which are already `Money` values rounded to four decimal places. Historical portfolio trend points then sum the stored rounded item amounts again.

The result is cumulative drift across accounts, holdings, and dates. It also means the historical series can disagree with the live total even when the same quotes and activities are used. Storing `NativeAmount` at high precision does not fully solve this because the historical aggregate currently does not reconstruct the exact base amount from it.

Recommended fix:

- Aggregate from `BaseAmountExact` or an equivalent exact intermediate representation while building the snapshot.
- Persist either an exact bounded base amount alongside the money-facing value or enough exact inputs to deterministically reconstruct it.
- Round only at the final DTO/display boundary.
- Add a persistence regression using a high-precision quantity × unit price, then compare live valuation, stored snapshot, reload, and both trend APIs.

### H4 — Position-transfer quantity overflow is silently ignored

Severity: High<br>
Type: Actual data-integrity bug at a supported numeric boundary<br>
Location: `internal/domain/change.go:1093-1121`, `buildPositionTransfer`

The position-transfer builder calculates the source and destination quantities with `NewQuantity`, but discards both errors. The adjustment path immediately below it checks the error correctly. If the destination quantity exceeds the domain’s quantity bounds, the endpoint view can contain the zero value or another invalid result, and the SQLite change repository uses that endpoint result when applying the effect.

This is a silent corruption path: the operation can be accepted as a valid transfer while storing an incorrect holding quantity. Ordinary transfer tests cover normal values, but not the maximum representable quantity plus a positive transfer.

Recommended fix:

- Return the `NewQuantity` errors from `buildPositionTransfer` and abort the change before repository writes.
- Keep the operation atomic and verify that both the source and destination holdings remain unchanged on failure.
- Add boundary tests for overflow, underflow, zero quantity, and same-instrument transfers.

### H5 — Average cost is normalized to two decimals despite higher-precision unit prices

Severity: High<br>
Type: Financial precision defect or undocumented product constraint<br>
Location: `internal/domain/cost_basis.go:67-71,213-236`; quantity/unit-price precision in `internal/domain/decimal.go:9-19`

`blendCost` rounds average cost with `RoundBank(2)` before constructing the new unit price. The domain accepts unit prices with up to eight fractional digits, so a portfolio containing fractional-price assets can lose information each time buys are blended. The current tests encode the two-decimal result, which makes the behavior intentional in code but not necessarily correct for the supported instrument set.

Recommended fix:

- Decide and document whether average cost is a money-facing two-decimal field or a calculation field.
- If it is a calculation field, retain the declared unit-price precision and round only realized/display outputs.
- If two decimals are required by product policy, narrow the input contract and document the resulting accounting limitation.
- Add tests covering repeated buys at prices such as `1.23456789` and compare realized gain against an independently calculated exact result.

### H6 — Buy-side fees are not included in cost basis

Severity: High if gains are intended to be after fees; otherwise Medium contract debt<br>
Type: Accounting-policy gap<br>
Location: trade effect construction in `internal/domain/change.go:1250-1267`; cost-basis replay in `internal/domain/cost_basis.go:127-202`

Buy fees are represented as a separate effect, while the buy event’s unit cost is derived from the trade price. The cost-basis replay therefore does not add the buy fee to the acquired position’s basis. A later sale can report a gain that ignores acquisition costs even though cash was reduced by the fee.

This may be correct if the product deliberately reports price-only gain, but the current naming and user-facing analytics do not make that distinction explicit. The absence of a test also makes the policy easy to change accidentally.

Recommended fix:

- Define whether gains are gross, net of sell fees, or net of both buy and sell fees.
- Encode that policy in a named cost-basis rule rather than relying on effect ordering.
- Add buy-fee, sell-fee, and round-trip tests with expected basis, proceeds, cash, and realized gain.

### H7 — Gain services drop archived instruments while valuation retains their active holdings

Severity: High<br>
Type: Actual historical/accounting consistency bug<br>
Location: `internal/application/gain_service.go:77-83,148-155,215-310`; active-holding valuation in `internal/application/valuation.go:250-267`

Valuation preserves an active holding even when its instrument has been archived, which is necessary for historical identity and existing balances. Holding, account, realized-gain, and dividend calculations skip archived instruments. Archiving an instrument can therefore make the same holding appear in valuation but disappear from gains or income analytics.

Recommended fix:

- Treat instrument archival as reversible identity metadata, not as a reason to remove historical positions from calculations.
- Use a single “holding participates in this calculation” policy that distinguishes active holdings from archived holdings and handles archived instruments consistently.
- Add tests for an active holding whose instrument is archived, covering current value, holding gain, account gain, realized gain, dividend, and history display.

### H8 — v9 schema repair has a fragile foreign-key and verification boundary

Severity: High<br>
Type: Migration/recovery reliability risk<br>
Location: `internal/infrastructure/sqlite/schema_repair.go:68-109`; verification in `internal/infrastructure/sqlite/schema_verify.go:48-110`

The v9 repair disables foreign keys before starting the replacement-table transaction, copies rows, drops and renames the table, recreates indexes, and commits. The repair function does not perform an explicit `foreign_key_check` after the replacement before ordinary startup verification. Error paths and process interruption behavior are not covered by a repair fixture or crash-style test.

The repair is handling a user database, so an unusual row or an interruption during this boundary must result in either a verified upgraded database or a recoverable original—not a partially trusted file.

Recommended fix:

- Make the repair operation a single, explicitly audited migration boundary with guaranteed restoration of connection settings on every exit path.
- Run schema and foreign-key verification immediately after repair and before returning the database to the application.
- Preserve a recoverable copy or journal state until post-repair verification succeeds.
- Add fixtures for both accepted v9 account-check variants, malformed rows, interruption/error paths, and data preservation.

### H9 — Read-only backup verification rejects a valid unrepaired v9 database

Severity: High<br>
Type: Actual backup/restore compatibility bug<br>
Location: `internal/infrastructure/sqlite/readonly.go:15-55`; v9 repair in `internal/infrastructure/sqlite/schema_repair.go:15-42`

`OpenReadOnlyForVerify` opens the database with `mode=ro`, requires the exact current schema version, and calls verification without running repair. The normal writable open path can repair an older accepted v9 shape, but backup inspection and restore verification cannot. A backup containing that accepted v9 shape can therefore be rejected even though the application would repair it successfully when opened normally.

Recommended fix:

- Choose one compatibility contract and apply it consistently: either verify all accepted v9 shapes read-only, or stage a verified copy and repair that copy without touching the source archive.
- Keep the original backup immutable.
- Add a backup fixture test that writes each supported v9 shape, inspects it, confirms it, and verifies restored data.

### H10 — Several row-scan error paths leak `Rows`

Severity: High for long-lived sessions; Medium otherwise<br>
Type: Actual resource-lifetime bug<br>
Location: `internal/infrastructure/sqlite/activity_repository.go:176-197`; `internal/infrastructure/sqlite/observation_repository.go:30-83`

`ListActivities` and `ListAccountStateObservations` close their query rows only on the success path. A scan error returns before `Rows.Close` is called. The paginated activity methods handle this more carefully, so the problem is localized but real.

With a single SQLite connection this can keep statements or read state alive and make later operations fail or appear stuck after malformed data or a driver-level scan error.

Recommended fix:

- Defer `rows.Close()` immediately after every successful `QueryContext`/`Query` call.
- Preserve explicit close checks where the driver can surface iteration errors.
- Add a repository test that forces a scan/iteration error and then performs a second query on the same database.

### H11 — Startup collapses actionable database failures into generic unavailability

Severity: High<br>
Type: Operability and recovery UX defect<br>
Location: `cmd/nestworth/main.go:63-115`; `internal/wailsapi/app/app.go:11-56`; `internal/wailsapi/apierror/apierror.go:13-58`

The command entry point replaces any `BootstrapError` from SQLite open with the generic `domain.ErrUnavailable`. The Wails startup DTO consequently loses the distinction between a future schema, failed repair, integrity failure, permissions problem, or ordinary unavailable database. The safe error wrapper is appropriate for unexpected errors, but startup needs a controlled recovery contract.

Recommended fix:

- Map known bootstrap failures to stable, safe error codes such as `schema_upgrade_required`, `database_corrupt`, `database_inaccessible`, or `database_unavailable`.
- Keep paths and driver details in local logs only, with redacted user-facing messages and an explicit recovery action.
- Add Wails adapter tests for each bootstrap category.

### H12 — Backup quiescing does not cover all writers

Severity: High<br>
Type: Actual consistency/concurrency risk<br>
Location: `internal/wailsapi/data/data.go:100-181`; write gates in `internal/application/exclusive.go`

`CreateBackup` begins an exclusive operation and locks writes around `SnapshotTo`, but it releases the database write lock before all backup work is complete. Default-directory creation during `Bootstrap`, refresh persistence, and some ordinary mutation paths are not uniformly guarded by the same exclusive state. Settings and the database are therefore not guaranteed to describe one quiescent application state when they are packaged together.

Recommended fix:

- Define one application-wide quiesce protocol covering user mutations, refresh writers, default-directory initialization, WAL checkpointing, database snapshot, and settings read.
- Hold the protocol until all files in the package have been read and checksummed, or capture each source under a documented consistent snapshot boundary.
- Add concurrent backup-versus-refresh and backup-versus-mutation tests that verify either a consistent archive or a clean refusal.

### H13 — Historical FX policy is not symmetric across analytics

Severity: High<br>
Type: Actual calculation-policy inconsistency<br>
Location: `internal/application/gain_service.go`; live valuation and FX preference handling in `internal/application/valuation.go` and `internal/application/service.go`

Realized-gain and dividend paths use historical FX resolution, while the live valuation path can synthesize a provider-based rate when a stored historical preference or quote is absent. The same economic event can consequently be converted using different source, date, or fallback rules depending on which analytics endpoint reads it.

Recommended fix:

- Define one FX policy for event conversion, live valuation, and historical snapshots: source preference, date selection, missing-rate behavior, and precision.
- Persist the chosen rate/source for immutable historical calculations where reproducibility is required.
- Return an explicit incomplete state when the policy cannot produce a rate; do not silently substitute a different source.
- Add cross-endpoint tests for the same multi-currency event and date.

### H14 — Per-account gain queries cause full-snapshot N+1 work

Severity: High at portfolio scale; Medium for a small personal dataset<br>
Type: Actual performance/design issue<br>
Location: `frontend/src/queries/analytics.ts:29-55`; `internal/application/gain_service.go:53-100`

The frontend starts one `AnalyticsService.AccountGain(accountId)` query per account. Each backend call loads and evaluates a full account snapshot, so a household with many accounts repeats substantially the same database and valuation work.

Recommended fix:

- Add a batch account-gain endpoint that loads the source snapshot once and groups results by account.
- Keep the frontend query keyed by the complete analytics input and invalidate it with the existing activity/quote mutations.
- Add a performance-oriented test or benchmark that records query count and valuation work as account count grows.

## Domain and Financial Model

### Strengths

- `Money`, `Quantity`, `UnitPrice`, `NativeAmount`, and `FxRate` have distinct types and bounded decimal parsing. This is a good foundation for avoiding accidental float arithmetic.
- The activity model is append-oriented and supports reversals/fixes through explicit effects. The application service coordinates multi-entity mutations through transactions.
- Live valuation preserves exact intermediate amounts and treats missing market data as incomplete rather than silently inventing a value.
- Account inclusion flags and holding/cash distinctions are represented explicitly, and ownership shares use integer basis points.
- The current scope is intentionally a personal-finance ledger rather than a full brokerage engine. Lack of FIFO lots, short positions, margin, corporate actions, and tax-lot selection is acceptable if it remains explicit in product documentation.

### Medium findings

#### Database-level ownership completeness is weaker than the application contract

Severity: Medium<br>
Location: `internal/infrastructure/sqlite/schema.sql:99-107`; startup checks in `internal/infrastructure/sqlite/schema_verify.go:162-224`

SQLite checks each ownership row’s range but does not enforce that a household’s ownership rows sum to 10,000 basis points. The aggregate invariant is checked at startup and in application code, not at the database write boundary. A future import, maintenance command, or overlooked repository path could persist an incomplete split.

Recommended fix: add a transaction-level repository invariant, trigger, or a single ownership replacement operation that validates the aggregate before commit. Keep startup verification as defense in depth.

#### Numeric TEXT columns do not enforce canonical decimal syntax in SQLite

Severity: Medium<br>
Location: `internal/infrastructure/sqlite/schema.sql:108-197,416-458`

Financial numeric values are intentionally stored as text, but most columns only require non-null text rather than the canonical decimal grammar enforced by the Go domain. Startup verification catches some invalid states, but invalid data can exist between writes and reads, and different SQL tooling can insert values the domain would reject.

Recommended fix: centralize all numeric writes through repositories and add schema-level checks where practical, or clearly document that the database is not independently writable and strengthen `schema_verify` to validate every numeric field with exact decimal parsing.

#### Rounding modes are not presented as a single accounting policy

Severity: Medium<br>
Location: `internal/domain/cost_basis.go`; decimal helpers and trade calculations in `internal/domain`

The cost-basis path uses banker’s rounding while other calculations use a mixture of `Round`, bounded parsing, and money conversion. The differences may be intentional, but they are not expressed as a single policy document or shared set of named helpers. This increases the chance that future features will produce penny-level inconsistencies.

Recommended fix: document rounding by field and operation, expose named calculation helpers, and add half-way-value regression cases.

#### Persisted inclusion flags need a clear consumer contract

Severity: Medium<br>
Location: account model, `internal/application/valuation.go`, and `docs/architecture/account-container-and-position-model-design.md`

`include_in_portfolio` is persisted and replayed but is currently not used by live portfolio aggregation; the architecture document describes this as an intentional current rule. `include_in_liquid_assets` likewise needs an explicit metric consumer or a documented “reserved for future metrics” status. This is not necessarily a bug, but exposed settings that do not affect a named output create user and testing ambiguity.

Recommended fix: either wire each flag into a named metric, remove it from the active UI until supported, or document the current semantics in the IPC contract and acceptance tests.

## Architecture

The layer direction is sound: domain code does not depend on Wails or SQLite, application services own cross-aggregate orchestration, and Wails adapters translate DTOs/errors at the boundary. The repository interfaces are composed from focused ports even though the concrete service is broad. The local-first choice, one SQLite database connection, WAL mode, and immutable activity history are reasonable for this product.

### Medium architectural findings

#### `application.Service` is becoming a broad façade

Severity: Medium<br>
Location: `internal/application/service.go` and its split service files; `internal/application/repository.go`

The repository ports are relatively focused, but the concrete `Service` owns onboarding, directory administration, accounts, activities, analytics, snapshots, CSV, refresh, and backup coordination. This makes dependency and lock coverage difficult to reason about and encourages Wails adapters to depend on a large object.

Recommended fix: keep the current ports, but compose explicit use-case services behind them. Start with high-risk boundaries such as historical analytics, backup/restore, and directory administration rather than introducing a generic framework.

#### `Bootstrap` is not a pure read/startup operation

Severity: Medium<br>
Location: `internal/application/service.go:145-176,217-298`

`Bootstrap` calls `ensureDefaultDirectory`, which can create institutions or groups while a read path is initializing. Onboarding also completes its main transaction and default-directory setup in separate transactions. This makes startup/read behavior stateful and complicates the backup quiesce model.

Recommended fix: make bootstrap read-only after initialization, or move idempotent default-directory creation into one explicitly gated initialization transaction. Do not let ordinary read APIs mutate the user’s directory implicitly.

#### CSV and Wails adapters know more infrastructure than necessary

Severity: Medium<br>
Location: `internal/application/csv.go`, `internal/application/csv_plan.go`, `internal/wailsapi/data/data.go`, `internal/wailsapi/recovery/recovery.go`

Application CSV code imports the infrastructure CSV codec, while Wails data/recovery adapters directly coordinate `*sqlite.DB`, filesystem paths, settings, archive extraction, and process lifecycle. These choices are workable, but they blur the stated boundary and make non-GUI testing harder.

Recommended fix: expose application-level ports for CSV encoding, backup packaging, and restore orchestration. Keep Wails responsible for transport and user interaction, not persistence lifecycle.

#### Recovery pending state has no expiry or size policy of its own

Severity: Medium<br>
Location: `internal/wailsapi/recovery/recovery.go:75-141`

Inspected backup packages are retained in an in-memory pending map until confirmation or a not-found path. The archive parser has useful member and total-size limits, but an abandoned inspection can retain staged data and metadata indefinitely during the process lifetime.

Recommended fix: add a TTL and bounded pending-entry policy, clean staged directories on expiry, and make confirmation tokens single-use with explicit lifecycle metrics.

## Persistence and SQLite

### Strengths

- The schema has an explicit version, startup integrity checks, foreign-key checks, and domain-specific invariants.
- Writable open configures `foreign_keys=ON`, a busy timeout, `txlock=immediate`, one connection, and WAL mode. Parent directories and backup artifacts use restrictive permissions.
- Backup containers validate exact members, reject unsafe paths/symlinks, cap member and total sizes, verify checksums, and treat the database file group as `.db`, `-wal`, and `-shm`.
- Restore uses a journal/reconciliation flow with rollback/quarantine behavior, which is materially safer than replacing a live database directly.

### Medium findings

#### Snapshot provenance is not fully referential and exact base values are not retained

Severity: Medium<br>
Location: `internal/infrastructure/sqlite/schema.sql:416-458`; `internal/infrastructure/sqlite/snapshot_repository.go:28-68`

Snapshot item provenance columns such as quote, FX, state, and preference observation IDs are mostly plain text; only one of the relevant relationships has an inline foreign key. Snapshot items retain high-precision native values but persist only a rounded money-facing base amount. This weakens both auditability and reproducibility when source observations are removed or the calculation path changes.

Recommended fix: make provenance immutable and referential, or store a self-contained observation record with the snapshot. Store an exact bounded base amount in addition to the display `Money` value.

#### Schema verification casts exact quantities through `REAL`

Severity: Medium<br>
Location: `internal/infrastructure/sqlite/schema_verify.go:113-160`

The cost-basis invariant uses `CAST(quantity AS REAL) > 0` in SQL. This is inconsistent with the domain’s exact decimal contract and can lose precision for large or high-scale quantities. Verification should not introduce floating-point semantics into the integrity gate.

Recommended fix: compare canonical decimal strings with a safe integer/scale strategy, or load the bounded values and validate them with the domain parser before applying the sign/zero rule.

#### Malformed optional directory IDs are silently dropped during reads

Severity: Medium<br>
Location: account-record scanning and institution/group ID parsing in `internal/infrastructure/sqlite/repository.go`

Some optional institution/group identifier parsing paths convert malformed text into a missing ID rather than returning a read error. This can make corrupted relationships appear as ordinary uncategorized data and hides the difference between “not set” and “invalid.”

Recommended fix: return a typed corruption/read error for non-empty malformed IDs. Reserve `nil` only for SQL `NULL` or the explicitly supported empty representation, and add a fixture test.

#### Live database file mode is not explicitly normalized

Severity: Medium<br>
Location: `internal/infrastructure/sqlite/database.go:71-140`

The parent directory is created with mode `0700`, and backup database files are explicitly `0600`, but writable live database creation does not explicitly normalize the existing database file mode. On a reused file or a permissive umask, the directory may protect the file while the file itself has broader permissions than intended.

Recommended fix: after opening/creating the live database, explicitly enforce the documented file mode and test it on a pre-existing database. Avoid changing permissions on a path that is not verified to be the application database.

#### List APIs perform avoidable per-row hydration

Severity: Medium<br>
Location: `internal/infrastructure/sqlite/activity_repository.go:176-209,211-321`; snapshot listing in `internal/infrastructure/sqlite/snapshot_repository.go:123-195`

Activity list methods load the page and then fetch effects/details for each activity. Snapshot list methods likewise load items per snapshot. This is straightforward and likely acceptable for a small personal dataset, but it scales with page length and date range and makes the row-lifetime/concurrency surface larger.

Recommended fix: batch-load child rows by parent IDs or use carefully bounded joins, while retaining explicit ordering and avoiding duplicate parent rows.

## Wails Integration

The Wails boundary is explicit and generally disciplined. DTOs use strings for financial values, unexpected errors are converted to a safe wire shape, and the backend derives household/timezone context rather than trusting frontend identity fields. The backup and restore paths also use explicit confirmation and process restart semantics.

### Findings

- High findings H11 and H12 apply directly to startup recovery and backup consistency.
- `cmd/nestworth/main.go:121-165` uses `os.Exit(1)` if `app.Run()` fails. That can bypass the deferred database close and any future cleanup added there. Prefer returning an error to `main` after cleanup, or install an explicit shutdown path.
- `internal/wailsapi/data` and `recovery` own filesystem/database lifecycle details that would be easier to test outside Wails if moved behind application ports.
- The project is pinned to Wails v3 beta APIs in Go and frontend dependencies. This is a material upgrade and packaging risk even though the current automated tests pass; keep the beta version pinned and schedule a deliberate upgrade validation rather than upgrading transitively.

Native Wails behavior remains unverified in this review: startup error screens, window lifecycle, backup/restore dialogs, process restart, file chooser behavior, and packaged-app permissions need a separate native smoke test.

## Backend Services and API Contracts

The application service has good transaction-oriented mutation flows and exposes stable safe error categories at the Wails edge. It also uses a clock/provider abstraction, which is valuable for deterministic history tests.

The main backend concerns are the high findings above plus the following medium items:

- The overview path fans out across accounts, instruments, holdings, origin, activities, and settings. `frontend/src/features/overview/OverviewPage.tsx:77-115` demonstrates the consumer side. A single read model or batch DTO would reduce startup latency and make loading/error states less fragmented.
- History pagination exists in the backend query shape, but `frontend/src/features/history/HistoryPage.tsx:42-64` requests a fixed 50-item page and `frontend/src/queries/history.ts:49-61` does not advance the returned cursor. Older activities become inaccessible from the UI once the first page is full.
- Mutation endpoints are serialized by locks but do not appear to have a user-supplied idempotency key. A retry after a lost response can therefore create a duplicate activity. This is a medium reliability risk for imports, trades, and transfers; add request IDs if the UI or future automation will retry writes.

## Frontend

### Strengths

- React Query owns server state and mutation invalidation, while Zod validates form input before transport.
- The frontend generally displays backend-derived values instead of reimplementing ledger or valuation rules.
- Money formatting preserves canonical decimal strings; chart conversion to JavaScript `Number` is isolated to ECharts presentation/layout code, and chart tooltip labels are escaped.
- Loading, error, empty, and partial/incomplete valuation states are represented in the feature pages.

### Findings

#### Overview and analytics fan out too aggressively

Severity: Medium in addition to H14<br>
Location: `frontend/src/features/overview/OverviewPage.tsx:77-115`; `frontend/src/queries/analytics.ts:29-55`

The overview starts several independent requests, while account gains create one request per account. This increases IPC round trips and can produce inconsistent screen slices if one request observes a different activity/quote state from another.

Recommended fix: add read-oriented aggregate DTOs for overview and analytics, with one consistent source snapshot per screen. Keep the existing smaller endpoints for detail pages.

#### Visited pages remain mounted and hidden

Severity: Low to Medium<br>
Location: `frontend/src/App.tsx:30-31,91-150`

Previously visited pages remain mounted inside hidden containers. This preserves local state, but their queries and effects can remain active and refresh in the background. As more analytics pages are added, memory and IPC traffic can grow even when the user is looking at one page.

Recommended fix: explicitly pause polling/background work for hidden pages, or unmount pages whose state can be recovered from query/cache state. Add a navigation performance check before changing the behavior.

#### One ownership conversion still relies on JavaScript `Number`

Severity: Low<br>
Location: `frontend/src/features/accounts/accountCatalog.ts:123-146`

Custom ownership percentages are converted through `Number` before basis-point rounding. Current inputs are ordinary percentages and the backend validates the final ownership shares, so this is not an observed loss for normal values. It does, however, make the frontend a participant in a financial precision boundary.

Recommended fix: parse decimal percentages as strings and convert to basis points with the existing exact helpers, leaving `Number` only in presentation code.

## Reliability and Security

### Positive controls observed

- Backup ZIP member validation rejects traversal, absolute paths, unexpected members, directories, and symlinks; member and total-size limits are enforced.
- Restore staging uses restrictive modes, syncs before rename, and has journal reconciliation/rollback paths.
- SQL access is parameterized, SQLite foreign keys are enabled for normal writes, and the read-only verifier uses `mode=ro`/`query_only`.
- Wails error serialization avoids sending raw unexpected errors to the frontend.
- Provider HTTP requests have timeouts, body limits, redirect refusal, and concurrency limits.

### Medium / Low concerns

- `cmd/nestworth/main.go` logs the database path during startup. `internal/infrastructure/marketdata/httpclient.go` also logs wrapped provider errors at debug level. These are local logs, but paths and transport errors can contain user-environment or provider details. Redact or classify log fields under the privacy contract in `docs/architecture/system-overview.md`.
- Explicitly enforce live database file permissions as described in the persistence section.
- Pending restore/inspection state needs TTL and cleanup as described in the architecture section.
- No critical remote attack surface was found in static review. Native webview configuration, packaged permissions, local filesystem ACL behavior, and OS-level quarantine/signing were not tested and should not be inferred from Go/frontend test results.

## Performance and Scalability

For a personal dataset with a modest number of accounts and activities, the current design is likely adequate. The main scale boundaries are predictable:

1. Account-gain N+1 work repeats full snapshot reads and valuation.
2. Overview and history use multiple IPC calls and per-row hydration.
3. Closed-day trend reads can lazily create snapshots and replay historical data as part of an analytics read. This is a reasonable lazy strategy, but it can make a read unexpectedly become a write and can block on larger date ranges.
4. Hidden visited pages retain active query/effect lifecycles.
5. Snapshot and activity child loading grows with page length/date range.

Recommended performance order: batch account gains and overview data first, measure snapshot rebuild/replay cost with a generated multi-year fixture, then optimize child hydration and hidden-page lifecycle based on measurements. Do not trade away exact arithmetic or snapshot reproducibility for a micro-optimization.

## Testing and Engineering Practices

The automated baseline is currently healthy: the full Go suite, race detector, vet, build, formatting check, and the full frontend test/typecheck/lint set passed in this review. That is strong evidence for regression safety in covered paths, not proof that all financial invariants are covered.

### Missing or insufficient regression coverage

Add focused tests for:

- historical `IncludeInNetWorth` changes and excluded accounts;
- negative net worth through snapshot persistence and trend reload;
- exact historical aggregation versus live valuation;
- position-transfer quantity overflow and unchanged-database-on-error behavior;
- high-precision average-cost blending and explicit buy-fee policy;
- archived instruments in all gain/dividend services;
- both accepted v9 schema forms through read-only backup inspection and restore;
- repair failure/interruption and post-repair foreign-key verification;
- row-scan error cleanup followed by a second query;
- concurrent backup with refresh/default-directory writes;
- history cursor advancement beyond the first page;
- batch analytics query count and multi-account consistency;
- backup/restore confirmation expiry and duplicate mutation retry behavior.

The repository contains no GitHub Actions workflow under `.github`; only the pull-request template is present. If CI is expected, add a workflow that runs the same Go/frontend checks with a writable cache location and a separate optional native/package job. Keep native GUI smoke and signing gates visibly separate from unit/typecheck gates.

## Technical Debt and Product Boundaries

The following are not immediate defects but should remain explicit:

- The current portfolio semantics intentionally use active asset holdings and omit cash from portfolio totals; `include_in_portfolio` is not yet a live override.
- Advanced brokerage accounting—lots, shorting, margin, corporate actions, tax treatment, and multi-leg settlement—is outside the present model.
- Wails v3 beta dependencies and the absence of a native packaging gate increase release risk.
- The broad application façade, infrastructure imports from application code, and Wails-owned persistence lifecycle make future modularization harder.
- Documentation should keep the current schema, activity taxonomy, inclusion semantics, and provider-history availability aligned with code and tests.

## Recommended Roadmap

### Before relying on historical analytics

1. Fix H1–H4: shared eligibility, signed net worth, exact historical aggregation, and transfer overflow propagation.
2. Resolve H5–H7 as an explicit accounting policy: precision, buy fees, and archived instrument participation.
3. Add the end-to-end regression fixtures listed above, especially save → reload → trend assertions.

### Before calling backup/restore production-safe

1. Resolve H8–H12: repair verification, read-only v9 compatibility, row cleanup, startup error categories, and a global backup quiesce protocol.
2. Exercise journal rollback and restore confirmation in a native/package smoke test.
3. Verify live file permissions and redact operational logs.

### Next scalability pass

1. Batch account gains and overview data.
2. Implement history cursor consumption and batch child hydration.
3. Measure lazy snapshot rebuilds and hidden-page background work with realistic fixtures.
4. Establish CI and a pinned Wails package validation job.

## Final Assessment

The repository is well structured and has a credible automated engineering baseline. The highest-value work is concentrated rather than architectural rewrite: make the historical path obey the same inclusion, sign, precision, archive, and FX rules as live valuation; harden the v9/backup boundary; and replace repeated analytics reads with batch APIs. Until those items are addressed, current net worth is more trustworthy than historical trend and gain outputs, and backup correctness depends on concurrency and schema-shape cases that are not yet covered by tests.
