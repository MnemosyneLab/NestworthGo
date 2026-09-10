# Nestworth Insights QA Round-3 — STATUS (Remediation Retest)

| Field | Value |
|-------|-------|
| When | **2026-09-09** SGT (UTC+8) |
| Branch | **`qa/analytics-linux-r3-2026-09-09`** |
| Commit under test | **`fa46d30`** — `fix(analytics): preserve exact projection precision` |
| Full SHA | `fa46d30e5210450f168755694f7c12162fdc0f18` |
| Remediation parent | **`d678cf3`** |
| Platform | Linux amd64 |
| Docs | `docs/qa/analytics-linux-r3-2026-09-09/` |
| Run root | `/workspace/nestworth-analytics-qa-r3-retest/` |
| Full report | `REPORT-RETEST-2026-09-09.md` |
| PR | https://github.com/MnemosyneLab/NestworthGo/pull/15 |

## Verdict (one-pager)

**REMEDIATION RETEST PASS** — R3-D5 / R3-D6 / C5–C9 / incomplete matrix / D8 invalidation closed with automated probes **and** native Wails screenshots. Original `REPORT.md` left unchanged as historical baseline @ `91d10c0`.

| Gate | Result |
|------|--------|
| Build | **PASS** (19 855 904 B) |
| Unit Review/HistoryHint/Waterfall/… | **PASS** |
| FE vitest | **PASS** — 344 / 44 files |
| FE bun-native | **SKIP** (use vitest) |
| D5 probe reconcile | **PASS** — delta **0 AUD**; begin 54258.02 → end 59784.3685 |
| Desktop D5 | **PASS** — UI A$54,258.02 → A$59,784.37; no false mismatch |
| Desktop D6 | **PASS** — History Instrument=Apple Inc |
| C5–C8 quantitative | **PASS** — all deltas 0 |
| C9 clean / corrupt | **PASS** — issues 0 / 2 (±100 AUD) |
| C1 / C2 / C4 / DEF-R2-02 | **PASS** — C4 partial 21/45 no panic |
| Incomplete matrix | **PASS** — **15/15** (missing-both/price/fx × banner/Trend/cal/Drivers/zh-CN) |
| Desktop D8 | **PASS** — +A$100 → Net Worth Change +A$5,561.81 → +A$5,661.81 |
| Automation rollup | **ALL PASS** (`RETEST-SUMMARY.json`) |

## Closed this retest

- **R3-D5** — exact reconcile + clean desktop Change Drivers Aug
- **R3-D6** — Realized HistoryHint Instrument=Apple Inc
- **R3-Q1 / C5–C9** — quantitative + residual controls
- **R3-Q2** — incomplete matrix desktop (incl. correct Drivers + zh-CN)
- **R3-Q3** — D8 GUI mutation soft-invalidation
- **DEF-R2-02** — reconfirmed fixed (probe + matrix Trend All)

## Carry

1. **DEF-R2-01** — Linux cold-3y perf (known, non-blocking)

## Exit criteria

- Met: remediation targets with probe + native evidence; report/evidence archived; commit on PR #15 branch
- See `REPORT-RETEST-2026-09-09.md` for full tables and screenshot index
