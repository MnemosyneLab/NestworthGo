# Edit managed-product terms and forecast policy

With ledger_write, discover `preview_product_terms` and commit with `commit_plan`.
Read the contract in [products](products.md). Lifecycle facts use
[product lifecycle](product-lifecycle.md). Ordinary non-managed access rules use
[liquidity policy](liquidity-policy.md).

Read `get_product` immediately before editing and use
summary.detail.product.revision as expectedRevision. Supply complete terms and
policy, including unchanged financial fields and explicit nulls for unknowns.
`terms.shortName` is the only optional field: omitted or null keeps it, and an
empty string clears it. Kind and startOn cannot change. For a closed product
only name and note may change. Financial terms and policy must match current
facts. Original currency is fixed. annualRate is a fraction, or use
annualRatePercent as specified by the App schema. Keep actual/365 versus
actual/360 and interestPaidThroughOn. Supply one rate representation. If both
are present, a nonempty annualRatePercent takes precedence under the existing
GUI rule. Review the canonical annualRate shown in before/after before
committing. Changing predicted interest or availability posts no cash, income,
quantity, cost or quote and never changes reservations.

Review before/after terms, policy and zero netWorthDelta. Preview metadata times
are provisional. Commit recordedAt, UpdatedAt and ConfirmedAt use actual
recording time. Commit only the planId with `commit_plan`. Its immutable product
result describes that edit, without the complete active-reservations array.
Read `get_product` for current facts and all reservations.

This example retains the synthetic deposit's manual maturity amount. Real edits
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

## When the edit fails

`commit_plan` may omit `operationId`. The server derives one from `planId`. An
explicit id is retried unchanged. Submit the plan within ten minutes. A changed
fact, restart or restore makes an uncommitted plan stale. The same plan with a
new explicit operationId returns the original terms receipt and does not write
again. Reuse the same explicit id and input after an interruption, then call
`get_operation` with that id, or with the derived id when you omitted it.
Read `get_product` afterwards. The receipt is not the current reservation list.

Optional reconciliationLink copies the current session, account, item and
financial revisions into the immutable command. A correction never confirms a
product check. Reread terms before linking. See
[session workflow](reconciliation-and-corrections.md#account-by-account-sessions-discover-support-first)
and [recovery](recovery.md).
