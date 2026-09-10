# Nestworth Insights QA Round-3 — STATUS

| Field | Value |
|-------|-------|
| When | **2026-09-09** SGT (UTC+8) |
| Branch | **`qa/analytics-linux-r3-2026-09-09`** |
| Commit | Base QA evidence **`91d10c0`**; remediation is currently uncommitted working-tree changes |
| Full SHA | `91d10c0216d31d274857e9bbf71d971dd6b8c7bd` |
| Lineage | DEF-R2-02 soft-FX fix present since **`ea94f48`** lineage |
| Platform | Linux amd64 |
| Docs | `docs/qa/analytics-linux-r3-2026-09-09/` |
| Run root | `/workspace/nestworth-analytics-qa-r3/` |

## Verdict (one-pager)

**AUTOMATED REMEDIATION GREEN; NATIVE DESKTOP REPLAY PENDING** — DEF-R2-02 remains fixed; D5/D6 and C5–C9 are fixed/covered in code and automated probes. The old D5/D6 screenshots remain historical, while missing-price/missing-fx/zh-CN and post-mutation native Wails checks were not rerun.

| Gate | Result |
|------|--------|
| Build | **PASS** (~19.8 MB / 19 835 424 B) |
| Go cold-3y | **FAIL** ~5.08 s want &lt;3 s (DEF-R2-01, non-blocking) |
| MissingFX / Return targeted | **PASS** |
| Phase A DEF-R2-02 harness | **PASS** — FIXED |
| Seed v3 four scenarios | **PASS** — 4/4 OVERALL (35/35 each) |
| Probe C1–C9 | **9 PASS / 0 FAIL / 0 SKIP** (new quantitative + residual controls) |
| Desktop D1–D8 (complete) | Historical **6 PASS / 2 PARTIAL**; D5/D6 fixes not replayed in native Wails |
| Incomplete matrix desktop | **PARTIAL/PENDING** — missing-both evidence exists; missing-price/missing-fx/zh-CN not rerun |
| DEF-R2-02 desktop | **FIXED** — Trend All Partial 21/45, chart OK, no generic load error |

## Open / carry

1. **DEF-R2-01** — Linux cold-3y perf (known, non-blocking)
2. **Native replay** — verify D5 warning is gone and D6 filters land after rebuilding the application
3. Incomplete-matrix desktop — missing-price, missing-fx and zh-CN still pending
4. Native D8 post-mutation replay — application/frontend invalidation tests pass; GUI mutation not rerun

## Closed this round

- **DEF-R2-02** — FIXED (probe partial 21/45 no panic; desktop chart + Partial coverage)
- **R3-D5** — FIXED in application attribution/precision path; top-level exact probe delta=0
- **R3-D6** — FIXED in realized HistoryHint application + Wails DTO path; frontend navigation tests pass
- **R3-Q1/C9** — C5–C9 DB-backed probes pass, including clean/corrupt residual controls

## Exit criteria

- Met: four DBs; no unexpected probe FAIL; C5–C9 automated probes pass; D5/D6 backend and frontend regression tests pass; report written
- Pending: native Wails replay, missing-price/missing-fx/zh-CN matrix, and GUI-level D8 data mutation evidence

See `REPORT.md` for full tables, screenshot index, and evidence paths.
