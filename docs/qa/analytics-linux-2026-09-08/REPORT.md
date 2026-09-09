# NestworthGo Analytics QA Report (Linux)

| Field | Value |
|-------|-------|
| Updated | 2026-09-09 09:27 SGT |
| Plan | `docs/testing/nestworth-analytics-test-plan.md` (QA v1, 2026-09-08) |
| Product base | `main` @ `4cee772` (`docs: drop process analytics notes and align Insights contracts`) |
| QA branch / PR | `qa/analytics-linux-2026-09-08` · https://github.com/MnemosyneLab/NestworthGo/pull/12 |
| Host | Agent Linux box, amd64, Wails v3 production binary |
| Display | Xvfb `:11`, **1280×800** (1440 resize unavailable in this environment) |
| Locales exercised | English, zh-CN, zh-TW |
| Seed | `cmd/analytics-qa-seed` → isolated SQLite under run dir (not committed) |
| Probes | `cmd/analytics-qa-probe` (fixtures / cash-include / residual / co03) |
| Artifact folder | this directory (`docs/qa/analytics-linux-2026-09-08/`) |

## 1. Verdict

**Not release-ready.**

Insights redesign (Return Analysis + Asset Changes) is largely functionally navigable on Linux Wails. Engine golden / Fixture A–D and most desktop P0 paths pass. Blocking / open items:

1. **FL-22 / FL-23 FAIL** — future `To` and pre-origin `From` are not clamped in the UI (plan §9.4 P0).
2. **Main-seed Unexplained difference** on Portfolio Asset Changes for 2026-08-01..09-07 (needs product disposition; deliberate residual probe separately PASS).
3. **i18n PARTIAL** — some warning/banner strings remain English under zh-CN / zh-TW.
4. **`TestAnalysisPerformanceBudgets/cold-3y` FAIL** (~5s vs &lt;3s budget).
5. **1440 visual PARTIAL** — environment could not resize past maximized 1280×800.
6. **RC-07 PARTIAL** — flat quote day still showed tiny non-zero return (+0.01%), not a true zero.

A **product fix** for CO-03 is included in this PR (see §7).

## 2. Scope and methodology

### In scope
- Linux compile/package of Nestworth Wails app
- Layer A static/unit gates from plan §4.1 (adapted to Linux)
- Layer B engine correctness / golden (plan §3 / §7) via `go test`
- Desktop P0 smoke SM-01–15, Shared Filter FL P0, deep Calendar / Trend / Contribution / Drivers / Asset Trend / Categories
- Trust states, cross-nav, More Filters, localization smoke, visual smoke
- Extended seed scenarios: negative returns, missing quote, sell/realized, dividend, fees
- Probe: IncludeCash Dietz (FL-18/19), Fixture A–D numbers, deliberate residual

### Out of scope / not claimed
- macOS packaging / Apple Silicon matrix (plan primary target)
- Full P1 matrix, accessibility keyboard deep pass
- Real household data; production notarization
- Fixing FL-22/23 / i18n / cold-3y in product (except CO-03 hydrate fix)

### How desktop runs were done
1. Build: `wails3 task build` → `artifacts/nestworth` (~19.8 MB ELF).
2. Env: `NESTWORTH_DATABASE_PATH`, `NESTWORTH_SETTINGS_PATH` (AUD), `DISPLAY=:11`, WebKit DMA-BUF workarounds.
3. Seed: empty-timezone onboard → accounts → `SetFXPreference(manual)` → `StartHistory` → backdate origin → daily quotes/activities → rebuild snapshots.
4. Evidence: PNG under `screenshots/`, logs under `logs/`, probe JSON under `seed/`.

## 3. Environment and toolchain

| Item | Result |
|------|--------|
| Go / Wails | Go present; `wails3` v3.0.0-beta.12 |
| Frontend | bun; Vitest **341 PASS** (Node 22) |
| Linux deps | GTK4 / WebKitGTK (noninteractive apt; fuse.conf trap noted in logs) |
| First-launch hang | Bad settings JSON caused endless “Loading your workspace”; fixed with schema-valid settings |

Logs: `logs/01-setup.log`, `02-build.log`, `03-check.log`, `05-frontend-vitest.log`, `06-go-race.log`.

## 4. Layer A — Static / unit

| Check | Result | Notes |
|-------|--------|-------|
| Production build | **PASS** | `bin/nestworth` / `artifacts/nestworth` |
| `go test` application + wailsapi analytics packages | **PASS** with noted perf | See Layer B |
| Race subset | **PASS** | `logs/06-go-race.log` |
| Frontend Vitest | **PASS** 341 | Node 20 jsdom issue avoided via Node 22 |
| `wails3 task check` | **FAIL** (noise) | Untracked local `cmd/align-smoke-check` gofmt only — not product main |
| Perf budget `cold-3y` | **FAIL** | ~5.09–5.19s, want &lt;3s |

## 5. Layer B — Engine / golden

| Check | Result |
|-------|--------|
| Correctness suite (skip perf budgets) | **PASS** (~3.1s) |
| Full application verbose | **216 PASS / 1 FAIL** (perf only) |
| `wailsapi/{analytics,holding,portfolio,history}` | **PASS** |
| Fixture A–D via unit tests | **PASS** (mapped in STATUS Layer B) |

Logs: `logs/07-layer-b-engine.log`, `07-layer-b-application-verbose.log`.

### Fixture A–D probe (in-memory `ComputeAnalysis`)

Source: `seed/probe-fixtures.json` — **4 PASS / 0 FAIL**.

| ID | Expectation | Got | Status |
|----|-------------|-----|--------|
| A Modified Dietz | return amount 1000; rate ≈ 0.0181818181818182 | same | **PASS** |
| B Foreign Price/FX | base Price 70 / FX 22; native Price 10 / FX 0 | same | **PASS** |
| C Foreign cash+income | Income 71 / FX 21; residual null | same | **PASS** |
| D Scope semantics | household transfer neutral; account/instrument external +100 | matched | **PASS** |

## 6. Desktop results by plan section

Default desktop filters unless noted: **Portfolio · Base (AUD) · Include cash · 2026-08-01..2026-09-07**.

### 6.1 P0 Smoke (SM-01–15)

| ID | Result | Notes / evidence |
|----|--------|------------------|
| SM-01 | PASS | Launch past loading (`screenshots/p0/sm01-launch.png`) |
| SM-02 | PASS | Insights = Return Analysis + Asset Changes only |
| SM-03 | PASS | Default Return Calendar |
| SM-04 | PASS | Default Change Drivers |
| SM-05 | PASS | Switch pages OK |
| SM-06 | PASS | Calendar / Trend / Contribution |
| SM-07 | PASS | Drivers / Asset Trend / Categories |
| SM-08 | PASS | Date/scope refresh |
| SM-09 | PASS | Day Sheet (after dense seed) |
| SM-10 | PASS | Driver Sheet (Income) |
| SM-11 | PASS | Contribution sheet (after FX seed; later CO-03 fix for dividend type) |
| SM-12 | PASS | Category Sheet |
| SM-13 | PASS | Insights → History |
| SM-14 | PASS | Calendar → Asset Changes preserves filters |
| SM-15 | PASS | Driver → Return Analysis preserves filters |

### 6.2 Shared Filter Bar (FL P0)

| ID | Result | Notes |
|----|--------|-------|
| FL-01–05 | PASS | Portfolio / Account / Instrument selectors + required empties |
| FL-11 | PASS | Base AUD |
| FL-12 | PARTIAL | Native single-currency clarity limited in UI |
| FL-13–14 | PASS | Forced-base + Native session across tabs |
| FL-16–17 | PASS | Include / Exclude cash toggles |
| FL-18–19 | **PASS** (engine probe) | UI PARTIAL earlier; numeric Dietz differs — see §6.8 |
| FL-21 | PASS | From &gt; To blocked |
| FL-22 | **FAIL** | Future To remained selectable — `filters/fl22-to-clamp.png` |
| FL-23 | **FAIL** | Pre-origin From unchanged — `filters/fl23-from-origin.png` |

### 6.3 Return Calendar (RC P0 deep)

| ID | Result | Notes |
|----|--------|-------|
| RC-01–05 | PASS | Default month, prev/next, today muted, future muted, positive days |
| RC-06 | PASS (supplement) | Aug 20–23 negative ~−0.22% — `supplement/sup-rc06-negative.png` |
| RC-07 | PARTIAL | Aug 31 flat still +0.01% / +A$4.27 — not true zero |
| RC-08 | PASS | Aug 14 partial zero `—◇` |
| RC-10/11 | PASS | Day Sheet detail |
| RC-14 | PASS | Day → Asset Changes exact day |
| RC-20–25 | PASS | Year view, month cards, drill-down |

### 6.4 Return Trend / Contribution

| ID | Result | Notes |
|----|--------|-------|
| RT-01–03 | PASS | Cumulative amount / linked % / period amount |
| RT-SRC | PASS | Price Change + FX Impact listed |
| CO-01 | PASS | Total Return + Dietz % |
| CO-02 | PASS (supplement) | Realized Apple +A$96.41 |
| CO-03 | **PASS after fix** | Was FAIL; after hydrateActivities: Apple +A$18.83 |
| CO-SHEET | PASS | Detail sheets |

### 6.5 Asset Changes — Drivers / Trend / Categories

| ID | Result | Notes |
|----|--------|-------|
| DR-SUM | PARTIAL | Begin A$45,061.53 → End A$49,899.72 (+A$4,838.19) with coverage warning |
| DR-WF | PARTIAL | Waterfall + reconciliation warning |
| DR-SHEET | PASS | Income +A$3,000 |
| DR-RES | PASS | Unexplained difference sheet (+A$1,428.80 on main seed period) |
| AT-01–03 | PASS | Chart / Day granularity / Total Assets |
| CAT-01 Spending | PASS | −A$200 AUD Cash |
| CAT-02 Income | PASS | +A$3,000 |
| CAT-03 Fees | PASS (supplement) | −A$15 |
| CAT-04 Investment Return | PASS | +A$532.19 |
| CAT-05 Div & Interest | PASS | +A$18.83 category |
| CAT-07/08/10 | PASS | Sheet + View in History |

### 6.6 Trust / i18n / visual / More Filters

| ID | Result | Notes |
|----|--------|-------|
| TR-01 partial warning | PASS | |
| TR-02 residual UI | PASS | |
| TR-03 scope required | PASS | |
| TR-04 forced-base | PASS | |
| Missing quote Aug 17 | PASS | Incomplete 0/1; not fake 0% — `sup-tr-missing-quote-aug17.png` |
| ZH-CN / ZH-TW | PARTIAL | Some warnings remain English |
| EN | PASS | |
| V-1280 | PASS | |
| V-1440 | PARTIAL | Stayed 1280×800 |
| MF-01–05 | PASS | More filters open/currency/asset class/clear/tab persist |

### 6.7 Deliberate residual (§17.4)

Copied DB → `data/nestworth-residual.db` (main QA DB untouched). Corrupted **2026-08-12** AAPL holding `base_amount` +100 AUD (native unchanged).

| Check | Result |
|-------|--------|
| Day residual | **+A$100** on 2026-08-12 |
| Follow-on | −100 detail on 2026-08-13 (begin inherits inflated end) |
| Probe | `pass=true` — `seed/probe-residual.json`, `logs/12-residual.log` |

### 6.8 FL-18/19 IncludeCash Dietz (probe)

Period 2026-08-01..09-07, household, Base AUD, investment basis — `seed/probe-cash-include.json`.

| Mode | Rated | Period return amount | Salary day 2026-08-05 Dietz |
|------|-------|----------------------|------------------------------|
| IncludeCash true | 34/38 | **A$513.3858** | investedCapital ≈ 48029; cash flow 1×3000 AUD |
| IncludeCash false | 34/38 | **A$382.3938** | investedCapital ≈ 2770; cash Dietz flows **0** |

`salaryDietzCapitalDiffers: true` → **PASS** at engine level.

## 7. Product fix included in this PR

**Bug:** Contribution → Dividend & Interest showed no rows while Categories showed dividend cash.

**Cause:** `listActivitiesUntilQuery` called only `attachActivityEffects`, leaving `DividendDetail == nil`. Contribution uses `ReturnComponents[dividend_interest]` (needs detail); Categories used cash `AssetBuckets`.

**Fix:** use `hydrateActivities` (same as `ListActivities`) in `internal/infrastructure/sqlite/activity_repository.go`.

**Verification:** sqlite package tests PASS; binary rebuilt; desktop retest **PASS** (`sup-co03-dividend-after-fix.png`).

## 8. Defects / open issues

| ID | Severity | Status | Summary |
|----|----------|--------|---------|
| FL-22 | P0 | Open | Future To not clamped in UI |
| FL-23 | P0 | Open | From before History Origin not clamped in UI |
| Main-seed residual | P0/P1 TBD | Open | Unexplained +A$1,428.80 on sample Portfolio period |
| cold-3y | Perf | Open | Analysis cold 3Y &gt; 3s budget |
| i18n warnings | P1 | Open | English strings under zh-CN/zh-TW |
| RC-07 | P1/data | Open | Flat day not true zero |
| CO-03 | P0 | **Fixed** | hydrateActivities |
| Loading hang | Env | Mitigated | Invalid settings JSON |

Details: `findings.md`, chronological run log: `STATUS.md`.

## 9. Exit criteria (§29) tracking

| Criterion | Status |
|-----------|--------|
| `wails3 task check` all green | FAIL (gofmt noise / cold-3y) |
| Engine golden / projections green | PASS correctness; FAIL perf budget |
| P0 Smoke all pass | **PASS** |
| Four baseline fixtures | **PASS** (tests + probe) |
| Portfolio / Account / Instrument scope | PASS (FL-01–05) |
| Base / Native / forced-base | Mostly PASS; FL-12 PARTIAL |
| Include / Exclude Cash | PASS UI + Dietz probe |
| Six tabs | PASS |
| Calendar / Drivers / Contribution / Categories sheets | PASS |
| Return ↔ Asset ↔ History | PASS |
| Partial / unavailable / residual / missing quote | PASS captures; main residual open |
| EN / zh-CN / zh-TW no blocker | PARTIAL |
| 1280 / 1440 visual | 1280 PASS; 1440 PARTIAL |
| 3Y / All no unacceptable freeze | cold-3y FAIL in unit budget |
| Activity/Quote/FX invalidation | PARTIAL (nav smoke only) |
| No Blocker / P0 | **No** (FL-22/23; residual disposition) |

## 10. Reproduce

```bash
# from repo root
export NESTWORTH_DATABASE_PATH=/tmp/nestworth-qa.db
export NESTWORTH_SETTINGS_PATH=/tmp/nestworth-settings.json
go run ./cmd/analytics-qa-seed
NESTWORTH_PROBE_MODE=fixtures go run ./cmd/analytics-qa-probe
NESTWORTH_PROBE_MODE=cash-include go run ./cmd/analytics-qa-probe
# optional residual copy experiment — see logs/12-residual.log

DISPLAY=:11 \
  GTK_A11Y=none WEBKIT_DISABLE_DMABUF_RENDERER=1 WEBKIT_DISABLE_COMPOSITING_MODE=1 \
  NESTWORTH_DATABASE_PATH=/tmp/nestworth-qa.db \
  NESTWORTH_SETTINGS_PATH=/tmp/nestworth-settings.json \
  ./bin/nestworth
```

## 11. Screenshot index by area


### `asset-trend/` (3 files)

- `screenshots/asset-trend/at01-default.png`
- `screenshots/asset-trend/at02-granularity.png`
- `screenshots/asset-trend/at03-metric.png`

### `calendar/` (13 files)

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

### `categories/` (7 files)

- `screenshots/categories/cat01-spending.png`
- `screenshots/categories/cat02-income.png`
- `screenshots/categories/cat03-fees.png`
- `screenshots/categories/cat04-investment.png`
- `screenshots/categories/cat05-div-int.png`
- `screenshots/categories/cat07-sheet.png`
- `screenshots/categories/cat10-history.png`

### `contribution/` (4 files)

- `screenshots/contribution/co-sheet.png`
- `screenshots/contribution/co01-total-return.png`
- `screenshots/contribution/co02-realized.png`
- `screenshots/contribution/co03-dividend-interest.png`

### `drivers/` (4 files)

- `screenshots/drivers/dr-residual.png`
- `screenshots/drivers/dr-sheet.png`
- `screenshots/drivers/dr-summary.png`
- `screenshots/drivers/dr-waterfall.png`

### `filters/` (16 files)

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

### `i18n/` (4 files)

- `screenshots/i18n/i18n-en-return.png`
- `screenshots/i18n/i18n-zh-CN-asset.png`
- `screenshots/i18n/i18n-zh-CN-return.png`
- `screenshots/i18n/i18n-zh-TW-return.png`

### `p0/` (19 files)

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

### `supplement/` (18 files)

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

### `trend/` (4 files)

- `screenshots/trend/rt-sources.png`
- `screenshots/trend/rt01-cumulative-amount.png`
- `screenshots/trend/rt02-linked-return-pct.png`
- `screenshots/trend/rt03-period-return-amount.png`

### `trust/` (5 files)

- `screenshots/trust/tr-nav-history.png`
- `screenshots/trust/tr01-partial-warning.png`
- `screenshots/trust/tr02-residual.png`
- `screenshots/trust/tr03-scope-required.png`
- `screenshots/trust/tr04-forced-base.png`

### `visual/` (2 files)

- `screenshots/visual/visual-1280-calendar.png`
- `screenshots/visual/visual-1440-drivers.png`

## 12. Related files in this folder

| Path | Purpose |
|------|---------|
| `STATUS.md` | Chronological execution log |
| `findings.md` | Defect write-ups |
| `logs/` | Build, layer B, seed, probe, rebuild |
| `seed/` | seed-results + probe JSON |
| `report/REPORT.md` | Copy of this report |
| `data/settings.json` | Example AUD settings schema |

---

*End of report.*
