# Market-data repair eligibility and query details

## Changes

- Repair preview and latest execution share `syncLatestTargets`. Recent successful checks are skipped using the configured quote-cache TTL (default 12 hours). Preview lists those skips and excludes them from estimated requests. Execution re-evaluates eligibility after history downloads, which may remove more latest requests.
- Persistent provider `fetched_at` is considered after restart. An unchanged quote updates its fetch timestamp without rewriting its observation time or creating a new observation. In-memory check identity includes provider, symbol and instrument update time.
- Explicit single-target refresh and Force Refresh All remain explicit rechecks; Force Recheck bypasses repair eligibility. Normal Update Latest and Repair All reuse fresh checks.
- CoinGecko latest estimates account for batches of up to 100 distinct IDs per currency. Estimates are not a promise of exact HTTP counts: provider caches, retries, conversion dependencies and mid-job eligibility changes can affect actual work.
- Prior job failures remain in job details and are not independently added to current data-health findings. Current missing data and valuation issues still appear. Historical instrument failure keys now use instrument IDs rather than provider symbols.
- Repair previews on both Market Data and Data Health show names, symbols, sources, date ranges and decisions. Sync details expose the current query and completed item outcomes, including failures. Market Data's asynchronous latest refresh publishes request-scoped progress and displays each target while running. Completed sync details remain accessible.
- Provider history begins seven calendar days before the first effective holding date. Opening holdings use the household origin; never-held instruments use their creation date. Existing opening anchors no longer eliminate the seven-day lead-in. The existing thirty-day fallback for a missing opening anchor and CoinGecko history limit remain in effect. The first repair after this change may therefore legitimately add older prices.

## Manual checks

1. Update latest prices, then open Repair All. Fresh latest prices and FX should appear as reused, excluded from the request estimate.
2. Restart and repeat: successful rechecks should still be reused even when the provider returned the same observation.
3. Inspect preview rows for names, provider symbols, sources and explicit historical date ranges.
4. Start a repair; verify the current query and completed results on Market Data / Data Health. Reopen progress after completion. Check cancellation.
5. Update latest prices; verify named per-target progress. Force Refresh All should still request current data.
6. Check a previously reported QQQM authentication error: old errors should no longer count as current health gaps. Genuine missing history remains visible.
7. For first ownership on September 18, inspect a history start of September 11 even when September 17's close already exists. For a never-held instrument, use creation minus seven days. Backdated ownership should move the range earlier.

## Verification

- Application and Wails market-data tests: PASS, excluding two previously confirmed baseline legacy fixture failures (`TestLegacyFixtureSupportsRepositoryGainReads`, `TestGainServiceTransferUsesSendingCostAtTransferTime`).
- SQLite tests: PASS excluding existing `TestOpenRejectsSchema6FixtureWithoutWriting` (unchanged HEAD expects supported schema 10, current schema is 11).
- New regressions cover shared eligibility/preview decisions, persisted unchanged-price rechecks across restart, seven-day lead-in with an existing anchor, old failure health handling, and named running/completed refresh events.
- Frontend related tests: 27 passed, including named preview rows, date range, provider and current-query display.
- Frontend production build and lint: PASS. Existing bundle-size warnings remain.
- Go vet for the changed backend packages: PASS.
- Native app, real household data, and live provider requests: NOT RUN.
- No commit or push performed.

## Follow-up: inferred exchange closures (September 19)

- Default equity repair treats an empty date between earlier and later canonical closes from the same coverage identity as an inferred closure, including expired no-observation records. Such dates no longer generate missing-history health issues or automatic rechecks.
- Leading history before the first known close, trailing dates after the last close, explicitly unverified observations, crypto, and unknown markets are not inferred closed. Existing weekend handling remains unchanged.
- Force Recheck can query inferred weekday closures. The inference is not persisted as provider-confirmed coverage or as a synthetic price.
- Historical valuation applies the same coverage rule while carrying the preceding price; it does not use the later price as the earlier day's value. Provider, binding revision and source policy matching remain required.
- Domain/application tests passed with the two previously documented legacy fixture tests excluded. New tests cover Tuesday holidays, expired no-observation records, force rechecks, leading/trailing gaps, crypto, explicit unverified dates and binding isolation. No native app or provider requests run.
