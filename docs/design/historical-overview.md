# Historical overview

Implemented as an independent, read-only page. Desktop visual acceptance is
pending; automated component and application tests cover the contracts below.

## Meaning and scope

A selected date reconstructs the household's retained economic state using
facts available **now**. Later corrections and backfilled historical prices can
change that reconstruction. This is not a record of what the application knew
or displayed on that date.

Dates range from the history origin through the last closed household day. The
default is the previous month end, clamped to the origin. Account balances and
quantities use the last instant belonging to that civil date, including days
with a skipped or repeated midnight. A completely skipped civil date is
rejected; transaction wall-time validation is unchanged. Prices and FX use the
existing market-date daily-summary resolver: a finalized close with the selected
market date can occur after the household cutoff. This is not a simultaneous
midnight mark across markets. Before-origin and intraday views are unavailable.

Names and classifications use current metadata; those fields are not versioned.
Retained archive and net-worth-inclusion observations are replayed. Rows without
existence evidence on one side are absent, not zero. Deleted or never-retained
entities cannot be reconstructed from this page; supported archive operations
preserve identity and are shown explicitly. No member attribution or ownership
comparison is offered.

## Interaction

- Enter through Overview or the independent Insights navigation destination.
- Pick a closed date, step by day, or return to the last month end.
- Optionally compare with another closed date or a captured current state.
- Expand accounts into balance, cash-currency, and holding rows; inspect debt
  balances, native and base amounts, quantities, source dates and missing inputs
  in an independent read-only detail sheet.
- Filter to changed balances, quantities, inclusion/status or availability.
  A repeated manual confirmation or quote revision with identical values does
  not alone count as a balance change.
- Refresh explicitly to capture both sides again. A current comparison is fixed
  to the read, not an automatically updating quote panel or today's close.
- Exit to the source page, or Overview when entering through navigation.

Current workspace pages are unmounted while this view is active, including
pages previously retained for navigation. Historical detail never mounts a
current account or holding editor. Pending responses are keyed to the selected
dates; prior rows and sheets are hidden while a new selection loads. The sheet
supports Escape, and expansion is keyboard accessible. Labels are localized in
English, Simplified Chinese and Traditional Chinese.

## Financial contract

The application service returns exact decimal strings; the frontend performs
presentation formatting only. Assets and positive liabilities are separate;
net worth is assets minus liabilities. Each eligible leaf component contributes
once. Archived and excluded rows remain inspectable but do not enter totals.
Consequently internal transfers and principal repayment do not create household
wealth; their account-level movements are still visible.

Unknown is distinct from zero, cleared holdings, absent entities and archives.
An incomplete eligible component makes the relevant headline total and net
worth unknown. Available estimates remain visible as explicitly incomplete
known subtotals. Historical coverage gaps remain incomplete even when an older
usable quote exists. No current quote or invented FX substitutes for missing
historical evidence. Manual balances are carried forward from recorded economic
facts and show their source date; that is not a fresh market appraisal. For an
origin balance, the source date is the starting point, not a claimed original
prehistory appraisal date.

Differences are right minus left. Base differences require complete amounts on
both sides. Native differences require the same currency and known native
amounts; they can remain available when FX is missing. No delta treats absence
as a zero balance. A balance difference is not a return or contribution
attribution. Fixed-FX comparisons and new return decomposition are outside scope.

Allocation shows included valued assets by current classification and native
currency, measured in base currency. Debt is separate. Shares use the known
asset subtotal and are explicitly partial when estimates are incomplete.

## Read path and boundaries

`application.Service.HistoricalOverview(ctx, date, compareTo)` obtains one
`LoadHistoricalSnapshotBatch` transaction for both sides. A historical side
reuses `HistoricalReplay` and `ValuationService`; a current side values that
batch's current portfolio. The FX provider and quote TTL are captured once.
Both sides share the input generation, capture time and resolver policy.
Copies of account/holding slices allow archived values to be inspected without
mutating replay inputs or changing inclusion in totals. The internal valuation
result retains each component's original missing-input list before account-level
deduplication. A missing FX rate on an excluded archived position cannot
contaminate an included zero holding of the same currency; real nonzero
exposures still require FX.

`wailsapi/analytics.Service.HistoricalOverview` exposes this plain presentation
model through generated Wails bindings. No schema change, snapshot ensuring,
derived-data rebuild, provider request or persistence occurs in this read path.
It deliberately does not call the existing trend read paths that may
materialize daily snapshots. The frontend query is session-scoped, has no
automatic refetch on invalidation/focus/reconnect, and is discarded on unmount.

Existing Return Analysis and Asset Changes are separate destinations. No links
transfer the historical page's context into those views because their inclusion
and endpoint semantics differ. Chart-point entry, metadata versioning,
contribution/market/FX attribution, exports and global historical mode remain
outside this MVP.

## Validation

Tests use disposable SQLite databases or mocked IPC. Coverage includes SQLite
`query_only`, one input batch despite a concurrent write, current capture and
refresh, corrected economic dates, manual source dates, historical backfill,
quote/FX gaps, market closes after the household cutoff, incomplete coverage,
archive/clearing/absence/zero distinctions, exact sub-cent arithmetic, internal
transfers and debt principal, DST and invalid dates. UI tests cover date races,
comparison changes, frozen current state, read-only sheet lifecycle, keyboard
interaction, loading/error/empty states, navigation exit and localization.

No production migration or data mutation is needed to evaluate this feature.
Native desktop visual checks must use a separately authorized synthetic profile.
