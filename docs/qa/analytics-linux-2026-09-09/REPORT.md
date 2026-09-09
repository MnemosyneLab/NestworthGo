# NestworthGo Analytics Linux QA Report — Round 2

| Field | Value |
|-------|-------|
| Product | NestworthGo (Wails v3 Insights / Analytics) |
| Repo | `/workspace/NestworthGo-fe-be-audit` |
| Commit under test | **367c3b0** — `fix(analytics): resolve QA findings` |
| Full SHA | `367c3b024eed9983bbd35f313508d21df4a287d2` |
| Commit date | 2026-09-09 14:20:53 +0800 (SGT) |
| QA date | **2026-09-09** (Asia/Singapore, UTC+8) |
| Platform | Linux **amd64** agent box |
| Display (desktop) | Xvfb / WebKitGTK; settings viewport **1280×719**; AUD; English + zh-CN exercised |
| Binary | Wails production ELF `binaries/nestworth` / `artifacts/nestworth` (**19 835 424** bytes ≈ **18.9 MiB** / ~19.8 MB, stripped) |
| Artifact root (run) | `/workspace/nestworth-analytics-qa-r2/` |
| Docs copy | `docs/qa/analytics-linux-2026-09-09/` |
| Prior round | `docs/qa/analytics-linux-2026-09-08/` (baseline `4cee772` + Round-1 findings) |
| Fixtures | `data/nestworth-complete.db`, `data/nestworth-missing-both.db` |
| Settings | `data/settings.json` (currency=AUD; language switched en ↔ zh-CN in desktop) |
| Supporting Layer A/B notes | `LAYER-AB.md` |

---

## 1. Verdict / 结论

**Overall: NOT release-blocker free.**

| Area | Result |
|------|--------|
| Build (`wails3 task build`) | **PASS** |
| Layer A — Vitest | **PASS** 344/344 |
| Layer A — Go `./...` | **FAIL** only `cold-3y` (~5.04 s want &lt;3 s) — **non-blocking** vs M3 Pro policy |
| Layer B — seeds / fixtures / probes | **PASS** |
| Desktop reverify (complete DB) | **8/8 PASS** |
| Deep gaps (complete DB) | **7/7 PASS** |
| Desktop incomplete UX (missing-both) | **OVERALL PARTIAL** — 4/5 PASS + **1 NEW P1-candidate defect** |

Prior Round-1 product findings (FL-22/23 date clamp, RC-07 true-zero, CO-03 dividend, friendly labels, Return % axis, zh-CN Insights strings, clean residual / no bad Residual) are **largely fixed and re-verified PASS** on **367c3b0**.

**Release hold:** missing-both fixture + Return Trend preset **All** surfaces a generic **"Insights could not be loaded"** error (see §9 / DEF-R2-02). Until that path is fixed or explicitly accepted, Round-2 is **not** release-blocker free.

---

## 2. Scope and methodology / 范围与方法

### In scope
- Linux production build of Nestworth Wails app at **367c3b0**
- Layer A: Vitest + `go test ./...`
- Layer B: analytics QA seeds (complete + missing-both), Fixture A–D, IncludeCash Dietz (FL-18/19), true-zero 2026-08-31, clean residual
- Desktop re-verify of Round-1 fix list on **complete** DB
- Deep-gap desktop cases (More Filters, Contribution group-by, Categories drill, empty state, calendar year/nav, History→Insights soft stale, Native valuation)
- Incomplete UX on **missing-both** DB (partial coverage, gap day Aug 17, Drivers warnings, zh-CN incomplete strings, labels)

### Out of scope / not claimed
- macOS / Apple Silicon matrix
- Full P1 accessibility keyboard deep pass
- Production notarization / real household data
- Git commit/push of this QA docs folder (parent owns)
- Declared root-cause of Trend **All** load error (hypothesis only — §9)

### How runs were done
1. Build: `wails3 task build` → binary under `binaries/` / `artifacts/` (~19.8 MB). Log: `logs/01-build.log`.
2. Unit: Vitest (`logs/10-vitest.log`); Go `./...` (`logs/11-go-test-key.log`).
3. Seed/probe: `cmd/analytics-qa-seed` + `cmd/analytics-qa-probe` against isolated DBs under `data/`.
4. Desktop: launch with `NESTWORTH_DATABASE_PATH` / `NESTWORTH_SETTINGS_PATH`; Xvfb; capture PNG + `RESULTS.json` under `screenshots/{reverify,deep,missing-both}/`.

Default complete-DB desktop filters unless noted: **Portfolio · Base (AUD) · Include cash · sample range ~2026-07-26..2026-09-08** (case-specific ranges in tables).

---

## 3. Environment and toolchain / 环境

| Item | Result |
|------|--------|
| Host | Linux amd64 agent box |
| Go / Wails | Go toolchain auto-selected for wails v3.0.0-beta.16 (go ≥1.25 → go1.26.8); `wails3 task build` **OK** |
| Frontend | bun; Vitest **v4.1.11**; **44** files / **344** tests |
| Linux UI | GTK4 / WebKitGTK; Xvfb |
| Viewport | settings `window_width=1280`, `window_height=719` |
| Currency / locales | AUD; English + 简体中文 |
| Artifact root | `/workspace/nestworth-analytics-qa-r2/` |

---

## 4. Build / 构建

| Item | Result | Evidence |
|------|--------|----------|
| `wails3 task build` | **PASS** | `logs/01-build.log` |
| Binary size | **19 835 424** bytes (~18.9 MiB / ~19.8 MB) | `binaries/nestworth` |
| Form | ELF 64-bit LSB executable, x86-64, stripped | file(1) |

**Build: PASS**

---

## 5. Layer A — Static / unit / 静态与单元

| Check | Result | Evidence |
|-------|--------|----------|
| Frontend Vitest | **PASS** — 44 files / **344** tests | `logs/10-vitest.log` |
| `go test ./... -count=1 -timeout 10m` | **FAIL** — 1 package only | `logs/11-go-test-key.log` |
| `internal/application` `TestAnalysisPerformanceBudgets/cold-3y` | **FAIL** — ~**5.04 s** want **&lt;3 s** | same; also `logs/02-perf-and-dietz.log` ~5.27 s prior |
| Other Go packages (sqlite, wailsapi/*, domain, …) | **PASS** | `logs/11-go-test-key.log`, `logs/03-sqlite.log` |

**Policy:** Linux `cold-3y` overshoot is a **known, non-blocking** gate versus the M3 Pro budget. Engine correctness is otherwise green.

**Layer A summary:** Frontend **PASS**. Go **PASS except known cold-3y**.

---

## 6. Layer B — Engine / golden / probes / 引擎与探针

| Check | Result | Evidence |
|-------|--------|----------|
| Seed **complete** | **26 PASS / 0 FAIL** | `seed/seed-results-complete.json`, `logs/04-seed-complete.log` |
| Seed **missing-both** | **26 PASS / 0 FAIL** | `seed/seed-results-missing-both.json`, `logs/05-seed-missing-both.log` |
| Fixture **A–D** (in-memory) | **4/4 PASS** | `seed/probe-fixtures.json`, `logs/06a-probe-fixtures.log` |
| FL-18/19 IncludeCash Dietz | **PASS** — rated 38/38 both; `salaryDietzCapitalDiffers=true`; include **A$562.8283** vs exclude **A$424.9383** | `seed/probe-cash-include.json` |
| True-zero day **2026-08-31** | **PASS** — amount=0, rate=0, status=ok on **both** DBs | `seed/probe-truezero-residual-*.json` |
| Clean residual (AssetChange) | **PASS** — `residualIssueCount=0`, drivers=0 on **both** DBs | same |
| Deliberate residual mode on clean DBs | expected `pass=false` (no corruption injected in r2); complete issues=0; missing-both NULL `base_amount` on gap item | `seed/probe-residual-*.json`, `logs/12-residual-*.log` |
| Missing-both verify | **PASS** — 45 snaps; incomplete **22**; gap day 2026-08-17 | `logs/07-missing-both-verify.log` |

### Fixture A–D detail

| ID | Name | Status |
|----|------|--------|
| A | Modified Dietz ~1.81818% | **PASS** |
| B | Foreign holding Price 70 / FX 22 base | **PASS** |
| C | Foreign cash Income 71 / FX 21 | **PASS** |
| D | Scope semantics instrument external +100 / transfer relative | **PASS** |

### IncludeCash (FL-18/19) numbers

| Mode | Period return amount | Rated days | Notes |
|------|---------------------|------------|-------|
| Include cash | **A$562.8283** | 38/38 | salary day 2026-08-05 cash Dietz flow A$3000 |
| Exclude cash | **A$424.9383** | 38/38 | no cash Dietz flows on salary day |

### True-zero confirm (Analyze 2026-08-01..2026-09-07)

| DB | 2026-08-31 amount | rate | Status |
|----|-------------------|------|--------|
| complete | **0** AUD | **0** | **PASS** |
| missing-both | **0** AUD | **0** | **PASS** (flat day outside gap 2026-08-17) |

**Layer B summary:** Seeds, fixtures, IncludeCash, true-zero, and clean residual all **PASS**.

More detail: `LAYER-AB.md`.

---

## 7. Desktop reverify (complete DB) — 8/8 PASS / 桌面回归

Source of truth: `screenshots/reverify/RESULTS.json` (`build: main@367c3b0`).

| ID | Case | Result | Notes / evidence |
|----|------|--------|------------------|
| 01 | Insights home / Return Analysis open | **PASS** | `screenshots/reverify/01-insights-home.png` |
| FL-22/23 | Date clamp (From/To pickers) | **PASS** | From: July 1–25 disabled, July 26–31 enabled; To: dates after last closed day disabled. Native picker prevents out-of-range. `02-from-picker.png`, `03-to-picker.png` |
| RC-07 | Aug 31 true-zero calendar | **PASS** | **0%** and **A$0.00** on 2026-08-31. `04-calendar-aug-zero-day.png` |
| CO-03 | Contribution dividend Apple | **PASS** | Range 2026-08-28..2026-09-08, Dividend & Interest → **Apple Inc +A$18.83**. `05-contribution-dividend.png` |
| P-04 | Friendly labels (no UUID) | **PASS** | Apple Inc visible; no raw UUID. `06-labels-not-uuid.png` |
| P-05 | Return % axis | **PASS** | Trend → Return %; axis labels sensible (+0.1%, 0%, −0.1%). `07-trend-pct-axis.png` |
| i18n | zh-CN Insights | **PASS** | 简体中文 via top-right language menu; headings/labels/legend Chinese; no visible leftover English. `08-zh-cn-insights.png` |
| Drivers | No bad Residual | **PASS** | Full sample 2026-07-26..2026-09-08: Cash flows + Market & Investment; **no Residual** row. Partial coverage banner OK (incomplete asset days). `09-drivers-no-bad-residual.png` |

**Desktop reverify: 8/8 PASS**

---

## 8. Deep gaps (complete DB) — 7/7 PASS / 深挖缺口

Source of truth: `screenshots/deep/RESULTS.json` (timestamp 2026-09-09T14:41:00+08:00). Summary: PASS=7, PARTIAL=0, FAIL=0.

| ID | Case | Result | Notes / evidence |
|----|------|--------|------------------|
| MF deep | More Filters intersections | **PASS** | Portfolio → Account/US Brokerage; Base; Include/Exclude cash; stock class. Ending value **A$32,132.16 → A$3,657.33**. Native option present. `mf-01-open.png`, `mf-02-intersection.png`, `mf-03-include-cash.png` |
| Contribution group-by | Group / sort | **PASS** | Instrument → asset class; sort Return rate high→low; stock **+A$434.47 / +12.18%**. Groupings: instrument, account, currency, asset class. `co-groupby.png` |
| Categories deep | Drill Apple | **PASS** | Asset Changes → Categories → Investment Return; Apple Inc total **+A$9.54**; account + dated records; no UUIDs. `cat-drill.png` |
| Empty / error-ish UX | Empty contribution | **PASS** | Single day 2026-09-07 + bond → clear empty copy: *No contribution data for this period. Return days are unavailable.* `empty-state.png` |
| Calendar year / nav | Year view + month | **PASS** | Year 2026 Partial coverage **39/45**, Issues (6); month nav to August loaded. `rc-year-or-nav.png` |
| Soft stale | History → Insights | **PASS** | History then back to Return Analysis; August reloaded (**+A$628.23, +1.29%**). `stale-or-nav-back.png` |
| Native valuation | Account US Brokerage | **PASS** | Return Trend → Native; **−$60.00 / −0.28%**; no fallback banner. `native-valuation.png` |

**Deep gaps: 7/7 PASS**

---

## 9. missing-both desktop — OVERALL PARTIAL / 不完整夹具

Source of truth: `screenshots/missing-both/RESULTS.json`. Fixture: incomplete_days=**22**, gap_day=**2026-08-17**. **overall: PARTIAL**.

| ID | Case | Result | Notes / evidence |
|----|------|--------|------------------|
| 01 | Insights partial coverage | **PARTIAL** | August shows Partial coverage **13/31** with Issues (18). **Selecting All in Return Trend → generic "Insights could not be loaded" error** — **NEW DEFECT** (DEF-R2-02). `01-insights-partial.png` |
| 02 | Gap day Aug 17 | **PASS** | 2026-08-17 drawer opens safely: amountStatus-style diamonds, A$0.00, 0/1, *Daily return is incomplete*. `02-gap-day-aug17.png` |
| 03 | Drivers incomplete | **PASS** | Partial coverage + *One or more asset days are incomplete* + reconciliation warning. `03-drivers-incomplete.png` |
| 04 | zh-CN incomplete warnings | **PASS** | 简体中文 localizes partial-coverage / incomplete-day warnings; **no leftover English "Missing quote"**. `04-zh-incomplete-warning.png` |
| 05 | Labels | **PASS** | Instruments shows friendly **Apple Inc**; no UUID. `05-labels.png` |

**missing-both desktop: OVERALL PARTIAL** (4 PASS + 1 PARTIAL / new defect)

---

## 10. Open defects / 未关闭缺陷

| ID | Severity | Title | Status | Notes |
|----|----------|-------|--------|-------|
| DEF-R2-01 | Known / non-blocking | Linux `cold-3y` perf budget | Open (carry) | ~5.04 s want &lt;3 s on Linux; M3 Pro policy treats as non-blocking. Evidence: `logs/11-go-test-key.log` |
| **DEF-R2-02** | **P1 candidate (NEW)** | missing-both + Return Trend preset **All** → generic load error | **Open — NEW** | UI shows *"Insights could not be loaded"* when All is selected on incomplete fixture. Partial coverage banner otherwise works. **Hypothesis (not proven root cause):** engine/API fails when the selected range includes many incomplete FX/quote days. Needs product triage / repro on shorter incomplete ranges. Evidence: `screenshots/missing-both/RESULTS.json`, `01-insights-partial.png` |

### Round-1 items re-verified closed (product)

| Prior finding | Round-2 status |
|---------------|----------------|
| FL-22/23 date clamp | **PASS** (reverify) |
| RC-07 true-zero Aug 31 | **PASS** (probe + calendar UI) |
| CO-03 dividend Apple | **PASS** |
| P-04 friendly labels | **PASS** (complete + missing-both) |
| P-05 Return % axis | **PASS** |
| zh-CN Insights / incomplete warnings | **PASS** (complete Insights; missing-both incomplete strings) |
| Bad Residual / unexplained on clean DB | **PASS** (issueCount=0; Drivers no Residual) |

---

## 11. Exit criteria / 退出标准

| Criterion | Met? | Notes |
|-----------|------|-------|
| Production Linux build succeeds | **YES** | ~19.8 MB binary |
| Vitest green | **YES** | 344/344 |
| Go correctness (excl. cold-3y policy) | **YES** | cold-3y known fail only |
| Seeds complete + missing-both | **YES** | 26/26 each |
| Fixtures A–D | **YES** | 4/4 |
| IncludeCash FL-18/19 | **YES** | amount delta confirmed |
| True-zero 2026-08-31 both DBs | **YES** | amount=0 rate=0 |
| Clean residual both DBs | **YES** | issueCount=0 |
| Desktop reverify of Round-1 fixes | **YES** | 8/8 |
| Deep gap coverage | **YES** | 7/7 |
| Incomplete UX (missing-both) clean | **NO** | Trend **All** generic load error (DEF-R2-02) |
| Release-blocker free | **NO** | Hold on DEF-R2-02 |

**Exit criteria summary:** Prior Round-1 product findings largely fixed and re-verified. Round-2 introduces **one new incomplete-fixture Trend All load error** → overall **NOT release-blocker free**.

---

## 12. Pass / fail counts (this round)

| Check | PASS | FAIL / PARTIAL / known |
|-------|------|------------------------|
| Vitest | 344 | 0 |
| Go `./...` packages | all but 1 | 1 (cold-3y only) |
| Seed complete | 26 | 0 |
| Seed missing-both | 26 | 0 |
| Fixtures A–D | 4 | 0 |
| True-zero (both DBs) | 2 | 0 |
| Clean residual (both DBs) | 2 | 0 |
| Desktop reverify | 8 | 0 |
| Deep gaps | 7 | 0 |
| missing-both desktop | 4 | 1 PARTIAL (DEF-R2-02) |

---

## 13. Screenshot index / 截图索引

Relative paths under `docs/qa/analytics-linux-2026-09-09/`.

### Reverify (complete DB)

| File | Case |
|------|------|
| `screenshots/reverify/01-insights-home.png` | Insights / Return Analysis home |
| `screenshots/reverify/02-from-picker.png` | FL-22/23 From clamp |
| `screenshots/reverify/03-to-picker.png` | FL-22/23 To clamp |
| `screenshots/reverify/04-calendar-aug-zero-day.png` | RC-07 Aug 31 0% / A$0 |
| `screenshots/reverify/05-contribution-dividend.png` | CO-03 Apple +A$18.83 |
| `screenshots/reverify/06-labels-not-uuid.png` | Friendly labels |
| `screenshots/reverify/07-trend-pct-axis.png` | Return % axis |
| `screenshots/reverify/08-zh-cn-insights.png` | zh-CN i18n |
| `screenshots/reverify/09-drivers-no-bad-residual.png` | Drivers no Residual |
| `screenshots/reverify/RESULTS.json` | Machine-readable results |

### Deep gaps

| File | Case |
|------|------|
| `screenshots/deep/mf-01-open.png` | More Filters open |
| `screenshots/deep/mf-02-intersection.png` | Scope intersection |
| `screenshots/deep/mf-03-include-cash.png` | Include cash filter |
| `screenshots/deep/co-groupby.png` | Contribution group-by |
| `screenshots/deep/cat-drill.png` | Categories drill |
| `screenshots/deep/empty-state.png` | Empty contribution UX |
| `screenshots/deep/rc-year-or-nav.png` | Calendar year / month nav |
| `screenshots/deep/stale-or-nav-back.png` | History → Insights soft stale |
| `screenshots/deep/native-valuation.png` | Native valuation |
| `screenshots/deep/RESULTS.json` | Machine-readable results |

### missing-both

| File | Case |
|------|------|
| `screenshots/missing-both/01-insights-partial.png` | Partial coverage + Trend All error (**DEF-R2-02**) |
| `screenshots/missing-both/02-gap-day-aug17.png` | Gap day incomplete UX |
| `screenshots/missing-both/03-drivers-incomplete.png` | Drivers incomplete warnings |
| `screenshots/missing-both/04-zh-incomplete-warning.png` | zh-CN incomplete warnings |
| `screenshots/missing-both/05-labels.png` | Apple Inc label |
| `screenshots/missing-both/RESULTS.json` | Machine-readable results |

### Other supporting docs in this folder

| File | Role |
|------|------|
| `REPORT.md` | This report |
| `STATUS.md` | One-page verdict |
| `LAYER-AB.md` | Layer A/B probe facts |

---

## 14. Logs & seed artifacts (run root)

Under `/workspace/nestworth-analytics-qa-r2/` (not all copied into docs):

| Path | Role |
|------|------|
| `logs/01-build.log` | Build |
| `logs/10-vitest.log` | Vitest 344 |
| `logs/11-go-test-key.log` | Go ./... + cold-3y |
| `logs/04-seed-complete.log` / `05-seed-missing-both.log` | Seeds |
| `logs/07-missing-both-verify.log` | Incomplete snap verify |
| `seed/probe-*.json` | Probe outputs |
| `binaries/nestworth` | Binary under test |

---

## 15. Recommendations for parent / 建议

1. **Triage DEF-R2-02 as P1 candidate** before calling analytics release-blocker free: reproduce Return Trend **All** on missing-both; capture API/engine error; compare vs shorter ranges that still show Partial coverage without hard fail.
2. Treat wording as **hypothesis** until confirmed: failure when range spans many incomplete FX/quote days.
3. Keep Linux `cold-3y` as known non-blocking (DEF-R2-01); do not conflate with DEF-R2-02.
4. Prior Round-1 product findings can be marked **verified fixed** on **367c3b0** for the complete-DB + probe matrix above.

---

*End of Round-2 report — commit **367c3b0**, 2026-09-09 SGT.*
