# Financial audit payload budget correction

## Reproduction and cause

On Linux amd64 with Go 1.27.3, main at
`3b8c6c6ac77473c779e6c8ec397dd44458a9f952`, an isolated synthetic
`TestMCPPayloadBudgetsLarge` reproduces the nightly failure exactly:
`compare_financial_context all sections` uses 67 calls and 2,070,252 final
HTTP JSON-RPC response bytes, exceeding the 64-call budget but not 4 MiB.

The compact-read budget introduced by #217 used financial continuation defaults
of 50 rows. #225 (`94302de0`) intentionally standardized those defaults to 25,
but the full-audit walkers continued omitting `limit`. This was not an increase
in financial rows or loss of byte-aware packing. The documented
[bulk audit workflow](../development/mcp-pagination.md) already recommends
explicit `limit=100` when all evidence is needed.

## Correction and evidence

The complete financial audit walkers now explicitly request 100 rows on
continuations. Initial summary reads, production default 25 / maximum 100,
financial calculations, disclosure, frozen captures and every original call,
byte and 64 KiB per-response ceiling remain unchanged. Byte-aware sizing may
still return fewer than 100 rows.

The compact walker now compares the complete ordered union of each section with
the same immutable cached source, rather than only checking counts. An additional
small, database-free context/comparison regression checks default 25, explicit
100, invalid limits, exact unions and frozen identity/expiry.

Paired measurements from the same synthetic large fixture and environment:

| Complete journey | Default-25 baseline calls / bytes | Explicit-100 audit calls / bytes |
| --- | ---: | ---: |
| Financial context, all sections | 43 / 947,243 | 19 / 916,043 |
| Financial comparison, all sections | 67 / 2,070,252 | 37 / 2,031,251 |

The comparison preserves all 480 positions, 614 gaps and 410 evidence rows.
The context preserves 480 positions, 307 gaps and 205 evidence rows.
No larger budget or production optimization is claimed. These are actual SDK
HTTP response bytes, not tokens, latency or macOS performance measurements.

## Validation boundary

Both representative and perf-tagged large payload journeys pass. The focused
financial default/bulk regression passes. This is a test-harness correction;
production code and frontend are unchanged. Native macOS CI, GUI acceptance,
release packaging, signing and real-household runs are separate from this Linux
synthetic evidence. No real ledger was accessed.
