# Analysis input and computation maintenance

## What the budget measures

`TestAnalysisPerformanceBudgets/cold-3y` times only `ComputeAnalysis`, the pure
in-memory analysis kernel. Input construction happens before the timer. It does
not time desktop startup, SQL, snapshot maintenance, service memo population, or
frontend rendering. The race build explicitly skips this subtest; a race pass
is not evidence that the cold budget passed.

The unchanged synthetic fixture has 500 USD balance components, each valued at
100, with no activities or quotes. The range is 2023-01-01 through 2025-12-31:
1,096 days, 548,000 result component-days, and 1,097 snapshots including the
opening day. Every benchmark iteration computes the entire result without a
service memo. All experiments used synthetic inputs and temporary test data.

## Kernel implementation

The current kernel avoids redundant work:

1. `NewSignedMoney` uses `Round(4)` when the decimal exponent is already at least
   -4. There can be no half-even tie in this case. This keeps the exact value
   and the original -4 result exponent while avoiding `RoundBank`'s remainder
   and tie checks. Finer scales still use `RoundBank(4)`. Currency validation
   and the overflow check before rounding stay in place.
2. Component keys use a fixed four-element scratch array instead of growing a
   slice, and each day reuses one key for its three map lookups. Key strings and
   identity rules are unchanged.
3. Residual tolerance uses exact decimal constants `2 × 10^-places` and
   `1 × 10^-9`, replacing repeated multiplication and float-to-decimal parsing.

No cache contracts, financial formulas, fixture sizes, budgets, or tests are
removed or relaxed.

## Correctness evidence

Regression tests compare the amount and exponent against the original
`RoundBank(4)` operation across exponents -16 through 8, positive and negative
half-even ties, exact values, and values close to the money limit. Separate
checks cover overflow before rounding, currency normalization/rejection, all
component-key branches, and residual tolerances around both currency floors.

The analysis correctness, snapshot invalidation, FX, holding, attribution and
boundary tests remain the broader regression coverage. Historical measurements
and their limitations belong in the [performance report](performance-optimization-report.md).

## Analysis quote input loading (G, 2026-10-20)

The SQLite analysis input loader now batches complete quote histories through
an optional application port, preserving the legacy per-instrument fallback
for other adapters. One read transaction covers household/instrument validation
and bounded 900-ID chunks; dates and candidates are not clipped. Requested
instrument order and the legacy quote tie order remain unchanged. Household
portfolio hydration, snapshot maintenance, the analysis kernel and memo policy
remain unchanged. See [G correctness and query evidence](https://github.com/MnemosyneLab/NestworthGo/blob/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d/docs/testing/performance-g-analysis-quotes-2026-10-20.md)
for differential tests, net SQL counts and the limits of two small Linux timing
observations. This input-loading evidence does not establish overall latency or RSS.

## Account snapshot component loading (H2b, 2026-10-20)

Normal SQLite analysis hydrates selected-account stored snapshot items for
explicit account scope, including households with activities. It retains full
activities, archived-inclusive portfolio metadata, FX and selected source evidence.
All historical account IDs must resolve against the captured portfolio; unknown
accounts and other scopes retain full hydration. If any requested household header
is incomplete, the request also retains full item hydration: omitted outside-account
items may distinguish a funding-only gap from an unrelated missing price, and
that distinction controls activity-proven opening/closing zeroes. Complete requests
retain selected-account hydration. Headers and full origin opening
construction remain unchanged. The initial activity-free restriction was removed
after tracing classifier/daily dependencies and adding real-loader differential
coverage for transfers, corrections, positions and outside cash attribution.
See [H2b dependency boundary and synthetic evidence](https://github.com/MnemosyneLab/NestworthGo/blob/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d/docs/testing/performance-h2b-analysis-snapshots-2026-10-20.md).
