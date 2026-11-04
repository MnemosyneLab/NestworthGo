# MCP account and Agent audit input scope

The bounded MCP capture protocol freezes selected results after authoritative
application reads. Response pagination alone does not bound database input work.

## Account valuation captures

`get_account_snapshot` and `get_account_valuations.accountIds` intersect the
existing account filters in `domain.AccountFilter` before account hydration and
valuation. Explicit unknown or excluded IDs produce no account rows; the snapshot
tool retains its existing not-found response. An omitted/empty ID list retains the
existing broad account-filter behavior.

For explicit IDs, the current portfolio transaction restricts holdings and current
cash values to the matched accounts, and instruments, current instrument quote
candidates and closure facts to instruments referenced by their holdings. This
includes archived holdings/instruments required by the existing authority; it does
not change the valuation engine's handling of archived/zero/missing inputs.
Current quote grouping and full-history correction/retraction predicates remain
unchanged. Household metadata, history origin, FX candidates/preferences and
household directories remain shared inputs. Account display-currency conversion
therefore retains its established FX and completeness semantics.

`totalAccountCount` now counts accounts within both the ordinary filters and
`accountIds`; `countsScope=request_account_filters_and_accountIds` makes this
explicit. `matchedAccountCount` counts returned account headers. Neither is a
household-wide count when IDs are provided. Component/evidence pages, rollup
semantics, wire caps and credential checks are unchanged.

## Agent audit captures

`list_agent_market_data` uses a separate application/repository read. Target IDs,
oriented FX pairs, source observation-date labels and inclusive created-at bounds
are applied in SQLite before audit/quote expansion. Instrument and FX targets
remain intersecting categories when both are supplied, as in the existing
contract. Targeted instrument/FX selection seeks audit records using the existing
unique quote/target indexes. No schema or index migration is required.

A correction uses its new quote's identity and source date. A retraction uses its
immediate target's identity and source date, including targets outside the
created-at selection. A multi-step correction chain never inherits its root's
date. Closure dates remain source civil labels; they are not converted to UTC
calendar dates. Latest observations without a source date do not match an
observation-date filter.

Eligibility is checked against the complete household audit history in the same
read transaction. An invalidator outside the requested date, created-at interval,
operation or kind must still make its target inactive. Retracting the replacement
does not reactivate an earlier corrected quote. A retraction remains inactive.
`active` continues to mean eligible, independently of current valuation selection.

`totalRecordCount` counts the SQL candidate scope (targets, source dates and
created bounds), not the household audit population. The added
`countsScope=target_observation_date_created_bounds` declares this scope.
`matchedRecordCount` additionally applies operation, kind and active filters.
Those latter filters and deterministic sorting remain in the MCP capture layer.

Created-at timestamps are stored at millisecond precision. A sub-millisecond
inclusive lower bound rounds **up** to the next representable stored instant; an
upper bound rounds down. Offset timestamps normalize to UTC. This preserves
instant comparisons rather than silently including a preceding stored row.

The SQL preflight charges selected audit provenance plus quote payload (including
conversion text) before decoding; unrelated provenance does not consume the
capture budget. Selected corrupt/missing quotes fail through the existing quote
scanners. The capture still fails closed when its selected input or frozen output
exceeds the shared budget. These are input filters, not permission boundaries:
the household credential and frozen cursor/generation checks are unchanged.

## Limits and validation

The change does not introduce SQL pagination. Selected quote hydration still uses
per-record point queries, and a large selected target can exceed the existing
capture budget. Target quote-ID discovery may traverse that target's quote history;
date-only queries can scan household quote/audit indexes. This is not a claim of
constant SQLite VM visits, page reads, latency, or memory use. Broad account reads
and the legacy full Agent audit API retain their existing behavior.

No MCP input schema, installed skill, financial facts or provider preferences are
changed. See [the paired synthetic evidence](../qa/mcp-target-pushdown-2026-11-04.md)
for request wire sizes, SQL result rows/calls and unverified cases.
