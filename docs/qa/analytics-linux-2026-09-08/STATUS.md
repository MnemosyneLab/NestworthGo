# NestworthGo Analytics QA — STATUS

- **When:** 2026-09-08 ~18:21 SGT (UTC+8) / 10:21 UTC
- **Repo:** `/workspace/NestworthGo-fe-be-audit`
- **Commit:** `4cee772` (`4cee77220fde42a60131282ef2f637c5750cb96b`) — main
- **Branch/push:** not created (per instructions; parent owns that)

## Phase 1 — Linux build/package

| Step | Result | Log |
|------|--------|-----|
| apt deps (webkitgtk-6.0 / gtk4 / pkg-config / file / dpkg-dev) | **PASS** (after noninteractive recovery from fuse.conf prompt) | `logs/00-apt-deps.log`, `logs/00-apt-deps-fix.log` |
| `PACKAGE_MANAGER=bun wails3 task setup` | **PASS** | `logs/01-setup.log` |
| `PACKAGE_MANAGER=bun wails3 task build` | **PASS** (CGO warnings only: deprecated gdk_x11_*) | `logs/02-build.log` |

### Binary

| Field | Value |
|-------|-------|
| Built path | `/workspace/NestworthGo-fe-be-audit/bin/nestworth` |
| Artifact copy | `/workspace/nestworth-analytics-qa/artifacts/nestworth` |
| Size | **19814944** bytes (~18.9 MiB) |
| `file` | ELF 64-bit LSB executable, x86-64, dynamically linked, stripped |
| SHA256 | `082a42f0e38d64115425659996bc5fcfbde15de2e49eaa4b9bbf077ef37efc96` |
| Metadata | `artifacts/nestworth.file.txt`, `nestworth.size.txt`, `nestworth.sha256.txt` |

**Build result: PASS**

## Phase 2 — Layer A (§4.1)

| Check | Result | Notes | Log |
|-------|--------|-------|-----|
| `wails3 task check` | **FAIL** | Stopped at gofmt gate: `cmd/align-smoke-check/main.go` not gofmt-clean. Frontend build dep ran OK before that. | `logs/03-check.log`, `logs/03-gofmt-diff.txt` |
| `go test ./internal/application/... ./internal/wailsapi/...` | **FAIL** (1 package) | `internal/wailsapi/*` all **PASS**. `internal/application` **FAIL**: `TestAnalysisPerformanceBudgets/cold-3y` took **5.196s**, want **&lt; 3s** (`analysis_projections_test.go:678`). | `logs/04-go-application-wailsapi.log` |
| Frontend `bun run test` (vitest) | **PASS** | **44** files / **341** tests. Requires **Node ≥22** (system Node 20.19.2 → jsdom `markAsUncloneable` worker crash; used `/workspace/.local/node` = v22.19.0). | `logs/05-frontend-vitest.log` |
| `go test -race ./internal/application/...` | **PASS** | ~53.5s; no race reports. Perf budget case passed under `-race` (timing flaky on box load). | `logs/06-go-race.log` |

### Layer A checklist

- [x] Build binary available
- [ ] `task check` green — **FAIL** (gofmt)
- [ ] Focused Go application/wailsapi — **FAIL** (perf budget only)
- [x] Frontend vitest — **PASS** (Node 22)
- [x] Optional race — **PASS**


## Phase 2 — Layer B (§3 Engine correctness / §7 fixtures)

| Check | Result | Notes | Log |
|-------|--------|-------|-----|
| Fixture-mapped golden/Dietz/FX/scope tests (`-v -count=1`) | **PASS** | All §7 A–D mapped cases green | `logs/07-layer-b-engine.log` |
| Analysis/engine filter suite (`TestAnalysis\|TestAsset*\|TestReview\|…`) | **FAIL** (perf only) | Same sole failure: `TestAnalysisPerformanceBudgets/cold-3y` ~5.1–5.2s want &lt;3s | `logs/07-layer-b-engine.log` |
| Full `go test ./internal/application/ -count=1 -v` | **FAIL** (1 test) | **216** PASS lines; **1** FAIL (`TestAnalysisPerformanceBudgets`) | `logs/07-layer-b-application-verbose.log`, `logs/07-layer-b-engine.log` |
| Correctness-only (`-skip TestAnalysisPerformanceBudgets`) | **PASS** (~3.1s) | Confirms engine math green without perf gate | `logs/07-layer-b-engine.log` |
| Perf subtests alone | **PARTIAL** | `warm-memo` **PASS**, `cold-month` **PASS**, `cold-3y` **FAIL** (~5.19s) | `logs/07-layer-b-engine.log` |
| `go test ./internal/wailsapi/{analytics,holding,portfolio,history}/` | **PASS** | analytics 4, holding 5, portfolio 8, history 19 | `logs/07-layer-b-engine.log` |

### §7 Fixture mapping (engine unit tests)

| Fixture | Expectation (plan) | Covering tests | Result |
|---------|--------------------|----------------|--------|
| **A — Modified Dietz** | Return +1000; denom 55 000; daily ≈1.81818% (not 10%); salary→Income + Dietz capital when Include Cash | `TestAnalysisReturnModifiedDietzAndGeometricLinking`, `TestAnalysisReturnCase19NoonContributionUsesWeightedDietzCapital`, `TestAnalysisReturnCase20SalaryIsIncomeAndCashInclusionControlsDietz`, `TestAnalysisReturnCases19And20UseWeightedCapitalOnlyWhenCashIncluded` | **PASS** |
| **B — Foreign holding Price/FX** | Base: Price +70, FX +22; Native: Price +10, FX 0 | `TestAnalysisReviewCases09And26ForeignHoldingFX`, `TestAnalysisReviewCases09And26And32FXPaths` | **PASS** |
| **C — Foreign cash + income** | Income +71, Cash FX +21, Residual 0 (not FX +92) | `TestAnalysisReviewCase32TrackingBalanceCashFormula`, `TestAnalysisReviewCases09And26And32FXPaths` | **PASS** |
| **D — Scope semantics** | Instrument buy = External Flow; account buy stays internal; in-kind transfer scope-relative | `TestAnalysisReviewCases13And14ScopeSemantics`, `TestAnalysisReviewCase35InKindTransferHasNoPriceAndIsScopeRelative` | **PASS** |

### Layer B checklist

- [x] Modified Dietz / geometric link golden
- [x] Foreign holding Price / FX attribution
- [x] Foreign cash Income + Cash FX formula
- [x] Scope semantics (account vs instrument / transfer)
- [x] Broader analysis attribution / return / projections / review / fixture suites (correctness)
- [x] Wails analytics-related API packages
- [ ] Perf budget cold-3y — **FAIL** (known; not scored as engine math failure)

**Layer B engine correctness: PASS** (fixtures A–D + correctness suite).  
**Layer B package gate including perf: FAIL** solely on `cold-3y` (already noted in Layer A).

## Blockers / notes for parent

1. **No hard blocker for GUI Layer C** from build: Linux binary built successfully.
2. Layer A not fully green: gofmt offender + flaky/perf cold-3y budget overshoot (~5.2s vs 3s) under default `go test` (passed with `-race`).
3. Layer B engine correctness / §7 fixtures A–D **PASS**; package-level `go test ./internal/application` still **FAIL** only on cold-3y (ignore untracked `cmd/align-smoke-*` gofmt for Layer B scoring).
4. Frontend vitest must run with Node 22+ on this box; document PATH for GUI follow-up if FE checks re-run.
5. Do **not** start full GUI Layer C from this agent — parent owns desktop GUI + screenshots.

## Next (parent)

- Desktop GUI Layer C + screenshots under `screenshots/` (do not start from this agent)
- Git branch/push after full QA
- Optional: revisit cold-3y budget or box load before treating perf as release blocker

## Layer B — Engine correctness / golden (2026-09-08 18:25 SGT)

- Commit: `4cee772`
- Engine correctness / golden: **PASS** (fixtures A–D + correctness suite)
- Package gate with perf: **FAIL** only `TestAnalysisPerformanceBudgets/cold-3y` (~5.09–5.19s, want <3s) — known from Layer A
- Correctness-only skip budgets: **PASS** (~3.1s)
- Full verbose application: **216 PASS / 1 FAIL** (perf)
- `wailsapi/{analytics,holding,portfolio,history}`: all **PASS**
- Logs: `logs/07-layer-b-engine.log`, `logs/07-layer-b-application-verbose.log`
- Next: Layer C P0 desktop SM-01–15 in progress

## Layer C — P0 SM-01–07 (2026-09-08 18:46 SGT)

- **PASS** all SM-01..07 after settings/WebKit relaunch
- Screenshots: `screenshots/p0/sm01-launch.png` … `sm07-*.png`
- Next: SM-08–15

## Layer C — P0 SM-08–15 (2026-09-08 18:56 SGT)

| Case | Result | Notes |
|------|--------|-------|
| SM-08 | PASS | Date/scope refresh OK |
| SM-09 | PARTIAL | No clickable calendar days (thin seed) |
| SM-10 | PARTIAL | No Change Driver rows |
| SM-11 | PARTIAL | No Contribution rows |
| SM-12 | PARTIAL | No Category rows |
| SM-13 | PASS | History nav OK |
| SM-14 | PARTIAL | No day item; filters preserved |
| SM-15 | PARTIAL | No driver; filters preserved |

Screenshots: `screenshots/p0/sm08-*.png` … `sm15-*.png`
Next: enrich seed for Insights rows, retest SM-09–12/14–15

## Seed fix (2026-09-08 21:51 SGT)

- Rewrote `cmd/analytics-qa-seed`: empty-TZ onboard → accounts → StartHistory → backdate → activities → rebuild
- Verify: origin_components=1, usable snapshots=45, snapshot_items=89, sample assets=17800 AUD
- settings currency=AUD
- Next: retest SM-09–12 / 14–15 sheets

## Layer C — P0 sheet retest after seed fix (2026-09-08 22:11 SGT)

| Case | Result | Notes |
|------|--------|-------|
| SM-01–07 | PASS | earlier |
| SM-08 | PASS | earlier |
| SM-09 | PASS | Day Sheet 2026-08-01 |
| SM-10 | PASS | Income driver +A$3000 |
| SM-11 | PARTIAL | Contribution 0/38 incomplete; no rows |
| SM-12 | PASS | AUD Cash category -A$200 |
| SM-13 | PASS | earlier |
| SM-14 | PASS | Calendar→Asset filters preserved |
| SM-15 | PASS | Driver→Return filters preserved |

Next: Shared Filter Bar (FL-*) + remaining Layer C–F P0 cases; note SM-11 coverage gap

## Contribution coverage fix (2026-09-08 22:23 SGT)

- Root: manual FX ignored (implicit provider preference) → brokerage cash incomplete → 0/38 rated return days
- Seed: `SetFXPreference(USD,AUD,manual)` before StartHistory; backdate instrument/holding created_at; rebuild
- Verify: 45/45 complete, max components 3, Analyze Contribution available, **37/38** rated, 2 rows
- Nestworth relaunched PID 83578; FL P0 + SM-11 retest in progress

## In progress (2026-09-08 22:38 SGT)

- Shared Filter FL P0: screenshots through FL-19; finishing FL-21–23 + SM-11 desktop retest
- Next batch queued: Return Calendar RC-01..11 P0 deep (`screenshots/calendar/`)
- Report skeleton: `report/REPORT.md`

## Shared Filter FL P0 + SM-11 (2026-09-08 22:51 SGT)

| Case | Result | Notes |
|------|--------|-------|
| FL-01–05 | PASS | Scope selectors / required empties |
| FL-11 | PASS | Base AUD |
| FL-12 | PARTIAL | Native single-currency unclear |
| FL-13–14 | PASS | Forced-base + Native session |
| FL-16–17 | PASS | Include/Exclude cash toggles |
| FL-18–19 | PARTIAL | Salary Dietz effect not fully verified in UI |
| FL-21 | PASS | From>To blocked (earlier To disabled) |
| FL-22 | **FAIL** | Future To remained selectable (no clamp UX) |
| FL-23 | **FAIL** | Pre-origin From unchanged (no clamp UX) |
| SM-11 | **PASS** | Contribution rows + detail sheet |

Screenshots: `screenshots/filters/`, `screenshots/p0/sm11-contribution-sheet.png`
Next: Return Calendar RC P0 deep

## Return Calendar RC P0 deep (2026-09-08 23:05 SGT)

| ID | Result | Notes |
|----|--------|-------|
| RC-01–05 | PASS | default/nav/today/future/positive |
| RC-06 | PARTIAL | no negative day in seed |
| RC-07 | PARTIAL | no true-zero day |
| RC-08 | PASS | Aug 14 partial zero `—◇` |
| RC-10/11 | PASS | Day Sheet detail |
| RC-14 | PASS | Day→Asset exact day range |
| RC-20–25 | PASS | Year view + month drill |

Screenshots: `screenshots/calendar/`
Next: Return Trend (RT) + Contribution (CO) P0

## Return Trend + Contribution P0 (2026-09-08 23:15 SGT)

| ID | Result | Notes |
|----|--------|-------|
| RT-01–03 | PASS | Cumulative / Linked % / Period amount |
| RT-SRC | PASS | Price + FX listed |
| CO-01 | PASS | Total Return + % |
| CO-02 | PARTIAL | no Realized Gain in seed |
| CO-03 | PARTIAL | no Dividend & Interest in seed |
| CO-SHEET | PASS | detail sheet |

Screenshots: `screenshots/trend/`, `screenshots/contribution/`
Next: Asset Changes Drivers / Trend / Categories P0
## Asset Changes Drivers/Trend/Categories P0 (2026-09-08 23:30 SGT)

| ID | Result | Notes |
|----|--------|-------|
| DR-SUM/WF | PARTIAL | loaded + coverage/recon warning |
| DR-SHEET | PASS | Income +A$3000 |
| DR-RES | PASS | Unexplained +A$1428.80 sheet |
| AT-01–03 | PASS | chart / Day / Total Assets |
| CAT-01/02/04/07/10 | PASS | spending/income/invest/sheet/history |
| CAT-03/05 | PARTIAL | Fees / Div empty in seed |

Screenshots: `screenshots/drivers|asset-trend|categories/`
Next: Trust / i18n / visual / stale (Layer D–F)
## Trust / i18n / visual Layer D–F (2026-09-08 23:52 SGT)

| Check | Result |
|-------|--------|
| TR-01–04 | PASS |
| NAV History | PASS |
| ZH-CN / ZH-TW | PARTIAL — some warning strings remain English |
| EN | PASS |
| V-1280 | PASS |
| V-1440 | PARTIAL — resize unavailable (stayed 1280×800) |

Next: finalize REPORT → branch + push
