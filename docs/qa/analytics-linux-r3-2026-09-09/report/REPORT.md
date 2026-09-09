# Nestworth Insights QA Report — Round 3 (Acceptance-grade)

| Field | Value |
|-------|-------|
| Product | NestworthGo (Wails v3 Insights / Analytics) |
| Repo | `/workspace/NestworthGo-fe-be-audit` |
| Branch | **`qa/analytics-linux-r3-2026-09-09`** |
| Commit under test | **`91d10c0`** — `feat(qa): analytics-linux-qa-v3 mini-family seed (#14)` |
| Full SHA | `91d10c0216d31d274857e9bbf71d971dd6b8c7bd` |
| Commit date | 2026-09-09 15:43:36 +0800 (SGT) |
| Lineage (DEF-R2-02 fix) | Soft-FX classifier path present since **`ea94f48`** lineage (Round-2 docs @ `367c3b0` fix set; classifier `convertValuationAmount` soft-missing-FX; sibling coverage `85f7f8b`) |
| QA date | **2026-09-09** (Asia/Singapore, UTC+8) |
| Platform | Linux **amd64** agent box |
| Display (desktop) | Xvfb / WebKitGTK; settings viewport **1280×719**; AUD; English (+ zh-CN planned for incomplete matrix) |
| Binary | Wails production ELF `binaries/nestworth` / `artifacts/nestworth` (**19 835 424** bytes ≈ **18.9 MiB** / ~19.8 MB, stripped) |
| Artifact root (run) | `/workspace/nestworth-analytics-qa-r3/` |
| Docs copy | `docs/qa/analytics-linux-r3-2026-09-09/` |
| Prior round | Round-2 `docs/qa/analytics-linux-2026-09-09/` (reverify+deep PASS; **DEF-R2-02** open) |
| Fixture version | **`analytics-linux-qa-v3`** (seed mini-family) |
| Scenario DBs | `data/nestworth-{complete,missing-price,missing-fx,missing-both}.db` |
| Settings | `data/settings.json` (currency=AUD; window 1280×719) |
| Plan | `PLAN.md` / `report/PLAN.md` |
| Supporting notes | `report/DEF-R2-02.md`, `report/LAYER-R3.md`, `logs/A-phase-summary.md`, `seed/B-SUMMARY.md`, `probes/C-phase.md` |

> **Remediation update (2026-09-09, authoritative for this working tree):** R3-D5 is fixed in the application attribution/precision path and the fixed v3 complete probe reports `ending - beginning - sum(waterfall) = 0 AUD` with zero residual issues. R3-D6 is fixed in realized HistoryHint, Wails DTO mapping, and frontend navigation coverage. C5–C8 quantitative probes and C9 clean/corrupt residual controls are **9/9 PASS**; see `probes/REMEDIATION-2026-09-09.json`. The D5/D6 screenshots below are historical evidence from before these fixes, not a claim that native Wails replay has been completed. Missing-price, missing-fx, zh-CN incomplete-matrix checks, and GUI-level D8 mutation replay remain **NOT_RUN**.

---

## 1. Verdict / 结论

**Overall: AUTOMATED REMEDIATION GREEN; NATIVE DESKTOP REPLAY PENDING — DEF-R2-02 CLOSED; Linux cold-3y remains a known non-blocking observation.**

| Area | Result |
|------|--------|
| Build (`wails3` / native go build) | **PASS** (`BUILD_EXIT:0`) |
| Layer A — Go application suite | **FAIL only** `cold-3y` (~5.08 s want &lt;3 s) — **known non-blocking** (DEF-R2-01) |
| Classifier / MissingFX / Return cases | **PASS** (targeted logs) |
| Phase A — DEF-R2-02 harness | **PASS** — FIXED on probe + desktop |
| Phase B — Seed v3 four scenarios | **PASS** — **4/4 OVERALL**; each **35 PASS / 0 FAIL** |
| Phase C — Probe pack C1–C9 | **9 PASS / 0 FAIL / 0 SKIP** after remediation |
| Phase D — Desktop D1–D8 (complete) | Historical **6 PASS / 2 PARTIAL**; corrected D5/D6 native replay not run |
| Desktop incomplete matrix (D7 plan) | **PARTIAL/PENDING** — missing-both evidence exists; missing-price/missing-fx/zh-CN not rerun |
| DEF-R2-02 (Trend All on missing-both) | **FIXED** — probe partial 21/45 no panic; desktop chart + Partial coverage, no generic load error |

**Green acceptance** (PLAN §3) additionally required DEF-R2-02 fixed → **met**. The remaining acceptance gate is native Wails replay for corrected D5/D6, the missing-price/missing-fx/zh-CN desktop matrix, and GUI-level D8 mutation evidence.

---

## 2. Scope and methodology / 范围与方法

### In scope
- Linux production Insights path at **`91d10c0`** (seed v3) with DEF-R2-02 fix lineage from **`ea94f48`** / product soft-FX path
- Phase A: automated Trend All / preset repro on missing-both + complete; defect card; desktop Trend All confirm
- Phase B: one narrative household × four completeness DBs (`complete` / `missing-price` / `missing-fx` / `missing-both`)
- Phase C: probe pack C1–C9 against scenario DBs (JSON-first)
- Phase D: desktop acceptance D1–D8 on complete; DEF-R2-02 desktop confirm on missing-both
- Full report + STATUS (this document)

### Out of scope / not claimed
- macOS / Apple Silicon matrix
- Full timezone / DST matrix (Round-4)
- 1440 / responsive deep visual
- Real household data / notarization
- Loan (FC-07) stretch; deliberate residual corrupt DB
- Native Wails replay on this remediation host
- Git commit / push of docs (explicitly not done this pass)

### Methods / 方法
1. **Seed + probe first**, desktop second — assert coverage/amounts/status in JSON; screenshots for UX / deep-link / defect confirm.
2. Build: `wails3` Linux native → `binaries/nestworth` (~19.8 MB). Log: `logs/02-build.log`.
3. Go targeted: cold-3y / Classification / MissingFX / Return cases — `logs/01-go-analysis.log`, `logs/01b-*.log`, `logs/01c-*.log`, `logs/01d-*.log`.
4. Seed: `cmd/analytics-qa-seed` with `NESTWORTH_QA_SCENARIO=…` → `seed/seed-results-*.json`, `seed/B-SUMMARY.md`.
5. Probes: `cmd/analytics-qa-probe` modes `fixtures` / `cash-include` / legacy / `residual` / `co03` / `reconcile` / `quantitative` → `probes/SUMMARY.json`, `probes/C-phase.md`, `probes/REMEDIATION-2026-09-09.json`.
6. Desktop: launch with `NESTWORTH_DATABASE_PATH` / `NESTWORTH_SETTINGS_PATH`; capture PNG + `RESULTS.json` under `screenshots/{def-r2-02,desktop-d,incomplete}/`.

Default complete-DB desktop filters unless noted: **Portfolio · Base (AUD) · Include cash · sample ranges around Aug–Sep 2026** (case-specific in Phase D table).

---

## 3. Environment and toolchain / 环境

| Item | Result |
|------|--------|
| Host | Linux amd64 agent box |
| Repo / branch | `NestworthGo-fe-be-audit` @ `qa/analytics-linux-r3-2026-09-09` |
| Under test | **`91d10c0`** (seed v3); DEF-R2-02 soft-FX in lineage including **`ea94f48`** |
| Go / Wails | Wails v3 production tags; `Version=v0.3.1` `Build=2` in ldflags |
| Frontend | bun + Vite build inside `wails3` task |
| Linux UI | GTK4 / WebKitGTK; Xvfb |
| Viewport | settings `window_width=1280`, `window_height=719` |
| Currency / locale | AUD; English (zh-CN incomplete matrix PENDING) |
| Anchor / window | History origin **2026-07-26**; rebuild **45** snapshots `2026-07-26..2026-09-08` |
| Artifact root | `/workspace/nestworth-analytics-qa-r3/` |

---

## 4. Build / 构建

| Item | Result | Evidence |
|------|--------|----------|
| Linux native build | **PASS** — `BUILD_EXIT:0` | `logs/02-build.log` |
| Binary size | **19 835 424** bytes (~18.9 MiB / ~19.8 MB) | `binaries/nestworth`, `artifacts/nestworth` |
| Form | ELF 64-bit LSB executable, x86-64, stripped | file(1) |

**Build: PASS**

---

## 5. Layer A — Static / unit / 静态与单元

| Check | Result | Evidence |
|-------|--------|----------|
| `TestAnalysisPerformanceBudgets/cold-3y` | **FAIL** — ~**5.08 s** want **&lt;3 s** | `logs/01-go-analysis.log` |
| Missing FX classification / Return cases (targeted) | **PASS** | `logs/01b-go-missing-fx-classification.log`, `logs/01c-go-all-return.log`, `logs/01d-targeted.log` |
| `TestAnalysisReturnMissingFXOnAssociatedCashDoesNotFailPeriod` | **PASS** | `logs/01d-targeted.log` |

**Policy:** Linux `cold-3y` overshoot remains **known, non-blocking** (DEF-R2-01 carry). Engine correctness otherwise green on targeted suites.

**Layer A summary:** Go **PASS except known cold-3y**. Soft-FX / missing-FX return path **PASS** (supports DEF-R2-02 fix).

---

## 6. Phase A — DEF-R2-02 harness / 缺陷复现与确认

**Goal:** Pin Trend **All** on missing-both as automated case; confirm fix.

| ID | Work | Outcome | Evidence |
|----|------|---------|----------|
| A1 | missing-both + All (origin→last closed) | **PASS** — coverage **partial 21/45**, exit 0, `panicked=false` | `logs/03-probe-missing-both-all.out`, `logs/03-probe-exit.txt`, `logs/03-probe-summary.json` |
| A2 | Trend presets × missing-both + complete | **PASS** — see table below | `probes/A-trend-presets.json`, `logs/A-trend/*` |
| A3 | Desktop Trend All (+ 30D) | **PASS** — chart renders; Partial 21/45 (All) / 21/30 (30D); **no** generic load error | `screenshots/def-r2-02/RESULTS.json`, `01-trend-all.png`, `02-trend-30d-or-mid.png` |
| A4 | Defect card | **Written / updated FIXED** | `report/DEF-R2-02.md` |

### A2 preset matrix

| Scenario | Preset | Range | Result | Coverage | Panic |
|----------|--------|-------|--------|----------|-------|
| missing-both | All | 2026-07-26..2026-09-08 | **partial** | 21/45 | false |
| missing-both | Custom mid | 2026-08-01..2026-09-07 | **partial** | 20/38 | false |
| missing-both | Approx30D | 2026-08-10..2026-09-08 | **partial** | 21/30 | false |
| complete | All | 2026-07-26..2026-09-08 | **partial*** | 39/45 | false |
| complete | Custom mid | 2026-08-01..2026-09-07 | **ok** | 38/38 | false |
| complete | Approx30D | 2026-08-10..2026-09-08 | **ok** | 30/30 | false |

\*complete All still partial due to origin UNAVAIL + early Jul partial days in seed window (expected shape).

**Limitations:** YTD / 1Y / 3Y UI presets not separately probed (fixture window ~45 days). Approx30D is inclusive last-30-closed-days probe construction, not a verified UI calendar mapping.

**Exit A:** DEF-R2-02 reproducible without GUI and **FIXED** on probe **and** desktop. See `report/DEF-R2-02.md`.

---

## 7. Phase B — Seed v3 mini-family / 种子数据

**When:** 2026-09-09 ~15:44 SGT  
**Commit:** `91d10c0` (fixture `analytics-linux-qa-v3`)  
**Summary:** `seed/B-SUMMARY.md`

### Overall

| Scenario | OVERALL | Checks | DB |
|----------|---------|--------|----|
| complete | **PASS** | **35 PASS / 0 FAIL** | `data/nestworth-complete.db` |
| missing-price | **PASS** | **35 PASS / 0 FAIL** | `data/nestworth-missing-price.db` |
| missing-fx | **PASS** | **35 PASS / 0 FAIL** | `data/nestworth-missing-fx.db` |
| missing-both | **PASS** | **35 PASS / 0 FAIL** | `data/nestworth-missing-both.db` |

**All four: 4/4 OVERALL PASS.**

### Completeness / gap / true-zero

Gap day = **2026-08-17** (offset 22). True-zero day = **2026-08-31** (offset 36).

| Scenario | incomplete_days | gap complete | gap missing_items | true-zero (Aug 31) |
|----------|----------------:|-------------:|------------------:|--------------------|
| complete | 0 | 1 | 0 | **PASS** — status=ok amount=0 rate=0 |
| missing-price | 18 | 0 | 3 | **PASS** — status=ok amount=0 rate=0 |
| missing-fx | 22 | 0 | 5 | **PASS** — status=ok amount=0 rate=0 |
| missing-both | 22 | 0 | 5 | **PASS** — status=ok amount=0 rate=0 |

### Fixture shape (all scenarios)

- **Accounts:** AUD Cash, US Brokerage, SG Brokerage  
- **Instruments:** AAPL (USD), QQQ (USD), ES3 (SGD)  
- **Activities:** contrib / transfer / buy / sell / salary / interest / dividend / fee (v3 mini-family)  
- **Rebuild:** 45 snapshots `2026-07-26..2026-09-08`

**Exit B:** Four scenario DBs build cleanly from one seed story; IDs in `seed/seed-results-*.json`.

---

## 8. Phase C — Probe pack / 探针包

**When:** 2026-09-09 15:46 SGT  
**Against:** complete DB unless noted; missing-both for C4  
**Counts:** **9 PASS / 0 FAIL / 0 SKIP** for the automated probe pack; native desktop checks are separate NOT_RUN evidence.
**Artifacts:** `probes/SUMMARY.json`, `probes/C-phase.md`

| ID | Name | Status | Detail / artifact |
|----|------|--------|-------------------|
| C1 | Fixtures A–D | **PASS** | 4 PASS / 0 FAIL — `seed/probe-fixtures.json` |
| C2 | IncludeCash FL-18/19 | **PASS** | salaryDay=2026-08-05; investedCapital include=`56825.49` vs exclude=`2770.821`; cashDietzFlows include=1(3000) exclude=0 — `seed/probe-cash-include.json` |
| C3 | True-zero Aug 31 | **PASS** | sample_zero includes `2026-08-31 rate=0 status=ok`; All-range complete still Analyze `status=partial` (origin UNAVAIL) — `logs/C3-true-zero-complete.out` |
| C4 | missing-both All | **PASS** | partial **21/45**, exit 0, no panic (DEF-R2-02 regression) — `logs/C4-missing-both.out` |
| C5 | Scope triangulation | **PASS** | quantitative mode uses independent boundary snapshot totals; household/account/instrument + explicit cash delta=0 |
| C6 | Native vs Base | **PASS** | quantitative mode verifies mixed-currency forced Base and instrument Native quote currency |
| C7 | Return sources sum | **PASS** | quantitative mode verifies Price/FX/Dividend/Fee source sum and coverage |
| C8 | Transfer neutrality FC-01 | **PASS** | quantitative mode discovers persisted transfer day and verifies household external_flow=0 |
| C9 | Residual corrupt day | **PASS** | clean copy issues=0; corrupt copy +100 AUD retains ±100 AUD details and native unchanged |

**Existing modes:** `fixtures`, `cash-include`, `co03`, `residual`, legacy/default.  
**No modes yet for:** scope triangulation, native/base forced flag, return-sources reconciliation, transfer-day ≈0.

**Exit C:** **9/9 PASS**, no unexpected FAIL or SKIP. Raw compact evidence: `probes/REMEDIATION-2026-09-09.json`.

---

## 9. Phase D — Desktop acceptance / 桌面验收

### D1–D8 on complete (seed v3)

Source: `screenshots/desktop-d/RESULTS.json`.

| ID | Case | Status | Summary | Screenshots |
|----|------|--------|---------|-------------|
| D1 | Smoke — Return / Asset Changes | **PASS** | Return Calendar opened at September 2026 default; switched to Asset Changes → Change Drivers and back | `d1-return.png`, `d1-asset.png` |
| D2 | Scope — Account / Instrument | **PASS** | Account → US Brokerage refreshed totals; Instrument → Apple Inc refreshed totals | `d2-account.png`, `d2-instrument.png` |
| D3 | Include Cash + Categories Income | **PASS** | Cash Exclude/Include toggled; Categories → Income with Include cash showed salary **+A$3,000.00** (partial coverage noted) | `d3-cash.png` |
| D4 | Contribution Realized + Dividend | **PASS** | Realized: Apple Inc **+A$96.41**, Invesco QQQ Trust **+A$15.02**; Dividend & Interest by Asset Class: Cash **+A$500.00**, stock **+A$18.83** | `d4-realized.png`, `d4-dividend.png` |
| D5 | Drivers waterfall | **AUTOMATED FIX / NATIVE REPLAY PENDING** | Historical screenshot had the warning. Fixed probe: exact Aug 1–31 `delta=0 AUD`, residual issues=0; corrected screenshot not captured | `d5-drivers.png`, `probes/REMEDIATION-2026-09-09.json` |
| D6 | History deep-link | **AUTOMATED FIX / NATIVE REPLAY PENDING** | Historical screenshot had All filters. Realized HistoryHint + Wails DTO + frontend payload tests now pass; corrected screenshot not captured | `d6-history.png`, `probes/REMEDIATION-2026-09-09.json` |
| D7 | True-zero day (calendar) | **PASS** | Return Calendar August 2026 day **31** showed exactly **0%** and **A$0.00** | `d7-zero.png` |
| D8 | Soft invalidation / back | **PASS** | History opened, then Return Analysis reloaded successfully with August 2026 summary | `d8-back.png` |

**Desktop complete summary:** Historical **6 PASS / 2 PARTIAL**; D5/D6 automated remediation is green, but native replay remains pending. No unexplained crash.

### DEF-R2-02 desktop (missing-both)

Source: `screenshots/def-r2-02/RESULTS.json`.

| Case | Status | Notes | Screenshots |
|------|--------|-------|-------------|
| Trend All | **PASS / FIXED** | Cumulative-return chart rendered; **Partial coverage 21/45**; no generic *"Insights could not be loaded"* | `01-trend-all.png` |
| Trend 30D | **PASS** | Chart rendered; **Partial coverage 21/30** | `02-trend-30d-or-mid.png` |

### Incomplete matrix desktop (plan D7 multi-scenario)

| Item | Status |
|------|--------|
| `screenshots/incomplete/RESULTS.json` | **MISSING** |
| Folder | Contains the existing missing-both shallow run and recheck screenshots |
| Verdict | **PARTIAL / PENDING** — missing-price / missing-fx / zh-CN / correct Change Drivers page were not executed in this remediation turn |

---

## 10. Defects / 缺陷登记

| ID | Severity | Title | Status | Notes / evidence |
|----|----------|-------|--------|------------------|
| **DEF-R2-01** | Known / non-blocking | Linux `cold-3y` perf budget | **Open (carry)** | ~5.08 s want &lt;3 s on Linux; M3 Pro policy non-blocking. `logs/01-go-analysis.log` |
| **DEF-R2-02** | Was P1 release hold | missing-both + Return Trend **All** → generic Insights load error / probe panic | **FIXED** | Probe: partial 21/45, no panic. Desktop: chart + Partial 21/45, no generic error. Fix: `convertValuationAmount` soft-missing-FX. Card: `report/DEF-R2-02.md`. Evidence: `logs/03-probe-*`, `probes/A-trend-presets.json`, `screenshots/def-r2-02/*` |
| **R3-D5** | Attribution/precision | Change Drivers waterfall reconcile warning | **FIXED in code; native replay pending** | Exact v3 probe delta=0 and residual issues=0; old screenshot remains historical. |
| **R3-D6** | Deep-link contract | History deep-link leaves account/instrument filters as **All** | **FIXED in code/tests; native replay pending** | Realized HistoryHint, DTO mapping, and frontend navigation payload are covered. |

No new P0/P1 crash-class defects in Round-3 desktop/probe runs.

---

## 11. Exit criteria / 退出标准

From PLAN §3 (Round-3 acceptance-grade):

| # | Criterion | Met? | Comment |
|---|-----------|------|---------|
| 1 | Four scenario DBs build cleanly from one seed story | **YES** | 4/4 OVERALL PASS; 35/35 each |
| 2 | Probe pack C1–C9 no unexpected FAIL | **YES** | **9 PASS / 0 FAIL / 0 SKIP** after remediation; DEF-R2-02 remains fixed |
| 3 | Desktop D1–D8 executed with RESULTS | **PARTIAL** | Historical D1–D8 results exist; corrected D5/D6 and missing-price/missing-fx/zh-CN native replay remains pending |
| 4 | Scope / transfer / multi-instrument ≥1 quantitative probe each | **YES** | C5/C8 quantitative probes pass; instrument + cash partition is explicit |
| 5 | Report published on branch/PR | **DOCS WRITTEN** | This REPORT + STATUS under artifact root + `docs/qa/analytics-linux-r3-2026-09-09/`; **no git commit/push this pass** |

**Green acceptance** additional requirement: DEF-R2-02 fixed or waived → **YES (FIXED)**.

### Exit summary / 退出摘要

- **DEF-R2-02 closed** (probe + desktop) — Round-2 release hold lifted for that defect.
- Seed v3 mini-family + four completeness variants are **acceptance-ready**.
- Automated probe pack C1–C9 is green; the raw compact result is `probes/REMEDIATION-2026-09-09.json`.
- D5 reconcile warning and D6 History filter All are fixed in code/tests; replay the native app before closing the desktop evidence gate.
- Finish **incomplete-matrix** desktop missing-price/missing-fx/zh-CN and correct Change Drivers page before claiming full D7.
- Linux **cold-3y** remains known non-blocking.

**Verdict label:** **AUTOMATED REMEDIATION GREEN / DESKTOP REPLAY PENDING** (DEF-R2-02, D5/D6 code paths, C5–C9 probes fixed; remaining gaps are explicitly native GUI evidence).

---

## 12. Screenshot index / 截图索引

### DEF-R2-02 (`screenshots/def-r2-02/`)

| File | Case |
|------|------|
| `01-trend-all.png` | missing-both Trend All — Partial 21/45, chart OK |
| `02-trend-30d-or-mid.png` | missing-both Trend 30D — Partial 21/30, chart OK |
| `RESULTS.json` | Verdict PASS / FIXED |

### Desktop D (`screenshots/desktop-d/`)

| File | Case |
|------|------|
| `d1-return.png` | D1 Return Calendar |
| `d1-asset.png` | D1 Asset Changes / Drivers entry |
| `d2-account.png` | D2 Account scope US Brokerage |
| `d2-instrument.png` | D2 Instrument Apple Inc |
| `d3-cash.png` | D3 Include Cash / Income |
| `d4-realized.png` | D4 Realized Gain |
| `d4-dividend.png` | D4 Dividend & Interest |
| `d5-drivers.png` | Historical D5 Change Drivers waterfall (pre-remediation PARTIAL screenshot) |
| `d6-history.png` | Historical D6 History deep-link (pre-remediation PARTIAL screenshot) |
| `d7-zero.png` | D7 True-zero Aug 31 |
| `d8-back.png` | D8 Back to Insights |
| `RESULTS.json` | D1–D8 statuses |

### Incomplete matrix (`screenshots/incomplete/`)

| File | Case |
|------|------|
| *(none)* | **PENDING** — directory empty; no `RESULTS.json` |

---

## 13. Evidence path index / 证据路径

| Path | Contents |
|------|----------|
| `PLAN.md` | Round-3 plan |
| `report/REPORT.md` | This report |
| `report/STATUS.md` | One-pager |
| `report/DEF-R2-02.md` | Defect card (FIXED) |
| `report/LAYER-R3.md` | Layer A/B/C notes |
| `logs/A-phase-summary.md` | Phase A summary |
| `logs/01-go-analysis.log` | cold-3y FAIL |
| `logs/01b-*.log` / `01c-*.log` / `01d-*.log` | Targeted Go PASS |
| `logs/02-build.log` | Build PASS |
| `logs/03-probe-missing-both-all.*` | DEF-R2-02 probe |
| `logs/A-trend/*` | Preset matrix logs |
| `logs/B-seed-*.log` | Seed runs |
| `logs/C*.out` / `C*.log` | Phase C probe logs |
| `seed/B-SUMMARY.md` | Seed summary |
| `seed/seed-results-*.json` | Per-scenario seed results |
| `seed/probe-*.json` | Probe JSON artifacts |
| `probes/SUMMARY.json` | C1–C9 rollup |
| `probes/C-phase.md` | Phase C narrative |
| `probes/A-trend-presets.json` | A2 matrix |
| `screenshots/def-r2-02/` | DEF-R2-02 desktop |
| `screenshots/desktop-d/` | D1–D8 desktop |
| `screenshots/incomplete/` | PENDING |
| `data/nestworth-*.db` | Four scenario DBs |
| `binaries/nestworth` | Production binary |

Docs mirror: `docs/qa/analytics-linux-r3-2026-09-09/` (REPORT, STATUS, PLAN, DEF-R2-02, LAYER notes, probes SUMMARY, seed B-SUMMARY, screenshot trees).

---

## 14. Recommendations / 建议（下一轮）

1. Execute **incomplete-matrix desktop** (missing-price / missing-fx / missing-both banners, gap day Aug 17, zh-CN, Trend All per scenario) → fill `screenshots/incomplete/RESULTS.json`.
2. Add probe modes for **C5 scope**, **C6 native/base**, **C7 sources sum**, **C8 transfer neutrality**; optional **corrupt residual DB** for C9.
3. Triage **R3-D5** waterfall reconcile warning (product vs fixture expectation).
4. Triage **R3-D6** History deep-link account/instrument filter population.
5. Keep **DEF-R2-01** Linux cold-3y as known non-blocking; do not conflate with closed DEF-R2-02.
6. Open docs PR on `qa/analytics-linux-r3-2026-09-09` when ready (not done this pass).

---

## 15. Chinese executive bullets / 中文要点

- **结论：** 自动化 remediation **9/9 PASS**；Round-2 阻断项 **DEF-R2-02 已关闭**；D5/D6 代码和测试已修复，剩余是原生桌面 replay、缺价/缺 FX/中文矩阵及 Linux cold-3y 已知观察项。
- **环境：** 分支 `qa/analytics-linux-r3-2026-09-09`，提交 **`91d10c0`**（seed v3），修复血统含 **`ea94f48`** soft-FX；二进制 ~19.8 MB。
- **种子：** 四场景 **4/4 PASS**（各 35 PASS）；家庭 = AUD Cash + US/SG Brokerage；标的 AAPL/QQQ/ES3；缺口日 08-17；真零日 08-31。
- **探针：** C1–C9 **PASS**；Scope/Native-Base/Sources/Transfer 及 clean/corrupt residual 均有 DB-backed 结果。
- **桌面 complete：** D1–D4/D7/D8 为历史 PASS；D5/D6 已有自动化修复，但原生截图尚未重录。
- **DEF-R2-02：** missing-both Trend All → Partial 21/45 出图，无 “Insights could not be loaded”。
- **未做：** `screenshots/incomplete/` 矩阵桌面；本轮不 git commit/push。

---

*End of Round-3 REPORT. Generated 2026-09-09 SGT from artifact root `/workspace/nestworth-analytics-qa-r3/`.*
