# Page market history, Agent audit rows and import comparison

These reads use `get_market_data`, `list_agent_market_data` and
`compare_market_data_import`. Quote meaning, import and FX source selection stay
in [market data](market-data.md). Shared cursors, expiry and too_large rules stay
in [paging and batches](paging-and-batches.md).

`targets` selects one `get_market_data` capture. A lone instrument or currency
pair is still that tool. Follow each returned section cursor.

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

<!-- example: scale-market-records -->
```json
{"targets":[{"instrumentId":"${instrumentId}"}],"view":"history","fromDate":"2026-09-01","toDate":"2026-09-28","historyLayout":"records","limit":100}
```

`historyLayout: "records"` returns `records` and `targets` sections. Read all
relevant cursors. `chartSelected` marks selected chart membership without
duplicating the raw observation. Dates and exact FX direction and basis rules
are in [market data](market-data.md). Older history calls keep per-target
sections.

<!-- example: scale-agent-retractions -->
```json
{"filter":{"instrumentIds":["${instrumentId}"],"operations":["retract"],"active":false},"sort":{"field":"createdAt","direction":"desc"}}
```

<!-- example: scale-agent-closures -->
```json
{"filter":{"instrumentIds":["${instrumentId}"],"observationKinds":["closed"]},"limit":100}
```

## Compact comparison delivery

`compare_market_data_import` defaults to compact batch results. Read its results
cursor to finish classification delivery. Detailed sections remain deferred, not
empty. Server-computed counts are not evidence-delivery claims. Use
`get_read_item` section=results and id=clientItemId for focused details from the
same capture and follow any child cursors. Optional `limit` (default 25, max
100) bounds initial rows per child section; `sections` selects short child names
`targets`, `matches`, `relations`, `evidence` or `audits`. Omit `sections` for all
five. Unselected names are listed in `availableSections`. Item reads preserve
the capture and do not renew its expiry. Bulk
detail readers can continue result_details, matches, evidence, audits or targets,
or choose view=full upfront. Do not create a second capture and mix its evidence
into the first.

<!-- example: compare-import-summary -->
```json
{"items":[{"clientItemId":"csv/001","instrumentId":"${instrumentId}","kind":"nav","currency":"USD","date":"2026-09-28","value":"1.00000001"}]}
```

<!-- example: compare-import-item -->
```json
{"captureId":"${captureId}","section":"results","id":"csv/001","limit":100}
```

## When a market page fails

`read_expired` or `read_revoked` means discard every page and restart the
original targets, filter or comparison. A too_large single item is not recovered
by a smaller limit and is not an empty section. Comparison pages default to 25
rows, maximum 100. Do not treat deferred detail as delivered evidence. A
summary capture's `delivery.selectionComplete` covers the results section only.
