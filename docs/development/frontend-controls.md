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

## Confirmation recovery

Repeat and cash-balance confirmations register their immutable plan identity
and reviewed payload with the owning Sheet before dispatch. While confirmation
is pending or its outcome is uncertain, the Sheet guards Escape, Close and
outside presses, and application navigation and failed background reads preserve the mounted review.
Fields and Back to draft remain disabled; retry uses the same plan and receipt.
Success closes once. A proven backend refusal releases the guard and permits
a fresh preview or draft cancellation. `stale_preview` with `previewToken`
means a proven refusal; `planId` means the in-memory plan is unavailable and
cannot prove whether an earlier confirmation committed. No-field legacy
`stale_preview` responses are treated conservatively as unavailable plans.
Reconciliation session commits additionally return `stale_preview` with
`expiresAt` only after durable receipt lookup proves an expired preview did not
commit. This definite expiry releases the guard for dismissal and fresh review;
`planId` and unscoped stale errors retain the original recovery identity.
Product correction commits likewise check durable receipts before refusing an
expired/invalidated `previewToken` or changed `reviewedStateHash`, before any
write. Reconciliation product reviews release their guard for these named stale
fields, permitting dismissal and a fresh preview. Receipt conflicts remain unknown,
and retries of committed products still return the receipt before checking expiry.

Ordinary Record and Fix submissions also register with the owning Sheet
synchronously before dispatch. The owner retains the immutable reviewed command,
original client mutation ID, Record/Fix family, Fix source Activity, reviewed
resulting values, household and QueryClient session. Pending and unknown outcomes
block edits, new previews, dismissal, application navigation and target
replacement, including the account-action deposit/withdraw toggle. A surviving
Sheet restores this submission after a child form remount. Failed background
reads preserve the owner and review.

| Outcome | In-session behavior |
| --- | --- |
| Pending | Retain identity and review; prevent a second dispatch |
| Unknown | Keep the guard; offer explicit **Retry same submission** |
| Confirmed success | Resolve only the current owner/ID and notify/close once |
| Proven precommit refusal | Release the guard and return to preview/edit |
| Changed household, session or Fix source | Keep recovery; block dispatch and stale completion |

Retry sends the saved request and mutation ID, plus the original Fix source;
it never reconstructs a request from refreshed defaults. Record's audited
business refusals (`invalid_change`, `no_change`, `insufficient_balance`,
`insufficient_quantity`, `managed_position_conflict`) precede its atomic write
and follow durable receipt lookup. Named Record conflicts on reconciliationLink,
financialRevision or expectedRevision also follow that receipt lookup and prove
a precommit refusal; refresh reconciliation evidence before renewed review.
A mutationId/operationId conflict or an unscoped receipt conflict remains unknown. Other errors remain unknown. Fix commits its
inverse/replacement before snapshot maintenance, and same-ID replay reruns that
maintenance. Generic domain errors such as validation, conflict, decimal overflow
and history errors therefore remain unknown for Fix; only `cannot_fix_change`,
which originates in its precommit correction plan, releases its guard. A
mutation-ID conflict is not proof that no prior write exists.

This scope covers History Record/Fix and account-action Record forms (deposit,
withdraw, FX, transfer, buy, sell, dividend and history-started value updates).
Pre-history direct value writes, position import, Undo and other mutation
families keep their existing behavior. The backend request hash, schema and MCP
plan/receipt behavior are unchanged; the frontend binds family and Fix source
separately because they are absent from the ordinary request hash.

This is in-session recovery, not persistence across process restart. Desktop
plans are held in memory; if a plan has actually disappeared, retry cannot
resolve its receipt through the current desktop API. Keep the review guarded
rather than silently assigning a new identity. Native window/process close and
restart recovery require separate backend/product work and acceptance.

## Financial display and identities

Ordinary money (including JPY/KRW) uses two display decimals through
`formatAmount(value, currency)`; use `formatMoney` for currencyless money.
`currencyFractionDigits` remains the ISO command-calculation scale. Never round
DTOs, persisted decimals, inputs, or submitted/retried payloads for presentation.
`formatQuotePrice` preserves unit-price/NAV precision; currencyless
`formatAmount` preserves quantities and FX rates. Charts use canonical strings
for tooltips/tables and numbers only for geometry. Known zero is money, missing
is unavailable, and a known subtotal is never a fabricated full total. Rounded
nonzero sub-cent values carry a direction/size hint without a signed zero;
explicit exact audit/confirmation detail retains the canonical value.

GUI labels use business instrument/account/source/date information. UUIDs,
observation IDs, internal row keys and protocol/version tokens stay in backend,
MCP and internal control values. Unknown metadata uses localized unavailable
copy. Exact collisions use business metadata or stable session ordinals; never
merge identities or expose UUID suffixes. Historical warning tooltips are
bounded summaries; a keyboard-accessible disclosure retains the full list.

Product ownership in the enriched GUI inventory comes from every ProductContract
state. Ordinary management and mutation consumers use `ordinaryInstrument`;
wealth/history/analytics keep the complete inventory. Failed ownership reads
must not enable ordinary actions, and preselected IDs must be validated before
preview or submission. Backend product guards remain authoritative.

Holding detail hosts mount the shared `HoldingDetailPanel` only for expanded,
active rows. Query keys share instrument price history/performance, while money
uses the correct account/aggregate scope and one cost replay. No per-row polling,
provider refresh or investment-review state writes. Descriptive 7/30-day metrics
end at the latest real valid observation; use the nearest baseline on/before
the target, actual civil days, and explicit dates. Insufficient history hides
that metric. Performance accepts comparable dated daily Agent, manual and provider
facts independently of the saved source preference. Source identity and valid
date-only/session/timestamp encodings do not change economic comparability;
realtime observations remain excluded. Unknown/adjusted price bases, mismatched
currency/binding and unsafe corporate actions remain unavailable. When no window
qualifies, show a localized reason rather than silently removing the section.
Both account and portfolio rows use HoldingDisclosure: canonical ghost icon
Button, Lucide chevrons, horizontal instrument label, localized expand/collapse
names, aria-expanded/controls and independent sibling action menus. Details put
scoped cost/value/unrealized gain first, then performance/chart, compact metadata
and a collapsed full-text note disclosure. Do not inherit alert freshness or
baseline-gap cutoffs, fabricate prices, or infer returns from whole-position
values or income-distributing money-fund NAV without income evidence. Flat
ordinary unit NAV can show 0% NAV change; never infer money-fund status from
flatness. Derived percentages use concise two-decimal presentation; signed
money uses `formatSignedMoney`, including neutral rounded zero and its tiny-value
hint.

NativeSelect and FilterableSelect share `selectFieldClass`, retaining native
select semantics. A combobox search session starts empty, commits only an
explicit option, and cancels on Escape/Tab/blur. IME composition never confirms
an option or submits the outer form. Preserve disabled, error and active-option
accessibility semantics; test reopening while already focused.
