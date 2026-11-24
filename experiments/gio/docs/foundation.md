# Gio desktop foundation and component preview

## Status and scope

The [Gio desktop preview](../README.md) implements the shell, session
preferences, shared editors and pickers, component preview and **Directory →
Institutions** pilot designed below. The multiline IME test lives under
Development. English, Simplified Chinese and Traditional Chinese are available
in the [localization package](../internal/i18n). The
[desktop migration roadmap](roadmap.md) tracks further workflows and the
evidence needed for Wails parity. Wails remains the production desktop.
[Shared Wails control contract](../../../docs/development/frontend-controls.md)

## Application shell

- The shell owns the window, sidebar groups, active route, page header,
  preferences, and a window-level overlay host. Routes use stable IDs. Their
  labels are resolved from the active language.
- Each page owns its draft values and persistent Gio widget instances. Frames
  read current translations and theme tokens from the shell and reuse the same
  widgets. Switching language or appearance preserves the active route,
  draft text, selected values, and editor selection.
- Shared controls own rendering, transient interaction state, and input
  events. Pages supply labels, options and constraints, handle accepted values,
  and own validation and submission. For a searchable select, the page owns the
  selected value and the control owns the current search session and active row.
- The component preview uses in-memory fixtures. The Institutions pilot calls
  the existing Go application services for reads and writes and reloads the
  authoritative result after a successful save. Financial validation and date
  boundaries remain in the existing Go layers. [Engineering guide](../../../docs/development/engineering-guide.md#financial-implementation-rules)

## Internationalization and appearance

Use stable message keys grouped by purpose, such as `nav.directory`,
`field.clear`, and `date.nextMonth`. Provide complete English and Simplified
Chinese catalogs for the first milestone, with English as the runtime fallback.
Catalog checks verify key coverage and matching interpolation parameters for
each enabled language. Labels, hints, empty states, field errors, and accessible
names use the same catalog service. New languages can be added through the
catalogs and language registry. Add Traditional Chinese and Japanese before
claiming Wails language coverage. [Current language resolver](../../../internal/settings/language.go)

Keep translated messages separate from value formatting. A locale change can
change the displayed date, weekday, currency, or time label while preserving
the underlying selection. Week start, date format, and 12/24-hour format use
explicit display preferences compatible with the existing settings. Money
formatting takes authoritative decimal amounts and currencies as input.

Define semantic theme tokens for window and sidebar surfaces, field and popup
surfaces, text, borders, accent, hover, focus, selection, error, and disabled
states. Share typography, spacing, and corner radius across the two palettes.
Resolve the active palette in the shell and adapt it to any Gio Material
controls in use. Keep session-only switching in the first milestone. The
settings integration reuses the existing persistence and `system`, `light`,
and `dark` preferences. Verify system appearance changes and the native
title bar separately from the themed Gio content. [Existing settings types](../../../internal/settings/settings.go) · [Gio theme](https://gioui.org/doc/architecture/theme)

## Shared controls and Wails parity

Build the first controls from Gio's editor, click targets, and scrolling lists.
Use a shared `Button`, `Field`, `Popover`, and option list to keep behavior
consistent. `Field` supplies the visual label, help text, and error treatment.
The actual interactive control carries the corresponding semantics. Evaluate
`gioui.org/x/component` for menu layout and overlay primitives with a focused
prototype before adding it as a dependency. The Nestworth API owns the
interaction contract regardless of the primitives used underneath. [Gio widget architecture](https://gioui.org/doc/architecture/widget) · [Gio extension components](https://pkg.go.dev/gioui.org/x/component)

| Control | Wails precedent | Proposed Gio behavior |
| --- | --- | --- |
| Text input and multiline text | `Input`, `Label`, `FieldError` | Wrap `widget.Editor` and share label, hint, disabled, read-only, and error styles. Preserve selection, clipboard, undo, and composition. Provide a UTF-8 byte counter and validation when the caller specifies a storage limit. |
| Search field | Search inputs built on `Input` | Reuse text input with a search icon and clear action. Report query changes. Callers own filtering, provider requests, and result states. |
| Select | `NativeSelect` | Compose a trigger and an anchored option list. Support a placeholder, stable option IDs, disabled choices, keyboard selection, and an explicit empty selection where allowed. Display the localized option label. |
| Searchable select | `FilterableSelect` | Reuse the search field and option list. Start a fresh query on open. Commit only an explicit selection, and keep the selected value when a search is dismissed. Keep keyboard highlight and committed selection distinct. |
| Date picker | `DatePickerControl`, `DatePicker`, `Calendar` | Build a month grid with keyboard day movement, month and year navigation, localized labels, and a clear action when allowed. Apply the caller's range constraints and week-start preference. |
| Time picker | `TimePicker` | Compose hour and minute lists in a shared popover. Preserve the Wails behavior of using `00` for the unselected part on the first choice and allow explicitly clearing optional values. |

The shell draws anchored popovers above page content and keeps them inside the
window as the anchor moves or available space changes. The host handles
outside press and dismissal. Escape closes the active popup and restores its
trigger focus. For a searchable select, Tab dismisses the query and continues
focus traversal. Removing or disabling the trigger closes its popup. During
IME composition, candidate navigation and confirmation take precedence over
popup shortcuts and form submission. [Wails picker and focus behavior](../../../docs/development/frontend-controls.md#interaction-and-visual-conventions) · [Gio input events](https://gioui.org/doc/architecture/input)

### Date and time values

Dates are validated civil days in `YYYY-MM-DD` form. For ledger fields, the
caller supplies today in the household timezone. Apply an explicit `min`
when supplied. An explicit `max` takes precedence over the default upper
bound of today, which callers can lift by allowing future dates. The user
commits a value by selecting a day. Calendar navigation and highlighting
today are separate actions. Invalid dates receive field feedback.

Times are validated `HH:mm` values, including midnight `00:00`. A locale or
12/24-hour display change preserves the canonical hour and minute. Callers
preserve their existing empty-string or null boundary and use the existing
application logic to combine date, time, and timezone into an instant. [Wails date and form contract](../../../docs/development/frontend-controls.md#dates-and-forms)

### Follow-on controls

Add tabs and segmented controls, checkboxes, confirmation dialogs, side
panels, page states, and tables when a pilot page needs them. Start from the
existing Wails behavior for each and extract common variants from real usage.

## Component preview page

Add **Development → Component Preview** at the bottom of the experimental
sidebar. Use the existing sidebar preferences to exercise the page in both
languages and both themes. Register the route only in the experiment.

The main example is a working synthetic form with institution name, notes,
standalone institution search, account type select, searchable institution
select, effective date, and effective time. A results panel shows search text,
committed values, and a submit counter so an accidental selection or submit is
visible. Put the results alongside the form when there is room and below it in
a narrow window. Include focused examples for validation errors, disabled and
read-only fields, no search matches, long option lists, and date bounds. The
form and examples use the same component implementations as the pilot page.

## Implementation and acceptance

1. **Shell and preferences.** Move the current experimental route and
   preference handling into the shared shell. Add the preview route and
   verify live language and appearance switching while a draft is open.
2. **Text, search, and overlay.** Implement the editor-backed fields and a
   shared popover. Validate native composition, clipboard, keyboard focus,
   dismissal, and window-edge positioning with a small form.
3. **Selects and temporal controls.** Add both select types and the date and
   time pickers. Verify empty values, disabled options, date boundaries,
   leap days, week starts, midnight, and locale changes. Use the preview to
   exercise a long list and a popup opened near the bottom of the window.
4. **Institutions pilot.** List, search, and edit institutions with existing
   Go services. Verify loading, empty, error, and successful-save behavior
   against isolated data. Use equivalent Gio and Wails work before drawing
   conclusions about startup, memory, or responsiveness.

Automated checks cover translation keys, formatting, value validation, and
control state transitions. Native macOS checks cover Tab and Shift-Tab order,
focus restoration, Pinyin candidate positioning and confirmation, option
selection with Return, and dismissal with Escape. Confirming a candidate must
preserve both the submit count and selected option. Repeat these checks in
narrow windows with long translated labels and both appearances.

The prototype's macOS accessibility tree contained only the native window
controls during inspection. Before migrating forms in bulk, verify VoiceOver
can navigate the shared controls and read their names, roles, values,
selection, and errors. If the first controls
still fail this check, investigate the macOS accessibility bridge in the pilot.
Report native results separately from automated or offscreen checks. Use the
existing `com.nestworth.dev` bundle and verified synthetic database and settings
paths for all native checks. [Isolated native development QA](../../../docs/development/local-workflow.md#isolated-native-development-qa)
