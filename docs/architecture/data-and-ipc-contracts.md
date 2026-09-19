# Data and Application Contracts

The Wails v3 application has a real IPC boundary: `internal/wailsapi` DTOs
cross into the React frontend. Domain, application, and SQLite contracts below
that boundary remain backend-owned.

## Ownership of contracts

The domain defines business invariants. Application use cases define commands
and query results. The current `0.3.3` line owns one complete SQLite schema
`11`. Schema `9` migrates through `10` to `11`; schema `10` migrates to
`11` on open (offline, no network). Older
database generations, including schemas `6`, `7`, and `8`, are rejected without
migration. UI code
consumes view models and must not reconstruct authoritative financial values.

The repository contains the Go implementation of the Household balance-sheet,
portfolio, Activity, history, snapshot, and trend contracts. Provider refresh
remains explicit and ordinary history/snapshot workflows are local-only.

## SQLite runtime

Business data lives in a local SQLite database under the platform-specific
application data directory. Infrastructure is the only database client.
Writable connections enable foreign keys and a bounded busy timeout, and
startup verifies `foreign_key_check` and `integrity_check`.

The database path, schema version, and integrity failures are surfaced through
safe startup states. The application does not silently delete, replace, or
recreate a user's database after an open or migration failure.

## Migration compatibility state machine

| Condition | Required behavior |
| --- | --- |
| Database absent | Create the current schema, verify it, then initialize settings |
| Supported and current | Open and verify it |
| Supported older generation (`9`, `10`) | Migrate to the current schema in one local transaction, then verify |
| Older generation (`6`–`8` and earlier) | Block startup without writes; tell the user to create a new database |
| Newer than supported | Block business writes with a safe error |
| Integrity failure | Block startup; preserve the original database |
| Path/open failure | Show an unavailable-database state |

Compatibility is rechecked on the writable connection before schema or business writes. Older and unsupported future databases receive zero persistent application writes.

## Persistence responsibilities

The current schema implements Household, Member, Institution, Group,
Account, Ownership, Account Value, Instrument, Holding, Account
Cash Value, Instrument Quote, FX Quote, FX Preference, History Origin, Activity,
snapshot, and dirty-state persistence. Local backup/restore and CSV import/export
are application-owned sidecar and file workflows; they do not add business schema
tables.

Manual Instrument, Holding, cash, quote, preference, archive, and
foreign-currency Account commands are application-owned and have no provider
or network dependency.

| Concept | Responsibility |
| --- | --- |
| Household | Singleton balance-sheet root and base currency |
| Member | Household people used for ownership allocation |
| Institution and Group | Optional account organization |
| Account | Type, balance-sheet role, tracking mode, currency, lifecycle, and inclusion |
| Ownership | Exact member shares in basis points |
| Account Value | Append-only balance/manual-value observations |
| Instrument and Holding | Investment identity and quantity |
| Account Cash | Append-only cash-by-currency observations |
| Instrument/FX Quote | Append-only price and FX observations |
| History Origin | Cutover boundary for trustworthy reconstructed history |
| Activity and Activity Leg | Immutable explanations for post-origin changes |
| Activity mutation key | Client-generated UUID plus payload hash making Record/Fix retries idempotent |
| Daily Snapshot | Append-only closed-day valuation revision |
| Average Cost Evidence | Starting Point and cost-bearing Activity inputs replayed into derived cost/gain views |
| Gain Read Models | Derived Holding, Account, and realized-period results with explicit unavailable state |
| Application Settings | Singleton presentation preferences and selected FX provider |

Physical table names and indexes are defined by the current `schema.sql` and
documented here without duplicating SQL. The current supported schema is `10`.
Schema `9` is the only older generation that migrates; future and older schema
generations are blocked before business or settings writes.

## Transaction guarantees

- A multi-row mutation begins one transaction and commits once.
- Validation, lookup, persistence, and view-model assembly errors roll back the
  complete mutation.
- Reads combining multiple collections use one consistent read snapshot.
- List queries are bounded and deterministic; no query-per-result-row loops.
- Unknown targets return a stable not-found error and write nothing.
- Append-only observations never overwrite prior financial evidence.
- Posted Activities are immutable; reversal and correction append linked records.
- Record, Commit, and Fix Activity commands accept an optional client-generated
  `mutationId`. The same ID with the same payload returns the original result;
  the same ID with a different payload returns `conflict`. Empty IDs remain
  valid for tests and older callers. The key is stored in
  `activity_mutation_keys` without bumping schema version.

Schema `10` adds market-data observation revisions, canonical observation
slots, provider-binding revisions, coverage (`market_data_day_status`), and
dirty `input_generation` / `resolver_policy_version` columns. Historical
instrument and FX batches persist in one transaction with invalidation; fail-
closed Tiingo mappings (invalid, unsupported, adjClose-only, malformed) write
nothing. Existing latest and manual quotes remain `realtime`/`latest` and
`manual`; migrated unverifiable provider quotes are `legacy` and do not fill
close slots.

## Tiingo API key configuration

Tiingo API keys are persisted in the local settings JSON file, which is
created with private permissions and written atomically. The key is not part
of SQLite business data and never crosses the Wails boundary. The settings
service returns only a derived `configured` boolean and the provider reads
the current key from the settings store when it makes a request.

## Current valuation and provider refresh

`ValuationService` is the sole authority for current Account and Portfolio
values. It consumes one `PortfolioSnapshot` of persisted Accounts, Holdings,
cash, Instruments, quote preferences, and quote observations. It never opens a
network connection. Missing selected quotes exclude only the affected
component, preserve the remaining subtotal, and mark the parent incomplete.
Exact decimal precision is retained until the application Money boundary.

`MarketDataRegistry` is the application provider port. Production registers
Yahoo Finance for Instrument quotes and Frankfurter as the only production FX
provider. Settings persists the FX provider choice for compatibility; explicit
FX refresh resolves Frankfurter, while Instrument refresh resolves each Instrument's saved provider key and symbol. Manual and
passive read paths make zero provider calls. Refresh results expose only stable
target/status/error-code values. Cancellation, generation checks, retry
state, and completion belong to `MarketDataService` events plus the
frontend. The frontend-facing refresh contract is `StartRefreshAll`,
`StartRefreshInstrument`, `StartRefreshFX`, `StartRefreshMissingOrStale`,
`StartRefreshRequiredFX`, and `CancelRefresh`, with the request ID carried by
`marketdata.refresh.completed`. The synchronous `Refresh*` methods remain
backend compatibility helpers and are not a frontend path. An event for an
abandoned request ID is ignored.

## Serialization and view models

Application results crossing the Wails boundary use explicit DTOs rather than
passing database rows or driver-specific errors. Recommended rules:

- IDs are typed in Go and serialized as lowercase hyphenated UUID strings.
- Timestamps are UTC RFC 3339 strings with millisecond precision.
- Currency codes are three uppercase ASCII letters.
- Money, Quantity, FX, and return values are canonical decimal strings.
- Optional values are explicit pointers or nullable result fields.
- Errors contain a stable code, safe message, and optional field context.
- Raw SQL, credentials, provider payloads, local paths, and sensitive values do
  not enter user-facing view models.

The UI formats strings for display but never calculates totals, reciprocal FX
rates, ownership percentages, gain, or return.

### Generated bindings are the frontend contract

The TypeScript files under `frontend/bindings/` are generated from the
exported Wails services and are the only frontend-facing API contract. Every
frontend service call must use the generated method with its generated
signature. Optional casts, method-existence checks, silent no-ops, and
fallbacks to a different method hide backend/binding drift and can change
business semantics. Missing or stale bindings must fail generation, typecheck,
or CI rather than being discovered at runtime.

`pnpm run generate:bindings` regenerates the bindings before frontend
development, builds, and tests. CI also runs `pnpm run check:bindings`, which
generates into a temporary directory and compares the result with the working
bindings. Persistence structs are not Wails DTOs: settings keep snake_case
on disk while `SettingsDTO` exposes camelCase IPC fields. Persistence
compatibility belongs in a migration, not in a frontend dual-read.

Daily snapshot and trend data carries an explicit `status`: `complete`,
`incomplete`, or `missing`. Incomplete and missing points have nullable
valuation fields and are rendered as chart gaps. A partial or absent
valuation is never represented as zero.

### Canonical mutations and analytics scope

Quote observations and quote-source preferences are separate facts. The
canonical manual quote commands append an observation only;
`SetInstrumentQuoteSource` or `SetFXPreference` is the explicit preference
mutation. The old `Save*Quote` names are deprecated aliases.

Market-data refresh is an asynchronous operation: each `StartRefresh*` returns
an operation ID, and `marketdata.refresh.completed` reports the matching
`requestId` with `completed`, `failed`, or `cancelled` status. `CancelRefresh`
is idempotent; the frontend detaches listeners on completion, cancellation, or
unmount.

After history starts, financial mutations use `PreviewChange` followed by
`RecordChange`. `CreateHolding` is for creating an initial position,
`UpdateHolding` is metadata-only, and `UpdateHoldingQuantity` is a deprecated
pre-history compatibility path. `AppendAccountValue` is for bootstrap/import
baseline data; a user value correction is a `value_update` history change.
`HistoryMutationAllowed` is a frontend preflight only; the backend enforces
the same rule again in the record/mutation command.

Analysis queries use `domain.AnalysisQuery` (typed scope, local-date range,
valuation, basis, `includeCash`, and filters) through
[`internal/wailsapi/analysis`](../../internal/wailsapi/analysis/analysis.go).
Named ranges such as `30d` and `ytd` are UI presets that clamp to History
Origin; they are not a second query language. Range readers are the canonical
path for custom dates.

## Backup, restore, and CSV IPC

`RecoveryService` is always bound, including when the business database could
not be opened. `DataService` is bound only when a live database session exists.

Backup writes a `.nestworth-backup` ZIP (`manifest.json`, `database.sqlite`,
`settings.json`) using `VACUUM INTO`, then stores a filename-only summary in
`.nestworth-backup-status.json` next to the live database. Restore verifies the
package, replaces the live SQLite file group through a journaled swap, quits,
and finishes verification on the next launch. CSV import/export is create-only
for Accounts and Holdings, with preview and an all-or-nothing commit. CSV is
not a backup substitute.

## Account persistence and recovery details

The Account row persists `account_type`, `balance_sheet_role`,
`tracking_mode`, default currency, lifecycle state, ownership references, and
compatibility inclusion fields. The schema CHECK expresses the legal triple,
not merely independent enum membership. `balance_sheet_role` and
`tracking_mode` are immutable after creation; an `account_type` update is
accepted only when the existing role and tracking mode remain a legal schema-9
combination. Holdings and Account Cash rows belong only to Holdings Accounts;
Account Value rows belong only to Simple Accounts. There is no `SubAccount`
table or compatibility alias for the removed category fields.

Schema verification runs integrity, foreign-key, ownership, combination, and
component-shape checks before business reads or writes. A rejected older,
future, or structurally invalid database receives no persistent application
writes. The only supported schema-9 adjustment is a lossless widening of the
`cash_on_hand` combination CHECK when the existing database is otherwise schema
9 and valid.

Recovery and portability are file workflows rather than business tables. A
backup has the fixed members `manifest.json`, `database.sqlite`, and
`settings.json`; verification is read-only and checks format, limits,
checksums, schema, foreign keys, and integrity before replacement. Restore uses
a journaled file-group swap and rolls back an incomplete replacement. The
Tiingo key follows the local settings JSON backup/restore policy and is not
stored in SQLite. CSV
Accounts and Holdings imports are create-only: mapping and preview happen
before one atomic commit, and observation dates remain explicit input rather
than silently using the current time. The detailed user flow and stable error
surface live in [Backup, restore, and CSV portability](../design/backup-restore-and-csv-portability.md).

## Error contract

Application errors should be grouped into stable categories such as:

- validation, not found, conflict, and already-onboarded
- invalid money, quantity, FX, ownership, or account combination
- unavailable or incomplete valuation
- unsupported database, migration failure, and integrity failure
- invalid Activity, insufficient balance/quantity, or correction conflict
- unavailable provider, rate limit, malformed provider response
- backup format, checksum, schema, restore confirmation, and restore swap
- CSV encoding, mapping, row, duplicate, limit, and commit failures
- internal error

Detailed database/driver errors stay in local diagnostics. They must not be
shown by default in the UI.

## Compatibility evidence

The current schema must have sanitized fixtures and tests for create, reopen,
integrity, representative business rows, and unsupported older/future versions.
Compatibility is claimed only where the current repository has an explicit
fixture, verification path, or migration test.

## Local diagnostics and quote history metadata

`SettingsDTO.logLevel` is optional for compatibility; absent/empty and `off`
mean file logging is disabled. Accepted enabled levels are `error`, `warn`,
`info`, and `debug`. `logFilePath` is a read-only path supplied on Load/Reset;
Save ignores a client-supplied path. Preferences persist as `log_level` in the
settings file. The desktop applies level changes immediately and rolls back the
runtime level if saving fails. Logs use JSON lines at
`<settings directory>/logs/nestworth.log`, with a 5 MiB current file and two
rotated files, private file permissions, and no provider credentials or request
bodies in diagnostic events. Existing settings without this field stay valid.
These are application logs, not a capture of browser console output.

`QuoteSeriesPointDTO.observationKind` and `effectiveDate` preserve stored quote
metadata in both chart points and the complete observations list. Instrument
kinds are `manual`, `realtime`, `close`, and `legacy`; FX kinds also include
`latest` and `daily_reference`. An absent/legacy kind must not be inferred to be
a live quote from its timestamp or delayed flag. Market closure is not a new
observation kind: carry-forward belongs to historical valuation, not a fabricated
quote-history row.

Asset Changes displays the existing authoritative waterfall driver amounts as
zero-centered contribution rows with exact amounts. Beginning/ending values
remain in the summary; reconciliation checks remain active. Analysis range
shortcuts end at the last closed day in the history-origin timezone and clamp
the beginning to the history origin. Month shortcuts use calendar months and
inclusive date endpoints.

## Metals and crypto in 0.3.3

Schema 11 adds optional metal-template and quantity-unit metadata. Existing
instruments retain empty values and are not automatically rebound. Gold/silver
use Yahoo `GC=F`/`SI=F` USD-per-troy-ounce futures references. Go converts to
the instrument currency and divides by `31.1034768` for grams, rounding only
at the existing unit-price boundary. Missing FX or prices remain unavailable.
Historical conversion uses eligible historical FX, never today's rate.
CSV preserves template/unit identity; manual prices already use the target unit.

CoinGecko uses exact coin IDs separately from display tickers. Its local Demo
key is not returned by settings DTOs. Daily points retain their UTC reference
timestamps; the adapter enforces its 365-day Demo-history boundary and leaves
older required gaps visible. Existing stored history and provider bindings remain intact.

History starts at the first effective holding date, bounded by History Origin;
never-held instruments fall back to creation date. Planning includes a seven-day
lead-in and bounded opening-anchor fallback. Normal repair reuses fresh checks;
force recheck remains explicit. Repair estimates may differ from actual requests
because of batching, caches, retries, and conversion dependencies.

Backup restore validates the current schema. Older-schema backup archives do
not directly pass schema-11 restore validation, although supported database files
upgrade through the normal open path. Keep originals before upgrade and create
a fresh backup afterwards.
