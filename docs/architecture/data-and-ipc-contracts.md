# Data and Application Contracts

This filename is retained for compatibility with the source documentation. The
Go + Fyne application has no Tauri IPC boundary. Its equivalent public
boundary is the typed contract between Fyne views, application use cases,
domain results, and infrastructure ports.

## Ownership of contracts

The domain defines business invariants. Application use cases define commands
and query results. The v0.1.1 SQLite migration remains the compatibility base;
the implemented v0.1.2 Phase 2 migration extends it to schema `3`, and v0.1.3
extends that boundary through schema `4` with history persistence. UI code
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
| Database absent | Create it, run migrations, verify it, then initialize settings |
| Supported and current | Open and verify it |
| Supported and older | Create a recoverable sibling snapshot before migration |
| Newer than supported | Block business writes with a safe error |
| Migration failure | Block startup; preserve the original database |
| Integrity failure | Block startup; preserve the original database |
| Path/open failure | Show an unavailable-database state |

Compatibility is rechecked on the writable connection before migration or schema writes. An unsupported future database receives zero persistent application writes.

## Persistence responsibilities

The ordered migrations implement Household, Member, Institution, Group,
Account, Ownership, Account Value, Media Asset, Instrument, Holding, Account
Cash Value, Instrument Quote, FX Quote, FX Preference, History Origin, Activity,
snapshot, and dirty-state persistence. Recovery remains a future extension.

Manual Instrument, Holding, cash, quote, preference, archive, and
foreign-currency Account commands are application-owned and have no provider
or network dependency.

| Concept | Responsibility |
| --- | --- |
| Household | Singleton balance-sheet root and base currency |
| Member | Household people used for ownership allocation |
| Institution and Group | Optional account organization |
| Account | Classification, tracking mode, currency, lifecycle, and inclusion |
| Ownership | Exact member shares in basis points |
| Account Value | Append-only balance/manual-value observations |
| Instrument and Holding | Investment identity and quantity |
| Account Cash | Append-only cash-by-currency observations |
| Instrument/FX Quote | Append-only price and FX observations |
| History Origin | Cutover boundary for trustworthy reconstructed history |
| Activity and Activity Leg | Immutable explanations for post-origin changes |
| Daily Snapshot | Append-only closed-day valuation revision |
| Cost Basis Declaration | Append-only user-supplied basis for unknown lots |
| Media Asset | Household-scoped normalized image bytes |
| Application Settings | Singleton presentation preferences and selected FX provider |

Physical table names and indexes are defined by the ordered migrations and
documented here without duplicating migration SQL. The current supported
schema is `4`; schema `2` receives the v0.1.2 portfolio migration, schema `3`
receives the v0.1.3 history migration, and future schema versions are blocked
before business or settings writes.

## Transaction guarantees

- A multi-row mutation begins one transaction and commits once.
- Validation, lookup, persistence, and view-model assembly errors roll back the
  complete mutation.
- Reads combining multiple collections use one consistent read snapshot.
- List queries are bounded and deterministic; no query-per-result-row loops.
- Unknown targets return a stable not-found error and write nothing.
- Append-only observations never overwrite prior financial evidence.
- Posted Activities are immutable; reversal and correction append linked records.

## Current valuation and provider refresh

`ValuationService` is the sole authority for current Account and Portfolio
values. It consumes one `PortfolioSnapshot` of persisted Accounts, Holdings,
cash, Instruments, quote preferences, and quote observations. It never opens a
network connection. Missing selected quotes exclude only the affected
component, preserve the remaining subtotal, and mark the parent incomplete.
Exact decimal precision is retained until the application Money boundary.

`MarketDataRegistry` is the application provider port. Production registers
Yahoo Finance for Instrument and FX current quotes and Frankfurter for FX-only
daily rates. Settings persists the FX provider choice, defaulting to Yahoo for
legacy settings; explicit FX refresh resolves that choice, while Instrument
refresh resolves each Instrument's saved provider key and symbol. Manual and
passive read paths make zero provider calls. Refresh results expose only stable
target/status/error-code values, and the Fyne worker owns cancellation,
generation checks, retry state, and UI-thread completion.

## Serialization and view models

Application results crossing into Fyne should use explicit structs rather than
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

## Error contract

Application errors should be grouped into stable categories such as:

- validation, not found, conflict, and already-onboarded
- invalid money, quantity, FX, ownership, or category
- unavailable or incomplete valuation
- unsupported database, migration failure, and integrity failure
- invalid Activity, insufficient balance/quantity, or correction conflict
- unavailable provider, rate limit, malformed provider response
- invalid media and internal error

Detailed database/driver errors stay in local diagnostics. They must not be
shown by default in the Fyne UI.

## Media contract

The implemented media contract:

- Accept PNG, JPEG, and WebP within a bounded input size.
- Decode safely, reject oversized pixel dimensions, resize to a bounded dimension, and normalize to PNG.
- Store only Household-scoped normalized PNG bytes and MIME metadata.
- Return display-safe state to Fyne without exposing arbitrary filesystem paths.
- Replace references atomically while preserving shared assets; clear behavior remains a future extension.
## Compatibility evidence

Every migration must have sanitized fixtures and tests for upgrade, reopen,
integrity, representative business rows, and unsupported future versions.
The first Go implementation must not claim compatibility with the copied
Tauri/Rust database until an explicit importer or migration proves it.
