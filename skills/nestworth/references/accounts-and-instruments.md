# Accounts, ownership and instrument identity

For “创建券商/银行/信用卡账户” read `get_catalog` and existing members,
institutions, groups and accounts. Resolve the actual owners and allocation;
do not assign the chatting user or split jointly owned assets without evidence.
An institution identifies the bank/broker; a group is a separate organizational
label. Create missing definitions first and retain their committed IDs.

Choose from `accountCombinations`; account type, asset/liability role and tracking
mode are a validated combination. Bank balances commonly use `balance`,
brokerages use `holdings`, and property/insurance valuations can use a supported
`manual_value` combination. Credit cards/loans are liabilities; do not enter debt
as a negative cash balance or record borrowed principal as income. Explain the
net-worth/portfolio/liquid-assets inclusion flags. Tracking mode is immutable.

`create_account` creates an empty account: `initialAmount` must be omitted/zero
for balance/manual-value accounts and omitted for holdings accounts. Establish
opening state through the App before starting history, or use the appropriate
authorized ledger/reconciliation operation after history has started. A real
deposit from another owned account is a transfer, not new household income.

Example `create_account`, choosing a sole owner explicitly:

<!-- example: create-brokerage -->
```json
{"operationId":"${operationId}","input":{"name":"Example broker","accountType":"brokerage","balanceSheetRole":"asset","trackingMode":"holdings","defaultCurrency":"USD","includeInNetWorth":true,"includeInPortfolio":true,"includeInLiquidAssets":false,"ownerIds":["${memberId}"]}}
```

After creation query `list_accounts` / `get_account_snapshot`. Directory
creation is not atomic with subsequent funding or trades. If a later step
fails, recover that step; do not recreate the account.

## Instruments

Call `list_instruments` first. Use name, market/exchange, currency, share class,
ISIN or provider identifier as available. Match a crypto asset by chain/coin
identity as well as symbol. External search currently covers stock/ETF/crypto;
it does not guarantee a match for every Chinese fund or bank product.

| User asset | Definition and evidence |
| --- | --- |
| US stock | `stock`; confirm exchange/market, symbol and currency; use a returned supported provider binding |
| ETF | `etf`; distinguish listing market, trading currency and share class |
| Cryptocurrency | `crypto`; retain the search result's actual provider coin identifier, not an assumed ticker mapping |
| Chinese mutual fund | `mutual_fund`; confirm fund code/share class and CNY unit NAV; use `agent` supply when no suitable provider exists |
| Ordinary NAV-priced bank wealth product | `bank_investment_product`; confirm units, unit NAV and currency; never substitute advertised yield for price |
| Bond | `bond`; confirm supported quantity and price units; use sourced observations if no provider mapping exists |
| Gold/silver | `precious_metal`; choose `metalTemplate` and `quantityUnit` (`g` or `troy_oz`) and currency; local-currency conversion needs USD FX |
| Other asset | `other` only when its quantity, price and valuation interpretation are explicit |

Example `create_instrument` for a fund supplied by Agent observations:

<!-- example: create-fund -->
```json
{"operationId":"${operationId}","input":{"name":"Example fund A","type":"mutual_fund","quoteCurrency":"CNY","quoteSource":"agent","symbol":"${fundCode}","marketCode":"CN","countryCode":"CN"}}
```

`provider` requires a supported key/symbol binding. `agent` means Agent-only
quotes and no normal provider pulls. Existing manual/provider preferences can
also accept Agent overlays; adding an observation need not change preference.
There is no generic MCP manual-quote setter; use `import_market_data` for sourced
Agent observations or the App for manual entry.

Creating a definition does not create ownership or a trade. Continue with
[positions-and-trades.md](positions-and-trades.md) or [market-data.md](market-data.md).

For updates, omitted fields are preserved; supplied ownership replaces the
allocation. Instrument `replace=true` requires the complete form, so prefer
partial changes for a narrow edit. Institution updates currently rename only.
Use the relevant `set_*_icon` tools for member/institution/group icons; account
and instrument icons use updates. Archive/restore uses `archive_*` with
`archived:true/false` and preserves history; it does not delete references.

App-managed term deposits and locked/closed wealth products have specialized
cash, maturity, reservation and redemption rules. Current MCP does not expose
their full lifecycle or permit generic edits to their managed holdings/quotes.
Guide the user to the product workflow in the App, even if a similarly named
ordinary instrument could be created.
