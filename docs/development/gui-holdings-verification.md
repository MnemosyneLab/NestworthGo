# GUI consistency and holding details

Implementation baseline: `main` at `2bcad6cde1dfecb61493dbd9521f802c7eaa972c`.
The GUI audit and its October 14 supplement were read from their materialized
Library files in the implementation executor. The supplement's minimal scope
and descriptive-performance date rules take precedence.

| Scoped item | Implementation and regression evidence |
| --- | --- |
| Ordinary money, including historical cards/tables/comparisons/tooltips/drawers | Shared decimal/string rounding, fixed two display decimals including JPY/KRW; tiny nonzero direction/size hint, neutral display zero, explicit exact audit/confirmation evidence. ISO command scales and financial inputs remain exact. Money, historical overview, liquidity, account, History and insight tests cover the paths. |
| Price/NAV, quantities and FX | Dedicated price formatter preserves canonical precision; migrated saved prices, quote history, trade/position hints, metal evidence and transaction unit prices. Quantity/rate formatting and calculation helpers retain their existing precision. |
| Business labels and historical warnings | Evidence uses shared observation semantics; internal IDs/policies are omitted. Historical missing entries carry account/instrument business names. Tooltips cap summaries at three entries and wrap within the viewport; full warnings remain in date-row disclosures. Repair blockers, missing metadata, analysis residuals and duplicate selectors use business fallbacks/ordinals. Historical and identity regressions plus a narrow source guard cover recurrence. |
| Managed products | GUI inventory joins authoritative ProductContract ownership across all states, without product valuation reads. Shared ordinary eligibility filters catalog, Buy/Sell, dividend/transfer/adjustment and new holding/position choices, validates preselected IDs, and blocks failures with retry. Managed archive/price actions are absent. Wealth/history/liquidity inventory and backend guards remain. Ownership tests include settlement and an ordinary manual bank investment; existing product-operation guards remain tested. Zero holdings show their actual contract quantity. |
| Account layout and shared details | Cash and holdings are full-width stacked cards with table-local overflow. Both account and Portfolio entries mount the same lazy detail body. Portfolio retains account breakdown and both views. Metadata, native-currency cost/value/P&L and local price history have separate loading/error behavior. Account summaries use one scope-specific cost replay, including explicit archived-account reads. |
| Performance and caching | Pure decimal window calculation shared with investment review; latest real observation, nearest earlier baseline, actual civil days and visible dates. No alert freshness/baseline-gap cutoff. Missing 30-day history hides that metric. Whole valuations and unsupported unit/distribution changes do not produce returns. Flat unit NAV can show zero NAV change; it does not establish income or personal return. Agent, Yahoo and Tiingo raw closes are supported, including a zero stock/ETF endpoint with a positive baseline. Agent closes retain their Agent source identity and raw-close basis; comparable observations must match source, source key, binding revision, currency and timestamp/price bases, with known splits rejected. Raw stock price changes remain distinct from personal return. Quote history retains each observation’s original currency in its table, plots only the current currency, and breaks connections when price basis changes. Instrument-keyed finite caches, cancellation and existing invalidation cover expanded active rows; no per-row polling or provider fetch. Domain, SQLite and lazy-detail tests cover this contract. |
| Select controls | Shared field appearance; native select semantics retained. Combobox committed value and temporary query are separate, reopening shows all candidates, blur/Escape/Tab cancel, disabled choices are skipped, IME confirmation is guarded, and active descendants have stable IDs. Component and existing Settings/Analysis regressions cover interaction. |

Verification uses synthetic fixtures only. No real household database was opened,
no release/package identity was changed, and no user screenshot or financial data
is included in the PR. Browser screenshots used for private inspection are
synthetic and are not repository deliverables.

Native macOS/WebView Chinese IME, VoiceOver, packaged launch/relaunch, signing,
notarization and release acceptance require a Mac and remain unverified. Linux
server/headless builds, jsdom tests and Chromium checks are separate evidence.
Exact command results and environmental build limitations are recorded in the PR.

## Executor evidence

Environment: Linux amd64, Go 1.26.0, Node 24.19.0, pnpm 12.2.1,
Wails v3.0.0-beta.28. The executor's environment setup supplies
`GOFLAGS='-mod=readonly -tags=server'` and writable Go/Node caches.

- `CGO_ENABLED=0 go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.28 task check`
  passed: production frontend build, generated-bindings freshness, formatting,
  `go test ./...`, `go vet ./...`, server build, frontend lint/typecheck/tests,
  and `git diff --check`. Frontend result: 116 files, 1,222 tests passed.
- Subsequent frontend lint/typecheck passed after final integration cleanup.
  The focused money/presentation/lazy-detail smoke also passed.
- Open and settled managed-product regression exercises generic instrument-only
  Buy, Buy with a holding, Sell, dividends, transfer, adjustment and import at
  preview and fresh commit. Create holding, metadata edit, archive and manual
  quote writes are refused as well; the before/after financial snapshot is equal.
- Chromium, using an in-memory synthetic QueryClient fixture: stacked cards at
  1440px; account and Portfolio detail expansion; precise saved NAV and actual
  performance dates; searchable select reopen/filter/cancel; 390px dark-mode
  layout with document width exactly 390px; English and simplified Chinese;
  no page errors or visible protocol keys. These are browser checks, not native
  desktop, actual-provider, or end-to-end database acceptance.
- The frontend build reports large ECharts/shared display chunks (about 588/683
  kB before gzip). It succeeds; no unrelated bundling redesign is included.
- `GOFLAGS=-mod=readonly go build ./cmd/nestworth` cannot compile the canonical
  Linux desktop target because GTK4/WebKitGTK6 and their development metadata
  are absent. Native race compilation has the same dependency limitation.
  These failures do not count as successful desktop or Mac acceptance.

- Application race coverage used the repository checksum selector across all
  four parts: `go test -race -p 1 -parallel 4 -timeout 10m -run "$regex"
  ./internal/application`, with `GOMAXPROCS=2`. All parts passed (221.849,
  505.946, 173.257 and 524.642 seconds). The initial unpartitioned attempt was
  replaced by these canonical CI partitions.
- `go test -race -p 1 ./internal/domain ./internal/infrastructure/...
  ./internal/wailsapi/analytics ./internal/wailsapi/instrument
  ./internal/wailsapi/quote` passed. A final scoped race rerun of descriptive
  performance, quote metadata/series, ownership guards and SQLite performance
  evidence also passed after the chart-currency addition.
- A parallel validation run exceeded the existing cold-analysis 10-second
  budget (11.1 seconds) while race jobs were busy. The unchanged budget test
  passed on retry, and the final frozen-code aggregate passed. No thresholds or
  validation requirements were relaxed.

## Independent review corrections

- Both supported Yahoo and Tiingo raw-close bases permit descriptive stock/ETF
  price change, while source, binding, currency, basis and split consistency remain
  required. Yahoo stock and ETF regressions assert 100 to 105 over eight days.
- A flat ordinary unit-NAV fund shows 0% NAV change over its actual nine days.
  Flatness is not evidence of money-fund status. Distribution evidence and
  whole-position valuation remain unsupported for these NAV metrics; the UI
  describes price/NAV change, never income or personal return.
- Daily close selection keeps the latest zero stock endpoint, yielding -100%
  with a positive baseline and exact dates. Zero divisors and unsupported latest
  evidence suppress the metric instead of silently selecting a stale endpoint.
  NAV prices must remain positive.
- Distribution and share-change safety checks use the existing corrected
  economic activity timeline. A legacy Oct 5 dividend replacement recorded Oct 20
  still suppresses the Oct 1–9 NAV metric.
- Waterfall reconciliation uses ordinary two-decimal display precision for JPY
  as well as other currencies, independently of financial command scales.
- Account holdings wait for both holdings and instrument metadata and show
  errors/retry before an empty state or buy actions. Cash remains independent.
- Derived descriptive percentages use two decimals. Account cash changes and
  insight signed amounts use shared signed-money presentation, preserving the
  tiny nonzero hint without a signed rounded zero.

Review regressions use isolated synthetic fixtures. Final check counts and
commands are recorded in the PR; native Mac acceptance remains unverified.
