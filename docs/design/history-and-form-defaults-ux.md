# History and Related-Form Defaults UX

- Owner: Design and Frontend
- Status: **Implemented**
- Baseline: Nestworth-go `0.3.1` / SQLite schema `9` / Wails v3 `v3.0.0-beta.16`
- Surfaces: History, Record change, Account action sheets, Start History, Settings, cash reconciliation, simple values, existing positions, and activity detail
- Companions: [Domain Model](../architecture/domain-model.md), [Data and Application Contracts](../architecture/data-and-ipc-contracts.md), and [Account Container Interaction](account-container-interaction.md)

This document defines current user-visible form behavior. Go application services
remain authoritative for validation, financial calculations, timestamps, and
persisted facts. The frontend owns labels, field state, accessibility, and
editable defaults; it never invents authoritative totals.

## 1. Shared form rules

### 1.1 Default versus user override

Auto-filled amounts are defaults, not locks. Each form retains the last canonical
value it computed:

- An empty field, or a field that still equals the last computed value, may be
  recomputed when its source inputs change.
- A value typed by the user remains unchanged when a quote or unrelated field
  changes.
- Clearing an amount allows the next valid calculation to fill it.
- A form opened for Fix treats existing amounts as user values on mount. Loading
  a quote must not replace them; a later user change to quantity, instrument, or
  currency may calculate a new default.
- Successful mutations invalidate related queries and reload authoritative DTOs;
  they do not optimistically construct totals.

### 1.2 Exact decimal strings

Money, quantity, unit price, and FX values stay canonical decimal strings in form
state. The UI does not use JavaScript `Number` for financial arithmetic. Shared
string-decimal helpers provide multiplication and division with the domain's
currency or quantity scale and midpoint-nearest-even rounding. Invalid or empty
inputs do not produce a default. Preview and Record always perform final
validation in Go.

### 1.3 Quote hints and explicit refresh

A `QuoteHint` appears beside fields that depend on a market quote:

| State | User-visible behavior |
| --- | --- |
| Loading | Show the existing pending state; do not calculate yet |
| Quote available | Show value, source, delayed state, and quoted-at time |
| No quote | Show `marketData.noQuoteYet` and an `Update` action when the source is a provider |
| Refresh failed | Keep the form input and show a retryable error |
| Manual source without a quote | Explain that the user must enter/save a manual price on the relevant market-data surface |

Opening a form never starts a provider request. `Update` is an explicit action
for the selected Instrument or FX pair. The form uses the loaded quote for a
default only; it does not replace user-entered amounts.

### 1.4 Locked currencies

Amounts may be edited, but currencies constrained by the selected account or
Instrument are not free text. The UI hides or disables those currency selectors
and resets them when the related account or Instrument changes.

| Field | Required currency |
| --- | --- |
| Simple value or balance update | Selected Account `defaultCurrency` |
| Debt principal, interest, and fee | Selected cash Account `defaultCurrency` |
| FX fee | Sold currency |
| Trade gross and fee | Selected Instrument `quoteCurrency` |
| Money in/out and cash-transfer amount | A supported selected cash currency |

The UI must not leave a form combination that Preview will always reject.

### 1.5 Context-rich choices

Holding choices use one shared label:

```text
{instrumentName} · {accountName} · {formatted quantity}
```

Sell lists only non-zero holdings in the selected settlement Account. Buy lists
non-archived Instruments and may create an Instrument. Debt lists only liability
Accounts for the debt endpoint and non-liability `balance` or `holdings` Accounts
for the cash endpoint. `other` is filtered by role, not by its name.

## 2. Account and onboarding defaults

Onboarding creates ordinary editable defaults when no active record exists:

- a default Institution with type `other`;
- a default Group;
- the names are localized at creation, not special entity types.

New Account creation preselects the first unarchived Institution and Group by
stable sort order, while allowing the user to change either or create a new
record. Existing records are not duplicated. Account creation exposes only the
supported net-worth inclusion control. `include_in_portfolio` and
`include_in_liquid_assets` remain persisted compatibility fields and are not
user-facing controls; the live Portfolio metric ignores both.

Holdings enter Portfolio when they are active, Instrument-backed components of
active asset-role Accounts. Cash, liabilities, archived Holdings, and Simple
Account values do not. Account detail still displays cash independently.

## 3. Current form behavior

### 3.1 Trade: quantity and gross

Buy and Sell show the selected Instrument's quote currency and current price.
The user may enter either quantity or gross total and may edit both. `Calculate`
fills only the empty or zero side:

- quantity plus a valid price fills `gross = quantity × unitPrice`;
- gross plus a valid non-zero price fills `quantity = gross ÷ unitPrice`;
- two positive sides are never overwritten;
- no price leaves the fields unchanged and asks the user to update the quote or
  enter a manual price.

Fee is optional and remains independent. The command submits quantity and gross;
the displayed price is not a separate financial fact. Sell still requires a
positive holding in the settlement Account. A first Buy creates the Holding and
updates cash and quantity in one successful operation.

### 3.2 FX conversion

Sold and bought currencies must both be present and different. The bought
currency starts empty rather than silently matching the sold currency. The form
shows the direct selected-pair quote and its as-of time; it does not synthesize
USD/EUR from two quotes against a CNY base.

Both amounts are editable. `Calculate` follows one rule:

| Input state | Result |
| --- | --- |
| Exactly one positive side | Fill the empty or zero side using the visible direct rate |
| Both sides positive | Leave both values unchanged |
| Both sides empty or zero | Show an actionable message; change nothing |
| Quote unavailable | Show an actionable update message; change nothing |

Changing a currency or refreshing a quote never changes amounts automatically.
The optional fee is locked to the sold currency and is visible during Fix. The
form is not previewable until the currencies differ and both required amounts
are valid.

Any two supported currencies may be a direct provider or manual pair. A missing
preference can be configured as the selected global provider by the explicit
Update action; no network request occurs merely because the form opened.

### 3.3 Cash transfers

After both Accounts are selected, the form resolves their cash currencies.
For a same-currency transfer, the sent amount is editable and the received
amount is a read-only mirror with the same currency. There is no second editable
field and no calculation button. An optional fee is in the sent currency.

For a cross-currency transfer, both amounts are editable, the two currencies are
locked to their endpoints, and the visible pair quote supports the same
Calculate rule as FX: only an empty or zero side is filled; two positive sides
are not overwritten; both positive sides are required for Preview and Record.
Missing quotes remain an actionable incomplete state.

### 3.4 Value, cash, debt, and position forms

- Simple value and balance forms show the current authoritative amount and
  currency. The value form prefills the current total, locks its currency, and
  disables Preview/Save while the canonical new value is unchanged. The helper
  asks for a different value. Cash reconciliation displays the current selected
  currency but leaves the resulting amount empty so the user deliberately enters
  it.
- Account sheets filter value updates to `balance` and `manual_value` Accounts.
  Holdings Accounts cannot be selected for a Simple value update.
- Record existing position is separate from Buy. It requires a positive quantity,
  does not reduce cash, and may default unit cost from the visible current quote.
  The user can edit the cost; no quote does not fabricate one.
- Position adjustment uses an explicit Added/Removed radio group. One choice is
  always selected, and unit cost is shown only for Added.
- Debt forms filter the debt endpoint to liability Accounts and the cash endpoint
  to non-liability `balance` or `holdings` Accounts. An empty filtered list
  disables Preview and explains why.

### 3.5 Native amounts when base FX is missing

A missing native-to-household FX quote removes only the base-currency valuation.
Account lists, Account detail, and Holdings detail continue to show the native
amount and identify it as awaiting conversion or missing FX. The UI never
replaces a visible native amount with an empty value or “no value,” and never
uses zero as a substitute.

## 4. Timezone, date, and time behavior

Settings owns the presentation timezone and the selected FX provider. The
Settings timezone is `system` or a valid IANA zone chosen through a filterable
keyboard-accessible select. `system` resolves to the browser/system IANA zone.
Ordinary display timestamps, including quote times, use this presentation zone.

History Origin owns the ledger timezone after History starts. Start History shows
the resolved Settings timezone and start date read-only; it does not edit an
existing Origin. History displays the Origin timezone, and Record change resolves
its local date and time in that zone. Changing Settings later changes ordinary
presentation timestamps but never rewrites historical local dates.

Record change uses a calendar popover and a separate 24-hour time picker:

- Date uses the shadcn/Base UI calendar with `react-day-picker` and the Settings
  date format; the submitted value is `YYYY-MM-DD`.
- Time uses hour `00–23` and minute `00–59`; it never uses the operating system's
  12-hour preference or a native `time` input.
- A new Record defaults to the current time in the Origin timezone and rejects
  dates before the Origin or after now.
- Fix shows the original effective timestamp read-only and submits no original
  timestamp. The reversal and replacement occur at now; copying the old time
  would rewrite historical replay before the correction happened.
- Cash reconciliation and Simple value updates retain their current now-based
  behavior.

## 5. Market-data settings and refresh

The global quote-cache TTL is selectable as `1h`, `3h`, `12h`, or `24h`, with a
default of `12h`. The same TTL drives freshness labels and the decision to skip a
provider request. It never blocks a forced refresh and does not apply to manual
quotes.

Market Data exposes two explicit actions:

- **Refresh missing or stale** requests only valuation-required provider
  Instruments and FX pairs that have no local quote or are at least as old as
  the TTL. Required FX pairs use the selected global FX provider even when a
  per-pair preference has not previously been stored.
- **Force refresh all** ignores the TTL and requests all saved provider targets,
  running valuation-required FX before extra saved pairs so rate limits cannot
  starve required inputs.

Manual sources are never sent to a provider. Switching the global provider
changes selection for future provider-sourced FX reads and refreshes; it does
not rewrite manual facts or silently claim that an old vendor quote came from
the new vendor.

## 6. History presentation and activity detail

History and Overview use the same localized `activitySentence`. Trade sentences
include the settlement Account, Instrument, direction, quantity, gross total,
and fee when present. Activity detail is read-only and exposes the complete
user-visible fields for trades, transfers, FX, value updates, debt, notes, local
date/time, and timezone. Reversal and correction relationships are marked
without changing the immutable activity facts.

Value-update sentences name the resulting value, not just the delta. For
example, a change from 300 to 320 says “updated to 320” or “increased by 20,
updated to 320”; it must never say “updated to 20.”

## 7. Analytics and state handling

Trend data combines closed-day snapshots with the current live point. If History
has no closed day yet, the UI explains that a trend begins after the next closed
day rather than drawing a misleading single-point line. When the current day is
also the last snapshot day, it is not duplicated. Snapshot rebuild failures are
visible with a retry action and do not silently collapse the chart to a fake
zero or one point.

Every query-backed page and sheet covers loading, empty, partial, unavailable,
and error states. Missing quote/FX inputs remain explicit. Failed mutations
preserve the user's input and field errors. Submit controls have a real
in-progress state and reject duplicate activation. After a successful mutation,
related Account, Overview, Portfolio, and History queries reload from Go.

Interactive controls are keyboard-complete, have localized labels, expose focus
and selected/checked semantics, and do not use color as the only status code.
Opening a sheet moves focus to its title or first field; closing returns focus
to the trigger. Destructive archive actions have distinct visual weight from
Buy, Update, and other ordinary actions.

## 8. Validation boundary

This document owns user-visible flow and field state. The [Domain Model](../architecture/domain-model.md)
owns accounting meaning, classification, precision, and immutable Activity
rules. [Data and Application Contracts](../architecture/data-and-ipc-contracts.md)
owns persistence, provider refresh, DTO, recovery, and error boundaries. When
copy and implementation disagree, current code and tests determine the
implemented behavior; this document is updated to describe that behavior.
