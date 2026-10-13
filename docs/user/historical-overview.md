# Historical panorama

Owner: Historical panorama maintainers. Status: implemented P0/P1/P2a and endpoint reconciliation behavior;
automated contracts are tested, native visual acceptance is pending.

Historical panorama reconstructs retained balances at the end of a household
day. Later corrections and backfilled observations can change a new read of
that day. Names and classifications use current metadata. Opening the panorama
reads local facts once; it does not fetch prices, rebuild snapshots or edit data.

Choose a closed day for a single-date view, or use A (baseline) and B (comparison)
for a two-date view. Every difference is B minus A. Reverse comparisons are
allowed. Current is available only for B and is fixed at the displayed capture
time. Use **Refresh both dates** to capture both sides together.

The view tabs have one stop in the keyboard Tab order. Use Left/Right arrows
to select an adjacent view, or Home/End for the first/last view. Tab moves into
the selected panel, which is labelled by its view. Changing views uses the
current capture without reading the dates again.

Net worth, assets and liabilities each show their own A/B values and difference.
A balance difference is not investment return or an explanation of what caused
the change. Unknown values remain unknown, including for same-date comparisons.

Single-date composition shows assets. Negative values use signed bars instead
of a donut. Category comparison lets you choose assets or liabilities, then
current category or original currency. All monetary bars use the base currency.
Liability changes describe debt balances, without reversing their sign into net
worth effects.

Category cards show the backend monetary difference and share difference in
percentage points. A missing side, incomplete valuation or changed inclusion
scope can make a comparison unavailable. Its reason remains visible. Shares
marked **of known subtotal only** describe valued components; missing components
have unknown value. These shares cannot be compared as a complete allocation.
A category with no known component amounts has an unknown subtotal and no
share; an observed zero remains distinct. A zero or negative denominator does
not produce a percentage. Expand **Full
source data table** to inspect every category, including zero and missing states.

Select a category to inspect its accounts without changing the household totals.
Use **Back to categories** to restore your position and selection. Account
details default to all accounts; **Only changed rows** is an explicit filter. Open
an account's read-only detail, then select its cash/holding child or return to
the parent. When a category filter is active, details group matching and other
retained components explicitly. The category filter leaves captured account amounts unchanged; archived and
excluded components retain their existing status and counting rules. Opening
an other component labels it outside the filter. The source
table names missing components with stable identities alongside them. Details show exact values and the applied price/FX observation's
available source, date, rate direction and valuation basis. Missing inputs say
which currency or instrument is affected. Missing provenance is left blank;
there is no replacement with a current quote.

**Leave panorama · View transaction history** opens a separate current view for
the explicit account/instrument and date range. For a current comparison that
range stops at the last closed day; today's activity may also affect the
capture. Use the App's **Back to Historical overview** action to recover the
same frozen capture, dates, filters, open detail and position. Exit through the
page's return action or main navigation to end that session.

## Explain net worth change

A two-closed-date comparison also shows **Explain net worth change**:

`B − A = supported contributions + unexplained residual`.

Expand **How these amounts reconcile** for exact category values and evidence
limits. This uses the same captured ledger, base currency and inclusion rules
as the endpoint totals, with no extra database queries, snapshot writes or
provider fetches. Unknown endpoint totals produce unknown attribution and
residual, including on the same day. A complete same-day comparison is zero.
Reverse comparisons negate the same chronological bridge. Current B retains
its ordinary comparison but attribution requires two closed dates.

Recorded monetary contributions cover external flows, income/gifts, spending,
dividends/interest, fees/taxes, recorded remeasurement adjustments and liability
impact according to the existing ledger classifications. Positive debt changes
reduce net worth. Internal principal transfers and debt repayment/draw do not
create wealth within the included scope.

This first iteration is an **endpoint bridge**, not investment return or an
event-time cash-flow report. Money movements convert at the chronologically
later endpoint's FX rate. Opening balances contribute the FX rate change;
unchanged holding quantities with comparable quotes contribute price change
at the later FX rate. A change between date-label and actual source-timestamp
price observations breaks comparability and leaves the holding movement in the
residual with a quote-basis reason. These contributions do not overlap. Arithmetic uses exact
decimal strings and the existing decimal FX division precision, with no hidden
rounding of small residuals. The source table preserves full decimal amounts.

Trades, transfers or other activity touching a holding quantity defer its
price/FX path attribution, even when the ending quantity matches the beginning.
Foreign internal/trading exposure changes defer opening-only FX attribution.
Missing evidence, changed scope/state, incompatible quote basis and unsupported
activities remain visible as reasons. Any remaining amount stays explicitly
**unexplained**; it is never called investment profit. A zero residual can still
have uncertainty reasons (for example offsetting unsupported movements).
A later correction or reversal changes a new capture through the same economic
replay used by the endpoint values, without modifying immutable ledger facts.

Additional reconciliation work has a 100,000-entry budget counting retained
accounts/rows, activities/effects and state/preference/binding observations, and a ten
second computation bound. Budget rejection leaves the ordinary comparison
available and reports attribution unavailable. Cancellation abandons the whole
capture. Intermediate daily prices are not reconstructed: endpoint price
coverage and quote authority remain those of the existing valuation resolver.

## Bounded time trend

Open **Time trend**, choose A and B and select **Enable trend · New capture**.
The range includes every civil date, from one to 31 closed days, in your history
timezone. This explicitly reads a new frozen overview and trend together. A
single-day range is one observation. Reverse, current and longer comparisons
remain available with the usual comparison controls; they do not create a
trend.

Choose net worth, assets or liabilities. The line shows complete totals only:
unknown values and skipped civil dates leave gaps, and changes in included
components or valuation basis break connections. Complete assets can still be
shown when liabilities are missing. Known subtotals are labelled separately in
the full source table and are never drawn as complete totals. Each table row
shows its exact values, component coverage and missing identities/reasons.

Select a point in the chart or its date button in the table. Selection, metric
and tab changes use the current capture. **Set as A/B · Recapture range** reads
both the updated overview and trend again. **Open day · New capture** explicitly
leaves the trend for a single-date overview. History Back restores the frozen
series and your metric and selection. Refresh reads the active bundle together;
focus and reconnect do not refresh it.

The first local trend has an input/computation budget. A profile with more than
1,000 retained possible components, 100,000 retained facts, 32 MiB of serialized
input, or a computation exceeding ten seconds is rejected explicitly. This
counts the whole retained profile, including archived facts; shortening the
range may not remove an input-budget rejection. No dates or facts are silently
removed. Ordinary historical comparisons remain available. A later correction
or backfill can change the next explicit capture; this view is reconstructed
from retained facts and current metadata, not what was known at that past time.
