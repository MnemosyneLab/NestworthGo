# Data and Application Contracts

The Wails v3 application has a real IPC boundary: `internal/wailsapi` DTOs
cross into the React frontend. Domain, application, and SQLite contracts below
that boundary remain backend-owned.

## Ownership of contracts

The domain defines business invariants. Application use cases define commands
and query results. The current `0.3.0` line owns one complete SQLite schema
`9`; older database generations, including schemas `6`, `7`, and `8`, are rejected without
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
| Older generation | Block startup without writes; tell the user to create a new database |
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
documented here without duplicating SQL. The current supported schema is `9`.
Future and older schema generations are blocked before business or settings
writes.

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
  `activity_mutation_keys` without bumping schema version 9.

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
frontend: an event for an abandoned request ID is ignored.

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
