# Edit ordinary access and withdrawal rules

Read `get_liquidity_overview` in [products](products.md) and copy the exact
`sourceRef` from a source row. Rules belong to an account value, one
account/currency cash source, or one holding. They are never a global instrument
setting. With `read_write`, discover `preview_save_liquidity_policy` and
`preview_reset_liquidity_policy`. Both plans commit with `commit_plan`. Neither
is available with directory permission. For `policyOrigin: "assumed"`, create
with expectedRevision 0. For an explicit policy, use its current revision.
Managed term_deposit and locked_product rules still use the complete
`preview_product_terms` workflow in [product terms](product-terms.md). Ordinary
writes return managed_position_conflict.

Save is a full replacement. Supply every policy field, using null to clear or
represent unknown. Omission never preserves old values. The discovery schema
may require an explicit null. Fees, caps and gross amounts are exact decimal
strings with up to four fractional digits in the source currency. Zero fee is
known. Null fee is unknown. Settlement counts are 0–365. Calendar counts every
day. Weekdays excludes weekends, without a public-holiday promise. Dates are
civil YYYY-MM-DD. Early allowed requires settlement count, basis, fee and amount
mode. fixed_gross additionally requires its gross amount. These rules change
liquidity forecasts. They do not record withdrawals, income, trades or bank
instructions.

Example creation for one account-value source. The values are synthetic:

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
Copy these identities from the overview. Do not invent them. Only active asset
sources are accepted. Archived, liability and unavailable identities fail.
Inspect effective before/after policies and origins, then commit the exact plan:

<!-- example: liquidity-policy-commit -->
```json
{"operationId":"${operationId}","input":{"planId":"${planId}"}}
```

`commit_plan` reads whether the plan saves or resets. It may omit `operationId`.
The server derives one from `planId`. For a lost response,
[compute the original ID locally](recovery.md#recovering-when-operationid-was-omitted)
and inspect its receipt without resubmitting. An explicit id is retried unchanged.
Ten-minute expiry and a concurrent write, restart or restore invalidate an
uncommitted preview; an uncertain outcome first requires receipt/state inspection.
A successful same-operation and same-input retry returns the original
receipt. Changed input conflicts. A used plan with another explicit operationId
is stale_preview. Unlike managed-product business receipts, a pending or unknown
ordinary policy receipt is never automatically replayed. Inspect `get_operation`
and the current `get_liquidity_overview` before a new intentional preview.
Changing the operation UUID is not a retry strategy. Omitting `operationId`
again for the same planId retries the same derived id. Report recorded-time
receipts separately from current policy. No reservation changes or releases are
exposed.

Reset removes the explicit policy and restores the same assumed defaults as the
GUI, without persisting a default row. Use the current explicit revision:

<!-- example: liquidity-policy-reset -->
```json
{"sourceRef":{"kind":"account_value","accountId":"${accountId}"},"expectedRevision":1}
```

The reset preview shows effective defaults with afterOrigin assumed and revision
zero. Its committed result is the GUI's revision-zero acknowledgement. Read the
overview again for current defaults, evaluated routes and reservation evidence.
See [recovery](recovery.md).
