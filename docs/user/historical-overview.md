# Historical panorama

Owner: Historical panorama maintainers. Status: implemented P0/P1/P2a behavior;
automated contracts are tested, native visual acceptance is pending.

Historical panorama reconstructs retained balances at the end of a household
day. Later corrections and backfilled observations can change a new read of
that day. Names and classifications use current metadata. Opening the panorama
reads local facts once; it does not fetch prices, rebuild snapshots or edit data.

Choose a closed day for a single-date view, or use A (baseline) and B (comparison)
for a two-date view. Every difference is B minus A. Reverse comparisons are
allowed. Current is available only for B and is fixed at the displayed capture
time. Use **Refresh both dates** to capture both sides together.

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
