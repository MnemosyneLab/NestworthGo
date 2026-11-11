# Page a business read and recover it

Liquidity, products, instruments, upcoming actions, Agent records, account
valuations, directory lists, holdings, `scan_data_health` and
`preview_data_repair` return mcp-read/2 by default. `get_data_repair_job`
defaults to compact progress without a capture. Only view=evidence uses
mcp-read/2. There is one contract. Do not pass responseMode. Historical and
current overview and period reads inline small results: at least five rows per
section when they fit, at most 25, within a byte budget. Continuations default
to 25. Every top-level initial mcp-read/2 capture carries
`delivery.selectionComplete` and `delivery.pendingSections`. Continuation pages
do not. Focused ordinary item reads also report delivery when they inline child sections. When
`selectionComplete` is true, every selected row of that capture is already
inline and no cursor is needed. For `compare_market_data_import` view=summary
the selection is the `results` section only. Deferred detail sections are not
reported as pending, so it never claims evidence delivery. Data completeness
(`complete`, `missingInputs`, `displayValuation`) is separate: a fully delivered
selection can still describe incomplete data. Pure minimal financial-context
workflows retain their disclosure path in [financial context](financial-context.md).

Task arguments live with the task: [product pages](product-pages.md),
[directory batches](directory-batches.md), [market read pages](market-read-pages.md).

`get_read_item` accepts `limit` for every capture kind, including market import
comparison and period analysis: default 25, maximum 100 initial rows per selected
child section, subject to the response byte budget. Optional `sections` selects
short child names; `availableSections` lists unselected names. Unknown names fail
validation. Follow each selected child's remaining cursor with `get_read_page`.
`delivery.selectionComplete` describes selected children only, not omitted evidence.

## Completeness and recovery

An initial mcp-read/2 capture has captureId, captureKind, capturedAt, summary
and sections. Hashes, expiry and query echo are server-owned metadata and are
not returned. The agent need not calculate or compare hashes. Each section
has items and page {total, returned, hasMore, nextCursor}. Empty arrays mean no
matches only when hasMore is false and nextCursor is null. A deferred section
may have returned=0 and a cursor. Follow every relevant cursor using
`get_read_page` with only cursor and optional limit. The signed cursor retains
capture, section, target and query. Filters and sort cannot change mid-read.
`get_read_page` returns the same capture identity and capturedAt plus section,
items and page. It does not repeat query, summary or unrelated sections. Retain
the initial header, append only the returned section and update only that
section's saved page state. Default limit is 25 for all business continuations.
Maximum is 100. Actual pages may be smaller to fit 65,536 final HTTP bytes,
including the structured result and JSON-RPC envelope. For a complete bulk
import or list audit, use limit=100 upfront and on continuations when that
evidence is needed. Default 25 favors focused reads. Never claim “all” from one
page. Preserve captureId and check complete unions.

<!-- example: scale-page -->
```json
{"cursor":"${cursor}","limit":25}
```

Captures have a fixed five-minute TTL without renewal, a 4 MiB parent JSON budget,
16 MiB total cache, eight entries, two builders and a ten-second build budget.
SQL reads also preflight row and payload budgets. Expired or evicted captures
return read_expired. A revoked generation returns read_revoked. Reconnection,
permission changes, restore or restart invalidate old captures. Lost caches may
report read_expired, and revoked credentials may fail authentication. Discard
every old page and restart the original query. Invalid, tampered or
wrong-section, target or capture cursors return validation. Do not stitch
captures. too_large distinguishes summary, single_item and capture. An oversized
single item is never skipped or truncated. Lowering limit cannot fix that item.
Send one JSON-RPC object per business call. JSON-RPC array batches are rejected.

Arrays within one filter are OR. Different fields are AND. Empty arrays are
unrestricted. Unknowns remain null and do not match either hasRule=true or
hasRule=false. Sorting is global before pages, null-last in both directions,
with fixed identity ascending ties. Names compare trimmed case-insensitive text
without locale. Amounts and percentages use exact decimals. IDs determine write
identity, not labels.

## Reconciliation evidence pages

Discover `reconciliation_session_read`. `list_reconciliation_sessions` filters
status, account and date and sorts ascending or descending.
`get_reconciliation_session_page` pages accounts, items, changes, links and
attempts, with account and outcome filters and sort. Default 25, maximum 100.
Pages shrink automatically to fit the complete 65,536-byte JSON-RPC response,
including the structured result and envelope. A single oversized item fails with
too_large. Reducing limit cannot recover it. Never treat a rejected response as
an empty section. Cursors bind the filters, sort, full current evidence and
session revision. stale_cursor means restart that read, not guess an offset.
Item summaries expose attemptCount. Full item evidence and pageable attempts
are separate. Preserve recorded and effective dates on changes. First sessions
have no prior successful-check watermark. The session steps are in
[reconciliation](reconciliation-and-corrections.md).

## Live reads and focused evidence

All business page descriptors share {returned, hasMore, nextCursor, consistency,
total?}. Terminal nextCursor is null. All continuations use `get_read_page` with
only cursor and optional limit (default 25, maximum 100). Live and version-bound
initial reads return items/page. Ordinary frozen initial reads use sections[name]
with items/page. Financial initial and focused-item responses retain their
immutable minimal-disclosure content and positionsPage, gapsPage and evidencePage
descriptors. These descriptors use the same page fields and the universal
continuation entry. Follow a cursor even after a short or deferred page. Counts
are not proof that evidence has been delivered. Total is omitted when it is not
authoritative.

consistency=frozen retains immutable capture identity and server-enforced TTL. consistency=version_bound
retains publication, filter or reconciliation evidence and revision. Restart on
staleness. consistency=live does not promise a snapshot. Restart after source
changes before claiming a coherent audit. Rules and suppressions conservatively
continue every nonempty page through a final empty page, with an opaque signed
cursor. No client constructs last-ID, triple-keyset or native product cursors.
A complete instrument group or event is never split. An oversized single item
fails explicitly with too_large and must not be skipped or interpreted as
absence. Initial financial summaries can answer totals and coverage questions
without reading every evidence section. Read only relevant detail, but finish
all requested classification, list, absence and audit evidence before claiming
it complete. Counts and empty deferred arrays do not mean delivered evidence.
Read limits never truncate a write's execution receipt or `get_operation`
recovery evidence.

## Defaults by response family

Limits are upper row counts. Complete response bytes may produce fewer rows.

| Family | Default / maximum | Continuation |
| --- | --- | --- |
| Tool discovery (protocol exception) | 16 definitions | tools/list nextCursor |
| Ordinary initial business reads and every continuation | 25 / 100 | get_read_page({cursor, limit?}) |
| Current/historical overview and analyze_period initial sections | 5 floor, 25 within a byte budget / 100 | get_read_page |
| Financial context/comparison/focused-item initial sections | At most 1 row per section | get_read_page |
| Repair progress | No capture or pages | Poll exact jobId; view=evidence uses frozen pages |

Initial financial headers and focused-item responses retain their established
disclosure, alias and hash structure, without duplicating rows in a second
shape. Their section cursors carry all routing scope, so
`get_read_page({cursor, limit?})` is the only continuation entry.

For mcp-read/2, fixed TTL is 5 minutes; parent JSON budget is 4 MiB; its cache
is 16 MiB / 8 captures, with 2 builders and a 10-second build budget. Financial
context and comparison use a separate cache with the same TTL, 4 MiB package and
16 MiB / 8-entry limits. These are retained-data budgets, distinct from the
65,536-byte complete HTTP response limit. Executed write and recovery receipts
are excluded from read response interception.

mcp-read/2 lookup errors are read_expired/read_revoked. Financial packages use
context_expired/context_revoked. Restart the original initial request and all
requested related sections after either family's invalidation. Published review
cursors use conflict with cursor_expired on a changed publication. Reconciliation
uses stale_cursor on changed evidence. Restart their original read. Never convert
validation, read_busy, too_large, not_found or an authentication failure into an
empty successful result. See [recovery](recovery.md) for pending or unknown write
outcomes, which are a separate authority from read-cache errors.

Shared read tools advertise the mcp-read/2 envelope, including capture identity,
capture time and initial-summary versus section-page forms. Repair advertises
progress or evidence. Nested rows keep their own exact-money and provenance
contracts. The envelope does not describe every GUI field, and write or
`get_operation` receipts gain no extra post-execution schema. Error metadata uses
the mcp-error/1 shape in [recovery](recovery.md), not a successful data result.
MCP implementation 5.0.0 returns successful data in `structuredContent` with
an empty `content` array; errors retain text. Earlier implementations also
returned a JSON text copy. Text-only clients cannot use successful results in
5.0.0. The complete response budget includes the structured result and envelope;
do not treat HTTP bytes as model tokens.

Period drill-down JSON is stored separately but charged to the same 16 MiB
cache. Parent JSON plus frozen detail bytes, keys and metadata allowance must
fit 8 MiB per capture; at most 32,768 day/driver entries are retained. Exceeding
either returns `too_large` before admission; narrow the query. These limits do
not bound process RSS or temporary analysis allocations. Paging does not renew
TTL, and eviction requires restarting all sections.
