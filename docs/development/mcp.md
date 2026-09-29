# Local MCP integration

Owner: Nestworth application maintainers. Scope: the local desktop agent boundary.
Status: directory maintenance, single-record and atomic batch ledger preview/commit, existing-position imports, and period queries implemented; reconciliation remains planned. Native UI and external-agent
acceptance remain separate from automated integration tests.

## Delivery plan

The agreed scope is a complete business entry point: directory maintenance,
daily ledger operations, queries/analysis, and reconciliation/corrections.
Implementation is delivered in reviewable stages.

| Stage | Scope | Status |
| --- | --- | --- |
| 1 | Local transport, credentials, permission modes, directory/account/instrument maintenance, current queries, UI refresh, write receipts | Implemented; validation below |
| 2 | Income/expense, buys/sells, transfers/FX, dividends/debt, existing holdings, preview/commit, atomic batches | Implemented; ledger batches include first buys and existing-position imports |
| 3 | Period income/expense, asset changes, investment returns, contribution, historical detail and quality | Period summary and activity detail implemented; dedicated contribution tools pending |
| 4 | Target-state balance/quantity/cost reconciliation, pure cost correction, safe undo/fix | Pending |

Permissions are explicit and household-wide. `read_only` registers only queries;
`directory_write` adds directory mutations; `ledger_write` adds daily ledger
preview/commit and includes directory maintenance. Existing installations retain
their selected permission. Account creation still permits only an empty/zero
initial amount. Enable ledger permission explicitly to record subsequent funding.

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
and `position_import` (details below).
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
below when a group must succeed together. Position transfers/adjustments and
target-state reconciliation remain outside this tool set.

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

The additive `change_batch_mutation_keys` table follows the existing mutation
key repair path: opening an older writable database adds it without changing
existing facts or the schema version. Read-only verification accepts older schema-v13 backups without creating a
table; the existing restrictions on read-only v9–v12 backups are unchanged. Full SQLite backups preserve batch receipts;
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

## Later-stage acceptance

- Income/expense excludes own-account transfers and investment principal flows.
- Trades update settlement cash, quantity, fees/cost and realized gain together.
- Existing-holding import does not imply cash funding or a new purchase.
- Period queries resolve household history timezone and disclose coverage gaps.
- Reconciliation takes target quantity/total cost; it does not infer a trade.
- Pure cost corrections and batches combining directory creation with ledger
  writes require additional domain support.
- Preview plans bind to current data and reject stale concurrent edits.
- Validate against isolated databases and copied real data, never mutate the
  user's live ledger as an implementation test.

## Validation

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
