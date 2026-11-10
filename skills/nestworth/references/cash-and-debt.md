# Income, spending, transfers, FX and debt

<!-- shared-recipes:start -->
<!-- Generated from internal/agentguidance/recipes.json; update that source. -->
### Shared recipe: cash-and-positions

Completed facts only: exact amounts/currencies and copied IDs. Salary: money_added/income; spending: money_removed/expense (kind/reason). Own-account moves: cash_transfer; balance fixes: reconciliation. Debt: debt_draw/debt_payment; principal is not spending. FX source affects valuation only; fx_conversion needs actual amounts/fees. Valuation is not spendable cash; definitions are not ownership. position_import needs original quantity/cost; trades need executions. Inspect preview, then commit_plan. Local: YYYY-MM-DD/HH:MM. Preserve seconds: RFC3339 effectiveAt, confirmed offset, no local fields. Never truncate or guess.
<!-- shared-recipes:end -->

Use `get_catalog.moneyInReasons` / `moneyOutReasons` and existing account cash.
All examples below are arguments for `preview_batch`; inspect effects and use
`commit_plan` as described in [positions-and-trades.md](positions-and-trades.md).

| Intent | Record |
| --- | --- |
| Salary/other actual income | `money_added`, reason `income` |
| Spending | `money_removed`, reason `expense` |
| Interest credited | `money_added`, reason `interest` |
| Money added from outside the tracked household | `money_added` with the evidenced catalog reason, often `contribution` |
| Moving money between tracked own non-liability accounts | `cash_transfer`, not income/expense |
| Correcting a known target balance | Reconciliation, not invented income/spending |
| Updating a manual valuation of a property/other asset | Reconciliation with the actual target value; not a cash deposit |

Example income:

<!-- example: income -->
```json
{"commands":[{"kind":"money_added","accountId":"${accountId}","amount":"1000","currency":"USD","reason":"income","effectiveAt":"${effectiveAt}"}]}
```

Example expense:

<!-- example: expense -->
```json
{"commands":[{"kind":"money_removed","accountId":"${accountId}","amount":"50","currency":"USD","reason":"expense","effectiveAt":"${effectiveAt}"}]}
```

Income categories describe economic intent, not the mere presence of a cash
increase. If the source was another owned account, use a transfer. Holdings
accounts track cash per currency; reading the account's total valuation does
not establish how much cash is available to spend. Query after each commit.

## Transfers and exchange

`cash_transfer` records `fromAccountId`, `toAccountId`, `sent`, `sentCurrency`,
`received`, `receivedCurrency` and an optional separate `fee`/`feeCurrency`.
It supports same-currency and cross-currency actual transfers. Obtain the actual
amounts on both sides; an estimated current FX rate does not establish settlement.
Both endpoints must be non-liability accounts. For borrowing or repayment with
a liability endpoint, use `debt_draw` or `debt_payment`; a cash transfer cannot
represent debt principal. Same-currency principal amounts must match; separate
fees reduce source cash in addition to principal.

“账户内用 100 美元换成 90 欧元，另付 1 美元手续费”:

<!-- example: fx-conversion -->
```json
{"commands":[{"kind":"fx_conversion","accountId":"${accountId}","sold":"100","soldCurrency":"USD","bought":"90","boughtCurrency":"EUR","fee":"1","feeCurrency":"USD","effectiveAt":"${effectiveAt}"}]}
```

The example consumes 101 USD and adds 90 EUR. Clarify whether charges are
separate or already deducted. Confirm that the account supports the currencies
and sufficient balances. `set_fx_source` or importing an FX quote changes
valuation data, not currency balances. Exchange spread/fees can affect asset
changes without being a household external principal flow.

## Credit cards, loans and repayments

`debt_draw` takes `debtAccountId`, `cashAccountId`, `principal`,
`principalCurrency`: borrowing increases debt and cash; principal is not income.
`debt_payment` reduces debt principal and cash, with optional separate
`interestOrFee` / `interestOrFeeCurrency`. Repaid principal is not expense;
interest/fees have a different effect.

Example repayment:

<!-- example: debt-payment -->
```json
{"commands":[{"kind":"debt_payment","debtAccountId":"${debtAccountId}","cashAccountId":"${accountId}","principal":"100","principalCurrency":"USD","interestOrFee":"10","interestOrFeeCurrency":"USD","effectiveAt":"${effectiveAt}"}]}
```

Credit-card purchase recording is not an arbitrary cash expense from a liability
account. The current generic debt draw requires a cash counterpart; do not
invent one to represent a card purchase. Where the transaction has no supported
MCP shape, guide the user to the App or reconcile the evidenced current debt,
explaining that reconciliation will not reproduce itemized spending history.

Keep debt balances in the App's positive owed-principal convention. Inspect the
preview to ensure cash and debt move in the intended directions. For historical
records, later repayments/balances must still be valid; the App validates replay.

For an active reconciliation session, choose a direct target adjustment or a
missing real income/expense/transfer using the user's intent. Attach the current
reconciliationLink to the existing preview; inspect committed links, reread and
explicitly check again. See [session workflow](reconciliation-and-corrections.md#account-by-account-sessions-discover-support-first).

## Exact tool arguments

Use `preview_batch` followed by `commit_plan` for both one and many ordinary
ledger records. Do not use a separate direct cash-write tool. An authorized
intent does not need an extra user confirmation merely because there are two
tool calls; inspect the preview before committing.

`reason` is a machine enum, not a free-text category: salary uses `income` and
its description belongs in `note`. Trade `side` is `buy` or `sell`. Supply only
fields for the selected `kind`. Use either `effectiveAt` or the paired
`effectiveLocalDate`/`effectiveLocalTime`, never both. Optional fees require both
amount and currency; omit both when no fee is being recorded.

For `preview_correction`, `action=fix` requires a complete replacement without
timestamps; `action=undo` forbids replacement. A correction trade requires an
existing holding ID; importing a position is not a correction replacement.
