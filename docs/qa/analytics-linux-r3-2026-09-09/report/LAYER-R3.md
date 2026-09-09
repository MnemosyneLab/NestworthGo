# Layer A / B / C notes — Nestworth Insights QA Round 3

**Repo:** `/workspace/NestworthGo-fe-be-audit`  
**Branch:** `qa/analytics-linux-r3-2026-09-09`  
**HEAD under test:** `91d10c0` (`feat(qa): analytics-linux-qa-v3 mini-family seed (#14)`)  
**Full SHA:** `91d10c0216d31d274857e9bbf71d971dd6b8c7bd`  
**DEF-R2-02 lineage:** soft-FX path present since `ea94f48` / product classifier fix  
**Date:** 2026-09-09 (Asia/Singapore UTC+8)  
**Host:** Linux amd64 agent box  
**Artifact root:** `/workspace/nestworth-analytics-qa-r3/`

Also see: `logs/A-phase-summary.md`, `seed/B-SUMMARY.md`, `probes/C-phase.md`.

## Layer A — static / unit

| Check | Result | Evidence |
|-------|--------|----------|
| Build (`wails3` / native) | **PASS** `BUILD_EXIT:0` | `logs/02-build.log` |
| Binary | **19 835 424** B ELF stripped | `binaries/nestworth` |
| `TestAnalysisPerformanceBudgets/cold-3y` | **FAIL** ~5.08 s want &lt;3 s | `logs/01-go-analysis.log` |
| Missing FX / Return targeted suites | **PASS** | `logs/01b-*.log`, `logs/01c-*.log`, `logs/01d-targeted.log` |
| `TestAnalysisReturnMissingFXOnAssociatedCashDoesNotFailPeriod` | **PASS** | `logs/01d-targeted.log` |

**Layer A summary:** Build PASS. Go PASS except known Linux cold-3y (DEF-R2-01). Soft-FX / MissingFX path PASS.

## Layer B — seed v3

| Scenario | Result | Evidence |
|----------|--------|----------|
| complete | **35 PASS / 0 FAIL** | `seed/seed-results-complete.json` |
| missing-price | **35 PASS / 0 FAIL** | `seed/seed-results-missing-price.json` |
| missing-fx | **35 PASS / 0 FAIL** | `seed/seed-results-missing-fx.json` |
| missing-both | **35 PASS / 0 FAIL** | `seed/seed-results-missing-both.json` |

Fixture: AUD Cash + US/SG Brokerage; AAPL/QQQ/ES3; 45 snaps `2026-07-26..2026-09-08`; gap **2026-08-17**; true-zero **2026-08-31**. Narrative: `seed/B-SUMMARY.md`.

## Layer C — probes

| ID | Status | Note |
|----|--------|------|
| C1 Fixtures A–D | **PASS** | 4/4 |
| C2 IncludeCash | **PASS** | salary capital include≠exclude |
| C3 True-zero | **PASS** | 2026-08-31 rate=0 status=ok |
| C4 missing-both All | **PASS** | partial 21/45, no panic |
| C5–C9 | **PASS** | DB-backed quantitative + clean/corrupt residual controls; compact evidence in `probes/REMEDIATION-2026-09-09.json` |

Rollup: `probes/SUMMARY.json` — **9 PASS / 0 FAIL / 0 SKIP** for automated probes.

## Phase A trend presets (tie-in)

See `probes/A-trend-presets.json`. missing-both All/Custom/~30D all **partial** no panic; complete Custom/~30D **ok**, All **partial** (origin UNAVAIL expected).

## Desktop pointer

- DEF-R2-02: `screenshots/def-r2-02/RESULTS.json` → **FIXED**
- D1–D8: `screenshots/desktop-d/RESULTS.json` → 6 PASS / 2 PARTIAL
- Incomplete matrix: missing-both evidence exists; missing-price/missing-fx/zh-CN native replay → **PENDING**

### Chinese short bullets

- Layer A：构建 PASS；Go 仅 cold-3y 失败（~5s / 预算 3s）；MissingFX 相关 PASS。  
- Layer B：seed v3 四场景各 35 PASS。  
- Layer C：C1–C9 automated PASS；native missing-price/missing-fx/zh-CN replay仍待执行。
- DEF-R2-02：探针 + 桌面 FIXED。  
