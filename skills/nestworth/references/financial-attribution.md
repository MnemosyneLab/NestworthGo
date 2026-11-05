# Link a comparison to change attribution

For “why did these two values change; was that profit?”, call
`compare_financial_attribution` directly. Minimal needs no `get_context`, catalog,
Bootstrap or directory read. Differences alone use
[financial comparison](financial-comparison.md), which does not write. This tool
is a **new coherent capture**, with a new `comparisonId`, after derived snapshot
maintenance. It does not attach live analysis to, validate or modify an old
frozen comparison. Reuse the selected dates, scope and disclosure, then discard
the old page set and interpret the new package. Never supply an old
`comparisonId` to this tool.

<!-- example: financial-attribution -->
```json
{"leftAsOf":"${leftDate}","rightAsOf":"${rightDate}","scope":{"kind":"household"},"disclosure":"minimal"}
```

A and B must be closed history-local dates, with A before B. The server
normalizes the inclusive analysis period to **A's next local date through B**,
with A's close exclusive and B's close inclusive. A/B is not an inclusive
analysis range: including A again double counts that day's changes. Use the
returned timezone, `period.from`, `period.to`, `startExclusive` and
`endInclusive`. DST days need not last 24 hours. Same-day and reverse requests
fail validation. Dates before history, today and future closed-date labels are
invalid.

The first version supports household or one real account UUID:

<!-- example: financial-attribution-account -->
```json
{"leftAsOf":"${leftDate}","rightAsOf":"${rightDate}","scope":{"kind":"accounts","accountIds":["${accountId}"]},"disclosure":"minimal"}
```

Use a supplied UUID or an already authorized identity lookup. Package aliases
are not UUIDs. Never discover extra identities silently. Multiple distinct
accounts return `unavailable` / `unsupported_account_set`. Do not aggregate
account return rates. A current right endpoint returns `unavailable` /
`right_endpoint_not_closed`, with no fabricated period return. Named disclosure
follows the explicit identity-disclosure rule. The link preserves `leftScope`
and `rightScope`. Its `scope` is their shared-ref endpoint account union.
`scopeBasis` declares that `includedAccountCount` counts accounts included on
either endpoint. Read both endpoint scopes: an account created during the period
can enter the right scope and attribution while being absent from the left.
Union counts describe those endpoints, not an extra directory read or permission
expansion. The server fills actual missing days of A..B even if an existing
completion watermark is later. Requesting a later range first must not strand
early days. It rebuilds dirty days within the request while retaining pending
earlier days. A later request cannot mark an unbuilt earlier dirty range
complete. Midnight preflight includes A itself and every day through B+1.
Unsupported ambiguous or nonexistent boundaries return
historical_boundary_unsupported before snapshot maintenance.

Check `content.attribution.status` before an explanation:

- `compatible`: every daily snapshot's scoped evidence and cutoff matches fresh
  replay on the captured facts and configuration, and the historical inclusion
  agrees with the current analysis universe. Endpoint checks and the waterfall
  reconciliation are additional guards, not numeric proof of compatibility.
  Explain drivers, preserving `assetStatus`, `available`, `missingReason`,
  `residualIssueCount` and all amount and return coverage fields. Compatible can
  still have partial classification and a nonzero residual.
- `incompatible`: stop the joined explanation. Report `mismatchReasons`, such
  as `inclusion_mismatch`, `snapshot_evidence_mismatch`,
  `snapshot_cutoff_mismatch`, `resolver_policy_mismatch`,
  `source_revision_changed`, `analysis_endpoint_mismatch`,
  `driver_reconciliation_mismatch` or `historical_boundary_unsupported`.
  Do not describe these as zero profit or join a separately fetched live report.
  An unusual ambiguous or nonexistent midnight can be supported by the comparison
  but unsupported by the current analysis engine. A repair is a separate,
  user-authorized workflow, never an automatic fallback.
- `unavailable`: explain the missing evidence or unsupported scope or endpoint.
  `missing_valuation_evidence`, `missing_snapshot_boundary`, `missing_boundary`
  or `missing_driver_evidence` must not become zero. `account_endpoint_absent`
  and `empty_analysis_universe` preserve the lack of a usable account boundary
  or eligible analysis universe. Keep partial measured values labeled partial
  and nullable values unknown.

`beginningValue` and `endingValue` are the existing analysis engine's base-money
projection: half-even round each signed component boundary to four places on
each day, then sum. Comparison values retain exact decimals. Never round the
whole household instead of its components. Exact component and day driver buckets
are aggregated by period driver, then the existing deterministic four-place
waterfall projection is reused. `explainedDelta` sums those projected drivers
except residual. Read `precision.amountScale`, `rounding`, `boundaryBasis` and
`driverBasis`. Preserve all signed adjustments, including values beyond four
places. For compatible measured values:

```text
analysisDelta = endingValue - beginningValue
analysisDelta = explainedDelta + residual + precision.driverAdjustment
change.netWorth = analysisDelta + precision.boundaryAdjustment
precisionAdjustment = precision.boundaryAdjustment + precision.driverAdjustment
change.netWorth = explainedDelta + residual + precisionAdjustment
```

The server strictly checks exact scoped boundaries, projected component
boundaries, each component and day's exact buckets plus proven neutral transfer
or principal legs, and each day's exact buckets before declaring an adjustment
to be precision. Real exact gaps still stop as driver_reconciliation_mismatch,
even if small or cancelling across components or days. No epsilon is used.
`precisionAdjustment` is the difference between the proven exact change and
existing projected drivers and residual. Its boundary and driver parts may
oppose. It is **neither profit nor unknown residual** and must not be added to
investmentReturn. For USD 1 with FX 7.123456 to 7.234567: exact change 0.111111
= projected drivers 0.1111 + residual 0 + precision adjustment 0.000011. An
unchanged six-place valuation can have rounded boundaries but zero delta and
adjustment. Missing or incompatible reconciliation fields remain null. Never
invent zero adjustments.

Residual is unexplained change, never assumed market profit. Opposing residuals
can cancel. Keep `residualIssueCount`. `investmentReturn` is the independent
investment universe, with cash excluded in this version. Preserve its `amount`,
nullable `rate`, `available`, `status`, `missingReason`, `ratedDays`, `totalDays`
and source breakdown. Income, contributions, adjustments, transfers and
repayment principal are not investment profit. Household internal transfer and
repayment principal do not create wealth. Single-account external flows can
change that account's value. Missing price or FX remains unknown.

The link is a bounded summary of existing asset drivers and investment return
sources. For instrument, account or activity drilldown, call `analyze_period`
or `list_contributions`, then `get_read_item` on that capture. Contribution
rows use section=`rows` and id=the row key; child sections are
`rows/<key>/components` and `rows/<key>/byAccount`. A return day uses
section=`returnDays` and id=YYYY-MM-DD. An asset-change bucket uses
section=`assetDrivers` and id=the bucket key. Those period sections are frozen
with `analyze_period` and are not a second capture. These identity-bearing
workflows require the corresponding disclosure intent. Use the normalized
period and actual scope ID. **They are not detail pages or proof for this
comparisonId.** Do not quietly substitute them after a compatibility failure.
See [analysis](analysis.md).

## When the link fails

Read positions, gaps and evidence with `get_read_page`, saving the three section
states independently as in [financial context](financial-context.md). Retain the
initial link and precision summary. Pages return only section, items, page and
capture identity, hash and expiry. The cached link and fixed five-minute TTL
remain unchanged. On expiry, eviction or revocation, recapture with
`compare_financial_attribution`, discard all old pages and restart every section
with new cursors. Never mix old and new IDs, even if hashes coincide. Send one
JSON-RPC object. Mixed batches are rejected. The 64 KiB wire and package budgets
also cover the link. An oversized mandatory summary or row cannot be fixed with
a smaller page.

This analysis tool may materialize **derived** snapshots and invalidate a ledger
preview. Its MCP annotation is non-read-only and non-destructive, while it stays
available in `read_only`. It writes no financial facts, refreshes no providers
and adds no token permissions. Run it before ledger previews.
`compare_financial_context` remains the strictly no-write choice for differences
alone. A repository skill update does not install or update the user's App,
client skill or MCP connection.
