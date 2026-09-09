# Nestworth Insights QA — Round-4 Loan + Timezone STATUS

| Field | Value |
|-------|-------|
| When | 2026-09-09 (Linux Cloud VM; desktop not run) |
| Branch | `qa/analytics-linux-r3-2026-09-09` |
| Docs | `docs/qa/analytics-linux-r3-2026-09-09/PLAN-R4-LOAN.md` |

This stub is not a Round-4 desktop acceptance report. Leave `REPORT.md` / `REPORT-RETEST-2026-09-09.md` / `STATUS.md` as Round-3 history.

## Verdict

**TOOLING + LINUX HARNESS READY.** Every **runnable** PLAN checklist row was executed. Desktop visual / macOS Apple Silicon / November snapshot-rebuild remain **BLOCKED** with reasons. One **SKIP**: instrument scope on the loan-only fixture (instrument smoke ran on v3 complete instead).

## Harness (this VM)

| Gate | Result |
|------|--------|
| Seed `loan-fc07` (AUD, Asia/Singapore, monday) | **PASS** (`fc07_draw_nw0` / `fc07_repay_nw0` / `fc07_interest_spending`) |
| Seed `loan-fc07-utc` | **PASS** (tz=UTC) |
| Seed `loan-fc07-cny` | **PASS** (base=CNY) |
| Seed `loan-fc07-usd` | **PASS** (base=USD) |
| Seed `loan-fc07-week-sunday` | **PASS** (`week_start=sunday`) |
| Seed v3 `complete` | **PASS** (AUD R3 mini-family unchanged) |
| Probe `loan-fc07` (all five DBs) | **PASS** 7/7 each (FC-07 + Base/Native + household/account + Include/Exclude draw + settings) |
| Probe `timezone-r4` | **PASS** 23/23 (includes LA gap/ambiguity reject + SGT/UTC/LA Origin DBs) |
| Probe `r4-matrix` | **PASS** `44 PASS / 0 FAIL / 1 SKIP / 10 BLOCKED` |
| Unit `TestAnalysisReviewCases16And17RealDebtPaymentPath` | **PASS** (cases 15–17) |
| Unit `TestResolveLocalDateTimeRejectsDSTGapAndAmbiguity` | **PASS** |
| `go test ./cmd/analytics-qa-seed ./cmd/analytics-qa-probe` | **PASS** |

FC-07 probe oracles (AUD SGT, draw 2026-07-27 / repay 2026-07-29 / interest 2026-07-31):

- Draw/repay: `nwDelta=0`, `spending=0`, `returnAmount=0`, `dividendInterest=0`
- Interest: `nwDelta=-500`, `spending=-500`, categories Spending `-500`, return 0
- Interest day still records principal **1000** + fee **500** (API rejects InterestOrFee-only; Case 17)
- Draw Include vs Exclude: cash Dietz flows 1 vs 0
- Native on single-currency loan: not forced to Base

`r4-matrix` SKIP/BLOCKED (none are unchecked runnable rows):

| Probe ID | Status | Reason |
|---|---|---|
| `m_scope_instrument_loan` | SKIP | loan fixture has no instruments; `m_scope_instrument` PASS on complete DB |
| `m_os_macos_apple_silicon` | BLOCKED | host is Linux, not darwin/arm64 |
| `m_locale_visual_*` | BLOCKED | locale strings are desktop later |
| `m_window_visual_*` | BLOCKED | visual window QA is desktop later |
| `d_fc07_insights` / `d_tz_history_origin` | BLOCKED | desktop Insights after this tooling |
| `m_dst_fall_snapshot_rebuild` | BLOCKED | 2026-11-01 is not a closed day on a 2026-09-09 wall clock |

## Self-review

Walked `PLAN-R4-LOAN.md` checklist after the harness. No runnable row left unimplemented. Engine week fold (`assetTrendPeriod` Monday-only) stays BLOCKED as a product/UI split, not a missing probe.

See `PLAN-R4-LOAN.md` §2 for the full ID table.
