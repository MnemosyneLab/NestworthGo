# Visual Analytics and Market History

- Owner: Product, Design, and Analytics
- Status: **In progress**
- Baseline: Nestworth-go `0.3.0` / SQLite schema `9` / Wails v3 `v3.0.0-beta.16`
- Scope: chart surfaces and local quote history in Overview, Portfolio, Analytics, and Market Data
- Companion: [Domain Model](../architecture/domain-model.md) and [Data and Application Contracts](../architecture/data-and-ipc-contracts.md)

This document describes the current chart contract and the remaining work for
historical-series providers. It does not authorize network access when a page
opens. Local snapshots and quote observations are the first source for every
chart.

## 1. User questions and chart choices

The surfaces answer four separate questions:

1. **Overview:** Where are current assets concentrated, and what liabilities
   make up the balance sheet?
2. **Portfolio:** Which active Instrument-backed holdings are present, and what
   is their current valued subtotal?
3. **Analytics:** How have assets, liabilities, net worth, realized gains, and
   dividends changed over time?
4. **Market Data:** Which local Instrument and FX observations exist, from which
   source, and for what observation times?

| Relationship | Default chart | Avoid |
| --- | --- | --- |
| Current composition | Donut chart plus sorted detail | A line chart for a composition |
| Time change | Line chart | A pie chart for history |
| Category comparison | Horizontal bar chart | A wall of small pie charts |
| Positive and negative results | Zero-axis bar chart | Putting negative values in a donut |

## 2. Implemented surfaces

The current frontend already provides the following behavior:

- `EChart` renders the supported line, bar, and donut chart types with the
  existing theme and accessibility/data-table support.
- Overview and Portfolio expose backend composition results; their chart
  surfaces use those results rather than recomputing categories or totals.
- `AnalyticsPage` renders the net-worth trend and the multi-series, signed
  realized-gain, and dividend views from application queries.
- Portfolio trend data is available through the application read model and is
  rendered as a backend-authoritative time series.
- Local directional Instrument and FX quote-series reads are available for
  saved observations.
- `QuoteHistorySheet` presents local Instrument and FX quote history, including
  source, delayed state, observation time, and the selected FX direction.
- Page loading, empty, incomplete, and provider-error states remain separate;
  missing values are not converted into zero points.

The remaining in-progress area is external historical-series fetching. Yahoo
and Frankfurter currently provide latest values in production; a provider must
explicitly advertise any future daily-history capability before a fetch action
is shown.

## 3. Page contracts

### 3.1 Overview

Overview keeps the net-worth, asset, and liability cards. Composition charts
use the existing `assetsByType` and `liabilitiesByType` backend results:

- Assets and liabilities use separate charts and denominators.
- The center value is the valued subtotal, not an assumed complete total.
- Missing components are excluded from the valued subtotal and the missing
  count is visible.
- Categories with zero value are hidden from the chart; the accessible data
  table still exposes the complete returned result.
- Institution, member, group, and Account-type breakdowns remain compact
  lists unless a dedicated design reuses the same backend aggregate.

### 3.2 Portfolio

Portfolio is holdings-only. Its chart and trend include active
Instrument-backed holdings on active asset-role Accounts. Cash, liabilities,
archived Holdings, and Simple Account totals are excluded, and the persisted
`include_in_portfolio` field is ignored by the live metric.

The current composition view uses the Portfolio read model and may group by
Instrument type, native currency, country, or another backend-provided
allocation dimension. The frontend must not derive a second allocation from
current rows. Missing quote or FX inputs remain visible as incomplete and are
excluded from the valued subtotal rather than treated as zero.

Portfolio trend points contain a local date, valued subtotal, completeness, and
missing count. Closed days use persisted snapshot holding components; the
current day uses the live Portfolio read model. The frontend never reconstructs
history from current Holdings.

### 3.3 Analytics

Analytics keeps one global range control and combines:

- a wealth trend with assets, liabilities, and net worth as switchable series;
- a signed realized-gain bar chart with a zero baseline;
- a dividend-income bar chart; and
- expandable details that retain exact backend values.

The trend uses closed-day snapshots plus a current live point. If the current
day is also the last saved snapshot day, it appears once. If no closed day
exists, the UI explains that a trend starts after the next closed day instead
of drawing a misleading single-point line. Rebuild failures remain visible and
retryable.

Assets and liabilities are separate signed concepts: liabilities are shown as a
positive magnitude with an explicit liability label, while net worth is the
signed result returned by Go. Gains may be positive or negative and dividends
are non-negative income results. Unavailable inputs produce an explicit
unavailable or incomplete state.

### 3.4 Market Data and quote history

Each saved Instrument and FX pair can open a local history view. The history
sheet or page shows:

- Instrument name, symbol when present, quote currency, and source; or the
  selected FX direction such as `USD/CNY`;
- a line chart of the unit price or directional rate;
- quoted value, `quotedAt`, source kind, source key, and delayed state;
- bounded ranges such as `30d`, `1y`, and `all`;
- source filtering where supported; and
- a complete accessible data table ordered by observation time.

FX inverse values are calculated by Go and returned with the selected direction.
The frontend does not calculate reciprocal rates.

## 4. Local-series rules

The first stage reads only local immutable `InstrumentQuote` and `FXQuote`
observations:

- Page opening and ordinary range changes never call a provider.
- Identical observation times are selected deterministically by `quotedAt`,
  `createdAt`, and ID; the raw table can still show every stored observation.
- Missing dates remain gaps. The chart does not interpolate them or insert zero.
- Short ranges may show observations directly; longer ranges are reduced by the
  backend to the last observation for each local calendar day.
- Source kind, source key, and delayed status remain visible. A source change
  must not be presented as one uninterrupted provider history.
- Local quote history remains readable when the network is disabled or a
  provider is unavailable.

Application read models bound the range and target. A typical series contains:

```text
QuoteSeriesDTO
  range
  displayCurrency or pair direction
  points[]
    quotedAt
    value
    sourceKind
    sourceKey
    delayed
```

Instrument and FX series are directional and target-specific. They do not
silently broaden to every saved pair or every Instrument.

## 5. External historical providers

External history is a separate, planned capability. It may be added only after
the local series is stable:

- Show `Fetch history` only when the selected provider advertises the requested
  daily Instrument or FX capability.
- The user selects the range and explicitly starts the request.
- The UI reports fetched, skipped, failed, and rate-limited targets.
- Provider responses pass currency, price, rate, time, and response-size
  validation before becoming local append-only quote observations.
- The provider adapter does not write SQLite directly. A validated batch is
  committed atomically and uses target/source/observation/value idempotency.
- Existing manual observations are never overwritten. Required valuation targets
  run before optional extra pairs when rate limits apply.

Yahoo and Frankfurter must not be assumed to support historical endpoints from
their latest-value adapters alone.

## 6. Shared chart and accessibility rules

- Continue using the existing tree-shaken ECharts entry; do not add a second
  chart library.
- Use theme tokens consistently for assets, liabilities, gains, and losses.
- A donut center contains one main number and one range/subtotal explanation.
- Responsive layouts put the chart before the legend on narrow windows and do
  not require horizontal scrolling for key values.
- Every chart has a localized `aria-label`, a conclusion-oriented summary, and
  a view-data-table action.
- Legend, range, and dimension controls are keyboard-operable with visible
  focus. Color is never the only status encoding.
- Animation respects `prefers-reduced-motion`.
- Tooltips and data tables use canonical decimal strings from DTOs. Any numeric
  conversion used to place pixels is not a financial calculation.
- Donuts accept positive valued categories only. Negative values use bars.

## 7. State and validation boundary

| State | Required behavior |
| --- | --- |
| No Account or Holding | Keep the existing empty state and give the next creation action |
| One trend point | Explain that historical data is insufficient; retain the data table |
| No result in selected range | Say the range is empty and offer `all` |
| Partial valuation | Show available values, completeness, and missing count |
| No local quote history | Say that no local history exists; keep manual/refresh actions available |
| Provider lacks history | Hide the fetch action; local history continues to work |
| Provider fetch failure | Keep local charts and show a retryable error separately |
| Snapshot rebuild in progress | Show a skeleton or previous chart with an explicit rebuild state, never fake zeroes |

Go owns valuation, percentages, signed values, directional FX, trend points,
selection, and completeness. React owns layout, selection controls, chart
rendering, and accessible summaries. This document does not redefine the
financial rules in the [Domain Model](../architecture/domain-model.md) or the
persistence/provider boundary in [Data and Application Contracts](../architecture/data-and-ipc-contracts.md).

## 8. Remaining work

The remaining planned work is limited to provider-declared historical-series
fetching, including capability declarations, bounded requests, batch
validation, atomic append-only writes, and rate-limit handling. It must remain
explicit, local-first, and compatible with the current quote and valuation
contracts.
