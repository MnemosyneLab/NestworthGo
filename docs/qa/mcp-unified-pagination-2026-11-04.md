# Unified MCP pagination verification — 2026-11-04

Version clarification (2026-11-05): the current repository skill is 3.0.1,
unpublished. Version labels and measurements below retain the original tested
commits; they do not describe a published release or the normalized tree.
See the [current skill guide](../user/nestworth-skill.md#skill-version-policy)
for the explicit 6.0.1 → 3.0.1 normalization and default PATCH increment rule.

Runtime/test source: `1e93bb2` (MCP implementation `08d85b8`, built-in adapter
follow-up `1e93bb2`), based on main `9666aab`. Subsequent documentation-only delivery
changes do not change the verified runtime. All data was synthetic and isolated.

## Coverage

The [inventory and contract](../development/mcp-pagination.md) covers ordinary
frozen management/financial/history/market/health captures; financial context,
comparison/attribution and focused-item evidence; live activity/product/rule/
suppression reads; publication-bound review groups/daily evidence/history/events;
and revision/content-bound reconciliation sessions and evidence. Initial reads
use `limit`; every business continuation uses `get_read_page({cursor,limit?})`.

Coverage includes complete identity unions and order across targets, signed scope
and tamper rejection, wrong capture/section, session/generation isolation,
revocation/expiry recovery, publication/reconciliation staleness, final-empty
live identity pages, short byte-limited pages, deferred rows, and oversized single
items/groups. Focused-item pages now retain only ref/hash/TTL and the requested
section. A large initial target may defer a child that fits the compact page;
a truly oversized next row fails for both limit=1 and limit=100 without skipping
it. Exact financial values, native/base distinctions, provenance, disclosure,
closed-period meaning and mutation/recovery receipt guards remain asserted.

The same registry serves external MCP and in-process capabilities. Built-in AI's
own wrapper also uses cursor-only continuations and retains its read allowlist,
output validation, evidence tracing and navigation guards.

## Completed checks

- Full `internal/mcpserver`, `internal/agentcapability`, `internal/builtinai` and
  `internal/assistant` suites passed on Linux with the workspace Go 1.27.3 tools.
- 136 Python tooling/installer/version tests passed. The 13 skill installer tests
  were repeated after final source-reference changes and passed.
- Skill guard: 77 parseable argument examples; registered-schema and executable
  skill workflow tests passed with the MCP suite.
- Skill 6.0.0 standalone packaging and adjacent SHA-256 verification passed;
  archive payloads and embedded source use the same versioned skill directory.
- Formatting and whitespace checks passed.
- Runtime head `1e93bb21ea4df7d94ac83d115847a8b00cb61ea4` passed native macOS
  [check](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41898780351)
  and [Eino provider acceptance](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41898780326).
  The check job took 3m14s, after roughly nine seconds before job start; no CI
  timeout or assertion was weakened.

The host-wide Go attempt encountered missing GTK/WebKit development libraries;
that environment failure is separate from the passing backend suites. Full-repository
vet and desktop Go build passed in the existing Debian Trixie native-dependency
container. The local full Go run passed all other packages but reported one
existing wall-clock gate: TestAnalysisPerformanceBudgets/cold-2y took 5.062s
against 5s while other builds/suites ran. Its isolated rerun passed all three
subtests, including cold-2y in 3.168s and the unchanged exact financial assertions.
No application analysis code, threshold or assertion changed. Host contention is
an inference, not a measured performance attribution; the complete exact-runtime
CI check above is independently green. Native macOS GUI, packaging/signing and real
model-token consumption are not established by these Linux checks.

## Small synthetic HTTP evidence

These are complete actual HTTP JSON-RPC bodies, including SDK JSON text and
structuredContent; they are not tokenizer or real-provider measurements. Final
MCP source `08d85b8` was used for this run.

| 35 targets / 233 import classifications | Calls | Total HTTP bytes | Largest response |
| --- | ---: | ---: | ---: |
| Summary, default 25 | 10 | 51,990 | 10,690 |
| Summary, explicit 100 | 3 | 39,392 | 19,846 |
| Full evidence, default 25 | 11 | 315,544 | 64,644 |
| Full evidence, explicit 100 | 5 | 304,644 | 64,562 |

Both budgets returned all 233 classifications in order; deferred evidence stayed
explicitly unread. Explicit 100 reduced call/header overhead for this bulk case;
there is no claim that choosing the default 25 improves every bulk journey.

The representative 20-account/eight-instrument workflow covered 60 tools and
126 calls, with maximum response 63,634 bytes. Tool discovery exposed 104 ledger
capabilities across protocol pages, using 189,451 actual HTTP bytes in total;
shared frozen-page schema references avoid repeating descriptor definitions.
Every read remained within the 65,536-byte response cap. No latency, allocation,
RSS, p95 or global performance claim follows from this small fixture.

## Deliberate exceptions

Financial initial/focused-item initial headers preserve immutable content,
disclosure and aliases, with at most one initial row per section. Overview and
period-analysis initial sections retain five rows to prioritize summaries. All
continuations use the same default 25/max 100 and section/items/page shape;
focused items additionally retain ref. Explicit-scope financial tools remain
aliases. SDK tools/list uses the protocol's own cursor and 16-definition budget.
Repair progress polls exact jobId; executed write/get_operation receipts are
execution authorities and never become truncated read pages.

## Review follow-up — skill 6.0.1

Corrected the remaining market-data reference and compare_market_data_import
advertised default from 100 to 25 (maximum remains 100). Both market comparison
and financial attribution descriptions now direct continuation to get_read_page
with only cursor and optional limit. The independent discovery baseline retains
its captured before metadata; only these two intentional after hashes changed.
The shared defaults test now checks market reference text and both registered
descriptions alongside the existing actual continuation/default and byte-budget
assertions. Financial calculations, cursor codecs and consistency rules did not
change. Skill packaging/version guidance advances to 6.0.1; the 6.0.0 evidence
above describes the earlier pinned run.
Targeted skill/schema/discovery and 35-target/233-classification regression tests
passed, together with all 136 tooling tests, the skill guard, packaging/checksum,
formatting and whitespace checks. No financial suite or timing gate was changed.
