# Nestworth Insights QA — Round-4 Loan + Timezone STATUS (stub)

| Field | Value |
|-------|-------|
| When | 2026-09-09 (tooling verified in cloud VM; desktop not run) |
| Branch | `qa/analytics-linux-r3-2026-09-09` |
| Docs | `docs/qa/analytics-linux-r3-2026-09-09/PLAN-R4-LOAN.md` |

## Verdict

**TOOLING READY; DESKTOP / FULL REPORT NOT YET RUN.** This stub is not a Round-4 acceptance report. Leave `REPORT.md` / `REPORT-RETEST-2026-09-09.md` / `STATUS.md` as Round-3 history.

| Gate | Result |
|------|--------|
| Seed `loan-fc07` | **PASS** (cloud VM: OVERALL PASS, `fc07_draw_nw0` / `fc07_repay_nw0` / `fc07_interest_spending`) |
| Probe `loan-fc07` | **PASS** (3/3) |
| Probe `timezone-r4` | **PASS** (21/21; spring Origin DBs + DST oracles) |
| v3 `complete` seed | **PASS** (unchanged mini-family) |
| Desktop FC-07 / TZ | not started — Linux QA agent |

See `PLAN-R4-LOAN.md` for exit criteria and runbook.
