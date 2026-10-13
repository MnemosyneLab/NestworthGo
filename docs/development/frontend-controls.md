# Shared frontend controls

This is the control-reuse contract referenced by `frontend/AGENTS.md`; the
engineering guide remains authoritative for architecture and financial rules.

## Discover and reuse

Search `src/components/ui`, `src/components/layout` and neighboring features
before writing UI. Use DatePicker/TimePicker for date/time entry, NativeSelect
or FilterableSelect for choices, Button/Input/Label/FieldError for forms,
Sheet for panels, AlertDialog for confirmations, Popover for anchored content,
and PageChrome/PageState for page framing and loading/empty/error states.
Extend these wrappers before adding a new primitive dependency or local clone.

## Dates and forms

Keep civil days as validated `YYYY-MM-DD` strings; do not parse them as UTC
instants or round-trip them through `toISOString()`. The ledger timezone owns
"today", closed-day and transaction boundaries; callers supply those bounds.
Keep timestamp conversion in the existing time/domain helpers. `allowFuture`
controls calendar navigation, not business validation; explicit min/max own
allowed dates. Preserve empty strings versus null at each existing boundary,
clearability, disabled state, form errors and server-side validation. Explicit
future contract dates must remain reachable. Do not silently normalize invalid
dates or replace an intentionally empty selection with today.

Provide a localized accessible name, associate errors with aria-describedby,
and expose aria-invalid. Labels must target the actual interactive trigger.
Preserve refs and focus restoration when changing an input into a picker.

## Interaction and visual conventions

Reuse Sheet/AlertDialog/Popover focus and Escape behavior, including nested
popovers, finalFocus and pending-mutation close guards. Do not install competing
document Escape listeners or custom modal overlays. Test keyboard opening,
selection, dismissal and return focus for changed interactions.

Use NativeSelect for simple lists, FilterableSelect for searchable lists and
existing ChoiceGrid/SegmentedControl/Tabs for their established patterns.
Expose selected state semantically (aria-pressed/selected/checked as applicable),
not only with color. Keep loading, empty, unavailable, stale and error distinct;
reuse PageState when it fits and preserve retry actions and accessible status.
Use theme tokens (bg-card, text-muted-foreground, border-border, ring-ring),
shared button variants and nearby gap/padding conventions. Check narrow layouts,
long translated labels, light/dark themes and financial-value readability.

## Enforced boundaries and exceptions

ESLint rejects native temporal inputs, raw select/dialog elements, raw dialog
roles, direct Base UI/day-picker imports and direct Calendar imports outside
components/ui. For temporal types and dialog roles, quoted JSX values and
string-literal JSX expressions (single or double quotes) are checked equally.
This is a lightweight boundary, not constant evaluation: templates, computed
expressions, spreads and dynamic aliases require explicit review, including
supplied types, roles and imports.

Shared wrapper internals and test fixtures are excluded. Native checkboxes,
radios and file inputs remain valid (there is no canonical checkbox wrapper).
Calendar's native month/year dropdowns are internal accessible primitives.
ReturnCalendarTab is a financial data visualization, not a date-entry picker;
chart range presets and day/month navigation also remain appropriate. Specialized
interactive rows, chart buttons and read-only connection text areas may keep
semantic HTML with existing focus and theme styles. Any new exception needs a
narrow documented rationale and focused coverage, not a blanket lint disable.
