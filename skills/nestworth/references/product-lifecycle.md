# Record a completed managed-product fact

Discover `preview_product_operation` and `commit_plan` only with ledger_write.
Read the contract first in [products](products.md). GUI permittedActions describe
GUI options. Tool discovery and permission govern MCP. These tools record facts
already completed. They never send bank instructions. Open, record_existing,
receive_interest, whole-contract settle, renewal without a reservation change,
and the latest safe group undo are supported. Terms and value use their own
preview/commit pairs. Reservation creation, edit and release still require the
App. Generic trades, corrections, reconciliation or quote imports cannot
substitute for a managed operation. Never fill releaseReservationIds or silently
release or restore reserves.

Use exactly one matching payload. Money is original-currency decimal strings.
For an existing product, original totalCostBasis and currentValue are explicit.
`cashExcludesProduct=true` confirms account cash already excludes this product.
Do not double-count the deposit in cash. Recording time is frozen by the server.
startOn describes the contract and does not backdate the holding acquisition.
For open, interest and settle, empty effectiveAt freezes App now. Alternatively
supply an actual RFC3339 instant, or effectiveLocalDate and effectiveLocalTime
in the history timezone. Undo time is server-frozen. Settle closes the entire
contract, even if returned principal differs. It never means partial redemption.
Forecast locks do not prevent recording an actual early receipt. Active reserves,
or an undo that would restore reserves, return unresolved_reservation_release.
Use the GUI.

The following synthetic examples use USD, principal 1000 and manual maturity
interest 50. Replace dates, currency, complete terms, policy and amounts with
the user's actual contract. Do not infer rates, costs, cash confirmation or fees.

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

Renew when no reservation release is required. Supply the actual settlement
facts and the complete new principal, opening fee, terms and policy. The preview
shows the atomic predecessor closure and successor opening. Do not assume a
maturity date implies renewal or receipt. Reservation-affecting renewal remains
a GUI workflow.

For a reconciliation session, this preview accepts optional reconciliationLink.
Copy the current session, account, item and financial revisions. The link is
part of the immutable command and request hash, and commits in the financial
transaction. A correction never confirms a product check. Reread terms,
principal, lifecycle and current evidence first. Due or matured is not proof of
actual receipt. See [session workflow](reconciliation-and-corrections.md#account-by-account-sessions-discover-support-first).

## Review, commit and recover

Review the stored normalized command, cash, value and net-worth effects, actual
and forecast interest, dates and warnings. Submit only the matching planId
within ten minutes. Draft product and activity IDs are provisional. A changed
fact, restart or restore makes an uncommitted plan stale.

<!-- example: product-commit -->
```json
{"operationId":"${operationId}","input":{"planId":"${planId}"}}
```

`commit_plan` may omit `operationId`. The server derives one from `planId`. An
explicit id is stored and retried unchanged. Reuse that same id and input to
recover an interruption. The same plan with a new explicit operationId returns
the original immutable business receipt and does not post again. Call
`get_operation` with the id you sent, or the [locally derived ID](recovery.md#recovering-when-operationid-was-omitted) when you omitted it.
`list_product_operations` returns business operation UUIDs. `get_product` reads
current facts. See [recovery](recovery.md).

Undo preserves the original events and adds compensation at the reviewed time.
It does not historically replace them. Settlement then undo can retain a
different instrument time-weighted exposure, capital and return rate from never
settling. With cash included, the household or account principal transfer
between product and cash is internal and does not create external capital flows.
Isolated interest undo does not change invested capital. Managed products
currently have no historical fix path. Resolve mistakes within GUI support.
Never use generic fix tools that bypass managed protection. An integrity_failed
receipt is not a failed financial transaction to retry under a fresh intent.
Inspect current facts and use the GUI. Do not repost to repair metadata.
