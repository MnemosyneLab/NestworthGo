# Page directory rows and commit a metadata batch

Directory lists return explicit identity projections. Continue them with
[paging and batches](paging-and-batches.md). Create and edit definitions in
[accounts and instruments](accounts-and-instruments.md).

Members and groups keep IDs, names and archive status. Institutions also keep
type and country. Accounts keep classification, currency, inclusion, lifecycle
and ownership, with institution and group labels. They do not embed latest
values. Holdings keep account and instrument IDs, exact quantity and archive
status. Use valuation tools for current balances.

Instrument rows retain full and short names, type, symbol, market, country,
ISIN, quote currency and unit, quote routing, managed and archive status and
metadataVersion. A shared ticker or display name is not sufficient identity.
Lists omit notes, icons, sort metadata, household IDs and internal timestamps.
For full metadata, call `get_read_item` with the same captureId, section
(`instruments`, `members`, `institutions`, `groups`, `accounts` or `holdings`)
and row id. The response summary is the complete frozen item, including the
instrument metadataVersion and managed flag. Get editable fields there before an
update. Do not treat omitted list fields as empty. Old captures
stay frozen after writes. Refresh before a new write intention or when checking
a stale-version error.

<!-- example: scale-instruments -->
```json
{"filter":{"ids":["${instrumentId}"]},"sort":{"field":"displayName","direction":"asc"},"limit":1}
```

## Commit one metadata batch

Use the ordinary `{operationId, input}` envelope with 1–100 distinct targets.
`operationId` is required on `update_instruments_batch`. Verify every ID,
metadataVersion or revision and the clearing intent before writing. Every item
succeeds or the whole batch fails. Short receipts describe execution, not
current state. The same operationId and identical input return the original
receipt. Changed input conflicts. Pending or unknown must be checked with
`get_operation` and fresh reads before another intentional operation. There is
no automatic replay across the business-write and outer-receipt crash window.
Metadata requires directory_write or ledger_write. No generic arbitrary
executor, provider refresh or financial posting is introduced.

<!-- example: scale-metadata -->
```json
{"operationId":"${operationId}","input":{"items":[{"id":"${instrumentId}","expectedMetadataVersion":"${metadataVersion}","changes":{"shortName":"Fund A","name":"Synthetic fund","note":null}}]}}
```

## When the batch fails

A stale metadataVersion, a managed identity or an archived target rejects the
whole batch. Do not retry a changed input under the same operationId. If the
outcome is pending or unknown, read `get_operation` and a fresh instrument page
before a new operation. `commit_plan` is not this tool. It may omit operationId
only for a stored preview plan.
