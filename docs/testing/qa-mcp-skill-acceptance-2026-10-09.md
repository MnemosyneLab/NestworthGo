# Real MCP and skill acceptance — scoped user report

**Status:** Passed for the scenarios listed below, as reported by the user.
This record is based on the user's summary of a Mac mini acceptance report
retained on that machine; the report itself was not accessible from this cloud
workspace and was not independently inspected here.

## Tested setup

- Nestworth source: `2cad89eddd0c02de28f68a631fc26a72d6f6fcc3`.
- Codex CLI `0.160.1`, model `gpt-6.1-sol`, Nestworth skill `1.6.0`.
- Real model calls over the local MCP connection, with test/synthetic financial
  data. This was client acceptance, not native GUI acceptance.

## Reported passing scenarios

- The model used read, managed-product, liquidity, financial-comparison and
  attribution tools. Three independent cursor sequences made 268 continuation
  requests in total.
- Across separate turns, a product preview was reviewed, explicit user
  confirmation was given, the corresponding commit completed, and a replay
  returned the original result. In the synthetic fixture, cash moved from 500
  to 510, forecast interest moved from 50 to 40, and exactly one interest
  receipt was recorded.
- A write attempt in `read_only` was rejected. Financial facts remained
  unchanged by that rejected attempt.
- A large liquidity fixture returned `too_large`; retrying with a smaller
  fixture returned successfully.

## Scope limits and environment cleanup

This does not validate other product-write variants, real market/provider
connections, native GUI flows, or a freshly built release artifact. The
acceptance used source SHA `2cad89e`, before the PR #47 CI-only dependency
installation change merged. It is not evidence that a v0.3.7 package was built,
checksummed, launched, or accepted.

The user reports that the temporary test service was stopped and no real ledger
or global client configuration was changed. Mac native GUI acceptance remains
explicitly deferred to the user.
