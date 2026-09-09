# Layer A / B + Probe Facts -- Nestworth Analytics QA Round 2

**Repo:** `/workspace/NestworthGo-fe-be-audit`
**HEAD:** `367c3b0` (`fix(analytics): resolve QA findings`)
**Full SHA:** `367c3b024eed9983bbd35f313508d21df4a287d2`
**Date:** 2026-09-09 (Asia/Singapore UTC+8)
**Host:** Linux amd64 agent box
**Artifact root:** `/workspace/nestworth-analytics-qa-r2/`

## Layer A -- static / unit

| Check | Result | Evidence |
|---|---|---|
| Frontend Vitest | **PASS** 44 files / **344** tests | `logs/10-vitest.log` |
| `go test ./... -count=1 -timeout 10m` | **PASS** except one package | `logs/11-go-test-key.log` |
| `internal/application` cold-3y | **FAIL** ~5.04s want <3s | same; prior `logs/02-perf-and-dietz.log` ~5.27s; Linux non-blocking |
| sqlite `./internal/infrastructure/sqlite/...` | **PASS** | `logs/03-sqlite.log` + full `./...` |

**Layer A summary:** Frontend PASS. Go PASS except known Linux cold-3y budget.

## Layer B -- engine / golden / probes

| Check | Result | Evidence |
|---|---|---|
| Seed complete | **26 PASS / 0 FAIL** | `seed/seed-results-complete.json`, `logs/04-seed-complete.log` |
| Seed missing-both | **26 PASS / 0 FAIL** | `seed/seed-results-missing-both.json`, `logs/05-seed-missing-both.log` |
| Fixture A-D in-memory | **4 PASS / 0 FAIL** | `seed/probe-fixtures.json`, `logs/06a-probe-fixtures.log` |
| FL-18/19 IncludeCash Dietz | **PASS** rated 38/38 both; salaryDietzCapitalDiffers=true; include A$562.8283 vs exclude A$424.9383 | `seed/probe-cash-include.json` |
| True-zero day 2026-08-31 | **PASS** amount=0 rate=0 status=ok (complete + missing-both) | `seed/probe-truezero-residual-*.json` |
| Clean residual (AssetChange) | **PASS** residualIssueCount=0, drivers=0 (both DBs) | same |
| Deliberate residual mode on clean DBs | expected pass=false (no corruption); complete issues=0; missing-both NULL base_amount on gap item | `seed/probe-residual-*.json`, `logs/12-residual-*.log` |
| Missing-both verify | **PASS** 45 snaps; incomplete 22 | `logs/07-missing-both-verify.log` |

## Confirm Aug 31 true-zero

Re-probed Analyze (2026-08-01..2026-09-07, household/base/includeCash/investment):
- complete: status=ok, amount=0 AUD, rate=0 -> **PASS**
- missing-both: status=ok, amount=0 AUD, rate=0 -> **PASS** (flat day outside gap 2026-08-17)

## Build

| Item | Result |
|---|---|
| wails3 task build | **OK** |
| Binary | `binaries/` / `artifacts/` |
| Log | `logs/01-build.log` |

## Pass/fail counts (this round)

| Check | PASS | FAIL/known |
|---|---|---|
| Vitest | 344 | 0 |
| Go ./... packages | all but 1 | 1 (cold-3y only) |
| Seed complete | 26 | 0 |
| Seed missing-both | 26 | 0 |
| Fixtures A-D | 4 | 0 |
| True-zero (both DBs) | 2 | 0 |
| Clean residual (both DBs) | 2 | 0 |

## Open (Layer A/B)

- Linux `cold-3y` still FAIL (~5s vs <3s) -- non-blocking versus M3 Pro gate.
- Deliberate residual corrupt-copy experiment (sec 17.4) not rebuilt in r2; clean residual confirmed instead.
- Desktop/GUI re-verify pending (REPORT.md section 4; no GUI started).

### Chinese short bullets
- Layer A: Vitest 344 all pass; Go only cold-3y fail (~5s / budget 3s), known Linux.
- Layer B: both seeds 26 PASS; Fixtures 4 PASS; IncludeCash OK.
- True-zero: 2026-08-31 amount=0 rate=0 (both DBs PASS).
- Clean residual: residualIssueCount=0 (both DBs PASS). Deliberate mode not injected; pass=false on clean DBs expected.

