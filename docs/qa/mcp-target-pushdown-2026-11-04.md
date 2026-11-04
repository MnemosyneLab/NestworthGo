# MCP target pushdown evidence — 2026-11-04

Baseline: latest fetched `main`, `34f048f` (2026-11-04).
Environment: managed Linux amd64, Go 1.27.3, isolated synthetic temporary SQLite
files, no real household data and no live provider requests. Initial checks use
`GOFLAGS='-mod=readonly -tags=server -p=4'`. These are Linux results, not native
macOS GUI/package acceptance.

## Main reproduction

The portable `TestPushdownRequestEvidence` runs unchanged in a detached baseline
worktree and the implementation worktree. It creates 40 irrelevant accounts and
40 irrelevant instrument observations, then requests one checking account and one
instrument/date audit subset.

| Measurement | main | Scoped implementation |
| --- | ---: | ---: |
| Account objects expanded by repository | 44 | 1 |
| Instrument objects expanded for account request | 41 | 0 |
| Legacy full-audit objects expanded | 41 | 0 (scoped repository path replaces it) |
| `get_account_snapshot` complete MCP result wire | 2,898 B | 2,926 B |
| `list_agent_market_data` complete MCP result wire | 2,786 B | 2,896–2,898 B |
| Tool calls per request | 1 | 1 |

The extra scope metadata slightly increases wire size. Capture IDs and timestamps
can cause a few bytes of variation. Zero legacy audit objects is not a claim of
zero SQL rows: the SQL meter below measures the replacement path.

The perf-tagged portable `TestPushdownRequestLargeWireEvidence` additionally
clones isolated synthetic rows to reach 5,000 irrelevant accounts/observations.
It passes on both main and this implementation:

| Large MCP request | main | Scoped implementation |
| --- | ---: | ---: |
| Account objects expanded | 5,004 | 1 |
| Instruments expanded for account request | 5,001 | 0 |
| Legacy full audit objects expanded | 5,001 | 0 |
| Account snapshot complete result wire | 2,902 B | 2,926 B |
| Target/date Agent audit complete result wire | 2,790 B | 2,898 B |
| Tool calls per request | 1 | 1 |


## Large paired SQL experiment

`go test -tags=perf ./internal/infrastructure/sqlite -run '^TestTargetReadPushdownLargeSynthetic$' -v`

5,000 unrelated accounts, holdings, instruments and Agent quote/audit records;
one target account with one holding and one target date/quote. The old full read
is retained as an oracle, then the scoped authority is compared on identical data.
Exact target valuations, composition and missing evidence match.

| Repository SQL projection | Full legacy read | Scoped read |
| --- | ---: | ---: |
| Agent audit result rows | 10,004 | 3 |
| Agent audit SQL calls | 10,005 | 4 |
| Portfolio input result rows | 25,009 | 14 |
| Portfolio input SQL calls | 35 | 22 |

The SQL meter counts actual result rows using diagnostic COUNT subqueries and
counts production SQL calls, including aggregate budget preflights and quote point
lookups. Diagnostic COUNT/EXPLAIN calls are excluded. Transaction setup and the
application household lookup are excluded equally. These are SQL **result rows**,
not SQLite VM visits, storage-page reads, disk bytes or process RSS. EXPLAIN verifies
activation of the instrument target index and audit quote/target indexes. Account
scoping adds preflights; small fixtures can use more SQL calls despite fewer rows.
The 30-unrelated-row regular test measured audit 64→3 rows / 65→4 calls and account
159→14 rows / 15→22 calls. No latency or p95 claim is made.

## Correctness coverage

- Multi-step historical corrections moving between source dates and a later
  withdrawal; excluded invalidators still suppress earlier observations.
- Retraction identity follows the immediate target, including targets omitted by
  created bounds; active status is independent of selection by valuation.
- Created-at offset equivalence and sub-millisecond inclusive boundaries.
- Closure source date in Pacific/Kiritimati, oriented FX selection and withdrawal.
- Unknown IDs and wrong household return no records; cancelled reads fail.
- SQLite query-only connections work. Oversized unrelated provenance is excluded;
  selecting that provenance fails the shared budget before row expansion.
- Exact full-versus-scoped account valuation and existing complete evidence pages,
  account filters, unknown/archived-account not-found behavior.
- Frozen capture stability across later corrections; credential rotation rejects
  the old session; read-only discovery excludes the import writer.
- Existing Agent correction/retraction date sorting and single-row continuation
  tests retain frozen pagination, deterministic ties and full target identities.

## Remaining coverage limits

No native macOS GUI, signed packaging, frontend visual acceptance, race suite,
live provider test, restored schema21 database, adversarial concurrent restore,
SQLite page-cache/disk measurement, or process-memory measurement is claimed.
The existing single-transaction read architecture is retained; the new targeted
path has not received a new concurrent-writer stress experiment. Broad/date-only
requests and very long histories of one selected target may still scan large SQL
indexes or exceed budgets. Unrelated corrupt out-of-scope facts are not decoded;
this endpoint is not a household integrity scan. Shared FX/directories remain
household inputs for account reads.

No files owned by in-progress #212, #215, #216 or #217 are edited. Existing shared
input schemas and the distributable skill are unchanged; the new scope/count
semantics are documented in [the implementation contract](../development/mcp-target-pushdown.md).
