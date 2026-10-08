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
| `read_only` | Financial context and frozen pages, current valuation, managed products/history and liquidity reads, catalogs, history, analysis, market-data/audit reads, health scans, repair previews/status |
| `directory_write` | All reads plus account/instrument/member/institution/group maintenance |
| `ledger_write` | All above plus ledger previews/commits, quote imports/source selection, reconciliation, corrections and starting data repair |

Discover the actual tools before choosing a workflow. An absent tool may reflect
permissions or an older App. Explain the specific needed setting/update; do not
attempt another tool to bypass it. `get_context.capabilities` is helpful, but
actual tool availability and schemas decide whether an operation is supported.
For a pure minimal summary, discovery followed directly by `get_financial_context`
is sufficient. Do not require `get_context`: it returns household, member,
institution and group names. Ordinary management still starts with `get_context`
and `get_catalog`. If the context tools are missing, explain the needed App
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
| Definition maintenance | `create_member`, `update_member`, `archive_member`, `set_member_icon`; corresponding institution/group tools; `create_account`, `update_account`, `archive_account`; `create_instrument`, `update_instrument`, `archive_instrument` |
| Current state | `get_overview`, `get_account_valuations`, `get_account_snapshot` |
| Managed contracts/history and liquidity | `list_products`, `get_product`, `list_product_operations`, `get_liquidity_overview` |
| Instrument search | `search_market_instruments` (query is sent to the configured provider) |
| Managed lifecycle facts (ledger_write) | `preview_product_operation` / `commit_product_operation` |
| Single/batch ledger records | `preview_change` / `commit_change`, `preview_batch` / `commit_batch` |
| Reconciliation/correction | `preview_reconciliation` / `commit_reconciliation`, `preview_correction` / `commit_correction` |
| History and reports | `list_activities`, `get_activity`, `analyze_period`, `list_contributions`, `get_contribution_item`, `get_return_day`, `get_asset_driver_detail` |
| Quotes and audit | `get_market_data`, `list_agent_market_data`, `import_market_data`, `set_fx_source` |
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
