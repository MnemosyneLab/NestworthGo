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

Use the default bounded contracts for list_members, list_institutions,
list_groups, list_accounts and list_holdings. Existing filters still apply;
follow every cursor. get_read_item with the section and id preserves full notes
and metadata from that capture. get_account_valuations uses the bounded contract:
compact account headers, with separate component/composition/missing/evidence
sections reached through get_read_item accounts/id. Never add account rollups to
their component amounts.

After creation query `list_accounts` / `get_account_snapshot`. Directory
creation is not atomic with subsequent funding or trades. If a later step
fails, recover that step; do not recreate the account.

## Instruments

`shortName` is optional display metadata, not an instrument identity. The primary
label is trimmed short name, otherwise symbol/code, otherwise full name, for
every instrument type. Duplicate aliases are allowed: resolve using saved IDs,
full name, symbol/code, market, currency and ISIN. To edit only an alias, pass
`changes.shortName` to `update_instrument`; an empty string clears it and omission
preserves it, including older complete-form requests. Managed product aliases
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

For updates, omitted fields are preserved; supplied ownership replaces the
allocation. `update_account` clears institutionId, groupId, note, iconKey,
openedOn or closedOn only with the matching `<field>Set:true` (for example
`{"note":"","noteSet":true}`); an empty value without `Set` keeps the stored one.
Use `update_member`, `update_institution` or `update_group` with the record's
`id` and at least one of `name`, `iconKey` and `archived`. These are partial
updates: omitted fields keep their stored values, and when name and icon are
both supplied they apply together. `archived: true` archives and
`archived: false` restores; omit `archived` to keep the stored archive state.
A household must keep at least one active member, and a member who is the only
active owner of a live account cannot be archived. Use a non-empty name and a
supported non-empty icon key. An institution's `institutionType` cannot be edited.

Example `update_institution` to change only the icon:

<!-- example: institution-icon -->
```json
{"operationId":"${operationId}","input":{"id":"${institutionId}","iconKey":"bank"}}
```

Instrument `replace=true` requires the complete form, so prefer partial changes
for a narrow edit. Account and instrument icons use their update tools. Use
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
created. See [products.md](products.md) for supported actions and boundaries.

## Frozen discovery and metadata batches

See [paging and batches](paging-and-batches.md). Paged `list_instruments` supports
filter.types/ids and either top-level query or filter.query (not both),
plus top-level includeArchived. Sort displayName (default asc), name,
type, updatedAt or id sorting. Each row has metadataVersion and managed status.
Resolve exact IDs and versions before `update_instruments_batch`. Only name,
shortName and note are accepted. Omission preserves; empty shortName clears;
note=null clears; name cannot be empty/null. Duplicate aliases are allowed,
duplicate targets are rejected. One stale/managed/archived target rejects every
change. Managed identities require product terms. No quotes, positions, source
preferences or ledger facts change. directory_write or ledger_write is required.


Directory and instrument lists are compact identity projections. Full notes,
display metadata and timestamps require `get_read_item` with the original
captureId, row id and matching section (`instruments` for instruments).
Read full metadata before a complete replacement; omission from a compact row
does not mean an empty field. Instrument list metadataVersion remains available
for atomic metadata updates. See [paging and batches](paging-and-batches.md).
