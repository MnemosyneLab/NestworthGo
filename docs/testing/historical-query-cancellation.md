# Historical query cancellation checks

The [historical overview contract](../design/historical-overview.md) owns capture,
date and financial semantics. Use synthetic fixtures and the pinned toolchain
from the [local workflow](../development/local-workflow.md).

## Regression boundaries

- Changing dates/modes, clearing invalid input, leaving the page and unmounting
  abandon the obsolete request. Late success or rejection must not overwrite
  the final selection or a new page session.
- Cancellation must reach application/SQLite work, including a request waiting
  for a connection; no partial financial capture is returned. A replacement
  read must not wait for the full obsolete burst to drain.
- Cancellation delivery failure must not block replacement or accept stale data.
  Live errors keep their normal error/retry presentation.
- The frontend cancellation helper handles only its own abandoned promise's
  recognized late Wails cancellation rejection. Ordinary errors, lookalikes,
  unrelated promises and callback failures remain reportable. Rerun actual
  installed-runtime tests when upgrading Wails; an old beta's behavior is not
  a compatibility guarantee for a new runtime.
- Same-date mode changes, comparison-to-current, swap, repeated dates and
  navigation restoration must retain exact final totals and missingness.

Application historical overview/series and SQLite hydration tests cover backend
cancellation and capture consistency. Frontend runtime, query-hook and historical
overview tests cover request identity, late replies and error presentation.

## Optional performance investigation

The retained synthetic benchmarks compare obsolete-work drain and final-selection
latency separately. Run only for a relevant performance investigation, with
fixture scale, toolchain, host/load and actual path recorded:

```sh
CGO_ENABLED=0 go test -tags 'server perf' ./internal/application -run '^$' \
  -bench BenchmarkHistoricalOverviewRapidSwitch -benchtime=1x -count=1 -benchmem
NW_HISTORICAL_SWITCH_EVIDENCE=1 CGO_ENABLED=0 \
  go test -tags 'server perf' ./internal/application \
  -run TestHistoricalOverviewDenseRapidSwitchEvidence -count=1 -v
```

Small synthetic samples are not p95, RSS ceilings or native IPC measurements.
The original October timing tables and late-error reproduction logs remain in
Git history; their obsolete toolchain and elapsed times are not current targets.

## Native acceptance

On the isolated development identity, verify normal and rapid single/two-date/
current comparisons, invalid and pre-origin dates, leave/return while loading,
and a lightweight read immediately after cancellation. Check the final controls
match final values with no stale flash, spurious internal error or blocked queue.
Repeat keyboard, locale and theme paths and record final-selection versus drain
timings. Browser/jsdom evidence does not prove native IPC cancellation or focus.
