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
| Directory, instruments, holdings, accounts, products, liquidity, upcoming actions, Agent audits, overview/history, attribution, market history/import comparisons, health/repair evidence, review preview | captureId + section + signed cursor | Frozen capture, hash, fixed TTL, generation |
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
These immutable disclosure packages have financial schema/hash and alias
contracts. Their descriptors use the same page fields and every cursor uses the
same continuation entry. Ordinary continuations remain compact section/items/page;
focused item continuations use the same section/items/page shape plus ref, without
repeating target/header or unrelated progress. There is no duplicate
row representation. Financial initial sections remain at most one row; overview
and period-analysis initial sections retain five rows to prioritize summaries.
All continuations default to 25. Explicit-scope financial page tools and optional
captureId/section inputs remain aliases, not a separate required workflow.

Repair progress is polling by jobId, not pagination. Exact single-item details
fail explicitly when oversized. Executed mutation receipts and get_operation are
execution/recovery authorities and are never truncated or treated as pages.

## Integrity and budgets

Frozen native cursors are reused, not wrapped or replaced. Their owners still
check hashes, section/ref, generation, expiry and cache presence. There is no TTL
renewal. Live/version-bound routing uses an HMAC-authenticated query and native
cursor, bounded to 16 KiB. Routing is allowlisted to read tools; tool, account,
product, episode, filter and order cannot be substituted. It uses the session's
existing random signing key and generation; no cursor persistence is introduced.
Native publication and reconciliation stale checks remain authoritative.

Budget checks include the projected page and cursor inside both SDK text and
structuredContent, plus transport allowance, before choosing a page boundary.
Readers reduce their own row bound under the existing read fence, never trim
rows after minting a continuation. A single oversized row/group fails too_large;
it cannot be skipped, truncated or repaired by lowering limit. Native frozen
cache budgets remain 4 MiB per capture, 16 MiB/eight entries, five-minute TTL,
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
