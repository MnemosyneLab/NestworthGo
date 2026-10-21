# Data and Application Contracts

The Wails v3 application has a real IPC boundary: `internal/wailsapi` DTOs
cross into the React frontend. Domain, application, and SQLite contracts below
that boundary remain backend-owned.

## Ownership of contracts

The domain defines business invariants. Application use cases define commands
and query results. Current `main` uses schema `22`; the complete support,
migration and recovery policy is in
[Database support and storage recovery](#database-support-and-storage-recovery).
UI code consumes view models and must not reconstruct authoritative financial
values.

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
recreate a user's database after an open failure.

## Database support and storage recovery

Current `main` creates schema `22` databases and supports verified upgrades
from schemas `15` through `21`. Each migration step verifies its input,
publishes a flushed read-only recovery copy, then applies and verifies the
additive migration transaction before continuing. Schema `15` starts the
supported chain; schemas `14` and older, zero/unversioned, malformed, and
future schemas are blocked without repair or recreation.

| Condition | Required behavior |
| --- | --- |
| Database absent | Create and verify schema 22, then initialize settings |
| Verified schema 22 | Open and verify |
| Verified schema 15–21 | Migrate in order, retaining a verified read-only recovery copy at each step |
| Unsupported older, zero/unversioned, malformed, or future schema | Block before business/settings writes and preserve the source |
| Integrity or path/open failure | Block startup and preserve the source |

Existing files are inspected read-only before writable open and reverified at
each migration step. The historical v15→v16 short-name step is documented in
the [feature migration note](../development/investment-short-names.md); it is
one step in the current chain.

Read-only backup inspection accepts verified schemas 15–22 without upgrading
the inspected source. Local packages require manifest/database schema versions
to match. Restore validates a staged candidate before installation; normal live
open then runs supported migrations. Cloud recovery also verifies the staged
schema against its stream identity and leaves staged bytes and source-stream
metadata unchanged. New backup streams use schema 22. Recovery artifacts stay
read-only and must be copied to a new live path before opening.

## Persistence responsibilities

The current schema implements Household, Member, Institution, Group,
Account, Ownership, Account Value, Instrument, Holding, Account
Cash Value, Instrument Quote, FX Quote, FX Preference, History Origin, Activity,
snapshot, and dirty-state persistence. Local backup/restore and JSON export
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
documented here without duplicating SQL. Database version and recovery rules
are owned by [Database support and storage recovery](#database-support-and-storage-recovery).

## Transaction guarantees

- A multi-row mutation begins one transaction and commits once.
- Validation, lookup, persistence, and view-model assembly errors roll back the
  complete mutation.
- Reads combining multiple collections use one consistent read snapshot.
- List queries are bounded and deterministic; no query-per-result-row loops.
- Unknown targets return a stable not-found error and write nothing.
- Append-only observations never overwrite prior financial evidence.
- Posted Activities are immutable; reversal and correction append linked records.
- Financial reads resolve corrections to the original transaction's time and
  ordering position. Original and correction-reversal records remain in the audit
  history but do not count as additional financial events. This applies to legacy
  corrections and correction chains as well.
- Fix validates the complete later timeline before committing. Invalid later
  balances or quantities reject the fix without writes. Derived event balances
  are rebuilt in the correction transaction. Closed-day snapshots are rebuilt
  in bounded batches, preserving earlier revisions; generation checks prevent
  a concurrent write from marking stale results complete. Snapshot hash v3
  invalidates older replay results. Today remains a live valuation.
- Later absolute balance observations retain their recorded amount; their
  adjustment delta is recalculated from the corrected preceding balance.
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
`manual`; unverifiable provider observations do not fill
close slots.

### Historical coverage, negative checks, and opening anchors

Stored observations and coverage are separate facts. A `close` observation
records a published value; a `no_observation` coverage row records that a
provider successfully verified a date but returned no observation. It does
not mean the market was closed and is never a zero or synthetic quote. Only
provider-verified ranges can create this negative-cache status; pending or
uncertain ranges remain unresolved. Explicit closure facts and inferred
closures are separate: inference is limited to dates between known equity
closes, is not persisted as provider evidence, and does not fill leading
history or the open tail. See [calendar and carry rules](#shared-instrument-valuation-calendar).

Coverage identity includes the target, provider, household, binding revision,
source-policy version, and effective date. A successful no-observation check
expires after 24 hours for dates within the seven calendar days ending at the
latest finalized US-equity date, and after 30 days for older dates. Expiry makes the date eligible for another
fetch; it never turns the missing observation into a value. Force Recheck
bypasses both positive and negative caches. Independently, dates within the three calendar days ending at the latest
finalized date are rechecked after 24 hours so corrected recent provider
history can be discovered. These intervals are based on check time, not an
economic timestamp.

Historical repair also looks for an opening anchor before History Origin. It
searches the preceding 7 calendar days, then extends to 30 days when required.
The plan fetches uncovered dates; known closes, verified no-observation dates,
and applicable closed dates count as checked dates but only an eligible close
can be the anchor. `initial_anchor_missing` is blocking when the complete
30-day lookback is checked and still has no eligible anchor. In that state the
health issue offers manual entry; it is not a claim that a provider outage or
an unverified gap has been resolved. Incomplete lookbacks remain repairable
coverage gaps. The same distinction applies to instrument prices and FX
references.

Sync jobs are scoped to a workspace and refresh epoch. A repeated request for
the same active scope attaches to that job; a different scope conflicts rather
than silently broadening the work. Cancellation waits for the job to stop.
Already committed batches remain durable, while later writes and snapshot
rebuilds are fenced by the workspace/epoch check. Restore and service shutdown
advance the epoch and cancel active work, so a request from the old database
cannot write into its replacement.

| Contract | Code evidence |
| --- | --- |
| Negative-cache policy and recent-correction recheck | [`marketdata_sync_test.go`](../../internal/domain/marketdata_sync_test.go), [`vnext_persist_test.go`](../../internal/infrastructure/marketdata/vnext_persist_test.go) |
| Opening-anchor lookback and exhaustion | [`markethistory_plan_test.go`](../../internal/application/markethistory_plan_test.go), [`marketdata_health_test.go`](../../internal/application/marketdata_health_test.go) |
| Cancellation retains committed batches and isolates later writes | [`marketdata_sync_test.go`](../../internal/application/marketdata_sync_test.go) |

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
strict valuation fields. The additive `displayValuation` fields project the
same exact components: full provisional estimates render with dashed segments
and textual quality; partial known subtotals remain separately labeled in
tables. True unavailable/partial totals leave gaps. A partial or absent
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

## Backup, restore, and JSON export IPC

`RecoveryService` is always bound, including when the business database could
not be opened. `DataService` is bound only when a live database session exists.

Backup writes a `.nestworth-backup` ZIP (`manifest.json`, `database.sqlite`,
`settings.json`) using `VACUUM INTO`, then stores a filename-only summary in
`.nestworth-backup-status.json` next to the live database. Restore verifies the
package, replaces the live SQLite file group through a journaled swap, quits,
and finishes verification on the next launch. JSON export exposes the complete
structured business dataset and a derived current summary through `DataService.ExportJSON()`. It has no import path and is not a
backup substitute. See the [export contract](../design/json-export-format.md).

## Account persistence and recovery details

The Account row persists `account_type`, `balance_sheet_role`,
`tracking_mode`, default currency, lifecycle state, ownership references, and
compatibility inclusion fields. The schema CHECK expresses the legal triple,
not merely independent enum membership. `balance_sheet_role` and
`tracking_mode` are immutable after creation; an `account_type` update is
accepted only when the existing role and tracking mode remain a legal current-schema
combination. Holdings and Account Cash rows belong only to Holdings Accounts;
Account Value rows belong only to Simple Accounts. There is no `SubAccount`
table or compatibility alias for the removed category fields.

Schema verification runs integrity, foreign-key, ownership, combination, and
component-shape checks before business reads or writes. A rejected older,
future, or structurally invalid database receives no persistent application
writes. Missing required tables and old constraints fail verification; startup
does not repair the schema.

Recovery and portability are file workflows rather than business tables. A
backup has the fixed members `manifest.json`, `database.sqlite`, and
`settings.json`; verification is read-only and checks format, limits,
checksums, schema, foreign keys, and integrity before replacement. Restore uses
a journaled file-group swap and rolls back an incomplete replacement. The
Tiingo key follows the local settings JSON backup/restore policy and is not
stored in SQLite. JSON export reads the complete dataset and summary inputs from
one read transaction, excludes settings and credentials, and writes a versioned
file atomically. See [Backup, restore, and JSON export](../design/backup-restore-and-json-export.md).

## Error contract

Application errors should be grouped into stable categories such as:

- validation, not found, conflict, and already-onboarded
- invalid money, quantity, FX, ownership, or account combination
- unavailable or incomplete valuation
- unsupported database, migration failure, and integrity failure
- invalid Activity, insufficient balance/quantity, or correction conflict
- unavailable provider, rate limit, malformed provider response
- backup format, checksum, schema, restore confirmation, and restore swap
- internal error

Detailed database/driver errors stay in local diagnostics. They must not be
shown by default in the UI.

Cancellable holdings, instrument quote-history/performance and historical-overview
reads complete with a zero result and no error when their own Wails request
context is cancelled. Their JavaScript promises have already been abandoned;
no partial data or second error is delivered. Live requests still use the normal
sanitized wire-error contract, and mutation errors are unaffected. The frontend
retains its scoped handling for late beta.28 native errors already in transit.
See [PR91 cancellation evidence](../testing/pr91-merge-and-binding-errors.md).

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

Return Analysis separates the calendar browsing cursor from explicit date
filters. When leaving Calendar for Trend or Contribution, the page makes its
displayed analysis period explicit: an existing selection wins; otherwise the
visible month or year is used, clamped to History Origin and the last closed
ledger-local day. Switching views preserves object, valuation, cash and
additional filters. An empty period on the first local day remains empty and
does not store a reversed date range. Calendar month/year mode stays local to
the mounted Return Analysis page.
The contribution metric is part of the shared analysis state, so a retained
page receiving a new navigation context updates both its selector and query.
Realized contribution filters sale events by native event denomination and
current instrument `Type` metadata. Scope and explicit filters intersect;
contradictions return an available empty result. It groups only by instrument
or account. Currency/class filtering does not add grouping modes, member
filtering remains unsupported, and the household base currency remains the
output currency regardless of requested valuation. History hints identify
related activity and may be broader than a currency/class-filtered contribution.
Clearing additional filters removes only those filters; restoring portfolio
defaults also resets the scope, metric, valuation, cash choice and date range.

History navigation may carry an Activity ID alongside date, account, instrument,
and kind filters. The ID opens that exact Activity through the authoritative
read even when the record is outside the first filtered page or has since been
corrected or reversed; closing the detail returns to the list filters. History
filter choices include archived accounts and instruments for read-only lookup,
while new-record forms retain their separate eligibility rules. The page
reports its current filters to the navigation trail so returning after an
object drill-down restores user edits rather than the original entry filters.
Moving between Market Data and Data Health preserves the focused health issue
and adds a return step, allowing an analysis-to-repair path to be retraced.
The investment-assets workspace opens on owned holdings. Its saved-instrument
view includes unowned and archived records, while allocation and trend are a
secondary view. An `instrument-detail` navigation target carries the instrument
ID, optional account ID, section (`holdings`, `performance`, or `data`), date
range, and optional health focus. With an account ID, holding figures come from
that account's server-provided holding member; without one, they come from the
server-provided instrument total. Market performance and quote records describe
the instrument rather than the owner's return. Product-owned instruments link
to their Available Funds contract instead of displaying a market quote.
Plain legacy Market Data instrument links resolve to this read-only detail;
issue or editor links retain their Market Data repair destination. The single
Data Maintenance navigation entry hosts the existing Market Data and Data
Health routes as quote/FX and issue/repair tasks. Both Page IDs and their
focus contexts remain valid for existing links. Returning from an instrument
record restores the source tab and search state.
Account holdings offer Current, Closed, and All views. The closed view filters
zero-quantity account members without treating them as sell candidates, and it
does not display an unfiltered group subtotal as a closed-position subtotal.
Account and holding action menus also open related History with the current
account, instrument, and explicit date range; App retains the source view and
scroll position for Back.
Opening a new History record from a filtered list suggests only active account
and instrument IDs. Account suggestions also obey the destination field's cash,
value, holdings, investment or debt eligibility. Its effective time remains an
explicit form choice, not an inference from the list's date range. After
recording, the receipt offers a link to the exact new Activity even if it lies
outside that list range.

Reconciliation previews are scoped to the current editor request. Closing an
editor or changing sessions invalidates pending check, adjustment and product
previews, and session selection clears an open review.

## Metals and crypto

Schema 11 adds optional metal-template and quantity-unit metadata. Existing
instruments retain empty values and are not automatically rebound. Gold/silver
use Yahoo `GC=F`/`SI=F` USD-per-troy-ounce futures references. Go converts to
the instrument currency and divides by `31.1034768` for grams, rounding only
at the existing unit-price boundary. Missing FX or prices remain unavailable.
Historical conversion uses eligible historical FX, never today's rate.
JSON export preserves template/unit identity; manual prices already use the target unit.

CoinGecko uses exact coin IDs separately from display tickers. Its local Demo
key is not returned by settings DTOs. Daily points retain their UTC reference
timestamps; the adapter enforces its 365-day Demo-history boundary and leaves
older required gaps visible. Existing stored history and provider bindings remain intact.

History starts at the first effective holding date, bounded by History Origin;
never-held instruments fall back to creation date. Planning includes a seven-day
lead-in and bounded opening-anchor fallback. Normal repair reuses fresh checks;
force recheck remains explicit. Repair estimates may differ from actual requests
because of batching, caches, retries, and conversion dependencies.

Backup inspection validates supported v15–22 databases without modifying the
inspected database. Local restore requires matching manifest and database
versions; supported older copies upgrade on normal open, while the source package
stays unchanged. Other older, unversioned, future, or invalid backup databases are rejected before
replacing the live database. This does not provide a general archive upgrade path.


## Available-funds product contracts

The liquidity Wails service exposes `ListReservations` for the current household,
including active and released rows. Reservation edits/releases carry their ID and
expected revision; unresolved sources can still have their reservations released.
`ProductPolicyInput.accessibleAmountCap` is a nullable native-currency amount for
ordinary sources: null removes the cap, zero is an explicit zero limit. Managed
products reject caps. Product identity (including kind/start date) is immutable;
closed contracts accept name/note edits only and preserve policy revisions.

Open, settle and interest commands accept `effectiveLocalDate` and
`effectiveLocalTime` together as an alternative to `effectiveAt`. The application
resolves them in History Origin's timezone and rejects missing pairs, DST gaps or
ambiguities, dates before Origin, future dates, and invalid product chronology.
Renewal uses one resolved timestamp for redemption and acquisition. Recording an
existing product remains a present-time recording operation.

Product previews include affected cash balances in the contract currency,
`productBefore`, `productAfter`, signed `netWorthDelta`, `netWorthKnown`, resolved
`effectiveAt`, and `reservationReleaseDetails`. A product operation is confined to
one account/currency, including renewal and grouped undo. Product amounts use the
portfolio valuation authority; absent prices remain null. The impact is the net
change in affected cash plus product value, not a second household valuation.
Reviewed-state hashes include related reservation facts and policy revisions as
well as cash, holdings, contracts, quote evidence and local date. Editing these
facts requires a new preview.

Live database open and read-only backup verification share current-schema product
integrity checks: private holding/instrument ownership and currency, lifecycle
quantity 1/0, operation ownership/currency, renewal lineage, policy/reservation
source consistency, and domain term/policy validation. These checks do not repair
corrupt data or change the backup/export format.

### Data Health and incomplete daily snapshots

A completed snapshot build cursor does not imply complete valuation. The local
health scan checks the latest persisted snapshot revisions and re-evaluates
incomplete dates against current local facts, without contacting providers.
For a metal daily reference that has not reached its finalization boundary,
`history_pending` identifies the instrument and affected date;
`HealthIssueDTO.nextCheckAt` is the earliest next check in the History timezone
(RFC3339 with offset), not a guarantee of provider publication. Pending data is
non-executable and prevents an "all healthy" result. Finalized missing data
continues through the normal history-repair workflow.

When current inputs can fully value a previously incomplete snapshot, Data
Health exposes an executable snapshot rebuild even if its dirty marker is
absent. Repair preview and execution both include these recoverable dates;
publication still uses the existing input-generation checks. Coverage-only
updates remain valuation inputs: they invalidate snapshots even when no price
observation changes. Return totals stay partial until the rebuilt snapshot is
complete.

Daily snapshot items keep separate decimal contracts: `NativeAmount` accepts
up to 16 fractional digits (quantity times unit price), while `BaseAmountExact`
accepts up to 28 fractional digits, sufficient for a single snapshot FX
conversion (16 native places plus 12 rate places for multiplication; at most
16 native places plus 8 rate integer digits plus 4 Money places for division). Both remain non-negative and bounded by `999999999999.9999`.
Snapshot construction, SQLite writes/reads, and database-open integrity checks
apply the same base-amount rule. Storage never rounds converted components;
`Money` projections round only at the existing display/aggregation boundary.
The existing `base_amount` TEXT column is unchanged, so this requires no schema
migration. Older binaries with the 16-place base validator cannot open databases
once extended-precision snapshot items have been persisted.

Available-funds calculations accept valuation `NativeAmount` values with up to
16 fractional digits. Current-value routes retain this precision through fees,
reservations, route selection, aggregation, and FX conversion. `Money` fields in
liquidity and product-detail responses are rounded display projections; they do
not replace the exact valuation input. User-entered monetary amounts retain the
existing four-fractional-digit validation contract.

Liquidity read DTOs use `displayState: needs_info` for unknown access rules or
missing receipt timing, distinct from a confirmed lock. Consumers must render a
request to complete the rules, not “no action”. Product preview warnings use
stable localization codes (`existing_position_increases_assets`) rather than
English prose. History uses `Activity.productContext.productId` to open product
management; its generic correction/undo controls do not apply to product-linked
activities, including product interest entries.

### Full derived-data rebuild

`DataService.RebuildDerivedData` replays the complete ledger from the immutable
History Origin through the current time. It holds the application write gate,
loads one consistent historical input batch, validates every intermediate balance
and quantity, and computes every closed local day's valuation before publication.
A single SQLite transaction replaces current replay projections, repairs event
projections, updates holding quantities, replaces daily snapshot revisions and
items, and resets snapshot completion state. An input-generation check prevents
publishing a stale batch. A failed calculation or transaction leaves the previous
results in place. Analysis caches are invalidated after publication; gain and
return views are calculated from the rebuilt inputs when read.

Activities, correction links, opening balances, account and instrument
observations, product operations, prices, FX observations, coverage evidence,
settings and credentials remain intact. A `value_update` event balance is the
persisted absolute amount entered by the user, so it is retained as an input; its
relative effect is recalculated against the preceding balance. Missing prices or
FX remain missing rather than being invented by a rebuild. Today's valuation is
live; closed-day snapshots end yesterday in the History timezone.

Historical entry is validated at its effective time and then replayed through
later events before committing. The preview shows the event-time result; current
projections contain the final replayed result. A later absolute balance
observation remains fixed. Inserting an entry that makes a later balance or
quantity invalid fails without writing the entry.

## Continuous backup configuration and status

Schema and stream support follow [Database support and storage recovery](#database-support-and-storage-recovery).
The separate private `backup_config.db` is excluded from business backup and
exports. R2 credential updates save the complete pair with configuration
atomically; blank means keep, removal is explicit. Outbound DTOs contain a
configured boolean and no raw keys. Existing provider/AI/MCP configuration
remains in the business database and is included in backup.

The UI service exposes status, configuration, connection check, manual backup,
recovery listing, staged preview and explicit confirmation. Last attempt and
last successful remote-confirmed backup are distinct; a captured LTX equality
is not a claim that later business writes have been scanned. These operations
are not exposed through MCP. See [continuous backup](../development/continuous-backup.md).


### Additive display valuation contract

Overview/trend household amounts expose assets, liabilities and signed net worth
as `ValuationAmount`; portfolio/row/Asset Trend level amounts expose one amount.
Each contains nullable `total` and `knownSubtotal`, currency and typed quality.
Legacy strict fields retain their existing meanings. Estimated endpoint changes
use exactly the requested full boundaries, include flows and are not returns.
Historical overview/series and financial-context summaries expose this additive
projection through existing read-only MCP serialization and response budgets.
Quote evidence retains effective NAV/reference date, value-effective timestamp
and retrieval timestamp separately. No observation is synthesized for carry.
Coverage-only valuation notices are informational and do not inflate actionable
health counts. Supported provider finalization rules retain their existing dates;
unknown publication schedules receive no guessed deadline.

The mounted ledger clock checks local civil date on timer/focus/resume and
invalidates valuation captures when it changes. Quote/activity/source changes
also invalidate historical captures; focus/reconnect alone do not. A refetch
retains the same date's successful data with an updating indicator, or a prior
result/error/retry label. Navigation has no cross-date placeholder.


### Shared instrument valuation calendar

The domain calendar is shared by current and historical valuation, data health,
history repair planning, asset-change and return projections, and investment
review. Instrument quote-source preference is effective as of the valuation day.

| Instrument type | Default Saturday/Sunday policy |
| --- | --- |
| Stock | Closed; carry the last eligible price |
| ETF | Closed; carry the last eligible price |
| Mutual fund | No NAV publication; carry the last eligible NAV |
| Bank investment product (unit NAV) | No NAV publication; carry the last eligible NAV |
| Bond | Closed; carry the last eligible price |
| Precious metal | Closed; carry the last eligible price |
| Crypto | Daily quotation required |
| Other | Daily quotation required unless explicitly closed |

For every type, manual quote preference makes its selected eligible price valid
until replaced. Agent `kind: "closed"` facts declare a single instrument/date
closed, including a holiday or a date with explicitly no NAV publication. Every
intervening date after the anchor must be closed; an older missing trading day
remains incomplete. An absent or ineligible anchor stays unknown. Carry retains
source, quote ID, date, currency, and price basis; it never inserts price rows or
pretends that a new observation was published. Valid carry creates no pending,
stale, provisional, or partial-coverage count merely because the price is old.

Asset changes and investment returns consume the same authoritative daily
valuations and original quote IDs. Review amount thresholds use the same price
selector. Review performance retains `latestOn` as the actual observation date,
records `asOfOn` as the evaluation date, and anchors lookbacks and annualization
to that evaluation date. Its comparable-history and baseline-gap requirements
remain in force; manual valuation alone cannot establish an unknown return.
Descriptive quote charts continue to contain actual published observations.

FX uses its own source preference and reference-rate calendar; instrument closure
facts do not close currency pairs. Review currency conversions retain their
explicit reference-day/timestamp age limits. Cash, account balances, pensions,
liabilities and manually valued accounts use ledger state, not this instrument
calendar. Managed term/locked bank products continue to use their dedicated
principal, accrual, maturity and valuation rules rather than unit-NAV rules.


The instrument editor exposes **Set closed day** for active, unmanaged instruments
with any quote preference. `quote.Service.SetInstrumentClosedDate` adapts the
selected civil date to the same `ImportAgentMarketData(kind: "closed")` command
used by MCP, with explicit user-declared provenance. No quote-source switch or
synthetic price is written. The editor retains the operation UUID for retries,
refreshes valuation/analysis/review and snapshot-state queries after success,
and distinguishes a durable closure save from a pending snapshot rebuild.

## Frontend preference writes

Settings edits retain only explicitly edited fields. Header, onboarding, and
Settings saves share a queue per query client; each save loads the current
nonsecret document before merging its patch through the existing full-document
`Save` IPC. Explicit fields win in submission order, including when a user
explicitly edits a field also changed in the header. Untouched fields retain
newer values. Reset participates in the same queue. Failed writes do not restore
cached snapshots or prevent later writes. The service continues to preserve
separately managed provider credentials. This ordering covers the desktop's
shared query client, not independent external processes writing the settings
file.
