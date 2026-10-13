# Historical panorama reconciliation (first iteration)

## Design before implementation

Keep the existing single immutable batch, replay cutoffs, net-worth scope and
exact endpoint valuation. Add a read-only reconciliation to the captured
comparison (including the bounded trend bundle); no snapshots, providers,
ledger changes or extra repository queries. No new chart is needed: a compact
summary and expandable exact contribution table explain the identity better.

For complete compatible endpoints:

`B net worth − A net worth = supported contributions + unexplained residual`.

Compute chronologically over `(earlier civil-day cutoff, later cutoff]`, then
negate every contribution and residual for reverse comparisons. Same-day
complete endpoints have zero movement; incomplete totals stay unknown. Current
B keeps its explicit current boundary and is left unavailable for attribution
in this first iteration because historical and current quote authorities differ.

Monetary ledger effects use the existing asset-bucket classification (income,
spending, external flows, dividend/interest, fees, adjustments and liability
impact). Convert at the **chronologically later endpoint rate**, explicitly an
endpoint bridge convention rather than event-time cash flows or investment
return. For stable components, opening native amount × (later rate − earlier
rate) is FX. An unchanged holding quantity with comparable price evidence uses
quantity × (later price − earlier price) × later FX for price, and opening
native value × rate change for FX. These terms do not overlap.
Price continuity includes timestamp basis: an Agent date-label anchor and an
actual source timestamp are different authorities. A switch leaves the affected
holding movement in residual with `basis_changed`, matching the trend contract.

Require included, complete same-currency components at both ends and no relevant
intermediate scope or preference changes. Ignore parent rows to avoid duplicate
counting. Fully internal principal legs are neutral; their discrepancies remain
residual. Do not attribute price for quantities touched by activities (including
round trips), trades, splits or transfers. Foreign internal/trade exposure
changes do not receive an opening-only FX estimate; their movement stays
residual. Missing endpoint values, changed scope, unsupported or unknown
monetary/quantity effects and internal imbalances are disclosed. Corrections
and reversals consume the repository's same normalized economic activities as
historical replay, never raw facts or a second reversal filter.

Bound additional work by retained account/row/effect/observation counts and cancellation checks.
Exceeding the reconciliation budget leaves the ordinary comparison available
with an explicit unavailable reason. Do not round contributions or hide small
residuals; exact decimal arithmetic uses the existing valuation conversion
precision. A zero net residual does not erase uncertainty reasons.

## Delivery and deferred decisions

Expose total, supported subtotal and residual in the comparison view, with
localized expandable definitions, exact category amounts and reason codes.
Reuse existing Tabs, DatePicker, capture cancellation and theme controls.
Synthetic application tests establish decimal identity, multi-currency bridge,
neutral transfers, liability signs, missing evidence, corrections/reversals,
same/reversed dates and bounded cancellation. Frontend tests establish readable
unknown/zero/negative states, keyboard disclosure and latest-capture behavior.

Defer trading/path attribution and event-time FX until a compatible bounded
read-only adapter to the existing daily analysis is justified. This iteration
must not reuse the snapshot-maintaining MCP attribution endpoint. Native macOS
GUI acceptance remains separate from Linux builds and synthetic UI tests.
