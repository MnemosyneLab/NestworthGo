# MCP batch-history window and unified-record evidence

Environment: isolated cloud Linux workspace, Go 1.27.3, synthetic temporary
SQLite databases only. No household ledger/CSV was read or uploaded. Initial
source base was main `adf7ea47fa94cbeb40b45b52d14b57cba18b4b57`; main's subsequent
navigation-only change was incorporated before delivery. This is query/call-count
evidence, not a latency, allocation, RSS or p95 claim.

`TestMarketHistoryRecords35Targets233Quotes` creates 35 instruments and exactly
233 manual observations (23 targets with seven dates; 12 with six). Values are
exact eight-place decimals. Both paths use the same explicit inclusive seven-day
window and `limit=100`. The count includes the capture and all relevant target,
points/observations or records continuation SDK `tools/call` round trips.

| Measurement | Result |
| --- | --- |
| Legacy per-target traversal | 73 calls |
| Unified records plus all target metadata | 6 calls |
| Unified call breakdown | 1 capture, 4 records continuations, 1 target continuation |
| Unified first-response records | 47 of 233 |
| Separate direct HTTP first-response probe | 63,508 actual bytes |
| First-response reserved wire size | 64,498 bytes |
| Maximum records-page reserved wire size | 64,694 bytes |
| Frozen unified capture | 163,059 bytes, two sections, 233 unique observations |
| Retained limits | 65,536 wire bytes, 4,194,304 capture bytes, 100 rows per section/page |

Reserved wire size includes duplicated SDK text/structured content and a
1,024-byte framing allowance. The independent direct HTTP probe is not included
in the six-call traversal measurement. No default-limit or all-payload one-page
claim is made. Longer source keys, identities, conversion evidence and FX
metadata alter page sizes. Five-minute capture expiry still requires restarting.

New regression coverage includes invalid/partial/empty/reversed dates and range
exclusion; spring/fall DST (23/25-hour days), leap day, market effective date
versus timestamp, absent-origin UTC fallback and invalid-zone errors; unknown
target versus empty range; NAV basis preservation; original/reversed FX evidence
through SDK; exact decimal tails; selected versus unselected record flags;
correction/retraction and frozen old captures; signed cursor tampering/expiry;
single-item wire and 4 MiB failure; no duplicated history storage; and no provider
calls. The existing snapshot test also exercises the new date path under SQLite
`query_only`, verifies unchanged data/change counts, and retains concurrent-pair
snapshot/budget coverage.

Follow-up reverse-FX regression first reproduced a false empty history for a
stored USD/SGD rate of `0.000000000001` on September 27 while a newer rate of `2`
remained selected current. Its SGD/USD reciprocal exceeds FxRate's range, so the
legacy chart projection silently omits it. The unified capture now checks every
original quote ID against the projection and returns explicit `unavailable`
instead of `no_quotes_in_range` or a partial capture. Tests cover entirely and
partially unrepresentable windows, unchanged legacy output, and exact retrieval
in the original direction.

The immutable pre-extension external-discovery hashes remain assertions. A narrow
projection removes only the three newly optional batch-history properties and
restores that tool's previous description before comparing the original hashes;
all other discovery fields/tools and local/external metadata equality remain
checked. Legacy history shape and per-target cursors are retained.

The affected domain, MCP and SQLite regressions pass locally. Application and
SQLite complete suites also passed during development. Local whole-repository
test/vet and desktop build attempts cannot pass their native desktop compilation
gate because this Linux image lacks GTK4, WebKitGTK6, libsoup and gio pkg-config
development files. The repository's full native macOS CI is the authoritative
regular gate for the delivered commit; consult the PR's exact-head run rather
than interpreting these Linux results as native Mac acceptance. No GUI,
packaging, signing or notarization acceptance was performed.

The read-only import-comparison interface is **design only** in
[the proposed contract](../design/mcp-import-comparison.md). Same/conflict/missing,
ordered duplicate inputs, withdrawal evidence and selection-impact classification
are not implemented or claimed as tested API behavior. Their required acceptance
matrix is specified there. Existing active-history records must not be used as a
substitute for a complete comparison or a later-write CAS precondition.
