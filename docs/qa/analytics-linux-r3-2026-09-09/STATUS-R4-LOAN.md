# Nestworth Insights QA — Round-4 Loan + Timezone STATUS

| Field | Value |
|-------|-------|
| When | **2026-09-09** (Asia/Singapore) — Linux Cloud VM **with desktop evidence** |
| Branch | `qa/analytics-linux-r3-2026-09-09` |
| Tooling head | **`dd6cf35`** |
| Desktop binary lineage | **`fa46d30`** (R3 remediation retest Wails ELF) |
| PR | [#15](https://github.com/MnemosyneLab/NestworthGo/pull/15) |
| Full report | `REPORT-R4-LOAN-2026-09-09.md` |
| Plan | `PLAN-R4-LOAN.md` |
| Evidence | `screenshots/r4-loan-2026-09-09/`, `probes/r4-loan-2026-09-09/`, `seed/r4-loan-2026-09-09/`, `report/r4-loan-2026-09-09/` |

Leave Round-3 `REPORT.md` / `REPORT-RETEST-2026-09-09.md` / `STATUS.md` / `STATUS-RETEST-2026-09-09.md` as historical baselines.

## Verdict

**ROUND-4 LOAN+TZ PASS (Linux).** Seed → probe → unit/FE baseline → **native desktop** FC-07 and env visuals complete. Every runnable PLAN row is PASS or intentional SKIP/BLOCKED. PARTIALs (DESK-TZ, DESK-WEEK, 1440 width) are product/UI/display limits — not missed tests.

## Harness + desktop (this VM)

| Gate | Result |
|------|--------|
| Seed `loan-fc07` (AUD, Asia/Singapore, monday) | **PASS** (`fc07_draw_nw0` / `fc07_repay_nw0` / `fc07_interest_spending`) |
| Seed `loan-fc07-utc` | **PASS** (tz=UTC) |
| Seed `loan-fc07-cny` | **PASS** (base=CNY) |
| Seed `loan-fc07-usd` | **PASS** (base=USD) |
| Seed `loan-fc07-week-sunday` | **PASS** (`week_start=sunday`) |
| Seed `complete-usd` / `complete-cny` (+ via-env USD) | **PASS** (onboarded sqlite; 36/0) |
| Probe `loan-fc07` | **PASS** 7/7 |
| Probe `timezone-r4` / LA DST | **PASS** 23/23 (`dst_gap_la` rejects 2026-03-08 02:30) |
| Probe `r4-matrix` @ `dd6cf35` | **PASS** `46 PASS / 0 FAIL / 1 SKIP / 10 BLOCKED` |
| Unit baseline + FE gaps | **PASS** (`SUMMARY.json`, `FE-GAP.json`) |
| `go test ./cmd/analytics-qa-seed ./cmd/analytics-qa-probe` | **PASS** |
| **Desktop FC-07** (History / Drivers / Categories) | **PASS** — begin **A$20,000** → end **A$19,500** change **−A$500** spending **−A$500**; DI **—** |
| **DESK-LOC** EN / zh-CN / zh-TW | **PASS** |
| **DESK-WIN** default + narrow | **PASS** (1440 N/A on 1280 display) |
| **DESK-TZ** System→UTC | **PARTIAL** informational |
| **DESK-WEEK** week-start UI | **PARTIAL** no control; calendar Mon-first |
| macOS Apple Silicon | **BLOCKED** |
| M-DST-FALL-REBUILD Nov closed-day | **BLOCKED** |
| M-WK-ENGINE Monday hardcode | **BLOCKED** |
| cold-3y (DEF-R2-01) | Known FAIL non-blocking |

FC-07 probe + desktop oracles (AUD SGT, draw 2026-07-27 / repay 2026-07-29 / interest 2026-07-31):

- Draw/repay: `nwDelta=0`, spending/return/DI = 0
- Interest: `nwDelta=-500`, `spending=-500`, categories Spending `-500`, return 0, DI —
- Interest day principal **1000** + fee **500** (Case 17)
- Draw Include vs Exclude: cash Dietz flows 1 vs 0

Probe matrix SKIP/BLOCKED that remain host/calendar (desktop visual placeholders superseded by § desktop above):

| Probe ID | Status | Reason |
|---|---|---|
| `m_scope_instrument_loan` | SKIP | loan fixture has no instruments; `m_scope_instrument` PASS on complete DB |
| `m_os_macos_apple_silicon` | BLOCKED | host is Linux, not darwin/arm64 |
| `m_dst_fall_snapshot_rebuild` | BLOCKED | 2026-11-01 is not a closed day on a 2026-09-09 wall clock |
| `m_locale_visual_*` / `m_window_visual_*` / `d_fc07_insights` / `d_tz_history_origin` | Probe-time BLOCKED | **Closed on desktop** (locale/window/FC-07 PASS; TZ PARTIAL) — see `RESULTS-DESK-ENV.json` + `fc07/RESULTS.json` |

## Self-review

Walked `PLAN-R4-LOAN.md` checklist after desktop archive. **No runnable row still open.** PARTIALs = product/UI/display limits. Full case tables and screenshot index: `REPORT-R4-LOAN-2026-09-09.md`.
