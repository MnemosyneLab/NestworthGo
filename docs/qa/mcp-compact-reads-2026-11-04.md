# Compact MCP management reads and financial continuations

Original paired-measurement base: main `34f048f797c4667689a729cd2324bdbd9eb005f8`.
Current integration base: main `002b4f0` (frozen contributions, import comparison
and complete reconciliation wire budgets included).
Environment: Linux amd64, Go 1.27.3, MCP Go SDK 1.7.0, server build tag,
isolated temporary synthetic SQLite households. No real ledger or live provider.
The PR records the exact tested head. These changes are default-enabled in the
branch, not merged or deployed.

## Contract and scope

Directory lists use explicit identity projections. Accounts retain ownership,
classification, currency, lifecycle and inclusion but omit latestValue; full
same-capture detail retains it. Members/groups retain name/ID/archive status;
institutions retain type/country too. Holdings retain exact quantity and IDs.
Instruments retain full/short name, type, symbol/market/country/ISIN,
currency/unit, quote routing, managed/archive guards and metadataVersion.
Notes, display metadata, household IDs and internal timestamps use get_read_item.
Instrument details are now supported with section=instruments and the row ID;
they include the original version and full metadata. Sorting/search precede
projection. Complete replacement writes require full detail first.

Financial context/comparison pages return only capture identity/hash/time/expiry,
section, items and page. Clients retain the initial header, including currency,
disclosure, coverage, comparison attribution and exact precision adjustment.
Only requested progress is returned. Initial captures and the related-item tool
keep their original shape. Canonical content hashes, financial calculations,
permissions, fixed TTL and generation revocation remain authoritative.

This breaking continuation contract ships with distributable skill 4.0.0 and
updated discovery descriptions. No legacy runtime response path was added.
No import-comparison, contribution snapshot or reconciliation-budget code is
changed. No application/domain/GUI code is changed.

## Actual SDK wire measurements

One paired sample per tier, using the identical measureCompactReadJourneys probe
in this branch and an isolated baseline checkout. payloadRecorder observes the
real SDK HTTP response, including JSON-RPC envelope and duplicated text plus
structuredContent. Counts exclude discovery and initialization. Opaque IDs and
timestamps cause small byte differences. There are no token or latency claims.

Ordinary fixture: 20 accounts, 24 holdings, eight shared instruments, four
currencies, 42 retained days. Larger: 200 accounts, 100 holdings, 25 shared
instruments, four currencies and sparse two-year history. Initial management
limit is five, continuation 25; financial continuation default is 50. Every
relevant cursor is consumed. Comparison is a closed date against current state.

| Journey | Ordinary baseline calls / bytes | Ordinary new calls / bytes | Larger baseline calls / bytes | Larger new calls / bytes |
| --- | ---: | ---: | ---: | ---: |
| All account identity pages | 2 / 35,288 | 2 / 21,632 | 9 / 358,116 | 9 / 200,082 |
| All account pages + one full detail | 3 / 38,576 | 3 / 24,920 | 10 / 361,405 | 10 / 203,369 |
| All instrument identity pages | 2 / 14,880 | 2 / 7,888 | 2 / 39,508 | 2 / 17,658 |
| All instrument pages + one full detail | unavailable | 3 / 10,386 | unavailable | 3 / 20,157 |
| Financial context, all sections | 6 / 158,748 | 6 / 125,663 | 26 / 1,220,584 | 24 / 921,120 |
| Comparison, all sections | 9 / 390,291 | 8 / 274,094 | 62 / 3,578,559 | 47 / 2,041,663 |

The larger journeys reduce account identity bytes by 44.1%, instrument identity
bytes by 55.3%, context bytes by 24.5% and comparison bytes by 42.9%.
Discovery remains 103 tools; descriptions grow the real tools/list response
from 148,293 to 148,894 bytes (+601). This is separate from business results.
Both tiers also pass the existing 60-tool payload probes. Maximum observed new
business response: 64,165 / 64,632 bytes, below 65,536. Invalid limit and absent
capture errors are observed through the same recorder. Complete evidence still
requires multiple calls; it is not claimed to fit in a single response.

## Validation and limits

Focused regressions cover duplicate-name/symbol instruments distinguished by
currency, three permission modes, complete same-capture metadata and versions,
source mutation between pages, exact frozen page unions, deterministic retries,
empty matches, incorrect section/cursor, near-limit escaped wire rows, and
oversized items. Existing tests retain stale CAS, managed/archive guards,
atomic writes, receipt recovery, cursor isolation, TTL and restore revocation.
Round-robin client tests consume all sections independently. Attribution tests
preserve the cached exact link and precision while checking it is not repeated.
Discovery preserves the immutable baseline: the five intentional description
deltas are checked exactly before projection back to that baseline; schemas,
permissions and all other tools still match.

Final full MCP package run passed in 90.628s; focused tests and MCP vet passed.
Both paired payload tiers passed. The skill reference/version checker passed
with 71 executable JSON examples, and skill packaging/checksum passed.

Commands: go test -tags server ./internal/mcpserver; go vet -tags server
./internal/mcpserver; go test -tags 'server perf' ./internal/mcpserver -run
'^TestMCPPayloadBudgets(Representative|Large)$' -v; skill reference/version check;
Python tooling unittest discovery; skill packaging and SHA-256 verification.

The first full MCP run found only the intentional discovery-description delta;
the strict baseline projection was then added and the focused test passed.
The first Python run passed 134/136 cases; the Go embed inventory needed the
installed Go toolchain on PATH, and an unrelated cancellation fixture read an
empty PID file. Both exact cases passed together on retry with the correct PATH.
No assertion was removed or production workaround added for these harness issues.

Native desktop/macOS, frontend, real external assistants, dense multi-year data,
more than 25 distinct instruments and model token use were not measured.
Large full metadata remains subject to capture and single-item budgets; it fails
explicitly rather than truncating. Pages require retaining the original header;
old clients must update their readers. CI and independent review remain pending. No merge or deployment was performed.


## Integration with main 002b4f0

The shared get_read_item route now keeps main's market-import-comparison detail
handler and dedicated contribution-kind guard. Instruments join the generic
frozen detail branch without changing either route. Main's contribution row
capture/detail mapping, import comparison compact defaults and reconciliation
HTTP budget are retained. Discovery still checks the immutable baseline, now
including the upstream comparison-tool addition and contribution metadata
projection alongside the exact compact-description deltas.

The distributable skill is 4.0.0, above main's 3.0.1; this separately breaking
financial continuation contract also retains contribution captureId/groupKey,
read-only import comparison and the 64 KiB reconciliation guidance. Published
assets remain unchanged.

Fresh actual SDK wire samples on the merged implementation:

| Complete journey | Ordinary calls / bytes | Larger calls / bytes |
| --- | ---: | ---: |
| All account identity pages | 2 / 21,628 | 9 / 200,082 |
| All account pages + one detail | 3 / 24,914 | 10 / 203,369 |
| All instrument identity pages | 2 / 7,884 | 2 / 17,658 |
| All instrument pages + one detail | 3 / 10,380 | 3 / 20,157 |
| Financial context, all sections | 6 / 125,651 | 24 / 921,122 |
| Comparison, all sections | 8 / 274,110 | 47 / 2,041,665 |

Discovery now has 104 tools and measures 150,734 actual HTTP JSON-RPC bytes.
Both ordinary and larger payload tiers passed; maximum business response remains
64,165 / 64,632 bytes. The earlier before/after tables describe the original
paired experiment, rather than a new baseline containing main's added tool.


Post-integration full server-tagged MCP package passed in 159.983s, including
all skill workflows, frozen contribution, import-comparison and reconciliation
wire regressions; MCP vet passed. The ordinary/perf payload pair passed in
19.252s. The reference/version check passed 73 executable JSON examples and
verified the 3.0.1 -> 4.0.0 bump. These wall times are test-run observations
under concurrent local checks, not runtime performance comparisons.

Full Python tooling discovery passed all 136 tests in 218.733s on the integrated
source, with the installed Go toolchain on PATH. Skill 4.0.0 packaging and its
adjacent SHA-256 checksum passed. No retries or skipped assertions were needed
for this integration validation. Native/macOS/frontend checks remain unclaimed.
