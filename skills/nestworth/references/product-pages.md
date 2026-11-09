# Page managed products and liquidity sources

Use these filters on `get_products` and `get_liquidity_overview` after the
read steps in [products](products.md). Shared capture, cursor and expiry rules
are in [paging and batches](paging-and-batches.md). Continue with
`get_read_page({cursor, limit?})`.

<!-- example: scale-liquidity-funds -->
```json
{"filter":{"instrumentTypes":["mutual_fund","bank_investment_product"],"policyOrigins":["explicit"]},"sort":{"field":"displayName","direction":"asc"},"limit":1}
```

<!-- example: scale-liquidity-unconfigured -->
```json
{"filter":{"hasRule":false},"limit":1}
```

<!-- example: scale-products -->
```json
{"limit":1}
```

<!-- example: scale-product-item -->
```json
{"captureId":"${captureId}","section":"products","id":"${productId}"}
```

<!-- example: scale-empty-match -->
```json
{"filter":{"policyOrigins":["explicit"],"hasRule":false}}
```

Liquidity source filters support accountIds, typed sourceRefs, sourceKinds,
instrumentIds, actual instrumentTypes, accessKinds, policyOrigins, optional boolean
hasRule and excluded (omit for no filter; true/false are accepted, null is
rejected), earlyKinds, currencies and query. Managed sources also
expose nullable productKind from the same contract capture. It stays separate
from the actual instrumentType. Cash and value sources have null instrument
identity, type and productKind. Never invent cash as an instrument type.
hasRule means a saved explicit or contract policy, not any effective default.
assumed is false. explicit and contract are true. Unresolved policy facts are
null, including excluded rows whose policy cannot be resolved. Exclusion and
saved-rule origin are independent. policyEditable is business eligibility, not
MCP write permission. Managed contracts require product terms. Copy the complete
policy, revision and sourceRef for an ordinary full-replacement preview in
[liquidity policy](liquidity-policy.md).

Sort liquidity by sourceKey (default asc), displayName, instrumentType,
accessKind, policyOrigin, normalReceiptOn, currentValueBase or availableBase.
availableBase requires sort.horizonOn from captured buckets. Amount keys use
exact same-capture base-currency values. Unknowns remain null-last. Household
bucket totals do not change with source filters. Unresolved reservations are
household scope and a separate section. Source filters never hide them.

Paged products retain accountId and includeClosed, plus filter.productKinds,
states and query. Sort startOn (default desc), maturityOn, displayName or state,
with productId ascending as the tie. Rows are explicit compact summaries with
reservationCount. `get_read_item` section products returns the full ProductDTO,
actions and reasons from that capture. reservations/<productId> pages sort
createdAt ascending, then reservationId ascending. `get_products` returns bounded
pages. Do not substitute summary rows for complete revisioned terms, and do not
change live product-operation-history semantics.

## When a page fails

An empty filter match is an empty section only when hasMore is false and
nextCursor is null. `scale-empty-match` asks for explicit origin and hasRule
false together, which cannot describe one row. A deferred section can return
zero rows and a cursor. Follow it. An oversized summary or single item fails
with too_large. Lowering limit does not recover that item. Do not omit the row
or treat the failure as absence. Discard an expired or revoked capture and
restart the original filtered read. Do not stitch two captures.
