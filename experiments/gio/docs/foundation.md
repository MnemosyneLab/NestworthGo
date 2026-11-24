# Gio desktop foundation and component preview

## Status and scope

The Gio desktop runs against a verified synthetic household. It includes
General settings, Ledger activity, the Institutions pilot and a Component
Preview page for the shared controls. Wails remains the
production desktop. The roadmap records workflow coverage and acceptance
evidence for each stage. [Preview guide](../README.md) · [Migration roadmap](roadmap.md) · [Shared Wails control contract](../../../docs/development/frontend-controls.md)

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
  The controls live in their own [ui package](../internal/ui). The
  [desktop package](../internal/desktop) composes them into pages and the shell.
- The component preview uses in-memory fixtures. The Institutions pilot calls
  the existing Go application services for reads and writes and reloads the
  authoritative result after a successful save. Financial validation and date
  boundaries remain in the existing Go layers. [Engineering guide](../../../docs/development/engineering-guide.md#financial-implementation-rules)

## Internationalization and appearance

English, Simplified Chinese, Traditional Chinese and Japanese have complete
message catalogs. Labels, hints, empty states, errors and accessible names use
the same typed keys. Catalog checks verify coverage and interpolation
parameters. The System language option uses the same locale resolver as Wails.
[Gio message catalogs](../internal/i18n) · [Shared language resolver](../../../internal/settings/language.go)

Keep translated messages separate from value formatting. A locale change can
change the displayed date, weekday, currency, or time label while preserving
the underlying selection. Week start, date format, and 12/24-hour format use
explicit display preferences compatible with the existing settings. Money
formatting takes authoritative decimal amounts and currencies as input.

The shell resolves system, light or dark appearance and applies one of the five
Wails accent palettes. Buttons, fields and popovers share visual tokens for
surfaces, text, borders, focus, selection and errors. The sidebar language
menu and appearance button stay available in the collapsed rail and save
immediately. Accent selection and the System language option live in Settings.
[Wails color tokens](../../../frontend/src/index.css) · [Shared settings types](../../../internal/settings/settings.go)

### General settings and storage

The Appearance tab groups eight display preferences in two columns and stacks
them vertically at compact widths. The household base currency is read-only.
The timezone picker supports searching named zones and following the system.
Its help text distinguishes the current display zone from the zone attached
to the household's recorded history.

The page keeps edits as a draft, validates the separators, and offers save
and discard. The confirmation dialog restores only the eight General fields.
The shell serializes writes and publishes a change after persistence
succeeds. A failed save keeps the draft available for retry.

The preferences adapter merges edited fields into the latest shared settings
under the application's write permit. Timezone uses the existing SQLite
configuration, and presentation preferences use the settings file. Both are
scoped to the verified preview run. The adapter obtains currency and history
origin from the existing application services. [Preferences adapter](../internal/preferences/service.go)

## Shared controls and Wails parity

The shared controls use Gio's editor, click targets and scrolling lists.
Page code supplies labels, help text and validation errors. The control
registers input and semantic operations and draws the corresponding visual
state. The Settings timezone picker and the component preview's institution
picker use the same searchable select. [Shared controls](../internal/ui) · [Gio widget architecture](https://gioui.org/doc/architecture/widget)

| Control | Wails precedent | Interaction contract |
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

General settings owns its reset dialog. Extract a shared confirmation
dialog when another workflow needs the same behavior. Add segmented
controls, checkboxes, side panels and tables as the directory and ledger
workflows require them.

## Component preview page

Open **Manage → Component Preview** from the sidebar to exercise controls in all four
languages and both appearances. Use the sidebar for live changes and Settings
for the accent palette.

The main example is a working synthetic form with institution name, notes,
standalone institution search, account type select, searchable institution
select, effective date, and effective time. A results panel shows search text,
committed values, and a submit counter so an accidental selection or submit is
visible. Put the results alongside the form when there is room and below it in
a narrow window. Include focused examples for validation errors, disabled and
read-only fields, no search matches, long option lists, and date bounds. The
form and examples use the same component implementations as the pilot page.

## Interaction acceptance

Use the multiline **Notes** field in Component Preview to check native IME
candidate placement, confirmation and editing across lines. Use the rest of
the form to exercise clipboard, focus, dismissal, empty values, disabled
options, leap days, midnight and popup
position near the edge of a narrow window. Check language and appearance
switches while a draft or selection is active. Use the actual Settings and
Institutions routes to exercise saving, retrying and reopening the same
isolated fixture. The roadmap records the evidence available for each stage.

Automated checks cover translation keys, formatting, value validation, and
control state transitions. On macOS, verify Tab and Shift-Tab order,
focus restoration, Pinyin candidate positioning and confirmation, option
selection with Return, and dismissal with Escape. Confirming a candidate must
preserve both the submit count and selected option. Repeat these checks in
narrow windows with long translated labels and both appearances.

The prototype's macOS accessibility tree contained only the native window
controls during inspection. Before migrating forms in bulk, verify VoiceOver
can navigate the shared controls and read their names, roles, values,
selection, and errors. If the first controls
still fail this check, investigate the macOS accessibility bridge in the pilot.
Report native results separately from automated or offscreen checks. Use
`com.nestworth.gio` and its verified synthetic profile for native Gio checks.
Use a separate fixture root for scenarios that need a clean ledger.
[Gio test profiles](../README.md#persistent-test-data)
