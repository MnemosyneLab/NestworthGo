# MCP contract changelog

This file is the stage, extension and follow-up narrative that used to live in
[mcp.md](mcp.md). The current rules are that contract. Figures here are the
measurements recorded when each change landed. They are not a second budget.

The local MCP implementation version is `3.0.0`. Skill versions cited below
are the repository skill at that commit, not the published installer.

## Stage 1 tools

Status at the time: development complete for the user-approved scope on
2026-09-29 (directory maintenance, daily and atomic batch ledger operations,
position imports and transfers, period analysis, contribution drill-down,
current balance, quantity and total-cost reconciliation, pure cost
corrections, and guarded activity undo/fix). Native UI and external-agent
acceptance were left to release records.

The first inventory included separate `archive_member`,
`archive_institution` and `archive_group` tools, separate
`get_historical_overview_series` and `get_market_data_batch`, and one commit
tool per plan kind. Those names are gone. See the later sections. Account and
instrument archive were not folded.

## Agent-supplied market data

Status: implemented (2026-09-29). Agent-supplied data is a local quote source
(`agent`), separate from manual observations and provider adapters. The App
does not call an Agent endpoint.

Selection, as shipped with that extension: current selection resolves daily
observations by effective date (Agent wins the same date) and separately
selects the latest realtime value. Those winners compare actual quote
timestamps, with Agent winning ties. Date-label anchors keep their original
timestamp. A date-label anchor is not an asserted midnight market quote.
Historical Agent close/NAV/reference values win for their exact effective
date. Withdrawing an FX quote appends a recalculation using the fallback
rate. Normal sync skips exact daily dates covered by Agent observations.
Explicit force refresh may fetch the provider again and does not remove Agent
data. Agent-only targets never pull.

Import rebuilds affected closed-day snapshots through the existing service.
Quote commit, local revaluation and rebuild outcome are reported separately.
Validation that day used the full Go suite, application Agent-flow and MCP
import race tests, SQLite race tests, and the frontend suite then current
(72 files, 559 tests), on temporary databases and fake providers.

## Data health repair extension

Status: implemented (2026-09-29). The extension reuses the frontend Data
Health service and its `repair_all` pipeline. It does not change ledger
facts. Validation that day: full `go test ./...`, focused MCP repair and
marketdata adapter race tests, and `go vet` for the MCP and marketdata
adapters. External provider integration, native UI and external-agent
acceptance were not run. No live household data was modified.

## Ordinary liquidity policy and mcp-read/1

Liquidity policy writes were added as GUI-parity previews committed through
the plan tools, using the conservative operation receipt rather than
managed-product atomic business receipts. Scalable management reads then
moved liquidity, products, upcoming actions, instruments, Agent records,
directories, holdings, valuations, overviews and health reads onto
`mcp-read/1` only. `responseMode` and Agent offset were removed.

Historical overview and series responses moved to
`summary.responseVersion=historical-read/2` inside `mcp-read/1`. There is no
legacy `data.rows` mode. Compact directory and instrument lists switched to
explicit identity projections. Financial continuations dropped the removed
`get_financial_context_page` and `get_financial_comparison_page` aliases in
favor of `get_read_page`.

## Bounded discovery (skill 3.0.1)

`tools/list` page size became 16. Repeated permission, receipt and
`operationId` prose moved into the server instructions. Structural schemas
took activity cursor triplets, market target alternatives, paired date
windows and ledger-kind required inputs. Output schemas were added for
`mcp-read/1` success envelopes. Safe tool errors gained
`_meta["nestworth.errorRecovery"]` (`mcp-error/1`, `automaticRetry` false).
Business pagination was unified on `get_read_page`. The MCP implementation
version was set to 3.0.0. The skill version moved independently and is no
longer 3.0.1.

## Discovery size before the Phase 3 schema cut

`discovery_contract_test.go` hashes name, annotations and schemas and ignores
description prose. `discovery_budget_test.go` bounds `tools/list` bytes, the
longest description and the instructions.

Slimming measurement from `TestDiscoveryBudget`, before to after. "After"
includes the exact `delivery` output property (both fields required, string
array) added to 28 read tools, about 6.6 KB per mode. Before that property
the reductions were −7.2%, −7.9% and −8.6%.

| Mode | Tools | `tools/list` | Descriptions | Instructions |
| --- | --- | --- | --- | --- |
| `read_only` | 55 → 53 | 108,144 → 107,005 (−1.1%) | 22,274 → 16,842 | 3,111 → 2,313 |
| `directory_write` | 74 → 72 | 128,513 → 124,976 (−2.8%) | 26,377 → 18,547 | 3,111 → 2,313 |
| `ledger_write` | 104 → 102 | 187,634 → 178,055 (−5.1%) | 43,889 → 30,017 | 3,111 → 2,313 |

Wire bytes, not model tokens. Output schemas then dominated (roughly 45 KB
per mode, mostly the `mcp-read/1` envelope repeated per tool).

### Paired scenario measurement (headless, synthetic data)

Measured with the production Streamable HTTP server and the Go MCP SDK client
over loopback (`payloadRecorder`: complete JSON-RPC response bytes, including
`structuredContent`, text fallback and escaping), on a synthetic ledger
(`MCP ledger`, USD base, one member). Before = `4eaafa6e` (main), after =
the slimming branch, same call script, one run each. The byte counts are
deterministic. There is no timing claim.

| Scenario | Calls before → after | Bytes before → after |
| --- | --- | --- |
| Discovery (`initialize` + `tools/list`, 7 calls, ledger_write) | 7 → 7 | 189,510 → 175,925 (−7.2%; measured before the exact `delivery` schema, which adds about 3.9 KB per mode) |
| 1. Current net worth (`get_financial_context`) | 1 → 1 | 9,038 → 9,036 |
| 2. Record a buy (income + buy previews/commits, holdings read) | 5 → 5 | 21,183 → 21,307 (+0.6%) |
| 3. Balance reconcile (preview + commit) | 2 → 2 | 7,002 → 7,002 |
| 4. Import fund NAV (create instrument, import, read, scan) | 4 → 4 | 6,506 → 6,630 |
| 5a. `get_overview`, every section | 1 → 1 | 17,285 → 17,411 |
| 5b. `analyze_period`, 28 days, every section | 2 → 2 | 17,680 → 17,864 |
| 5d. `analyze_period`, 20 days, every section | 2 → 1 | 14,768 → 13,029 (−11.8%) |
| 6. Data repair (scan, preview, start, poll) | 4 → 4 | 7,666 → 7,916 |

The small per-response increase (about 125 bytes) is the `delivery` object on
top-level initial captures. Not measured: native macOS (`com.nestworth.dev`
isolated QA was not run), real LLM client behavior, and model tokens. A later
external-client baseline is in
[mcp-wave1-a-baseline.md](mcp-wave1-a-baseline.md) and is not this table.

### Phase 3: icon merge, `commit_plan` and compact output schemas

Three changes removed 13 tools. They are breaking for clients that call the
removed names. Design notes:
[mcp-consolidation-design.md](mcp-consolidation-design.md). Update-convention
audit: [mcp-update-conventions.md](mcp-update-conventions.md).

| Change | Before | After |
| --- | --- | --- |
| Icons | `set_member_icon`, `set_institution_icon`, `set_group_icon` | `update_member`, `update_institution`, `update_group` take `{id, name?, iconKey?}`; at least one field; both apply in one statement |
| Commit | ten per-kind `commit_*` tools | one `commit_plan({operationId, input:{planId}})` |
| Output schema | fully typed `mcp-read/1` envelope per tool | compact envelope |

The receipt kept the historical per-kind tool name so hashes, replay and
`get_operation` still match receipts written before the merge. Liquidity
kinds do not replay. At this point `operationId` was still required. Making
it optional is a later section.

Measurement (`TestDiscoveryBudget`; before = the Phase 1/2 row above):

| Mode | Tools | `tools/list` bytes |
| --- | --- | --- |
| `read_only` | 53 → 53 | 107,005 → 65,160 (−39.1%) |
| `directory_write` | 72 → 69 | 124,976 → 82,029 (−34.4%) |
| `ledger_write` | 102 → 90 | 178,055 → 125,943 (−29.3%) |

Wire bytes, not model tokens. Not measured: native macOS, real client token
use, or whether agents make fewer calls with one commit tool. The server no
longer enforces the strict envelope at run time. The strict contract stayed
in `output_schemas_full_test.go`.

## Directory archive folded into update

`archive_member`, `archive_institution` and `archive_group` were removed.
`archived` on the matching `update_*` tool keeps, archives or restores.
`archive_account` and `archive_instrument` stayed. Measured `tools/list`
bytes on the same fixture as `TestDiscoveryBudget`: `read_only` stayed 53
tools / 65,160 bytes; `directory_write` 69 → 66 tools and 82,029 → 80,583
bytes; `ledger_write` 90 → 87 tools and 126,422 → 124,976 bytes. Budgets were
not raised. The skill text was updated and the version bump was left for the
coordinator.

## Historical overview and market data merged

`get_historical_overview_series` and `get_market_data_batch` were removed.
Each surviving tool takes one of two inputs via `oneOf`. Measured
`tools/list` bytes after this merge, on top of the directory-archive fold:
`read_only` 51 tools / 63,796 bytes; `directory_write` 64 tools / 79,219
bytes; `ledger_write` 85 tools / 123,612 bytes. Against the fold's measured
sizes that is −2 tools and −1,364 bytes in every mode. Budgets were not
changed. `skills/nestworth/VERSION` stayed 3.0.4 at that commit.

## E2 large input schemas

Measured with `TestDiscoveryBudget` after the directory-archive fold and the
historical/market merge. Tool counts did not change. `read_only` stayed 51
tools / 63,796 bytes and `directory_write` stayed 64 tools / 79,219 bytes.

| Schema | Before | After |
| --- | --- | --- |
| `preview_product_operation` input | 7,609 | 5,396 |
| `preview_change` input | 3,221 | 2,516 |
| `ledger_write` `tools/list` | 123,612 | 120,694 |

`preview_product_operation` keeps one `productTerms`, `productPolicy` and
`productSettle` definition. Its kind `oneOf` names the required payload.
`preview_change` shares one string schema plus a name allowlist. A per-field
`$ref` was longer than `{"type":"string"}`, so it was not used. `added`,
`mutationId`, `newValue` and `newValueCurrency` were omitted because the
handler rejects them for every supported kind. Each kind's required names
were unchanged. The skill version was not bumped for this schema edit.

## Nullable input schemas

Input schemas stopped publishing `"type": ["null", ...]`. Optional pointers
and slices decode JSON null the same as omission, so those fields are a
single type and callers omit them. Required pointers, `shortName`, and
`update_instruments_batch` `note` keep null through `anyOf`. Measured
`tools/list` bytes after that rewrite, on top of the schema slimming above:
`read_only` 51 tools / 63,022 bytes (−774); `directory_write` 64 tools /
78,083 bytes (−1,136); `ledger_write` 85 tools / 120,426 bytes (−268).
`preview_change` 2,516 → 2,498 and `preview_product_operation` 5,396 → 5,855
(`anyOf` on the shared required pointers). Budgets were not raised.

## Optional `commit_plan` operationId

Walt's decision: only `commit_plan` relaxes `operationId`. A missing or blank
value is UUID v5 of the canonical lowercase `planId` in namespace
`f1f65af0-bbb8-5d57-af9c-b624de715dcb` (URL-namespace v5 of
`nestworth:mcp:commit_plan:operation`). Explicit ids are unchanged. A derived
id that already belongs to another tool or input returns `conflict` and does
not post. Liquidity pending/unknown receipts are still not replayed. Other
write tools still require `operationId`. The discovery note recorded with
the following description edit used ledger_write 120,224 bytes and
instructions 2,373 bytes after this change. `read_only` and `directory_write`
were unchanged by it. The skill later moved from 3.0.4 to 3.0.5 in the same
branch (F1), which covers this contract text. This changelog does not bump
the skill again.

## Wave 2 account-entry descriptions and toolGroups removal

`get_account_snapshot` and `get_account_valuations` stayed two tools. Their
descriptions now say they share one current-valuation capture, and that a
missing snapshot id is `not_found` while an unmatched valuations filter is an
empty page. That prose was the only `tools/list` size change. The optional
`_meta.nestworth.toolGroups` middleware was removed and was not on the default
list path. `TestDiscoveryBudget` against the B2 sizes:

| Mode | Tools | `tools/list` bytes |
| --- | --- | --- |
| `read_only` | 51 | 63,022 → 63,347 (+325) |
| `directory_write` | 64 | 78,083 → 78,408 (+325) |
| `ledger_write` | 85 | 120,224 → 120,549 (+325) |

Instructions stayed 2,373 bytes. Budgets were not raised. Name, annotation
and schema hashes were unchanged. Both `structuredContent` and JSON text
stayed on success results. C2 (folding drill-down into `get_read_item`) was
closed as measure-only. D and E3 were deferred. See
[mcp-wave2-c2-measure.md](mcp-wave2-c2-measure.md),
[mcp-wave2-c3-nav-entries.md](mcp-wave2-c3-nav-entries.md),
[mcp-wave2-g-toolgroups.md](mcp-wave2-g-toolgroups.md) and
[mcp-wave2-deferred.md](mcp-wave2-deferred.md).

## F2 contract rewrite and budget tighten

`docs/development/mcp.md` was rewritten from this chronological stack into the
current contract. The measurement above is still the live `tools/list` size:
51 / 63,347, 64 / 78,408, 85 / 120,549, instructions 2,373, longest
description 1,137 (`commit_plan`). Mode ceilings moved from 66,500 / 83,500 /
128,000 to 64,700 / 80,000 / 123,000, about 2% above those measurements. The
1,500-byte description cap and the 2,400-byte instruction cap were left in
place. `skills/nestworth/VERSION` stayed 3.0.5 because F1 had already applied
the Wave 3 PATCH and this change does not edit skill content.
