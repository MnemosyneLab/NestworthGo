
## 2026-09-08 — Analytics QA Phase 1 + Layer A (`4cee772`)

### Build
- Linux production binary **PASS**: `artifacts/nestworth` (19 814 944 bytes, ELF x86-64 stripped).
- Setup/build used `PACKAGE_MANAGER=bun`. Initial apt stalled on interactive fuse.conf; fixed with `DEBIAN_FRONTEND=noninteractive` + `--force-confdef/old`.

### Layer A findings
1. **gofmt gate (check FAIL):** `cmd/align-smoke-check/main.go` listed by `gofmt -l`. Causes `wails3 task check` to exit before Go tests / FE lint/typecheck/test in that task.
2. **Analysis perf budget (application FAIL):** `TestAnalysisPerformanceBudgets/cold-3y` — cold 3Y/500-component took **5.196s** (budget **&lt; 3s**). Same package **PASS** under `go test -race` (~53s suite), so treat as **timing sensitivity / box load** unless reproducible cold.
3. **Frontend Node runtime:** vitest/jsdom fails on system Node **v20.19.2** (`webidl.util.markAsUncloneable is not a function`, 44 worker errors). **PASS** on Node **v22.19.0** (`/workspace/.local/node`): 44 files / 341 tests.
4. **CGO noise only:** Wails `linux_cgo.c` deprecated `gdk_x11_*` warnings during build; not treated as failure.

### Not done here
- No git branch/push.
- No full GUI Layer C / screenshots.

## 2026-09-08 — Layer B engine correctness / golden (`4cee772`)

### Result
- **Engine correctness / §7 fixtures A–D: PASS**
- **Package gate including perf: FAIL** only `TestAnalysisPerformanceBudgets/cold-3y` (~5.09–5.19s, want &lt; 3s) — already known from Layer A; `warm-memo` and `cold-month` PASS. Correctness-only run (`go test ./internal/application -skip TestAnalysisPerformanceBudgets -count=1`) **PASS** in ~3.1s.

### Packages / suites
| Suite | Result |
|-------|--------|
| Fixture-mapped focused (`Dietz` / FX / scope / golden valuation) | PASS |
| Full `internal/application` verbose | 216 PASS / 1 FAIL (perf) |
| `internal/wailsapi/analytics` | PASS (4 tests) |
| `internal/wailsapi/holding` | PASS |
| `internal/wailsapi/portfolio` (incl. Overview golden) | PASS |
| `internal/wailsapi/history` | PASS |

### Fixture mapping notes
- **A:** noon +90 000 contribution → Dietz denom 55 000, rate 1/55 ≈ 1.818%; return amount +1000; salary Income + cash-inclusion Dietz — covered by `analysis_return_test.go`.
- **B:** $100→$110 @ 7.0→7.2 → Price 70 / FX 22 (base), Price 10 / FX 0 (native) — `analysis_attribution_review_test.go` + `analysis_fixture_test.go`.
- **C:** USD cash salary path → Income 71 / FX 21 / Residual 0 — Case32 + FXPaths.
- **D:** instrument purchase External Flow +100; account scope keeps buy internal; in-kind transfer scope-relative — Cases 13/14 + Case35.

### Logs
- `logs/07-layer-b-engine.log`
- `logs/07-layer-b-application-verbose.log`

### Out of scope / ignored
- No GUI Layer C.
- No git branch/push.
- Untracked `cmd/align-smoke-*` gofmt not scored for Layer B product engine.

## P0 GUI hang (2026-09-08 18:39 SGT)

- First launch stuck on **Loading your workspace** (SM-01 PARTIAL).
- Root cause likely bad `settings.json` (wrong keys: locale/theme/marketDataProvider) → `could not load saved settings` + `could not persist window size`. Frontend gated on `Settings.Load()` / bootstrap after Startup available.
- Fix: copy schema-valid settings from align-smoke; seed DB via `align-smoke-seed` with `NESTWORTH_DATABASE_PATH`; relaunch with `GTK_A11Y=none`, `WEBKIT_DISABLE_DMABUF_RENDERER=1`, `WEBKIT_DISABLE_COMPOSITING_MODE=1`.
- Second launch: no settings WARN; settings.json rewritten by app (persist OK). Re-running SM-01–07.

## SM-11 Contribution incomplete coverage (2026-09-08 22:11 SGT)

- After correct StartHistory seed, Asset/Calendar sheets work; Contribution shows **0/38 incomplete return coverage** so no rows / no Contribution Sheet.
- Likely return-path needs denser complete instrument+FX quotes for Dietz days, or holdings valuation gaps on brokerage days.
- Not a nav/crash bug; document as P0 PARTIAL / investigate in Layer D financial E2E if time.


## SM-11 Contribution 0/38 — root cause + seed fix (2026-09-08 22:20 SGT)

### Root cause
- Contribution coverage uses `finalizeAnalysisReturns` RatedDays (complete component days with positive Dietz capital). UI shows `AvailabilityMarks` as Incomplete `rated/total` when `returnAvailability` is partial/unavailable.
- Historical USD brokerage cash snapshot items were incomplete (`missing account value or FX rate`) on 44/45 days: seed wrote **manual** FX quotes but never `SetFXPreference(USD,AUD,manual)`. Valuation’s `implicitFXPreference` defaults to **provider**, so manual quotes were ignored → `missing_count=1` every day after USD cash appeared → **0 rated return days** for 2026-08-01..09-07.
- Secondary: AAPL `holdings.created_at` / `instruments.created_at` stamped at seed “now”, so historical replay skipped the holding (not in origin) → no holding snapshot items (component_count stuck at 2).

### Seed fix (no product code)
- `SetFXPreference(ctx,"USD","AUD","manual")` **before** `StartHistory` (captured in `history_origin_fx_preferences`).
- SQL backdate `instruments.created_at` to origin and `holdings.created_at` to first buy before rebuild.
- Rebuilt + re-verified: snapshots **45/45 complete**, holding items present, Analyze coverage **37/38** rated, Contribution `available=true` with 2 rows (AAPL + cash). One partial day 2026-08-14 (second buy; Dietz capital not positive).

### Artifacts
- Seed: `cmd/analytics-qa-seed/main.go`
- Probe: `cmd/analytics-qa-probe` / `nestworth-analytics-qa/seed/probe_returns.go`

## Contribution 0/38 incomplete (2026-09-08 22:23 SGT) — RESOLVED in seed

- `finalizeAnalysisReturns` rates a day only if all investment-universe ComponentDays are CompletenessOK and Dietz capital > 0.
- Seed wrote manual FX quotes but no FX preference → valuation defaulted to provider → missing FX on USD brokerage cash (44/45 days).
- Fix in `cmd/analytics-qa-seed`: SetFXPreference manual before StartHistory; SQL backdate instruments/holdings created_at for historical holding replay.
- Probe: 37/38 rated; one partial day 2026-08-14 (second buy, Dietz capital not positive).

## FL-22 / FL-23 date clamp (2026-09-08 22:51 SGT) — FAIL

- Plan: To=today/future should clamp to last closed day; From before History Origin should clamp to origin.
- Observed on Linux UI: future To still selectable; pre-origin From remained unchanged (no auto-clamp visible).
- Severity: P0 per plan §9.4 — track for product fix or confirm if clamp is request-only (silent) without UI feedback.
## Unexplained difference on Portfolio Asset Changes (2026-09-08 23:30 SGT)

- Period 2026-08-01..09-07 Portfolio Base AUD Include cash: Drivers show **Unexplained difference +A$1,428.80** with reconciliation warning; partial coverage warning on summary.
- May be seed/history-origin backdate artifact or real residual — investigate before release sign-off; not a crash.
## FL-18/19 Include vs Exclude Cash Dietz (2026-09-09 00:42 SGT) — PASS (probe)

Engine-level: IncludeCash changes invested capital and period return amount on 2026-08-01..09-07; salary day 2026-08-05 has Dietz cash flow only when IncludeCash=true. See `seed/probe-cash-include.json`.
## CO-03 Contribution Dividend empty (2026-09-09 00:55 SGT) — FAIL

- Seed recorded cash dividend 2026-08-28 (+USD 12.50); Categories Dividend & Interest shows +A$18.83.
- Contribution tab Dividend & Interest type shows **no rows** for 2026-08-01..09-07 Portfolio Base Include cash.
- Likely UI/query filter vs category attribution mismatch — product investigation.
## CO-03 Contribution dividend empty (2026-09-09 07:40 SGT) — FIXED (product)

- Cause: `listActivitiesUntilQuery` only `attachActivityEffects`; `DividendDetail` nil → Contribution dividend_interest empty while Categories used cash AssetBuckets.
- Fix: call `hydrateActivities` instead (same as ListActivities).
- File: `internal/infrastructure/sqlite/activity_repository.go`
