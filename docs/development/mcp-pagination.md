# Business pagination

All business continuations use `get_read_page({cursor, limit?})`. The opaque
cursor retains the original tool, filters, order, target, section and consistency
scope. Only the row budget may change. `limit` defaults to 25 and is at most 100;
zero also selects the default. Byte budgets may produce shorter pages. For bulk
classification/list audits, explicitly request limit=100 on both the initial read
and continuations to reduce call/header overhead. Default 25 favors focused reads.
Tool protocol discovery (`tools/list`) remains SDK-owned, with 16 definitions per page.

## Inventory and ownership

| Reads | Previous continuation | Consistency retained |
| --- | --- | --- |
| Directory, instruments, holdings, accounts, products, liquidity, upcoming actions, Agent audits, overview/history, attribution, market history/import comparisons, health/repair evidence, review preview | captureId + section + signed cursor | Frozen capture identity, fixed TTL, generation |
| Financial context, comparison/attribution, focused related evidence | contextId/comparisonId + section/ref + signed cursor | Frozen financial package, disclosure/aliases/ref, hash, fixed TTL, generation |
| Activities | effectiveAt + createdAt + ID | Live keyset in exact descending order |
| Product operation history | productId + native cursor | Live product-scoped history and current undo header |
| Review rules/suppressions | required pageSize + last identity until empty | Live identity order; final empty read required |
| Published review alerts, coverage, daily evidence, episode history/events | required pageSize + revision/filter cursor | Version-bound publication and complete instrument groups |
| Reconciliation sessions/accounts/items/changes/links/attempts | limit <= 50 + evidence/revision cursor | Version-bound full evidence/query/revision |

Live and version-bound results now return `items` and a `page` descriptor;
metadata (publication revision/status, global summary, undo availability) stays
outside `page`. Frozen ordinary reads already use `sections[name] = {items,page}`
and section continuations use `section/items/page`. The common page descriptor is
`{returned, hasMore, nextCursor, consistency, total?}`; terminal nextCursor is
always null. Total is included only when the owner has an authoritative count.
Never infer completeness from returned < limit or invent a total for live reads.

## Deliberate initial-envelope exceptions

Financial initial and focused-item initial responses retain `content` (or focused
positions/gaps/evidence) and positionsPage/gapsPage/evidencePage descriptors.
The cursor binds disclosure, alias scope, capture identity and an internal
semantic hash. Financial continuations carry capture identity, capture time,
section, items and page, plus ref for a focused item. Initial calls use `sections`
for row selection. Every descriptor retains its total and continuation even
when rows were deferred. With no explicit limit, selected financial sections
receive up to 25 rows each within the 24 KiB inline target. An explicit limit
uses the 64 KiB wire budget. Historical and period analysis grow their initial
sections from a five-row target to up to 25 per section when they fit. Current
overview uses that policy only for the sections the caller requests. Every top-level initial `mcp-read/2` capture response carries
`delivery:{selectionComplete,pendingSections}` (both required inside the object;
the property is optional on the envelope because continuation pages and some
comparison details do not carry it; ordinary focused items also report delivery
when they inline selected child rows): whether every row of the selected
sections is already inline. It is transport completeness only and is independent
of `complete`, `missingInputs` and `displayValuation` in the summary.
All continuations default to 25. `get_read_page({cursor,limit?})` is the only
continuation tool: the explicit-scope financial page tools were removed, and the
optional captureId/section inputs remain aliases, not a separate required workflow.

Repair progress is polling by jobId, not pagination. Exact single-item details
fail explicitly when oversized. Executed mutation receipts and get_operation are
execution/recovery authorities and are never truncated or treated as pages.

## Integrity and budgets

Ordinary frozen reads use compact r2 binary/HMAC cursors binding capture UUID,
section, generation and offset. Immutable cache entries provide content identity;
hashes, expiry and query echoes are not sent in mcp-read/2 responses. The owner
checks signatures, section/ref, generation, expiry and cache presence. Financial
disclosure cursors retain their internal hash binding and fixed expiry; neither
field is repeated in the response. There is no TTL
renewal. Live/version-bound routing uses an HMAC-authenticated query and native
cursor, bounded to 16 KiB. Routing is allowlisted to read tools; tool, account,
product, episode, filter and order cannot be substituted. It uses the session's
existing random signing key and generation; no cursor persistence is introduced.
Native publication and reconciliation stale checks remain authoritative.

Budget checks include the projected page and cursor in `structuredContent`,
plus transport allowance, before choosing a page boundary.
Readers reduce their own row bound under the existing read fence, never trim
rows after minting a continuation. A single oversized row/group fails too_large;
it cannot be skipped, truncated or repaired by lowering limit. Native frozen
cache budgets are 4 MiB parent JSON, 8 MiB combined retained period capture
(including frozen details and metadata allowance), 16 MiB/eight entries, five-minute TTL,
two builders and a ten-second build timeout. Financial packages retain their
separate cache and disclosure semantics.

An unread or zero-row deferred section is not absence. Finish every section and
target relevant to the claim; summaries/counts can be authoritative without
claiming detailed evidence was delivered. Records-layout market history and
comparison child sections retain global order, targetKey and exact provenance.
Never merge unrelated targets by label or combine evidence from new captures.

On read_expired/read_revoked or context_expired/context_revoked, discard all
related pages and restart the initial query. On review cursor_expired/conflict or
reconciliation stale_cursor, restart that version-bound read. Live traversal is
not a consistent snapshot: restart after source changes when making an audit
claim. Validation/authentication/too_large failures never mean an empty result.

Both HTTP MCP and the built-in AI use this projection and routing in the same
capability registry. No provider calls, database writes or financial arithmetic
are added by pagination. HTTP-byte savings are not measured model-token savings;
external clients determine which SDK representation reaches the model.

## Period drill-down retention

`analyze_period` freezes return days and eleven asset-driver details on the same
read fence as its summary. `get_read_item` selects `returnDays` by `YYYY-MM-DD`
or `assetDrivers` by bucket key; child sections use `get_read_page`.
Contribution detail uses the already frozen parent row. None of these reads
refreshes snapshots or extends expiry.

Period details are privately owned per-item JSON, outside the marshaled parent.
They are not uncharged DTO pointers: detail bytes, keys, 128 bytes per entry and
256 bytes of base allowance count toward the combined 8 MiB retained budget
and the 16 MiB shared cache. At most 32,768 day/driver entries are retained.
Admission fails before publication when limits are exceeded; expiry, eviction
and revocation release the same charge. This is not an RSS/working-set ceiling.

The opt-in `TestMeasureC2ParentCaptureExtremes` compares production parent bytes
with a naive all-details-in-parent representation:

```sh
NESTWORTH_C2_MEASURE=1 go test ./internal/mcpserver \
  -run '^TestMeasureC2ParentCaptureExtremes$' -count=1 -timeout 30m -v
```

Run it only for relevant retention/performance work, recording source, host and
fixture. `period_drilldown_test.go` is the ordinary retention/ownership/budget
regression suite. Old pre-budget measurements do not establish current memory
usage, model-token savings or native-client acceptance.
