# Current wealth, returns and asset-change explanations

<!-- shared-recipes:start -->
<!-- Generated from internal/agentguidance/recipes.json; update that source. -->
### Shared recipe: historical

Use date/compareTo for endpoints; from/to for 1–31 closed days in household history timezone. Endpoint movement includes principal, not investment return. Filters select leaves; totals/reconciliation may stay household-wide. Never sum account rollups with components. Single-date pending: get_historical_overview with date, filter={included:true,quality:"pending"}, view="evidence". Pending means carried price/FX, not missing valuation. Do not scan all rows or regex JSON (carriedCount:0 is normal); check pendingContribution > 0 on counted leaves. Follow only relevant filtered cursors.

### Shared recipe: review-rule

Daily investment review is a reminder, not a sell recommendation or ledger entry. A closed-day predicate and triggerPolicy are separate: single, consecutive count, or rolling windowDays/minHits. Missing evidence is unknown, not a hit; today is unclosed. Use the complete advertised spec: fixed version/evaluator/dataPolicy, target, predicate, schedule and triggerPolicy. Predicate root is all/any with at most two group levels and 20 leaves. amount/cost compare exact money with currency and fxMode; price_change/annualized use percent and a 7/30/90/180/365-day lookback. Annualized conditions need an eligible type gate in the same all path. Choose an explicit fixed IANA timezone. The synthetic example illustrates format; replace its timezone, scope and threshold with the user's intent.

<!-- example: review-rule-preview -->
```json
{"instrumentId":"${instrumentId}","spec":{"specVersion":1,"metricPolicyVersion":"daily_metrics_v1","evaluator":"daily_predicate_v1","target":{"kind":"all_current"},"predicate":{"all":[{"leaf":{"metric":"amount","operator":">","value":"1000","currency":"USD","fxMode":"native"}}]},"schedule":{"period":"calendar_day","timezone":"UTC","cutoffVersion":"civil_day_end_v1"},"triggerPolicy":{"kind":"consecutive","count":2},"dataPolicy":{"missing":"unknown","calendar":"all_civil_days","quoteCarry":"forbid_for_daily_signal","baselineMaxGapDays":7,"fxReferenceMaxAgeDays":7,"fxTimestampMaxAgeHours":12}}}
```
<!-- shared-recipes:end -->

Use `get_overview` or `get_account_valuations` for **current** values.
The default overview returns `displayValuation`, valuation quality and a
missing-input count. A nullable `total` and a separate `knownSubtotal` describe
each amount. Treat the amount as complete only when its quality supports that.
Request `sections` such as `["missingInputs","byAccountType"]` only when needed.
`availableSections` lists valid selectors. A new overview request captures fresh
data. Use `get_read_page` to continue sections from the current capture.
Account valuations accept one or many accountIds plus account filters; unmatched
filters return an empty page. Default summary includes totals, asset types and
price freshness. Use detail=holdings for component evidence in the same call.
Do not add rollups to components or repeatedly fetch the same totals.
Use `analyze_period` for returns and `get_historical_overview` for endpoint
movement. Preserve ownership, inclusion, currency and incomplete-value indicators.

From `get_account_valuations`, open `get_read_item` accounts/id only for additional evidence. Select child
names using `sections` from `availableSections`. Its
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

Use `get_historical_overview` for both shapes. `{date, compareTo}` reads one or
two historical states (`date` may be empty for the GUI default month end;
`compareTo` is an optional second closed date or `current`). `{from, to}` reads
a bounded daily trend with its endpoint views from the same capture. Do not send
both shapes together. Dates are closed `YYYY-MM-DD` civil labels in
the household history timezone. Series accepts 1–31 forward closed dates. New Apps return `historical-read/2` in a frozen `mcp-read/2` envelope: summary,
compact endpoint leaves, and independent series points. Omitting limit fills small results within the byte budget (at least five
when they fit, at most 25); explicit maximum is 100 and continuation default is 25. Follow every relevant
`get_read_page` cursor. `get_read_item` with section=rows/id returns full row
price/FX evidence; section=points/id=YYYY-MM-DD opens pageable missing evidence;
section=overview/id=endpoints opens categories, reconciliation contributions
and reconciliationDetails; section=reconciliationDetails/id=<key> opens
separately pageable related records;
section=states/id=left or right returns endpoint allocations. `view=evidence`
expands row pages. Do not expect a nested full Overview in series output.
A one-day series retains its point but has only the left endpoint, with no
self-comparison rows or reconciliation. A right-side selector maps to left for
that single day. Explicit date/compareTo comparisons retain both endpoints.

For “which item is pending update on this date?”, use the targeted recipe:

<!-- example: historical-pending -->
```json
{"date":"2026-10-01","filter":{"included":true,"quality":"pending"},"view":"evidence"}
```

Read the returned name, price/FX dates and reason codes directly. Pending is
carried evidence; points/date missing evidence is not a pending-item directory.
Do not fetch all rows first. If inspecting rows locally, require countedLeaf=true
and pendingContribution > 0; never search JSON text for "carried" or "pending"
(field names also occur on healthy rows).

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
mcp-read/2: retain sourceToken/latestClosedOn/window from summary, and follow the
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

With `read_write`, v2 management uses
`create_investment_review_rule`, `update_investment_review_rule`,
`archive_investment_review_rule`, `acknowledge_investment_review_alert`,
`set_investment_review_suppression`, `clear_investment_review_suppression`
and `refresh_investment_reviews`. The reads above are available in both modes.
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
`compare_financial_attribution` directly; see [financial-attribution.md](financial-attribution.md#link-a-comparison-to-change-attribution).
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
| “这行收益为什么是这个数？” | `get_read_item` on the list capture, section=`rows`, id=the row key; frozen child sections via `get_read_page` |
| “某一天为什么亏损？” | `get_read_item` on the `analyze_period` capture, section=`returnDays`, id=that YYYY-MM-DD |
| “净资产涨了，究竟是存钱、涨价还是汇率？” | Period assetChange buckets, then `get_read_item` section=`assetDrivers` and id=that bucket key |
| “对应哪条交易？” | Follow history hints with `list_activities`, then `get_activity` |

Contribution lists use `mcp-read/2`: inspect `summary` and `sections.rows`, then
follow only the rows needed with `get_read_page` (captureId, section=`rows`, signed
cursor; default limit 25, maximum 100). There is no offset argument. For selected
row evidence call `get_read_item` with that same captureId, section=`rows` and id=the row key;
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

On an `analyze_period` capture, `get_read_item` section=`returnDays` id=YYYY-MM-DD
opens that closed day. Page `returnDays/<date>/composition`,
`returnDays/<date>/contributors` and `returnDays/<date>/issues`.
`get_read_item` section=`assetDrivers` id=the bucket key opens
`assetDrivers/<key>/byInstrument`, `assetDrivers/<key>/byAccount` and
`assetDrivers/<key>/residualDetails`. The date must fall inside the capture
range. Both item kinds accept `limit` (default 25, max 100) for initial rows
per child section within the response byte budget. Optional `sections` selects
short child names (`composition`, `contributors`, `issues` for days;
`byInstrument`, `byAccount`, `residualDetails` for drivers). Omitted selects all
children; `availableSections` lists unselected names. Follow remaining child
cursors with `get_read_page`. Bucket keys are the assetChange keys already returned:
external_flow, income, spending, dividend_interest, price_change, fx_impact,
fx_conversion_spread, fee, liability_impact, adjustment, residual. These
sections are frozen with the period capture and do not renew its TTL. They
are omitted from the parent JSON but count toward the retained-data budget.
Large day × instrument detail sets can fail `too_large`; narrow the query
explicitly and never silently drop evidence.

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

Period analysis now returns mcp-read/2: summary.income/expenses/assetChange/
investmentReturns contain headers; sections income/rows, expenses/rows,
assetChange/waterfall, assetChange/groups, investmentReturns/points and
investmentReturns/sources hold complete collections. Follow every relevant cursor.
Contribution-item, return-day and asset-driver detail also return summary headers
and independent evidence sections. Do not parse these as embedded GUI arrays.

## Choosing overlapping tools

Use the intent table's default, not several overlapping totals. Use
`get_historical_overview` for daily series or detailed historical leaf evidence;
`compare_financial_context` is the default two-state comparison. Expand a frozen
capture with the item tool that owns it: financial contexts use
`get_financial_context_item`, other captures use `get_read_item`.

For a single general instrument edit use `update_instrument`; for atomic
name/shortName/note edits across instruments use `update_instruments_batch` with
expected metadata versions. These are alternatives for one intent, not two
steps. Prefer structured tools for supported business writes; UI operations
must not bypass permission or retry an uncertain write through another route.
