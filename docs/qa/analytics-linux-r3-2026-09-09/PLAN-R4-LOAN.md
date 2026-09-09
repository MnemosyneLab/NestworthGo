# Nestworth Insights QA — Round-4 Loan + Timezone Plan

| Field | Value |
|---|---|
| Base | Branch `qa/analytics-linux-r3-2026-09-09` (PR #15); do not rewrite Round-3 `REPORT.md` |
| Prior | Round-3 mini-family v3 (`complete` / `missing-*`); FC-07 loan skipped; full TZ/DST matrix deferred |
| Goal | Linux QA can run **seed → probe** for FC-07 Loan and every **runnable** item in test-plan §5 + FC-07 |
| Out | Rewriting R3 reports; inventing FX fills; weakening Cases 16/17 or history_regression oracles |
| Artifact root (suggested) | `/tmp/nestworth-qa-loan-fc07/` and `/tmp/nestworth-qa-v3-complete/` |
| Docs target | `docs/qa/analytics-linux-r3-2026-09-09/` (this plan + `STATUS-R4-LOAN.md` only) |

Self-review rule: walk the checklist before declaring done. Any **runnable** row that is not PASS must be implemented or marked **BLOCKED** with a reason. SKIP is allowed only when a fixture is absent by design (and `allPass` still holds). Desktop-visual / macOS-host rows stay BLOCKED on this Linux VM.

---

## 0. Principles

1. **Separate scenario family, not a v3 graft** — `loan-fc07*` must not change complete / missing-price / missing-fx / missing-both.
2. **Seed + probe first**, desktop second — JSON PASS/FAIL/SKIP/BLOCKED is the gate.
3. **Stay inside test-plan v1 FC-07** — draw / repay / cash interest numbers are acceptance oracles.
4. **Reuse existing DST oracles** — `ResolveLocalDateTime` gap/ambiguity, `activityLocalDate`, Case 40, `ComputeAnalysis` day assignment. Do not invent calendar rules.
5. **AUD remains the R3 default**; CNY and USD are **separate scenario IDs** so they cannot break AUD R3 fixtures.

---

## 1. Product contracts (FC-07)

From `docs/testing/nestworth-analytics-test-plan.md` and `TestAnalysisReviewCases16And17RealDebtPaymentPath`:

| Step | Ledger | Net worth | Must not be |
|---|---|---|---|
| Drawdown | Cash +100000, Debt +100000 | change = 0 | Investment return |
| Principal repayment | Cash −10000, Debt −10000 | change = 0 | Investment return |
| Cash interest (`InterestOrFee` on `DebtPaymentInput`) | Cash −(principal+500), debt principal −principal only | −500; **Spending −500** | Dividend & Interest / investment return |

API: `DebtDrawInput`, `DebtPaymentInput` (principal **must be > 0**; interest/fee optional). Account `TypeLoan` / `RoleLiability`. Interest-day principal in the fixture is **1000** (same as Case 17) because an InterestOrFee-only payment is rejected. This matches existing review cases; do not invent a zero-principal path.

UI **Portfolio** scope is engine `ScopeHousehold`.

Engine note: Insights week fold in `assetTrendPeriod` is **hardcoded Monday**. Settings `week_start` (`monday`/`sunday`) is a **UI** preference (`settings.Validate`). Sunday is still a required seed/settings row.

---

## 2. Full checklist

Status values: **PASS** (automation green on this host), **SKIP** (fixture optional/absent, not a fail), **BLOCKED** (cannot run here; reason in Notes), **PENDING** (desktop Linux QA after this tooling — still a checklist row). After a full harness run, replace PENDING/blank with the observed probe/seed status in `STATUS-R4-LOAN.md`. Do not edit Round-3 `REPORT.md`.

| ID | Item | runnable? | owner | status | Notes |
|---|---|---|---|---|---|
| FC07-S1 | Seed draw: NW delta 0, not return | yes | seed | PASS | `fc07_draw_nw0` |
| FC07-S2 | Seed repay principal: NW delta 0 | yes | seed | PASS | `fc07_repay_nw0` |
| FC07-S3 | Seed cash interest: Spending −500, NW −500, not DI | yes | seed | PASS | `fc07_interest_spending` |
| FC07-S4 | InterestOrFee-only needs non-zero principal | yes | docs | PASS | principal 1000 + fee 500 = Case 17 |
| FC07-P1 | Probe draw NW 0 | yes | probe | PASS | `loan-fc07` |
| FC07-P2 | Probe repay NW 0 | yes | probe | PASS | `loan-fc07` |
| FC07-P3 | Probe interest Spending not return | yes | probe | PASS | `loan-fc07` |
| FC07-U1 | `TestAnalysisReviewCases16And17RealDebtPaymentPath` | yes | unit | PASS | keep green |
| FC07-D1 | Desktop Insights FC-07 | no (desktop later) | desktop | BLOCKED | Linux desktop QA after probes |
| M-OS-LINUX | Linux seed/probe host | yes | probe | PASS | `r4-matrix` `m_os_linux` |
| M-OS-MAC | macOS Apple Silicon | no | desktop | BLOCKED | this runner is not darwin/arm64 |
| M-WIN-1280 | `settings.Validate` width 1280 | yes | probe | PASS | `m_window_1280` |
| M-WIN-1440 | `settings.Validate` width 1440 | yes | probe | PASS | `m_window_1440` |
| M-WIN-NARROW | `settings.Validate` width 800 | yes | probe | PASS | `m_window_narrow` |
| M-WIN-VIS-1280 | Visual ~1280×800 | no | desktop | BLOCKED | desktop QA row; Linux can do later |
| M-WIN-VIS-1440 | Visual ~1440×900 | no | desktop | BLOCKED | desktop QA row |
| M-WIN-VIS-NARROW | Visual narrow width | no | desktop | BLOCKED | desktop QA row |
| M-LOC-EN | `settings.Validate` language=en | yes | probe | PASS | enum only |
| M-LOC-ZHCN | `settings.Validate` language=zh-CN | yes | probe | PASS | enum only |
| M-LOC-ZHTW | `settings.Validate` language=zh-TW | yes | probe | PASS | enum only |
| M-LOC-VIS-EN | English UI strings | no | desktop | BLOCKED | locale strings are desktop later |
| M-LOC-VIS-ZHCN | 简体中文 UI strings | no | desktop | BLOCKED | locale strings are desktop later |
| M-LOC-VIS-ZHTW | 繁体中文 UI strings | no | desktop | BLOCKED | locale strings are desktop later |
| M-CCY-AUD | Seed `complete` AUD | yes | seed | PASS | default v3; must stay green |
| M-CCY-CNY | Seed `complete-cny` + `loan-fc07-cny` | yes | seed+probe | PASS | real onboarded DB; `env_base_cny` |
| M-CCY-USD | Seed `complete-usd` + `loan-fc07-usd` | yes | seed+probe | PASS | real onboarded DB; `env_base_usd`; `NESTWORTH_QA_BASE_CURRENCY=USD` |
| ENV-BASE-USD | Onboarded `complete` household base USD | yes | seed+probe | PASS | `complete-usd` or env knob; not in-memory |
| ENV-BASE-CNY | Onboarded `complete` household base CNY | yes | seed+probe | PASS | `complete-cny` or env knob; not in-memory |
| M-TZ-SGT | Origin `Asia/Singapore` | yes | seed+probe | PASS | `loan-fc07` + timezone-r4 Origin DB |
| M-TZ-UTC | Origin `UTC` | yes | seed+probe | PASS | `loan-fc07-utc` + timezone-r4 Origin DB |
| M-TZ-LA | Origin `America/Los_Angeles` spring | yes | probe | PASS | timezone-r4 Origin DB (closed days) |
| M-DST-GAP-LA | Reject LA 2026-03-08 02:30 | yes | probe | PASS | `dst_gap_la` |
| M-DST-AMB-LA | Reject LA 2026-11-01 01:30 | yes | probe | PASS | `dst_ambiguity_la` |
| M-DST-SPRING | LA spring-forward 23h + day assign | yes | probe | PASS | timezone-r4 |
| M-DST-FALL | LA fall-back 25h + day assign | yes | probe | PASS | in-memory; not snapshot rebuild |
| M-DST-FALL-REBUILD | Rebuild snapshots on 2026-11-01 | no | probe | BLOCKED | Nov 2026 is not a closed day on 2026-09-09 wall clock |
| M-WK-MON | Seed/settings week_start=monday | yes | seed+probe | PASS | `loan-fc07` |
| M-WK-SUN | Seed/settings week_start=sunday | yes | seed+probe | PASS | `loan-fc07-week-sunday` |
| M-WK-ENGINE | Engine week fold follows settings | no | unit | BLOCKED | `assetTrendPeriod` is hardcoded Monday; settings is UI |
| M-VAL-BASE | Analyze/AssetChange Base on loan | yes | probe | PASS | `r4_valuation_base_native` |
| M-VAL-NATIVE | Native not forced on single-ccy loan | yes | probe | PASS | cheap smoke |
| M-VAL-NATIVE-MIXED | Native forces Base on mixed complete | yes | probe | PASS | needs complete DB; else SKIP |
| M-SCP-PF | Scope Portfolio = household | yes | probe | PASS | loan DB |
| M-SCP-ACCT | Scope Account cash + loan | yes | probe | PASS | loan DB |
| M-SCP-INST | Scope Instrument | yes | probe | PASS | complete DB `inst_*`; SKIP on loan-only |
| M-CASH-INCL | Include cash Dietz on draw day | yes | probe | PASS | loan draw is a capital-flow day |
| M-CASH-EXCL | Exclude differs on draw day | yes | probe | PASS | same |
| M-CASH-SALARY | Include vs Exclude on v3 salary day | yes | probe | PASS | complete DB FL-18/19 pattern; else SKIP |
| D-TZ | Desktop History Origin tz | no | desktop | BLOCKED | after probes |
| V3-COMPLETE | v3 `complete` seed still OVERALL PASS | yes | seed | PASS | must stay green |

---

## 3. Phases

### Phase L — Loan seed family (`cmd/analytics-qa-seed`)

| ID | Work | Done when |
|---|---|---|
| L1 | `{CCY} Cash` + `{CCY} Loan` before `StartHistory`; tz from scenario; anchor `2026-07-26T00:00:00Z` | DB created under output dir |
| L2 | Day 1 draw 100000; day 3 repay 10000; day 5 pay 1000 + interest 500 | Activities persist; rebuild window covers all three |
| L3 | Named steps `fc07_draw_nw0`, `fc07_repay_nw0`, `fc07_interest_spending` | OVERALL PASS |
| L4 | Scenario IDs `loan-fc07-utc` / `-cny` / `-usd` / `-week-sunday` | each OVERALL PASS; v3 `complete` unchanged |
| L5 | `complete-usd` / `complete-cny` (or `NESTWORTH_QA_BASE_CURRENCY`) onboard real DBs | `households.base_currency` matches; snapshots rebuild; AUD `complete` / `missing-*` unchanged |

**Exit L:** every `loan-fc07*` scenario in the table seeds OVERALL PASS; `complete-usd` and `complete-cny` OVERALL PASS.

### Phase P — Probes (`cmd/analytics-qa-probe`)

| ID | Mode | Asserts |
|---|---|---|
| P1 | `loan-fc07` | FC-07 oracles; Base vs Native; household + account; Include vs Exclude on draw; settings week/currency |
| P2 | `timezone-r4` | Origin TZ SGT / UTC / LA; local dates around DST; **LA and NY** gap/ambiguity rejected; LA 23h / 25h |
| P3 | `r4-matrix` | P2 + settings.Validate week/window/locale + host BLOCKED rows + loan extras + complete instrument/cash-include + `env_base_usd` / `env_base_cny` when those DBs exist |

Fall-back 2026-11-01 is **after** a 2026-09-09 wall clock, so snapshot rebuild cannot cover November. P2: (a) builds spring Origin DBs (closed days), (b) asserts fall-back via `activityLocalDate` + in-memory `ComputeAnalysis`. No FX fills.

**Exit P:** all three modes write JSON `allPass=true` and exit 0 (SKIP/BLOCKED allowed).

### Phase D — Desktop (Linux QA agent, after this tooling)

| ID | Work | Done when |
|---|---|---|
| D-FC07 | Insights on `loan-fc07` DB: draw/repay NW flat; interest Categories Spending −500 | RESULTS.json |
| D-TZ | Optional: timezone-r4 DB History Origin tz | RESULTS.json |
| D-VIS | Window 1280 / 1440 / narrow; locale strings | RESULTS.json |
| D-MAC | macOS Apple Silicon | **not this host** |

---

## 4. Suggested runbook

```bash
ROOT=/tmp/nestworth-qa-loan-fc07
for s in loan-fc07 loan-fc07-utc loan-fc07-cny loan-fc07-usd loan-fc07-week-sunday; do
  NESTWORTH_QA_OUTPUT_DIR="$ROOT/$s" NESTWORTH_QA_RESET=1 \
  NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
  NESTWORTH_QA_SCENARIO=$s \
  go run ./cmd/analytics-qa-seed
done

NESTWORTH_DATABASE_PATH="$ROOT/loan-fc07/data/nestworth.db" \
NESTWORTH_QA_OUTPUT_DIR="$ROOT/loan-fc07" \
NESTWORTH_PROBE_MODE=loan-fc07 \
NESTWORTH_PROBE_OUT="$ROOT/loan-fc07/seed/probe-loan-fc07.json" \
go run ./cmd/analytics-qa-probe

NESTWORTH_QA_OUTPUT_DIR="$ROOT" \
NESTWORTH_PROBE_MODE=timezone-r4 \
NESTWORTH_PROBE_OUT="$ROOT/seed/probe-timezone-r4.json" \
go run ./cmd/analytics-qa-probe

NESTWORTH_QA_OUTPUT_DIR="$ROOT" \
NESTWORTH_QA_LOAN_DB="$ROOT/loan-fc07/data/nestworth.db" \
NESTWORTH_QA_COMPLETE_DB=/tmp/nestworth-qa-v3-complete/data/nestworth.db \
NESTWORTH_QA_COMPLETE_USD_DB=/tmp/nestworth-qa-v3-complete-usd/data/nestworth.db \
NESTWORTH_QA_COMPLETE_CNY_DB=/tmp/nestworth-qa-v3-complete-cny/data/nestworth.db \
NESTWORTH_PROBE_MODE=r4-matrix \
NESTWORTH_PROBE_OUT="$ROOT/seed/probe-r4-matrix.json" \
go run ./cmd/analytics-qa-probe
```

§5 USD/CNY onboarded DBs (AUD `complete` stays default):

```bash
for s in complete-usd complete-cny; do
  NESTWORTH_QA_OUTPUT_DIR=/tmp/nestworth-qa-v3-$s NESTWORTH_QA_RESET=1 \
  NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z NESTWORTH_QA_SCENARIO=$s \
  go run ./cmd/analytics-qa-seed
done
```

Keep v3 smoke (`complete` / `missing-*`) unchanged; see `cmd/analytics-qa-seed/README.md`.

---

## 5. Decision log

- AUD default (not CNY) for R3 continuity; Case 16/17 numbers are currency-agnostic oracles
- CNY / USD for §5 use **separate** `complete-usd` / `complete-cny` (or `NESTWORTH_QA_BASE_CURRENCY` on `complete`); `missing-*` remain AUD
- USD/CNY FX quotes are the existing AUD schedule retargeted to the household base (plus a documented USD/CNY series); not a missing-quote fill
- Interest day uses principal 1000 + fee 500 because `DebtPaymentInput` rejects zero principal
- Fall-back DST via in-memory analysis: November 2026 is not a closed day on a Sep 2026 wall clock
- Product code for debt/DST is **out of scope** unless probes FAIL against the existing oracles
- Locale **visual** strings and window **visual** layout stay desktop rows even though `settings.Validate` is cheap on Linux
- macOS Apple Silicon is BLOCKED on this Linux host, not skipped silently
