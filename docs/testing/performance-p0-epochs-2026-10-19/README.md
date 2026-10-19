# Synthetic P0 PR-B evidence

- `before-*`: immutable source `eb096bc2a55fcda31f498aa6a6ed3e4e8d341db6`.
- `after-*`: query source `a949c01b618bb02b772665644d7a4b030fec634f`.
- `initial-epoch-regression.log`: expected failing full-replay gate before its fix (`11ca5f1`); retained rather than discarded.
- `focused-gates.log`: passing query-count, repeated-epoch and acknowledged midnight regressions after the fix.
- `distributions.csv`: complete nearest-rank median/p95 summaries, milliseconds; path 0 is baseline, path 1 candidate.
- Validation logs record actual exit status and native dependency blockers; see the [report](../performance-p0-epochs-2026-10-19.md) for interpretation.
- `SHA256SUMS` covers the raw logs and CSV. No household database, settings, provider data or credentials are included.

All formal timing jobs ran serially before validation/build/race work. Each workload has 30 paired samples for each of four fixed scenarios, not 120 different semantic cases. Raw outliers and unfavorable tails remain included; production default activation is blocked.
