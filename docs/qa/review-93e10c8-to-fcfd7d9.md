# Review after 93e10c8

Baseline `93e10c8c3f77a36a153d16a3432d0b4287b9c55c` (exclusive) through endpoint `fcfd7d99464873f48d201b2d71c1dafc3b5879d0` on `main`. The range contains 57 commits. Associated pull requests are #106–#116, #118–#131, #133–#137, #139, #143–#145, #147, and #149–#164. Six commits have no pull request number: `4c3dc34`, `77aae76`, `1615057`, `cf7d9f6`, `dc199c1`, and `c14308c`.

Three confirmed defects remain at the endpoint. None change stored ledger amounts. There is no P0 or P1.

Line numbers below are from `fcfd7d9`.

## P2 — A funding gap is stamped on every same-currency holding

- File: `internal/application/funding_gap.go`, lines 19–25. The day downgrade is at line 249. The rate gate is `internal/application/analysis_return.go`, line 142.
- Introduced by `6539d7b`, pull request #144.

`annotateReplayFundingGaps` writes `funding cash is recorded after the product start` onto every complete holding in the gap account whose native currency matches. `valueHistoricalSnapshot` stores that reason. `applyFundingGapMark` then turns any OK component day that touches those items into `CompletenessPartial`. A daily investment rate is published only when every investment component that day is OK.

The trigger is a holdings account whose cash goes negative because a product starts before its funding deposit, while another complete holding in that currency remains, such as an ETF bought earlier. `ReplayFundingGap` records the account, currency, and the activity that first made cash negative. The stamp does not use that activity. The existing term-deposit test has only one holding, so it still passes. Historical overview marks the missing cash sleeve and the account parent, and does not stamp sibling holdings.

The other holding keeps its amount. Its stored reason makes household and instrument-scoped investment returns partial and drops that day's rate, including when cash is excluded. The unrelated holding stays labeled as a funding-gap product until those snapshots are rebuilt.

Stamp the synthetic cash line and the holdings affected by the gap activity. Leave the other holdings' missing reasons unset, and keep the snapshot incomplete so household totals stay unpublished.

## P3 — Signed yen and won that round to zero show two plus signs

- Files: `frontend/src/lib/money.ts`, line 466, with the zero-scale hint at lines 366–368. The same prefix is in `frontend/src/features/historical-overview/historicalAmount.ts`, line 9.
- Introduced by `9c21b2de`, pull request #136. That change made JPY and KRW round to zero decimal places and put a sign in the tiny-amount hint. The outer `> 0.005` prefix was already present.

`formatSignedMoney` and signed `formatHistoricalAmount` prepend `+` when the raw amount is greater than `0.005`. A positive JPY or KRW amount in `(0.005, 0.5]` now rounds to zero and still receives that plus. `frontend/src/lib/money.test.ts` already locks unsigned `0.1` JPY as `¥0 · + nonzero, below ¥1`. The signed path produces `+¥0 · + nonzero, below ¥1`. `0.5` JPY stays on zero by half-even rounding and does the same. `0.5001` JPY becomes `+¥1`. A negative amount does not get the outer plus. USD `0.004` stays a single hint because it is not above `0.005`.

This appears on signed returns, historical deltas, and holding gains in JPY or KRW. Stored amounts are unchanged.

Add the leading `+` only when the currency-rounded amount is nonzero, and leave the sign to the tiny hint when the displayed amount is zero.

## P3 — Three review dates stay in ISO form

- File: `frontend/src/features/investment-review/InvestmentReviewPage.tsx`, lines 192, 209, and 240.
- Introduced by `9c21b2de`, pull request #136.

The same Investment review card sends window, evidence, and coverage dates through `formatCivilDate`. These three civil dates stay raw:

- `content.publishedOn`, the published closed day
- `rule.suppressedUntil`
- `item.UntilOn`

With day-first, month-first, or localized date settings, one card shows both `15/10/2026` and `2026-10-15`.

Pass those three fields through `formatCivilDate` before interpolation.

## Areas with no confirmed issues

Monetary and quantity calculations: reviewed; no issues found. Blended cost uses exact remainder division and half-even rounding at eight places. CoinGecko prices use banker's rounding to eight places after the numeric lexeme is preserved. Historical review amounts outside the native-amount or money range become unknown evidence. Exact FX snapshot amounts stay unrounded until `NewMoney`.

Data and migrations: reviewed; no issues found. Writable open accepts schemas 21 and 22. Schema 21 upgrades in place after a verified recovery copy. Older files are refused and left unchanged. Backup and continuous backup copy the whole SQLite file and use the same schema check, so the source journal, temporal checkpoints, and closure table are included. The Sonic wrapper was checked against `encoding/json` for money decimals, time values, nil and empty slices, HTML escaping, and stream framing.

Application logic other than the funding-gap stamp: reviewed; no issues found. Account-scoped snapshot loading widens to the full item set when a snapshot is incomplete or an item account is unknown. Quote batching checks household ownership. Direct FX suffix reuse fails closed unless both dates, provenance, and the journal agree.

Frontend other than the two display findings: reviewed; no issues found. Complete historical amounts are formatted from the exact value once. USD displays as `US$`, and TWD is pinned to `NT$` where a bare `$` would appear. Settings save sends the edited patch onto a fresh load. Review preview output is cleared when the spec changes. Market-data sync start stays disabled until the latest preview matches the current force-recheck flag. Locale JSON key sets match across English, Simplified Chinese, and Traditional Chinese. Reconciliation entries kept for Back are keyed by navigation request id.

Dead code and performance: reviewed; no issues found. The net tree removes the retired review experiments and the pre-21 migration chain. No new N+1 on the changed financial paths was confirmed.

Tests: reviewed; no issues found. The new half-even cost, CoinGecko lexeme, out-of-range historical amount, single-holding funding gap, schema-21 restore, and sync-preview cases exercise those branches. They do not cover a second holding beside a funding gap, a signed sub-yen amount, or the three raw review dates.

At `fcfd7d9`, `go test` passed for `TestReplayCostBasis`, `TestReviewDailyAmountBounds`, and `TestCoinGeckoPriceHalfEven`, plus the matching schema and funding-gap tests in `internal/application`, `internal/infrastructure/sqlite`, and `internal/infrastructure/continuousbackup`. Frontend unit tests were not rerun in this environment because frontend dependencies were not installed.

## Disposition

Issues that can be fixed immediately:

- Limit the funding-gap stamp to the gap activity's holdings and the synthetic cash line.
- Prefix `+` only when the currency-rounded signed amount is nonzero.
- Format the three remaining review dates with `formatCivilDate`.

Issues that require a design discussion first: none.
