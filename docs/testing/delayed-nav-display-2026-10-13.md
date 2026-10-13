# Delayed NAV display verification — 2026-10-13

The approved 27,761-byte Library plan version 0 was materialized and read in
full before edits. Remote main was rechecked as
`36fa9035267bc48dde0e087cd4e5f99bf3610600`. All data used here was synthetic,
created under temporary paths; no private backups, screenshots or real ledger
were accessed. No provider requests or desktop app launch were performed.

## Regression matrix evidence

| Plan cases | Evidence |
| --- | --- |
| 1–5: carried NAV, rollover, quantities, late economic revision, no lookahead | New `valuation_quality_test.go`: full provisional 200, quantity 150 → 300, next-day NAV revises 200 → 210, fetch-day value 210, actual observation count unchanged by reads |
| 6: verified closure versus unverified carry | Existing agent selection and historical series selection suites; strict selectors unchanged |
| 7–9: missing price/FX/account value, distinct counts, none/empty/zero, missing liability | New quality tests; existing valuation/FX/native/overview regressions; missing snapshot does not prove empty liabilities |
| 10: economic versus fetched dates | New carried price/FX bounds test; wire evidence adds separate effective/fetched fields; three-locale quality disclosure tests |
| 11: binding/source/retraction/revision/basis | Existing agent quote, snapshot and series selection suites; no selector or quote-history synthesis changes |
| 12: exact requested endpoint changes | Additive estimated changes require full compatible requested endpoints; legacy strict endpoint tests retained |
| 13–15: return isolation, revision invalidation, level-only estimates, actual final bucket date | New Asset Trend regression checks strict beginning/end, capital/attribution/return/rate fields remain empty despite full display; missing final day stays unknown for week/month; existing return/golden/revision suites |
| 16: scope/filter/native/base | Existing analysis universe, valuation and projection regressions; portfolio uses holdings only |
| 17: conservative legacy and read-only captures | New legacy subtotal test and SQLite total_changes assertion across panorama/series; existing MCP immutable capture tests |
| 18–20: generations, cancellation, rollover/timezones, bounded reads | Trend generation guard; existing snapshot/civil-date/cancellation/input-budget suites; new Shanghai clock/resume test; existing performance budget passes |
| 21: chart, keyboard, table, locales, 390px | New quality/segment tests; full frontend interaction suite; synthetic Chromium checks at 1280/390 in en, zh-CN, zh-TW; keyboard locale focus; no page overflow/errors |
| 22: actual quote observations only | New observation count assertion; raw QuoteHistorySheet and quote selectors unchanged |
| 23: health information versus repairs | Coverage-only notices are info/action none, actual absent inputs remain blockers; health lifecycle regressions and full frontend health tests |
| 24: no provider request on reads/rollover | New clock only invalidates local read queries; existing fake-provider health/selection tests; browser harness has no backend/provider connection |

## Commands and platform boundaries

Go uses the pinned module's Go 1.26.0 toolchain with temporary GOPATH/GOCACHE.
Linux checks use `CGO_ENABLED=0` and `-tags server`; native GTK/WebKit headers
are absent. The initial native-dependent aggregate attempt did not pass and was
replaced by the server-profile aggregate. The isolated performance regression
passed in 3.89 seconds after an earlier contended aggregate exceeded its budget.

Frontend uses the locked dependencies, pnpm 12.2.1 and Wails beta.28. Binding
scripts run with `GOFLAGS=-tags=server`, `CGO_ENABLED=0`, writable temporary
Go caches and `NESTWORTH_WAILS_GOCACHE`. Generated bindings/bundles are ignored,
not committed. Production build reports the existing large-chunk advisory.

Required verification: focused financial/return tests, aggregate server Go
`test -p 1 ./...`, `vet -p 1 ./...`, server build, gofmt, diff check; frontend
lint/typecheck/full tests/build; pinned binding freshness; portable skill check
and 53 tools tests. The full frontend suite has 110 files / 1,169 assertions.
The initial contended frontend run had one timeout (passed isolated) and an
extra query refetch (fixed by retaining the original invalidation granularity).

Synthetic browser QA renders actual shared quality, chart and historical metric
components with generated examples, rather than launching the user's app.
Screenshots were inspected locally and not uploaded. The narrow historical
source table keeps its width inside a horizontal scroll container.

Native macOS/Wails, VoiceOver, packaging, signing/notarization and release smoke
are unrun. Aggregate race CI is a separate gate; no Linux test is claimed as
native Mac acceptance.

## Concurrent integration and response limits

PR82 at `1380ebbbea61fd0b10f2059cc172f9ad475f39d3` was inspected without
importing its unmerged changes. A temporary three-way merge of the overview
adapter completed without conflicts and retained PR82's financial-row/action-row
composition. Amount font classes remain unchanged; compact quality sits beside
the existing title. No existing PR description was edited.

Additive quality increases response bytes. The unchanged MCP wire guard rejects
an oversized synthetic ten-date capture with explicit `too_large`; a narrower
three-date capture matches the GUI exactly. Dates are never silently truncated.
New paging/batch infrastructure remains owned by the concurrent MCP scalability
task. This branch only adds display semantics and keeps budgets/read-only rules.


## PR83 review revision

Confirmed review findings were fixed without editing the PR description:

- Real ECharts models now lose obsolete provisional segments on refetch; a
  many-segment provisional chart becomes one revised verified series on the
  same instance, with true gaps retained. Zoom and legend state survive. This
  is an opt-in series replacement policy; other consumers keep their policy.
- Production Asset Trend leaves retain exact signed decimals separately from
  rounded display views. Two 0.00006 leaves aggregate to 0.00012 and display as
  0.0001 across current overview, snapshot trend, panorama and day/week/month
  level projection. Strict return calculations are not fed display values.
- Selected snapshot evidence uses scoped 256-ID batches in one read transaction,
  rather than the whole-profile panorama reader. A synthetic profile with 1,100
  unrelated archived accounts still serves ready and current-only trends; the
  current-only window performs no evidence/history read. Panorama limits remain.
- New estimated changes omit incompatible included scopes and selected quote
  basis/source changes. Actual overview/series/trend regressions retain the
  original strict +100 inclusion change and zero source-change difference.
- A USD NAV dated 10-09 used on 10-12 remains full/provisional in native USD even
  without CNY FX. The base valuation remains unavailable; absent FX does not
  erase unverified price coverage.
- All-partial and mixed partial/full/unknown Asset Trend levels keep exact known
  contribution, quality and market-date text in the actual keyboard-accessible
  source table. Partial amounts never become full-total chart points.

New-head aggregate validation and CI are reported separately from the first
head's successful CI. Native Mac gates remain unrun; parent review remains the
readiness gate.

The review revision was rebased without conflicts onto merged main
`f13dbfacc21bad0439848cbe1df64ad9ad1f1823`, retaining the latest homepage and
Available Funds standard-review behavior. Final integrated server Go aggregate
(test/vet/build), frontend lint/typecheck/build, 112 frontend files / 1,203
assertions, and pinned binding freshness passed. New estimated-basis tests also
ensure ordinary conversion price/rate/date updates do not become basis changes.
The existing skill check remains passing. New-head CI and final independent
parent review remain required before merger; this agent does not merge PR83.

### Row-level compatibility follow-up

Leaf and account display cells now retain the same private scope/basis records
used by aggregate estimated changes. Account keys aggregate included children;
no per-row database reads or wire evidence expansion is added. A manual-source
A to carried agent-NAV B regression checks full/provisional B, no leaf/account
estimated differences, and unchanged strict BaseChange. A same-basis provisional
comparison retains estimates. Verified source changes also retain strict
BaseChange without granting a new estimate. The new-quality UI uses guarded
estimated row changes authoritatively; legacy projections retain their fallback.

The residual revision also prefers declared/known-subtotal native currency for
all-partial Asset Trend headings (USD regressions use CNY household settings).
CI run 38427640126 failed only the cold 3Y/500-component wall-clock budget at
10.42s versus 10s; all race jobs passed. Analysis now memoizes complete unquoted
balance display projections within one capture, preserving exact decimals,
liability sign and copy isolation. Quoted/provisional evidence remains on its
normal date-dependent path. The unchanged local performance test passed in
3.36s alongside the targeted precision/coverage regressions; no budget was raised.

This revision rebases without conflicts onto merged main
`eed6362c2ecc9ac512f1f3b4707e853b7b69de5a`, preserving PR84's MCP changes.

Final residual local verification on the merged MCP base passed server Go
aggregate test/vet/build and frontend lint/typecheck/build, 112 files / 1,206
assertions, pinned binding freshness and the merged skill's 69 JSON examples.
New-head CI is checked separately; native Mac verification remains unrun.
