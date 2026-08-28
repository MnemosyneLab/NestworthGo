# History and related-form defaults UX

- Status: **Implemented** (see also [gap review](history-and-form-defaults-ux-gap-review.md) for follow-up fixes after `e2687aa`).
- Baseline: Nestworth-go `0.2.1` / schema v8 / current Wails frontend
- Surfaces: History Record change, Account action sheets that reuse that form, Start History, Settings, cash reconcile, simple value, record existing position, timeline sentences
- Companion: [Account Container Interaction](../architecture/account-container-interaction-design.md)
- Superseded in part by [Trial UX optimization](trial-ux-optimization.md): FX, trade gross, and transfer amounts use a Calculate button instead of live auto-fill; native date/time inputs are replaced by shadcn Calendar (react-day-picker on Base UI Popover) plus a 24-hour hour/minute picker. Quote hints, locked currencies, and Fix time-as-read-only still apply.

This document is the experience plan for recording changes: auto-filled defaults the user can edit, live price/FX preview, closed-vocabulary selects, and honest current-balance echo. It covers the two original requests (trade quantity → gross, FX one-side → other-side) and the fifteen follow-up items.

## 1. Goals and non-goals

### Goals

1. The user should rarely type a number the app already knows (price × quantity, FX rate, same-currency transfer, current balance, latest unit cost).
2. Every auto-filled **amount** is a **default**, not a lock. Editing it must stick until the inputs that produced it change. Currencies the domain requires to match an account or instrument are **locked** (§2.5), not free overrides.
3. Quotes and rates are visible next to the fields that depend on them. Missing market data is explicit and recoverable with one user-triggered refresh.
4. Selects, not free text, for closed vocabularies (timezone, currency, accounts, holdings, side).
5. Account and holding pickers show enough context to tell two similar rows apart.
6. Settings owns household-wide presentation: timezone and FX vendor. Start History stops asking for timezone. The History page always shows the timezone history is using.
7. Any supported currency pair (including USD↔EUR when household base is CNY) is a first-class FX quote: query the provider directly and persist the pair. Do not synthesize a rate from two legs against base.

### Non-goals

- Background or startup provider refresh. Refresh stays explicit and user-triggered.
- Frontend recomputation as financial authority. Auto-fill uses string-decimal math for **defaults only**. Preview/Record still validates in Go.
- Changing History Origin timezone after history has started (that would rewrite `effectiveLocalDate` on past activities). Settings timezone is the editor **before** start; origin stores a snapshot.
- History rewrite: Fix must not move a replacement onto the original activity’s `effectiveAt`. Replay filters `effective_at <= cutoff` and ignores `created_at` ([`historical_replay.go`](../../internal/application/historical_replay.go), [`ListActivitiesUntil`](../../internal/infrastructure/sqlite/activity_repository.go)).
- Yahoo as an FX vendor. Yahoo is instrument-only. The FX select lists providers with `LatestFX` (today: Frankfurter only).
- A new datetime library. Native date/time inputs plus Go `ResolveLocalDateTime` for DST-safe conversion.
- Optimistic valuation totals after record. Existing invalidation/reload stays.
- Putting extra (non-valuation) FX pairs into `RefreshRequiredFX`. That path stays snapshot-required vs-base only.

## Review corrections (accepted)

Static review of this plan (2026-08-28). All items below are accepted and override earlier wording in the same file.

| ID | Decision |
| --- | --- |
| P0 Fix time | Fix shows the original `effectiveAt` as **read-only**. Replacement (and its reversal) keep **now**. Do not prefill or submit the original timestamp. |
| P1 ToCommand TZ | `HistoryService` loads Origin timezone and passes it into `ToCommand` for Preview, Record, PreviewFix, and Fix. Define field precedence and half-filled / out-of-range errors (§4.15). |
| P1 Settings TZ | Settings timezone **is** the global presentation zone. Wire it into ordinary UI timestamps (Market Data / Investments `quotedAt`, etc.), not only Start History. History **ledger** dates stay on Origin. Bindings fields are `timezone` and `fx_provider`. |
| P1 FX vendor | `CurrentFXQuote` / valuation must select provider quotes by **current** `FXProviderKey` (`source_key`), not only `sourceKind`. Market Data must not hardcode “Frankfurter”. |
| P1 Value no-op | Prefill current value, but disable Preview/Save while unchanged; show “enter a different new value”. |
| P1 Locked ccy | Value-update, debt principal, FX fee, trade gross/fee currencies are locked to the domain constraint. Switching account/instrument resets or locks; do not keep a combination the backend will reject. |
| P2 FX empty + RL | Empty bought option, `useCurrentFXQuote` guard `a !== b`, Preview disabled until a real pair. `RefreshAll` runs valuation-required FX **before** extra saved pairs so a rate limit on extras cannot skip required pairs. |

## 2. Shared rules

These rules apply to every auto-fill in this plan.

### 2.1 Default vs override

Keep the last auto-computed canonical string.

- If the field is empty, or still equals that last auto value, overwrite when the source inputs change.
- If the user typed a different value, keep it.
- Clearing the field restores auto-fill.
- **Fix** (pre-filled `initial`): treat existing amounts as overrides on mount. Do not replace them when a quote loads. Auto-fill only after the user changes a source input (quantity, instrument, currency, sold/bought).

### 2.2 Decimal strings

Do not use JavaScript `Number` for money, quantity, price, or FX.

Add canonical helpers next to [`frontend/src/lib/money.ts`](../../frontend/src/lib/money.ts):

- `multiplyCanonical(a, b, fractionDigits)`
- `divideCanonical(a, b, fractionDigits)`

Reuse the existing banker's rounding already used by `formatAmount`. Round money defaults to `currencyFractionDigits` (JPY/KRW 0, others 2). Invalid or empty inputs yield no default.

### 2.3 Quote hint

A small `QuoteHint` under the relevant fields:

| State | Copy | Action |
| --- | --- | --- |
| Loading | existing pending copy | none |
| Has quote | `portfolio.latestPrice` / `marketData.latestRate` + `quotedAsOf` | none |
| No quote | `marketData.noQuoteYet` | **Update** button (see §4.3) |
| Refresh failed | existing error display | retry via the same button |

Reuse Market Data / Investments strings. Do not invent a second price vocabulary.

### 2.4 Account currency default

When the user selects an account (or the form is locked to one), set the **amount** currency from that account per §2.5. Household `baseCurrency` is the fallback only when the account has no `defaultCurrency`.

Applies as the **source** of the locked or defaulted currency, not as a free second picker the user can desync.

### 2.5 Locked currencies vs editable amounts

“Default you can edit” applies to **amounts** (gross, sold, bought, received, new value, unit cost). These **currencies are not free text** once the related account or instrument is known — the domain will reject a mismatch:

| Field | Must equal | When known |
| --- | --- | --- |
| Value update `newValueCurrency` | Account `defaultCurrency` | Account selected |
| Debt `principalCurrency` (and interest/fee) | Cash account `defaultCurrency` | Cash account selected |
| FX `feeCurrency` | Sold currency | Sold currency selected |
| Trade `grossCurrency` / `feeCurrency` | Instrument `quoteCurrency` | Instrument selected |

**UI:** hide the currency `<select>` or render it `disabled` with the required code. Changing account/instrument **replaces** the currency (and, if the amount was an auto-default, may recompute; if the user had typed an amount, keep the digits but not a stale currency). Money in/out and cash-transfer currencies stay selectable (multi-currency cash is allowed).

Do not leave a combination that Preview will always reject.

## 3. Shared building blocks

### 3.1 Filterable select

There is no combobox today, only `NativeSelect`. Timezone lists are too long for a raw `<select>`.

Add [`frontend/src/components/ui/filterable-select.tsx`](../../frontend/src/components/ui/filterable-select.tsx):

- Text input filters options (case-insensitive match on value and label).
- Listbox of filtered options; keyboard: Arrow, Enter, Escape, type-ahead.
- Empty filter result: localized “No matches”.
- Must be keyboard-only usable (existing `keyboardOnly` coverage).

Use it for **timezone**. Other long lists (instruments, holdings) can keep `NativeSelect` in this plan; richer **labels** are the fix there, not typeahead.

### 3.2 Quote queries on the form

[`RecordChangeForm`](../../frontend/src/features/history/RecordChangeForm.tsx) already has React Query. Wire:

- `useCurrentInstrumentQuote(instrumentId)` for trade and record-position
- `useCurrentFXQuote(soldCurrency, boughtCurrency)` **enabled only when** both codes are non-empty **and** `soldCurrency !== boughtCurrency` (hook + queryFn; do not call Go with a same-currency pair)
- new `useRefreshInstrument` / `useRefreshFX` wrapping `MarketDataService.RefreshInstrument` / `RefreshFX` (bindings exist; the Record change form does not call them)

### 3.3 Holding option label

One formatter, used everywhere a holding is chosen:

`{instrumentName} · {accountName} · {formatted quantity}`

Unknown instrument → existing `portfolio.unknownInstrument`.

---

## 4. Item plans

Each item: problem, current behavior, target, implementation.

### 4.1 Trade: quantity × latest price → gross default

**Problem.** Buy/sell asks for quantity and gross independently. After entering quantity, the total should default from the latest price and remain editable.

**Current.** [`RecordChangeForm`](../../frontend/src/features/history/RecordChangeForm.tsx) trade block: settlement (holdings accounts), full instrument catalog, side, quantity, gross, optional fee. No quote fetch. No unit-price display.

**Target.**

1. Selecting an instrument shows latest price (or `noQuoteYet` + Update).
2. Entering quantity fills **Gross total** = quantity × `unitPrice`, currency = quote/instrument currency.
3. User can edit gross. Changing quantity or instrument recomputes only per §2.1.
4. Fee stays independent (not auto-derived).

**Implementation.**

- On instrument change: set `grossCurrency` / `feeCurrency` from `instrument.quoteCurrency` and **lock** those selects (§2.5).
- `useEffect` (or a small helper) when `quantity`, `quote.unitPrice`, and currency are valid: `patch({ gross })` if not overridden.
- Show `QuoteHint` under quantity/gross.
- Fix flow: `initial.gross` is an override until quantity/instrument changes. Do **not** copy `initial.effectiveAt` into the replacement (§4.15).

Command shape unchanged: still submit `quantity` + `gross`. Price is display/default only.

### 4.2 FX: fill one side → default the other; show rate

**Problem.** Sold and bought are independent. Filling one side should compute the other from the latest rate. The rate itself is not shown.

**Current.** Two `MoneyFields`. `emptyChangeRequest` sets **both** currencies to household base. No `useCurrentFXQuote`. Fee omitted (see §4.7).

**Target.**

- Show `1 {base} = {rate} {quote}` and as-of time.
- Editing **sold** (amount or currency) fills **bought** when bought is empty or still auto.
- Editing **bought** fills **sold** the same way.
- Same-currency pair is forbidden (§4.6). No convert, no rate query.

**Rate orientation.** `CurrentFXQuote` returns a stored quote: `1 baseCurrency = rate quoteCurrency`. Convert with that direction or the inverse via `multiplyCanonical` / `divideCanonical`. Do not assume A→B equals the function argument order.

**Any supported pair is direct.** Today `SetFXPreference` and `AppendManualFXQuote` reject pairs that do not include household base (`currency pair must include the Household base currency`). Drop that rule. USD↔EUR with a CNY household is the same as USD↔CNY: one preference, one provider `LatestFX` call, one persisted `FXQuote`.

Valuation can keep converting holdings/cash to household base via native→base quotes. Cross-base pairs are extra stored facts for conversion (and Market Data), not a replacement for valuation routing.

**Implementation.**

- Form calls `useCurrentFXQuote(sold, bought)` only when both currencies exist and differ — including neither equal to base.
- `MoneyFields` for bought (and any optional currency) must support an empty option (`value=""` + `history.selectEmpty`). `emptyChangeRequest` leaves `boughtCurrency` empty.
- Last-auto strings for sold and bought independently. Currency changes that remain a valid pair recompute the non-overridden **amount**.
- Preview disabled until sold and bought currencies exist and differ.
- Update / first use of a new pair: `SetFXPreference(sold, bought, "provider")` then `RefreshFX(sold, bought)`, which writes the quote (see §4.3).

### 4.3 No quote: `marketData.noQuoteYet` + Update

**Problem.** Missing price/rate is silent. The user has no in-form way to fetch it.

**Current.** Market Data can `RefreshAll` / `RefreshRequiredFX`. `RefreshFX(a,b)` and `RefreshInstrument(id)` exist in Go but have no form hooks. `RefreshFX` **skips** (`ErrNotFound`) if the pair is not in the current snapshot’s required set — a pair chosen only on this form would not refresh.

**Target.** On `noQuoteYet`, show an **Update** button. Click fetches market data for **this** instrument or **this** pair, then the hint and auto-fill update.

**Implementation.**

1. **Drop the household-base pair restriction** on `SetFXPreference` and `AppendManualFXQuote` / `SaveManualFXQuote` in [`internal/application/portfolio.go`](../../internal/application/portfolio.go). Any two different **supported** currencies are a valid pair. Schema already keys preferences and quotes by canonical unordered pair (`NormalizeFXPair`); no migration.
2. **Relax `RefreshFX`** in [`internal/application/refresh.go`](../../internal/application/refresh.go):
   - Build a target for the requested pair even when it is not in the snapshot “required” set (required today means native→household-base only).
   - Call the FX provider with **that pair** (not rewritten to vs-base). Persist `FXQuote` with the provider’s base/quote/rate.
   - Skip `sourceKind=manual`. Return `fetched` / `failed` / `rate_limited`, not `not_found` merely because the pair is unused in valuation.
3. **Refresh grouping (rate limits):**
   - `RefreshRequiredFX`: **only** snapshot-required native→base pairs (valuation). Do not add extra conversion pairs here.
   - `RefreshAll`: first instruments + those required FX, **then** extra provider `FXPreference` pairs (cross-base conversions). If the provider returns rate-limited during extras, required pairs have already run. Today `refreshTargets` sorts by key and, after one `rate_limited`, skips later targets of the same provider — extras must not sort ahead of required pairs.
   - `RefreshFX(a,b)`: single pair (form Update).
4. Form **Update** for FX:
   - If no preference: `SetFXPreference(a, b, "provider")` then `RefreshFX(a, b)`.
   - If preference is provider: `RefreshFX` only.
   - If preference is manual: do not call the network; copy that they can save a manual rate on Market Data (same honesty as manual instruments).
5. Form **Update** for instrument:
   - `quoteSource === "provider"`: `RefreshInstrument`.
   - `quoteSource === "manual"`: no network button; keep `noQuoteYet` and point to Investments “Set price” (Record-position can still type unit cost).
6. After success, invalidate `queryKeys.quote.instrument.current` / `quote.fx.current` (already in [`invalidation.ts`](../../frontend/src/queries/invalidation.ts)).

This stays user-triggered. It does not refresh on form open. Market Data lists a new pair once the preference exists (it already unions preferences into the FX list).

### 4.4 Settings: global FX provider select (default Frankfurter)

**Problem.** FX vendor is persisted (`Settings.FXProvider`, default `frankfurter`) and applied at save via `SetFXProvider`, but Settings UI never shows it. Users cannot see or change the vendor used by every provider FX refresh.

**Current.**

- [`internal/settings/settings.go`](../../internal/settings/settings.go): `FXProvider`, default Frankfurter.
- Yahoo cannot be selected: `SetFXProvider` rejects providers without `LatestFX`.
- Per-pair SQLite `fx_preferences` stores only `manual` | `provider`, not the vendor. Vendor is global.

**Target.** Settings grows a **Market data** field: FX provider `<select>`. Default Frankfurter. Changing it applies to **all provider-sourced FX pairs** for the **next refresh**. Manual pairs stay manual. Instrument quotes keep their own Yahoo/manual binding.

**Quote selection (required for a second vendor).** `FXPreference` stays `manual` | `provider` (no schema change in this plan). Provider **quotes** already store `source_key` (Frankfurter writes `frankfurter`). Change `CurrentFXQuote` and `selectFXQuote` so that when `sourceKind === provider`, only quotes whose `source_key` equals the current `Service.FXProviderKey()` win. Otherwise switching vendor still shows the previous vendor’s latest row.

**Implementation.**

- Options: providers from the registry with `LatestFX` (today one option: Frankfurter). Do not list Yahoo.
- Generated Settings field is `fx_provider` (not `fxProvider`). Include `timezone` and `fx_provider` in Settings dirty detection. Save already calls `SetFXProvider` when `fx_provider` changes.
- Reuse i18n `settings.providers.*` / `option.provider.*`.
- Market Data source label for `sourceKind === "provider"` uses the **current** Settings `fx_provider` (or `FXProviderKey()`), not hardcoded `marketData.providerFrankfurter`.
- “Use Frankfurter” button should become “Use provider” / configure as `SetFXPreference(..., "provider")` — it still does not pick a vendor.

### 4.5 Timezone: Settings searchable select; remove from Start History

**Problem.** Start History uses a free-text IANA input. Settings already has `Timezone` (`"system"` default) used for presentation, but the page does not edit it. The user wants one global timezone control, as a filterable select.

**Current stores (easy to mix):**

| Store | Role |
| --- | --- |
| `Settings.Timezone` | `"system"` or IANA; presentation (`format.Location`) |
| `HistoryOrigin.Timezone` | Required IANA snapshot at Start History; all activity `effectiveLocalDate` |

There is no timezone catalog. Bootstrap has no timezone. Origin cannot be updated after start.

**Target.**

1. Settings: timezone field using FilterableSelect.
   - Options: **System timezone** (`system`) plus `Intl.supportedValuesOf("timeZone")`.
   - Filter matches IANA id (e.g. `Asia/Shanghai`) and a short label.
   - Helper under the control: if `system`, show the resolved zone (`Intl.DateTimeFormat().resolvedOptions().timeZone`).
2. Start History: **remove** the timezone editor. Show a **read-only timezone** (resolved Settings IANA) and the read-only start date (today in that zone). Help text: history starts on this date in this timezone.
3. `StartHistory` / `StartHistoryWithCosts` is called with that resolved IANA name (`system` is not valid for `NewHistoryOrigin`).
4. After history has started, Settings timezone remains editable for **presentation**. It does **not** rewrite `HistoryOrigin`. Record-change wall time is interpreted in **origin** timezone (ledger). If Settings resolved zone ≠ origin, show a quiet Settings note: history was started in `{origin}`; past local dates stay in that zone.
5. **History always shows the ledger timezone** (`HistoryOrigin.Timezone`) after start; Start History shows the Settings-resolved zone read-only (§6).
6. **Presentation timestamps elsewhere use Settings timezone** (resolved `system` → detected IANA). Today Market Data / Investments `formatQuotedAt` uses the browser default (`new Intl.DateTimeFormat(language)` with no `timeZone`). Change that helper (one shared function) to pass `timeZone: resolvedSettingsTimezone`. Same for any other ordinary `quotedAt` / clock display in the React app. Go `format.Location` already reads Settings; keep it consistent.

**Implementation.**

- Dirty detection includes `timezone` and `fx_provider`.
- Resolve helper: `settings.timezone === "system" ? detected : settings.timezone`.
- `HistoryPage`: when `origin.data` exists, render `origin.data.timezone` (copy such as “Times in {timezone}”).
- Tests that typed Start History timezone switch to Settings-driven date + `StartHistory(resolvedZone)` and that the started History page shows the origin timezone; Market Data quoted-at uses Settings zone.
- Do not add `UpdateHistoryOriginTimezone` in this plan.

### 4.6 FX: sold and bought must differ

**Problem.** Both sides default to the same currency, so the form is not a conversion until the user notices.

**Target.**

- Initial: sold currency = account default or household base; **bought currency empty** (placeholder “Select currency”).
- Changing **sold** currency to equal **bought**: clear bought currency **and** bought amount (and last-auto bought).
- Changing **bought** to equal sold: reject that option — either omit the sold code from the bought `<select>`, or reset bought to empty immediately.
- Preview disabled until both currencies exist and differ.

Do not run `CurrentFXQuote` or convert while currencies are equal or bought is empty.

### 4.7 FX fee, aligned with backend and timeline

**Problem.** `FXConversionInput.Fee` is optional; fee currency must match sold. `activityToInitialCommand` already maps `role: "fee"`. The form has no fee fields, so Fix can resubmit a hidden fee. Timeline sentence is `Converted {sold} to {bought} in {account}` with no fee. i18n `history.fxFee` is unused.

**Target.**

- FX block: optional fee `MoneyFields` labeled `history.fxFee`. Currency locked to **sold** currency (not a second independent picker), matching backend.
- Empty fee = no fee (today’s optional parse).
- Fix: fee visible and editable.
- Timeline / Overview sentence: if a fee effect exists, append localized fee (new `history.sentence.convertedWithFee` or a `withFee` suffix). Trade and debt-payment sentences similarly include fee/interest when present (same honesty pass).

**Implementation.** [`activitySentence.ts`](../../frontend/src/features/history/activitySentence.ts) + `activityToCommand` tests. No backend change if fee already persists.

### 4.8 Same-currency cash transfer: default received = sent

**Problem.** Sent and received are independent even when currencies match.

**Target.** When `sentCurrency === receivedCurrency`, filling sent defaults received (editable, §2.1). Changing sent currency to match received (or the reverse) starts/stops this coupling. Cross-currency transfer does **not** apply FX auto-fill in this plan (that remains FX conversion on a holdings account). Received currency still defaults from **to-account** `defaultCurrency` (§2.4).

### 4.9 Select account → amount currency

**Problem.** Amount currencies default to household base, ignoring the selected account.

**Target.** On account select (including lock from Account detail):

| Kind | Currency field | Editable? |
| --- | --- | --- |
| Money in/out | `currency` | Yes (catalog) |
| Value update | `newValueCurrency` | **Locked** to account default (§2.5) |
| FX | `soldCurrency` selectable; bought empty until chosen; fee locked to sold | |
| Cash transfer | sent from from-account default; received from to-account default | Yes (catalog) |
| Debt | principal (+ interest) locked to cash-account default | **Locked** |
| Trade | gross/fee locked to instrument quote currency | **Locked** |

### 4.10 Value update: show and prefill current value

**Problem.** Value update starts blank. The current amount is only on the detail card behind the sheet. The account select lists every account, including holdings (backend will reject).

**Target.**

- Filter accounts to `trackingMode` `balance` or `manual_value`.
- Show current: `latestValue.amount` + currency (fallback: valuation component without `instrumentId`). Currency **locked**.
- Prefill `newValue` from that current amount so the user edits a total.
- Domain rejects an unchanged value (`ErrNoChange` in `buildValueUpdate`). **Disable Preview/Save** while `newValue` equals the displayed current amount (canonical compare). Helper: “Enter a different new value.”
- Fix: prefill from the activity amount (not a live requote). Enable Preview when any editable field differs from `initial` (amount, reason, note). Do not disable merely because the amount equals today’s `latestValue` — after inverse, that amount is a real replacement.

Account-sheet `SimpleValueForm` uses the same current display + prefill (§4.16).

### 4.11 Debt draw/payment: filter account lists

**Problem.** Both dropdowns list every account. Backend `validateDebtEndpoints` then rejects illegal combinations.

**Target.** Match domain rules ([`change.go` `validateDebtEndpoints`](../../internal/domain/change.go), catalog `accountCombinations`):

- **Debt account:** `balanceSheetRole === "liability"` (legal types are liability + `balance` only).
- **Cash account:** not liability, and `trackingMode` is `balance` or `holdings` (exclude `manual_value`). Exclude the selected debt account.
- Empty filtered list: localized empty state, Preview disabled.

Do not filter by type name alone (`other` exists on both sides).

### 4.12 Holding dropdown: account name and quantity

**Problem.** Options are instrument name only. The same ETF in two accounts is indistinguishable. Quantity is hidden.

**Target.** Position transfer (from/to) and position adjustment use the holding label in §3.3. Keep one option per holding id.

### 4.13 Sell / trade instrument list

**Problem.** Trade always lists **all** instruments. Account-detail Sell only appears when the account has holdings, but the form still allows picking an instrument this account does not hold. Sell with empty `holdingId` creates a zero holding and fails `ErrInsufficientQuantity`.

**Target.**

| Side | Instrument / holding control |
| --- | --- |
| **Sell** | Holdings of the **settlement account** with quantity not zero. Label §3.3. No “create instrument”. Changing the option sets `instrumentId` + `holdingId`. |
| **Buy** | Full non-archived instrument catalog + existing “Create instrument”. Holdings of this account may appear first as a convenience, but buying a new ticker remains possible. `holdingId` from `matchingHoldingId` when already held. |

When the user switches side buy → sell, clear an instrument that is not a positive holding of the settlement account.

History-page trade (no lock): same rules after settlement account is chosen. Sell with no settlement account: disable the holding select.

### 4.14 Position adjustment: added / removed radios

**Problem.** A checkbox toggles added/removed. Deposit/withdraw already uses radios.

**Target.** `role="radiogroup"` with two radios: Added / Removed (`history.added` / `history.removed`). Unit cost remains visible only for Added. Unchecking is impossible; one side is always selected (default Added, matching `emptyChangeRequest`).

### 4.15 Effective date and time

**Problem.** `ChangeCommandRequest.effectiveAt` exists (RFC3339). The Record form never sends it, so every **new** record is “now”. Timeline shows `effectiveLocalDate` only. i18n keys exist. Go already has DST-safe [`ResolveLocalDateTime`](../../internal/domain/change.go). `ToCommand` today only receives `householdID` and cannot load Origin timezone; [`PreviewChange`](../../internal/wailsapi/history/history.go) calls it directly. Domain already rejects future times and times before Origin (`effectiveAt.After(now)` / `Before(OriginAt)`).

**P0 — Fix must not reuse the original time.** Fix appends a reversal and a replacement **now**. Historical replay loads `activities WHERE effective_at <= cutoff` and does not consult `created_at`. If the replacement copied the original `effectiveAt`, it would appear in snapshots **before the correction happened**. This plan does **not** add history-rewrite semantics.

**Target — Record change (new activity only).**

- Date: native `<input type="date">`
- Time: native `<input type="time">` (minute precision)
- Default: now in **History Origin** timezone
- Helper: values are in `{origin timezone}`
- Before Origin or after now: disable Preview and show the same meaning as domain (`change time cannot precede the Starting point` / `cannot be in the future`)

**Target — Fix.**

- Do **not** show an editable date/time picker.
- Show original `effectiveAt` / `effectiveLocalDate` as **read-only** (“Originally recorded …”).
- Submit **empty** `effectiveAt` and empty local date/time so domain uses `now`.
- `activityToInitialCommand` must **not** copy `effectiveAt` into the replacement request.

**ToCommand contract.** `HistoryService` Preview / Record / PreviewFix / Fix all:

1. Load History Origin (required once history has started).
2. Call `ToCommand(householdID, origin.Timezone, request)`.

Field precedence:

| Client sends | Result |
| --- | --- |
| Both `effectiveLocalDate` and `effectiveLocalTime` | `ResolveLocalDateTime` in Origin TZ; ignore `effectiveAt` |
| Only one of date or time | Validation error: both required |
| Neither local field; `effectiveAt` RFC3339 | Use `effectiveAt` (tests / empty-now) |
| Neither local field; empty `effectiveAt` | Zero time → domain `now` |
| Local pair that is DST gap/repeat | `ResolveLocalDateTime` error, shown in the form |

Frontend Record path submits **only** the local pair, never a JS-built RFC3339.

Cash reconcile / simple value `effectiveAt` stay “now” in this plan.

### 4.16 Cash reconcile / simple value: show current balance

**Problem.** [`CashBalanceForm`](../../frontend/src/features/accounts/AccountActionSheets.tsx) and `SimpleValueForm` start empty. Current cash/value is only on the detail page behind the sheet.

**Target.**

- **Simple value:** show current `latestValue` (amount + currency, locked). Prefill the input with that amount. Label stays “new / resulting” total, not a delta. **Disable Save** while the amount equals current (`ErrNoChange`); helper “Enter a different new value.”
- **Cash:** for the selected cash currency, show the matching valuation cash component (`components` without `instrumentId`, that currency). If none, show zero / “no balance in this currency”. **Display** current; do not prefill resulting balance. Switching currency updates the current hint. Save still requires a typed resulting balance (empty remains invalid).

Pass `valuation` (or cash components + `latestValue`) into the sheet from `AccountDetail` — the parent already has it.

### 4.17 Record existing position: latest price on unit cost

**Problem.** Unit cost is an unlabeled optional number. After history, domain falls back to current quote only at submit; if both are missing, `ErrCostBasisRequired`. The user cannot see the price used.

**Target.** When an instrument is selected:

- `QuoteHint` as in trade.
- If unit cost is empty or still auto, default it to `quote.unitPrice`.
- User can edit. Provider-missing: Update per §4.3; manual-missing: type cost or set price on Investments.

Does not change cash (record-position must not pose as a buy — already in the Account interaction contract).

---

## 5. Timeline and Fix echo (follow-through)

These are required for the form changes to stay honest after save.

| Gap | Change |
| --- | --- |
| FX fee not in sentence | §4.7 |
| Trade fee not in sentence | Include fee when present |
| Debt payment interest/fee | Include when present |
| Fix FX fee hidden | Form field §4.7 |
| Fix original time | Read-only; replacement is **now** (§4.15) |
| Fix trade overwriting gross when quote loads | §2.1 |

`activityToInitialCommand` already maps most money fields; extend tests rather than rewriting the inverse.

## 6. Settings and Start History layout (target)

**Settings** (same page, additional fields, still Save-to-apply):

1. Appearance, language, display currency (unchanged)
2. Timezone — FilterableSelect (§4.5)
3. FX provider — NativeSelect, default Frankfurter (§4.4)

**Start History:**

- Title, description, **read-only timezone** (resolved from Settings), **read-only start date**, holding unit-cost list if needed, Start / Cancel
- No timezone editor

**History (after start):**

- Page shows `HistoryOrigin.Timezone` in the header/timeline chrome
- Record change may backdate in **origin** zone; Fix does not

## 7. Backend changes (minimal)

| Change | Why |
| --- | --- |
| Remove “pair must include household base” on `SetFXPreference` and manual FX quote writes | Direct USD↔EUR (§4.2, §4.3) |
| `RefreshFX` for any preferred supported pair; persist that pair | In-form Update (§4.3) |
| `RefreshAll` extras **after** valuation-required FX; `RefreshRequiredFX` stays required-only | Rate-limit isolation (§4.3) |
| `CurrentFXQuote` / `selectFXQuote` filter provider quotes by current `FXProviderKey` | Vendor switch must not keep old `source_key` (§4.4) |
| `ToCommand(householdID, originTimezone, request)` used by Preview, Record, PreviewFix, Fix | Local date/time (§4.15) |
| Optional `effectiveLocalDate` + `effectiveLocalTime` on `ChangeCommandRequest` | DST-safe Record backdating (§4.15) |
| Frontend hooks for `RefreshInstrument` / `RefreshFX` | Form Update |

No schema change (`fx_preferences` / `fx_quotes` already store arbitrary canonical pairs; quotes already have `source_key`). No History Origin mutation. No Fix history-rewrite. Money still canonical strings. Portfolio valuation still uses native→base quotes for holdings; extra pairs are conversion facts.

## 8. Implementation order

Order is dependency-first so later form work can reuse hints and helpers.

1. **Foundations:** decimal helpers; FilterableSelect; QuoteHint; holding label; locked-currency MoneyFields (empty option); `useCurrentFXQuote` same-currency guard; quote refresh hooks; drop base-only FX pair rule; `RefreshFX` any pair; `RefreshAll` required-then-extras; CurrentFXQuote/`selectFXQuote` filter by `FXProviderKey`.
2. **Settings + History chrome:** timezone combobox + `fx_provider` select; dirty on `timezone`/`fx_provider`; presentation `formatQuotedAt` uses Settings TZ; Start History consumes Settings TZ; History page shows origin TZ.
3. **ToCommand timezone:** Origin loaded in HistoryService for Preview/Record/PreviewFix/Fix; local date/time precedence; Record picker only (not Fix).
4. **RecordChangeForm — quotes and defaults:** trade gross (§4.1), FX pair + fee (§4.2, 4.3, 4.6, 4.7), transfer default (§4.8), account currency (§4.9, §2.5).
5. **RecordChangeForm — pickers and echo:** value-update filter/prefill/dirty (§4.10), debt filters (§4.11), holding labels (§4.12), sell vs buy (§4.13), adjustment radios (§4.14).
6. **Account sheets:** cash current hint, simple value prefill+dirty, record-position price (§4.16–4.17).
7. **Sentences:** FX/trade/debt fee in `activitySentence`; Fix tests for fee and **not** copying `effectiveAt`.

Do not start UI auto-fill until P0 Fix-time and P1 ToCommand/locked-currency/FX-provider-filter contracts are in. Do not ship Settings timezone without History chrome and presentation `timeZone`. Do not ship auto-fill without QuoteHint.

## 9. Test plan

- Unit: decimal multiply/divide (USD 2dp, JPY 0dp, bankers, invalid input); same-currency FX query not enabled.
- `HistoryPage.test.tsx`: trade auto-fill; FX auto-fill including USD/EUR with CNY base; Update → `SetFXPreference` + `RefreshFX`; fee submitted; sell = holdings; Start History no TZ editor; started page shows origin TZ; **Fix does not send original `effectiveAt`**; value-update Preview disabled when amount unchanged; FX Preview disabled until bought currency chosen.
- `SettingsPage.test.tsx`: `timezone` + `fx_provider` (default `frankfurter`) in dirty/save.
- `MarketDataPage.test.tsx`: quoted-at uses Settings timezone; provider label not hardcoded if `fx_provider` is set.
- `AccountsPage.test.tsx`: simple value Save disabled when unchanged; cash shows current; record-position unit cost default.
- `activityToCommand`: must not copy `effectiveAt`; fee echo.
- Go: `SetFXPreference("USD", "EUR")` with CNY base; `RefreshFX` stores that pair; `RefreshAll` required FX before extras under rate limit; `CurrentFXQuote` ignores other `source_key`; `ToCommand` with origin TZ; DST gap error; Fix replacement `effectiveAt` is now (existing Fix tests should keep passing).
- Keyboard-only: FilterableSelect timezone.

## 10. Acceptance checklist

- [ ] Quantity + latest price fills editable gross; override survives until quantity/instrument change.
- [ ] FX: typing one side fills the other from the visible **direct** pair rate; override survives; identical currencies impossible; bought starts empty.
- [ ] USD↔EUR (household base CNY) queries and stores that pair; it is not computed from USD/CNY × EUR/CNY.
- [ ] Missing quote shows `marketData.noQuoteYet` and Update fetches that pair/instrument (provider only).
- [ ] Settings FX provider defaults to Frankfurter; provider quotes are selected by current `source_key`; Market Data does not hardcode Frankfurter.
- [ ] Settings timezone is a filterable select; ordinary quoted-at displays use it; Start History has no editor; started History page shows Origin timezone.
- [ ] FX fee is editable, currency locked to sold, Fix-visible, timeline when present.
- [ ] Same-currency transfer defaults received = sent.
- [ ] Locked currencies: value update, debt principal, trade gross/fee; money in/out remain selectable.
- [ ] Value update / simple value show current, prefill, and disable submit while unchanged.
- [ ] Debt vs cash lists match liability / non-liability+balance|holdings.
- [ ] Holding options show instrument, account, quantity.
- [ ] Sell is holdings of the settlement account; buy can still pick or create any instrument.
- [ ] Position adjustment uses radios.
- [ ] Record change can backdate in Origin zone; out-of-range dates disable Preview; **Fix does not change original event time**.
- [ ] Cash reconcile shows current balance for the selected currency.
- [ ] Record-position unit cost shows and defaults from latest price.

## 11. Mapping to the original list

| # | User item | Section |
| --- | --- | --- |
| — | Trade quantity → gross; show price | §4.1, §2.3 |
| — | FX one side → other; show rate | §4.2, §2.3 |
| 1 | `noQuoteYet` + Update to fetch rate | §4.3 |
| 2 | Settings global FX provider, default Frankfurter | §4.4 |
| 3 | Timezone in Settings, filterable select; not Start History; History shows the zone in use | §4.5, §6 |
| 4 | FX sold/bought cannot match; sold change clears bought | §4.6 |
| 5 | FX fee in form and history | §4.7, §5 |
| 6 | Same-currency cash transfer default | §4.8 |
| 7 | Account select → defaultCurrency | §4.9, §2.4 |
| 8 | Value update show + prefill current | §4.10 |
| 9 | Debt account list filters | §4.11 |
| 10 | Holding dropdown account + quantity | §4.12 |
| 11 | Sell/trade instrument list | §4.13 |
| 12 | Added/removed radios | §4.14 |
| 13 | Effective date-time picker | §4.15 |
| 14 | Cash/simple current balance | §4.16 |
| 15 | Record-position latest price | §4.17 |
