# Historical panorama P2a acceptance and performance

Owner: Historical panorama engineering and QA. Scope: bounded civil-day trend.
Base: main `c84910971e3bf30c110e1e88f74686679172e3a0` after P1 PR #55.
All fixtures are synthetic. Native screenshots/GUI acceptance remain pending;
component tests and builds do not establish visual acceptance.

## Contract and regression matrix

| Area | Automated evidence | Independent/native check |
| --- | --- | --- |
| Exact atomic endpoints | `historical_series_test.go`: P1 deep endpoint equality with cash/FX and tiny holding; one bounded loader, concurrent writer changes subsequent capture only | Cross-check A/B cards, categories, endpoint details and first/last series values |
| Read only | SQLite `query_only`, unchanged `total_changes`, forbidden legacy/current-state read probes | Offline profile, no snapshot/provider/network writes |
| Input budget | Fact lookahead, byte/leaf rejection before batch hydration; default limits and SQL chunks exercised by dense benchmark | Explicit budget message, no hidden partial chart |
| Cancellation | Dense SQL-stage cancel scheduled at 10 ms; pre-cancel; replay cancellation checks; obsolete Wails request cancelled and late result suppressed | Rapid range/date/refresh then close page |
| Civil dates | 1/31/32, reverse/current/invalid/before-origin rejection; New York DST; Apia skipped interior/endpoint | Household date labels, single point wording, closed endpoint controls |
| Retained evidence | Late market-close/coverage equivalence with P1; missing-price/FX, tiny decimals, quote identity, backfill and corrected economic-time tests | No current quote repair or universal close/as-known claim |
| Coverage/connectivity | Negative net worth, zero, all missing subtotal null, missing debt leaves asset connection intact, archive scope change, segmented geometry | Missing/basis/scope gaps are honest; known subtotal never presented as total |
| UI capture/navigation | Whole-bundle enable/Set-A/open-day, failure/retry, local metrics/table/tab switches; UTC/Shanghai month-boundary History Back with series metric/selection | Keyboard chart equivalents, detail Escape/focus, original scroll restored |
| Locale/theme/shared UI | Trend three locales plus existing historical light/dark, shared charts and Sheet suites | Light/dark, en/zh-CN/zh-TW; wide/390/320px and 200% zoom |

## Agent closure selection regression (Q05)

The synthetic query-only reproduction failed on base
`788faf1eedf3c7e41e5eb214555da800d57829cc`: one US-equity unit, CNY 100,
history starting August 1, retained Agent closes on August 3 and August 5,
capture on August 6. August 4's ordinary panorama was complete at CNY 100.
The one-day series instead returned null assets/net worth, known assets 100,
and `history_coverage`. Selecting the quote from the full fact index inferred
the bounded closure, but resolving that selected quote again after account
projection lost the later close used as closure evidence.

Series valuation now carries each day's immutable instrument selection,
including its coverage decision and an explicitly absent quote, through both
point aggregation and endpoint rows. The carrier is private to application
reads; the resolver policy, exported DTOs, input budgets and cancellation
contract remain unchanged. A fresh capture resolves again from its retained
facts rather than reusing a previous capture's decisions.

`historical_series_selection_test.go` checks exact-date Agent observations,
bounded weekday closure, unbounded weekday tails, equity weekends, weekday and
weekend NAV carry, and correction/retraction recaptures. Each boundary compares
the point with ordinary panorama and deep-compares the complete endpoint
bundle, including quote identity, missing details and provenance. Query-only
reads and unchanged SQLite total changes establish the read-only contract;
withdrawing the later bounding close restores incomplete coverage. Existing
series budget, single-capture and cancellation regressions remain required.

## Synthetic benchmarks, saved cloud host

Measured on initial PR #57 head `36661f69bac215ca355f46ea8907b5f7c8c561fa`;
later review fixes are not new benchmark measurements.
AMD EPYC 9V74 80-Core Processor, Go server tag, CGO disabled. 100 brokerage accounts,
900 holdings in nine shared manual CNY instruments plus 100 cash leaves: exactly
1,000 possible leaves. Small values are exact `0.00012` per holding. Sparse
profile: 2,433 retained rows, 425,802 serialized input bytes. Dense: 86,433 rows,
13,513,678 bytes, including 9,000 quote backfills and 25,000 activity/effect/value
triples. Fixture setup is excluded. One iteration per range; these are measured
host observations, not a latency promise or statistical distribution.

| Profile | Days | Time | Total allocated bytes/op | Allocations/op | SQL queries/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| Sparse | 1 | 60.43 ms | 23,095,048 | 476,988 | 132 |
| Sparse | 7 | 116.11 ms | 68,108,368 | 1,123,223 | 132 |
| Sparse | 31 | 295.37 ms | 214,312,712 | 3,206,845 | 132 |
| Dense | 1 | 4.482 s | 275,105,560 | 5,017,534 | 622 |
| Dense | 7 | 4.777 s | 441,170,144 | 5,663,747 | 622 |
| Dense | 31 | 5.067 s | 1,071,655,488 | 7,747,495 | 622 |

Total allocated bytes are cumulative allocation, **not peak memory**. A separate
5 ms `runtime.MemStats` sampling run (GC before each request, setup excluded)
measured added heap high-water of 4.90/7.18/8.33 MB sparse and
84.31/110.04/118.63 MB dense at 1/7/31 days. Absolute sampled heap peaks were
5.38/7.65/8.81 MB sparse and 84.78/110.51/119.11 MB dense. Baseline heap was about
0.47 MB. Sampling can miss peaks; these values are not RSS or a hard memory
bound. Profiling slowed dense calls to 4.85/4.87/5.57 seconds. Large cumulative
allocation remains a follow-up optimization risk; initial input and ten-second
limits fail explicitly rather than silently reducing coverage.

Dense preflight cancellation returned after 10.255 ms with cancel scheduled at
10 ms (about 0.26 ms response after the scheduled cancellation). Hardware/load
can change this. SQL hydration previously hit SQLite's parameter limit with
25,000 activities; bounded 256-activity hydration fixed it without partial
financial replay. Query counts stay fixed with day count and depend on retained
activity hydration chunks, not public overview calls per point.

Reproduce with the repository toolchain:

```bash
CGO_ENABLED=0 go test -tags server ./internal/application -run '^$' \
  -bench BenchmarkHistoricalSeriesBounded -benchtime=1x -count=1 -benchmem
NW_SERIES_MEMORY_EVIDENCE=1 CGO_ENABLED=0 go test -tags server \
  ./internal/application -run TestHistoricalSeriesMemoryEvidence -count=1 -v
CGO_ENABLED=0 go test -tags server ./internal/application \
  -run TestHistoricalSeriesBenchmarkCancellationEvidence -count=1 -v
```

## Native acceptance to run

Use the reviewed immutable PR head and a new synthetic profile only. Check
complete/missing-price/missing-FX, negative/zero/tiny amounts, changed inclusion,
retained late-close and partial-role profiles at wide, 390/320px content widths
and 200% native zoom. Exercise one/31/32 dates, skipped civil date, local metric
and table selection, explicit endpoint/new-day recaptures, loading/errors/retry,
rapid selection/cancellation, offline reads, History Back across month boundary,
detail focus/scroll and another editable Sheet. Repeat en/zh-CN/zh-TW light/dark.
Record exact head, fixture, theme, locale, width and zoom with screenshots.
No P2b attribution, schema/provider expansion, current/reverse series or
unbounded-range acceptance is claimed.

## Saved-cloud validation record

- `CGO_ENABLED=0 go test -tags server ./...`: passed. Additional default-cap,
  basis-change and after-read cancellation regressions passed separately.
- `CGO_ENABLED=0 go vet -tags server ./...`: passed.
- `CGO_ENABLED=0 go build -tags server`: passed.
- Frontend: 95 files / 947 tests passed, including shared ECharts and Sheet.
- Frontend typecheck, ESLint, production build and generated-bindings check:
  passed; generation exposes 19 services, 141 methods and 193 models. Generated
  bindings are validated locally and restored rather than committed, following
  the repository's existing regeneration workflow.
- The production bundle retains the existing warning for chunks over 500 kB.
- Native desktop build, GUI screenshots, native keyboard/zoom and visual
  acceptance are unrun here: `pkg-config` cannot locate `gtk4` or
  `webkitgtk-6.0`. The server build is not a native desktop acceptance claim.
- Scoped race results are recorded in the PR after completion; native full-tree
  race is blocked by the same GTK/WebKit development dependencies.

## Review regressions: empty scope and selected evidence basis

Astra's two findings were reproduced before the fix. An available zero-total
portfolio entering an included CNY cash scope incorrectly retained all metric
connections. Manual price and FX observations followed by exact-date Agent
observations also incorrectly retained connections despite the selected source
changing. Saved preferences alone did not identify the applied evidence basis.

- `TestHistoricalSeriesEmptyScopeTransitions`: zero → included 100 → zero →
  unchanged zero → included 100 → unchanged 100. Both scope directions break
  all connections; unchanged empty/populated scopes reconnect normally.
- `TestHistoricalSeriesScopeAfterMissingCivilDate`: Apia's skipped civil date,
  both entering and leaving the cash scope after it. No flags/connections cross
  the gap; the next unchanged available point reconnects normally.
- `TestHistoricalSeriesSelectedEvidenceBasis`, price and FX cases: ordinary
  manual updates stay connected; selected manual → Agent source breaks;
  ordinary Agent date/value/identity updates stay connected; an Agent timestamp
  basis change breaks. Exact point totals and full P1 endpoint equality hold,
  and reads succeed with SQLite `query_only` enabled.

The fix keeps explicit preceding-available-point state and derives selected
source/basis signatures from valuation evidence and its retained observation.
Review-fix validation passed: full server-tag Go tests, focused application/API
vet, and `go test -race -short` for Historical Series, Historical Overview and
Agent quote/FX tests (71.985 seconds). Short race mode omits the dense synthetic
cancellation setup; that fixture passed in the full non-race Go run. Frontend
code/contracts are unchanged and its suite/build were not rerun for this fix.
Native visual acceptance remains pending. The reviewed new head is recorded in
PR #57.

## Further review: known-zero exposure and Atlas adjudication

The zero-exposure regression failed before production changes on `c058a65d`:
unchanged price and FX bases incorrectly broke the line when holding quantity
became zero. The fix keeps durable signatures separate from capture-local,
per-leaf actual-evidence comparison memory. It retains actual source semantics
through known-zero points, compares on resume, and clears memory on scope
removal, unknown nonzero evidence or nonexistent-date gaps. Zero valuation rows
remain evidence-free and keep their exact backend amounts.

`historical_series_zero_test.go` covers price and FX cases: unchanged
1→0→0→1 stays connected; changed A→0→0→B breaks at B; a selected range starting
at zero and an origin-zero holding have no prior actual source. Scope removal
and Apia gaps reset memory before resumed exposure. A durable FX-preference
change still breaks at zero. Query-only reads, exact totals and full P1 endpoint
equality are checked, and zero detail rows retain no price/FX evidence.

Atlas's all-unknown endpoint subtotal issue was confirmed with a failing page
regression: the point/table reported unknown but the card used P1's legacy zero
string. Series cards now use nullable endpoint point subtotals, preserving P1
API compatibility and distinguishing unknown from an observed-zero subtotal.

The invalid-date/budget precedence issue was also reproduced. Range validation
now reads authoritative origin metadata in the same transaction before any
financial facts. The over-budget fixture rejects before-origin/current/future
dates as invalid, with one metadata query and zero fact/byte scans. Valid series
captures add one metadata query to the original benchmark query counts; no new
benchmark timing or heap measurements are claimed.

Ignored endpoint decimal parse errors were unreachable under current exact
valuation-output invariants. They are now propagated explicitly to protect
against future invariant changes, without altering financial calculation rules.
Final checks and the new immutable head are recorded in PR #57. Native GUI
acceptance remains pending; independent Atlas measurements of 3.51 seconds for
dense 31-day capture and 10.24 ms preflight cancellation concern its original
`36661f69` review, not measurements of this follow-up.

This follow-up passed full server-tag Go tests (application 43.921 seconds),
focused application/SQLite/API vet, and focused Historical Series/Overview/Agent
quote/FX race tests with `-short` (84.615 seconds). The dense cancellation
fixture is omitted from short race mode and passed in the full non-race suite.
Frontend validation passed 95 files / 948 tests, typecheck, ESLint, production
build and generated-bindings check. Shared chart and Sheet suites are included.
Generated legacy binding blobs were restored per repository policy. Existing
bundle-size warnings remain. No native visual acceptance or new benchmark
measurement is inferred from these checks.

## Native dense-entry blocker: historical activity hydration

Atlas found that the initial P1 overview prevented entry to the trend on the
dense synthetic profile, despite the series API working. This was reproduced
on `cb09d746` with the same 100 accounts / 1,000 leaves / 25,000 activities:
both `HistoricalOverview` single-day and comparison reads failed with SQLite
`too many SQL variables`. The resulting-endpoint union repeats the full activity
ID list four times, exceeding the bundled SQLite parameter limit.

The historical loader now shares the existing series strategy: hydrate children
in at most 256-activity chunks using the caller's one transaction, then replay
the full economically ordered activity set once. Generic activity-page and
mutation hydration are unchanged. The economic correction query and effect
ordering are unchanged, and historical money replay now observes cancellation.
No input/date truncation, new admission limit, migration/provider, snapshot
write or unrelated repository refactor was added.

`TestHistoricalOverviewDenseEntrySingleAndComparison` exercises the actual
single-day and comparison entry APIs in a query-only profile, retaining all
1,100 rows and exact `0.108` assets/net worth, zero B−A and unchanged total
changes. Its 31-point series has 86,433 facts and deep-equal P1 endpoints. The
targeted post-fix run measured about 5.02 seconds single-day and 6.14 seconds
comparison, with fixture setup excluded. These are API observations, not native
GUI acceptance or a latency promise.

`TestHistoricalOverviewHydrationChunksPreserveEconomicReplay` crosses two
256-activity boundaries with 513 economic records and an earlier correction.
It verifies unique IDs, original economic ordering, one resulting endpoint and
effect per record, unchanged audit magnitudes and global absolute-balance
normalization (644 versus recorded 645; later delta 244; final balance 1500).
Infrastructure regressions check bounded SQL parameters, cancellation before
later chunks, and stable results across a concurrent derived-projection update
in a separate WAL connection. The read uses one unchanged snapshot throughout.

The dense entry fixture runs in full non-race validation; short race mode skips
its expensive 1,000-leaf setup while retaining the smaller chunk/correction and
transaction/cancellation tests. Final aggregate results and the new head are
recorded in PR #57. Atlas's native recheck is still required on that head.

This loader follow-up passed full server-tag Go tests (application 77.443
seconds), focused SQLite/application/API vet, and focused historical,
correction and financial-context race tests with `-short` (SQLite 9.770 seconds;
application 97.591 seconds). Dense entry runs in the full non-race suite; the
smaller chunk-order, cancellation and concurrent-transaction cases run under
race. Frontend/public DTOs are unchanged and frontend checks were not repeated.
Atlas's additional native 8,000-activity pass / 8,300-activity failure supports
the four-times-ID parameter diagnosis; its 5,000-activity GUI pass and native
date/Back/cancellation results do not substitute for rechecking the retained
25,000-activity profile on the new head. No model substitution or duplicate
commit was performed while resuming the existing work.

## CI race timeout and standalone snapshot follow-up

The downloaded run archive for CI `38262656778`, job `114856842991`, confirms
`panic: test timed out after 10m0s`, package duration 600.054 seconds. At timeout
the dense entry test had run 9m55s and its comparison subtest 3m9s. The runnable
stack was in modernc SQLite's instrumented sorter while reading all instrument
quotes; there was no `WARNING: DATA RACE` or assertion failure in that log.
Atlas independently reproduced this timeout; the non-race case takes about
27 seconds. A focused `-short` run had not exercised the failing full shard.

Full normal CI still requires the original 100 accounts / 1,000 leaves / 25,000
activities / 9,000 quote backfills, both successful overview entry APIs, exact
rows/totals and 31-point series endpoint equality. Race mode instead uses two
accounts / 20 leaves / 8,300 activities with the same nine instruments: the old
four-branch hydration still needs 33,200 variables, exceeding the actual SQLite
limit and reproducing the loader defect. All single/compare success, complete
row, exact amount/delta, entry equality, query-only and cancellation assertions
remain. No test is removed and neither CI nor production deadlines are raised.
Instrumented series reads must either pass all success/equality checks or return
only the unchanged ten-second computation-deadline error after ten seconds with
an empty bundle. Other errors, input-budget rejection and partial results fail.
The full-scale successful series result remains mandatory without instrumentation.

The first LOCAL representative full 288-test application 2/2 shard passed in 580.147
seconds with 1,000 race leaves, too close to the ten-minute limit. Account scale
was then reduced only for race mode, retaining the variable-limit failure case.
Final representative shard results are recorded in PR #57.

The final LOCAL full application 2/2 race shard selected the same 288 of 533 names and
passed in 438.135 seconds using the CI selector and command, without `-short`
or a timeout increase. LOCAL standalone/hydration race regressions passed
separately in 5.965 seconds. These are saved-cloud measurements, not GitHub CI
durations; application package selection and the race command match CI, with no
server tag in the local race run. A subsequent read-only toolchain check observed
Go1.26.0 locally. The historical test logs do not record their Go version, so no
historical toolchain version is asserted from those logs.

Atlas's standalone `ListActivitiesUntil` concern also reproduced: its multiple
database reads could combine an activity marker and a derived projection from
different atomic commits. The public standalone method now owns one read-only
transaction. Existing batch/transaction callers still use the private queryer
method with their supplied transaction, avoiding nested database acquisition.
The hydration helper comment now states that the caller supplies its queryer
and is responsible for a consistent transaction.

`TestStandaloneActivitiesUntilSnapshotAndTiedOrdering` calls the real public
method on 257 activities while another WAL connection commits new marker and
projection versions. It verifies one coherent snapshot and creation/ordering-ID
ties across the 256-record boundary. The regression failed before the fix,
then passed three repeated normal runs and race validation. No unrelated
repository interface or schema/provider changes are introduced.

The final LOCAL full non-race server-tag Go suite passed (application 70.993 seconds;
SQLite 5.012 seconds), retaining the original full dense success assertions.
Focused application/SQLite/API vet passed. Frontend and public DTOs are unchanged;
frontend validation was not repeated for this test/read-transaction follow-up.
