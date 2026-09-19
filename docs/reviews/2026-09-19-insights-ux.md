# Insights interaction review — 2026-09-19

Scope: Asset Changes (Asset Trend, Change Drivers, Categories) and Return Trend.
Evidence: supplied native screenshot plus before/after screenshots of the real Wails
server build using an isolated disposable household. No production data or provider
requests were used. This is a focused UX review, not a full accessibility audit.

## Journey and findings

1. **Open Asset Changes — improved.** Previously Change Drivers was first and monthly
   sampling collapsed the nine-day fixture to one point. Asset Trend is now first
   and initially selected, with daily sampling. Explicit driver navigation remains.
2. **Understand the period — improved.** Blank date placeholders hid the effective
   range. Asset Changes and non-calendar Return Analysis tabs now display effective
   dates, while retaining date editing and history/closed-date limits. Return
   Calendar keeps its existing independent month/year navigation.
3. **Read the outcome — improved.** A shared summary shows beginning value, ending
   value, amount change and percentage change before the trend. Drivers shows the
   same summary. Balance values no longer have a misleading positive-change prefix.
   Percentages include cash flows and are explicitly distinguished from returns.
4. **Adjust the chart — improved.** Granularity and metric controls are in the asset
   chart header. Redundant intro dividers/control cards were removed. Chart colors
   use resolved theme colors; asset charts identify currency and rate axes use %.
5. **Choose a return period — improved.** Return Trend uses one set of shortcuts in
   the shared filter bar. Direct date inputs handle custom periods. Reset clears
   filters without switching tabs or changing calendar cursors.
6. **Inspect categories — improved.** The category selector is compact; an absent
   total no longer produces an unexplained “Total —” above an empty result.

## Financial contract

The backend derives change from actual query boundary values, not first/last
sampled chart points. Ratio = (ending − beginning) / beginning, rounded to twelve
decimal places for transport. Net worth, assets and liabilities use their own
boundary values. Flow and investment-return metrics retain their existing semantics.

Incomplete periods, missing boundaries and non-positive beginning values have no
change ratio and carry an explicit reason. The frontend formats the provided ratio;
it does not derive a replacement from partial amounts. No schema migration or
automatic market requests were introduced.

## Verification

- Full Go suite passed. Added application and DTO tests subsequently passed for
  cash flows, negative changes, zero/negative denominators, incomplete/unavailable
  days, missing boundaries, asset/liability separation, sampling independence and
  nullable decimal transport.
- Full frontend: 53 files / 396 tests passed; final focused insights run: 9 files /
  51 tests passed after adding effective-date coverage. Lint and final production
  build passed. Build reports the existing large ECharts chunk warning.
- 1280 × 900: default trend/day, effective dates, all nine daily rows, resolved chart
  color and summaries inspected. Fixture: USD 7,000 → 8,600, +1,600 / +22.86%; same
  percentage in monthly sampling and Change Drivers.
- 1000 × 640: summary wraps to two columns; document width 1000 / scroll width 1000,
  main width 776 / scroll width 776. Controls remain available and chart/data table
  can be reached by scrolling. The entire chart is not expected above the fold.
- Keyboard Enter switches to Drivers, resets without leaving Drivers, and opens
  the chart data table. These checks do not establish full keyboard/WCAG compliance.
- Native macOS window, VoiceOver, production packaging, signing, notarization and
  live providers were not tested for this increment.

## Screenshot evidence (local, disposable)

Before: `/tmp/nestworth-insights-qa/01-drivers-before.png` through
`04-returns-before.png`. After: `05-trend-after.png`, `06-drivers-after.png`,
`07-categories-after.png`, `08-returns-after.png`, `09-trend-narrow.png` in the same
directory. Screenshots precede the final balance-tooltip prefix cleanup.

## Further design opportunities

Keep as follow-up work: contextual links from a trend date to that day's drivers;
compare the selected period with the preceding equivalent period; show driver
contribution relative to beginning value with a distinct label. These need clear
comparison/boundary rules before adding more percentages. Return Calendar's
month/year navigation and the shared optional date filters merit a separate review.
