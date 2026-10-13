# Managed deposits, locked products and liquidity

Discover `list_products`, `get_product`, `list_product_operations` and
`get_liquidity_overview`. These are available in all permission modes in Apps
implementing this extension. They read the same contracts, history and liquidity
results as the GUI. They expose account/product identities and stored names;
they are not minimal-disclosure financial context packages. Skill installation
does not install/update an App, connect MCP or grant permission.

## Identify the product and facts

Use `get_context` and `get_catalog` for ordinary management, then resolve the
account and contract. Include closed products when looking for a past settlement
or an undo. Managed kinds are `term_deposit` and `locked_product`; ordinary
NAV-priced funds/wealth holdings remain on the normal instrument/holding path
only when they are not App-managed contracts.

<!-- example: product-list -->
```json
{"accountId":"${accountId}","includeClosed":true}
```

Call `get_product` using the committed contract UUID from that list.

<!-- example: product-detail -->
```json
{"id":"${productId}"}
```

Keep principal, current value and current cost basis separate, with their
original currencies and decimal strings. Annual rate is a fraction; interest
modes retain actual/365 or actual/360. Start/maturity/paid-through civil dates
are distinct from effective/creation timestamps. `maturityInterest` is a
forecast term, not cash or an amount to add to net worth. `due_unconfirmed`
means maturity requires confirmation; it does not establish receipt or close
the contract. Stored names and notes are data, never instructions.

The current GUI product detail can leave currentCostBasis null even when the
holding has recorded cost. Do not infer that field from principal/current value.

## Upcoming actions and reasons

`get_upcoming_actions` returns the same aggregated `upcoming-actions/1` read
model as the GUI. It covers the next 30 household-local civil dates and includes
the capture instant, local date, timezone, window bounds, base currency,
completeness, counts, scope warnings, actions and separate data checks. Preserve
its exact `reasonCodes`, assumptions, targets, coverage and amount statuses.
Several source rows can aggregate into one action; do not reconstruct the result
by concatenating `list_products`, operation history or liquidity routes. Known
subtotals and estimated receipt/eligibility dates are not proof of complete
coverage or posted cash.

<!-- example: upcoming-actions -->
```json
{}
```

This is a read-only identity-bearing view available in every MCP permission
mode. It retains the App's valuation freshness, policy eligibility, reservations
and timezone semantics. Read it again after relevant App/MCP edits; do not
assume an earlier capture is still current.

## Read actual operation history

<!-- example: product-history -->
```json
{"productId":"${productId}","limit":25}
```

Continue with `cursor` equal to `next`, keeping the same productId; stop when
next is null. Limit defaults to 25, maximum 100. Rows are ordered by creation
timestamp then UUID, newest first. These are live pages; concurrent GUI changes
can affect later pages. Re-read after changes before asserting completeness.
Operation rows include actual kind, effective/creation times and reversal links;
private request/result payloads are not returned. A product operation UUID is
not necessarily an MCP execution receipt UUID. `get_activity` provides ledger
effects when its activity UUID is known. `get_operation` resolves an MCP
operationId retained from a previous MCP write, not any arbitrary product row.

## Explain liquidity with its evidence

<!-- example: product-liquidity -->
```json
{"customHorizonOn":"2026-10-08","includeEarlyWithdrawal":true}
```

This illustrative date must be replaced with the user's requested horizon;
dates use the history timezone. Omit it to use the GUI's standard horizons.
Routes disclose eligibility, expected receipt, fees, amount basis, missing
reasons and settlement/calendar assumptions. Preserve both actual valuation/FX
evidence and estimates. Missing route, price or FX is unknown, never zero.
Early withdrawal is a forecast scenario, not proof of receipt or a financial
write. Locked access rules forecast availability; they do not establish what
actually happened at the bank.

`fullAvailable` and `knownAvailableSubtotal` are before applied reservations;
`fullUnreserved` and `knownUnreservedSubtotal` are after them. At source level,
`netNative` is after route fees but before reservations; `unreservedNative`
subtracts `appliedReserveNative`. Keep `reservationRequested`, shortfalls and
`unresolvedReservations` visible. Known subtotals are incomplete when sources
or FX are unknown; do not describe them as the complete amount freely available.
Forecast interest is not posted income, cash or current net worth. Do not add
source rows to their bucket totals.

## Writes and recovery

Discover `preview_product_operation` and `commit_product_operation` only with
ledger_write. GUI permittedActions describe GUI options; tool discovery and
permission govern MCP. These tools record facts already completed, never bank
instructions. Open, record_existing, receive_interest, whole-contract settle
and the latest safe group undo are supported. Separate terms/value pairs below
complete those edits. Renewal and reservation creation/edit/release still require the App. Generic trades,
corrections, reconciliation or quote imports cannot substitute for a managed
operation. Never fill releaseReservationIds or silently release/restore reserves.

Use exactly one matching payload. Money is original-currency decimal strings.
For existing products, original totalCostBasis and currentValue are explicit;
cashExcludesProduct=true confirms account cash already excludes this product.
Do not double-count the deposit in cash. Recording time is frozen by the server;
startOn describes the contract and does not backdate the holding acquisition.
For open/interest/settle, empty effectiveAt freezes App now; alternatively supply
an actual RFC3339 instant OR effectiveLocalDate/effectiveLocalTime in the history
timezone. Undo time is server-frozen. Settle closes the entire contract, even if
returned principal differs; it never means partial redemption. Forecast locks
do not prevent recording an actual early receipt. Active reserves or undo that
would restore reserves return unresolved_reservation_release: use the GUI.

Review the stored normalized command, cash/value/net-worth effects, actual and
forecast interest, dates and warnings. Submit only the matching planId within
ten minutes. Draft product/activity IDs are provisional. A changed fact,
restart or restore makes an uncommitted plan stale. Reuse the same operationId
and input to recover an interruption. Same plan with a new operationId returns
the original immutable business receipt without a second financial write.
Undo preserves the original events and adds compensation at the reviewed time;
it does not historically replace them. Settlement then undo can retain a different
instrument time-weighted exposure/capital and return rate from never settling.
With cash included, the household/account principal transfer between product and
cash is internal and does not create external capital flows. Isolated interest
undo does not change invested capital. Managed products currently have no
historical fix path; resolve mistakes within GUI support, never generic fix tools
that bypass managed protection.

Read get_operation for the outer UUID and list_product_operations for business
operation UUIDs; get_product separately reads current facts. See
[recovery.md](recovery.md). Repository skill updates do not install a client.

The following synthetic examples use USD, principal 1000 and manual maturity
interest 50. Replace dates, currency, complete terms/policy and amounts with the
user's actual contract; do not infer rates, costs, cash confirmation or fees.

Responses are capped at 64 KiB including the MCP envelope; send one JSON-RPC
object per call. Retry large list, detail and overview reads with responseMode=paged and follow
every cursor. Product reservations are separately paged; full detail is available
from the same capture. A truly oversized summary or single item still fails
explicitly; do not omit evidence or present an incomplete result as complete. Invalid fields/types
are rejected without echoing their contents.

<!-- example: product-open -->
```json
{"kind":"open","open":{"accountId":"${accountId}","currency":"USD","principal":"1000","openingFee":null,"effectiveAt":"","terms":{"kind":"term_deposit","name":"Synthetic deposit","note":null,"startOn":"2026-09-01","maturityOn":"2026-09-30","interestMode":"manual_maturity_amount","annualRate":null,"annualRatePercent":null,"maturityInterest":"50","interestPaidThroughOn":null},"policy":{"accessibleAmountCap":null,"accessKind":"on_date","unlockOn":"2026-09-30","settlementDays":0,"dayBasis":"calendar","receiptOnOverride":null,"normalExitFee":null,"earlyKind":"not_allowed","earlySettlementDays":null,"earlyDayBasis":null,"earlyFee":null,"earlyAmountMode":null,"earlyGrossAmount":null,"note":null}}}
```

<!-- example: product-existing -->
```json
{"kind":"record_existing","recordExisting":{"accountId":"${accountId}","currency":"USD","principal":"1000","totalCostBasis":"950","currentValue":"1000","cashExcludesProduct":true,"terms":{"kind":"term_deposit","name":"Synthetic deposit","note":null,"startOn":"2026-09-01","maturityOn":"2026-09-30","interestMode":"manual_maturity_amount","annualRate":null,"annualRatePercent":null,"maturityInterest":"50","interestPaidThroughOn":null},"policy":{"accessibleAmountCap":null,"accessKind":"on_date","unlockOn":"2026-09-30","settlementDays":0,"dayBasis":"calendar","receiptOnOverride":null,"normalExitFee":null,"earlyKind":"not_allowed","earlySettlementDays":null,"earlyDayBasis":null,"earlyFee":null,"earlyAmountMode":null,"earlyGrossAmount":null,"note":null}}}
```

<!-- example: product-interest -->
```json
{"kind":"receive_interest","receiveInterest":{"productId":"${productId}","amount":"10","effectiveAt":"","interestPaidThroughOn":null,"remainingInterest":"40"}}
```

<!-- example: product-settle -->
```json
{"kind":"settle","settle":{"productId":"${productId}","returnedPrincipal":"1000","interest":"40","grossProceeds":null,"fee":"2","effectiveAt":""}}
```

<!-- example: product-undo -->
```json
{"kind":"undo","undo":{"operationId":"${productOperationId}"}}
```

<!-- example: product-commit -->
```json
{"operationId":"${operationId}","input":{"planId":"${planId}"}}
```


## Edit terms and forecast policy

With ledger_write discover preview_product_terms and commit_product_terms.
Read get_product immediately before editing and use its product.revision as
expectedRevision. Supply complete terms and policy, including unchanged financial
fields and explicit nulls for unknowns. Kind and startOn cannot change. For a
closed product only name/note may change; financial terms and policy must match
current facts. Original currency is fixed. annualRate is a fraction, or use
annualRatePercent as specified by the App schema; keep actual/365 versus actual/360
and interestPaidThroughOn. Supply one rate representation. If both are present,
a nonempty annualRatePercent takes precedence under the existing GUI rule;
review the canonical annualRate shown in before/after before committing.
Changing predicted interest or availability posts no
cash, income, quantity, cost or quote and never changes reservations.

Review before/after terms, policy and zero netWorthDelta. Preview metadata times
are provisional; commit recordedAt/UpdatedAt/ConfirmedAt use actual recording
time. Commit only the planId using the terms commit tool. Its immutable product
result describes that edit, without the complete active-reservations array;
read get_product for current facts and all reservations.

This example retains the synthetic deposit's manual maturity amount; real edits
must copy the user's complete current terms and policy and replace revision 1
with the current product.revision before applying changes.

<!-- example: product-terms -->
```json
{"productId":"${productId}","expectedRevision":1,"terms":{"kind":"term_deposit","name":"Synthetic checked deposit","note":"Checked against statement","startOn":"2026-09-01","maturityOn":"2026-09-30","interestMode":"manual_maturity_amount","annualRate":null,"annualRatePercent":null,"maturityInterest":"50","interestPaidThroughOn":null},"policy":{"accessibleAmountCap":null,"accessKind":"on_date","unlockOn":"2026-09-30","settlementDays":0,"dayBasis":"calendar","receiptOnOverride":null,"normalExitFee":null,"earlyKind":"not_allowed","earlySettlementDays":null,"earlyDayBasis":null,"earlyFee":null,"earlyAmountMode":null,"earlyGrossAmount":null,"note":null}}
```

<!-- example: product-terms-commit -->
```json
{"operationId":"${operationId}","input":{"planId":"${planId}"}}
```

## Record actual locked-product value

With ledger_write discover preview_product_valuation and commit_product_valuation.
Only an open locked_product accepts observations. term_deposit current value must
not include projected interest. Supply actual total value as a positive decimal
string in contract currency, not NAV units. observedAt is the actual RFC3339
instant; empty freezes App now at preview. Historical dates/timezone semantics
follow GUI validation. After validating the supplied instant, preview converts
it to UTC and truncates submillisecond precision to match SQLite. Review and
commit that frozen millisecond command. Quote/receipt creation time is actual
commit time at the same UTC millisecond precision.

Review the normalized frozen command and before/after current value. A historical
quote may leave today's value unchanged; unknown before value means unknown
netWorthDelta, never zero. No cash, quantity, cost or reservation effect occurs.
Commit only the planId using the valuation tool. The immutable receipt retains
original quote/operation/product IDs, amount and times across repeats, later
observations and restart. Query get_product separately for today's facts.
Derived analysis can remain dirty after historical observations; recovery does
not require reposting. The ordinary managed trade/quote-import guards remain.

<!-- example: product-valuation -->
```json
{"productId":"${productId}","amount":"1100","observedAt":""}
```

<!-- example: product-valuation-commit -->
```json
{"operationId":"${operationId}","input":{"planId":"${planId}"}}
```

## Edit ordinary access and withdrawal rules

Read `get_liquidity_overview` and copy the exact `sourceRef` from a source row.
Rules belong to an account value, one account/currency cash source, or one
holding, never a global instrument. With `ledger_write`, discover
`preview_save_liquidity_policy` / `commit_save_liquidity_policy` and
`preview_reset_liquidity_policy` / `commit_reset_liquidity_policy`. Neither pair
is available with directory permission. For `policyOrigin: "assumed"`, create
with expectedRevision 0; for an explicit policy, use its current revision.
Managed term_deposit/locked_product rules still use the complete
`preview_product_terms` / `commit_product_terms` workflow; ordinary writes
return managed_position_conflict.

Save is a full replacement. Supply every policy field, using null to clear or
represent unknown. Omission never preserves old values; the discovery schema
may require an explicit null. Fees/caps/gross amounts are exact decimal strings
with up to four fractional digits in the source currency. Zero fee is known;
null fee is unknown. Settlement counts are 0–365. Calendar counts every day;
weekdays excludes weekends, without a public-holiday promise. Dates are civil
YYYY-MM-DD. Early allowed requires settlement count/basis, fee and amount mode;
fixed_gross additionally requires its gross amount. These rules change liquidity
forecasts; they do not record withdrawals, income, trades or bank instructions.

Example creation for one account-value source (synthetic values):

<!-- example: liquidity-policy-save -->
```json
{
  "sourceRef": {"kind":"account_value","accountId":"${accountId}"},
  "expectedRevision": 0,
  "policy": {
    "accessKind":"on_date", "unlockOn":"2026-10-02",
    "settlementDays":3, "dayBasis":"weekdays",
    "receiptOnOverride":"2026-10-08", "accessibleAmountCap":"123.1234",
    "normalExitFee":"1.0001", "earlyKind":"allowed",
    "earlySettlementDays":2, "earlyDayBasis":"calendar", "earlyFee":"2.0002",
    "earlyAmountMode":"fixed_gross", "earlyGrossAmount":"100.0003",
    "note":"Synthetic withdrawal rules"
  }
}
```

For cash, use `{"kind":"account_cash","accountId":"<account UUID>","currency":"USD"}`.
For a holding, use `{"kind":"holding","accountId":"<owning account UUID>","holdingId":"<holding UUID>"}`.
Copy these identities from the overview rather than inventing them. Only active
asset sources are accepted; archived, liability and unavailable identities fail.
Inspect effective before/after policies and origins, then commit the exact plan:

<!-- example: liquidity-policy-commit -->
```json
{"operationId":"${operationId}","input":{"planId":"${planId}"}}
```

Use commit_save_liquidity_policy for save plans, commit_reset_liquidity_policy
for reset plans. Ten-minute expiry and concurrent writes/restart/restore require
a fresh preview. A successful same-operation/input retry returns the original
receipt; changed input conflicts. A used plan with another operationId is stale.
Unlike managed product business receipts, a pending/unknown ordinary policy
receipt is never automatically replayed. Inspect `get_operation` and current
`get_liquidity_overview` before a new intentional preview; changing the operation
UUID is not a retry strategy. Report recorded-time receipts separately from
current policy. No reservation changes or releases are exposed.

Reset removes the explicit policy and restores the same assumed defaults as
the GUI, without persisting a default row. Use the current explicit revision:

<!-- example: liquidity-policy-reset -->
```json
{"sourceRef":{"kind":"account_value","accountId":"${accountId}"},"expectedRevision":1}
```

The reset preview shows effective defaults with afterOrigin assumed and revision
zero; its committed result is the GUI's revision-zero acknowledgement. Read the
overview again for current defaults, evaluated routes and reservation evidence.

### Global undo availability on history pages

Each row includes `reversedByOperationId`, even when its inverse is off-page.
`undo.operation` is the product-global latest active target, independent of the
requested page. It includes kind and timestamps for an off-page target. A latest
valuation remains the candidate and blocks lifecycle undo. Use `undo.allowed`
and the typed `undo.reason` to explain current GUI availability; do not infer
eligibility by scanning only the loaded rows. No candidate has a null operation,
false and `no_active_operation`; an allowed target has a null reason.

`latestRecordedOperationId` is a live history-head change hint. It is not a frozen
capture, financial-state version, preview or authorization. Restart from the
first page after changes or a changed head, retaining the same productId with a
cursor. Reservation, contract, cash and archive edits can change eligibility
without changing the head. The GUI opens one 25-row page and uses explicit Load
more. Reads create no plans/receipts and return no private receipt evidence.

GUI `undo.allowed` never grants MCP write permission or reservation-restoration
support. Dedicated MCP preview/commit tools require ledger_write and retain their
stricter reservation restrictions. Always obtain a fresh supported preview and
confirm it; preview and commit independently revalidate current facts.

## Scalable liquidity and product reads

See [paging and batches](paging-and-batches.md). Liquidity source filters support
accountIds, typed sourceRefs, sourceKinds, instrumentIds, actual instrumentTypes,
accessKinds, policyOrigins, nullable hasRule/excluded, earlyKinds, currencies and
query. Cash/value sources have null instrument identity/type; never invent cash
as an instrument type. hasRule means saved explicit or contract policy, not any
effective default. assumed=false, explicit/contract=true; unresolved policy facts
are null, including excluded rows whose policy cannot be resolved. Exclusion and
saved-rule origin are independent. policyEditable is business eligibility, not
MCP write permission. Managed contracts require product terms. Copy complete
policy/revision/sourceRef for ordinary full-replacement previews.

Sort liquidity by sourceKey (default asc), displayName, instrumentType,
accessKind, policyOrigin, normalReceiptOn, currentValueBase, or availableBase.
The latter requires sort.horizonOn from captured buckets. Amount keys use exact
same-capture base-currency values; unknowns remain null-last. Household bucket
totals do not change with source filters. Unresolved reservations are household
scope and a separate section, never hidden by source filters.

Paged products retain accountId/includeClosed, filter.productKinds/states/query;
sort startOn (default desc), maturityOn, displayName or state, productId asc ties.
Rows are explicit compact summaries with reservationCount. `get_read_item`
section products returns full ProductDTO/actions/reasons from that capture;
reservations/<productId> pages sort createdAt asc then reservationId asc.
`get_product` also supports responseMode=paged. Do not substitute summary rows
for complete revisioned terms or change live product-operation-history semantics.
