# Nestworth Insights QA Report — Round-4 Loan + Timezone (Acceptance-grade)

| Field | Value |
|-------|-------|
| Product | NestworthGo (Wails v3 Insights / Analytics) |
| Repo | `/workspace/NestworthGo-fe-be-audit` |
| Branch | **`qa/analytics-linux-r3-2026-09-09`** |
| PR | **[#15](https://github.com/MnemosyneLab/NestworthGo/pull/15)** |
| Tooling / harness head under test | **`dd6cf35`** — `docs(qa): record complete-usd/cny onboarded ENV-BASE harness` |
| Full SHA (tooling) | `dd6cf3526ddeca94e7813dd4420d8b822a609642` |
| Product binary for desktop | R3 remediation retest build @ **`fa46d30`** lineage (`fa46d30e5210450f168755694f7c12162fdc0f18` — `fix(analytics): preserve exact projection precision`) |
| Loan seed tooling lineage | Loan family seeds recorded @ **`ce8aedb`** (fixture `analytics-linux-qa-loan-fc07`); complete-usd/cny @ **`dd6cf35`** |
| QA date | **2026-09-09** (Asia/Singapore, UTC+8) |
| Platform | Linux **amd64** agent box (Xvfb / WebKitGTK desktop) |
| Display (desktop) | Xvfb; physical width **1280**; settings viewport ~1280×719 |
| Artifact root (run) | `/workspace/nestworth-analytics-qa-r4/` |
| Docs copy | `docs/qa/analytics-linux-r3-2026-09-09/` (this file + `screenshots/r4-loan-2026-09-09/`, `probes/r4-loan-2026-09-09/`, `seed/r4-loan-2026-09-09/`, `report/r4-loan-2026-09-09/`) |
| Plan / status companions | `PLAN-R4-LOAN.md`, `STATUS-R4-LOAN.md` |
| Prior reports (do not edit) | `REPORT.md` / `STATUS.md` (R3) · `REPORT-RETEST-2026-09-09.md` / `STATUS-RETEST-2026-09-09.md` |

> **Scope of this document:** Full Round-4 **Loan FC-07 + timezone/DST + ENV-BASE USD/CNY + Linux desktop visual** acceptance record. Closes the Phase D desktop rows that the earlier tooling stub left BLOCKED. Round-3 and remediation-retest documents remain historical baselines; **this file is the authoritative R4 Loan+TZ acceptance record**.

---

## 1. Verdict / 结论

**Overall: ROUND-4 LOAN+TZ PASS (Linux) — seed/probe green; desktop FC-07 PASS; locale/window PASS; TZ/week PARTIAL (product UI limits); expected host BLOCKEDs remain.**

| Area | Result |
|------|--------|
| Seeds `loan-fc07*` (5 scenarios) | **PASS** — each OVERALL green (`14 PASS / 0 FAIL`) |
| Seeds `complete-usd` / `complete-cny` (+ via-env USD) | **PASS** — `36 PASS / 0 FAIL`; onboarded `base_currency` matches |
| Probe `loan-fc07` | **PASS** — **7/7** |
| Probe `timezone-r4` | **PASS** — **23/23** |
| Probe `r4-matrix` @ `dd6cf35` | **PASS** — **46 PASS / 0 FAIL / 1 SKIP / 10 BLOCKED** (`allPass: true`) |
| Unit baseline (debt/DST/settings/valuation) | **PASS** — `SUMMARY.json` / `BASELINE-UNIT.json` 51/51; FE targeted 19 PASS |
| FE gaps (insights/history/onboarding + Go subset) | **PASS** — `FE-GAP.json` |
| Desktop FC-07 (History / Drivers / Categories) | **PASS** — draw/repay/interest oracles match UI |
| Desktop locale EN / zh-CN / zh-TW | **PASS** |
| Desktop window default + narrow | **PASS** (1440 N/A on 1280 display) |
| Desktop timezone Settings | **PARTIAL** — System→UTC informational only |
| Desktop week-start UI | **PARTIAL** — no week-start control; calendar Monday-first |
| macOS Apple Silicon | **BLOCKED** — host is Linux amd64 |
| `M-DST-FALL-REBUILD` (Nov closed-day) | **BLOCKED** — wall clock 2026-09-09 |
| `M-WK-ENGINE` Monday hardcode | **BLOCKED** — product engine limit (settings is UI) |
| Linux `cold-3y` (DEF-R2-01) | **Known FAIL** — non-blocking carry from R2/R3 |

**Green acceptance:** Every **runnable** PLAN checklist row is PASS or an intentional SKIP/BLOCKED with reason. Desktop visual rows that this Linux host can exercise are closed with screenshots (FC07-D1, M-LOC-VIS-*, M-WIN-VIS-* with 1440 note, D-TZ PARTIAL). Remaining BLOCKEDs are host/calendar/product limits — not missed tests.

---

## 2. Scope and methodology / 范围与方法

### In scope
- Tooling head **`dd6cf35`** on branch `qa/analytics-linux-r3-2026-09-09` (PR #15)
- Seed family: `loan-fc07`, `loan-fc07-utc`, `loan-fc07-cny`, `loan-fc07-usd`, `loan-fc07-week-sunday`
- ENV-BASE: `complete-usd`, `complete-cny`, optional `complete` + `NESTWORTH_QA_BASE_CURRENCY=USD`
- Probes: `loan-fc07`, `timezone-r4`, `r4-matrix` (authoritative rollup `probe-r4-matrix-dd6cf35.json`)
- Unit + FE gap baselines archived under `report/r4-loan-2026-09-09/`
- Native Linux desktop: FC-07 Insights on `loan-fc07` DB; locale / window / timezone / week env captures
- New report + evidence tree only (no edits to Round-3 `REPORT.md` / `REPORT-RETEST` / `STATUS.md` / `STATUS-RETEST`)

### Out of scope / not claimed
- macOS / Apple Silicon native matrix
- Snapshot rebuild on **2026-11-01** fall-back closed day (`M-DST-FALL-REBUILD`)
- Changing engine week fold from hardcoded Monday (`M-WK-ENGINE`)
- Inventing FX fills or weakening Cases 16/17 oracles
- Performance fix for Linux `cold-3y`
- Committing unrelated untracked files (`build/linux/nestworth.desktop`, `frontend/bun.lock`)

### Methods / 方法
1. Seed → probe JSON → unit/FE baseline → desktop screenshots (automation first, GUI confirm second).
2. Artifact root: `/workspace/nestworth-analytics-qa-r4/` with rollup `HARNESS-LOCAL.json`.
3. Desktop: launch production ELF (`fa46d30` lineage) with `NESTWORTH_DATABASE_PATH` / settings; capture PNG + RESULTS JSON.
4. User rule: missing price / FX = **no invent/fill**; mark incomplete / partial only.
5. Docs archive under `docs/qa/analytics-linux-r3-2026-09-09/{screenshots,probes,seed,report}/r4-loan-2026-09-09/`.

Default FC-07 desktop filters: **Portfolio · Base (AUD) · Include cash · 2026-07-27..2026-07-31**.

---

## 3. Environment and toolchain / 环境

| Item | Result |
|------|--------|
| Host | Linux amd64 agent box |
| Repo / branch | `NestworthGo-fe-be-audit` @ `qa/analytics-linux-r3-2026-09-09` |
| Tooling under test | **`dd6cf35`** |
| Desktop product binary | **`fa46d30`** lineage (R3 retest Wails production ELF) |
| Go | go1.24.4 |
| Linux UI | GTK4 / WebKitGTK; Xvfb (display `:11` for FC-07) |
| Viewport | ~1280 wide (physical display 1280; narrow ~900 exercised) |
| Currency / locale (FC-07 desktop) | AUD; English (locale matrix also zh-CN / zh-TW) |
| History origin (loan fixture) | **Asia/Singapore**; anchor **2026-07-26T00:00:00Z** |
| Artifact root | `/workspace/nestworth-analytics-qa-r4/` |
| Loan primary DB | `/workspace/nestworth-analytics-qa-r4/loan-fc07/data/nestworth.db` |
| Complete AUD (matrix) | `/workspace/nestworth-analytics-qa-r3/data/nestworth-complete.db` |
| Complete USD / CNY | `/workspace/nestworth-analytics-qa-r4/complete-{usd,cny}/data/nestworth.db` |

---

## 4. Seed harness / 种子

**When:** 2026-09-09 ~22:42–22:55 SGT  
**Rollup:** `probes/r4-loan-2026-09-09/HARNESS-LOCAL.json` · seed JSON copies under `seed/r4-loan-2026-09-09/`

| Scenario | Status | Key IDs | Results summary | Evidence |
|----------|--------|---------|-----------------|----------|
| `loan-fc07` | **PASS** | tz=`Asia/Singapore`, base=`AUD`, week=`monday`; draw 2026-07-27 / repay 2026-07-29 / interest 2026-07-31 | 14 PASS / 0 FAIL | `seed-results-loan-fc07.json` |
| `loan-fc07-utc` | **PASS** | tz=`UTC`, AUD, monday | 14 PASS / 0 FAIL | `seed-results-loan-fc07-utc.json` |
| `loan-fc07-cny` | **PASS** | base=`CNY`, SGT, monday | 14 PASS / 0 FAIL | `seed-results-loan-fc07-cny.json` |
| `loan-fc07-usd` | **PASS** | base=`USD`, SGT, monday | 14 PASS / 0 FAIL | `seed-results-loan-fc07-usd.json` |
| `loan-fc07-week-sunday` | **PASS** | week_start=`sunday`, AUD, SGT | 14 PASS / 0 FAIL | `seed-results-loan-fc07-week-sunday.json` |
| `complete-usd` | **PASS** | `base_currency=USD`, 45 snapshots, incomplete_days=0 | 36 PASS / 0 FAIL | `seed-results-complete-usd.json` |
| `complete-cny` | **PASS** | `base_currency=CNY`, 45 snapshots, incomplete_days=0 | 36 PASS / 0 FAIL | `seed-results-complete-cny.json` |
| `complete` + `NESTWORTH_QA_BASE_CURRENCY=USD` | **PASS** | same onboarded USD pattern (`complete-usd-via-env`) | 36 PASS / 0 FAIL | harness note in `HARNESS-LOCAL.json` |

### FC-07 seed oracles (AUD SGT)

Named steps `fc07_draw_nw0` / `fc07_repay_nw0` / `fc07_interest_spending` all **PASS**:

| Step | Ledger | Net worth | Must not be |
|------|--------|-----------|-------------|
| Drawdown 100000 | Cash +100000, Debt +100000 | change = **0** | Investment return |
| Principal repay 10000 | Cash −10000, Debt −10000 | change = **0** | Investment return |
| Cash interest (principal **1000** + fee **500**) | Cash −1500, debt principal −1000 | **−500**; **Spending −500** | Dividend & Interest / investment return |

Interest-day principal is **1000** (Case 17) because `DebtPaymentInput` rejects InterestOrFee-only.

**Exit seeds: PASS**

---

## 5. Probe automation / 探针

### 5.1 `loan-fc07` — 7/7 PASS

Source: `probes/r4-loan-2026-09-09/probe-loan-fc07.json` (`allPass: true`, summary `7 PASS / 0 FAIL / 0 SKIP`)

| ID | Name | Status | Key got |
|----|------|--------|---------|
| `fc07_draw_nw0` | draw net-worth neutrality | **PASS** | begin/end `20000`; nwDelta `0`; spending `0`; return `0`; DI `0` |
| `fc07_repay_nw0` | repay principal NW neutrality | **PASS** | nwDelta `0`; spending `0`; return `0` |
| `fc07_interest_spending` | cash interest is Spending | **PASS** | nwDelta `-500`; spending `-500`; categoriesSpending `-500`; return `0`; DI `0` |
| `r4_valuation_base_native` | Base vs Native (single-ccy) | **PASS** | Native not forced; Analyze Native OK |
| `r4_scope_portfolio_account` | Portfolio / Account | **PASS** | household + cash + loan days OK; instrument N/A on loan fixture |
| `r4_cash_include_exclude_draw` | Include vs Exclude on draw | **PASS** | includeCashDietz `1` vs exclude `0`; investedCapital include `86666.6667` |
| `r4_settings_week_currency` | settings week/currency | **PASS** | week_start=`monday`, currency=`AUD`, width `1280` |

### 5.2 `timezone-r4` — 23/23 PASS

Source: `probes/r4-loan-2026-09-09/probe-timezone-r4.json` (`allPass: true`)

| Group | IDs (all PASS) | Notes |
|-------|----------------|-------|
| DST reject | `dst_gap_ny`, `dst_gap_la`, `dst_ambiguity_ny`, `dst_ambiguity_la` | LA 2026-03-08 02:30 gap; LA 2026-11-01 01:30 ambiguity |
| Local boundaries | `local_{la,sgt,utc}_{spring_boundary,fall_boundary,spring_afternoon}` | Origin-local day assignment |
| Day length | `la_day_length_spring`, `la_day_length_fall`, `la_origin_bounds_spring_fall` | 23h / 25h |
| Analysis Dietz | `analysis_la_spring_assigns_day`, `analysis_la_fall_assigns_day`, `*_dietz_noon_weight` | In-memory fall-back (no Nov rebuild) |
| Origin DBs | `db_origin_asia-singapore`, `db_origin_utc`, `db_origin_america-los_angeles` | Persisted Origin TZ |

### 5.3 `r4-matrix` @ `dd6cf35` — 46 PASS / 0 FAIL / 1 SKIP / 10 BLOCKED

Source: `probes/r4-loan-2026-09-09/probe-r4-matrix-dd6cf35.json` (`allPass: true`)  
Prior mid-harness: `probe-r4-matrix.json` (44 PASS before USD/CNY DBs) — superseded by dd6cf35 rollup.

| Status | Count | Notes |
|--------|------:|-------|
| PASS | 46 | OS Linux, week validate, locale validate, window validate, DST suite, Origin DBs, loan FC-07 suite, complete instrument/cash-include, **`env_base_usd`**, **`env_base_cny`** |
| FAIL | 0 | — |
| SKIP | 1 | `m_scope_instrument_loan` — loan fixture has no instruments; `m_scope_instrument` PASS on complete DB |
| BLOCKED | 10 | See §8 (probe placeholders at matrix time; desktop later closed several; host/calendar remain) |

Matrix env (from harness):

- `NESTWORTH_QA_LOAN_DB` → loan-fc07 DB  
- `NESTWORTH_QA_COMPLETE_DB` → r3 complete AUD  
- `NESTWORTH_QA_COMPLETE_USD_DB` / `_CNY_DB` → r4 complete-usd/cny  

**Exit probes: PASS** (`allPass: true` with SKIP/BLOCKED allowed)

---

## 6. Unit / frontend baseline / 单元与前端

| Gate | Result | Evidence |
|------|--------|----------|
| Debt/loan/interest unit suite | **PASS** | `report/r4-loan-2026-09-09/SUMMARY.json`, `BASELINE-UNIT.json` (incl. Cases 16/17, Case 17b, signed spending) |
| DST/timezone unit suite | **PASS** | `TestResolveLocalDateTimeRejectsDSTGapAndAmbiguity`, Case 40, Dietz noon weights |
| Settings / valuation adjacent | **PASS** | weekStart wire keys; sqlite daily valuation |
| Frontend targeted (ActivityDetailSheet debt UI, Settings TZ, date-time-picker weekStart) | **PASS** — **19** / 0 FAIL / 3 files | `SUMMARY.json` |
| FE-MORE + FE-ONBOARD + Go analysis subset + C1 fixtures | **PASS** — vitest **85** / 0 FAIL across 9 files; Go top-level 47 PASS | `FE-GAP.json` |
| `go test ./cmd/analytics-qa-probe` | **PASS** | harness `unit_tests.analytics-qa-probe` |
| `go test ./cmd/analytics-qa-seed` | **PASS** | harness `unit_tests.analytics-qa-seed` |
| Linux `cold-3y` | **Known FAIL** (non-blocking) | DEF-R2-01 carry; not re-gated as R4 exit |

---

## 7. Desktop Linux QA / 桌面

### 7.1 FC07-D1 — Insights on `loan-fc07` — **PASS**

Source: `screenshots/r4-loan-2026-09-09/fc07/RESULTS.json`  
Display `:11`; date range **2026-07-27 → 2026-07-31**; DB `loan-fc07`.

#### History

| Date | Event | Amount | Instrument | Category |
|------|-------|--------|------------|----------|
| 2026-07-27 | Drew | **A$100,000.00** | AUD Loan | Principal |
| 2026-07-29 | Paid | **A$10,000.00** | AUD Loan | Principal |
| 2026-07-31 | Paid | **A$1,000.00** + fee **A$500.00** | AUD Loan | Principal |

History check: **PASS** (`01-history.png`)

#### Change Drivers (Jul 27–31)

| Field | UI observed |
|-------|-------------|
| Beginning | **A$20,000.00** |
| Ending | **A$19,500.00** |
| Change | **−A$500.00** |
| Spending | **−A$500.00** |
| Cash flows | **−A$500.00** |
| False huge return from principal | **false** |

Drivers check: **PASS** (`02-drivers-range.png`)

#### Categories

| Field | UI observed |
|-------|-------------|
| Spending total | **−A$500.00** |
| Spending account | AUD Cash **−A$500.00** |
| Dividend & Interest | **—** (not DI) |

Categories / interest drivers: **PASS** (`03-categories-or-drivers-interest.png`)

**Desktop FC-07: PASS** — matches seed/probe oracles (draw/repay NW flat; interest Spending −500, not return/DI).

### 7.2 Environment desktop — locale / window / TZ / week

Source: `screenshots/r4-loan-2026-09-09/RESULTS-DESK-ENV.json` (+ mirrored `report/r4-loan-2026-09-09/RESULTS-DESK-ENV.json`)

| ID | Status | Notes | Screenshots |
|----|--------|-------|-------------|
| **DESK-LOC** (M-LOC-VIS-EN/ZHCN/ZHTW) | **PASS** | English, 简体中文, 繁體中文 all switched; Insights strings changed; Return Analysis remained loaded; restored to English | `locale/en.png`, `zh-CN.png`, `zh-TW.png` |
| **DESK-WIN** (M-WIN-VIS-*) | **PASS** | Usable at default ~1280 and ~900 narrow; no blank/crash; calendar reflowed. **True ~1440 N/A** on 1280-wide physical display; `win/03-1440.png` records max-width attempt | `win/01-1280.png`, `02-narrow.png`, `03-1440.png` |
| **DESK-TZ** (D-TZ) | **PARTIAL** | Settings timezone is **informational**: System timezone resolves to **UTC**; only that option listed — Asia/Singapore ↔ UTC switching **not available**. Insights/Return Analysis loaded; September 2026 calendar coherent; no crash | `tz/01-settings-tz.png`, `02-after-switch.png` |
| **DESK-WEEK** | **PARTIAL** | **No week-start setting** in Settings UI. Calendar Monday-first (Mon–Sun); Sunday mode could not be selected; `week/02-sunday.png` same observed state as monday | `week/01-monday.png`, `02-sunday.png` |

PARTIALs are **product/UI limits**, not missed desktop tests. Seed/settings still persist `week_start=sunday` (`loan-fc07-week-sunday` PASS); engine week fold remains Monday-hardcoded (`M-WK-ENGINE` BLOCKED).

---

## 8. Defects / observations / BLOCKED inventory

| ID | Severity | Status | Description |
|----|----------|--------|-------------|
| `m_os_macos_apple_silicon` | Host gap | **BLOCKED** | Runner is Linux amd64, not darwin/arm64 |
| `m_dst_fall_snapshot_rebuild` | Calendar | **BLOCKED** | 2026-11-01 is not a closed day on a 2026-09-09 wall clock; fall-back covered via `activityLocalDate` + in-memory `ComputeAnalysis` |
| `M-WK-ENGINE` | Product | **BLOCKED** | `assetTrendPeriod` week fold hardcoded Monday; settings `week_start` is UI preference |
| DESK-TZ / DESK-WEEK | Product UI | **PARTIAL** | No Origin TZ picker beyond System→UTC; no week-start control in Settings |
| M-WIN-VIS-1440 | Display | **PARTIAL/N/A** | Physical display 1280; max-width attempt archived |
| `m_scope_instrument_loan` | Fixture design | **SKIP** | Loan DB has cash+loan only; instrument smoke PASS on complete DB |
| DEF-R2-01 `cold-3y` | Known | **FAIL (non-blocking)** | Linux cold 3y performance — carry from R2/R3; not an R4 Loan exit gate |
| Probe matrix desktop placeholders | Historical | Superseded | At probe time `d_fc07_insights`, `d_tz_history_origin`, `m_locale_visual_*`, `m_window_visual_*` were BLOCKED placeholders; **desktop §7 closed** FC-07 / locale / window (TZ PARTIAL) |

No new product FAIL against FC-07 oracles. No invented FX fills.

---

## 9. PLAN checklist self-review / 自检

Walked `PLAN-R4-LOAN.md` §2 after desktop evidence. Runnable rows: **none left open**.

| ID | Final status | Notes |
|----|--------------|-------|
| FC07-S1..S4, FC07-P1..P3, FC07-U1 | **PASS** | Seed + probe + unit |
| **FC07-D1** | **PASS** | Desktop History/Drivers/Categories (§7.1) |
| M-OS-LINUX | **PASS** | |
| M-OS-MAC | **BLOCKED** | Host |
| M-WIN-1280/1440/NARROW (validate) | **PASS** | Probe settings.Validate |
| **M-WIN-VIS-1280 / NARROW** | **PASS** | Desktop; 1440 N/A note |
| **M-WIN-VIS-1440** | **PARTIAL** | Display limit, not missed test |
| M-LOC-EN/ZHCN/ZHTW (validate) | **PASS** | |
| **M-LOC-VIS-*** | **PASS** | Desktop EN/zh-CN/zh-TW |
| M-CCY-AUD/CNY/USD, ENV-BASE-USD/CNY | **PASS** | Onboarded sqlite |
| M-TZ-SGT/UTC/LA, M-DST-* (except rebuild) | **PASS** | timezone-r4 + matrix |
| M-DST-FALL-REBUILD | **BLOCKED** | Calendar |
| M-WK-MON/SUN | **PASS** | Seed/settings |
| M-WK-ENGINE | **BLOCKED** | Product hardcode |
| M-VAL-*, M-SCP-*, M-CASH-* | **PASS** (instrument loan SKIP by design) | |
| **D-TZ** | **PARTIAL** | System→UTC informational only |
| V3-COMPLETE | **PASS** | AUD complete still green for matrix |

**Runnable open rows: none.** PARTIALs documented as product/UI/display limits.

---

## 10. Exit criteria / 退出标准

| Criterion | Met? |
|-----------|------|
| Every `loan-fc07*` scenario seeds OVERALL PASS | **Yes** |
| `complete-usd` / `complete-cny` OVERALL PASS; AUD complete unchanged for matrix | **Yes** |
| Probes `loan-fc07`, `timezone-r4`, `r4-matrix` write JSON `allPass=true` exit 0 | **Yes** |
| FC-07 desktop Insights matches oracles (NW flat draw/repay; Spending −500; DI —) | **Yes** |
| Locale visual EN/zh-CN/zh-TW exercised | **Yes** |
| Window default + narrow usable; 1440 noted N/A | **Yes** |
| Remaining BLOCKED have explicit reasons (macOS / Nov rebuild / engine Monday) | **Yes** |
| Round-3 REPORT / RETEST / STATUS files untouched | **Yes** |
| Evidence archived under docs `*/r4-loan-2026-09-09/` | **Yes** |

**Exit: ACCEPT (Linux Round-4 Loan+TZ).**

---

## 11. Screenshot / evidence index / 证据索引

### Screenshots (`screenshots/r4-loan-2026-09-09/`)

| Path | Contents |
|------|----------|
| `fc07/01-history.png` | History draw / repay / interest |
| `fc07/02-drivers-range.png` | Drivers Jul 27–31 begin A$20,000 → end A$19,500 |
| `fc07/03-categories-or-drivers-interest.png` | Categories Spending −A$500; DI — |
| `fc07/RESULTS.json` | Desktop FC-07 rollup |
| `locale/{en,zh-CN,zh-TW}.png` | Locale visual PASS |
| `win/{01-1280,02-narrow,03-1440}.png` | Window visual (1440 = max attempt) |
| `tz/{01-settings-tz,02-after-switch}.png` | TZ PARTIAL evidence |
| `week/{01-monday,02-sunday}.png` | Week PARTIAL (no UI control) |
| `RESULTS-DESK-ENV.json` | DESK-LOC/WIN/TZ/WEEK statuses |

### Probes (`probes/r4-loan-2026-09-09/`)

| File | Role |
|------|------|
| `HARNESS-LOCAL.json` | Full seed+probe+unit rollup @ `dd6cf35` |
| `probe-loan-fc07.json` | 7/7 |
| `probe-timezone-r4.json` | 23/23 |
| `probe-r4-matrix.json` | Mid-harness (pre-USD/CNY matrix) |
| `probe-r4-matrix-dd6cf35.json` | Authoritative **46/0/1/10** |

### Seeds / report mirrors

| Path | Contents |
|------|----------|
| `seed/r4-loan-2026-09-09/seed-results-*.json` | Loan family + complete-usd/cny (JSON only) |
| `report/r4-loan-2026-09-09/RESULTS.json` | FC-07 desktop RESULTS mirror |
| `report/r4-loan-2026-09-09/RESULTS-DESK-ENV.json` | Env desktop RESULTS mirror |
| `report/r4-loan-2026-09-09/{SUMMARY,BASELINE-UNIT,FE-GAP}.json` | Unit/FE baselines |

Run-machine only (not all git-committed): scenario sqlite DBs under `/workspace/nestworth-analytics-qa-r4/{loan-fc07*,complete-*}/data/`, probe binaries under `bin/`, verbose logs under `logs/`.

Historical (unchanged): `REPORT.md`, `REPORT-RETEST-2026-09-09.md`, `STATUS.md`, `STATUS-RETEST-2026-09-09.md`, `screenshots/retest-2026-09-09/`.

---

## 12. Recommendations / 建议

1. Keep **macOS Apple Silicon**, **Nov fall-back snapshot rebuild**, and **engine Monday week fold** as explicit follow-ups — do not silently SKIP.
2. Product follow-up: expose History Origin timezone picker beyond System→UTC; optional Settings week-start control aligned with `settings.Validate` (`monday`/`sunday`).
3. Prefer vitest for FE gates; retain Cases 16/17 debt oracles as regression anchors for any loan UI change.
4. Do not conflate DEF-R2-01 `cold-3y` with R4 Loan acceptance.

---

## 13. Chinese executive bullets / 中文要点

- **结论：** Round-4 Loan+TZ **Linux ACCEPT**。种子/探针全绿；桌面 FC-07 **PASS**；语言/窗口 **PASS**；时区/周起始 **PARTIAL**（产品 UI 限制）；macOS / 11 月重建 / 引擎周一硬编码仍为 **BLOCKED**。
- **版本：** 工具链头 **`dd6cf35`**；桌面产品二进制 **`fa46d30`** 谱系；PR **#15**。
- **FC-07：** History 提款/还款/利息齐全；Drivers 期初 **A$20,000** → 期末 **A$19,500**，变动 **−A$500**，支出 **−A$500**；Categories 支出 **−A$500**，股息利息 **—**。
- **探针：** loan-fc07 **7/7**；timezone-r4 **23/23**；r4-matrix **46 PASS / 0 FAIL / 1 SKIP / 10 BLOCKED**。
- **种子：** loan-fc07 五场景 + complete-usd/cny 均 PASS；ENV-BASE 已落地真实 sqlite。
- **文档：** 本报告为新建文件；未改写 Round-3 `REPORT.md` / `REPORT-RETEST` / `STATUS.md` / `STATUS-RETEST`。

---

*End of Round-4 LOAN+TZ REPORT. Generated 2026-09-09 SGT from artifact root `/workspace/nestworth-analytics-qa-r4/` against tooling head `dd6cf35` with desktop binary lineage `fa46d30`.*
