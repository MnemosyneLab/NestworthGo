# Historical panorama reconciliation

The GUI and historical read tools explain two net-worth endpoints from one
immutable capture. This is an endpoint reconciliation, **not investment return**.
No snapshot maintenance, provider request, ledger mutation or extra repository
query is added. Current endpoints remain unavailable because their quote
selection authority differs from closed-day history.

For complete compatible endpoints, exact decimal strings preserve:

`B net worth − A net worth = supported contributions + unexplained residual`.

The basis is `endpoint_bridge/2; later_endpoint_fx; opening_exposure_fx;
paired_principal_valuation; retained_corrected_facts`. Compute over
`(earlier civil-day cutoff, later cutoff]`, then negate monetary contributions,
paired principal and residual for reverse comparisons. Quantities and related
records remain in chronological order and retain their economic directions.
Later corrections and undo consume the capture's normalized retained economic
facts; recover semantic kind/reason/trade detail through reversal chains without
filtering records or inverting their effects a second time.

## Recorded effects and principal

Keep the shared asset-bucket classifier's semantic precedence: income, spending,
dividends/interest, fees/taxes, adjustments, external flows and liability impact.
Monetary contributions use the chronologically later endpoint's adopted FX
conversion, not event-time FX. Fees never become trade principal or a second
holding contribution. Cost-only corrections change neither quantity nor wealth.

For a buy/sell, require exactly one quantity leg, one opposite cash principal
leg, and matching trade detail (holding, instrument, quantity, currency and gross).
Both components must be included and reliable, with compatible later FX evidence.
Assign the cash principal once and its exact opposite to the holding. Never
allocate a single gross amount to several quantity legs. Ambiguous multi-leg
trades remain unexplained. This also supports round trips, sales and liquidation.
A proven newly-created holding in an existing included account has zero opening
exposure; absent unknown values and newly-included accounts do not acquire zero
balances. A cleared zero holding can use its uniquely paired settlement cash's
later FX evidence, since zero valuation itself need not select an FX quote.

Reconcile `opening quantity + signed recorded quantity = closing quantity`.
For a holding whose quantity reconciles, verify endpoint native values against
quantity × adopted unit quote. Require compatible currency, source/source key,
price basis, observation kind, timestamp authority and source policy on positive
endpoint exposures, without intervening scope/preference/binding changes.
A real zero exposure needs no unit quote. Supported valuation change is:

`(closing native value − opening native value − net trade/transfer principal
  − measured quantity adjustments) converted at later FX`.

Call this **valuation change after principal**, never pure price return. It can
include execution-price versus endpoint-valuation spreads and is not daily path
attribution. Opening native exposure converted at each endpoint gives the
separate FX contribution. Changed FX authorities and missing conversion evidence
retain their affected difference. Finite decimal division can leave an exact
nonzero remainder; there is no epsilon or forced rounding/clearing.

Same-instrument position transfers require two opposite equal quantity legs,
compatible later quotes and FX, and included endpoints. Their virtual principal
cancels once across accounts. Monetary internal transfers and debt principal are
neutral only as a complete zero-sum set after later-rate conversion and liability
signs. Unbalanced foreign internal legs and their uncertain exposure remain in
residual, never income.

## Quantity remeasurement

A recorded reconciliation quantity effect is an **adjustment**, not income.
Measure its signed quantity at the later adopted unit quote; unit cost establishes
recorded cost semantics, not market value or earnings. It is not a cash principal
leg. For an unpriced remeasurement with changed unit quotes, a share restatement
such as a split is possible: retain the affected movement and
`remeasurement_basis` uncertainty instead of inventing income, a split ratio or
an adjustment amount. With unchanged comparable unit quotes, unpriced share
remeasurement can be measured as an adjustment (for example a flat-NAV money
fund). Quantity gaps or unsupported quantity effects block the holding bridge.
Zero net residual never erases a recorded uncertainty.

## Details and limits

Every leaf exposes exact `total`, contribution subtotal, paired `principal`,
`residual`, chronological quantity reconciliation, reason codes and related
records with signed native money/quantity effects. Per-leaf identity is:

`total = supported contributions + paired principal + residual`.

Across the household, paired principal sums to zero. Account parent rows are
excluded from calculation. Missing totals stay null; known leaf evidence remains
available even when the household total/residual is unavailable. Scope,
valuation, quote-basis and quantity mismatches retain explicit reasons. These
checks do not diagnose any particular real ledger's USDC/USDT difference.

The GUI groups details under readable account and instrument names, with shared
Button disclosures, exact audit amounts, localized timestamps and English,
Simplified Chinese and Traditional Chinese copy. Monetary headlines continue
using the shared display formatter. No frontend financial arithmetic is added.
The MCP historical header excludes details/records: `overview/endpoints` opens
pageable `contributions` and `reconciliationDetails`; a reconciliation detail
item opens its own `reconciliationRecords/<key>` section. These sections remain
household-wide even when endpoint-row filters are present, and retain the same
capture hash, byte budget and cursor semantics.

Work is bounded by retained row/account/holding/effect/observation counts and
cancellation/time limits. Budget refusal preserves the ordinary comparison.
Event-time FX, ambiguous multi-leg allocation, automatic corporate-action
inference and full daily return decomposition remain outside this bridge.
Native macOS GUI acceptance is a separate gate from Linux builds, browser
checks and synthetic tests.
