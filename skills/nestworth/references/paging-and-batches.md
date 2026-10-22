# Frozen management reads and atomic batches

Discover tools first. Liquidity, products, instruments, upcoming actions, Agent
records, account valuations, directory lists/holdings and health/repair reads
return mcp-read/1 by default. There is one contract; do not pass responseMode.
Historical/current overview reads default to five initial items per section;
continuations default to 25. Pure minimal financial-context workflows retain
their disclosure path. Inspect permissions before writes.

## Completeness and recovery

A `mcp-read/1` response has captureId, captureKind, capturedAt, expiresAt,
contentHash, summaryHash, normalized query, summary and sections. Each section
has items and page {total, returned, hasMore, nextCursor}. Empty arrays mean no
matches only when hasMore=false and nextCursor=null. A deferred section may have
returned=0 and a cursor. Follow every relevant cursor using `get_read_page` with
captureId, section, cursor and optional limit; filter/sort cannot change mid-read.
Default limit is 25, maximum 100; actual pages may be smaller to fit 65,536 final
HTTP bytes, including duplicated SDK text and structured content. Never claim
“all” from one page. Preserve captureId/contentHash and check complete unions.

Captures have fixed five-minute TTL without renewal, 4 MiB capture budget,
16 MiB total cache, eight entries, two builders and a ten-second build budget.
SQL reads also preflight row/payload budgets. Expired/evicted captures return
read_expired; connection, permission, restore or restart revocation returns
read_revoked. Discard every old page and restart the original query. Invalid,
tampered, wrong-section/target/capture cursors return validation. Do not stitch
captures. too_large distinguishes summary, single_item and capture; an oversized
single item is never skipped or truncated. Lowering limit cannot fix that item.
Send one JSON-RPC object per business call; JSON-RPC array batches are rejected.

Arrays within one filter are OR, different fields AND, empty arrays unrestricted.
Unknowns remain null and do not match either hasRule=true or hasRule=false.
Sorting is global before pages, null-last in both directions, with fixed identity
ascending ties. Names compare trimmed case-insensitive text without locale;
amounts/percentages use exact decimals. IDs determine write identity, not labels.

## Batch writes

Use the ordinary {operationId,input} envelope with 1–100 distinct targets.
Verify all IDs, metadataVersion/revision and clearing intent before writing.
Every item succeeds or the whole batch fails. Short receipts describe execution,
not current state. Same operationId and identical input return the original
receipt; changed input conflicts. Pending/unknown must be checked with
`get_operation` and fresh reads before another intentional operation; no automatic
replay across the business-write/outer-receipt crash window. Metadata requires
directory_write or ledger_write. No generic arbitrary executor, provider refresh or
financial posting is introduced.

## Executable argument examples

Replace UUID/version placeholders from current reads. Dates are examples; use
the user's household-local civil dates. Cursor examples use returned section
names exactly, including reservations/<productId> or a market target section.

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

<!-- example: scale-page -->
```json
{"captureId":"${captureId}","section":"${section}","cursor":"${cursor}","limit":25}
```

<!-- example: scale-market-current -->
```json
{"targets":[{"instrumentId":"${instrumentId}"},{"baseCurrency":"USD","quoteCurrency":"SGD"}],"view":"current","limit":1}
```

<!-- example: scale-market-coverage -->
```json
{"targets":[{"instrumentId":"${instrumentId}"},{"baseCurrency":"SGD","quoteCurrency":"USD"}],"view":"coverage","range":"30d","limit":1}
```

<!-- example: scale-market-history -->
```json
{"targets":[{"instrumentId":"${instrumentId}"},{"baseCurrency":"SGD","quoteCurrency":"USD"}],"view":"history","range":"30d","limit":1}
```

<!-- example: scale-agent-retractions -->
```json
{"filter":{"instrumentIds":["${instrumentId}"],"operations":["retract"],"active":false},"sort":{"field":"createdAt","direction":"desc"}}
```

<!-- example: scale-instruments -->
```json
{"filter":{"ids":["${instrumentId}"]},"sort":{"field":"displayName","direction":"asc"},"limit":1}
```

<!-- example: scale-metadata -->
```json
{"operationId":"${operationId}","input":{"items":[{"id":"${instrumentId}","expectedMetadataVersion":"${metadataVersion}","changes":{"shortName":"Fund A","name":"Synthetic fund","note":null}}]}}
```

<!-- example: scale-empty-match -->
```json
{"filter":{"policyOrigins":["explicit"],"hasRule":false}}
```

## Reconciliation evidence

Discover `reconciliation_session_read`. `list_reconciliation_sessions` filters
status/account/date and sorts asc/desc. `get_reconciliation_session_page` pages
accounts/items/changes/links/attempts, with account/outcome filters and sort.
Default 25/max 50; the wire budget is 96 KiB. Reduce limit on response_too_large;
never treat a rejected response as an empty section. Cursors bind the filters,
sort, full current evidence and session revision. stale_cursor means restart
that read, not guess an offset. Item summaries expose attemptCount; full item
evidence and pageable attempts are separate. Preserve recorded and effective
dates on changes. First sessions have no prior successful-check watermark.
