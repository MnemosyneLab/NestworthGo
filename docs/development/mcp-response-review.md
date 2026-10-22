# MCP response review and historical read design

Base: `1ee6cc4257a2efe4d1155e895b032370173738f8`. Synthetic data only.

## Response principles and decisions

- Keep exact decimal strings, currency/units, dates/timezone, scope, source,
  effective quality/reasons, unknown values and completeness. Account rows are
  rollups; only included leaves contribute to pending (`carriedCount`), once
  even when both price and FX are carried. Informational gaps are not pending.
- Prefer compact named views, stable bounded sections and explicit item IDs.
  Full evidence stays retrievable within the same immutable capture. No arbitrary
  field projection. No changes to arithmetic, quote selection or historical policy.
- Historical endpoints move to the existing `mcp-read/1` envelope by default.
  This is a documented breaking response-shape change (historical read v2),
  with no parallel legacy implementation. Existing date arguments remain valid.
  `view=compact` is the default; `view=evidence` expands page rows. Both use
  `get_read_page` and `get_read_item`, the existing hash/HMAC/generation/TTL
  cache, default 25/max 100 and complete 64 KiB wire sizing. Computation and
  capture limits remain separate. Oversized items fail explicitly; no truncation.
- Summary holds household endpoint totals and reconciliation header only.
  Rows, allocations/categories, reconciliation contributions, points and per-point
  missing evidence are independent sections. Series never embeds full Overview.
  Rows sort by immutable key; points by civil date. Filters select endpoint rows
  before paging: account/instrument, leaf/rollup, pending/actionable quality/reason
  and side. Household endpoint/trend totals remain explicitly household-scoped;
  matched included-leaf/pending counts describe exactly the filtered row set.
  Account/instrument filters do not invent a partial-scope reconciliation or trend.
- Effective freshness comes from valuation's adopted evidence, including manual
  preference with Agent provenance and valid closed-day carry. Raw observation
  freshness is separate diagnostic evidence; it cannot create stale warnings.
  Price and FX evidence remain independently attributable.
- Add opt-in `responseMode=paged` to account valuations: compact account headers,
  separate components/composition/missing sections and same-capture detail.
  This preserves existing current-account consumers while providing a bounded
  alternative to repeated GUI layouts. Prefer existing market batch/financial
  context/management paging routes over adding duplicate frameworks.
- Preserve SDK text + structuredContent: existing text-only consumers parse the
  JSON text. Removing it is a client migration, not a safe field optimization.
  Keep preview commands/effects, CAS revisions, authorization, receipts,
  idempotency and errors unchanged. No generic mutation-result shrinking.

## Registered endpoint inventory

All registered names are inventoried below by owning file; this includes writes
and all permission modes. `localTool`, attribution wrapper and explicit SDK
handlers share Response.data; SDK duplicates that object as text/structured
content. Error adaptation uses safeError; bounded HTTP tools additionally sanitize
schema errors, reject batches/oversized IDs, and cap the complete output. Other
legacy tools retain SDK behavior; a universal wire-contract change is deferred.

- `attribution_tools.go`: `list_contributions`, `get_contribution_item`, `get_return_day`, `get_asset_driver_detail`.
- `batch_tools.go`: `preview_batch`, `commit_batch`.
- `bounded_read.go`: `get_read_page`, `get_read_item`.
- `correction_tools.go`: `preview_correction`, `commit_correction`.
- `data_health_tools.go`: `scan_data_health`, `preview_data_repair`, `get_data_repair_job`, `start_data_repair`.
- `feature_tools.go`: `get_upcoming_actions`, `get_historical_overview`, `get_historical_overview_series`.
- `financial_comparison_tools.go`: `compare_financial_attribution`, `compare_financial_context`, `get_financial_comparison_page`.
- `financial_context_tools.go`: `get_financial_context_item`, `get_financial_context`, `get_financial_context_page`.
- `history_tools.go`: `list_activities`, `get_activity`, `analyze_period`.
- `instrument_batch_tools.go`: `update_instruments_batch`.
- `investment_review_v2_tools.go`: `list_investment_review_rules`, `preview_investment_review_rule`, `list_investment_review_alerts`, `get_investment_review_alert`, `list_investment_review_daily_evidence`, `list_investment_review_episode_history`, `list_investment_review_episode_events`, `get_investment_review_coverage`, `list_investment_review_suppressions`, `create_investment_review_rule`, `update_investment_review_rule`, `archive_investment_review_rule`, `acknowledge_investment_review_alert`, `set_investment_review_suppression`, `clear_investment_review_suppression`, `refresh_investment_reviews`.
- `ledger_tools.go`: `preview_change`, `commit_change`.
- `liquidity_policy_tools.go`: `preview_save_liquidity_policy`, `preview_reset_liquidity_policy`, `commit_save_liquidity_policy`, `commit_reset_liquidity_policy`.
- `market_data_batch_tools.go`: `get_market_data_batch`.
- `market_data_tools.go`: `get_market_data`, `list_agent_market_data`, `import_market_data`, `set_fx_source`.
- `product_data_tools.go`: `preview_product_terms`, `preview_product_valuation`, `commit_product_terms`, `commit_product_valuation`.
- `product_operation_tools.go`: `preview_product_operation`, `commit_product_operation`.
- `product_tools.go`: `list_products`, `get_product`, `list_product_operations`, `get_liquidity_overview`.
- `reconciliation_session_tools.go`: `list_reconciliation_sessions`, `get_reconciliation_session`, `get_reconciliation_session_page`, `get_reconciliation_item`, `preview_reconciliation_session`, `commit_reconciliation_session`.
- `reconciliation_tools.go`: `preview_reconciliation`, `commit_reconciliation`.
- `tools.go`: `get_context`, `get_catalog`, `get_operation`, `list_members`, `list_institutions`, `list_groups`, `list_accounts`, `list_instruments`, `list_holdings`, `get_account_valuations`, `get_account_snapshot`, `get_overview`, `create_member`, `update_member`, `archive_member`, `set_member_icon`, `create_institution`, `update_institution`, `archive_institution`, `set_institution_icon`, `create_group`, `update_group`, `archive_group`, `set_group_icon`, `create_account`, `update_account`, `archive_account`, `create_instrument`, `update_instrument`, `archive_instrument`, `search_market_instruments`.

## Shapes, bounds and compatibility assessment

| Family | Current shape / bound / repetition | Decision |
| --- | --- | --- |
| Context/catalog, members/institutions/groups/accounts/holdings | Bootstrap/catalog or directory arrays; query/account filters but directory cardinality unbounded; identities/ownership are useful | Retain contracts; use scoped discovery, financial context for summaries; directory-wide default migration deferred |
| Current overview/account valuations/snapshot | Household breakdown or account array/detail; components + composition details + slices repeat, unbounded holdings | Add bounded account projection with full evidence retrieval; retain explicit legacy contract |
| Historical overview/series | Full rows/evidence/categories/reconciliation; series 1–31 points plus full overview and per-point missing rows; wire can fail below compute limits | Default bounded historical read v2 |
| Financial context/comparison/attribution | Frozen 4 MiB captures, 5 min TTL, 64 KiB pages/item; repeated mandatory basis/summary and disclosure | Add effective leaf quality/contributions; retain coherence, aliases and financial precision evidence |
| Products/liquidity/upcoming/instruments/Agent history/market batch | Legacy full DTO plus opt-in mcp-read/1; compact sections, filters/sort, detail links, byte-aware pages | Reuse infrastructure; recommend paged paths; preserve current compatibility |
| Market single/search | Selected quote + range history (30d default, all unbounded); provider candidate arrays | Prefer existing bounded market batch current/coverage/history; external search bound belongs to provider contract |
| Activities/contributions | Activities keyset <=100, full item; contributions offset <=100, repeated aggregate metadata; related item/day/driver detail may grow | Retain precision/coverage and related-record semantics; frozen migration deferred |
| Period analysis | Four full projections with trends/categories/drivers/coverage; range computation bounded independently of encoded output | Prefer comparison attribution for two endpoints; named projection migration deferred |
| Review V2 | Rules/suppressions <=100; report groups, daily evidence, episodes/events/coverage revision-bound pages; one episode includes sibling alerts | Retain rule grouping and complete audit evidence; no unsafe field removal |
| Reconciliation sessions | Bounded header/page/item/attempt routes; links retain before/after review evidence | Retain; already separates heavy histories |
| Health/repair | Full issue/plan arrays; polling repeats health scan; job contains per-target detail | Defer named summary/paged job migration; retain unresolved issues and prerequisite evidence |
| Preview/commit, directory writes, market import, policy/product/review writes, operation receipt | One item or <=100 commands/targets; normalized requests/effects/receipts can be large | Preserve safety evidence, original outcomes and retry hashes; no destructive compaction |

Discovery includes every tool's input/output schema and long business/safety
instructions. Keep safety text and the SDK-required output schema. Repeated generic
paging guidance can be centralized in maintained docs; tools must still state
scope, limits and how to retrieve evidence. Do not omit tool schemas or silently
change permissions to save discovery bytes.

## Verification and measured response bytes

Measurements will use one small and one larger synthetic fixture, the actual
SDK CallToolResult (both text and structuredContent) plus JSON-RPC overhead.
Tests cover client summary -> filtered pages -> item evidence, immutable retries,
source changes, empty matches, cursor isolation/expiry and oversized items.
Baseline measured on Linux/amd64, Go 1.27.3, SDK 1.7.0, server build tags,
one paired sample per scale (`TestResponseReviewMeasurements`). Four synthetic
accounts, 3 or 80 manual stock holdings with old Agent close observations, ten
closed dates; no external providers. Wire figures include SDK duplication and
1,024 bytes reserved for the complete JSON-RPC envelope (the HTTP limit is 65,536).

| Holdings | Full overview | Full 10-day series | Full account valuations |
| --- | ---: | ---: | ---: |
| 3 | 54,210 | 88,574 | 20,922 |
| 80 | 485,490 | 520,142 | 228,222 |

Ledger-write discovery: 103 tools, 141,783 serialized result bytes. Discovery
cost is separate from call output limits. After figures follow implementation.
