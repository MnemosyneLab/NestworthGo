# Local MCP integration

Owner: Nestworth application maintainers. Scope: the local desktop agent boundary.
Status: stage 1 implemented; later stages planned. Native UI and external-agent
acceptance remain separate from automated integration tests.

## Delivery plan

The agreed scope is a complete business entry point: directory maintenance,
daily ledger operations, queries/analysis, and reconciliation/corrections.
Implementation is delivered in reviewable stages.

| Stage | Scope | Status |
| --- | --- | --- |
| 1 | Local transport, credentials, permission modes, directory/account/instrument maintenance, current queries, UI refresh, write receipts | Implemented; validation below |
| 2 | Income/expense, buys/sells, transfers/FX, dividends/debt, existing holdings, preview/commit, atomic batches | Pending |
| 3 | Period income/expense, asset changes, investment returns, contribution, historical detail and quality | Pending |
| 4 | Target-state balance/quantity/cost reconciliation, pure cost correction, safe undo/fix | Pending |

Stage 1 intentionally does not advertise ledger, batch, historical analysis,
per-account permission, or interactive approval tools. Directory-write permission
is explicit authorization for the exposed individual directory mutations across
the household. Account creation permits only an empty/zero initial amount.

## Connect

Open **Settings → AI / MCP**, choose read-only or directory maintenance, and
click Enable MCP. Show/copy the connection configuration into a local MCP client.
It supplies a Streamable HTTP `/mcp` URL and an Authorization bearer header.
Clients with another configuration format should map those two values into their
own connection settings. Keep the desktop App running. Remote/cloud clients
cannot reach this loopback endpoint without a separately designed relay.

The first activation chooses an available IPv4 loopback port, then persists it
for stable reconnects. A port conflict fails closed; it does not pick another
instance's database. Disabling revokes the token; enabling again or changing
permissions generates a new one, so recopy the connection configuration.
Normal restarts retain the token and port. Configuration and operation receipts live in SQLite, including the token, port,
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
Successful MCP changes emit `agent.data.changed`, invalidating frontend queries.

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

## Write receipts and interruption semantics

Every write requires `operationId` (UUID) and a typed `input` object. Before the
business call, a private, fsynced pending receipt is saved. A duplicate UUID with
identical normalized typed arguments returns the original successful receipt;
changed arguments are rejected. Successful receipts include the returned object.
A terminal domain validation error is saved as failed. Unexpected failures or
interruptions leave an unknown outcome. They are never automatically executed
again: inspect data before deciding on a new operation. `get_operation` retrieves
receipts across restarts; Settings lists the latest 20 summaries.

This is conservative duplicate protection for individual existing operations,
not a transaction spanning receipt storage and the household database. A crash
after commit but before saving success can therefore yield unknown. Receipts are
execution history, not proof of current state (especially after a backup restore).
Do not reuse an old UUID for a new intent, including reapplying a restored-away
change. Stage 2 must provide an atomic database-backed batch/receipt boundary
before claiming all-or-nothing multi-operation support. There is no automatic
rollback or receipt expiry in stage 1; receipt storage grows with writes.

## Later-stage acceptance

- Income/expense excludes own-account transfers and investment principal flows.
- Trades update settlement cash, quantity, fees/cost and realized gain together.
- Existing-holding import does not imply cash funding or a new purchase.
- Period queries resolve household history timezone and disclose coverage gaps.
- Reconciliation takes target quantity/total cost; it does not infer a trade.
- Pure cost corrections and all-or-nothing multi-entity batches require domain
  support, not compensating MCP tool calls that leave intermediate states.
- Preview plans bind to current data and reject stale concurrent edits.
- Validate against isolated databases and copied real data, never mutate the
  user's live ledger as an implementation test.

## Validation

Backend integration tests use a real temporary SQLite household and the official
MCP HTTP client, covering discovery, permissions, directory mutations, idempotent
retries, restart receipts, credential revocation and HTTP boundary checks.
Frontend tests cover permission activation, configuration visibility and errors.
Native connection and an external agent session are separate manual gates.

Automated verification on 2026-09-29:

- `go test ./...`: passed, including desktop package compilation (macOS linker
  deployment-target warnings remain).
- `go test -race ./internal/mcpserver`: passed; listener restart coverage also
  passed five repeated race-enabled runs before the final additions.
- `go vet ./internal/mcpserver ./internal/wailsapi/agent`: passed.
- Frontend typecheck, lint, 70 test files / 534 tests, and production build: passed.
- Wails bindings regenerated from Go source; no generated files were hand-edited.
- Native UI, a configured external agent, live market-provider search, and
  release packaging/signing: NOT RUN. No live ledger was modified.
