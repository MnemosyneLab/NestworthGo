# Upcoming actions synthetic QA

Owner: Nestworth maintainers. Scope: feature branch based on
`319ce0ba55244e769d454c5d0fe3d854aebe5438`. Status: automated server-profile acceptance passed; native desktop acceptance pending. Date: 2026-10-11.

## Review amendments

The reviewed v2 plan corrects six implementation hazards in the v1 proposal:
request-local valuation clock/provider/TTL; explicit phase precedence;
reservation classification before amount comparisons; scope notes distinct from
data incompleteness; deterministic nested arrays and one merged data issue per
source/category; active-only polling and typed navigation requests. The source
main matched the pinned baseline. No material product decision blocked the MVP.

## Automated evidence

Tests use synthetic fixtures, never real profiles. Domain tests cover civil
window endpoints, timezones/DST/leap/year boundaries, maturity versus unlock,
fixed versus rolling receipt, indefinite overdue retention, excluded/unknown
rules, zero versus missing amounts, missing fees/FX, native shortfalls, invalid
reservation currencies, source/currency deduplication, scope notes, shuffled
inputs and input immutability.

Application tests compare complete existing Overview output, count exactly one
clock and snapshot read, check request-local configuration and concurrent reads,
and exercise Preview cancellation, rejected writes, interest, loss settlement,
renewal and undo against authoritative product operations. A real synthetic
SQLite file is opened read-only: 35 advancing-day reads preserve both the
snapshot JSON and database SHA-256 and create no WAL/SHM sidecars. An embedded
repository spy panics on calls other than the permitted snapshot read; no
provider registry is installed for the read-only test.

Frontend tests cover shared query deduplication, active-only 60-second refresh,
focus refresh, mutation invalidation including History Origin, three locales,
summary/full counts, 50-row incremental rendering, partial/initial/stale states,
unknown types, repeated same-product navigation, missing-source reservation
routing, released-reservation focus and Back/filter retention.

Final commands (after sourcing the cloud tool environment):

- `CGO_ENABLED=0 env -u NESTWORTH_QA_DATABASE wails3 task check` — passed:
  Go tests/vet/build, generated binding check, frontend production build/lint/
  typecheck, 90 frontend test files / 782 tests, and `git diff --check`.
- `go test -race -short ./internal/domain ./internal/application ./internal/wailsapi/liquidity -run Upcoming -count=1` — all three packages passed.
- `go test ./internal/application -run '^TestUpcomingActionsPerformanceReport$' -count=1 -v` — passed, final measured sample below.

The tool environment uses `GOFLAGS='-mod=readonly -tags=server'` and
`GOMAXPROCS=5`; the repository task was not an untagged native build. Generated
Wails bindings were produced by the repository generator, without manual edits.

## Performance sample

Synthetic in-memory snapshot: 1,000 sources, 2,000 reservations and 500 managed
products. Linux amd64 on an AMD EPYC 9V74 (5 virtual CPUs), Go 1.26.0, server build
profile, GOMAXPROCS=5; three warmups
and 20 measured runs per path. One isolated sample gave projection P95 8.95ms,
endpoint P95 115.16ms and normal Overview P95 111.52ms. The comparison limit for
that sample was 189.40ms (Overview ×1.25 +50ms). Both proposed gates passed that
sample. Concurrent suite load can alter timings; these are measurements, not
stable CI assertions or claims about native UI latency. The endpoint sample
includes existing evaluation/valuation but not SQLite disk I/O.

Reproduce with `go test ./internal/application -run
TestUpcomingActionsPerformanceReport -count=1 -v`; scale read-count correctness
is independently asserted. Benchmarks are available through
`go test ./internal/application -run '^$' -bench BenchmarkUpcomingActionsScale`.

## Remaining acceptance gates

Native GUI is untested. This Linux environment lacks GTK4/WebKit6 development
libraries and has no sudo executable; native dependency installation could not
run. Server-tagged compilation and jsdom tests do not prove desktop behavior.
Before release, run an independent synthetic database on a supported Mac:
Overview → list → receipt Preview → cancel/confirm → Back → undo; reservation
release and policy edits; repeated same-object sheet navigation and focus;
relaunch/persistence, household midnight, sleep/resume, restore/reset;
keyboard/VoiceOver, 200% zoom and long names in all three locales. Confirm
restoration never displays the previous household's cached results. Check
native packaging and untagged repository checks. No CI polling, merge, tag,
release, deployment or production-data modification belongs to this task.

## Changed files

The three binding files below were already tracked at the baseline; their
changes were generated, not handwritten. No skill package, migration or MCP
registration is changed.

- `README.md`
- `docs/README.md`
- `docs/design/README.md`
- `docs/design/available-funds.md`
- `docs/design/household-navigation-and-health.md`
- `docs/design/upcoming-actions.md`
- `docs/testing/qa-upcoming-actions.md`
- `docs/user/nestworth-skill.md`
- `frontend/bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/index.ts`
- `frontend/bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models.ts`
- `frontend/bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/service.ts`
- `frontend/src/App.test.tsx`
- `frontend/src/App.tsx`
- `frontend/src/app/AppShell.tsx`
- `frontend/src/app/navigation.test.ts`
- `frontend/src/app/navigation.ts`
- `frontend/src/features/liquidity/AvailableFundsNavigation.test.tsx`
- `frontend/src/features/liquidity/AvailableFundsPage.test.tsx`
- `frontend/src/features/liquidity/AvailableFundsPage.tsx`
- `frontend/src/features/liquidity/OverviewLiquidityCard.tsx`
- `frontend/src/features/liquidity/ReservationManager.test.tsx`
- `frontend/src/features/liquidity/ReservationManager.tsx`
- `frontend/src/features/liquidity/liquidityDisplay.ts`
- `frontend/src/features/overview/OverviewPage.test.tsx`
- `frontend/src/features/overview/OverviewPage.tsx`
- `frontend/src/features/upcoming-actions/OverviewUpcomingActionsCard.tsx`
- `frontend/src/features/upcoming-actions/UpcomingActionList.tsx`
- `frontend/src/features/upcoming-actions/UpcomingActionsPage.test.tsx`
- `frontend/src/features/upcoming-actions/UpcomingActionsPage.tsx`
- `frontend/src/features/upcoming-actions/upcomingActionPresentation.test.ts`
- `frontend/src/features/upcoming-actions/upcomingActionPresentation.ts`
- `frontend/src/i18n/index.ts`
- `frontend/src/i18n/localeCoverage.test.ts`
- `frontend/src/i18n/upcomingActions.ts`
- `frontend/src/queries/history.mutation.test.ts`
- `frontend/src/queries/history.ts`
- `frontend/src/queries/invalidation.test.ts`
- `frontend/src/queries/keys.ts`
- `frontend/src/queries/liquidity.test.tsx`
- `frontend/src/queries/liquidity.ts`
- `frontend/src/test/upcomingActions.ts`
- `internal/application/liquidity.go`
- `internal/application/upcoming_actions.go`
- `internal/application/upcoming_actions_benchmark_test.go`
- `internal/application/upcoming_actions_readonly_test.go`
- `internal/application/upcoming_actions_test.go`
- `internal/domain/upcoming_actions.go`
- `internal/domain/upcoming_actions_test.go`
- `internal/wailsapi/liquidity/upcoming_actions_dto.go`
- `internal/wailsapi/liquidity/upcoming_actions_test.go`

## Follow-up: restored navigation requests

The App-level A → list → B → Back → Back regression reproduced the review
finding before the fix: product A's dialog reopened when Back restored its old
request ID. Available Funds now retains all consumed request IDs for the retained
page session, rather than only the last one. Restoring a consumed request does
not refetch or reopen its sheet; a genuinely new ID can still reopen the same
product. App integration tests verify the exact route and that same-page reopening
does not duplicate the navigation trail. A page-level test also checks request
read counts and restoring A after B.

`CGO_ENABLED=0 pnpm --dir frontend exec vitest run src/App.test.tsx
src/features/liquidity/AvailableFundsNavigation.test.tsx
src/features/liquidity/AvailableFundsPage.test.tsx` passed: 3 files / 36 tests.
The full server-profile `CGO_ENABLED=0 env -u NESTWORTH_QA_DATABASE wails3 task
check` also passed: 90 frontend files / 784 tests, Go tests/vet/build, generated
bindings, frontend build/lint/typecheck and whitespace checks. The native desktop
acceptance boundary above is unchanged. This follow-up changes only the page,
the two regression-test files and this QA report.

## Follow-up: same-page history, failed navigation and ordinary overrides

Review of `0ad76b9` was reproduced with failing tests: a same-page Data Health
View gap had no Back entry; an explicit successful Retry left its requested
product closed; ordinary unlock plus receipt override copied a fixed receipt
while labelling it as a request-date estimate. The fixes are scoped:

- Same-page history suppression now applies only to Available Funds object
  navigation. Data Health gap focus can return to its prior broad view.
- A failed target read remains pending. Explicit Retry processes its successful
  result; another error does not trigger a loop. Leaving or superseding a
  request cancels it, and its late result cannot reopen the sheet or be replayed
  by Back. Tests cover product, policy and reservation targets.
- Ordinary explicit unlock actions retain today/upcoming/eligible phases, but
  omit override receipt details from this projection. Available Funds keeps the
  authoritative fixed override unchanged; no managed receipt_due is created.
  Past/future receipts, today/future unlocks, and override without unlock are
  tested.

Focused frontend command (the same server-profile tool environment):
`CGO_ENABLED=0 pnpm --dir frontend exec vitest run src/App.test.tsx
src/features/liquidity/AvailableFundsNavigation.test.tsx
src/features/data-health/DataHealthPage.test.tsx
src/features/liquidity/AvailableFundsPage.test.tsx` passed: 4 files / 53 tests.
`go test ./internal/domain -run Upcoming -count=1` and
`go test -race ./internal/domain -run Upcoming -count=1` passed.
Full `CGO_ENABLED=0 env -u NESTWORTH_QA_DATABASE wails3 task check` passed:
Go tests/vet/build, generated bindings, frontend build/lint/typecheck,
90 frontend files / 790 tests and whitespace checks. Native GUI work was not
performed or duplicated; its acceptance boundary remains with the separate
review. No financial operations, schema or MCP tools changed.

## Follow-up: read timestamp and plural presentation

Parent-reported native Linux primary smoke at `03a569f` found raw RFC3339
nanosecond read timestamps and English singular counts rendered as plurals.
Upcoming Actions now formats its read instant with the reported household zone
and current locale, using the app’s medium-date/short-time convention and a
visible timezone name. Invalid timestamps, invalid civil timestamps, invalid
zones and absent/system zones show a localized unavailable label; they never
fall back to the browser clock or zone. The precise DTO asOf is unchanged.

New count copy uses i18next `_one`/`_other` forms for independent receipts,
reservation issues, schedules, data issues, notes, action totals, reservation
labels and relative days. Backend counts and their category breakdown are
unchanged. Archived-reservation focus and total-breakdown review are left to
the separate reviewer. No financial calculations or unrelated UI changed.

Focused presentation, locale-coverage and App tests passed: 4 files / 62 tests.
Coverage includes all three locales, nanosecond input, timezone midnight and
New York DST boundaries, invalid date/zone safety, singular/zero/plural forms,
and matching timestamp output on both the card and full list. No new native
GUI acceptance was performed by this task; the Linux smoke report applies to
the reviewed prior head, and macOS acceptance remains pending.

Full server-profile `CGO_ENABLED=0 env -u NESTWORTH_QA_DATABASE wails3 task
check` passed after the presentation changes: Go tests/vet/build, generated
bindings, frontend production build/lint/typecheck, 90 frontend files / 814
tests, and whitespace checks.
