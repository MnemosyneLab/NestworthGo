# PR #195 return analysis regression checks

Patch baseline: `dfc9814f5c37ec2aece31df228c3e238739d04d9`.
Verification date: 2026-11-03. Only synthetic fixtures were used. No desktop
application or real household ledger was opened.

## Covered behavior

- Each partial ranking row has a visible, accessible completeness label and
  its localized reason. Complete rows keep their normal presentation.
- The exported `Service.Contribution` path retains flat groups and known zero
  summaries. Separate cases cover a complete zero, a partial known zero,
  incomplete evidence and absent snapshots. Unknown values stay unavailable.
- Sorting retains the same request's list and focused select while the backend
  responds. Changing dates, account scope, metric or grouping drops prior
  amounts. Backend sorting and missing-value rules are unchanged.
- Calendar DTOs carry the effective basis and known currency. Synthetic service
  tests exercise native USD in a selected November range and forced-base CNY in
  its wider October-to-December grid, including missing FX evidence. UI tests
  cover a selected native CNY summary and a forced-base USD grid independently.
- Grid and summary loading, errors and retries are independent. Tests retain
  successful data when the other request is pending or rejected and verify that
  retry targets only the failed query.

## Automated checks

Environment: Linux x86_64, Go 1.27.3, Node 24.22.0 and pnpm 12.14.0.
GTK4/WebKitGTK compilation and binding generation ran in the prepared isolated
Linux container. These are Linux checks, not native macOS acceptance.

- `go test ./...`: passed (ordinary non-race suite).
- `go vet ./...`: passed.
- `go build ./cmd/nestworth`: passed with the Linux native dependencies.
- Targeted Go contribution/calendar/application-to-wire regressions: passed.
- Frontend lint (including locale coverage) and typecheck: passed.
- Full Vitest suite: 163 files, 1702 tests passed (309.51 seconds).
- Frontend production build: passed (Vite build 1.40 seconds). The existing
  documented ECharts vendor chunk remains above the 500 kB warning threshold.
- `gofmt` on changed Go files and `git diff --check`: passed.
- `node --check docs/qa/pr195-return-analysis/browser-regression.mjs`: passed.

## Follow-up: yearly grid and day drawer

Independent review identified that an explicit selected-range summary could
succeed while a separate yearly request failed silently. The new regression
cases failed on `047004f` before the fix. The yearly grid now owns its loading,
error, retry and empty states when its range differs from the summary. Retry
requests only January 1 through the last closed day; the narrower successful
summary remains visible and keeps its values. Shared queries retain a single
loading/error presentation.

The yearly grid labels its own effective basis, currency and forced-base
fallback independently of the selected-range summary. Tests keep a native CNY
summary visible while delayed or retried annual data uses base USD.

Day-detail responses now carry the same effective `valuation` and `currency`
metadata as the grid. Application/service and wire tests cover native and
forced-base day responses for the same grid query, including missing FX. The
drawer displays its own metadata and fallback message. UI coverage includes
native CNY, forced USD and unavailable currency/amount evidence; it never
borrows the selected-period summary's basis.

The focused frontend run passed 35 tests across ReturnAnalysisPage and
ReturnCalendarTab; the focused Go calendar/day/application-to-wire run passed.
The full Go test/vet/native-build and frontend checks were rerun after the
follow-up. The focused UI tests and typecheck were rerun again after disabling
transport retries in the annual manual-retry fixture.
Only additive response metadata and UI state/presentation changed; no additional
automatic queries, financial calculations or database writes were introduced.

## Browser evidence

`browser-regression.mjs` mounts the real ContributionTab, query hooks and shared
controls with intercepted synthetic Wails services. It starts no backend. The
fixture contains 100 rows, including an unavailable amount, and delays the
ascending-sort response by 500 ms. `browser-result.json` records Chromium
154.0.8037.57 on Linux at a 1280 × 720 viewport:

| Observation | Before | Pending | Completed |
| --- | ---: | ---: | ---: |
| Scroll Y | 461 | 461 | 461 |
| Document height | 4628 | 4628 | 4628 |
| Original sort control and list retained | — | Yes | Yes |
| Sort control retains focus | — | Yes | Yes |

The returned ascending order retains the backend's unavailable-first semantics.
No uncaught browser errors occurred. Run after generating frontend bindings:

```sh
PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs \
CHROMIUM_PATH=/path/to/chromium \
node docs/qa/pr195-return-analysis/browser-regression.mjs
```

This is a browser layout/focus check, with Chromium's sandbox disabled inside
an isolated runner. It does not establish native macOS WebKit or screen-reader
acceptance. No performance benchmark or production latency claim is made. The
fix adds no provider requests, database writes or sorting calculations in the
frontend; zero-return periods now take the normal group fold.
