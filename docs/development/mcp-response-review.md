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
  cache, initial default 5 per section, continuation 25/max 100 and complete 64 KiB wire sizing. Computation and
  capture limits remain separate. Oversized items fail explicitly; no truncation.
- Summary holds household endpoint totals and reconciliation header only.
  Rows, allocations/categories (with pageable missing identities), reconciliation contributions, points and per-point
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
| Context/catalog, members/institutions/groups/accounts/holdings | Bootstrap/catalog or directory arrays; query/account filters but directory cardinality unbounded; identities/ownership are useful | Add opt-in frozen directory/holdings pages and full item metadata; preserve existing filters/default contracts |
| Current overview/account valuations/snapshot | Household breakdown or account array/detail; components + composition details + slices repeat, unbounded holdings | Add bounded current overview and account projections preserving every GUI total/breakdown/evidence field; retain explicit legacy defaults |
| Historical overview/series | Full rows/evidence/categories/reconciliation; series 1–31 points plus full overview and per-point missing rows; wire can fail below compute limits | Default bounded historical read v2 |
| Financial context/comparison/attribution | Frozen 4 MiB captures, 5 min TTL, 64 KiB pages/item; repeated mandatory basis/summary and disclosure | Add effective leaf quality/contributions; retain coherence, aliases and financial precision evidence |
| Products/liquidity/upcoming/instruments/Agent history/market batch | Legacy full DTO plus opt-in mcp-read/1; compact sections, filters/sort, detail links, byte-aware pages | Reuse infrastructure; recommend paged paths; preserve current compatibility |
| Market single/search | Selected quote + range history (30d default, all unbounded); provider candidate arrays | Prefer existing bounded market batch current/coverage/history; external search bound belongs to provider contract |
| Activities/contributions | Activities keyset <=100, full item; contributions offset <=100, repeated aggregate metadata; related item/day/driver detail may grow | Retain precision/coverage and related-record semantics; frozen migration deferred |
| Period analysis | Four full projections with trends/categories/drivers/coverage; range computation bounded independently of encoded output | Prefer comparison attribution for two endpoints; named projection migration deferred |
| Review V2 | Rules/suppressions <=100; report groups, daily evidence, episodes/events/coverage revision-bound pages; one episode includes sibling alerts | Retain rule grouping and complete audit evidence; no unsafe field removal |
| Reconciliation sessions | Bounded header/page/item/attempt routes; links retain before/after review evidence | Retain; already separates heavy histories |
| Health/repair | Full issue/plan arrays; polling repeats health scan; job contains per-target detail | Add opt-in frozen issue/preview/job sections; retain unresolved issues, prerequisites and separate current health |
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
cost is separate from call output limits. After profiles, measured with the identical SDK + 1,024-byte reserve model:

| Holdings | Overview first page | 10-day series first page | Account headers |
| --- | ---: | ---: | ---: |
| 3 | 31,916 | 50,020 | 7,300 |
| 80 | 31,974 | 50,218 | 7,316 |

For 80 holdings this reduces initial output by approximately 93%, 90% and 97%.
These are initial pages, not claims that all evidence fits into one response.
The corresponding actual complete JSON-RPC results (512-byte request ID) were
31,495 / 49,739 / 6,837 bytes. Small differences between runs come from opaque
cursor/identifier lengths. No exact token counts or latency percentile claims.

## Realistic synthetic payload regression coverage

`payload_budget_test.go` observes the actual HTTP transport, including request
serialization and the complete JSON-RPC response with SDK text/structuredContent.
Success paths must return useful data; too_large is not accepted as a passing
normal result. Pathological single-item failures are separately tested through
an actual MCP client with deterministic single_item recovery guidance.

Two reusable shapes: ordinary = 20 accounts / 24 holdings / 8 instruments / 42
retained days; larger = 200 accounts / 100 holdings / 25 instruments / about two
years. Both have four currencies, shared holdings/quotes, 12 ledger activities,
long names/notes/provenance, two saved review rules with real published episodes,
a managed product and a reconciliation session. All facts are synthetic and
created with validated application services; none is sampled from user data.
The two-year history is intentionally sparse. Preparation is shared per tier;
analysis queries use two closed dates and one account, rather than rebuilding
full history independently for every probe.

The ordinary test runs in existing PR Go-test shards (~1.5 seconds locally).
The larger test is perf-tagged and has its own existing-perf-workflow matrix
entry, one focused test with a five-minute process bound (~6 seconds locally).
There are no additional full-suite or benchmark rounds in CI.

Every probed tool response must fit 65,536 actual bytes. Lower default budgets:
catalog 24 KiB, context 16 KiB, financial-context initial 28 KiB, historical
initial 56 KiB (five rows/points per section; measured near 50 KiB), bounded
current overview initial 32 KiB (measured near 28.5 KiB). Raw HTTP tools/list
discovery has a separate 160 KiB regression ceiling.
The baseline 103-tool figure (141.8 KB) is a serialized result-only measurement;
final tests separately log raw HTTP JSON-RPC and reconstructed JSON-RPC sizes
(149,386 bytes each in the final sample).
Initialize instructions are outside these figures. Discovery grows because
bounded options, filter enums and retrieval guidance are now published. SDK text compatibility and all safety instructions remain.

| Complete representative journey | Ordinary calls / bytes | Larger calls / bytes |
| --- | ---: | ---: |
| All directory pages + one full item | 2 / 36,868 | 9 / 359,699 |
| All 31-day history leaves/points + row and point detail | 10 / 311,920 | 32 / 1,374,210 |
| Targeted account historical holdings | 1 / 17,285 | 1 / 20,361 |
| All financial-context sections + one item | 7 / 165,813 | 27 / 1,227,654 |
| All health / repair-preview sections | 3 / 23,781 | 10 / 499,396 |
| Current overview: all breakdowns/evidence/labels | 5 / 58,939 | 7 / 228,604 |

Journey assertions bound both call counts and total bytes. Health audits request
100 on continuation to avoid tiny pages; every page still honors the wire cap.
The larger legacy health scan measured 482,246 bytes; the new first issue page
is about 18.6 KB. Complete evidence remains available and can legitimately use
more aggregate bytes than the old oversized response because pagination adds
metadata. Filtering is the main context reduction for targeted explanations.

Both tiers probe 60 registered tools. Current get_overview is tested through
its bounded opt-in path at both scales, with exact equivalence for every GUI
summary field and every section; the ordinary tier also probes its legacy path. Covered reads include directory/catalog/context,
account values/snapshot, both historical endpoints and frozen page/item routes,
financial context/comparison/attribution and pages/items, market current/history
and Agent provenance, activities/items, period analysis/contribution/item/day/
driver, product/list/operation/liquidity/upcoming, reconciliation header/page/item,
and rules/alerts/coverage/daily evidence/episode history/events/suppressions.
Synthetic ledger, product, session and rule preview/commit/receipt responses are
also measured without weakening their safeguards. Page unions, unique IDs,
matched counts, filters, shared evidence and same-capture detail are checked.

Coverage limits: get_data_repair_job paging is covered by the isolated existing
repair/retry test, not the 200-account tier. Remote search/provider I/O is excluded.
Specialized batch/correction/reconciliation/product-terms/product-valuation/
liquidity-policy previews retain their existing safety tests but are not claimed
as realistic-scale payload probes. Legacy full defaults (including current overview), long-range
whole-household return analysis, dense multi-year provider
history and hundreds of simultaneously active review rules remain unbounded or
unmeasured; use the documented bounded alternatives. No blanket claim that every
legacy result or every possible single-item payload fits 64 KiB.

Focused regressions additionally cover manual old Agent quotes across days,
valid closed-day carry, independently missing/carried FX, both carried counting
once, shared quotes, pending 1 -> 0, missing funding-cash leaf -> household
aggregation/reason filters, immutable source changes, empty matches, signed
cursor tampering/cross-capture/section/expiry and oversized item errors. Category
and point missing identities remain independently pageable. GUI accounting,
quote/ledger facts, as-of selection and preview/commit safety are unchanged.


Filtered captures retain full row evidence for household category missing-row
references even when those leaves are excluded from the list. rowDetailScope
states this explicitly; filtered counts and household monetary scope do not
change. A client regression follows every categoryMissing key from a filtered
capture and verifies matching capture hash and full row evidence.

Validation environment: Linux/amd64, Go 1.27.3, isolated temporary SQLite data.
Focused application/MCP regressions and vet passed; 69 Python tooling tests and
the maintained skill/selector checks passed. The single whole Go test attempt found two historical late-close freshness
regressions and an item test's obsolete 100/8 packing assumption. Follow-up
regressions verify the preserved market-date close normalization and complete
unique byte-bounded item union; manual/closed-day policy remains authoritative.
That whole-tree run also could not build cmd/nestworth because GTK4/WebKit6
(and related native development packages) are absent. Desktop build, whole-tree
vet and binding generation have the same host dependency blocker. No native
macOS, browser/UI, signing, deployment or real-household validation is claimed.


Current overview acceptance: the new opt-in mcp-read/1 projection splits only
collections, preserving every original field. The 200-account client follows
all breakdown/missing/activity/label cursors, compares exact header totals,
currency/completeness/quality and the full section union to the GUI adapter.
The skill and tool description prefer this bounded route; legacy defaults can
still overflow, and this review does not claim every MCP payload problem solved.
