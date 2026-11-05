# Record an actual locked-product value

With ledger_write, discover `preview_product_valuation` and commit with
`commit_plan`. Read the contract in [products](products.md). Only an open
locked_product accepts observations. A term_deposit current value must not
include projected interest. Supply the actual total value as a positive decimal
string in the contract currency, not NAV units. observedAt is the actual RFC3339
instant. Empty freezes App now at preview. Historical dates and timezone
semantics follow GUI validation. After validating the supplied instant, preview
converts it to UTC and truncates submillisecond precision to match SQLite.
Review and commit that frozen millisecond command. Quote and receipt creation
time is actual commit time at the same UTC millisecond precision.

Review the normalized frozen command and the before/after current value. A
historical quote may leave today's value unchanged. An unknown before value
means unknown netWorthDelta, never zero. No cash, quantity, cost or reservation
effect occurs. Commit only the planId. The immutable receipt retains the
original quote, operation and product IDs, amount and times across repeats,
later observations and restart. Query `get_product` separately for today's
facts. Derived analysis can remain dirty after historical observations. Recovery
does not require reposting. The ordinary managed trade and quote-import guards
remain.

<!-- example: product-valuation -->
```json
{"productId":"${productId}","amount":"1100","observedAt":""}
```

<!-- example: product-valuation-commit -->
```json
{"operationId":"${operationId}","input":{"planId":"${planId}"}}
```

## When the observation fails

`commit_plan` may omit `operationId`. The server derives one from `planId`. An
explicit id is retried unchanged. A changed fact, restart or restore makes an
uncommitted plan stale. The same plan with a new explicit operationId returns
the original valuation receipt and does not write another observation. Reuse
the same explicit id and input after an interruption. Call `get_operation` with
that id, or with the derived id when you omitted it, then read `get_product`.
Do not repost because today's value differs from a historical observation.

Optional reconciliationLink copies the current session, account, item and
financial revisions into the immutable command. A correction never confirms a
product check. Due or matured is not proof of the observed value. See
[session workflow](reconciliation-and-corrections.md#account-by-account-sessions-discover-support-first)
and [recovery](recovery.md).
