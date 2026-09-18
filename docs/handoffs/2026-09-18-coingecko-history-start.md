# CoinGecko and per-instrument history start

## Behavior

- Every instrument uses its first economic holding date across household accounts, bounded by the household history origin. Nonzero opening positions start at the household origin; later positions start at their first non-reversed quantity addition. Backdated entries extend the range; reversed entries do not create an artificial earlier start. Sold/archived holdings retain their historical requirement.
- Creation time is not used. An instrument that has never been held is a latest-only sync target. Manual-price health checks use the same first-holding date.
- Opening reference lookup starts seven calendar days before that date, expands to thirty days if the first window is exhausted, and stops there. Existing prices are retained.
- CoinGecko is selectable for crypto and is the default for new crypto search. Existing provider bindings are not migrated.
- Search returns exact coin IDs (e.g. bitcoin), distinct from display tickers (BTC). Coin IDs remain lowercase. The form defaults to USD and filters the local fiat currency catalog against CoinGecko when its currency catalog is available.
- New CoinGecko instruments attempt a latest quote after saving; failure does not discard the instrument.
- Latest requests batch up to 100 identities per currency. Requests use the Demo key header, a bounded credential-scoped cache, and one-second pacing. Search caches for five minutes; latest prices for sixty seconds.
- Historical requests use market_chart/range with daily interval. The real midnight UTC timestamp is retained as the reference timestamp; it is not shifted to the previous day or treated as an equity closing time. Trailing intraday observations are discarded. Missing daily points remain pending.
- Demo's rolling 365-day limit is enforced before requests. The oldest requested date is the first full UTC day inside the limit. Unavailable older gaps remain visible and make sync partial; recent prices can still sync. Stored older history is retained.
- Current eight-decimal unit-price precision remains in force. Positive prices smaller than the supported precision fail as unavailable rather than becoming zero.
- Settings expose save/delete/status for the key. Status/load DTOs never return it. No real key was embedded, written into this handoff, or used for live requests. No database schema migration is needed.

## Manual acceptance

1. Save the Demo key in Settings → market-data credentials.
2. Add an instrument, choose Crypto, search BTC, and select Bitcoin / bitcoin. Verify source CoinGecko, symbol BTC, ID bitcoin, and default USD.
3. Choose CNY or SGD, save, and check the latest price and provider time.
4. Before adding a holding, run instrument sync: latest is refreshed, with no historical backfill requirement.
5. Add a holding today and sync: history should begin around today minus seven days, regardless of household age.
6. Backdate a purchase and sync again: only the newly needed earlier range should be added. Another account's earlier holding should determine the shared instrument start.
7. Repeat the first-holding checks with a stock, metal template, and a manual instrument.
8. Open price history: CoinGecko attribution and daily UTC reference labels should appear. Existing Yahoo crypto should retain its source.
9. Check missing/invalid key and rate-limit feedback. If genuinely needed dates exceed the Demo window, verify a visible gap rather than a complete history or zero value.

## Verification

- Go build ./...: PASS (existing macOS deployment-target linker warnings).
- go vet ./internal/...: PASS.
- Frontend production build (bindings, TypeScript, Vite): PASS (large-chunk warning).
- Frontend lint: PASS.
- Go tests across application, domain, marketdata adapters, settings, and Wails settings/marketdata/catalog: PASS after explicitly excluding four failures also reproduced against the unchanged HEAD baseline.
- Focused adapter/flow tests cover identity search, batch prices, missing/tiny prices, rate-limit stopping, currency filtering, daily timestamps, missing daily points, persistence/valuation policy, history horizon, ownership starts, latest-only instruments, and secret persistence.
- Frontend: 36 passed, one baseline failure excluded across settings, instrument management, marketdata queries and locale coverage. Includes CoinGecko search and non-USD creation.
- Native app and live Yahoo/CoinGecko/FX requests: NOT RUN.

Baseline failures reproduced independently:
- TestLegacyFixtureSupportsRepositoryGainReads: old SQLite fixture lacks metal_template.
- TestGainServiceTransferUsesSendingCostAtTransferTime: same legacy fixture issue.
- TestLoadReturnsDefaultsWhenNoFileExists: expected DTO omits the resolved log path.
- TestResetRestoresDefaults: same log path expectation.
- InstrumentManagement “saves a manual instrument quote from Set price”: cannot find the Unit price input after the dialog transition.

During verification pnpm attempted to reinstall dependencies after a temporary baseline checkout reused node_modules. Restored the original workspace with pnpm install --frozen-lockfile, using cached packages; dependency manifests/lockfile are unchanged.

## Provider references

- https://docs.coingecko.com/demo/reference/authentication
- https://docs.coingecko.com/demo/reference/simple-price
- https://docs.coingecko.com/demo/reference/search-data
- https://docs.coingecko.com/demo/reference/coins-id-market-chart-range
