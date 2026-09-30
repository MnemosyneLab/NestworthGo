# Income, spending, transfers, FX and debt

Use `get_catalog.moneyInReasons` / `moneyOutReasons` and existing account cash.
All examples below are arguments for `preview_change`; inspect effects and use
`commit_change` as described in [positions-and-trades.md](positions-and-trades.md).

| Intent | Record |
| --- | --- |
| Salary/other actual income | `money_added`, reason `income` |
| Spending | `money_removed`, reason `expense` |
| Interest credited | `money_added`, reason `interest` |
| Money added from outside the tracked household | `money_added` with the evidenced catalog reason, often `contribution` |
| Moving money between tracked own accounts | `cash_transfer`, not income/expense |
| Correcting a known target balance | Reconciliation, not invented income/spending |
| Updating a manual valuation of a property/other asset | Reconciliation with the actual target value; not a cash deposit |

Example income:

<!-- example: income -->
```json
{"kind":"money_added","accountId":"${accountId}","amount":"1000","currency":"USD","reason":"income","effectiveAt":"${effectiveAt}"}
```

Example expense:

<!-- example: expense -->
```json
{"kind":"money_removed","accountId":"${accountId}","amount":"50","currency":"USD","reason":"expense","effectiveAt":"${effectiveAt}"}
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

“账户内用 100 美元换成 90 欧元，另付 1 美元手续费”:

<!-- example: fx-conversion -->
```json
{"kind":"fx_conversion","accountId":"${accountId}","sold":"100","soldCurrency":"USD","bought":"90","boughtCurrency":"EUR","fee":"1","feeCurrency":"USD","effectiveAt":"${effectiveAt}"}
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
{"kind":"debt_payment","debtAccountId":"${debtAccountId}","cashAccountId":"${accountId}","principal":"100","principalCurrency":"USD","interestOrFee":"10","interestOrFeeCurrency":"USD","effectiveAt":"${effectiveAt}"}
```

Credit-card purchase recording is not an arbitrary cash expense from a liability
account. The current generic debt draw requires a cash counterpart; do not
invent one to represent a card purchase. Where the transaction has no supported
MCP shape, guide the user to the App or reconcile the evidenced current debt,
explaining that reconciliation will not reproduce itemized spending history.

Keep debt balances in the App's positive owed-principal convention. Inspect the
preview to ensure cash and debt move in the intended directions. For historical
records, later repayments/balances must still be valid; the App validates replay.
