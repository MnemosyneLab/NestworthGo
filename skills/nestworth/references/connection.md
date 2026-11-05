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

## Permissions and discovery

| Mode | Available operations |
| --- | --- |
| `read_only` | Financial context and frozen pages, current valuation, managed products/history, liquidity and upcoming-action reads, daily investment-review reads, historical overview/series, catalogs, history, analysis, market-data/audit reads, health scans, repair previews/status |
| `directory_write` | All reads plus account/instrument/member/institution/group maintenance |
| `ledger_write` | All above plus ledger previews/commits, quote imports/source selection, reconciliation, corrections, managed lifecycle/terms/value pairs and starting data repair. Daily investment-review management writes stay hidden until Settings enables investment review management |

Use the tools this client lists; do not rediscover before every call. Follow
`nextCursor` if the client's tool list is paged. An absent tool may reflect the
permission mode or an older App: explain the specific setting or update needed
and do not attempt another tool to bypass it. `get_context.capabilities` helps,
but actual availability and schemas decide whether an operation is supported.
There is no need to inspect every tool before a task.

Ordinary reads call their tool directly. For a minimal summary call
`get_financial_context` directly: `get_context` returns identity-bearing
household metadata and is only for unknown instance, permission, timezone or
history-start metadata (`get_catalog` for unknown vocabulary or entity creation);
reuse both until the context changes. If they are missing, explain the needed App
update; an identity-bearing fallback requires an explicit disclosure choice.
Skill updates do not update the App or MCP connection. Analysis tools, including
`compare_financial_attribution`, can maintain derived snapshots under read_only
and carry non-read-only annotations; they grant no ledger permission and never
certify old frozen comparisons.

## Tool groups

| Area | Tools |
| --- | --- |
| Context and recovery | `get_context`, `get_catalog`, `get_operation` |
| Consistent financial summary and gaps | `get_financial_context`, `get_read_page`, `get_financial_context_item` |
| Coherent comparison and attribution link | `compare_financial_attribution`, `get_read_page` |
| Two-state differences, not returns | `compare_financial_context`, `get_read_page` |
| Definitions | `list_members`, `list_institutions`, `list_groups`, `list_accounts`, `list_instruments`, `list_holdings` |
| Definition maintenance | `create_member`, `update_member`; `create_institution`, `update_institution`; `create_group`, `update_group`; `create_account`, `update_account`, `archive_account`; `create_instrument`, `update_instrument`, `archive_instrument`. Member, institution and group archive is `archived` on the matching `update_*` tool |
| Current state | `get_overview`, `get_account_valuations`, `get_account_snapshot` |
| Managed contracts/history and liquidity | `list_products`, `get_product`, `list_product_operations`, `get_liquidity_overview`, `get_upcoming_actions` |
| Daily investment review v2 | `list_investment_review_rules`, `preview_investment_review_rule`, `list_investment_review_alerts`, `get_investment_review_alert`, `get_investment_review_coverage`, `list_investment_review_daily_evidence`, `list_investment_review_episode_history`, `list_investment_review_episode_events`, `list_investment_review_suppressions`. Management tools are hidden by default. With `ledger_write` and investment review management enabled: `create_investment_review_rule`, `update_investment_review_rule`, `archive_investment_review_rule`, `acknowledge_investment_review_alert`, `set_investment_review_suppression`, `clear_investment_review_suppression`, `refresh_investment_reviews` |
| Read-only historical view | `get_historical_overview` (`{date, compareTo}` or `{from, to}`) |
| Instrument search | `search_market_instruments` (query is sent to the configured provider) |
| Previews (ledger_write) | `preview_change`, `preview_batch`, `preview_correction`, `preview_reconciliation`, `preview_reconciliation_session`, `preview_product_operation`, `preview_product_terms`, `preview_product_valuation`, `preview_save_liquidity_policy`, `preview_reset_liquidity_policy` |
| Commit any preview (ledger_write) | `commit_plan` with the returned `planId`; the server reads the plan kind |
| History and reports | `list_activities`, `get_activity`, `analyze_period`, `list_contributions`, `get_read_item` |
| Quotes and audit | `get_market_data`, `compare_market_data_import`, `list_agent_market_data`, `import_market_data`, `set_fx_source` |
| Data repair | `scan_data_health`, `preview_data_repair`, `start_data_repair`, `get_data_repair_job` |

Initialize the household and its history starting point in the App. In particular,
ledger recording requires started history; the MCP does not start/reset it or
choose opening balances/costs. This is distinct from importing a position later
in an already started history.

JSON examples in these references are **business-tool arguments**, not raw HTTP requests.
`${...}` denotes a value obtained from the user or a previous response. Replace
all placeholders; never send them as IDs. Values/dates are illustrative, not
verified market information. Reads/previews usually take arguments directly;
mutations take `operationId` and `input`. `commit_plan` may omit `operationId`; other writes still require it. Use UUIDs from successful receipts,
not provisional IDs in a preview.

Managed capabilities are managed_product_read, liquidity_read and, only with
ledger_write, managed_product_lifecycle, managed_product_terms and
managed_product_valuation. Daily investment-review reads are available in all
modes. Rule, acknowledgement, suppression and explicit refresh tools also
require the investment review management switch, which is off by default.
`get_context.investmentReviewManagement` reports it, and
`investment_review_v2_write` appears in capabilities only when both that
switch and ledger_write are on. Enable it in Settings → AI / MCP, then apply;
a running connection gets a new access token. These tools update review
rules, episodes and derived projections, not financial facts. The App's own
review screen does not use this switch.

## Scale capabilities and permissions

See [paging and batches](paging-and-batches.md). The capabilities are bounded_read,
instrument_paging and market_data_batch. `get_read_page`/`get_read_item` and
bounded contracts are available in every permission mode. instrument_metadata_batch
and `update_instruments_batch` require directory_write or ledger_write.
Missing tools do not authorize filesystem/provider workarounds. Repository bundle updates do not install or enable a real client.

Reconciliation sessions expose reconciliation_session_read in every permission
mode and reconciliation_session_write only with ledger_write. Directory access
does not authorize ledger attestations. Follow [the session workflow](reconciliation-and-corrections.md#account-by-account-sessions-discover-support-first) with the actual session tool names.
