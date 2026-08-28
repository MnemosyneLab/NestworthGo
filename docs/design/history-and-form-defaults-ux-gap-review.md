# History and form-defaults UX — implementation gap review

- Date: 2026-08-28
- Baseline reviewed: `e2687aa` (`feat: implement account and history UX designs`) on `origin/main`
- Contract: [`history-and-form-defaults-ux.md`](history-and-form-defaults-ux.md)

This is a completeness review of that commit against the plan (including the accepted P0/P1/P2 review corrections). Most of the plan **is** implemented. The items below are incomplete, wrong, or untested. They are the work remaining after that commit.

## Verdict

P0 Fix-time is correct: Fix does not copy or submit the original `effectiveAt`; replacement uses now; the original time is read-only.

Backend contracts from §7 are in place: any supported FX pair, `RefreshFX` for that pair, `RefreshAll` required-then-extras, provider quotes filtered by `FXProviderKey` / `source_key`, `ToCommand` with Origin timezone, local date/time fields.

Remaining issues are UI contract mismatches, one Settings note placement miss, and the §9 test gaps.

## Done (not re-opened)

Shared decimal helpers, QuoteHint, FilterableSelect, holding labels, locked currencies, FX empty bought + `a !== b` query guard, trade gross auto-fill, FX rate auto-fill, Update refresh, FX fee + timeline `withFee`, same-currency transfer default, account currency defaults, debt filters, adjustment radios, effective date/time on Record, cash current hint, simple-value prefill + unchanged disable, record-position unit-cost default, Settings timezone + `fx_provider` dirty/save, Start History read-only timezone, History Origin timezone chrome, presentation `formatTimestamp`.

## Gaps to fix

### G1 — P1 WRONG — Sell still offers Create instrument

Plan §4.13: Sell is holdings of the settlement account only. **No “create instrument”.**

`RecordChangeForm` always renders Create instrument for every trade, including sell.

**Fix:** Hide create (and cancel an open create form) when `side === "sell"`. Put **Side** above the instrument/holding picker so the list matches the chosen side.

### G2 — P1 PARTIAL — Fix value-update Preview not gated on dirty-vs-initial

Plan §4.10: Fix prefills from the activity. Enable Preview only when amount, reason, or note differs from `initial`. Do not disable merely because the amount equals today’s `latestValue`.

Current code exempts Fix from the live `latestValue` compare, but still allows Preview with an untouched form.

**Fix:** For Fix + value update, disable Preview while the form still equals `initial`.

### G3 — P1 PARTIAL — Settings missing Origin vs presentation timezone note

Plan §4.5.4: if Settings resolved zone ≠ History Origin, show a quiet **Settings** note that history was started in `{origin}` and past local dates stay there.

The note exists only on `HistoryPage` (`history.settingsTimezoneDiff`). Settings has no Origin query and no note.

**Fix:** Load History Origin on Settings and show the same copy when the zones differ.

### G4 — P2 PARTIAL — Sell holding select not disabled without settlement

Plan §4.13: Sell with no settlement account: disable the holding select.

Empty options / “no matches” exist, but the control stays enabled.

**Fix:** Disable the select when `side === "sell"` and settlement is empty.

### G5 — P2 PARTIAL — Value update has no valuation-component fallback

Plan §4.10: show current from `latestValue`, fallback to a valuation component without `instrumentId`. Clearing the field should restore auto-fill (§2.1).

History `RecordChangeForm` only reads `latestValue` and does not refill after clear.

**Fix:** Fall back to `AccountValuations` cash/simple components; restore the current amount when the field is cleared (new records only).

### G6 — P2 PARTIAL — Account-sheet `initial` is treated as a Fix override

`autoValues` marks every non-empty `initial` field as `__user__` whenever `initial` is passed. Account action sheets pass `initial` for **new** records, so defaults are frozen as user overrides.

**Fix:** Mark `__user__` only for Fix (`fixActivityId` set).

### G7 — P2 MISSING tests (§9)

Present in the plan, not covered (or only partially):

| Test | File |
| --- | --- |
| FX amount auto-fill (USD↔EUR, household base CNY) | `HistoryPage.test.tsx` |
| FX fee submitted on Preview | `HistoryPage.test.tsx` |
| Sell = settlement holdings only; no create instrument | `HistoryPage.test.tsx` |
| Started History page shows Origin timezone | `HistoryPage.test.tsx` |
| Value-update Preview disabled when unchanged | `HistoryPage.test.tsx` |
| FX Preview disabled until bought currency chosen | `HistoryPage.test.tsx` |
| Simple value Save disabled when unchanged | `AccountsPage.test.tsx` |
| Cash reconcile shows current (or no cash) | `AccountsPage.test.tsx` |
| Record-position unit cost defaults from latest price | `AccountsPage.test.tsx` |
| Market Data quoted-at uses Settings timezone | `MarketDataPage.test.tsx` |
| Settings Origin≠presentation note | `SettingsPage.test.tsx` |

## Out of scope / accepted as-is

- Buy listing settlement holdings first is optional (“may”) in §4.13; not treated as a defect.
- Keyboard coverage for FilterableSelect exists in `keyboardOnly.test.tsx` (combobox is in the Settings tab order). A dedicated Arrow/Enter/Escape unit suite is nice-to-have, not blocking.
- Design doc status still said “Planned”; update it once these gaps close.

## Implementation order

1. G1, G4, G6, G2, G5 in `RecordChangeForm`.
2. G3 on `SettingsPage`.
3. G7 tests.
4. Flip the design-doc status to implemented.
