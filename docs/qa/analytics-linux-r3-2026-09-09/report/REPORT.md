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

---

## 1. Verdict / 结论

**Overall: ACCEPTANCE NEARLY MET — DEF-R2-02 CLOSED; residual gaps remain (incomplete desktop matrix PENDING; C5–C9 SKIP; D5/D6 PARTIAL; Linux cold-3y known).**

| Area | Result |
|------|--------|
| Build (`wails3` / native go build) | **PASS** (`BUILD_EXIT:0`) |
| Layer A — Go application suite | **FAIL only** `cold-3y` (~5.08 s want &lt;3 s) — **known non-blocking** (DEF-R2-01) |
| Classifier / MissingFX / Return cases | **PASS** (targeted logs) |
| Phase A — DEF-R2-02 harness | **PASS** — FIXED on probe + desktop |
| Phase B — Seed v3 four scenarios | **PASS** — **4/4 OVERALL**; each **35 PASS / 0 FAIL** |
| Phase C — Probe pack C1–C9 | **4 PASS / 0 FAIL / 5 SKIP** (C5–C9) |
| Phase D — Desktop D1–D8 (complete) | **6 PASS / 2 PARTIAL** (D5 waterfall reconcile warning; D6 History filters All) |
| Desktop incomplete matrix (D7 plan) | **PENDING** — `screenshots/incomplete/` empty (no `RESULTS.json`) |
| DEF-R2-02 (Trend All on missing-both) | **FIXED** — probe partial 21/45 no panic; desktop chart + Partial coverage, no generic load error |

**Green acceptance** (PLAN §3) additionally required DEF-R2-02 fixed → **met**. Full acceptance-grade claim is tempered by: incomplete-matrix desktop still PENDING; quantitative probes C5–C8 not implemented as DB modes; D5/D6 PARTIAL UX notes.

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
- Product code changes in this QA pass (fix already in lineage)
- Git commit / push of docs (explicitly not done this pass)

### Methods / 方法
1. **Seed + probe first**, desktop second — assert coverage/amounts/status in JSON; screenshots for UX / deep-link / defect confirm.
2. Build: `wails3` Linux native → `binaries/nestworth` (~19.8 MB). Log: `logs/02-build.log`.
3. Go targeted: cold-3y / Classification / MissingFX / Return cases — `logs/01-go-analysis.log`, `logs/01b-*.log`, `logs/01c-*.log`, `logs/01d-*.log`.
4. Seed: `cmd/analytics-qa-seed` with `NESTWORTH_QA_SCENARIO=…` → `seed/seed-results-*.json`, `seed/B-SUMMARY.md`.
5. Probes: `cmd/analytics-qa-probe` modes `fixtures` / `cash-include` / legacy / `residual` / `co03` → `probes/SUMMARY.json`, `probes/C-phase.md`, `probes/A-trend-presets.json`.
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
**Counts:** **4 PASS / 0 FAIL / 5 SKIP**  
**Artifacts:** `probes/SUMMARY.json`, `probes/C-phase.md`

| ID | Name | Status | Detail / artifact |
|----|------|--------|-------------------|
| C1 | Fixtures A–D | **PASS** | 4 PASS / 0 FAIL — `seed/probe-fixtures.json` |
| C2 | IncludeCash FL-18/19 | **PASS** | salaryDay=2026-08-05; investedCapital include=`56825.49` vs exclude=`2770.821`; cashDietzFlows include=1(3000) exclude=0 — `seed/probe-cash-include.json` |
| C3 | True-zero Aug 31 | **PASS** | sample_zero includes `2026-08-31 rate=0 status=ok`; All-range complete still Analyze `status=partial` (origin UNAVAIL) — `logs/C3-true-zero-complete.out` |
| C4 | missing-both All | **PASS** | partial **21/45**, exit 0, no panic (DEF-R2-02 regression) — `logs/C4-missing-both.out` |
| C5 | Scope triangulation | **SKIP** | No DB-backed scope probe mode (fixture D in-memory only via C1) |
| C6 | Native vs Base | **SKIP** | No probe mode / env flag |
| C7 | Return sources sum | **SKIP** | No sources-summation mode; **bonus** co03 `PASS_BOTH` on complete — `logs/C-co03.out` |
| C8 | Transfer neutrality FC-01 | **SKIP** | No transfer-day probe mode; seed includes transfers; fixture D asserts transfer price=0 in-memory (C1) |
| C9 | Residual corrupt day | **SKIP** | residual mode needs deliberate corrupt DB; ran on complete → `residualIssueCount=0`, `pass=false` (clean) — `seed/probe-residual.json` |

**Existing modes:** `fixtures`, `cash-include`, `co03`, `residual`, legacy/default.  
**No modes yet for:** scope triangulation, native/base forced flag, return-sources reconciliation, transfer-day ≈0.

**Exit C:** No unexpected FAIL. SKIPs are tooling gaps (documented), not product regressions. Failures would block acceptance claim — none present.

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
| D5 | Drivers waterfall | **PARTIAL** | Aug 1–31 Change Drivers rendered: External flows **+A$1,506.00**; Income +A$3,000.00, Spending −A$200.00, Div&Int +A$518.83, Price +A$534.27, FX +A$169.65, Fees −A$15.00; **UI warned waterfall drivers do not reconcile to ending value** | `d5-drivers.png` |
| D6 | History deep-link | **PARTIAL** | Contribution → View in History: dates From **2026-08-01** to **2026-09-08**; **account/instrument filters remained All**; Apple sale detail correctly showed Account US Brokerage, Instrument Apple Inc, Sell, Aug 27 2026 8:00 AM Asia/Singapore | `d6-history.png` |
| D7 | True-zero day (calendar) | **PASS** | Return Calendar August 2026 day **31** showed exactly **0%** and **A$0.00** | `d7-zero.png` |
| D8 | Soft invalidation / back | **PASS** | History opened, then Return Analysis reloaded successfully with August 2026 summary | `d8-back.png` |

**Desktop complete summary:** **6 PASS / 2 PARTIAL** (D5, D6). No unexplained crash.

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
| Folder | Present but **empty** (no PNGs) |
| Verdict | **PENDING / in progress** — not executed this pass for missing-price / missing-fx / missing-both UX banners / zh-CN / Trend All per incomplete scenario beyond DEF-R2-02 dedicated folder |

---

## 10. Defects / 缺陷登记

| ID | Severity | Title | Status | Notes / evidence |
|----|----------|-------|--------|------------------|
| **DEF-R2-01** | Known / non-blocking | Linux `cold-3y` perf budget | **Open (carry)** | ~5.08 s want &lt;3 s on Linux; M3 Pro policy non-blocking. `logs/01-go-analysis.log` |
| **DEF-R2-02** | Was P1 release hold | missing-both + Return Trend **All** → generic Insights load error / probe panic | **FIXED** | Probe: partial 21/45, no panic. Desktop: chart + Partial 21/45, no generic error. Fix: `convertValuationAmount` soft-missing-FX. Card: `report/DEF-R2-02.md`. Evidence: `logs/03-probe-*`, `probes/A-trend-presets.json`, `screenshots/def-r2-02/*` |
| **R3-D5** | UX / attribution note | Change Drivers waterfall reconcile warning | **Open (PARTIAL)** | Drivers look sane; UI still warns waterfall does not reconcile to ending value on Aug 1–31. `screenshots/desktop-d/d5-drivers.png` |
| **R3-D6** | UX / deep-link | History deep-link leaves account/instrument filters as **All** | **Open (PARTIAL)** | Dates filled; sale detail correct; scope filters not pre-selected. `screenshots/desktop-d/d6-history.png` |

No new P0/P1 crash-class defects in Round-3 desktop/probe runs.

---

## 11. Exit criteria / 退出标准

From PLAN §3 (Round-3 acceptance-grade):

| # | Criterion | Met? | Comment |
|---|-----------|------|---------|
| 1 | Four scenario DBs build cleanly from one seed story | **YES** | 4/4 OVERALL PASS; 35/35 each |
| 2 | Probe pack C1–C9 no unexpected FAIL | **YES** | 0 FAIL; C5–C9 **SKIP** (tooling); DEF-R2-02 no longer known FAIL |
| 3 | Desktop D1–D8 executed with RESULTS | **PARTIAL** | D1–D8 complete RESULTS present; **incomplete-matrix folder PENDING** |
| 4 | Scope / transfer / multi-instrument ≥1 quantitative probe each | **PARTIAL** | Desktop D2 scope UX PASS; C5/C8 SKIP; multi-instrument via seed + D4 amounts |
| 5 | Report published on branch/PR | **DOCS WRITTEN** | This REPORT + STATUS under artifact root + `docs/qa/analytics-linux-r3-2026-09-09/`; **no git commit/push this pass** |

**Green acceptance** additional requirement: DEF-R2-02 fixed or waived → **YES (FIXED)**.

### Exit summary / 退出摘要

- **DEF-R2-02 closed** (probe + desktop) — Round-2 release hold lifted for that defect.
- Seed v3 mini-family + four completeness variants are **acceptance-ready**.
- Probe core path (C1–C4) green; extend modes for C5–C8 and residual corrupt DB for C9 next.
- Desktop complete smoke strong; treat D5 reconcile warning and D6 History filter All as follow-ups.
- Finish **incomplete-matrix** desktop (`screenshots/incomplete/`) before claiming full D7.
- Linux **cold-3y** remains known non-blocking.

**Verdict label:** **ACCEPTANCE NEARLY MET** (blocker DEF-R2-02 fixed; residual PENDING/PARTIAL/SKIP items documented).

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
| `d5-drivers.png` | D5 Change Drivers waterfall (PARTIAL) |
| `d6-history.png` | D6 History deep-link (PARTIAL) |
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

- **结论：** Round-3 **接近验收**；Round-2 阻断项 **DEF-R2-02 已关闭**（探针 + 桌面）；尚有 incomplete 桌面矩阵 **PENDING**、C5–C9 **SKIP**、D5/D6 **PARTIAL**、Linux cold-3y 已知。
- **环境：** 分支 `qa/analytics-linux-r3-2026-09-09`，提交 **`91d10c0`**（seed v3），修复血统含 **`ea94f48`** soft-FX；二进制 ~19.8 MB。
- **种子：** 四场景 **4/4 PASS**（各 35 PASS）；家庭 = AUD Cash + US/SG Brokerage；标的 AAPL/QQQ/ES3；缺口日 08-17；真零日 08-31。
- **探针：** C1–C4 **PASS**；C5–C9 **SKIP**（缺模式/缺残差库）；无意外 FAIL。
- **桌面 complete：** D1–D4/D7/D8 **PASS**；D5 瀑布对账警告 **PARTIAL**；D6 History 筛选仍为 All **PARTIAL**。
- **DEF-R2-02：** missing-both Trend All → Partial 21/45 出图，无 “Insights could not be loaded”。
- **未做：** `screenshots/incomplete/` 矩阵桌面；本轮不 git commit/push。

---

*End of Round-3 REPORT. Generated 2026-09-09 SGT from artifact root `/workspace/nestworth-analytics-qa-r3/`.*
