# Shared-control audit — 2026-10-13

Initial base: `041abd6`; rebased onto `c257a05` after PR59 merged, then
merged main `c779e2e` after PR60 merged. Dedicated branch:
`fix/shared-control-consistency`. The supplied Library PNG was materialized
locally and its actual pixels inspected: it shows Historical Overview's native
baseline calendar. No screenshot-specific inference was made before inspection.

## Date/time screen inventory

| Screen / owning component | Fields and disposition |
| --- | --- |
| Historical Overview | A baseline, B comparison, trend A/from, trend B/to: all four native date inputs replaced with DatePicker; clear/invalid state, bounds, A/B direction, query orchestration and return focus preserved |
| OnboardingPage | History start: already DatePicker, explicit today maximum |
| AccountForm / AccountCreateWizard | Opened-on: already DatePicker, origin/today bounds and optional clear |
| StartHistoryForm | History start: already DatePicker |
| RecordChangeForm | Effective day/time: already DatePicker/TimePicker |
| HistoryPage | From/to filters: already DatePicker, clear and cross-field bounds |
| AnalysisFilterBar | Analysis from/to: already DatePicker, origin/closed-day/cross-field bounds |
| MarketDataPage | Manual FX effective date: already DatePicker |
| InstrumentManagement | Manual quote date: already DatePicker |
| ProductFields | Start, maturity, interest paid-through, unlock, receipt override: already DatePicker; contract/future/null semantics retained |
| DepositFields | Start, maturity, paid-through: already DatePicker; existing limits retained |
| ProductDetailSheet | Paid-through: already DatePicker, product start/paid-through maximum |
| ProductTimeFields | Effective day/time used by product actions: already shared pickers, household timezone and disabled state retained |
| SimplePolicyFields | Explicit unlock date: already DatePicker, future/clear/null retained |
| AvailableFundsPage | Custom horizon: already DatePicker, local-date minimum and future/clear retained |
| InvestmentReviewPanel (merged PR59) | No custom date entry; 7/30-day snooze presets preserved. Policy window raw select replaced with NativeSelect, preserving numeric conversion and fieldset disabling |
| UpcomingActionsPage / OverviewUpcomingActionsCard | Date labels/horizon navigation and action entry points; no independent date-entry picker on main; underlying product sheets covered above |
| ReturnCalendarTab | Return-data calendar/year visualization and drilldown: deliberate visualization exception |
| DateRangeControl / RangeToggle | Presets and bounded previous/next navigation, not arbitrary date-entry controls; retained |

No production native date/datetime-local/time/month/week inputs remain.
Calendar and DayPicker imports are confined to the canonical UI implementation.
Test-only date/time inputs are deliberate stubs used to inject invalid state.

## Related findings and prevention

- DatePicker previously normalized impossible dates (e.g. February 31) and
  interpreted years 00–99 through JavaScript's 1900 offset. It now verifies the
  civil-date round trip and constructs the full year explicitly.
- DatePicker now forwards the button ref, autofocus and accessible naming;
  Historical Overview's navigation/detail focus fallback targets that trigger.
- Explicit minimum/maximum exclusions use independent matchers; inclusive edge
  behavior is regression tested. Business validation remains at the caller.
- TimePicker selected hour/minute was conveyed only by button styling. It now
  exposes aria-pressed and supports accessible names/error context like DatePicker.
- Existing NativeSelect/FilterableSelect, Sheet/AlertDialog/Popover, Button and
  PageState conventions were reviewed. The merged InvestmentReview policy-window raw select was consolidated onto
  NativeSelect. No remaining competing production raw select,
  dialog/alertdialog role, Base UI or day-picker import was found outside UI.
- Existing native checkboxes/radios lack a single canonical styled wrapper;
  spacing/accent differences remain a separate design-system follow-up. Creating
  a new checkbox system is outside this targeted consolidation.
- Specialized chart rows/day cards, read-only connection textarea, file inputs
  and chart range presets are legitimate semantic/custom-layout exceptions.
- Existing loading/empty/error components and shared overlay focus behavior
  remain authoritative. No financial logic or general screen redesign changed.

Root `AGENTS.md` points to existing engineering rules; `frontend/AGENTS.md`
points to `docs/development/frontend-controls.md`. The engineering guide links
that contract without duplicating it. ESLint rejects temporal inputs, raw
select/dialogs, raw dialog roles, direct primitive imports and Calendar entry
bypasses outside UI. Guard tests prove rejection and scoped exceptions. Dynamic
aliases/spreads still require review; the guard is deliberately lightweight.

## Parallel draft coordination

PR60 (`fix/historical-query-cancellation`) overlaps
`HistoricalOverviewPage.tsx` and `HistoricalOverviewPage.test.tsx`.
Preserve its cancellation changes when integrating: this branch only replaces
controls and adapts control testing, not capture cancellation. This branch also
updates App.test assertions from input values to button text.
PR59 merged into main as `c257a05`; this branch was rebased onto that main
without conflict. Its panel/upcoming-action work was inspected and its policy
select now uses NativeSelect. This is an additional affected screen.
Neither draft branch was edited, checked out or cherry-picked. PR60 was subsequently merged by its owner and integrated through main as
described below.

## Verification

Results and platform limitations are recorded in the draft PR. Chromium checks
use real components/styles in a temporary isolated fixture, not Wails IPC or
real financial data. Historical Overview interaction also has a real-component
jsdom integration test; broader orchestration tests retain input stubs for
invalid-value injection. Native macOS WebKit, VoiceOver, magnification, native
select dropdown appearance and packaged-app smoke remain manual gates.

Reconciled-head checks:

- `CGO_ENABLED=0 pnpm run typecheck`: passed, including fresh Wails bindings.
- `pnpm run lint`: passed after replacing Investment Review's raw select.
- `CGO_ENABLED=0 pnpm run build`: passed; Vite reports >500 kB ECharts and
  invalidation chunks (bundle optimization deferred).
- `CGO_ENABLED=0 go build -p 1 -o /tmp/nestworth-date-headless ./cmd/nestworth`
  with the environment's `-tags=server`: passed; this is a headless server build.
- Chromium fixture checks passed in America/Los_Angeles and Pacific/Kiritimati;
  see `shared-control-browser-2026-10-13.json`. Actual dark-render pixels were
  inspected. Covers bounds, selection/clearing, Enter/Escape, return focus,
  nested sheet dismissal and time selection semantics.
- Standard native Go build/vet/test attempts are blocked by missing GTK4,
  WebKitGTK, libsoup and gio development packages. The broad Go test attempt
  was stopped after native compilation failures; do not interpret it as a Go
  suite pass. No Go production code is modified.
- The first frontend run exposed four old input-value assertions and two
  timeouts under competing Go work. Assertions were adapted; the subsequent
  complete pre-rebase run passed 102 files / 1,005 tests. The final full run on
  reconciled main passed **104 files / 1,026 tests** with two workers. The
  additional arrow-key/Enter test also passes independently (8 shared date/time
  tests).

Follow-up guard review: reproduced four bypasses for dialog/alertdialog roles
written as single- or double-quoted JSX string expressions. Added the matching
role-expression selector. All 19 guard tests pass, including both roles in
quoted/expression forms, wrapper/test exceptions, non-dialog roles and the
explicitly documented dynamic-alias limitation. Full lint, regenerated-binding
typecheck and diff whitespace checks pass. No application runtime changes in
this follow-up; prior frontend/build/browser evidence remains applicable.

## Combined cancellation integration

Merged main `c779e2e5b3b1fc3e145a98ae0f5a4a03db05c66d` (PR60) into
this branch with a normal merge, without force-pushing or changing its owner
branch. Both Historical Overview files merged cleanly. The combined page keeps
`isActive && validCapture` on overview and series queries, cancellation/late
reply tests, all four shared DatePicker controls and the button focus fallback.
Combined validation passed: 134 frontend tests across seven files (Historical
Overview orchestration, real picker integration, App navigation, capture and
Wails cancellation, date/time keyboard behavior and guard contracts); full lint;
regenerated-binding typecheck; and targeted Go cancellation tests in
`internal/application` and `internal/wailsapi/analytics` using the headless
server environment (`CGO_ENABLED=0`). `git diff --check` also passes.

Nonblocking preexisting follow-up: `components/ui/filterable-select.tsx`
tracks keyboard `activeIndex` and highlights options, but options have no stable
DOM IDs and the focused combobox has no `aria-activedescendant` pointing to the
active option. Thus arrow-key highlight changes are not exposed through that
accessibility relationship. Address this in a separate focused accessibility
change with option-ID/active-descendant, filtering and keyboard regression
coverage; no broad redesign is included here.
