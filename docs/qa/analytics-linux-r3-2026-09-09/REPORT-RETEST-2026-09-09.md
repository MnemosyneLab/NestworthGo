# Nestworth Insights QA Report — Round-3 Remediation Retest (Acceptance-grade)

| Field | Value |
|-------|-------|
| Product | NestworthGo (Wails v3 Insights / Analytics) |
| Repo | `/workspace/NestworthGo-fe-be-audit` |
| Branch | **`qa/analytics-linux-r3-2026-09-09`** |
| Commit under test | **`fa46d30`** — `fix(analytics): preserve exact projection precision` |
| Full SHA | `fa46d30e5210450f168755694f7c12162fdc0f18` |
| Remediation parent | **`d678cf3`** — `fix(analytics): remediate round-three QA findings` (2026-09-09 16:53 +0800) |
| Prior Round-3 report head | **`91d10c0`** (original REPORT.md evidence); this retest supersedes desktop-pending gates |
| QA date | **2026-09-09** (Asia/Singapore, UTC+8) |
| Platform | Linux **amd64** agent box |
| Display (desktop) | Xvfb / WebKitGTK; settings viewport **1280×719**; AUD; English + **zh-CN** (incomplete matrix) |
| Binary | Wails production ELF `binaries/nestworth` (**19 855 904** bytes ≈ **18.9 MiB**, stripped) |
| Artifact root (run) | `/workspace/nestworth-analytics-qa-r3-retest/` |
| Docs copy | `docs/qa/analytics-linux-r3-2026-09-09/` (this file + `screenshots/retest-2026-09-09/`, `probes/retest-2026-09-09/`) |
| Fixture version | **`analytics-qa-seed-v3`** (existing r3 DBs; not re-seeded this retest) |
| Scenario DBs | `/workspace/nestworth-analytics-qa-r3/data/nestworth-{complete,missing-price,missing-fx,missing-both}.db` |
| Settings | currency=AUD; window 1280×719 |
| Remediation plan | `REMEDIATION-PLAN.md` |
| Prior report (do not edit) | `REPORT.md` / `STATUS.md` — historical Round-3 @ `91d10c0` |

> **Scope of this document:** Full remediation retest after `d678cf3` + precision fix `fa46d30`. Closes the native desktop evidence gates that `REPORT.md` left as PENDING (R3-D5, R3-D6, incomplete matrix, D8 invalidation). Original Round-3 tables remain in `REPORT.md` as historical baseline; **this file is the authoritative post-remediation acceptance record**.

---

## 1. Verdict / 结论

**Overall: REMEDIATION RETEST PASS — remediation targets CLOSED with native desktop evidence.**

| Area | Result |
|------|--------|
| Build (`wails3` native) | **PASS** — binary 19 855 904 B |
| Unit gates (Review / HistoryHint / Waterfall / Attribution / MissingFX / ReturnMissing) | **PASS** |
| Frontend vitest | **PASS** — **344** passed / 44 files / ~16.5 s |
| Frontend `bun test` (Bun runner) | **SKIP** — no jsdom; use vitest |
| Probe D5 reconcile (Aug exact) | **PASS** — `delta = 0 AUD`, residual issues = 0 |
| Probe C5–C8 quantitative | **PASS** — all deltas 0 |
| Probe C9 clean / corrupt residual | **PASS** — clean issues=0; corrupt issues=2 (±100 AUD) |
| Probe C1 / C2 / C4 / DEF-R2-02 | **PASS** |
| Desktop D5 Change Drivers Aug | **PASS** — no false waterfall mismatch; begin **A$54,258.02** → end **A$59,784.37** |
| Desktop D6 Realized → History | **PASS** — Instrument = **Apple Inc** (not All) |
| Incomplete matrix desktop | **PASS** — **15/15** (3 scenarios × 5 checks; en + zh-CN) |
| Desktop D8 soft invalidation | **PASS** — +A$100 income → Net Worth Change **+A$5,561.81 → +A$5,661.81** |
| Automation rollup | **ALL PASS** — `probes/retest-2026-09-09/RETEST-SUMMARY.json` (`allPass: true`, `failIds: []`) |

**Green acceptance:** Remediation-plan targets R3-D5, R3-D6, C5–C9, incomplete matrix (R3-Q2), and D8 invalidation (R3-Q3) are **met** with both automated probes and native Wails screenshots. Linux `cold-3y` (DEF-R2-01) remains a known non-blocking observation (not re-gated this retest beyond unit Review suite green).

---

## 2. Scope and methodology / 范围与方法

### In scope (this retest)
- Product head **`fa46d30`** on branch `qa/analytics-linux-r3-2026-09-09`
- Rebuild Linux production binary; targeted Go unit gates; frontend vitest (344)
- Probe pack: D5 reconcile, C5–C8 quantitative, C9 residual clean/corrupt, C1 fixtures, C2 cash-include, C4 / DEF-R2-02 missing-both All
- Native desktop: D5 Change Drivers Aug (complete), D6 Realized HistoryHint (Apple Inc), incomplete matrix ×3 scenarios ×5 checks (en + zh-CN), D8 GUI mutation soft-invalidation
- New report + evidence tree only (no edits to existing `REPORT.md` / `STATUS.md`)

### Out of scope / not claimed
- Re-seed of v3 mini-family (reuse existing r3 scenario DBs)
- macOS / Apple Silicon matrix
- Full timezone / DST matrix; 1440 / responsive deep visual
- Loan (FC-07) stretch
- Performance optimization of Linux `cold-3y` (DEF-R2-01 carry)
- Committing unrelated untracked files (`build/linux/nestworth.desktop`, `frontend/bun.lock`)

### Methods / 方法
1. Build → unit → vitest → probe JSON → desktop screenshots (seed/probe first, GUI confirm second).
2. Automation root: `/workspace/nestworth-analytics-qa-r3-retest/` with rollup `probes/RETEST-SUMMARY.json` + `probes/FULL-RETEST.json`.
3. Desktop: launch production ELF with `NESTWORTH_DATABASE_PATH` / `NESTWORTH_SETTINGS_PATH`; capture PNG + `RESULTS.json`.
4. User rule: missing price / FX = **no invent/fill**; mark incomplete / partial only.
5. Docs archive under `docs/qa/analytics-linux-r3-2026-09-09/{screenshots,probes,report}/retest-2026-09-09/`.

Default complete-DB desktop filters unless noted: **Portfolio · Base (AUD) · Include cash · Aug 2026 window**.

---

## 3. Environment and toolchain / 环境

| Item | Result |
|------|--------|
| Host | Linux amd64 agent box |
| Repo / branch | `NestworthGo-fe-be-audit` @ `qa/analytics-linux-r3-2026-09-09` |
| Under test | **`fa46d30`** (after `d678cf3` remediation + precision fix) |
| Go / Wails | Wails v3 production tags |
| Frontend | bun + Vite build inside `wails3` task; vitest via `bun run test` |
| Linux UI | GTK4 / WebKitGTK; Xvfb (display `:11` for incomplete matrix) |
| Viewport | `window_width=1280`, `window_height=719` |
| Currency / locale | AUD; English + Simplified Chinese (incomplete matrix) |
| Anchor / window | History origin **2026-07-26**; snapshots through **2026-09-08** (~45 days) |
| Artifact root | `/workspace/nestworth-analytics-qa-r3-retest/` |
| Source DBs | `/workspace/nestworth-analytics-qa-r3/data/` (complete / missing-*) |

---

## 4. Build / 构建

| Item | Result | Evidence |
|------|--------|----------|
| Linux native build | **PASS** | `FULL-RETEST.json` item BUILD; run log `logs/01-build.log` (artifact root) |
| Binary size | **19 855 904** bytes (~18.9 MiB) | `binaries/nestworth` under artifact root |
| Form | ELF 64-bit LSB executable, x86-64, stripped | file(1) |

**Build: PASS**

---

## 5. Unit / frontend gates / 单元与前端

| Check | Result | Evidence |
|-------|--------|----------|
| Go targeted: Review / HistoryHint / Waterfall / Attribution / MissingFX / ReturnMissing | **PASS** — `ok …/internal/application 0.080s` | `FULL-RETEST.json` UNIT; `logs/02-unit-gates.log` |
| Frontend vitest | **PASS** — **344** passed / 44 files / 16.50s | `FULL-RETEST.json` FE_vitest; `logs/03c-vitest-full.log` |
| Frontend `bun test` (Bun runner) | **SKIP** — no jsdom; timed out / incomplete | `FULL-RETEST.json` FE_bun |

**Policy:** Prefer `bun run test` (vitest) as the frontend gate for this retest. Bun-native runner SKIP is expected tooling limitation, not a product failure.

---

## 6. Automation probe retest / 探针复测

**When:** 2026-09-09 ~17:29–17:33 SGT  
**Head:** `fa46d30`  
**Rollup:** `probes/retest-2026-09-09/RETEST-SUMMARY.json` — **`allPass: true`**, `failIds: []`  
**Detail:** `probes/retest-2026-09-09/FULL-RETEST.json`

### 6.1 Gate table

| ID | Name | Status | Key numbers / notes |
|----|------|--------|---------------------|
| BUILD | wails3 task build | **PASS** | size=19855904 |
| UNIT | go test Review\|HistoryHint\|Waterfall\|… | **PASS** | application 0.080s |
| FE_vitest | bun run test | **PASS** | 344 / 44 files |
| FE_bun | bun test | **SKIP** | use vitest |
| D5 | reconcile Aug 2026-08-01..31 | **PASS** | beginning `54258.02`, ending `59784.3685`, waterfallSum `5526.3485`, **delta `0` AUD**, residualIssueCount **0** |
| C5 | Scope triangulation | **PASS** | household/account/instrument deltas **0**; householdChange `5526.3485` |
| C6 | Native versus Base | **PASS** | mixed currencies → forced Base AUD |
| C7 | Return sources | **PASS** | periodReturn = sourceSum = `1235.3483`; delta **0**; coverage 31/31 |
| C8 | Transfer neutrality | **PASS** | day 2026-08-16 household/price/fx/fee/return deltas **0**; endpoint legs ±200 AUD |
| C9_clean | residual inject=false | **PASS** | issues=**0** |
| C9_corrupt | residual inject=true +100 AUD | **PASS** | issues=**2**; day residual +100 AUD; dates 2026-08-12 / 2026-08-13; native unchanged 1885 |
| C1 | fixtures A–D | **PASS** | in-memory fixtures |
| C2 | cash-include FL-18/19 | **PASS** | salaryDay 2026-08-05; investedCapital include `56825.49` vs exclude `2770.821`; cashDietzFlows include=1(3000) exclude=0 |
| C4 | missing-both All (legacy) | **PASS** | coverage rated=**21**/total=**45** status=**partial**; panic=false |
| DEF-R2-02 | alias of C4 | **PASS** | same 21/45 partial |

### 6.2 D5 reconcile detail (exact)

Source: `probes/retest-2026-09-09/D5-reconcile.json` + RETEST-SUMMARY `d5`.

| Field | Value |
|-------|-------|
| Query | complete DB; Base AUD; Include cash; **2026-08-01..2026-08-31** |
| Beginning | **54258.02 AUD** |
| Ending | **59784.3685 AUD** |
| Change | **5526.3485 AUD** |
| Waterfall sum | **5526.3485 AUD** |
| **Delta** `ending − beginning − sum(waterfall)` | **0 AUD** |
| Residual issues | **0** |
| Status | **ok** |

Waterfall buckets (exact): income `3000`, external_flow `1506`, dividend_interest `518.8325`, price_change `546.8668`, fx_impact `169.6492`, fee `-15`, spending `-200`.

### 6.3 C5–C8 quantitative highlights

Source: `probes/retest-2026-09-09/C5-C8-quantitative.json` (`allPass: true`).

| Probe | Expected vs actual |
|-------|--------------------|
| C5 Scope | householdBeginning `54258.02`, ending `59784.3685`, change `5526.3485`; accountPartitionChange / instrument+cash match; all deltas **0** |
| C6 Native/Base | mixedCurrencies true → `nativeValuationForced=base`, currency AUD |
| C7 Sources | periodReturn `1235.3483` = sourceSum; coverage 31/31; keys dividend_interest / fx_impact / price_change |
| C8 Transfer | 2026-08-16 with/without transfer: householdChange both `17.7696`; price/fx/fee unchanged; return `1235.3483` both; account endpoint changeDelta **0** vs legs ±200 |

### 6.4 C9 residual controls

| Mode | Pass | Issues | Notes |
|------|------|-------:|-------|
| Clean (`inject=false`) | **true** | 0 | copied fixture; day 2026-08-12 baseBefore=baseAfter `2833.909` |
| Corrupt (`inject=true` +100 AUD) | **true** | 2 | baseAfter `2933.909`; details +100 @ 2026-08-12 and −100 @ 2026-08-13; nativeUnchanged `1885` |

**Exit probes:** Automation pack green; no unexpected FAIL.

---

## 7. Desktop remediation replay / 桌面复测

### 7.1 D5 — Change Drivers waterfall (complete, Aug)

Source: `screenshots/retest-2026-09-09/d5-d6/RESULTS.json`.

| Field | Value |
|-------|-------|
| Status | **PASS** |
| Screen | Asset Changes → Change Drivers |
| Filters | From **2026-08-01**, To **2026-08-31**, Portfolio, Base (AUD), Include cash |
| Beginning (UI) | **+A$54,258.02** |
| Ending (UI) | **+A$59,784.37** |
| Change (UI) | **+A$5,526.35** |
| Warning | **None** — no waterfall reconciliation / ending-value mismatch warning |
| Screenshot | `screenshots/retest-2026-09-09/d5-d6/d5-drivers-aug.png` |

**Mapping to probe:** UI two-decimal display matches rounded exact amounts (begin `54258.02`, end `59784.3685` → `59,784.37`, change `5526.3485` → `5,526.35`). Probe delta remains exact **0**.

**Closes R3-D5** (false waterfall warning).

### 7.2 D6 — Realized HistoryHint deep-link

| Field | Value |
|-------|-------|
| Status | **PASS** |
| Screen | Return Analysis → Contribution → Apple Inc detail → View in History |
| Before click | returnType=**Realized Gain**, groupBy=**Instrument**, groupKey=**Apple Inc** |
| After click | Instrument filter=**Apple Inc**, Account=**All**, From **2026-08-01**, To **2026-08-31** |
| Notes | History opened with Instrument set to Apple Inc, **not All** |
| Screenshots | `d6-realized-apple.png`, `d6-history-filters.png` |

**Closes R3-D6** (HistoryHint instrument dimension).

### 7.3 Incomplete matrix — missing-both / missing-price / missing-fx × 5 checks

Source: `screenshots/retest-2026-09-09/incomplete-matrix/RESULTS.json`.  
**Overall: PASS — 3 scenarios × 5 checks = 15/15.** Language order: English → 简体中文 → English (restored).

User rule confirmed: missing price/FX surfaces as **Partial / incomplete** banners and markers; **no invent/fill** of unknown amounts.

#### Per-scenario checks

| Scenario | Banner | Trend All | Cal gap+zero | Change Drivers | zh-CN copy | Overall |
|----------|--------|-----------|--------------|----------------|------------|---------|
| missing-both | **PASS** — Partial 21/45; Issues (24) | **PASS** — chart; Partial 21/45; no generic load error; From 2026-07-26 To 2026-09-08 | **PASS** — Aug 17 ◇ incomplete; Aug 31 **0%** / **A$0.00** | **PASS** — Change Drivers; Partial; incomplete-day warning | **PASS** — Chinese partial banner + incomplete-day warning | **PASS** |
| missing-price | **PASS** — Partial 21/45; Issues (24) | **PASS** — same shape | **PASS** — Aug 17 ◇; Aug 31 true zero | **PASS** — Drivers incomplete warning | **PASS** — zh-CN | **PASS** |
| missing-fx | **PASS** — Partial 21/45; Issues (24) | **PASS** — same shape | **PASS** — Aug 17 ◇; Aug 31 true zero | **PASS** — Drivers incomplete warning | **PASS** — zh-CN | **PASS** |

#### Screenshot map (incomplete-matrix)

| File | Case |
|------|------|
| `missing-both-banner.png` | Partial coverage banner (en) |
| `missing-both-trend-all.png` | Trend All |
| `missing-both-cal-gap.png` | Calendar Aug gap + true zero |
| `missing-both-drivers.png` | Asset Changes → Change Drivers |
| `missing-both-zh-banner.png` / `missing-both-zh-drivers.png` | zh-CN |
| `missing-price-banner.png` | Partial banner |
| `missing-price-trend-all.png` | Trend All |
| `missing-price-cal-gap.png` | Calendar gap/zero |
| `missing-price-drivers.png` | Change Drivers |
| `missing-price-zh-banner.png` / `missing-price-zh-drivers.png` | zh-CN |
| `missing-fx-banner.png` | Partial banner |
| `missing-fx-trend-all.png` / `missing-fx-trend-chart.png` | Trend All (+ chart close-up) |
| `missing-fx-cal-gap.png` | Calendar gap/zero |
| `missing-fx-drivers.png` | Change Drivers |
| `missing-fx-zh-banner.png` / `missing-fx-zh-drivers.png` | zh-CN |
| `RESULTS.json` | Machine-readable 15/15 rollup |

**Closes R3-Q2** incomplete-matrix desktop gap (including correct Change Drivers page and zh-CN).

### 7.4 D8 — Soft invalidation via real data mutation

Source: `screenshots/retest-2026-09-09/d8/RESULTS.json`.

| Field | Value |
|-------|-------|
| Status | **PASS** |
| View | Asset Changes → Change Drivers |
| Filters | Portfolio, Base (AUD), Include cash, From **2026-08-01** (open end) |
| Headline | **Net Worth Change** |
| Baseline | **+A$5,561.81** |
| Mutation | Added **A$100.00** cash income to AUD Cash on **2026-08-20** via History UI |
| After | **+A$5,661.81** |
| Expected direction | increase by A$100.00 |
| Screenshots | `01-baseline.png`, `01b-mutation.png`, `02-after-mutation.png` |

**Closes R3-Q3** — proves Insights soft-invalidation after real activity write (not navigation-only smoke).

---

## 8. Defects / 缺陷登记（retest）

| ID | Severity | Title | Status after retest | Notes / evidence |
|----|----------|-------|---------------------|------------------|
| **DEF-R2-01** | Known / non-blocking | Linux `cold-3y` perf budget | **Open (carry)** | Not a remediation target; M3 Pro policy non-blocking. Not re-failed in Review unit gate. |
| **DEF-R2-02** | Was P1 | missing-both Trend All generic load / panic | **FIXED (reconfirmed)** | Probe C4: partial 21/45, no panic; incomplete matrix Trend All PASS all three scenarios |
| **R3-D5** | P1 triage → fix | False waterfall mismatch warning / precision | **CLOSED** | Probe delta=0; desktop no warning; UI A$54,258.02 → A$59,784.37 |
| **R3-D6** | P2 | Realized HistoryHint leaves filters All | **CLOSED** | Desktop Instrument=Apple Inc |
| **R3-Q1** | P1 acceptance gap | C5–C8 quantitative + C9 residual | **CLOSED** | All PASS in RETEST-SUMMARY |
| **R3-Q2** | P2 acceptance gap | Incomplete matrix desktop | **CLOSED** | 15/15 PASS with correct Drivers + zh-CN |
| **R3-Q3** | P2 acceptance gap | D8 soft invalidation | **CLOSED** | +A$100 → +A$5,561.81 → +A$5,661.81 |
| **R3-Q4** | Evidence governance | Docs / archive | **Addressed by this report** | New REPORT-RETEST + archived screenshots/probes; original REPORT.md left intact |

No new P0/P1 crash-class defects in retest desktop/probe runs.

---

## 9. Exit criteria / 退出标准

Mapped to REMEDIATION-PLAN §7 / Round-3 PLAN acceptance, updated for retest:

| # | Criterion | Met? | Comment |
|---|-----------|------|---------|
| 1 | D5 exact reconcile + no false UI warning | **YES** | Probe delta 0; desktop screenshot clean |
| 2 | D6 HistoryHint instrument/account dimensions | **YES** | Apple Inc filter on History |
| 3 | C5–C8 quantitative probes PASS | **YES** | Scope / Native-Base / Sources / Transfer |
| 4 | C9 clean + corrupt residual PASS | **YES** | issues 0 / 2 with ±100 AUD detail |
| 5 | Incomplete matrix missing-price/fx/both + zh-CN + real Drivers | **YES** | 15/15 |
| 6 | D8 real mutation invalidation | **YES** | Net Worth Change +A$100 |
| 7 | Build + unit + FE vitest green | **YES** | BUILD / UNIT / 344 vitest |
| 8 | Report + evidence on branch/PR | **YES** | This file + retest trees; commit on PR **#15** |

### Exit summary / 退出摘要

- Remediation head **`fa46d30`** is acceptance-green for Insights analytics Linux Round-3 remediation targets.
- Native desktop evidence now exists for D5, D6, incomplete matrix, and D8 — closing the PENDING gates from `REPORT.md`.
- DEF-R2-02 remains fixed; DEF-R2-01 cold-3y remains known non-blocking.
- Missing price/FX continues to mark **partial/incomplete** without inventing values.

**Verdict label:** **REMEDIATION RETEST PASS** (automated + native desktop).

---

## 10. Screenshot index / 截图索引（retest）

### D5 / D6 (`screenshots/retest-2026-09-09/d5-d6/`)

| File | Case |
|------|------|
| `d5-drivers-aug.png` | D5 Change Drivers Aug — no false mismatch |
| `d6-realized-apple.png` | D6 Realized Contribution Apple Inc detail |
| `d6-history-filters.png` | D6 History filters Instrument=Apple Inc |
| `RESULTS.json` | D5/D6 machine statuses |

### Incomplete matrix (`screenshots/retest-2026-09-09/incomplete-matrix/`)

| File | Case |
|------|------|
| `missing-{both,price,fx}-banner.png` | Partial coverage banners (en) |
| `missing-{both,price,fx}-trend-all.png` | Trend All |
| `missing-fx-trend-chart.png` | Trend chart close-up (fx) |
| `missing-{both,price,fx}-cal-gap.png` | Aug 17 gap + Aug 31 true zero |
| `missing-{both,price,fx}-drivers.png` | Change Drivers incomplete |
| `missing-{both,price,fx}-zh-banner.png` | zh-CN banner |
| `missing-{both,price,fx}-zh-drivers.png` | zh-CN Drivers |
| `RESULTS.json` | 15/15 rollup |

### D8 (`screenshots/retest-2026-09-09/d8/`)

| File | Case |
|------|------|
| `01-baseline.png` | Net Worth Change +A$5,561.81 |
| `01b-mutation.png` | History +A$100 income 2026-08-20 |
| `02-after-mutation.png` | Net Worth Change +A$5,661.81 |
| `RESULTS.json` | D8 PASS |

---

## 11. Evidence path index / 证据路径

| Path (docs-relative under `docs/qa/analytics-linux-r3-2026-09-09/`) | Contents |
|------|----------|
| `REPORT-RETEST-2026-09-09.md` | This report |
| `STATUS-RETEST-2026-09-09.md` | One-pager companion |
| `report/retest-2026-09-09/*-RESULTS.json` | Mirrored desktop RESULTS |
| `screenshots/retest-2026-09-09/d5-d6/` | D5/D6 PNG + RESULTS |
| `screenshots/retest-2026-09-09/incomplete-matrix/` | 15-check matrix PNG + RESULTS |
| `screenshots/retest-2026-09-09/d8/` | D8 mutation PNG + RESULTS |
| `probes/retest-2026-09-09/RETEST-SUMMARY.json` | Automation rollup |
| `probes/retest-2026-09-09/FULL-RETEST.json` | Per-gate detail |
| `probes/retest-2026-09-09/D5-reconcile.json` | Exact waterfall reconcile |
| `probes/retest-2026-09-09/C5-C8-quantitative.json` | Scope/Native/Sources/Transfer |
| `probes/retest-2026-09-09/C9-residual-*.json` | Clean/corrupt residual |
| `probes/retest-2026-09-09/C1-fixtures.json` | Fixtures A–D |
| `probes/retest-2026-09-09/C2-cash-include.json` | IncludeCash FL-18/19 |

Run-machine only (not git-committed): `/workspace/nestworth-analytics-qa-r3-retest/{binaries,logs,data}/`, scenario DBs under `/workspace/nestworth-analytics-qa-r3/data/`.

Historical (unchanged): `REPORT.md`, `STATUS.md`, `screenshots/{def-r2-02,desktop-d,incomplete}/`, `probes/REMEDIATION-2026-09-09.json`.

---

## 12. Remediation target crosswalk / 修复目标对照

| REMEDIATION-PLAN ID | Retest outcome | Primary evidence |
|---------------------|----------------|------------------|
| R3-D5 | **PASS / CLOSED** | D5-reconcile.json delta=0; `d5-drivers-aug.png` |
| R3-D6 | **PASS / CLOSED** | `d6-history-filters.png` Instrument=Apple Inc |
| R3-Q1 (C5–C8) | **PASS / CLOSED** | C5-C8-quantitative.json |
| R3-Q1/C9 | **PASS / CLOSED** | C9-residual-clean/corrupt.json |
| R3-Q2 incomplete matrix | **PASS / CLOSED** | incomplete-matrix/RESULTS.json 15/15 |
| R3-Q3 D8 invalidation | **PASS / CLOSED** | d8/RESULTS.json +A$100 |
| R3-Q4 evidence | **Addressed** | This report + archived trees |

---

## 13. Recommendations / 建议

1. Keep **DEF-R2-01** Linux cold-3y as known non-blocking; do not conflate with closed remediation items.
2. Prefer vitest (`bun run test`) over Bun-native `bun test` in CI gates until jsdom runner is available.
3. Optional follow-up: PR title/body update on **#15** to note remediation head `fa46d30` + retest PASS (docs already on same branch).
4. Do not treat historical `screenshots/desktop-d/d5-drivers.png` / `d6-history.png` as current state — use `screenshots/retest-2026-09-09/` instead.

---

## 14. Chinese executive bullets / 中文要点

- **结论：** 修复后复测 **PASS**。自动化 `RETEST-SUMMARY` 全绿；桌面 D5/D6、缺数据矩阵 **15/15**、D8 软失效均有原生证据。
- **版本：** 分支 `qa/analytics-linux-r3-2026-09-09`，提交 **`fa46d30`**（继 `d678cf3`）；二进制 ~19.86 MB。
- **D5：** 精确对账 delta=**0 AUD**；UI 期初 **A$54,258.02** → 期末 **A$59,784.37**；无虚假瀑布警告。
- **D6：** Realized → History，Instrument=**Apple Inc**（非 All）。
- **缺价/缺 FX：** Partial 21/45，Issues(24)；缺口日 08-17 标记；真零 08-31；中英文提示齐全；**不填造**未知金额。
- **D8：** History 新增 +A$100（2026-08-20）后 Net Worth Change **+A$5,561.81 → +A$5,661.81**。
- **探针：** C5–C8 定量 delta=0；C9 clean=0 / corrupt=2（±100 AUD）；C1/C2/C4/DEF-R2-02 PASS；FE vitest **344**。
- **文档：** 本报告为新建文件；未改写旧 `REPORT.md`/`STATUS.md`。

---

*End of Round-3 REMEDIATION RETEST REPORT. Generated 2026-09-09 SGT from artifact root `/workspace/nestworth-analytics-qa-r3-retest/` against head `fa46d30`.*
