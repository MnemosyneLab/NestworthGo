# Current wealth, returns and asset-change explanations

Use `get_overview` and `get_account_valuations` for
bounded current reads. Overview summary preserves GUI
exact totals, currency, completeness and display quality; independently page all
relevant breakdowns, missing inputs, recent activities and identity-label sections
with get_read_page. Its totals follow the existing account filters. Small results
come back inline (at least five rows per section when they fit, at most 25);
continuations are 25/max 100. `delivery.selectionComplete=true` means every selected row is
already inline; it says nothing about data completeness (`complete`, `missingInputs`). Preserve complete and quality
when interpreting amounts. get_financial_context remains the
direct minimal-disclosure summary and evidence path.

Use `get_overview`, `get_account_valuations`, or `get_account_snapshot` for
**current** net worth/account values, holdings, costs and missing inputs. They
do not answer last month's income or investment return. Preserve ownership,
inclusion filters, household base currency and incomplete-value indicators.

From `get_account_valuations`, open `get_read_item` accounts/id. Its
composition_types section groups components by instrument type/cash;
composition_details contains ranked details, and composition_detailSlices
retains the first ten rows plus an exact grouped
remainder when there are more than ten. `totalExact` and each
`baseAmountExact` are decimal strings; `baseAmount` is a presentation money
view and may be rounded. Read `status`, `complete`, `valuedCount` and
`missingCount` before describing a total. Unknown components have no exact
amount, never zero. `shareBps` uses the exact positive total; sum either the type
partition or detail partition, not both. A synthetic remainder's `detailRefs`
identify the collapsed detail keys. Liability or non-holdings accounts can be
`unsupported` for composition while their ordinary account valuation remains
available.

## Display estimates and strict returns

Read additive `displayValuation` independently of strict completeness. A full
provisional estimate is usable valuation, not confirmed return. A partial known
net contribution is not complete wealth or a lower bound. Pending coverage
differs from absent price/FX/account value; informational waiting notices do not
request repair. Effective NAV dates differ from retrieval time; never turn a
late download into a gain on its fetch date or invent publication deadlines or
target-date observations. `estimatedChange` includes flows and is not return.
Use existing detail routes for evidence rather than expanding every trend point.
Effective manual instruments (including Agent observations) and valid closed-day
carry follow the App's policy. Raw `observationFreshness` is diagnostic age and
does not imply a pending valuation. Each included leaf contributes at most one
pending count even when both price and FX are carried; account rollups contribute
zero. Read `countedLeaf`, `pendingContribution`, `displayValuation` and adopted
price/FX evidence; never infer the affected asset from informational gaps.

## Historical endpoint view and reconciliation

For one or two historical states, use `get_historical_overview`; for a bounded
daily trend with its endpoint views from the same capture, use
`get_historical_overview_series`. Dates are closed `YYYY-MM-DD` civil labels in
the household history timezone. Overview accepts an optional second closed date
or `current`; series accepts 1–31 forward closed dates. New Apps return `historical-read/2` in a frozen `mcp-read/1` envelope: summary,
compact endpoint leaves, and independent series points. Initial default is five
items per section, max 100; continuation default is 25. Follow every relevant
`get_read_page` cursor. `get_read_item` with section=rows/id returns full row
price/FX evidence; section=points/id=YYYY-MM-DD opens pageable missing evidence;
section=overview/id=endpoints opens categories, reconciliation contributions
and reconciliationDetails; section=reconciliationDetails/id=<key> opens
separately pageable related records;
section=states/id=left or right returns endpoint allocations. `view=evidence`
expands row pages. Do not expect a nested full Overview in series output.

Optional filter selects endpoint rows by accountIds, instrumentIds, kinds,
included, quality (pending/missing/provisional/verified), reasons and
side (left/right/either). Household totals/trend/reconciliation remain household
scope; matchedLeft/Right counts describe filtered leaves over all pages. Empty
matches are not zero household wealth. Only counted leaves can be summed;
account rollups duplicate their children. These routes retain the GUI's exact
amounts, coverage, capture time, source evidence and reason codes. These reads do not materialize snapshots,
refresh providers or modify ledger facts.

When two compatible closed-date endpoints support reconciliation, preserve the
returned basis `endpoint_bridge/2; later_endpoint_fx; opening_exposure_fx; paired_principal_valuation; retained_corrected_facts` and exact identity:

`B net worth − A net worth = supported contributions + unexplained residual`.

<!-- example: historical-overview -->
```json
{"date":"2026-10-01","compareTo":"2026-10-10"}
```

<!-- example: historical-overview-series -->
```json
{"from":"2026-10-01","to":"2026-10-10"}
```

The bridge uses the later endpoint's FX rate, not event-time FX or investment
return. For stable holdings, opening native exposure supports FX; price movement
uses reconciled quantities, uniquely paired trade principal and comparable price
evidence. Valuation change after principal can include execution-price spreads
and is not pure price return. Quantity remeasurement is a recorded adjustment,
never inferred income. Completeness, scope,
timestamp/source basis and intermediate changes can leave amounts in residual
or make the bridge unavailable; retain its `status`, `reasons`, nullable totals
and exact contribution amounts. Same-day complete endpoints have zero movement;
reverse-date comparisons negate contributions and residual. A current right
endpoint is unavailable for bridge attribution. Do not interpret a zero
residual as proof there were no evidence gaps.

This answers how endpoint values reconcile. `compare_financial_attribution`
answers a different question using its closed-period analysis and derived
snapshots. Neither replaces the other; do not substitute one result or its FX
convention for the other.

## Opt-in investment review reminders

Daily investment review v2 uses named rules. A rule evaluates its **daily
condition** on each closed civil date in its fixed timezone; the separate
`triggerPolicy` chooses one hit, consecutive hits, or N hits in M calendar
days. Missing data is unknown and contributes no hit. Today's unclosed value
is only a preview. `list_investment_review_rules` pages definitions (at most
100; follow page.nextCursor through the final empty page, even after a short page), while
`list_investment_review_alerts` pages current open alerts in whole instrument
groups (at most 100; follow `page.nextCursor`), and
`get_investment_review_coverage` includes current positive holdings whose
rules are unknown, confirming, healthy or uncomputed without an open alert.
`get_investment_review_alert` reads one current episode by ID;
`list_investment_review_suppressions` lists ignored scopes and absolute resume
dates. Its nonempty items/page results likewise continue using page.nextCursor
with get_read_page through a final empty page. All these reads, and `preview_investment_review_rule`, are read-only:
they never refresh source data or publish an evaluation. Rule previews return
mcp-read/1: retain sourceToken/latestClosedOn/window from summary, and follow the
frozen days section only for requested daily evidence (default 25/max 100).
expectedSourceToken rejects a changed source before capture. A full 365-day
policy window uses get_read_page; never replace it with a different shorter
policy merely to fit the response. Preview days stay frozen through source
changes and use fixed capture TTL/revocation. Published alert, daily-evidence,
coverage and episode cursors instead bind report revision/filter; restart those
reads from page one after a changed publication. Rule/suppression identity
cursors remain live, as described above.
`list_investment_review_episode_history` pages open and resolved episodes for
one daily rule, including an archived rule. Use
`list_investment_review_episode_events` with an episode ID to page its opening,
acknowledgement, evidence-correction and resolution events. These historical
reads preserve the original condition version and never reinterpret it as the
current rule; a changed publication expires their cursors.

With `ledger_write`, v2 uses `create_investment_review_rule`,
`update_investment_review_rule`, `archive_investment_review_rule`,
`acknowledge_investment_review_alert`, `set_investment_review_suppression`,
`clear_investment_review_suppression` and `refresh_investment_reviews`.
Rule/alert/suppression edits require their own `expectedRevision` and inner
`mutationId`; the MCP envelope also requires `operationId`. Reuse both IDs and
the exact input on a retry. Inspect `get_operation` and the relevant v2 read
after an unknown outcome before issuing a new operation. Refresh is explicit
and uses existing local facts; it does not fetch quotes. An all-rule permanent
suppression also applies to rules created later; acknowledgement applies only
to one episode. Neither action changes holdings.

For investment returns, income/expense or an inclusive period report, call
`analyze_period`. For two-state value differences use `compare_financial_context`;
for “why did two closed-date states change; was that profit?”, use
`compare_financial_attribution` directly; see [financial-context.md](financial-context.md#link-a-comparison-to-change-attribution).
Neither minimal comparison route needs get_context/catalog/directory reads.
Attribution may maintain derived snapshots and invalidate previews, including
in read_only mode; the plain comparison is strictly no-write.
`query.from` / `to` are inclusive
closed local dates in the history-origin timezone; the end must be before today
in that timezone. Clarify “this month” if it means current valuation versus
closed-day analysis. Do not substitute UTC dates for the household's dates.

Example closed-day query:

<!-- example: analyze-period -->
```json
{"query":{"from":"2026-09-01","to":"2026-09-28","scopeKind":"household","valuation":"base","basis":"investment"}}
```

Use actual schemas for supported account/instrument/currency/member/asset-class
filters. State the requested scope, date range, base versus native currency and
coverage. Analysis may materialize derived snapshots and invalidate an
outstanding ledger preview; perform analysis before preview/commit.

## Match the question to a report

| Question | Path |
| --- | --- |
| “上个月赚了多少？” | `analyze_period` investment return, then contribution if needed |
| “收入支出各多少？” | Period income/expense; own-account transfers and investment principal are not income/spending |
| “哪几只资产贡献最大？” | `list_contributions` with returnType, groupBy and ordering; frozen rows via `get_read_page` |
| “这行收益为什么是这个数？” | `get_contribution_item` with the list captureId and exact groupKey; frozen child sections via `get_read_page` |
| “某一天为什么亏损？” | `get_return_day`, date inside the same period |
| “净资产涨了，究竟是存钱、涨价还是汇率？” | Period assetChange buckets, then `get_asset_driver_detail` with an actual returned bucket key |
| “对应哪条交易？” | Follow history hints with `list_activities`, then `get_activity` |

Contribution lists use `mcp-read/1`: inspect `summary` and `sections.rows`, then
follow only the rows needed with `get_read_page` (captureId, section=`rows`, signed
cursor; default limit 25, maximum 100). There is no offset argument. For selected
row evidence call `get_contribution_item` with that same captureId and groupKey;
do not repeat the query, returnType or groupBy. Follow its independently named
child sections only when their evidence is needed. Pages and details never
recalculate or renew the five-minute TTL. On expiry, eviction or revocation,
restart the list and all related sections; never mix captures. Run analysis before
preparing financial writes because source capture may maintain derived snapshots.

Contribution return types are `total_return`, `realized`, `unrealized`,
`dividend_interest`; grouping is instrument/account/currency/asset_class.
Use the response's available/status/missingReason and ratedDays/totalDays.
The current backend can report unrealized attribution as unavailable; querying
that option is not proof it can be fully calculated. Realized attribution keeps
household base currency as its output regardless of valuation. Currency scopes
and filters match the sell event's native denomination; asset-class scopes and
filters match current instrument `Type` metadata. Scope and explicit filters
intersect, including on the same dimension. Contradictions yield available
empty results. Realized grouping remains instrument/account, and member filters
remain unsupported. Detail hints identify related history and may be broader
than a currency/class-filtered row's exact activity set.

Asset drivers include external flow, income, spending, dividend/interest, price,
FX impact, conversion spread, fees, liabilities, adjustments and residual.
Do not attribute residual to market performance without evidence. Reconcile
displayed contributors with authoritative totals; state a residual or partial
coverage rather than dropping it. Do not add per-account return percentages
or recompute an invented portfolio rate from rounded display values.

`list_activities` has inclusive local-date filters, account/instrument/kind
filters and bounded pagination. Continue `page.nextCursor` using `get_read_page` with only cursor and optional limit; do not silently analyze only
the first page. For a complete contribution traversal, follow the rows section's
`page.nextCursor` with `get_read_page({cursor, limit?})` until that section's `page.hasMore` is false. A shorter page does
not mean completion; keep the returned cursor rather than calculating an offset.

Zero is a measured result; null/unavailable is missing evidence. Report partial
amounts as partial, preserve native/base distinctions and don't treat asset
change as investment profit. If missing evidence prevents the requested answer,
describe what is known and route to [data-health.md](data-health.md).

## Frozen review and upcoming pages

Upcoming captures keep global counts/completeness/warnings and the shared
30-civil-day window, with independent items/dataChecks sections. Filter actions
by accountIds/productIds/kinds/groups; dataChecks use accountIds/productIds only.
Sort priority (default desc, then sortOn asc), sortOn or displayName, id asc ties.
An extreme single action with many targets may still exceed one page; report
that explicit error instead of omitting links or inventing posted cash.

Period analysis now returns mcp-read/1: summary.income/expenses/assetChange/
investmentReturns contain headers; sections income/rows, expenses/rows,
assetChange/waterfall, assetChange/groups, investmentReturns/points and
investmentReturns/sources hold complete collections. Follow every relevant cursor.
Contribution-item, return-day and asset-driver detail also return summary headers
and independent evidence sections. Do not parse these as embedded GUI arrays.
