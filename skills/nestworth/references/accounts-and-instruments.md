# Accounts, ownership and instrument identity

<!-- shared-recipes:start -->
<!-- Generated from internal/agentguidance/recipes.json; update that source. -->
### Shared recipe: identity

Resolve an investment by saved ID, full name, market, quote currency and share class, not ticker or alias alone. A definition creates neither a holding nor a trade. Managed product instruments use their product workflow. For directory edits, omission keeps a field, null clears only explicitly clearable fields, and an empty string does not mean clear. Directory mutations are direct operations; they do not produce a ledger plan.
<!-- shared-recipes:end -->

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

For a named account question, search `list_accounts` with the account or bank
name. Search matches account, institution and group names. Do not scan all
accounts or fetch institutions again when the returned identities suffice.
With known IDs, call `get_account_valuations` directly. It is the only current
account valuation entry: one or many `accountIds`, optional account filters.
Unknown/archived IDs and unmatched filters produce an empty accounts section.

Default `detail="summary"` returns exact totals, inclusion flags, ownership,
asset types and price-freshness counts/date range. This normally answers account
status questions without reading each holding. `detail="holdings"` also returns
the first page of components and missing inputs in the same frozen capture.
Only follow cursors for relevant unfinished sections. Do not fetch account totals
again after they have already been read in this turn unless facts changed.

<!-- example: account-summary -->
```json
{"accountIds":["${accountId}"],"detail":"summary"}
```

<!-- example: account-holdings -->
```json
{"accountIds":["${accountId}"],"detail":"holdings","limit":25}
```

`get_read_item` with captureId, section=accounts and id opens frozen full account
metadata and inlines components/missing inputs. Optional `sections` selects
child names instead; `availableSections` lists other evidence without minting
unused cursors. Use those only when the requested analysis needs them.

<!-- example: account-evidence -->
```json
{"captureId":"${captureId}","section":"accounts","id":"${accountId}","sections":["display_fxEvidence"],"limit":25}
```

Directory and holding lists use the same bounded contract. `get_read_item`
retains full notes and metadata. Never add account rollups to their components;
current value is not a return or endpoint movement. Freshness counts describe
price evidence, not bank real-time balances; missing inputs remain unknown.
After creation read the affected account. Directory creation is not atomic with
funding or trades: recover a failed later step without recreating the account.

## Instruments

`shortName` is optional display metadata, not an instrument identity. The primary
label is trimmed short name, otherwise symbol/code, otherwise full name, for
every instrument type. Duplicate aliases are allowed: resolve using saved IDs,
full name, symbol/code, market, currency and ISIN. To edit only an alias, pass
`changes.shortName` to `update_instrument`; JSON null clears it and omission
keeps it. An empty string is rejected. Managed product aliases
use the product contract's terms-edit path; do not edit its internal instrument
independently.

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

Patch updates share one rule. Omit a field to keep the stored value. JSON null
clears a clearable field. An empty string is rejected: it does not clear and it
does not keep. `update_account` clears institutionId, groupId, note, iconKey,
openedOn or closedOn with null, for example `{"note":null}`. Null and an empty
string are rejected for name, classification, currency, inclusion flags and
ownership. Supplied ownership replaces the whole allocation and cannot be
empty. `initialAmount` does not rewrite balance history.

Example `update_account` clearing only the note:

<!-- example: account-clear-note -->
```json
{"operationId":"${operationId}","input":{"id":"${accountId}","changes":{"note":null}}}
```

Use `update_member`, `update_institution` or `update_group` with the record's
`id` and at least one of `name`, `iconKey` and `archived`. Omitted fields keep
their stored values, and when name and icon are both supplied they apply
together. `archived: true` archives and `archived: false` restores; omit
`archived` to keep the stored archive state. Null is rejected for name, icon
and archived, because those fields cannot be cleared. A household must keep at
least one active member, and a member who is the only active owner of a live
account cannot be archived. Use a non-empty name and a supported non-empty icon
key. An institution's `institutionType` cannot be edited.

Example `update_institution` to change only the icon:

<!-- example: institution-icon -->
```json
{"operationId":"${operationId}","input":{"id":"${institutionId}","iconKey":"bank"}}
```

`update_instrument` is the same partial edit. JSON null clears shortName,
symbol, marketCode, countryCode, isin, note, providerKey and providerSymbol,
and resets iconKey to the type default. Clearing marketCode/provider fields
is rejected when the final configuration requires or defaults them: gold/silver
templates retain their required COMEX/Yahoo binding, and a CoinGecko binding
requires CRYPTO marketCode. These are validation failures, not reset operations;
the entire patch is rejected without applying other fields. Provider-source
instruments still require providerKey and providerSymbol. Omit binding fields
to keep them, or make an explicitly intended valid configuration change.
Null is rejected for name, type,
quoteCurrency, metalTemplate, quantityUnit, quoteSource and sortOrder. There
is no replace flag. Account and instrument icons use their update tools. Use
`archive_account` or `archive_instrument` with `archived:true/false` to archive
or restore while preserving history and references. Member, institution and
group archive uses `archived` on `update_member`, `update_institution` or
`update_group`.

App-managed term deposits and locked/closed wealth products have specialized
cash, maturity, reservation and redemption rules. MCP exposes dedicated
`preview_product_operation` and `commit_plan` for supported
completed lifecycle facts, plus dedicated revisioned terms and actual locked
value edits. Renewal without reservation release uses preview_product_operation kind renew
and `commit_plan`. Reservation-affecting renewal and reservation
creation/edit/release remain App workflows.
Never use generic trades, reconciliation or quote tools to bypass managed
holdings/quote guards, even if a similarly named ordinary instrument could be
created. See [products.md](products.md) for reads and [product-lifecycle.md](product-lifecycle.md) for supported writes.

## Frozen discovery and metadata batches

See [paging and batches](paging-and-batches.md) and [directory batches](directory-batches.md). Paged `list_instruments` supports
filter.types/ids and either top-level query or filter.query (not both),
plus top-level includeArchived. Sort displayName (default asc), name,
type, updatedAt or id sorting. Each row has metadataVersion and managed status.
Resolve exact IDs and versions before `update_instruments_batch`. Only name,
shortName and note are accepted. Omission keeps a field. JSON null clears
shortName and note. An empty string is rejected. name cannot be empty or null.
Duplicate aliases are allowed,
duplicate targets are rejected. One stale/managed/archived target rejects every
change. Managed identities require product terms. No quotes, positions, source
preferences or ledger facts change. read_write is required.


Directory and instrument lists are compact identity projections. Full notes,
display metadata and timestamps require `get_read_item` with the original
captureId, row id and matching section (`instruments` for instruments).
Read full metadata before an update; omission from a compact row
does not mean an empty field, and it does not clear one. Instrument list metadataVersion remains available
for atomic metadata updates. See [directory batches](directory-batches.md).
