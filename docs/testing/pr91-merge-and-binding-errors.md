# PR91 merge and binding error investigation

Date: 2026-10-15. Scope: PR91 (`feat/holdings-grouped-redesign`) after merging
main `3f3d73f` (PR89 reconciliation) into PR head `1c600e1` with normal merge
commit `713452b`. A final pre-push fetch found main had advanced to `2de0fd6`
(PR92's standalone actions page); that update was also merged normally without
conflicts. All data used here is synthetic in disposable SQLite databases
and jsdom. No user Mac, private database, provider network data, release, or PR
merge is involved.

## Merge decisions

- `AccountDetail.tsx`: retain PR91's `useInstrumentHoldingGroups` and shared
  `HoldingGroupsList`, and main's `AccountReconciliation` import and mounted
  control. Do not restore the obsolete flat holdings imports/table.
- `en.json`, `zh-CN.json`, `zh-TW.json`: retain both PR91's `holdingGroups` /
  `holdingPerformance` namespaces and main's `reconciliation` namespace.
- Backend, schema, audited corrections, reconciliation navigation, bindings
  registration and documentation merged without content conflicts.
- Keep performance selection and calculations unchanged: eligible daily closes
  only, chart/headline from one read, fixed 7/30/90/180/365-day returns, and All
  as a chart period only. Market Data keeps its raw observation history.

## Reported flow

The reporter later localized the incident to viewing instruments and account
holdings, expanding individual instruments for historical price line charts and
seven-day annualized returns. They have not established whether the UI failed
visibly or only terminal errors appeared. This narrows the investigated flow;
it does not establish that every observed error was cancellation.

## Confirmed defect and correction

Before the correction, a cancelled context passed to each of these bound reads
reproduces the reported wire payload exactly:

```json
{"code":"internal","message":"an unexpected error occurred"}
```

The paths are `InstrumentHoldings`, `AccountHoldingDetails`,
`InstrumentHoldingGroups`, `AccountHoldingGroups`, `InstrumentQuoteSeries`,
`InstrumentQuotePerformance`, `InstrumentQuoteReturns`, and
`InstrumentQuotePerformanceSeries`. Real SQLite returns cancellation;
`apierror.Wrap` correctly sanitizes this non-domain error to `internal`.
The pre-fix regression run failed all eight cancellation cases and recorded
`error_type=*errors.errorString` with the respective service file/line.

These read adapters now follow the existing `HistoricalOverview` /
`HistoricalOverviewSeries` cancellation contract: when their own request context
is cancelled, complete with a zero result and no error. Wails has already
cancelled the abandoned JavaScript promise; returning a second Go error causes
unnecessary diagnostics and can produce a late beta.28 binding rejection. No
partial result is returned. Live errors still go through `apierror.Wrap`.
Mutation behavior and the global sanitizer are unchanged. The frontend's existing
identity-scoped late-rejection handling remains necessary for replies already in
transit before Go sees cancellation.

The generated-binding integration test uses the actual pinned beta.28 runtime,
React StrictMode, TanStack Query and the real query hooks with a synthetic
transport. StrictMode abandons its first three calls; changing account and period
cancels two more calls while retaining the instrument's returns query. Replacement
reads succeed. A live native `internal` failure remains an error and succeeds on
explicit retry. This establishes a feasible cancellation path; it does not
reproduce a native macOS IPC session.

The delegating thread also reports an independent Atlas native Linux dev-WebView
check at the original PR head `1c600e1`: Account and Portfolio initial expansion,
remount, instrument switching, rapid period changes and collapse rendered synthetic
charts and seven-day returns without reproducing a binding error. That check
was reported by Atlas, was not executed in this workspace, and does not verify
the final merged build or a macOS session.

## What remains unknown

The original PR91 testing errors have not been attributed to a particular caller
or underlying exception. The generic wire payload alone cannot prove cancellation:
SQL failures, closed databases and other unexpected errors are deliberately
sanitized to the same payload. `*errors.errorString` alone is also ambiguous.
Synthetic successful reads do not exclude private-data-specific or native-only
failures. No claim is made that cancellation caused every reported error or that
this change fixes unrelated live failures.

Application diagnostics and Wails stdout are separate outputs. `cmd/nestworth`
sets the default slog logger to the opt-in diagnostics writer. With diagnostics
enabled, `apierror.Wrap` records `application request failed`, `code`, `caller`
and (for unexpected failures) `error_type` in the JSON application log at
`<settings directory>/logs/nestworth.log`. Logging defaults to off; absence of a
record is not proof of success. Wails stdout may show only the sanitized binding
error. The sanitizer must not be relaxed to expose raw SQL, paths or financial
data. Any remaining failure needs its timestamp, affected action and matching
application diagnostics record before further attribution.

## Regression evidence

- `internal/wailsapi/quote/cancellation_test.go`: cancelled real SQLite reads,
  successful replacements, live validation failures and unsuppressed deadlines.
- `internal/wailsapi/analytics/holdings_cancellation_test.go`: household/account
  holdings and grouped-read cancellation, replacements, live validation and
  unsuppressed deadlines.
- `internal/wailsapi/quote/performance_integration_test.go`: public synthetic
  Agent writers and bound reads for stock, bank NAV, metal and crypto; exact
  five-window returns, actual dates/annualization, daily-only chart points,
  headline alignment, unchanged raw history and PR89 reconciliation reads.
- `frontend/src/queries/holdingsCancellation.test.tsx`: real generated bindings
  and runtime with StrictMode, superseded account/period reads, live failures and
  explicit retry.
- `frontend/src/features/investments/HoldingDetailPanel.test.tsx`: real shared
  holding list, lazy detail, chart and returns components in both Portfolio and
  Account scopes. Initial expansion, collapse while pending, switching
  instruments, switching to 1W, late abandoned errors, rendered chart data and
  seven-day annualization are exercised. Independent live chart/return failures
  stay visible and recover through their own retry controls.
- Existing `frontend/src/lib/wailsCancellation.test.ts` additionally covers
  actual late native-error identity, live errors and cancellation-delivery
  failures.

Native macOS desktop, packaging, accessibility and live-provider acceptance have
not been run. Linux builds and jsdom tests cannot establish those gates.

## Validation commands

The checks use Go 1.26.0, generated Wails beta.28 bindings and locked frontend
dependencies. Linux native headers/libraries, compiler caches and tool setup are
isolated under `/tmp`; generated bindings/bundles are ignored and uncommitted.

```sh
go test -p 1 ./...
go vet ./...
go build ./cmd/nestworth
gofmt -l cmd internal
git diff --check
go test -race ./internal/wailsapi/quote ./internal/wailsapi/analytics ./internal/wailsapi/apierror ./internal/domain ./internal/diagnostics
node frontend/scripts/ensure-bindings.mjs --check
python3 tools/check-nestworth-skill.py
python3 -m unittest discover -s tools/tests -v
# From frontend/:
node node_modules/typescript/bin/tsc --noEmit
node node_modules/eslint/bin/eslint.js .
node node_modules/vite/bin/vite.js build --mode production
node node_modules/vitest/vitest.mjs run --maxWorkers=1
```

The first overlapping checks exhausted two Go performance budgets and several
frontend deadlines while initial native compilation and multiple test processes
shared the runner. Repeat validation bounds package/worker concurrency without
changing budgets, assertions or application behavior. The overlapping frontend
run was stopped after the timing failures. The complete Go repeat passed,
including the same performance-budget tests. Before the final PR92 refresh, the frontend repeat passed all
1,316 tests across 129 files; the focused expanded-holding component run passed
all 19 tests. Targeted race checks, binding freshness, typecheck, lint, production
build, formatting/whitespace checks and all 53 tooling tests passed as well.

After merging PR92 (`2de0fd6`), the final serial frontend run passed 1,314 tests
and found one deterministic stale assertion in main's `src/app/navigation.test.ts`.
That test still required Upcoming actions to be absent from the sidebar, while
PR92 intentionally adds it. The assertion now requires the Activity ordering
`upcoming-actions`, `history`, `reconciliation`, preserving both PR92 and PR89.
All four navigation tests passed after this test-only correction; the other
128 files passed in the complete serial run. No product behavior was changed
for this correction. The refreshed typecheck, production build, affected-file
lint, Linux Go build and `go test ./cmd/nestworth` also passed.

Concurrent frontend repeats hit existing account/history/instrument interaction
deadlines and were stopped. Limits and production behavior were not changed;
all 61 account-page tests passed in isolation, and the subsequent serial full
run had no timing failures. CI must run independently on the pushed head.
