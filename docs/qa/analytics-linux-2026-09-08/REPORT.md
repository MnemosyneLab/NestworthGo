# NestworthGo Analytics QA Report (Linux)

- **Updated:** 2026-09-09 07:42 SGT
- **Base:** `4cee772` · **Branch/PR:** `qa/analytics-linux-2026-09-08` / https://github.com/MnemosyneLab/NestworthGo/pull/12
- **Plan:** `docs/testing/nestworth-analytics-test-plan.md`

## Executive summary

| Area | Status |
|------|--------|
| Layer A/B | PASS (known cold-3y FAIL) |
| P0 Smoke / Filters / Tabs / Sheets | Mostly PASS; **FL-22/23 FAIL** |
| Trust / i18n / visual | Trust PASS; i18n PARTIAL; 1440 PARTIAL |
| Supplement seed gaps | RC-06/Trust quote/CO-02/Fees PASS; RC-07 PARTIAL |
| FL-18/19 Dietz IncludeCash | **PASS** (probe) |
| Fixture A–D | **PASS** (in-memory probe) |
| Deliberate Residual | **PASS** (+A$100 on 2026-08-12 residual DB) |
| More Filters MF-01–05 | **PASS** |
| **CO-03 dividend Contribution** | **PASS after product fix** (`hydrateActivities`) |

**Release sign-off: No** — remaining: FL-22/23 date clamp, i18n English warnings, unexplained residual on main seed period, cold-3y perf, 1440 env limit.

## Product fix in this PR

`internal/infrastructure/sqlite/activity_repository.go`: `listActivitiesUntilQuery` now calls `hydrateActivities` so analysis loads `DividendDetail` (fixes Contribution Dividend empty while Categories showed income).

## Key artifacts

- `seed/probe-fixtures.json`, `probe-residual.json`, `probe-cash-include.json`
- `logs/11-co03-div.txt`, `12-residual.log`, `13-rebuild-co03-fix.log`
- `screenshots/supplement/`

## Screenshot inventory (99)

- `screenshots/asset-trend/at01-default.png`
- `screenshots/asset-trend/at02-granularity.png`
- `screenshots/asset-trend/at03-metric.png`
- `screenshots/calendar/rc01-default-month.png`
- `screenshots/calendar/rc02-prev-next.png`
- `screenshots/calendar/rc03-today-muted.png`
- `screenshots/calendar/rc04-future-muted.png`
- `screenshots/calendar/rc05-positive.png`
- `screenshots/calendar/rc08-partial-zero.png`
- `screenshots/calendar/rc10-day-sheet.png`
- `screenshots/calendar/rc11-day-sheet-detail.png`
- `screenshots/calendar/rc14-day-to-asset.png`
- `screenshots/calendar/rc20-year-view.png`
- `screenshots/calendar/rc21-year-total.png`
- `screenshots/calendar/rc22-24-month-cards.png`
- `screenshots/calendar/rc25-month-drill.png`
- `screenshots/categories/cat01-spending.png`
- `screenshots/categories/cat02-income.png`
- `screenshots/categories/cat03-fees.png`
- `screenshots/categories/cat04-investment.png`
- `screenshots/categories/cat05-div-int.png`
- `screenshots/categories/cat07-sheet.png`
- `screenshots/categories/cat10-history.png`
- `screenshots/contribution/co-sheet.png`
- `screenshots/contribution/co01-total-return.png`
- `screenshots/contribution/co02-realized.png`
- `screenshots/contribution/co03-dividend-interest.png`
- `screenshots/drivers/dr-residual.png`
- `screenshots/drivers/dr-sheet.png`
- `screenshots/drivers/dr-summary.png`
- `screenshots/drivers/dr-waterfall.png`
- `screenshots/filters/fl01-portfolio.png`
- `screenshots/filters/fl02-account-scope.png`
- `screenshots/filters/fl03-account-required.png`
- `screenshots/filters/fl04-instrument-scope.png`
- `screenshots/filters/fl05-instrument-required.png`
- `screenshots/filters/fl11-base.png`
- `screenshots/filters/fl12-native.png`
- `screenshots/filters/fl13-forced-base.png`
- `screenshots/filters/fl14-native-session.png`
- `screenshots/filters/fl16-include-cash.png`
- `screenshots/filters/fl17-exclude-cash.png`
- `screenshots/filters/fl18-salary-include.png`
- `screenshots/filters/fl19-salary-exclude.png`
- `screenshots/filters/fl21-from-gt-to.png`
- `screenshots/filters/fl22-to-clamp.png`
- `screenshots/filters/fl23-from-origin.png`
- `screenshots/i18n/i18n-en-return.png`
- `screenshots/i18n/i18n-zh-CN-asset.png`
- `screenshots/i18n/i18n-zh-CN-return.png`
- `screenshots/i18n/i18n-zh-TW-return.png`
- `screenshots/p0/sm01-launch.png`
- `screenshots/p0/sm02-insights-nav.png`
- `screenshots/p0/sm03-return-calendar.png`
- `screenshots/p0/sm04-change-drivers.png`
- `screenshots/p0/sm05-switch.png`
- `screenshots/p0/sm06-calendar.png`
- `screenshots/p0/sm06-contribution.png`
- `screenshots/p0/sm06-trend.png`
- `screenshots/p0/sm07-asset-trend.png`
- `screenshots/p0/sm07-categories.png`
- `screenshots/p0/sm07-drivers.png`
- `screenshots/p0/sm08-date-scope.png`
- `screenshots/p0/sm09-day-sheet.png`
- `screenshots/p0/sm10-driver-sheet.png`
- `screenshots/p0/sm11-contribution-sheet.png`
- `screenshots/p0/sm12-category-sheet.png`
- `screenshots/p0/sm13-history-nav.png`
- `screenshots/p0/sm14-calendar-to-asset.png`
- `screenshots/p0/sm15-driver-to-return.png`
- `screenshots/supplement/mf-residual-sheet.png`
- `screenshots/supplement/mf01-more-filters-open.png`
- `screenshots/supplement/mf02-currency.png`
- `screenshots/supplement/mf03-asset-class.png`
- `screenshots/supplement/mf04-clear.png`
- `screenshots/supplement/mf05-tab-persist.png`
- `screenshots/supplement/sup-cat03-fees.png`
- `screenshots/supplement/sup-cat05-div.png`
- `screenshots/supplement/sup-co02-realized.png`
- `screenshots/supplement/sup-co03-dividend-after-fix.png`
- `screenshots/supplement/sup-co03-dividend-sheet-after-fix.png`
- `screenshots/supplement/sup-co03-dividend.png`
- `screenshots/supplement/sup-rc06-negative.png`
- `screenshots/supplement/sup-rc07-flat.png`
- `screenshots/supplement/sup-stale-still-ok.png`
- `screenshots/supplement/sup-tr-missing-quote-aug17.png`
- `screenshots/supplement/visual-1440-drivers.png`
- `screenshots/supplement/visual-1440-return.png`
- `screenshots/trend/rt-sources.png`
- `screenshots/trend/rt01-cumulative-amount.png`
- `screenshots/trend/rt02-linked-return-pct.png`
- `screenshots/trend/rt03-period-return-amount.png`
- `screenshots/trust/tr-nav-history.png`
- `screenshots/trust/tr01-partial-warning.png`
- `screenshots/trust/tr02-residual.png`
- `screenshots/trust/tr03-scope-required.png`
- `screenshots/trust/tr04-forced-base.png`
- `screenshots/visual/visual-1280-calendar.png`
- `screenshots/visual/visual-1440-drivers.png`