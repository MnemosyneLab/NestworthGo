# Local MCP integration

Owner: Nestworth application maintainers. Scope: the local desktop agent boundary.
Status: **Development complete (2026-09-29)** for the user-approved scope:
directory maintenance, daily and atomic batch ledger operations, position imports
and transfers, period analysis and contribution drilldown, current balance,
quantity and total-cost reconciliation, pure cost corrections, and guarded
activity undo/fix. End-to-end native UI and external-agent acceptance is assigned
by the user to other testers; it is not a remaining development gate.

## Delivery plan

The agreed scope is a complete business entry point: directory maintenance,
daily ledger operations, queries/analysis, and reconciliation/corrections.
The user confirmed completion after the following final three items:

- [x] Target total-cost reconciliation and pure cost correction.
- [x] Position transfers with cost preservation and atomic destination creation.
- [x] Return contribution queries and detail drilldown.

This development plan is complete. Receipt/plan cleanup, combined directory and
ledger transactions, and broader frontend/MCP parity are separate follow-ups.

| Stage | Scope | Status |
| --- | --- | --- |
| 1 | Local transport, credentials, permission modes, directory/account/instrument maintenance, current queries, UI refresh, write receipts | Implemented; validation below |
| 2 | Income/expense, buys/sells, transfers/FX, dividends/debt, existing holdings, preview/commit, atomic batches | Implemented, including atomic batches and position transfers |
| 3 | Period income/expense, asset changes, investment returns, contribution, historical detail and quality | Implemented, including contribution grouping, pagination and detail tools |
| 4 | Target-state balance/quantity/cost reconciliation, pure cost correction, safe undo/fix | Implemented, including distinct cost correction events |

Permissions are explicit and household-wide. `read_only` registers queries and
analysis tools. Analysis can maintain local derived snapshots and invalidate
ledger previews; it has non-read-only MCP annotations but cannot write financial
facts or provider data. Strict no-write context/comparison/page reads remain
distinct from this analysis path.
`directory_write` adds directory mutations; `ledger_write` adds daily ledger
preview/commit, reconciliation and corrections, data health repair, and includes directory maintenance. Existing installations retain
their selected permission. Account creation still permits only an empty/zero
initial amount. Enable ledger permission explicitly to record subsequent funding.

## Read-only financial context

`get_financial_context` captures current state or one closed date with a
consistent asset/liability summary, exact nullable amounts, local gaps and
evidence. `get_financial_context_page` reads bounded details from that same
frozen result. `get_financial_context_item` selects related account, position or
evidence rows from a frozen package at its already selected disclosure without
rereading SQLite. All three
use existing read permissions; default aliases minimize
output but do not isolate household-wide credentials. No model, network refresh,
snapshot materialization or repair runs. See [the full contract](financial-context.md)
for history semantics, disclosure, hash, pagination, revocation and budgets.

## Connect

Open **Settings → AI / MCP**, choose read-only, directory maintenance, or ledger recording, and
click Enable MCP. Show/copy the connection configuration into a local MCP client.
It supplies a Streamable HTTP `/mcp` URL and an Authorization bearer header.
Clients with another configuration format should map those two values into their
own connection settings. Keep the desktop App running. Remote/cloud clients
cannot reach this loopback endpoint without a separately designed relay.

The first activation chooses an available IPv4 loopback port, then persists it
for stable reconnects. A port conflict fails closed; it does not pick another
instance's database. Disabling revokes the token; enabling again or changing
permissions generates a new one, so recopy the connection configuration.
Normal restarts retain the token and port. Configuration, preview plans and operation receipts live in SQLite, including the token, port,
and instance ID. They are excluded from JSON household exports and included in
full SQLite backups. Legacy connection/receipt JSON files migrate on startup.
Development and release instances must use distinct database paths. No connection is enabled by default.

## Architecture

Managed product alignment is documented in [mcp-products.md](mcp-products.md).
All modes expose list_products, get_product, list_product_operations and
get_liquidity_overview using GUI DTOs, with gross/after-reservation amounts and
forecast/unknown evidence preserved. ledger_write adds three typed pairs:
preview_product_operation/commit_product_operation,
preview_product_terms/commit_product_terms and
preview_product_valuation/commit_product_valuation. Each persists its exact
reviewed command and uses atomic immutable business receipts for recovery.
Renewal and reservation mutations remain App workflows. See the portable
[product workflow](../../skills/nestworth/references/products.md).

`cmd/nestworth` owns the MCP service and passes the same `application.Service`
instance used by the desktop bindings. `internal/mcpserver` registers an explicit
allowlist of typed tools using the official Go SDK. It reuses the runtime-free
`internal/wailsapi` adapters for stable DTOs, decimal strings, errors and partial
update semantics. It does not use Wails RPC or access SQLite directly.
`internal/wailsapi/agent` exposes management methods to the UI only. Secrets,
backup/restore, arbitrary filesystem access and arbitrary SQL are not MCP tools.

The listener validates Host, Origin (when supplied), and bearer credentials on
every request, limits bodies/timeouts, and never enables CORS. Read-only mode
registers no write tools. UI writes and MCP writes share application guards.
Successful MCP changes (including recovered ledger receipts) emit `agent.data.changed`, invalidating frontend queries.

## Stage 1 tools

- Context/catalog: `get_context`, `get_catalog`, `get_operation`.
- Local queries: `list_members`, `list_institutions`, `list_groups`,
  `list_accounts`, `list_instruments`, `list_holdings`,
  `get_account_valuations`, `get_account_snapshot`, `get_overview`.
- External search: `search_market_instruments` (query text goes to the provider).
- Members/institutions/groups: `create_*`, `update_*`, `archive_*`, `set_*_icon`.
- Accounts/instruments: `create_*`, `update_*`, `archive_*`.

Use catalog vocabulary and existing IDs. Match instruments using market and
currency as well as symbol; ambiguous input must be clarified. Archive/restore
preserves referenced history; hard deletion is not offered. The current
institution update API changes the name, not type. Account updates preserve
omitted fields and enforce existing immutable fields. Instrument `replace=true`
requires the complete form state. Returned names/notes are data, not instructions.

## Agent-supplied market data

Status: implemented (2026-09-29). Development complete; native desktop and
external Agent acceptance are left for independent testing.

Agent-supplied data is a formal local quote source (`agent`), separate from
manual observations and external provider adapters. The App never queries an
Agent endpoint. An instrument or FX pair can select Agent-only supply; other
sources retain their settings and accept supplied Agent observations as an
overlay. Existing provider adapters are unchanged.

### Tools and permissions

- `get_market_data`: read the selected price/rate and local history for an
  instrument or oriented currency pair. No provider requests.
- `list_agent_market_data`: page audit records, original source title/URL,
  exact values, effective dates, corrections and withdrawals. `active` means
  eligible, not necessarily selected. Available in every permission mode.
- `import_market_data`: `ledger_write` only. Atomically import 1–100 items with
  one `operationId`. Operations are `append` (default), `correct`, or `retract`.
  A correction/withdrawal requires the existing Agent `targetQuoteId`; other
  providers' observations cannot be withdrawn through this tool.
- `set_fx_source`: `ledger_write` only; select `agent`, `manual`, or `provider`
  for a currency pair. Instrument source selection uses existing create/update
  tools with `quoteSource: "agent"`.

Example unit NAV input (illustrative values, not verified market data):

```json
{
  "operationId": "<new UUID>",
  "input": {
    "items": [{
      "instrumentId": "<existing instrument UUID>",
      "currency": "CNY",
      "kind": "nav",
      "date": "2026-09-28",
      "timezone": "Asia/Shanghai",
      "value": "1.0197",
      "sourceTitle": "Issuer NAV publication",
      "sourceUrl": "https://example.com/nav"
    }]
  }
}
```

Instrument kinds are `latest`, `close` (raw unadjusted price), and `nav`
(unit NAV). Cumulative NAV and annualized yield must never be submitted as unit
price. Instrument currency must explicitly match the instrument; zero is a
valid observed price, distinct from unavailable. FX uses `baseCurrency`,
`quoteCurrency`, `value` and `kind: "latest"` or `"daily_reference"`, with
`1 baseCurrency = value quoteCurrency`; FX rates must be positive.

Latest observations require the actual RFC3339 `quotedAt`, not the lookup time.
Daily observations require their market/NAV/reference `date`. An optional actual
`quotedAt` must agree with that date in `timezone`. Without a time, a date-label
anchor is stored and reported with `timestampBasis: "date_label"`; it is not an
asserted midnight market quote. Timezone defaults to the instrument market,
then the household history timezone, then UTC. Future observations are rejected.
Source title is required; a source URL is optional for document/screenshot input
and is stored only, never fetched. Optional `splitFactor` and `dividendCash`
preserve daily instrument metadata; they do not create ledger activities.

### Selection, coverage and rebuilding

- Current selection first resolves daily observations by effective date (Agent
  wins for the same date), and separately selects the latest realtime value.
  Those winners compare actual quote timestamps, with Agent winning ties.
  Daily date-label anchors retain their original timestamp. The portfolio and
  direct quote APIs use the same candidates and selection rules.
  Historical Agent close/NAV/reference values win for their exact effective
  date. Retained provider/manual facts become eligible again after withdrawal.
- Date-range charts filter daily NAV/close/reference values by `effectiveDate`,
  including when the observation timezone differs from the household timezone.
  Realtime values and explicit timestamp queries keep instant-based bounds.
- Metal conversion revisions retain the original raw observation and FX times.
  Withdrawing an FX quote appends a recalculation using the fallback rate, even
  when that rate has an older timestamp or restores a previously saved value.
  Without an eligible fallback rate, current converted prices are unavailable.
- Normal sync skips exact daily dates covered by Agent observations, including
  ordinary recent-correction rechecks. A fresh current Agent quote suppresses
  the corresponding normal latest fetch. Missing dates and stale latest quotes
  still use the configured provider. Explicit force refresh may fetch that
  provider again; it does not remove Agent data. Agent-only targets never pull.
- A carried NAV/price keeps its actual observation date and freshness state.
  Known stock closure days use the existing calendar rules to carry a close.
  Missing trading days and unobserved NAV/FX dates remain incomplete.
- Batch values, evidence, deactivation, the idempotency key and affected snapshot
  invalidation commit together. Reusing the same operation UUID and input does
  not duplicate facts; a different payload conflicts. Corrections append facts
  and keep the previous version in the audit trail.
- Import rebuilds affected closed-day snapshots through the existing service.
  Quote commit, local dependent revaluation (`currentValuationStatus`), and
  rebuild outcome are reported separately: `pending` means
  data was saved but derived work still needs repair, and
  `rebuilt_check_health` means the rebuild ran, not that all inputs are complete.
  Follow with `get_market_data` and `scan_data_health`. An initially unconfigured
  FX pair gets an Agent preference; an existing preference remains unchanged.
- Full SQLite backup and JSON export retain the Agent facts and audit records.
  Schema 15 stores the quote-source constraints and durable import/audit
  tables. Existing provider/manual records and source preferences are preserved.

Validation: full Go suite; application Agent-flow and MCP import race tests;
SQLite race tests; frontend suite (72 files, 559 tests); TypeScript, lint and
generated binding checks. Tests use temporary databases and fake providers.
No live household database or real provider service was used.

## Data health repair extension

Status: implemented (2026-09-29).

This extension reuses the frontend Data Health application service and its
`repair_all` sync pipeline, including provider limits, write guards, snapshot
rebuilds, verification and progress events. It does not implement a separate
repair engine or change ledger facts. UI and MCP use the same DTO converters;
MCP queries do not replace the desktop sync event listener.

| Tool | Permission | Result |
| --- | --- | --- |
| `scan_data_health` | All modes | Local issues, executable actions, prerequisites, coverage and missing/outdated/incomplete snapshots |
| `preview_data_repair` | All modes | `repair_all` targets, estimated provider requests, estimated snapshot days, items and unresolved prerequisites |
| `start_data_repair` | `ledger_write` | Durable submission receipt containing `job`, `attached`, `conflict`, and optional `reason` |
| `get_data_repair_job` | All modes | Exact job's progress/result and a fresh local health report |

Recommended flow:

1. Call `scan_data_health` with `{}`. Report actions that require provider
   settings, instrument edits or manual data before promising complete repair.
2. Call `preview_data_repair` with `{}` and inspect request/work estimates and
   `unresolved`. Like the frontend preview, this is an estimate; starting repair
   replans against current data. No provider HTTP or repairs occur in either read.
3. Call `start_data_repair` with `{"operationId":"<new UUID>","input":{}}`.
   Reuse the same operation UUID for a retry. Inspect the receipt's `result`:
   `attached=true` joins an equivalent running job; `conflict=true` means another
   scope is running and **no repair_all job was scheduled**. After that job ends,
   a new intentional repair requires a new operation UUID.
4. Poll `get_data_repair_job` with `{"jobId":"<result.job.jobId>"}` at a
   reasonable interval. Read `job.phase`, `outcome`, request/target counters,
   `snapshotDaysPlanned`, `snapshotDaysRebuilt`, `items`, `blockers` and
   `prerequisites`. Outcomes include `running`, `succeeded`, `partial`, `failed`
   and `cancelled`. A succeeded **submission receipt** is not a completed repair.
5. Report both rebuilt days and remaining `health.issues`. Rebuilt snapshots may
   still lack prices or FX; **snapshot rebuilding is not data completeness**.
   The health report is a fresh scan when queried, not a frozen completion report,
   and remains provisional while a job runs or other writes occur.

Starting repair may contact configured external providers and persist market
data and derived snapshots. It does not expose credentials or modify provider
settings. Accepted jobs continue independently of the MCP request/connection;
use the existing Data Health UI to cancel a running job. The frontend receives
the same sync events for jobs started through MCP.

Job snapshots remain available by their exact IDs in the running App process,
including older completed jobs. They are not persisted across App restarts.
`not_found` does not establish a job's outcome: run `scan_data_health` again to
inspect actual data. Durable operation receipts retain the original submission
result only. An interrupted pending receipt is `unknown` and is not automatically
replayed; inspect current health and the Data Health UI before a new repair.

Validation on 2026-09-29:

- Full `go test ./...`, focused MCP repair and marketdata adapter race tests,
  and `go vet` for MCP/marketdata adapters passed.
- Temporary-SQLite HTTP tests cover all permission modes, matching UI DTOs,
  durable submission retries, job ID isolation, and preservation of the desktop
  sync listener. A manual-price-missing holding rebuilds its planned snapshots
  while correctly retaining a partial outcome and incomplete-data issues.
- Generated Wails bindings check and six settings permission tests passed.
- External provider integration, native UI and external-agent acceptance were
  not run for this extension. No live household data was modified.
Current valuation is not yesterday's income or investment return; missing inputs
must remain visible rather than being treated as zero.

## Ledger preview and commit

With `ledger_write`, use:

1. `get_context`, `get_catalog`, and existing entity queries to resolve IDs,
   currency, history timezone and reasons. History must already be started in
   the App; MCP does not choose or reset the starting point.
2. `preview_change` with a typed change command. The result contains `planId`,
   `expiresAt`, the normalized `command`, and `preview` effects/resulting values.
3. Inspect the preview, then call `commit_change` with
   `{ "operationId": "<new UUID>", "input": { "planId": "<returned planId>" } }`.
   Ledger permission authorizes this flow without another App confirmation.
4. Read the returned operation receipt, or query `get_operation` by operation ID.

Supported kinds are `money_added`, `money_removed`, `trade` (`buy`/`sell`),
`cash_dividend`, `cash_transfer`, `fx_conversion`, `debt_draw`, `debt_payment`,
`position_transfer`, and `position_import` (details below).
Amounts, quantities and fees are decimal strings. For a trade, `gross` is the
**total consideration**, with a separate optional fee; it is not a unit price.
First buys create their holding and update settlement cash in one transaction.
Preview activity/new holding IDs are provisional; use committed receipt IDs.
Selling requires sufficient holdings; insufficient cash and incompatible
currencies fail under the same domain rules as the App. These are records of
completed transactions, never brokerage orders.

Specify either RFC3339 `effectiveAt`, or both `effectiveLocalDate` and
`effectiveLocalTime` in the immutable history timezone. An omitted timestamp is
frozen at preview. Historical entries use the existing replay path, including
validation of later balances and quantities. Fields irrelevant to the selected
kind and incomplete amount/currency fee pairs are rejected.

Plans expire after ten minutes. Their command is stored server-side and cannot
be replaced at commit. Application write permits (including failed writes,
snapshot materialization, and exclusive backup/restore work) conservatively
invalidate outstanding previews. Preview reads serialize with writers but do not
invalidate each other. Application restart invalidates uncommitted previews.
Request a fresh preview after a stale result. Check balances with read queries
before preparing the preview, and commit promptly afterward.

The plan ID is also the ledger's atomic database mutation ID. Posting a plan
again, even with a new operation ID, returns its original activity. A ledger
retry after an interruption can recover from the database mutation even after
plan expiry/restart; it never duplicates the activity. Expired plans are retained
for this recovery, so plan/receipt storage currently grows with use. Backups
include them; credential-free household JSON exports do not.

Each single commit is one complete business operation, including its cash/position
legs. Multiple independent tool calls are not an atomic batch; use the tools
below when a group must succeed together. Target quantity/cost adjustments
use the reconciliation tools.

## Reconciliation and corrections

`preview_reconciliation` takes `{ "targets": [ ... ] }` with 1 to 100 current
endpoints. Use either:

- `{ "accountId": "<UUID>", "targetBalance": "1250", "currency": "USD" }`:
  a simple account's balance, or a holdings account's cash in that currency.
- `{ "holdingId": "<UUID>", "targetQuantity": "12", "unitCost": "25" }`:
  a holding quantity. An increase requires the original per-unit cost of the
  **added quantity**; a decrease preserves existing average cost and must omit
  `unitCost`. These adjustments have no cash leg or sale proceeds.

The server resolves targets under the preview gate, freezes the current time,
and returns the requested `targets`, exact `commands`, and their `previews`. `commit_reconciliation`
uses the usual operation envelope and commits the stored adjustments atomically.
It never recomputes differences from a later balance. Duplicate endpoints,
unchanged targets, archived/managed holdings, unsupported currencies, and missing
cost basis fail without posting any target. This is current-state reconciliation;
historical statement dates are not supported.

For a **pure cost correction**, use:

```json
{ "holdingId": "<UUID>", "totalCost": "300", "currency": "USD" }
```

This creates a distinct `cost_adjustment` activity with a `holding_cost` effect,
without cash movement or a quantity change. The currency must match the
instrument. `totalCost` is the cost of the entire remaining position. To reconcile
quantity and cost together, include `targetQuantity`; do not also provide
`unitCost`. A combined target can produce a quantity adjustment followed by a
cost correction, both in the same transaction. The expanded plan is limited to
100 commands. A nonzero cost requires a positive position quantity.

Average unit cost uses the existing eight-decimal precision. The server checks
that multiplying it by the target quantity reproduces the requested total at
stored money precision; an unrepresentable target is rejected. Corrections take
effect at preview time and apply to subsequent sale/transfer cost calculations.
Earlier realized gains are not rewritten by a current cost target. To change an
erroneous historical trade, use the historical fix workflow instead.

Generic undo/fix is not offered for a cost-adjustment record; submit a new target
cost to correct it again. The frontend displays its cost effect in activity
history. Schema 15 is the current and only supported schema for existing
databases. Existing files are probed read-only and rejected without modification
if they are not schema 15; this includes the schema-11 database from v0.3.4,
older, unversioned, and future schemas. There is no automatic schema migration.
This cost-adjustment event is preserved in full SQLite backups and versioned
household JSON exports.

`preview_correction` takes `activityId` and one of:

- `action: "fix"`, with a complete `replacement` ledger command. Omit timestamps
  and mutation IDs: the original effective time is preserved. Trades require
  an explicit existing `holdingId`; `position_import` cannot be a replacement.
  The server replaces the historical economic record, preserves correction
  audit links, and replays later records. A dependent invalid balance or quantity
  rejects the entire correction.
- `action: "undo"`, without `replacement`. This records an inverse at the
  preview time, affecting that day's reports while preserving prior history.
  It checks current balances and quantities before accepting the reversal.
  A later quantity event on the same holding blocks undo to protect cost-basis
  history; use a historical fix for such a dependency.

For a fix, `preview.resulting` describes the state just after the historical
replacement, rather than today's balance. Read current valuations after commit.
Inspect `preview` and submit only `planId` through `commit_correction`. Already
reversed/corrected activities and managed products are rejected. Corrections use
an atomic database mutation receipt; retries return the original activity even
with a new operation ID or after expiry/restart. A post-commit snapshot refresh
failure remains recoverable by retrying the same operation and plan.

All four plan types are distinct: use the matching commit tool. The usual
expiry, stale-state, provisional-ID and receipt recovery rules apply.

## Position transfers

Use `preview_change` or `preview_batch` with `kind: "position_transfer"`,
`fromHoldingId`, `quantity`, and exactly one destination:

- `toHoldingId`: an existing holding of the same instrument in another account.
- `toAccountId`: resolve that account's holding of the source instrument, or
  create it atomically with the transfer if it does not exist.

No cash moves and no sale/income is recorded. The transferred units carry their
source cost at the effective time, including any earlier cost corrections.
A later correction does not change a transfer dated before it. Standard
preview/stale/idempotency rules apply. In a batch, later trades can resolve a
newly created destination by its account/instrument pair. For historical fixes,
supply explicit existing holding IDs; a fix cannot introduce a new holding.

## Atomic batches and existing positions

`preview_batch` takes `{ "commands": [ ... ] }` with 1 to 100 commands using the
same fields as `preview_change`. Commands must be in nondecreasing effective-time
order; equal times retain array order. Omitted times share one frozen instant.
Resolve existing member/account/instrument IDs first: this is a ledger batch,
not an account/directory creation batch. No placeholder IDs are accepted.

The preview returns `planId`, normalized `commands`, and a `previews` array in
input order. Post it with `commit_batch`, using the same operation envelope as
`commit_change`. Its receipt returns `result.changes` in input order. All new
holdings, activities, derived projections and the batch mutation receipt are
written in a single SQLite transaction. A failure leaves none of the batch
posted. Retry the same plan after an interruption; the durable batch mutation
returns all original activities even after a process restart. Changed arguments
for an operation ID are rejected. A saved successful operation receipt remains
unchanged. Recovery or a different operation ID for the same plan reads the same
activity IDs, but its resulting balances may reflect later historical replay;
use current valuation queries for current balances. Single-change and batch plans are not
interchangeable. Expiry and stale-preview rules apply to both.

A batch can fund an account and then buy, or import a position and then sell.
For later trades referencing the newly introduced position, omit `holdingId`
and use its account/instrument pair. Preview-generated IDs remain provisional.
Historical batches replay the combined timeline, including later existing
activities, and reject a resulting negative balance or quantity.

Use `kind: "position_import"` to record a position already owned:

```json
{
  "kind": "position_import",
  "accountId": "<existing holdings account UUID>",
  "instrumentId": "<existing instrument UUID>",
  "quantity": "10",
  "unitCost": "123.45",
  "currency": "USD",
  "effectiveLocalDate": "2026-09-28",
  "effectiveLocalTime": "09:00"
}
```

This creates a new active position with an acquisition-cost adjustment and no
cash movement or purchase. `unitCost` is the original per-unit acquisition
cost, not total cost or current market price. It is required; unknown cost is
never replaced with a quote or zero. The currency must match the instrument's
quote currency. An existing active position for that account/instrument is
rejected: this operation does not add to or reconcile an existing quantity.
Archived and managed-product instruments are also rejected. It can be used with
single preview/commit or inside an atomic batch.

Batch retries use the `change_batch_mutation_keys` table in supported schema-15
databases. Existing databases must already use schema 15: older databases and
backup files, including schema-v13 files, are rejected without adding a table
or changing the file. Full SQLite backups preserve batch receipts;
credential-free household JSON exports exclude retry bookkeeping.

## Historical queries

All three permission modes include:

- `list_activities`: inclusive history-local date filters, account/instrument/kind
  filters and bounded pagination. Pass the complete three-field cursor.
- `get_activity`: the recorded activity, effects and correction links.
- `analyze_period`: income, expenses, asset changes and investment return series
  using the App's analysis adapters. Own-account transfers and purchase principal
  are not household income/expenses.

Analysis takes `{ "query": { "from": "YYYY-MM-DD", "to": "YYYY-MM-DD" } }`
with the adapter's optional scope and filters. The end must be a closed local
day. It preserves missing coverage, nullable values and rate availability;
unavailable returns must not be reported as zero. The combined report holds the
application write gate for a consistent result and may rebuild derived snapshots.

## Write receipts and interruption semantics

Every write requires `operationId` (UUID) and a typed `input` object. Before the
business call, a private pending receipt is saved to the configuration store. A duplicate UUID with
identical normalized typed arguments returns the original successful receipt;
changed arguments are rejected. Successful receipts include the returned object.
A terminal domain validation error is saved as failed. Unexpected failures or
interruptions leave an unknown outcome. Directory operations are never automatically executed again: inspect data before
deciding on a new operation. Ledger commits can safely retry the same plan using
the atomic mutation key described above. `get_operation` retrieves
receipts across restarts; Settings lists the latest 20 summaries.

This is conservative duplicate protection for individual existing operations,
not a transaction spanning receipt storage and the household database. A crash
after commit but before saving success can therefore yield unknown. Receipts are
execution history, not proof of current state (especially after a backup restore).
Do not reuse an old UUID for a new intent, including reapplying a restored-away
change. Ledger batches use the atomic database boundary described above. Directory
mutations remain individual operations. There is no automatic
rollback or receipt expiry in stage 1; receipt storage grows with writes.

## Return attribution and drilldown

All permission modes expose these read tools, reusing the frontend's analysis
adapters and the same inclusive closed-day `query` shape as `analyze_period`:

- `list_contributions`: group returns by `instrument`, `account`, `currency`, or
  `asset_class`. Supply `returnType` (`total_return`, `realized`, `unrealized`, or
  `dividend_interest`), optional ordering and offset/limit (up to 100 rows).
- `get_contribution_item`: use an exact row key with the same query, return type,
  and grouping to inspect components, account breakdown and related-history hints.
- `get_return_day`: inspect one date's amount, rate, composition, contributors,
  coverage and valuation issues; the date must be within the query range.
- `get_asset_driver_detail`: inspect one `analyze_period.assetChange` bucket and
  its underlying instruments/accounts/residual sources.

Follow history hints with `list_activities` and `get_activity` for original
records. Preserve nullable amounts, availability and coverage fields. The
existing analysis backend reports unrealized contribution as unavailable; MCP
preserves that limitation rather than inventing an attribution. Analysis reads
can materialize snapshots and invalidate an outstanding preview, so query first.

## Acceptance criteria

- Income/expense excludes own-account transfers and investment principal flows.
- Trades update settlement cash, quantity, fees/cost and realized gain together.
- Existing-holding import does not imply cash funding or a new purchase.
- Period queries resolve household history timezone and disclose coverage gaps.
- Reconciliation takes current target balances, quantities and total costs
  without inferring a trade; cost-only corrections leave cash/quantity unchanged.
- Directory creation combined atomically with ledger writes, and plan/receipt
  retention cleanup, are follow-up extensions outside this completed scope.
- Preview plans bind to current data and reject stale concurrent edits.
- Validate against isolated databases and copied real data, never mutate the
  user's live ledger as an implementation test.

## Validation

Final scope verification on 2026-09-29:

- Full `go test ./...`: passed. Existing macOS deployment-target linker warnings
  remain. Targeted cost tests also pass after the final generic-fix guard.
- Focused race tests cover cost corrections, transfers, attribution and v13→v14
  migration: passed. Static `go vet` for the touched Go layers also passed.
- Frontend typecheck and lint passed; focused history/settings tests passed.
  Wails bindings were regenerated for the added cost-preview field and the
  generated-bindings consistency check passed.
- Migration regression preserves prior activity/effect/projection/mutation
  records. Cost tests verify unchanged cash/quantity, corrected subsequent sale
  gains, inherited transfer basis, preserved acquisition FX provenance, and no
  spurious asset-change flow. Export retains the cost effect and cost-basis event.
- External-agent and native UI acceptance is handed off to the user's testers.
  All automated ledger tests used temporary databases; no live ledger was changed.

Reconciliation/correction extension verification on 2026-09-29:

- Full `go test ./...`, focused application/MCP race tests, and `go vet` for
  application/MCP/SQLite: passed. Desktop compilation still emits existing
  macOS deployment-target linker warnings.
- Frontend `tsc --noEmit` and six settings permission tests: passed.
- Temporary-database HTTP tests verify current targets, original-date fixes,
  frozen-time reversals, stale/expired plans, mode gating and receipt recovery.
- Cost assertions cover added quantity and reductions; unavailable balances or
  costs and archived/managed holdings are rejected. A later holding trade blocks
  an unsafe undo without changing cash, quantity or activities.
- Fault injection verifies rollback of both correction activities and the
  mutation key, and recovery after a committed correction whose snapshot
  refresh returned a domain error.
- Native UI, external-agent acceptance and copied-real-data checks were not run
  for this extension. No live ledger was modified.

Batch/import extension verification on 2026-09-29:

- `go test ./...`: passed, including desktop package compilation. Existing
  macOS deployment-target linker warnings remain.
- Focused race tests across `internal/mcpserver`, `internal/application`, and
  `internal/infrastructure/sqlite`: passed for batches, imports, mutation replay,
  and guarded concurrent writes.
- `go vet ./internal/mcpserver ./internal/application ./internal/infrastructure/sqlite`:
  passed.
- Frontend typecheck, lint and the six settings permission tests: passed.
  Wails bindings regenerated without manual edits. This extension changes only
  permission descriptions in the frontend; the prior full frontend baseline
  was 71 test files / 549 passing tests and a successful production build.
- `git diff --check`: passed.

MCP tests use temporary SQLite households and the official HTTP SDK. New tests
cover dependent funding/buy/sell batches, import/sell batches, explicit-cost
imports without cash effects, duplicate active-position rejection, wrong commit
tools, timestamp ordering, request bounds, stale/expired plans and pending-receipt
recovery. An injected failure on the second Activity verifies that SQLite rolls
back new holdings, activities and the durable batch key together.

Application tests check historical insertion across existing later records,
same-time order followed by another single write under a fixed clock, and replay
after reopening the database. A compatibility test proves read-only verification
of a pre-feature v13 database leaves its bytes and missing batch table unchanged;
writable Open then adds the table while preserving existing household facts.

Native UI and a configured external agent using the new batch/import tools were
not exercised. No live ledger was modified.


Two-state read-only comparisons use `compare_financial_context` and
`get_financial_comparison_page`. They capture both states together and compute
right-minus-left changes with shared identity aliases. These are not returns or
attribution. See the [comparison contract](financial-context.md#two-state-comparison)
for scopes, category definitions, disclosure, nullable differences and shared
cache/transport budgets.

For “why did these two states change; was that profit?”, use
`compare_financial_attribution` for a new coherent comparison after derived
snapshot maintenance. It supplies compatible/incompatible/unavailable status,
normalized A+1..B local period, scoped evidence digest, existing net-worth
waterfall and separate investment-return summary. Stop on compatibility failure;
never append live analysis to an old frozen comparisonId. Household and a single
account are supported; account sets and current-right returns are unavailable.
The comparison retains exact valuation decimals. The link declares the existing
four-place half-even component/period-driver projection and separately discloses
signed precision adjustments: exact change.netWorth equals explainedDelta plus
residual plus precisionAdjustment. These adjustments are neither returns nor
unknown residuals; exact per-day bucket gaps still fail strict reconciliation.
Actual A..B snapshot coverage is backfilled independently of a later completion
watermark; later requests retain earlier unbuilt dirty markers. Unsupported
A..B+1 midnights stop before maintenance.
Its frozen pages use `get_financial_comparison_page`. See the
[attribution contract](financial-context.md#coherent-comparison-attribution) and
[portable workflow](../../skills/nestworth/references/analysis.md#link-a-comparison-to-change-attribution).

Analysis can write derived snapshots without changing financial facts. Accordingly
`analyze_period`, `list_contributions`, `get_contribution_item`, `get_return_day`,
`get_asset_driver_detail` and `compare_financial_attribution` publish
readOnlyHint=false/destructiveHint=false; all remain in existing permission modes.
`compare_financial_context` and frozen page tools retain strictly read-only hints.
Updating these repository skill files does not install them into a client.
