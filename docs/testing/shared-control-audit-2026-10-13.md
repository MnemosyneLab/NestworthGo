# Shared-control audit — 2026-10-13

Base: `041abd6` (latest fetched main). Dedicated branch:
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
  PageState conventions were reviewed. No competing production raw select,
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
PR59 (`feat/investment-review-reminders`) had no direct file overlap at audit
capture; its new panel/upcoming-action work must obey the shared-control guard.
Neither branch was edited, checked out, merged or cherry-picked.

## Verification

Results and platform limitations are recorded in the draft PR. Chromium checks
use real components/styles in a temporary isolated fixture, not Wails IPC or
real financial data. Historical Overview interaction also has a real-component
jsdom integration test; broader orchestration tests retain input stubs for
invalid-value injection. Native macOS WebKit, VoiceOver, magnification, native
select dropdown appearance and packaged-app smoke remain manual gates.
