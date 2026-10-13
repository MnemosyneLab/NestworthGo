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
