# NestworthGo Analytics QA Report (Linux)

- **When:** 2026-09-08 23:52 SGT
- **Base commit:** `4cee772` (`main`)
- **Plan:** `docs/testing/nestworth-analytics-test-plan.md`
- **Host:** Linux Wails, DISPLAY `:11` 1280×800
- **Seed:** `cmd/analytics-qa-seed`, `cmd/analytics-qa-probe`
- **Committed here:** screenshots, logs, STATUS, findings (no binary/DB)

## Executive summary

| Area | Status |
|------|--------|
| Layer A | PASS w/ known `cold-3y` FAIL |
| Layer B engine/golden | **PASS** |
| P0 Smoke SM-01–15 | **PASS** |
| Shared Filter | Mostly PASS; **FL-22/23 FAIL** |
| Calendar / Trend / Contribution | PASS (+ seed PARTIALs) |
| Asset Changes | PASS/PARTIAL; residual noted |
| Trust / i18n / visual | Trust PASS; i18n PARTIAL; 1440 PARTIAL |

**Release sign-off: No** (FL-22/23 P0, residual, i18n, perf).

## Reproduce seed

```bash
export NESTWORTH_DATABASE_PATH=/tmp/nestworth-qa.db
go run ./cmd/analytics-qa-seed
```

## Details

- `STATUS.md` — chronological run log
- `findings.md` — defects / seed issues
- `screenshots/` — evidence
- `logs/` — build/test/app logs

## Screenshot inventory (81)

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