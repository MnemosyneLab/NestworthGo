# Business pagination and atomic batches

Liquidity, products, instruments, upcoming actions, Agent
records, account valuations, directory lists/holdings, scan_data_health and
preview_data_repair return mcp-read/1 by default. get_data_repair_job defaults to
compact progress without a capture; only view=evidence uses mcp-read/1. There is one contract; do not pass responseMode.
Historical/current overview reads default to five initial items per section;
continuations default to 25. Pure minimal financial-context workflows retain
their disclosure path.

## Completeness and recovery

An initial `mcp-read/1` capture response has captureId, captureKind, capturedAt,
expiresAt, contentHash, summaryHash, normalized query, summary and sections. Each section
has items and page {total, returned, hasMore, nextCursor}. Empty arrays mean no
matches only when hasMore=false and nextCursor=null. A deferred section may have
returned=0 and a cursor. Follow every relevant cursor using `get_read_page` with only cursor and optional
limit; the signed cursor retains capture/section/target/query. Filters and sort
cannot change mid-read.
`get_read_page` returns the same capture identity/hash/expiry plus section,
items and page; it does not repeat query, summary or unrelated sections. Retain
the initial header, append only the returned section and update only that
section's saved page state.
Default limit is 25 for all business continuations; maximum 100. Actual pages
may be smaller to fit 65,536 final HTTP bytes, including duplicated SDK text and
structured content. For a complete bulk import/list audit, use limit=100 upfront
and on continuations
when that evidence is needed; this reduces per-call overhead while retaining the
same byte cap. Default 25 favors focused reads. Never claim
“all” from one page. Preserve captureId/contentHash and check complete unions.

Captures have fixed five-minute TTL without renewal, 4 MiB capture budget,
16 MiB total cache, eight entries, two builders and a ten-second build budget.
SQL reads also preflight row/payload budgets. Expired/evicted captures return
read_expired; a revoked generation returns read_revoked. Reconnection, permission
changes, restore or restart invalidate old captures; lost caches may report
read_expired and revoked credentials may fail authentication. Discard every old page and restart the original query. Invalid,
tampered, wrong-section/target/capture cursors return validation. Do not stitch
captures. too_large distinguishes summary, single_item and capture; an oversized
single item is never skipped or truncated. Lowering limit cannot fix that item.
Send one JSON-RPC object per business call; JSON-RPC array batches are rejected.

Arrays within one filter are OR, different fields AND, empty arrays unrestricted.
Unknowns remain null and do not match either hasRule=true or hasRule=false.
Sorting is global before pages, null-last in both directions, with fixed identity
ascending ties. Names compare trimmed case-insensitive text without locale;
amounts/percentages use exact decimals. IDs determine write identity, not labels.

## Compact directory identity and details

Directory lists return explicit identity projections. Members/groups keep IDs,
names and archive status; institutions also keep type/country. Accounts keep
classification, currency, inclusion, lifecycle and ownership, with institution/
group labels; they do not embed latest values. Holdings keep account/instrument
IDs, exact quantity and archive status. Use valuation tools for current balances.

Instrument rows retain full/short names, type, symbol, market/country/ISIN,
quote currency/unit, quote routing, managed/archive status and metadataVersion.
A shared ticker or display name is not sufficient identity. Lists omit notes,
icons, sort metadata, household IDs and internal timestamps. For full metadata,
call `get_read_item` with the same captureId, section (`instruments`, `members`,
`institutions`, `groups`, `accounts` or `holdings`) and row id. The response's
summary is the complete frozen item, including the instrument metadataVersion
and managed flag. Get editable fields there before a complete replacement; do
not treat omitted list fields as empty. Old captures stay frozen after writes;
refresh before a new write intention or when checking a stale-version error.

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
{"cursor":"${cursor}","limit":25}
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
Default 25/max 100; pages shrink automatically to fit the complete 65,536-byte
JSON-RPC response, including SDK text and structuredContent. A single oversized
item fails with too_large; reducing limit cannot recover it. Never treat a
rejected response as an empty section. Cursors bind the filters,
sort, full current evidence and session revision. stale_cursor means restart
that read, not guess an offset. Item summaries expose attemptCount; full item
evidence and pageable attempts are separate. Preserve recorded and effective
dates on changes. First sessions have no prior successful-check watermark.

### Unified bounded market history

<!-- example: scale-market-records -->
```json
{"targets":[{"instrumentId":"${instrumentId}"}],"view":"history","fromDate":"2026-09-01","toDate":"2026-09-28","historyLayout":"records","limit":100}
```

This opt-in layout returns `records` and `targets` sections. Read all relevant
cursors; `chartSelected` marks selected chart membership without duplicating the
raw observation. Dates and exact FX direction/basis rules are documented in
[market data](market-data.md). Older `history` calls keep per-target entrances.

## Compact comparison delivery

`compare_market_data_import` defaults to compact batch results. Read its results
cursor to finish classification delivery; detailed sections remain deferred, not
empty. Server-computed counts are not evidence-delivery claims. Use
`get_read_item` section=results id=clientItemId for focused details from the SAME
capture, follow any child cursors, and retain its hash/expiry. Bulk detail readers
can continue result_details/matches/evidence/audits/targets or choose view=full
upfront. Do not create a second capture and mix its evidence into the first.

<!-- example: compare-import-summary -->
```json
{"items":[{"clientItemId":"csv/001","instrumentId":"${instrumentId}","kind":"nav","currency":"USD","date":"2026-09-28","value":"1.00000001"}]}
```

<!-- example: compare-import-item -->
```json
{"captureId":"${captureId}","section":"results","id":"csv/001"}
```


## Live reads and focused evidence

All business page descriptors share {returned, hasMore, nextCursor, consistency,
total?}. Terminal nextCursor is null. All continuations use get_read_page with
only cursor and optional limit (default 25/max 100). Live/version-bound initial
reads return items/page; ordinary frozen initial reads use sections[name] with
items/page. Financial initial and focused-item responses retain their immutable
minimal-disclosure content and positionsPage/gapsPage/evidencePage descriptors;
these descriptors use the same page fields and universal continuation entry.
Follow a cursor even after a short or deferred page. Counts are not proof that
evidence has been delivered. Total is omitted when not authoritative.

consistency=frozen retains capture/hash/TTL. consistency=version_bound retains
publication/filter or reconciliation evidence/revision; restart on staleness.
consistency=live does not promise a snapshot: restart after source changes before
claiming a coherent audit. Rules/suppressions conservatively continue every
nonempty page through a final empty page, now with an opaque signed cursor.
No client constructs last-ID, triple-keyset or native product cursors.
A complete instrument group/event is never split; an oversized single item fails
explicitly with too_large and must not be skipped or interpreted as absence.

Initial financial summaries can answer totals/coverage questions without reading
every evidence section. Read only relevant detail, but finish all requested
classification/list/absence/audit evidence before claiming it complete. Counts
and empty deferred arrays do not mean delivered evidence. Read limits never
truncate a write's execution receipt or get_operation recovery evidence.


## Defaults by response family

Limits are upper row counts; complete response bytes may produce fewer rows.

| Family | Default / maximum | Continuation |
| --- | --- | --- |
| Tool discovery (protocol exception) | 16 definitions | tools/list nextCursor |
| Ordinary initial business reads and every continuation | 25 / 100 | get_read_page({cursor, limit?}) |
| Current/historical overview and analyze_period initial sections | 5 / 100 | get_read_page |
| Financial context/comparison/focused-item initial sections | At most 1 row per section | get_read_page |
| Repair progress | No capture or pages | Poll exact jobId; view=evidence uses frozen pages |

Initial financial headers and focused-item responses retain their established
disclosure/alias and hash structure, without duplicating rows in a second shape.
Their section cursors carry all routing scope, so `get_read_page({cursor, limit?})` is the only continuation entry.

For mcp-read/1, fixed TTL is 5 minutes; per-capture budget is 4 MiB; its cache
is 16 MiB / 8 captures, with 2 builders and a 10-second build budget. Financial
context/comparison uses a separate cache with the same TTL, 4 MiB package and
16 MiB / 8-entry limits. These are retained-data budgets, distinct from the
65,536-byte complete HTTP response limit. Executed write/recovery receipts are
excluded from read response interception.

mcp-read/1 lookup errors are read_expired/read_revoked; financial packages use
context_expired/context_revoked. Restart the original initial request and all
requested related sections after either family's invalidation. Published review cursors use conflict with cursor_expired on changed publication;
reconciliation uses stale_cursor on changed evidence. Restart their original read. Never convert validation, read_busy, too_large,
not_found or an authentication failure into an empty successful result. See
[recovery](recovery.md) for pending/unknown write outcomes, which are a separate
authority from read-cache errors.

<!-- example: scale-agent-closures -->
```json
{"filter":{"instrumentIds":["${instrumentId}"],"observationKinds":["closed"]},"limit":100}
```


Shared read tools advertise success outputSchema for the mcp-read/1 envelope,
including capture identity/hash/expiry and initial-summary versus section-page
forms. Repair advertises progress or evidence shapes. Business summaries/rows
retain their owning exact-money, nullability and provenance contracts; the
common envelope schema does not claim to describe every nested GUI field.
Write and get_operation recovery receipts gain no new post-execution schema
restriction. Error recovery metadata uses the separate mcp-error/1 shape described in
[recovery](recovery.md), not a successful data result.

The SDK sends both JSON text and structuredContent. Both count toward measured
wire bytes. A client can choose which representation to consume; model-token
cost depends on that client. Do not add the two copies as separate evidence or
claim model-token savings from HTTP-byte measurements. The built-in adapter uses
its direct capability path; external assistant consumption remains client-specific.
