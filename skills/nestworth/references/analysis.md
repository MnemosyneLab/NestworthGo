# Current wealth, returns and asset-change explanations

Use `get_overview`, `get_account_valuations`, or `get_account_snapshot` for
**current** net worth/account values, holdings, costs and missing inputs. They
do not answer last month's income or investment return. Preserve ownership,
inclusion filters, household base currency and incomplete-value indicators.

For a period request call `analyze_period`. `query.from` / `to` are inclusive
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
| “哪几只资产贡献最大？” | `list_contributions` with returnType, groupBy, ordering and pagination |
| “这行收益为什么是这个数？” | `get_contribution_item`, exact returned row key and same query/returnType/groupBy |
| “某一天为什么亏损？” | `get_return_day`, date inside the same period |
| “净资产涨了，究竟是存钱、涨价还是汇率？” | Period assetChange buckets, then `get_asset_driver_detail` with an actual returned bucket key |
| “对应哪条交易？” | Follow history hints with `list_activities`, then `get_activity` |

Contribution return types are `total_return`, `realized`, `unrealized`,
`dividend_interest`; grouping is instrument/account/currency/asset_class.
Use the response's available/status/missingReason and ratedDays/totalDays.
The current backend can report unrealized attribution as unavailable; querying
that option is not proof it can be fully calculated.

Asset drivers include external flow, income, spending, dividend/interest, price,
FX impact, conversion spread, fees, liabilities, adjustments and residual.
Do not attribute residual to market performance without evidence. Reconcile
displayed contributors with authoritative totals; state a residual or partial
coverage rather than dropping it. Do not add per-account return percentages
or recompute an invented portfolio rate from rounded display values.

`list_activities` has inclusive local-date filters, account/instrument/kind
filters and bounded pagination. Continue with all three returned cursor fields
(`afterEffectiveAt`, `afterCreatedAt`, `afterId`); do not silently analyze only
the first page. Contribution pages similarly use offset/limit/hasMore.

Zero is a measured result; null/unavailable is missing evidence. Report partial
amounts as partial, preserve native/base distinctions and don't treat asset
change as investment profit. If missing evidence prevents the requested answer,
describe what is known and route to [data-health.md](data-health.md).
