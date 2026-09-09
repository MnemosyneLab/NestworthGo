# Nestworth Insights QA Round-3 — STATUS

| Field | Value |
|-------|-------|
| When | **2026-09-09** SGT (UTC+8) |
| Branch | **`qa/analytics-linux-r3-2026-09-09`** |
| Commit | **`91d10c0`** `feat(qa): analytics-linux-qa-v3 mini-family seed (#14)` |
| Full SHA | `91d10c0216d31d274857e9bbf71d971dd6b8c7bd` |
| Lineage | DEF-R2-02 soft-FX fix present since **`ea94f48`** lineage |
| Platform | Linux amd64 |
| Docs | `docs/qa/analytics-linux-r3-2026-09-09/` |
| Run root | `/workspace/nestworth-analytics-qa-r3/` |

## Verdict (one-pager)

**ACCEPTANCE NEARLY MET** — Round-2 blocker **DEF-R2-02 FIXED** (probe + desktop). Residual: incomplete-matrix desktop **PENDING**; probes **C5–C9 SKIP**; desktop **D5/D6 PARTIAL**; Linux **cold-3y** known.

| Gate | Result |
|------|--------|
| Build | **PASS** (~19.8 MB / 19 835 424 B) |
| Go cold-3y | **FAIL** ~5.08 s want &lt;3 s (DEF-R2-01, non-blocking) |
| MissingFX / Return targeted | **PASS** |
| Phase A DEF-R2-02 harness | **PASS** — FIXED |
| Seed v3 four scenarios | **PASS** — 4/4 OVERALL (35/35 each) |
| Probe C1–C9 | **4 PASS / 0 FAIL / 5 SKIP** |
| Desktop D1–D8 (complete) | **6 PASS / 2 PARTIAL** (D5 waterfall reconcile; D6 History filters All) |
| Incomplete matrix desktop | **PENDING** (empty `screenshots/incomplete/`) |
| DEF-R2-02 desktop | **FIXED** — Trend All Partial 21/45, chart OK, no generic load error |

## Open / carry

1. **DEF-R2-01** — Linux cold-3y perf (known, non-blocking)
2. **R3-D5** — Drivers waterfall reconcile warning (PARTIAL)
3. **R3-D6** — History deep-link account/instrument filters remain All (PARTIAL)
4. Incomplete-matrix desktop (plan D7 multi-scenario) — **PENDING**
5. Probe modes C5–C8 + residual corrupt DB for C9 — **SKIP** / tooling gap

## Closed this round

- **DEF-R2-02** — FIXED (probe partial 21/45 no panic; desktop chart + Partial coverage)

## Exit criteria

- Met: four DBs; no unexpected probe FAIL; DEF-R2-02 fixed; D1–D8 RESULTS on complete; report written
- Partial: incomplete desktop matrix; quantitative C5/C8; docs not yet git commit/PR

See `REPORT.md` for full tables, screenshot index, and evidence paths.
