# Historical Panorama query cancellation

This change starts from main `5e8824a96b992f76ffc048541c5c99851ee71d53`.
Single-date and comparison observers previously discarded stale replies without
cancelling their native work. SQLite has one connection, so obsolete captures
could keep financial reads queued after the final selection.

## Contract and regression coverage

The query signal now cancels the original generated Wails promise before error
normalization removes its cancellation handle. Disabling a retained observer
also explicitly cancels its exact query. This covers mode changes and inactive
pages, which do not necessarily remove the observer. Sessions remain separate;
a settled capture remains static on History Back. Returning to an unfinished,
cancelled capture reads the same selection again.

There is no new database pool, request queue, result cache, financial formula,
snapshot write, input truncation or single-request deadline. Both comparison
sides still use one read-only transaction and the public `ListActivitiesUntil`
continues to own its transaction. Hydration still uses 256-activity chunks and
one subsequent economic replay. Existing date, scope, coverage and series-budget
semantics remain in force. Additional cancellation checks prevent partial
captures during replay/valuation/projection or after batch loading. The
application returns `context.Canceled`. The abandoned IPC call completes with an
empty result and no error: Wails has already rejected its cancelled JavaScript
promise with `CancelError`, and beta.28 would report a second native rejection
as an unhandled `CancelledRejectionError`. An already returned/in-transit native
error requires the additional scoped frontend handling described below. Live
errors retain their normal wire codes and retry UI. Expected cancellation is
not logged as an internal error.

New tests exercise ten repeated dates in single/date/current-comparison modes;
mode switches with unchanged date keys; invalid/inactive selection; unmount and
new page sessions; abort during native promise creation; synchronous/asynchronous
cancellation-delivery failure; late successes and rejections; successful final
exact values; live error/retry; cancellation after reading and before current
valuation; cancellation while waiting for a separately owned connection; and
an actual busy SQLite connection released to a SQL probe. An installed Wails
`CancellablePromise` is used in the frontend helper test. Page tests also check
account/category scope changes against the successful final frozen capture.
These scopes are presentation filters, not backend account query parameters.

Existing dense-entry, corrected hydration, tied-ordering and cross-chunk
transaction-consistency tests remain part of the relevant/full suites.

## Reproducible dense synthetic benchmark

Measured 2026-10-13 on Linux amd64, AMD EPYC 9V74, Go 1.26.0,
`GOMAXPROCS=5`, server build tag and CGO disabled. One iteration per arm;
fixture setup is excluded. The fixture has 100 synthetic brokerage accounts,
900 holdings, 100 cash leaves, 9,000 quote backfills, and 25,000
activity/effect/value triples (86,433 retained input rows). Every successful
result must have all 1,100 rows and exact assets `0.108` CNY; comparison delta
must be exactly `0`. No personal ledger or live provider data is used.

The same application and SQLite loader run in both arms. The baseline reproduces
the old UI's missing cancellation. The fixed arm cancels the previous context
and starts the replacement without waiting. Requests start 20 ms apart; the
benchmark waits for every request to finish, including obsolete ones. It records
final-selection latency separately because SQLite's pool does not guarantee
that the last selection is served last. Raw output is retained in
[historical-query-cancellation-benchmark.txt](historical-query-cancellation-benchmark.txt).

| Mode | Requests | Baseline drain | Cancelled drain | Baseline final selection | Cancelled final selection | Baseline/fixed peak pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Single | 6 | 26.643 s | 4.822 s | 26.540 s | 4.717 s | 6 / 2 |
| Single | 10 | 45.491 s | 5.293 s | 17.771 s | 5.110 s | 10 / 2 |
| Two dates | 6 | 28.513 s | 5.579 s | 14.992 s | 5.476 s | 6 / 2 |
| Two dates | 10 | 46.178 s | 5.489 s | 9.614 s | 5.305 s | 10 / 2 |

All five/nine obsolete calls returned `context.Canceled` and no partial capture.
Maximum measured cancellation-to-application-exit was 0.071/0.273/0.104/0.879 ms
in the four fixed arms. Peak database occupancy stayed at one connection;
sampled busy time fell from 26.259–45.290 s to 4.373–4.879 s. Cumulative pool
wait duration fell from 66.824–200.712 s to 0.426–1.514 ms. Wait duration sums
concurrent waiters and is not wall time. The separate 8,300-activity release
test observed cancellation-to-SQL-probe-and-application-exit at
0.048–0.130 ms, ending with zero connections in use.

```bash
GOFLAGS='-mod=readonly -tags=server' CGO_ENABLED=0 GOMAXPROCS=5 \
  go test ./internal/application -run '^$' \
  -bench BenchmarkHistoricalOverviewRapidSwitch -benchtime=1x -count=1 -benchmem

# Shorter dense fixed-arm evidence, including historical/current comparison:
NW_HISTORICAL_SWITCH_EVIDENCE=1 GOFLAGS='-mod=readonly -tags=server' \
  CGO_ENABLED=0 GOMAXPROCS=5 go test ./internal/application \
  -run TestHistoricalOverviewDenseRapidSwitchEvidence -count=1 -v

GOFLAGS='-mod=readonly -tags=server' go test -race -p 1 -parallel 4 \
  ./internal/application ./internal/infrastructure/sqlite ./internal/wailsapi/analytics \
  -run 'HistoricalOverview|HistoricalSeries|HistoricalActivityHydration|StandaloneActivitiesUntil|HistoricalCaptureCancellation' \
  -count=1
```

These are host observations, not a latency or concurrency guarantee. Sampling
can miss short connection transitions. The benchmark calls Go contexts directly;
it does not measure desktop IPC delivery or browser transport scheduling. Wails
cancellation delivery failure is handled without blocking replacements or
accepting stale results, but work cannot stop through an abort that never reaches
the native request. Normal dense final captures still take roughly five seconds;
this fixes accumulated obsolete work, not the cost of one complete capture.

The opt-in evidence command also passed against the final cancellation guards,
while the broader race tests were running. Ten-request single/date/current
bursts drained in 6.992/6.819/7.226 s (final selection 6.804/6.629/7.039 s),
with all nine obsolete requests cancelled, at most two pending calls and one
database connection in use. Maximum cancellation-to-exit was
1.225/3.670/1.514 ms under that load. These additional observations demonstrate
the final-code/current-comparison path and host-load sensitivity; they do not
replace the paired measurements above.

## Validation record

The final PR records the exact commit. Final stable-tree checks:

| Check | Result |
| --- | --- |
| Server-mode `go test -p 1 ./...` | Passed; 37 tested packages |
| Supported race packages | Passed; every application test and all other internal packages; repository root and both analytics QA commands |
| Frontend full suite | Passed after late-error follow-up; 102 files, 1,011 tests |
| Final targeted frontend suite (runtime/hooks/page/App navigation) | Passed after late-error follow-up; 106 tests |
| Lint, typecheck, generate/check bindings, production frontend build | Passed; existing bundle-size warning |
| Server `go vet` and `go build ./cmd/nestworth` | Passed |
| Skill validator and Python tool tests | Passed; 51 Python tests |
| gofmt and `git diff --check` | Passed |
| Paired dense benchmark and final opt-in evidence | Passed; metrics above |
| Native desktop build / Mac GUI acceptance | Pending parent verification |

The application race selector listed 551 runnable names. Part 0/2 passed all
253 selected tests. Part 1/2 reached the cumulative ten-minute timeout while
starting existing `TestFixtureCRenewalFaultInjectionRollsBack`, before its
parallel tests had run; no assertion failure or race was reported. That entire
subset was rerun as parts 1/4 (146 names, 249.920 s) and 3/4 (152 names,
339.489 s), both passing. Their union plus part 0/2 covers all 551 names,
including the new cancellation tests. The completed part 0/2 took 308.865 s.
Other internal race packages all passed, including SQLite (73.716 s),
continuous backup (275.252 s), MCP (299.085 s), and the analytics adapter.

Native Linux GUI
compilation is unavailable on this host because GTK4/WebKitGTK development
packages are missing. Supported server builds and tests use `-tags=server`;
frontend binding generation uses `CGO_ENABLED=0`. Mac packaging and GUI
acceptance are pending independent parent verification.

## Late native-error ordering regression

The first implementation at `87840379c31d2bd2b99e11364be558b370c45c5a`
quieted a Go completion when its request context was already cancelled. That
cannot cover a live native error already returned or in transit when the user
abandons the frontend selection. The installed beta.28 runtime forwards this
transport rejection into the cancelled original promise and creates a separate
orphan `Promise.reject(new CancelledRejectionError(...))`. Catching the original
promise does not catch that orphan.

This was reproduced before choosing the fix: the generated binding and real
installed runtime used `setTransport` with separate resolve/reject controls;
after abort and after awaiting the helper's rejection, the original Call
transport rejected with `Call.RuntimeError`. Vitest reported one unhandled
`CancelledRejectionError` despite both ordinary test assertions passing.
[Before-fix reproduction output](historical-query-cancellation-late-error-reproduction.txt)
retains the failure. This demonstrates rejection/log noise; it does not
demonstrate stale data or a crash.

The follow-up handles only this duplicate report. Before cancelling, the helper
records the exact abandoned original promise in a `WeakSet`. One lazily installed
browser `unhandledrejection` listener calls `preventDefault()` only when the
reason is an actual SDK `CancelledRejectionError`, its original promise is in
that set, its cause is an actual SDK `Call.RuntimeError`, and its info prefix
identifies beta.28's late promise rejection. The marker survives helper cleanup
without a strong promise reference and is deleted when the duplicate is handled.
There is no global unhandled-rejection setting or Node rejection handler in
production. This matcher depends on the pinned SDK's public wrapper shape and
specific late-rejection prefix; a runtime upgrade must rerun these tests.

The real-runtime test exercises both late success and late native rejection,
checks the orphan's original promise identity, verifies a live native error
retains wire code `unavailable`, and leaves another actual runtime call's
unhandled rejection reportable after cancellation outside the historical helper.
Ordinary errors, lookalikes, programming errors and cancellation-callback
exceptions/rejections also remain reportable, including those referencing the
same abandoned promise.

jsdom runs its promises on Node's host, which does not dispatch browser
`unhandledrejection` events. The regression test therefore observes the actual
orphan created by the unchanged installed SDK's `Promise.reject`, attaches a
test-only rejection observer, and delivers its real reason/identity through the
browser event contract. Explicit `defaultPrevented` assertions distinguish the
handled historical orphan from the reportable unrelated/callback errors. The
original reproduction ran without this host shim and failed as recorded above.

Follow-up aggregate checks passed: the full 1,011-test frontend suite, 106 affected
tests, lint, typecheck, production frontend build, generated binding verification,
all 37 server-mode Go test packages, vet/server build, skill validation and all
51 Python tests. Go code, database queries and financial/cancellation contracts
are unchanged by this follow-up; the prior supported race and dense benchmark
evidence applies to that identical Go tree. Native browser/Mac GUI verification
remains pending.

## Mac GUI acceptance

Use the exact draft-PR head on a dedicated checkout and an isolated synthetic
25,000-activity profile equivalent to the benchmark (or the prior PR #57 dense
acceptance fixture). Record head, fixture, macOS/architecture, locale/theme,
window width and timings. Never use real household data.

1. Generate bindings and build/run with the pinned Wails beta.28 toolchain.
   Confirm a normal single-date and two-date capture gives the expected complete
   exact totals, account rows, quote evidence and B-minus-A values.
2. Change A six times, then ten times, while loading. Repeat single-date,
   historical comparison and comparison-to-current. Alternate repeated dates as
   well as forward dates. Final date/details must match the final controls;
   loading must resolve in approximately one capture, with no stale-result flash
   or internal-error toast. Record final-selection and all-work-drained timings.
3. During loading switch single/comparison modes, change B, swap dates, enable
   trend, select a new range and open a day. Verify the obsolete mode stops and
   the successful final capture has the correct A/B. Retain the 31-day series
   limit, gaps, coverage and its explicit budget-error/retry behavior.
4. Clear/enter an invalid date during loading; no result should appear. Restore
   a valid date and confirm success. Test a pre-origin/future date and a skipped
   civil date with the existing synthetic timezone fixtures.
5. Leave for Overview, another account or History during loading, then return.
   Verify no old reply populates the new session. A completed History Back
   capture remains fixed; an interrupted capture resumes the same selection
   with a fresh read. Account/category filters and details use final capture
   values without recapturing the household.
6. Immediately after cancellation, open a lightweight account/current read.
   Verify it is not blocked behind the whole obsolete burst. Inspect process
   CPU/SQL activity after exit; obsolete computation should stop promptly. Check
   that expected cancellation produces no application `internal` error log.
7. Repeat offline, with keyboard controls, light/dark and en/zh-CN/zh-TW.
   Check loading responsiveness, detail Escape/focus, and History Back restoration.

No merge, release, provider outreach or private-data acceptance is included.
