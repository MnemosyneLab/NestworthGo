# Local MCP integration

Owner: Nestworth application maintainers. Scope: the local desktop agent boundary.
Status: **Development complete (2026-09-29)** for the user-approved scope:
directory maintenance, daily and atomic batch ledger operations, position imports
and transfers, period analysis and contribution drilldown, current balance,
quantity and total-cost reconciliation, pure cost corrections, and guarded
activity undo/fix. This document does not establish end-to-end native UI or
external-agent acceptance; consult the release record for their status.

Tool definitions and typed handlers register through the shared capability
registry; the external SDK adapter retains the existing protocol and behavior.
The [built-in AI prerequisite and target design](../architecture/builtin-ai.md)
documents direct in-process sessions and their closed production activation gates.
This does not install an App assistant or add App confirmation to external MCP.

## Read-only financial context

`get_financial_context` captures current state or one closed date with a
consistent asset/liability summary, exact nullable amounts, local gaps and
evidence. `get_read_page({cursor,limit?})` reads bounded details from that same
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
forecast/unknown evidence preserved. ledger_write adds three typed previews:
preview_product_operation, preview_product_terms and preview_product_valuation,
all committed with `commit_plan`. Each persists its exact reviewed command and
uses atomic immutable business receipts for recovery.
Renewal and reservation mutations remain App workflows. See the portable
[product workflow](../../skills/nestworth/references/products.md).

## Recent GUI read parity

All permission modes expose `get_upcoming_actions`, using the App's
`upcoming-actions/1` aggregation and DTO so dates, timezone, capture freshness,
coverage and reason codes are not reconstructed from product lists. All modes
also expose `get_historical_overview` as a
read-only application service call with default bounded `historical-read/2`
responses in the `mcp-read/1` envelope. Pass `{date, compareTo}` for one closed
date, optionally compared with another closed date or current, or `{from, to}`
for a 1–31 day trend. Do not combine the two input shapes. The overview preserves the App's immutable
history values and `endpoint_bridge/2` convention, including exact values,
completeness, timestamp basis, later-endpoint FX and no-write behavior. It does
not replace closed-period `compare_financial_attribution`.

Daily investment review v2 has pure read tools for named rules, one-rule
preview, grouped paged alerts, current alert detail, positive-holding coverage
(including unknown and uncomputed results), and suppressions. MCP reads only
the last complete publication. They never implicitly reconcile or fetch
quotes. Ledger-write tools create/update/archive rules, acknowledge one
episode, set/clear rule-scoped or all-rule suppressions, and explicitly
reconcile. The v2 rule spec separates predicate from single, consecutive and
rolling trigger policies; each result is for a closed civil date in its fixed
timezone. Empty or missing daily evidence is unknown. The MCP outer
`operationId` and the application's inner `mutationId` must both remain stable
for exact retries; CAS revisions protect each edited object. v2 reports are
cursor-bound to their filter and publication revision.

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
- Members/institutions/groups: `create_*`, `update_*` (name, iconKey or both; at least one), `archive_*`.
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

- `get_market_data`: read the selected current price/rate for an
  instrument or oriented currency pair. No provider requests.
- `list_agent_market_data`: page audit records, original source title/URL,
  exact values, effective dates, corrections and withdrawals. `active` means
  eligible, not necessarily selected. Available in every permission mode.
- `compare_market_data_import`: read-only, 1–500 ordered proposed rows / 100 targets /
  128 KiB. Default compact results preserve client IDs/order, exact classification
  and batch conflicts; default comparison page size is 100, capped by final bytes.
  Follow results pages for batch delivery. Source/selection/audit evidence remains
  deferred in the same capture; get_read_item section=results id=clientItemId reads
  focused details, or page result_details/matches/evidence/audits/targets for all
  details. view=full supports upfront bulk review. Computation completeness is
  distinct from evidence delivery; no import/refresh/write authorization occurs.
  See the [comparison API contract](../design/mcp-import-comparison-api.md).
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
  Both supported schemas store the quote-source constraints and durable import/audit
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
| `get_data_repair_job` | All modes | Compact exact-job progress by default; view=evidence freezes job details; current health is a separate scan |

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
   `snapshotDaysPlanned`, `snapshotDaysRebuilt` and evidence counts. Request
   `view=evidence` for frozen items/blockers/prerequisites when needed. Outcomes include `running`, `succeeded`, `partial`, `failed`
   and `cancelled`. A succeeded **submission receipt** is not a completed repair.
5. After a terminal outcome, run `scan_data_health` once and report both rebuilt
   days and remaining issues. Rebuilt snapshots may still lack prices or FX;
   **snapshot rebuilding is not data completeness**. This independent health
   capture describes current state, not immutable job-completion evidence.

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

1. Resolve IDs with filtered entity reads; use `get_context` for needed metadata
   and `get_catalog` for unknown vocabulary/entity creation,
   currency, history timezone and reasons. History must already be started in
   the App; MCP does not choose or reset the starting point.
2. `preview_change` with a typed change command. The result contains `planId`,
   `expiresAt`, the normalized `command`, and `preview` effects/resulting values.
3. Inspect the preview, then call `commit_plan` with
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

`cash_transfer` accepts only non-liability source and destination accounts.
The shared domain builder rejects liability endpoints with `invalid_change`
and `fromAccountId` or `toAccountId`, including batch and fix replacements.
Same-currency principal conserves its amount; a separate fee reduces source cash.
Cross-currency transfers retain actual sent/received amounts and their derived
settlement rate. Borrowing and repayment use `debt_draw` and `debt_payment`
because liabilities hold positive owed principal. Existing transfer records
remain readable and can be reversed or fixed to a valid command; this validation
does not rewrite their immutable effects or migrate stored history.

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
and returns the requested `targets`, exact `commands`, and their `previews`. `commit_plan`
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
history. Current schema is 22. Existing files are probed read-only; only verified
schema 21 upgrades to 22 after a verified, flushed recovery copy is retained.
Older, unversioned, future and invalid schemas remain blocked. See the
[storage contract](../architecture/data-and-ipc-contracts.md#database-support-and-storage-recovery).
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
Inspect `preview` and submit only `planId` through `commit_plan`. Already
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
input order. Post it with `commit_plan`, using the same operation envelope as
any other plan. Its receipt returns `result.changes` in input order. All new
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

Batch retries use the `change_batch_mutation_keys` table in both supported
schemas 21/22. Unsupported databases and backup files are rejected without
adding tables or changing the source. Read-only inspection preserves either
supported version; writable open upgrades 21 to 22. Full SQLite backups preserve batch receipts;
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

Every write except `commit_plan` requires `operationId` (UUID) and a typed
`input` object. `commit_plan` may omit `operationId`. Before the
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
  `dividend_interest`) and optional ordering. Returns `mcp-read/1` with a frozen
  `rows` section (default limit 25, maximum 100); continue with `get_read_page`
  using its captureId, section and signed cursor. Offset pagination is removed.
- `get_contribution_item`: supply the list capture's `captureId` and exact
  `groupKey`; query/returnType/groupBy/limit arguments are removed. Components,
  account breakdown and related-history hints come from the same frozen source,
  with independent `rows/<key>/components` and `rows/<key>/byAccount` sections.
  Follow their signed cursors with `get_read_page`. No recalculation or TTL renewal
  occurs; expiry, eviction or permission revocation requires restarting the list.
- `get_return_day`: inspect one date's amount, rate, composition, contributors,
  coverage and valuation issues; the date must be within the query range.
- `get_asset_driver_detail`: inspect one `analyze_period.assetChange` bucket and
  its underlying instruments/accounts/residual sources.

Follow history hints with `list_activities` and `get_activity` for original
records. Preserve nullable amounts, availability and coverage fields. The
existing analysis backend reports unrealized contribution as unavailable; MCP
preserves that limitation rather than inventing an attribution. Analysis reads
can materialize snapshots and invalidate an outstanding preview, so query first.
Realized contribution supports household, account, currency, instrument, and
asset-class scopes, plus account, instrument, currency, and asset-class filters.
Scope and filters intersect (AND), including when they constrain the same
dimension; contradictions return an available empty result. Currency filters
match each sale's native event denomination, not account default or output
currency. Asset-class filters match the instrument's current `Type`, as in the
analysis universe; historical sales follow current metadata if that type is
changed. Output stays in household base currency even when valuation is
requested as native. List/detail share the filtered projection, and an excluded
row key is not found. Realized grouping remains instrument or account only.
Member filters remain explicitly unavailable, and member is not a valid
analysis scope. History hints identify related activities and can be broader
than an exact currency/class-filtered breakdown.

## Current scope boundary

The local MCP contract and its executable tests define available operations.
Release records own version-specific validation and external/native acceptance;
this integration guide is not a historical test report or acceptance handoff.

## Ordinary liquidity policy parity

Available-funds access/withdrawal rules use the GUI's full policy inputs and
shared application preparation. All modes read `sources[].sourceRef`, `policy`
and `policyOrigin` through `get_liquidity_overview`. `ledger_write` additionally
advertises `liquidity_policy_write` and registers:

| Preview | Effect (committed with `commit_plan`) |
| --- | --- |
| preview_save_liquidity_policy | Create with expectedRevision 0 or fully replace the explicit revision |
| preview_reset_liquidity_policy | Remove the explicit revision and restore GUI assumed defaults |

Both previews and `commit_plan` are absent from read_only and directory_write.
Preview inputs are the GUI SavePolicyRequest/ResetPolicyRequest. Commit input
is only `{"operationId":"<UUID>","input":{"planId":"<preview UUID>"}}`.
Copy source identity from the overview: account_value uses accountId;
account_cash additionally uses currency; holding additionally uses holdingId.
A rule is never an instrument setting. Only active asset sources in that GUI
read model are accepted; invalid ownership/currency and archived/liability
sources fail before mutation. Managed positions retain managed_position_conflict
and must use atomic full terms+policy preview_product_terms.
No reservation or investment-review writes are exposed.

Save uses full replacement, not a patch. Every nullable field clears with null;
omission never means preserve (required schema fields must be supplied). The
complete form includes accessibleAmountCap, accessKind, unlockOn,
settlementDays, dayBasis, receiptOnOverride, normalExitFee, earlyKind,
earlySettlementDays, earlyDayBasis, earlyFee, earlyAmountMode, earlyGrossAmount
and note. Shared validation retains civil dates, 0–365 settlement counts,
calendar/weekdays, exact nonnegative decimal strings with up to four fractional
digits, unknown nullable fees versus known zero, early completeness and amount
modes. All non-null unlock dates are validated, including optional on_request
locks. No implicit rounding, percentage fee, bank withdrawal or ledger record
is introduced. See [executable examples](../../skills/nestworth/references/products.md#edit-ordinary-access-and-withdrawal-rules).

Previews serialize with all application writes and backup/restore, showing
effective before/after policies and origins. Assumed preview policies use the
sourceRef and revision zero to distinguish them from persisted explicit rows.
IDs/confirmation metadata are provisional. Commit validates the frozen full
command, expected revision, expiry and coordinator version inside the shared
write permit. Every attempted application write invalidates the version;
restart and exclusive restore also invalidate it. This prevents stale revision
and reset/recreate ABA overwrites. Wrong-family commits fail before the permit.
Reset commits return the same revision-zero acknowledgement as GUI ResetPolicy;
read the overview for evaluated defaults. Forecast availability can change;
ledger facts, current values, reservations and actual returns remain unchanged.

Ordinary policy writes deliberately use the established conservative operation
receipt contract, rather than managed-product atomic business receipts. Same
operationId/tool/input success returns the original recorded-time result;
changed arguments conflict. Pending/unknown receipts are not automatically
replayed. Inspect get_operation and current overview before a new intentional
preview. A used plan with a new operationId is stale and cannot write again;
there is no cross-operation business replay. Policies and outer receipts are
separate SQLite writes, so a crash after mutation but before receipt persistence
may remain unknown. This is a recoverability limitation, not permission to
repost. Production plans/receipts retain the restored database authority.
Successful writes emit the existing agent.data.changed event.

Synthetic Streamable HTTP tests cover discovery/permissions, every source kind,
full replacement and reset defaults, exact route/GUI parity, no ledger or reserve
changes, validation, CAS, stale/expired/wrong-family plans, operation retries and
unknown outcomes, plus continued atomic managed-terms compatibility. No native
UI, external client, provider, credentials or real financial data is required.

## Scalable management reads and batches (mcp-read/1)

The bounded mcp-read/1 contract is the sole response for liquidity, products,
upcoming actions, instruments, Agent records, directories/holdings, valuations,
overviews and health/repair reads. responseMode and Agent offset are removed;
unknown arguments fail schema validation. Filters, sorting and limits work with
ordinary requests. There are no old-version wrappers or full-response fallback.

The initial envelope includes schemaVersion=mcp-read/1, captureId/captureKind,
capturedAt/expiresAt, contentHash/summaryHash, normalized query, summary and
sections. Each section has items and page {total,returned,hasMore,nextCursor};
nextCursor is null on completion, arrays are always arrays. A zero-row deferred
section with a cursor is not empty. get_read_page takes cursor and optional limit and returns the same capture metadata/hash, section, items and page.
Filter and sort cannot change during continuation. get_read_item takes
captureId,section=products,id and returns the same capture metadata, full product,
permittedActions/disabledReasons and reservations/<productId> page entry.

Default limit 25, maximum 100; a page may return fewer rows because final SDK
text/structuredContent and JSON-RPC output must fit 65,536 bytes. Captures are
memory-only: fixed five-minute TTL, 4 MiB each, 16 MiB/eight entries total,
two concurrent builders, ten-second build deadline. SQL preflights row/payload
limits before expansion. Captures bind connection generation, capture type,
canonical query/hash and section identity. Section names encode reservation
parent or market target identity; signed cursors include version/generation/hash,
section and offset. Disable, mode change, restore and restart revoke both caches.
No financial content is logged or persisted. read_expired covers expiry/eviction;
read_revoked covers revocation; validation covers invalid or mismatched cursors.
Restart all sections from a fresh capture, never combine captures. too_large
messages distinguish summary, single_item and capture; rows are never skipped
or truncated. JSON-RPC arrays and IDs over 512 bytes are rejected for bounded
calls. The final guard also scrubs schema errors. Default calls receive the same complete wire cap and sanitized schema errors.

### Filter and sorting matrix

Within an array values are OR; different fields are AND. Empty arrays impose no
constraint; duplicates normalize away. ID arrays accept at most 100 UUIDs, query
at most 200 Unicode characters. Unknown enums, dates, currencies and fields are
validation errors. Nullable booleans distinguish omitted from false. All sorts
are global before pages and null-last in both directions, with identity asc ties.
Names compare trimmed case-insensitive text, not locale. Exact decimals compare
money/percentages, never float values or different native currencies.

| Read | Filters | Sort (default) | Sections |
| --- | --- | --- | --- |
| Liquidity | accountIds, sourceRefs, sourceKinds, instrumentIds, instrumentTypes, accessKinds, policyOrigins, hasRule, earlyKinds, currencies, excluded, query | sourceKey asc; displayName, instrumentType, accessKind, policyOrigin, normalReceiptOn, currentValueBase, availableBase | sources, unresolvedReservations |
| Products | accountId/includeClosed; productKinds, states, query | startOn desc; maturityOn, displayName, state | products; reservations/<productId> via item |
| Instruments | includeArchived; types, ids, query | displayName asc; name, type, updatedAt, id | instruments |
| Upcoming | accountIds, productIds, kinds, groups | priority desc then sortOn asc; sortOn, displayName | items; dataChecks filtered only by accountIds/productIds |
| Agent records | instrumentIds, oriented fxPairs, operations, active, observationKinds, fromDate/toDate, createdFrom/createdTo | createdAt desc; observationDate, operation | records |

Liquidity summaries retain household bucket totals and unresolved reservations;
source filters do not produce selected subtotals. Scope/count fields distinguish
total/matched/filtered-out sources. availableBase requires sort.horizonOn from
captured buckets and uses domain NetBase. currentValueBase uses the same
valuation/FX capture's unrounded BaseAmountExact. Source rows preserve full
policy/revision/routes/bucket/evidence/reservation data and add instrumentId/type,
nullable productKind/hasRule/policyEditable. productKind comes from the captured
managed contract and is separate from instrumentType; cash/value sources have
null instrument identity and productKind.
Saved explicit/contract rules mean hasRule=true; defaults mean false. Excluded
rows resolve policy facts from the same source snapshot; unavailable facts stay
null and do not match either boolean. policyEditable is eligibility, not token
permission. Managed rules still require dedicated product terms.

Product compact rows include identity, kind/name/shortName/symbol, currency,
principal/value/cost, state/displayState, revision and reservationCount. Full
terms/long notes/policy remain in same-capture detail; reservations sort createdAt
asc then reservationId asc. Product live history and global undo are unchanged. Upcoming keeps shared
projection counts, completeness, window and scopeWarnings. An extreme action's
nested targets can still exceed a single item and fail safely.

### Multi-target market protocol

get_market_data with targets takes 1–100 distinct instrumentId OR complete
oriented baseCurrency/quoteCurrency), view=current|coverage|history (current
default), range=30d|ytd|1y|all (30d default), sourceFilter=all|manual|provider|agent,
sort={field:targetKey|displayName,direction:asc|desc} and limit. Optional
historySort chooses quotedAt/effectiveDate and direction; points default
quotedAt asc, observations quotedAt desc, then createdAt/quoteId asc.

Alternatively supply paired `fromDate` / `toDate` (`YYYY-MM-DD`, inclusive).
An explicit `range` cannot accompany them. Partial, empty, invalid and reversed
bounds fail validation. Daily observations use their stored effective market/NAV
date; observations without an effective date use instants bounded by the
household history timezone's local midnights, including DST transitions. The
existing domain quote window owns these rules. An absent history origin uses
UTC; an invalid configured timezone fails the capture. Custom bounds are echoed
in query, summary and coverage; legacy named-range calls keep their prior shape.

`historyLayout=records` explicitly opts a history request into unified pagination.
The default `per_target` layout and existing cursors remain compatible. The new
layout has only `targets` and `records` sections, initially returning records as
space permits, then target metadata. Initial limit defaults to 100 in this layout;
explicit limits and continuation limits retain the 1–100 bound (continuation
default 25). There is no promise that 233 records fit one response. Targets follow
`sort`; within each target records follow `historySort` (default quotedAt asc,
createdAt and quoteId asc ties). One signed records cursor continues across all
targets. Each active observation is stored once; `chartSelected` identifies
membership in the existing selected chart points. It does not assert current
valuation selection, preferred-source eligibility or import equivalence.

Each record includes target identity, quoteId, civil `date` and `dateBasis`,
quotedAt/createdAt, observationKind, priceBasis, timestampBasis, exact decimal
`value`, currency, sourceKind/sourceKey, revision and chartSelected. Storage
observation kind alone cannot distinguish NAV from close: an Agent unit NAV has
`observationKind=close`, `priceBasis=agent_unit_nav_v1`. Preserve the basis.
For FX, `baseCurrency`, `quoteCurrency` and `value` describe the **original stored
direction**. `target` retains the requested direction; `displayValue` is its
existing, potentially inverted and rounded chart projection. It must never be
compared as an original exact FX rate. Missing raw FX evidence fails the capture.
The unified layout also verifies every original FX quote survived the existing
chart projection. If a reciprocal is not representable within FxRate bounds,
the entire capture fails with `unavailable`, including when every requested
history observation would otherwise disappear. Query the original direction or
narrow the window; this error must never be interpreted as missing quotes.
Provider and Agent sources remain distinct. Conversion evidence is retained.

`historyStatus` on target metadata distinguishes `not_found`,
`no_quotes_in_range`, and `available`, independently of current-price status.
The capture is complete at creation, but a client has not read all evidence until
both relevant section cursors finish. A zero-item deferred page is not evidence
of missing prices. `evidenceScope=active_observations_only` excludes withdrawn or
superseded facts; use Agent audit history for those historical facts. This read
cannot establish import absence or compute a safe write precondition. The
[implemented import comparison API](../design/mcp-import-comparison-api.md) specifies
the separate implemented tool, including inactive evidence and exact classifications.

The unified layout uses the same permission generation, fixed read-only SQLite
snapshot, content hash, HMAC-bound cursor, five-minute TTL, response byte budget,
4 MiB capture, cache/build and row limits as other `mcp-read/1` captures. It does
not store a second points copy or per-target history sections. `contentHash`
identifies captured content; it is not a CAS token for a subsequent mutation.

All metadata/preferences/current/history facts come from one read-only transaction
with a frozen clock/provider choice. Existing selectors, active correction
predicates, range and oriented FX/chart-point logic are reused. current returns
no historical arrays. coverage includes first/last, observations/selected-points
counts, outsideRange and gapAssessment=not_evaluated; it does not invent trading
day completeness. history returns paged target summaries with lazy entries for independent
<targetKey>/points and <targetKey>/observations sections. Unknown instrument IDs
remain per-target not_found; missing quotes remain missing with reasons. A target
cursor cannot be used for another target. No provider/network/configuration,
quote import, snapshot rebuild or repair occurs. Agent audit retractions retain
target observation identity for filters; active means not corrected/retracted,
independent of valuation selection. Civil observation bounds differ from RFC3339
creation timestamps.

### Atomic mutation protocol

All four new writes take {operationId,input} as one business call, 1–100 distinct
items. No best-effort mode or generic arbitrary executor. Before a business write,
validate all target IDs, changes, protections, versions/revision and worst-case
short receipt wire budget. Success returns an ordinary durable outer execution
receipt; pending/unknown is conservative and never automatically replayed.
Identical successful operationId/input retries return the original receipt;
changed input conflicts. Verify get_operation and fresh state after an unknown.

update_instruments_batch requires directory_write or ledger_write. Items are
{id,expectedMetadataVersion,changes:{name?,shortName?,note?}}. Omission preserves;
empty shortName clears; note=null clears; name cannot be empty/null. No other
fields or empty changes are accepted. The version hashes id/name/shortName/note
and archived/managed protection. The transaction rechecks all versions and
protections, then executes metadata-only SQL for all changed items. Other
instrument fields, source preferences, quotes and ledger facts are retained.
Managed/archived targets reject every item. The short result has requestedCount,
changedCount and items {id,changed,metadataVersion}.

### Remaining scale boundaries

Individual unusually large notes/provenance or nested source evidence can still
exceed the single-item budget; the server fails explicitly without truncation.
Capture size and computation limits are separate from page size.
Existing financial-context
aliases/disclosure, activity/contribution pagination, live product-operation
history, ledger batches/reconciliation and market imports remain their own
contracts. Ordinary liquidity policy batching and arbitrary detail batching are
outside this change.

## Account reconciliation sessions

See [the session architecture](account-reconciliation.md) and
[the user workflow](../user/account-reconciliation.md). Session evidence reads
are available in all permission modes; attestations require ledger_write.
Headers and pageable account/item/attempt/change/link evidence are separate.
Paged reconciliation evidence defaults to 25/max 100 rows and shrinks to fit
65,536 complete JSON-RPC bytes, including both SDK representations. Preserve
stale_cursor on changed revision/evidence; too_large identifies a single item
that cannot fit. No evidence is truncated or silently skipped. Session writes retain
preview/commit, durable receipt-first recovery and user-provided provenance.
Existing financial target reconciliation keeps its original meaning. Optional
reconciliationLink on ledger target/change and managed-product previews is part
of the immutable request hash and owning financial transaction. A correction
never confirms a check. Current repository skill 3.0.3 documents both routes, partial
completion, source precision and same-operation recovery.


Reconciliation follow-up: session start note and finish explanation are distinct
note/closingNote fields. referenceComparable explicitly identifies comparable
current-day numeric values; mismatches require explanation or correction.
Changes expose afterCompletion separately from the frozen original interval.
Private production plans use SQLite, and missing pending bodies remain unknown.
An explicit same-ID recovery can inspect session/adjustment business receipts;
unknown IDs never execute. See the maintained reconciliation and recovery Skill
references for continuation, concrete target links and export boundaries.

## Historical read v2 and compact account valuations

Historical overview and series now return `data.schemaVersion=mcp-read/1` and
`summary.responseVersion=historical-read/2`. This is a breaking response-shape
change: migrate direct `data.rows` / `data.overview` / `data.points` consumers to
`data.summary` and `data.sections`. There is no legacy historical mode. Dates,
financial calculations and the GUI contract are unchanged.

Initial historical responses default to five items per section, selected after
measuring SDK duplication; explicit limit is 1–100. Continuations default to 25.
`view=compact` is default; `view=evidence` preserves full row evidence on pages.
Both use the existing five-minute frozen captures and signed `get_read_page`
cursors. Default rows are leaves. Every row has `id`, `aggregation` (leaf/rollup),
account/holding/instrument identifiers and per-side `countedLeaf` /
`pendingContribution` plus effective `displayValuation`. Only included leaves
contribute to pending; price and FX carried on the same leaf count once.

`filter` supports accountIds, instrumentIds, kinds (account/holding/balance/cash),
included, quality (pending/missing/provisional/verified), reasons (display quality
reason codes or strict missing-input codes) and side (left/right/either). Right requires a comparison. Arrays
are OR, fields are AND, and all cell predicates must match the same side.
Filters select endpoint rows; `totalsScope=household` and series
`pointsScope=household` explicitly retain household financial scope.
`matchedLeft`/`matchedRight` describe the entire filtered set, independent of
page size; `rowDetailScope` also permits household category missing-row
references excluded by that list filter to be fetched from the same capture.
These extra item reads do not change the filtered counts. Summing included leaf pending contributions across its pages equals
those pendingCount values. `totalRowCount`, `totalLeafCount`, matchedRowCount and
pointCount distinguish household, filtered and time-point cardinalities.

`get_read_item` accepts the captureId, section and id:

- `rows/<row id>`: full row with exact adopted price/FX values, source/time/policy
  evidence and missing details. Compact rows keep provenance/date/freshness.
- `points/<YYYY-MM-DD>`: complete point header and a signed entry to the
  independent `missing/<date>` section; page it for all missing-row evidence.
- `categories/<category id>`: category header and separately pageable missing
  row identities, including large incomplete categories.
- `overview/endpoints`: signed categories/contributions entries; these retain
  full household bridge scope. `states/left` and `states/right` retain allocations.

Series points keep amounts, coverage, gap/scope/basis/connect flags and dates;
full Overview and per-point missing arrays are not embedded. Metadata is shared
once. No missing values, financial precision or evidence are discarded.
Effective freshness is valuation policy, including manual instruments with Agent
observations and valid closed-day carry. `observationFreshness`, when present,
is raw diagnostic age. It does not create stale_observation gaps. FX evidence is
independent. Informational gaps cannot identify a pending asset by themselves.

`get_account_valuations` accepts optional accountIds and limit with existing account filters.
Compact `accounts` rows hold exact totals/completeness/counts. Read
`get_read_item` with section=accounts and id for full account/ownership detail and
separate component, missing-input, display FX and composition-layout sections.
The GUI's ranked details, type grouping and slices are not repeated in a list row.

All new pages/items preserve the complete 65,536-byte wire ceiling, including
SDK text, structuredContent and JSON-RPC envelope. Computation bounds and the
4 MiB capture limit are separate. Empty matches return explicit zero counts.
Oversized single items return deterministic too_large/single_item; narrowing the
query is required, and reducing page limit cannot fix an oversized single item.
See [response review](mcp-response-review.md) for the complete inventory and
synthetic byte/coverage evidence.

Directory `list_members`, `list_institutions`, `list_groups`, `list_accounts` and
`list_holdings` accept limit (default 25/max 100) and existing filters. Paginated rows
sort by persistent id. `get_read_item` with the corresponding section and id
retains notes/metadata omitted from compact rows. Existing list_accounts.filter
selects rows before paging.

Health reads `scan_data_health` and `preview_data_repair` accept limit and return
bounded captures: issues for scans; items/unresolved for previews. Header counts
describe the entire capture. Domain issueCount/range counts retain their meaning.
`get_data_repair_job` defaults to compact progress without a scan or capture.
Its exact-job scalar getter holds the existing job lock, reads evidence counts
without cloning arrays and detaches the optional timestamp; allocation does not
grow with retained job evidence. Full GUI/evidence snapshots retain their
defensive copies;
`view=evidence` freezes job_items/job_blockers/job_prerequisites only. Neither
view includes health. Polling does not displace useful captures; continuation
does not rescan or extend TTL. A separate health scan after terminal outcome
checks current gaps. Submission receipts, provider permissions and unknown-job
recovery remain unchanged.

`get_overview` accepts the existing account filters and limit.
It preserves the complete current GUI overview: exact monetary values, complete,
displayValuation and historyStarted in the summary; independent missingInputs,
assetsByType/liabilitiesByType, byMember/byInstitution/byGroup/byAccountType,
recentActivities, accountLabels, instrumentLabels and holdingLabels sections.
Initial rows: floor of five per section, grown to at most 25 within a 24 KiB response budget when limit is omitted (see mcp-pagination.md), continuation default 25/max 100; all output
uses mcp-read/1. totalsScope=request_account_filters. Counts describe complete
section cardinalities. Use get_financial_context for a minimal-disclosure summary and adopted evidence.
A 200-account regression fetches every section and compares every field with the
application overview; no GUI DTO or valuation arithmetic changes.

### Additional compact read contracts

get_context returns household metadata, history origin, identity, permissions and
names of directory tools. Names/notes for members, institutions and groups are
retrievable through those tools instead of being embedded in bootstrap.
get_catalog is a small static vocabulary and retains its direct object shape.
get_account_snapshot returns the same account capture as get_account_valuations,
filtered to its required id; get_read_item accounts/id opens all evidence sections.
get_market_data returns only the selected current quote when called with
instrumentId or a currency pair. The same tool returns the batch capture when
called with targets (view=current/coverage/history). The two inputs are mutually
exclusive, and the output schema is a oneOf of `{current}` and the read envelope.
No history is discarded from the batch shape.

analyze_period returns mcp-read/1 with four projection headers in summary and
income/rows, expenses/rows, assetChange/waterfall, assetChange/groups,
investmentReturns/points and investmentReturns/sources sections (initial rows as for get_overview).
list_contributions freezes rows and detail together; get_contribution_item exposes
rows/<key>/components and rows/<key>/byAccount; get_return_day pages composition,
contributors/issues; get_asset_driver_detail pages byInstrument/byAccount/residualDetails.
These use the same capture/hash/cursor/TTL infrastructure. They retain analysis
snapshot maintenance and non-read-only annotations; execute before write previews.


### Compact management lists and financial continuations

Directory and instrument lists now use explicit identity projections, rather
than serialized GUI records with a few fields deleted. Members/groups retain
ID/name/archive status; institutions add type/country. Accounts retain their
nested account identity, classification, currency, lifecycle, inclusion and
ownership plus institution/group labels; latestValue is available only in full
detail. Holdings retain account/instrument IDs, exact quantity and archive status.
Instrument rows retain identity, currency/unit, quote routing, managed/archive
status and metadataVersion. Notes, display metadata, household IDs and internal
timestamps are read through same-capture get_read_item. Use section=instruments
for instrument details; full details retain the same version and managed guard.
Sorting/filtering occurs before projection and cursoring. Future GUI fields do
not automatically expand these lists. Complete replacement writes need full
metadata; compact-field omission does not clear a stored value.

Financial continuations through get_read_page (the removed
get_financial_context_page and get_financial_comparison_page aliases returned
the same shape) return only
contextId/comparisonId, contentHash, capturedAt, cacheExpiresAt, section, items
and page. The original content header, attribution/precision and the other
sections' progress are not repeated. Clients save the initial header, append
items to section, and update that section's progress from page. Identity/hash,
fixed TTL, generation revocation and exact row content remain authoritative.
Initial capture and get_financial_context_item response shapes are unchanged.
This is a breaking continuation contract, documented by current repository skill
3.0.1; there is one implementation and no legacy response option. Full wire
sizing still counts both SDK representations and the final JSON-RPC envelope.

### Reconciliation read wire budget

`list_reconciliation_sessions`, `get_reconciliation_session`,
`get_reconciliation_session_page` and `get_reconciliation_item` enforce the
complete 64 KiB HTTP JSON-RPC budget, including the SDK text and structured
representations. A requested page limit is an upper row bound: pages shrink
before response construction to fit both representations and envelope allowance.
Follow `nextCursor` even when fewer than the requested rows are returned.
Cursors remain live, content/revision-bound; `stale_cursor` requires restarting
that traversal. A single oversized evidence item returns `too_large` explicitly;
no evidence is truncated. Preview/commit responses and operation recovery
receipts retain their existing behavior and are not intercepted after execution.


## Bounded discovery and task-directed reads (skill 3.0.1)

The SDK tools/list page size is 16. Follow every nextCursor and cache the complete
tool inventory/schema for the connection. The complete HTTP discovery response
is capped at 64 KiB; total discovery size is measured separately from per-page
size. Common guidance lives in server instructions/skill, with concise
high-frequency tool and operationId descriptions. Structural schemas enforce
activity cursor triplets, market target alternatives and paired date windows,
records/history dependence, repair views and ledger-kind required inputs. Exact
decimal/date/identity/permission/CAS validation remains in the business layer.

Production HTTP read response policy derives from registered capability effects
(read, analysis, preview), including SDK pre-handler validation. Write effects and
get_operation always bypass response interception: a post-execution receipt must
never be replaced by a read-size error. In-process reads also reject oversized
successful values; this does not apply a model-token quota or trim receipts.

Activity, review and product-operation pages use byte-aware authoritative
requeries under one application operation fence; cursor positions advance after
exactly delivered rows. Single complete rows/groups and headers are not split or
truncated. Review publication cursors retain their revision/filter checks.
Activity/product history and rule/suppression lists remain live reads. Rules and
suppressions return items/page: continue every nonempty page via its signed
nextCursor until an empty page, including short pages. They do not gain frozen-audit guarantees.

Known-ID reads require neither context nor catalog calls. Initial financial
summaries answer summary questions without downloading every detail section;
complete lists, absence claims and audits require complete relevant evidence.
Follow the requested scope, preserve frozen identity/disclosure/quality and state
what evidence remains deferred. This changes fetch guidance, not financial
calculations or safety authority.


`preview_investment_review_rule` preserves the complete policy window in a frozen
mcp-read/1 capture: sourceToken/latestClosedOn/window stay in summary; days are
pageable (default 25/max 100). It evaluates the original requested policy once,
never shortens a 365-day window to meet the wire cap, and never publishes rules,
alerts, daily evidence or derived valuations. Expiry/revocation requires a fresh
preview capture. expectedSourceToken remains a source-change guard, not write CAS.


## Typed read contracts and optional task discovery

Shared mcp-read/1 success envelopes and financial package/page headers have
explicit outputSchema, as does progress-versus-evidence job reading. The SDK and
shared registry validate these shapes. Nested business payloads remain owned by
the existing exact-value DTO contract; this is not a claim to describe every GUI
field. No write or get_operation receipt receives a new post-execution output
gate. Discovery baseline tests isolate this intentional output-schema change.

Safe external tool errors retain isError and original code/field/message text;
result metadata `_meta["nestworth.errorRecovery"]` additionally carries mcp-error/1 with error and conservative
recovery action/automaticRetry/preserveIntent/discardCapture fields. Hints never
authorize replay or turn unknown execution into failure/success. AutomaticRetry
is false. Protocol/auth errors may remain outside this tool-error shape. Success
and recovery receipts remain unmodified values.

Clients can optionally pass tools/list `_meta.nestworth.toolGroups` as a nonempty
array of context/summary/analysis/directory/ledger/market/products/review/
reconciliation/health names. Discovery includes only selected categories plus
shared get_read_page/get_read_item/get_operation; schemas and annotations match
full discovery exactly. Registered permissions and tools/call enforcement do
not change. The extension is not a household scope or authorization boundary.
Default discovery stays complete. Clients that hide tools/list metadata cannot
be assumed to use it; no automatic model-tool reduction is claimed for them.

Filtered tool pages remain 16 items maximum. HMAC cursors bind normalized group
set and running server; retain identical metadata throughout pagination. Different
groups, tampered/full-list cursors and restarted servers require fresh discovery.
Metadata is built once per server, with no household/provider reads or mutable
cross-client profile. Selected requests and wire bytes are measured with the
production SDK; this is not model-token evidence.

SDK success responses continue to contain both structuredContent and JSON text.
The typed SDK constructs the text fallback when structured output is present.
Tests verify equal exact JSON payloads and count both copies plus JSON-RPC
escaping/overhead. The built-in Eino adapter receives one direct capability JSON
string (engine.go FunctionToolResult), not an external MCP envelope. External
client/model consumption is outside the server's observation; no real external
model-token or token-savings claim is made. Removing text without client evidence
could remove the representation a client actually consumes, so both are retained.


Recovery hints live in result `_meta`, not structuredContent: the latter is the
success outputSchema surface. See the [MCP tool output contract](https://modelcontextprotocol.io/specification/2026-07-28/server/tools).
This avoids presenting an error object as a successful read envelope. A client
can consume metadata programmatically; whether it forwards metadata to a model
is not observed or assumed. Original isError/text code/field/message remain.

## Unified business pagination

See [the pagination inventory and contract](mcp-pagination.md) for the authoritative
parameter, response, consistency, budget and recovery rules. Every business
continuation uses get_read_page with only cursor and optional limit (25/max 100).
Live/version-bound initial responses use items/page, while frozen initial sections
and financial disclosure headers keep their owning structure. No caller builds
triple activity keys, product cursors or last-ID review cursors. Both external
MCP and built-in AI use the same capability projection and routing. The MCP
implementation version is 3.0.0; the distributable skill is independently 3.0.3
(unpublished; see the
[skill version policy](../user/nestworth-skill.md#skill-version-policy)).

## Discovery size and the external contract snapshot

Every client pays the `tools/list` payload and the server instructions on
connection, so both are bounded by tests rather than by convention:

- `discovery_contract_test.go` compares, per permission mode and tool, a hash of
  the name, annotations, input schema and output schema against
  `testdata/discovery-contract.json`. Description prose is excluded, so guidance
  can be shortened without blessing a changed argument, permission or result
  shape. The golden file changes only by hand, as a reviewed contract change.
- `discovery_budget_test.go` bounds the serialized `tools/list` size per mode,
  the longest tool description and the instructions. Raising a limit needs a
  stated reason.

Boilerplate that repeated in every tool (permission requirement, receipt
semantics, operationId retry rules, analysis snapshot side effects) lives once
in the server instructions; the helper constructors no longer append it.

Slimming measurement (real `tools/list` JSON from `TestDiscoveryBudget`,
before -> after, bytes; "after" includes the exact `delivery` output property (both fields required, string
array) added to 28 read tools, which costs about 6.6 KB per mode; before it the
reductions were -7.2%, -7.9% and -8.6%):

| Mode | Tools | `tools/list` | Descriptions | Instructions |
| --- | --- | --- | --- | --- |
| `read_only` | 55 -> 53 | 108,144 -> 107,005 (-1.1%) | 22,274 -> 16,842 | 3,111 -> 2,313 |
| `directory_write` | 74 -> 72 | 128,513 -> 124,976 (-2.8%) | 26,377 -> 18,547 | 3,111 -> 2,313 |
| `ledger_write` | 104 -> 102 | 187,634 -> 178,055 (-5.1%) | 43,889 -> 30,017 | 3,111 -> 2,313 |

These are wire bytes, not model tokens. Output schemas then dominated
(roughly 45 KB per mode, mostly the `mcp-read/1` envelope repeated per tool);
Phase 3 below made them compact, a contract change with a stated cost.

### Paired scenario measurement (headless, synthetic data)

Measured with the production Streamable HTTP server and the Go MCP SDK client
over a loopback transport (`payloadRecorder`: complete JSON-RPC response bytes,
including structuredContent, text fallback and escaping), on a synthetic ledger
fixture (`MCP ledger`, USD base, one member). Before = `4eaafa6e` (main), after =
this branch, same call script, one run each (the byte counts are deterministic;
there is no timing claim). The scripts follow the documented skill examples.

| Scenario | Calls before -> after | Bytes before -> after |
| --- | --- | --- |
| Discovery (`initialize` + `tools/list`, 7 calls, ledger_write) | 7 -> 7 | 189,510 -> 175,925 (-7.2%; measured before the exact `delivery` schema, which adds about 3.9 KB per mode) |
| 1. Current net worth (`get_financial_context`) | 1 -> 1 | 9,038 -> 9,036 |
| 2. Record a buy (income + buy previews/commits, holdings read) | 5 -> 5 | 21,183 -> 21,307 (+0.6%) |
| 3. Balance reconcile (preview + commit) | 2 -> 2 | 7,002 -> 7,002 |
| 4. Import fund NAV (create instrument, import, read, scan) | 4 -> 4 | 6,506 -> 6,630 |
| 5a. `get_overview`, every section | 1 -> 1 | 17,285 -> 17,411 (small fixture: already complete) |
| 5b. `analyze_period`, 28 days, every section | 2 -> 2 | 17,680 -> 17,864 (28 points exceed the 25-row cap) |
| 5d. `analyze_period`, 20 days, every section | 2 -> 1 | 14,768 -> 13,029 (-11.8%) |
| 6. Data repair (scan, preview, start, poll) | 4 -> 4 | 7,666 -> 7,916 |

The small per-response increase (about 125 bytes) is the `delivery` object, which only top-level initial captures carry. Gains
appear only when a result has between six and 25 rows per section; with small
fixtures or more than 25 rows the call count is unchanged.

Not measured, and therefore not claimed: native macOS app behavior (the
`com.nestworth.dev` isolated QA flow was not run), real LLM client behavior and
model tokens (wire bytes are not tokens), and the effect of the shorter skill on
how many calls a real agent makes.

The Phase 3 consolidations are described below and in
[mcp-consolidation-design.md](mcp-consolidation-design.md); the update-convention
audit is in [mcp-update-conventions.md](mcp-update-conventions.md).

### Phase 3: icon merge, `commit_plan` and compact output schemas

Three changes remove 13 tools and cut discovery by about 40%. They are breaking
for clients that call the removed names; actual discovery is the authority.

| Change | Before | After |
| --- | --- | --- |
| Icons | `set_member_icon`, `set_institution_icon`, `set_group_icon` | `update_member`, `update_institution`, `update_group` take `{id, name?, iconKey?}`; at least one field; both apply in one SQL statement (`application.EditMember` and siblings); empty `name` or `iconKey` is invalid |
| Commit | ten per-kind `commit_*` tools | one `commit_plan({operationId, input:{planId}})` |
| Output schema | fully typed `mcp-read/1` envelope per tool | compact envelope (below) |

`commit_plan` reads the plan kind from the stored plan; a caller can neither
choose nor mislabel it. Routing is `commitPlanRoutes` in `commit_plan_tool.go`:

- The durable receipt keeps the historical per-kind tool name (`commit_change`,
  `commit_batch`, ...). Its hash, the replay and conflict rules, the host-review
  binding and `get_operation` therefore keep their meaning, and a receipt written
  before the change replays under the new tool.
- Ledger and managed-product kinds replay a pending or unknown receipt;
  liquidity-policy kinds never do. Both follow the original per-kind choice.
- A pruned plan file is routed from the operation receipt, then from the
  reconciliation database receipts, so receipt-first recovery still works. With no
  evidence the error is `operation_outcome_unknown`, as before.
- `operationId` is optional only on `commit_plan`. When it is omitted or blank,
  the server derives UUID v5 (SHA-1) of the canonical lowercase `planId` in
  namespace `f1f65af0-bbb8-5d57-af9c-b624de715dcb` (itself UUID v5 of
  `nestworth:mcp:commit_plan:operation` in the RFC 4122 URL namespace). The
  receipt hash still covers the submitted `input` text, so a retry must repeat
  that `planId` spelling. An explicit `operationId` is unchanged.
- If that derived id already has a receipt for a different tool or different
  input, the call returns `conflict` and does not post or replace the receipt.
  The same explicit id with the same input replays that receipt, including a
  pending or unknown liquidity receipt, which is still not replayed.
- A different explicit `operationId` for the same `planId` is unchanged: ledger
  and managed-product plans still deduplicate by `planId`; liquidity save/reset
  still return `stale_preview` for a new operation id on a used plan.
- A `planId` that is not a UUID or has an unknown kind is rejected before any
  receipt is written, so it does not consume the `operationId`. An omitted
  `operationId` with an invalid `planId` is the same validation failure and
  writes no receipt.
- Staleness and version checks still run under the shared write permit inside
  each family's commit function; nothing about them moved.
- A plan of another family can no longer be sent to a wrong tool; the earlier
  "wrong commit tool" validation errors do not exist any more.

Output schemas. The wire schema of every `mcp-read/1` tool now requires only
`schemaVersion` (const), `captureId`, `expiresAt` and `contentHash`; page tools
require `items` and `page.hasMore`/`page.nextCursor`; financial headers require
their id, `contentHash` and `content`. The strict contract (field types, the
summary/page exclusion, `delivery`, frozen-page counters) is kept in
`output_schemas_full_test.go` and real responses are validated against it
(`TestDeliveryOutputSchemaMatchesResponses`). Cost of the change: the server no
longer enforces those details at run time, and a client that validates
`structuredContent` against the discovered schema no longer rejects, for example,
a non-boolean `hasMore`.

Measurement (real `tools/list` JSON from `TestDiscoveryBudget`; before = the
Phase 1/2 numbers in the table above):

| Mode | Tools | `tools/list` bytes |
| --- | --- | --- |
| `read_only` | 53 -> 53 | 107,005 -> 65,160 (-39.1%) |
| `directory_write` | 72 -> 69 | 124,976 -> 82,029 (-34.4%) |
| `ledger_write` | 102 -> 90 | 178,055 -> 125,943 (-29.3%) |

Wire bytes, not model tokens. Not measured: native macOS behavior, real client
token use, or whether agents make fewer calls with one commit tool.

## Client-specific discovery notes

These notes were moved out of the distributable skill, which is loaded into
agent context, because they concern client configuration and protocol
parameters rather than ledger workflows.

### Inspector CLI configuration

Inspector CLI 2.10.0 requires its own explicit Streamable HTTP type. Add
`"type": "http"` to the App-copied entry; a missing type can be interpreted as
stdio and fail before connection. This is Inspector-specific, not a requirement
for other clients. The format follows the Inspector's
[server configuration format](https://github.com/modelcontextprotocol/inspector/blob/main/docs/mcp-server-configuration.md).

```json
{"mcpServers":{"nestworth":{"type":"http","url":"${endpoint}","headers":{"Authorization":"Bearer ${token}"}}}}
```

Replace the endpoint and token locally; placeholders are not expanded. Keep the
file private and out of reports. Then list tools with:

```bash
npx @modelcontextprotocol/inspector@2.10.0 --cli --config nestworth-inspector.json --server nestworth --method tools/list
```

### Task-scoped discovery protocol parameters

The following are `tools/list` protocol parameters, not business-tool input:

```json
{"_meta":{"nestworth.toolGroups":["summary"]}}
```

Valid groups are context, summary, analysis, directory, ledger, market,
products, review, reconciliation and health. Keep the same metadata on every
`nextCursor` request; cursors are signed and bound to the normalized group set
and running server. Changing groups, reconnecting to a restarted server or using
a full-discovery cursor starts from the first page. The filter is not an
authorization boundary, and a group cannot reveal tools absent from the current
permission mode. The behavior is covered by `task_tool_discovery_test.go`.

### Directory archive folded into update

`archive_member`, `archive_institution` and `archive_group` are removed.
`update_member`, `update_institution` and `update_group` take optional
`archived`: omit it to keep the stored state, `true` archives and `false`
restores. Name and icon stay partial fields and still apply in one statement.
The call still requires `directory_write`, still returns one receipt, and still
enforces the member archive rules (at least one active member, and a member
who is the only active owner of a live account cannot be archived). A rejected
archive does not apply a name or icon sent in the same call. `archive_account`
and `archive_instrument` are unchanged.

Measured `tools/list` bytes on this change, same fixture as
`TestDiscoveryBudget`: read_only stayed 53 tools / 65160 bytes; directory_write
69 → 66 tools and 82029 → 80583 bytes; ledger_write 90 → 87 tools and
126422 → 124976 bytes. Budgets were not raised. Skill reference text names the
folded tools; `skills/nestworth/VERSION` is left for the coordinator's PATCH.

### Historical overview and market data merged

`get_historical_overview_series` and `get_market_data_batch` are removed.

`get_historical_overview` takes one of two inputs, enforced by input `oneOf`.
`{date, compareTo}` is one closed endpoint, with an empty date selecting the GUI
default month end and an optional second closed date or `current`. `{from, to}`
is a forward series of 1–31 closed dates. Do not combine the shapes. Both
results stay `mcp-read/1` envelopes. A series adds household points and does not
nest a full overview. Endpoint movement is still not a return.

`get_market_data` takes one of two inputs, also enforced by input `oneOf`. One
`instrumentId` or an oriented currency pair returns `{current}` and no history.
`targets` (1–100) returns the previous batch capture: `view` current, coverage
or history, named range or inclusive `fromDate`/`toDate`, per-target sections or
`historyLayout=records`, and original FX `value` versus oriented `displayValue`.
The output schema is `oneOf` `{current}` and the compact read envelope. Batch
paging, the 1–100 target budget and evidence fields are unchanged.

Measured `tools/list` bytes with `TestDiscoveryBudget` after this merge, on top
of the directory-archive fold: read_only 51 tools / 63796 bytes; directory_write
64 tools / 79219 bytes; ledger_write 85 tools / 123612 bytes. Against the fold's
measured sizes that is −2 tools and −1364 bytes in every mode. The same −1364
byte delta was measured for this merge before the fold. Budgets were not
changed. Skill reference text names the merged tools; `skills/nestworth/VERSION`
stays 3.0.4 for the coordinator.

### E2 large input schemas

Measured with `TestDiscoveryBudget` on this branch after the directory-archive
fold and the historical/market merge. The two input schemas are `jsoncodec`
bytes of the published tool schema. Tool counts did not change. `read_only`
stayed 51 tools / 63796 bytes and `directory_write` stayed 64 tools / 79219
bytes. Budgets were not changed. `skills/nestworth/VERSION` was not bumped.

| Schema | Before | After |
| --- | --- | --- |
| `preview_product_operation` input | 7,609 | 5,396 |
| `preview_change` input | 3,221 | 2,516 |
| `ledger_write` `tools/list` | 123,612 | 120,694 |

`preview_product_operation` keeps one `productTerms`, `productPolicy` and
`productSettle` definition and points the payloads at them. Its kind `oneOf`
names the required payload and rejects the other payload keys. It does not copy
terms, policy or settle. The kind sentence was already in the tool description
(792 bytes, unchanged) and is no longer repeated on the schema.

`preview_change` already had a kind `oneOf` of required names only (1,320 bytes
on the 3,221-byte schema, no field schemas) and no description keys. Plain
string properties now share one string schema plus a name allowlist. A per-field
`$ref` is longer than `{"type":"string"}`, so it was not used. `added`,
`mutationId`, `newValue` and `newValueCurrency` are omitted because the handler
rejects them for every supported kind. Each kind's required names are unchanged.

### Nullable input schemas

Input schemas no longer publish nullable type unions (`"type": ["null", ...]`).
Optional pointers and slices decode JSON null the same as omission, so those
fields are a single type and callers omit them. Required pointers, `shortName`,
and `update_instruments_batch` `note` keep null through `anyOf`. Measured
`tools/list` bytes with `TestDiscoveryBudget` after that rewrite, on top of the
schema slimming above: read_only 51 tools / 63022 bytes (−774);
directory_write 64 tools / 78083 bytes (−1136); ledger_write 85 tools / 120426
bytes (−268). Published input schemas moved with the same rewrite:
`preview_change` 2,516 → 2,498 and `preview_product_operation` 5,396 → 5,855
(`anyOf` on the shared required pointers). Budgets were not raised.
`skills/nestworth/VERSION` was not bumped.

