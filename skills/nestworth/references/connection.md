# Connection, permissions and tool map

Open Nestworth **Settings → AI / MCP**, choose a permission mode and enable MCP.
Copy the App's connection configuration into this local Codex client's MCP
settings. It supplies a Streamable HTTP URL and an Authorization bearer header;
use the exact values. Keep the App running. Do not put credentials in this skill,
reports, scripts or example arguments. A cloud/remote client cannot reach the
App's loopback address without a separately configured relay.

Normal App restarts retain the port and token. Disabling MCP revokes the token;
enabling again or changing permission mode issues a new token, requiring updated
client configuration. Connection failures can mean a stopped App, stale token,
wrong instance, or occupied port. Ask the user to inspect App settings rather
than scanning databases or silently attaching to another instance.

After installing/updating the skill, reopen the Codex chat if it has not appeared
in skill discovery. Invoke `$nestworth` explicitly or describe a Nestworth task.
MCP configuration and skill discovery are separate.

## Inspector CLI configuration

Inspector CLI 2.10.0 requires its own explicit Streamable HTTP type. For this
client, add `"type": "http"` to the App-copied entry; a missing type can be
interpreted as stdio and fail before connection. This is an Inspector-specific
adaptation, not a requirement to change other clients' connection formats.
The following is **client configuration**, not a tool's arguments:

<!-- example: inspector-http-config -->
```json
{"mcpServers":{"nestworth":{"type":"http","url":"${endpoint}","headers":{"Authorization":"Bearer ${token}"}}}}
```

Replace endpoint/token locally with the App's actual values; placeholders are
not automatically expanded. Keep the file private and omit it from reports.
With that local file, discover tools using:

```bash
npx @modelcontextprotocol/inspector@2.10.0 --cli --config nestworth-inspector.json --server nestworth --method tools/list
```

Inspector may report a schema portability warning for nullable arrays such as
`accountIds` with `type: ["null", "array"]`. This is a valid nullable schema
also used by existing tools; a portability warning alone is not proof of missing
permission or an unavailable tool. Check discovery and actual call validation.
If a client cannot consume that schema, report the client compatibility problem
rather than changing disclosure or bypassing request validation.

## Permissions and discovery

| Mode | Available operations |
| --- | --- |
| `read_only` | Financial context and frozen pages, current valuation, managed products/history, liquidity and upcoming-action reads, daily investment-review reads, historical overview/series, catalogs, history, analysis, market-data/audit reads, health scans, repair previews/status |
| `directory_write` | All reads plus account/instrument/member/institution/group maintenance |
| `ledger_write` | All above plus ledger previews/commits, quote imports/source selection, reconciliation, corrections, daily investment-review rule/acknowledgement/suppression/refresh writes, managed lifecycle/terms/value pairs and starting data repair |

Discover all tools/list pages, following nextCursor even when a page is short.
Cache this connection's tool schemas rather than rediscovering before every call.
Discover the actual tools before choosing a workflow. An absent tool may reflect
permissions or an older App. Explain the specific needed setting/update; do not
attempt another tool to bypass it. `get_context.capabilities` is helpful, but
actual tool availability and schemas decide whether an operation is supported.
For a pure minimal summary, discovery followed directly by `get_financial_context`
is sufficient. Do not require `get_context`: it returns identity-bearing household metadata. Ordinary reads can call their tool directly. Use `get_context` only for needed
instance/permission/timezone/history metadata and `get_catalog` for unknown
vocabulary or entity creation; reuse both until their context changes. If the context tools are missing, explain the needed App
update; an identity-bearing fallback requires an explicit disclosure choice,
not a silent directory read. Skill updates do not update the App or MCP connection. Analysis tools, including
`compare_financial_attribution`, can maintain derived snapshots under the existing
read_only mode and advertise non-read-only, non-destructive annotations. They
do not grant new ledger permissions or certify old frozen comparison packages.

## Tool groups

| Area | Tools |
| --- | --- |
| Context and recovery | `get_context`, `get_catalog`, `get_operation` |
| Consistent financial summary and gaps | `get_financial_context`, `get_financial_context_page`, `get_financial_context_item` |
| Coherent comparison and attribution link | `compare_financial_attribution`, `get_financial_comparison_page` |
| Two-state differences, not returns | `compare_financial_context`, `get_financial_comparison_page` |
| Definitions | `list_members`, `list_institutions`, `list_groups`, `list_accounts`, `list_instruments`, `list_holdings` |
| Definition maintenance | `create_member`, `update_member`, `archive_member`, `set_member_icon`; `create_institution`, `update_institution`, `archive_institution`, `set_institution_icon`; `create_group`, `update_group`, `archive_group`, `set_group_icon`; `create_account`, `update_account`, `archive_account`; `create_instrument`, `update_instrument`, `archive_instrument` |
| Current state | `get_overview`, `get_account_valuations`, `get_account_snapshot` |
| Managed contracts/history and liquidity | `list_products`, `get_product`, `list_product_operations`, `get_liquidity_overview`, `get_upcoming_actions` |
| Daily investment review v2 | `list_investment_review_rules`, `preview_investment_review_rule`, `list_investment_review_alerts`, `get_investment_review_alert`, `get_investment_review_coverage`, `list_investment_review_daily_evidence`, `list_investment_review_episode_history`, `list_investment_review_episode_events`, `list_investment_review_suppressions`; ledger_write: create/update/archive rule, acknowledge alert, set/clear suppression, refresh |
| Read-only historical view | `get_historical_overview`, `get_historical_overview_series` |
| Instrument search | `search_market_instruments` (query is sent to the configured provider) |
| Managed lifecycle facts (ledger_write) | `preview_product_operation` / `commit_product_operation` |
| Ordinary access/withdrawal rules (ledger_write) | `preview_save_liquidity_policy` / `commit_save_liquidity_policy`, `preview_reset_liquidity_policy` / `commit_reset_liquidity_policy` |
| Managed terms/policy (ledger_write) | `preview_product_terms` / `commit_product_terms` |
| Actual locked-product value (ledger_write) | `preview_product_valuation` / `commit_product_valuation` |
| Single/batch ledger records | `preview_change` / `commit_change`, `preview_batch` / `commit_batch` |
| Reconciliation/correction | `preview_reconciliation` / `commit_reconciliation`, `preview_correction` / `commit_correction` |
| History and reports | `list_activities`, `get_activity`, `analyze_period`, `list_contributions`, `get_contribution_item`, `get_return_day`, `get_asset_driver_detail` |
| Quotes and audit | `get_market_data`, `get_market_data_batch`, `compare_market_data_import`, `list_agent_market_data`, `import_market_data`, `set_fx_source` |
| Data repair | `scan_data_health`, `preview_data_repair`, `start_data_repair`, `get_data_repair_job` |

Initialize the household and its history starting point in the App. In particular,
ledger recording requires started history; the MCP does not start/reset it or
choose opening balances/costs. This is distinct from importing a position later
in an already started history.

Except the explicitly labeled Inspector client configuration, JSON examples in
these references are **tool arguments**, not raw HTTP requests.
`${...}` denotes a value obtained from the user or a previous response. Replace
all placeholders; never send them as IDs. Values/dates are illustrative, not
verified market information. Reads/previews usually take arguments directly;
mutations take `operationId` and `input`. Use UUIDs from successful receipts,
not provisional IDs in a preview.

Managed capabilities are managed_product_read, liquidity_read and, only with
ledger_write, managed_product_lifecycle, managed_product_terms and
managed_product_valuation. Daily investment-review reads are available in all
modes; rule, acknowledgement, suppression and explicit refresh tools require
ledger_write. These update review rules/episodes and derived projections, not
financial facts.
Discover tools and verify the connected permission mode before acting.

## Scale capabilities and permissions

See [paging and batches](paging-and-batches.md). Discover bounded_read,
instrument_paging and market_data_batch. `get_read_page`/`get_read_item` and
bounded contracts are available in every permission mode. instrument_metadata_batch
and `update_instruments_batch` require directory_write or ledger_write.
Missing tools do not authorize filesystem/provider workarounds. Repository bundle updates do not install or enable a real client.

Reconciliation sessions expose reconciliation_session_read in every permission
mode and reconciliation_session_write only with ledger_write. Directory access
does not authorize ledger attestations. Discover the actual session tool names
before following [the session workflow](reconciliation-and-corrections.md#account-by-account-sessions-discover-support-first).


## Optional task-scoped tool discovery

Clients that expose MCP tools/list metadata can request a smaller discovery
set with `_meta.nestworth.toolGroups`. Valid groups are context, summary,
analysis, directory, ledger, market, products, review, reconciliation and health.
The same permission mode still governs every registered call. This is a discovery
filter, not an authorization boundary or a financial data scope. Shared
get_read_page/get_read_item/get_operation remain listed for evidence/recovery.

The following is **tools/list protocol parameters**, not business-tool input:

<!-- example: task-tool-discovery -->
```json
{"_meta":{"nestworth.toolGroups":["summary"]}}
```

Keep the same metadata on every nextCursor request; cursors are signed and bound
to the normalized group set and running server. Changing groups, reconnecting
to a restarted server or using a full-discovery cursor requires starting from
the first page. Cache the selected inventory for that connection/group set.
A group cannot reveal tools absent from the current permission mode. Without
this metadata, discovery returns the complete existing tool set. Clients that
hide tools/list metadata use full discovery and cache its schemas; this optional
extension is not claimed to run automatically in every external assistant.
