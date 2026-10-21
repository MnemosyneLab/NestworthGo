# Wealth history

Implemented as an independent, read-only page. Desktop visual acceptance is
pending; automated component and application tests cover the contracts below.

## Historical panorama P0

The first panorama UI milestone organizes net worth, assets and liabilities by
metric, with explicit A, B and backend supplied B − A values. Single-date asset
composition uses the existing ECharts wrapper: a donut for eligible nonnegative
values, signed bars for negative or invalid composition, and an honest zero or
table fallback. Two-date asset categories share one order and axis; a missing
category on one side stays missing. Charts only convert exact decimal strings to
`Number` for geometry. Backend amounts, shares and row `baseChange` remain the
sole financial values shown in details and source tables.

The comparison overview ranks up to ten comparable changed account balances by
exact absolute delta, separates asset and debt accounts, and offers expansion.
Incomparable and scope/status changes remain visible separately. The full
account table defaults to all accounts, with an explicit changes-only toggle.
Category buttons filter account leaves without changing household totals. Detail
still uses the same captured bundle and existing read-only Sheet. Small windows
get account cards plus a full table option; loading uses fixed-height skeletons
and never labels stale totals with a new selection.

This milestone adds no new API, schema, provider, trend or attribution path.
Category amount/percentage-point deltas and richer price/FX evidence are now
provided by the P1 projection below. Bounded trends now use the separate P2a
contract below; attribution still requires a new backend contract. The P0 DOM,
financial and server-tag regressions are automated; native wide/narrow/zoom
screenshots and first-open Sheet visual acceptance remain pending on a GTK4 and
WebKitGTK capable host with a synthetic profile.

## Historical panorama P1

`HistoricalOverviewResult.categories` is a same-batch comparison projection,
separate from the existing asset-only `byClass`/`byCurrency` arrays. Stable
`role` (asset or liability), `dimension` and category `key` identify each row.
Only component leaves are aggregated; account parent balances are never added.
Classification uses current metadata. Each side reports:

- `status`: complete, partial, excluded or absent;
- nullable complete `amount`, separate `knownAmount`, included leaf count and
  missing leaf keys. `knownAmount` remains null when no included member has a
  parsed base amount; a retained actual zero is still a known amount;
- nullable percent-string `share` and `shareBasis`: total, known_subtotal or
  unavailable. Liability shares have their own liability denominator.

`amountChange` is exact decimal B minus A and is unavailable for partial,
absent, excluded, changed existence/inclusion/category membership or different base currency.
`amountReasons` states these limits. Both complete category amounts can remain
visible when their difference is incomparable. A missing category is never
zero-filled. Confirmed zero leaves remain complete zero values.

Shares use the existing backend allocation convention (percent rounded to two
places). A share is only available for nonnegative included balances and a
positive side denominator and at least one parsed amount in that category. A
partial side's share describes its known subtotal;
it never becomes a whole-state allocation claim. `shareChangePp` subtracts the
two backend share strings in Go and is only available when the monetary
comparison is eligible and both role denominators are complete. Incomplete
liabilities do not invalidate complete asset shares, and conversely. Percent
points are explicitly labelled, rather than presented as growth or return.
There is no frontend financial arithmetic, Other sum, debt sign inversion or
optional balance growth-rate projection.

Evidence presents the selected observation's exact unit price/rate, human source
and observation labels, effective date/time and quality. FX retains its recorded
pair and multiply/divide direction. Observation IDs, policy versions, protocol
keys and row identities remain internal, including missing-input identifiers.
Manual values retain their source date; unknown observation types use localized
unavailable copy. Zero quantity needs no price and same-currency values need no
FX. Ordinary native/base amounts, comparisons, trend tables and tooltips use the
shared two-decimal money policy; canonical values remain authoritative and
price/NAV/rate precision is preserved.

Rows carry explicit account/holding/instrument IDs for navigation. Category
controls drill into both sides' matching leaves, including retained excluded and
incomplete items. Missing entries show account/component business names. Trend
warnings show a count and at most three tooltip summaries, with a keyboard
accessible full list in each date row. Tooltip geometry wraps within the viewport.
Category filters leave household
totals unchanged. Returning to categories restores
the dimension, asset/debt choice, position and keyboard trigger. Account detail
provides child/parent navigation, including in narrow card layouts. With a
category filter active, the complete account context remains available in
explicit matching/other component groups. A scope note says the captured account
amounts are unchanged by the category filter. Existing component eligibility
and archive rules still apply. Navigating to another component labels it as
outside the active filter. Navigation can initialize A/B, view, filter, expansion and table preferences explicitly.

The explicit transaction-history action leaves the read-only panorama and uses
projected identities and an ordered date range. For current comparisons that
range ends at the last closed day, with a visible explanation that the capture
can also contain today's activity. A historical source retained on the App's
Back trail keeps its immutable query session, dates, filters, selected detail
and scroll/focus position. The default A is resolved once when the household
origin becomes available; crossing a household month boundary does not change
that selection or recapture a completed capture on Back. Explicitly cleared input remains invalid.
Its Sheet is closed while inactive. Returning to a settled capture does
not recapture. An unfinished read is cancelled while inactive and a return
starts a fresh capture of the same selection. A top-level exit removes the
retained session; an explicit new
panorama navigation creates a new session. No saved database snapshots,
providers, schema changes, trend/attribution series or automatic repairs are
added by P1.

Single-date, comparison and trend observers forward their abort signal to the
original Wails cancellable promise. Date/key changes, mode changes, invalid
selections, inactive pages and unmounts abandon pending work. Both comparison
sides still use one read-only SQLite transaction; activity hydration retains
256-activity chunks and one economic replay. Cancellation closes that
transaction and propagates through replay/valuation without returning a partial
capture. The cancelled Wails call completes quietly with an empty result because
its JavaScript promise has already rejected with `CancelError`; a second native
rejection would be reported as unhandled by beta.28. The application layer still
returns `context.Canceled`. It is an expected request outcome, while a live
validation, budget or infrastructure error keeps its normal error/retry handling. Account and category
filters remain local to a successful frozen household capture.

A live native error may already be in transit when the frontend abandons a
selection. beta.28 then creates a separate unhandled `CancelledRejectionError`
outside the original promise's catch. The helper records the exact abandoned
promise weakly and consumes only the SDK's late-native-error report associated
with that promise. Live errors, other runtime calls, programming errors and
cancellation-callback failures retain their normal reporting. The pinned-runtime
regression test covers both late resolve and reject orderings.

Automated acceptance and native checks are recorded in
[Historical panorama P1 acceptance](../testing/historical-panorama-p1.md) and
[query cancellation evidence](../testing/historical-query-cancellation.md).

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
- Pick a closed date, step by day, or jump to the last month end, last closed
  day or history origin. The month-end shortcut is unavailable when that date
  predates retained history. Clearing either date leaves it visibly invalid and
  prevents a read; it never silently selects a default or drops comparison.
  Date-label arithmetic is independent of the browser timezone.
- Optionally compare with another closed date or a captured current state.
  Show backend-computed changes in assets, liabilities and net worth, with
  unknown deltas kept separate from known zero. Positive signs indicate
  direction, not investment performance. An identical-date comparison is
  explained explicitly.
- Expand accounts into balance, cash-currency, and holding rows; inspect debt
  balances, native and base amounts, quantities, source dates and missing inputs
  in an independent read-only detail sheet. Holding/cash detail retains its
  parent account, date, household timezone and capture/cutoff timestamps.
  Source times are localized to that timezone with the exact UTC instant
  retained in the time element and tooltip.
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
supports Escape and returns focus to its opening row. Entering the view
focuses its date; exiting restores focus to the current Overview entry even
when the source page was unmounted. Expansion labels reflect expand/collapse
state. Summary metrics choose their column count from card width rather than
the viewport breakpoint; currency amounts never break inside a number. An
oversized amount remains fully available in a keyboard-focusable scroll area,
without truncation or ellipsis.

The comparison table has a keyboard-focusable horizontal scroll region and
explicit left/right controls when content overflows. Arrow keys scroll the
region without stealing keys from row buttons. Each step overlaps the previous
unobscured monetary area, subtracting the measured fixed-column width so no
content is skipped behind account names. Account names stay fixed while
the region is at least 360 px wide; smaller regions release the fixed column
so it cannot cover the monetary columns. Labels are localized in
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

Differences are right minus left. Household total deltas and row deltas are
computed in Go. Base differences require complete amounts on
both sides. Native differences require the same currency and known native
amounts; they can remain available when FX is missing. No delta treats absence
as a zero balance; such rows say “Not comparable.” A balance difference is not
a return or contribution attribution. Fixed-FX comparisons and new return
decomposition are outside scope.

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

## Pending native visual acceptance

Automated DOM interaction tests are not visual acceptance. The first isolated
native pass at `24b7fb7` confirmed the main financial/interaction paths but
found an English narrow-window summary splitting the final decimal digit.
The isolated retest at `f14f42c` passed amount layout, continuous horizontal
access and the fixed-name fallback. At nominal 200% native zoom (Actual Size
then 20 effective Zoom In actions, without a visible percentage readback),
opening a detail left its panel and Close beyond the visible right edge.
Sheets now follow VisualViewport size and offsets while preserving zoom,
scrolling and the dialog's focus/dismissal behavior. The `ffb4072` native retest
confirmed visible panel content and viewport tracking, but the separate
absolute-positioned Close remained invisible even after scrolling to the top.
The sticky title region remained visible. Close now belongs to that same
non-shrinking header in normal flow, with a visible text label and icon, instead
of the scrolling popup's separate positioned layer. Native evidence does not
expose the old button's paint bounds;
the precise WebView clipping/compositing mechanism is still unverified. This
structural correction passed visible Close clicking in the `8bd60b5` native run,
but first opening at zoom left the title and body clipped beyond the left edge.
An isolated instrumented copy of that exact commit reproduced the failure and
measured its cause: with a 640 CSS-pixel visual viewport at scale 2, focusing the
entering 448px sheet scrolled its `overflow: hidden` outer frame to 448px. That
offset persisted after the slide-in translate ended, leaving the panel at
[-256, 192] rather than [192, 640]. The frame itself stayed at x=0 and the visual
viewport offset stayed zero. Native panning then triggered a viewport update
and the frame's scroll offset returned to zero. Ordinary form Close also worked
when observed after settling; no dismissal fix is inferred from earlier immediate
snapshots.

The outer frame now uses `overflow: clip`, which cannot become a programmatic
scroll container; the popup retains `overflow-y: auto` for its content. Viewport
coordinate handling, slide animation, focus policy and Close behavior are
unchanged. DOM tests guard this clipping contract at the measured zoomed and
Actual Size widths, but do not simulate WebKit's geometry or prove native fit.
A fresh native pass must confirm first opening without corrective panning,
body scrolling, visible Close, keyboard behavior and normal-form focus.
Previous native passes apply only to their tested commits.
The shared sheet's default initial focus skips its header Close action when a
usable content control exists; Close remains in keyboard tab order. Explicit
focus policies, form autofocus, touch opening and read-only fallback behavior
are retained. Regressions cover real directory creation, account settings and
instrument creation without test-supplied focus overrides.
Use an isolated synthetic profile and capture screenshots during the following
checks before marking native UI acceptance complete:

- At wide desktop, a narrow window (about 375 px content width), and 200% zoom,
  compare two dates with long account/instrument names, large amounts and
  English/Chinese labels. Check horizontal scrolling, sticky names, clipping,
  summary wrapping and sheet scrolling/close-button reachability.
- At native zoom, open detail before any corrective horizontal pan: the panel
  and Close must be visible immediately. Pan and resize while open, dismiss
  with Escape and Close, and confirm focus returns to the trigger. Restore
  Actual Size, check a regular account sheet, then quit the QA app normally.
- Tab from the date through shortcuts and comparison controls; open an account
  and cash/holding detail with Enter, close with Escape, and verify focus
  returns to its row. Exit and re-enter from both Overview and navigation.
- Clear either date, choose the same date on both sides, step quickly between
  dates, and use the origin/last-closed shortcuts. Check that stale rows never
  appear under a new date, invalid dates show no totals, and changes-only can
  recover from an empty result.
- Use manual values, missing prices/FX, an absent earlier account, archived
  holdings and offsetting movements. Inspect native/base amounts, unknown
  total changes, source timestamps and parent-account context in detail.
- Compare captured current state, refresh twice quickly, and verify both sides
  update together. Exercise a failed read and retry; old totals must not remain
  visible as a successful fresh read.

## Historical panorama P2a: bounded local civil-day series

Saved daily snapshots, ranged snapshot rebuilds, derived-data snapshot staging,
and wealth/portfolio trend iteration use the same UTC civil labels and tested
day-end resolver as panorama. Missing or repeated midnight is a valid date;
a completely skipped date has no saved snapshot and remains a trend gap.
Direct snapshot requests still reject nonexistent labels and current dates.
Ranges are checked before publication and contain at most 31 closed labels.

Resolver policy `market-date-daily-summary-v6` invalidates older derived
snapshot completeness on database open, marks coverage dirty from the origin,
and increments the input generation once. Snapshot hash version `v4` triggers
append-only replacement through the existing generation-guarded rebuild.
Repeated rebuilds and reopening are idempotent. Ledger activities and source
observations are preserved; the schema and public API are unchanged.

`HistoricalOverviewSeries(ctx, from, to)` is a separate read-only application
contract and Wails method. It returns an `overview` with full P1 endpoint rows
and `points` with summary data for every civil date in the inclusive range.
Endpoints must be closed, forward dates on or after the household history
origin. One through 31 labels are accepted; reverse/current/longer comparisons
remain available through P1, without implicitly requesting a series. A skipped
civil date inside a range remains an explicit unavailable point and breaks the
line; an unavailable endpoint is rejected. Cutoffs use the existing household
civil-day function and market-date resolver, including retained late close
observations. This is current retained evidence/current metadata reconstruction,
not an as-known historical archive.

The optional repository port performs preflight and hydration in **one read-only
transaction**. It first reads origin metadata and validates the authoritative
closed-date window/endpoints before budget scanning or financial hydration; no
second financial batch is read. Before financial decoding it scans at most 100,001 retained rows
and accepts at most 100,000 facts, 1,000 possible component leaves and 32 MiB of
serialized column payload. The cap conservatively counts the whole retained
profile, including archived facts, corrected activity effects and product
metadata. Unrelated retained facts can therefore reject a small selected range;
there is no hidden truncation or retry through the unbounded P1 loader. Counts
include preflight/hydration SQL but exclude transaction begin/commit. Fixed
schema identifiers are used; snapshots/items and app configuration are never
loaded. Activity hydration uses bounded 256-activity SQL parameter batches,
then replays money effects once over the complete ordered activity set.

One capture timestamp, input generation, FX provider preference and quote TTL
apply to every point and both endpoints. Instrument facts, FX pairs, coverage
and source activities are indexed once. Selected observations are resolved
once per instrument/pair per date. Middle points only value included component
leaves; full account/evidence rows and category comparisons are built for the
two endpoints. The public overview, AnalysisService, snapshot ensuring or
rebuilding and provider calls are not used. No schema or provider is added.
P1 and the account-compositions contract retain their existing implementations.

Each point exposes nullable exact decimal strings for assets/liabilities/net
worth, separate nullable known subtotals, independent role completeness/counts,
and stable missing leaf keys/reasons. A subtotal is null when all included
amounts are unknown; an empty role or observed zero is a known zero. Missing
liabilities do not erase a complete asset series. Parent rows are never added
to their component leaves. Unknown values are not substituted into the line.
Connections are evaluated independently for assets and liabilities; net worth
connects only when both sides remain comparable and both net-worth totals exist.
Scope, instrument type/unit, binding revision and native/base-currency changes
break only the affected role's connection. An available empty role has a valid
empty scope; entering or leaving it breaks that role's connection too. A missing
civil date resets preceding point availability without comparing across the gap.
Selected quote provenance (provider/Agent/manual source and key), supported
raw-close provider policy names and timestamp encodings do not change economic
units. Supported Yahoo, Tiingo and Agent raw closes can therefore connect across
sources. FX rates compare their currency pair independently of source, saved
source preference, timestamp encoding and stored pair direction; valuation still
uses the authoritative oriented rate. Price types, unsupported/adjusted price
bases, split evidence and conversion units/policies retain their distinct basis.
The broad scope/basis flags remain explanatory summaries; they no longer force
an unrelated asset or liability series to disconnect. Endpoint comparison,
valuation amounts, evidence and source-selection policies remain unchanged.
Quote identity, dates, ordinary values, revisions and freshness do not change
the connection signature. A known-zero exposure with no applied price/FX evidence
is neutral: comparison state retains each leaf's last actual economic basis
across contiguous known-zero points. The first actual basis after an initial
zero has no earlier actual basis to differ from. Scope removal, unknown evidence
on nonzero exposure and nonexistent-date gaps clear the relevant memory. This is
capture-local comparison state, never invented valuation evidence or amounts.
Endpoint totals and rows use the P1 conventions and retained provenance.
Series endpoint cards take nullable known subtotals from the same series points,
so an all-unknown subtotal is shown as unknown, while observed zero stays zero;
the P1 result shape and its legacy subtotal strings remain compatible. Decimal
parse failures during endpoint allocation accumulation are returned explicitly,
even though current valuation output guarantees valid exact strings.

The application imposes a 10-second deadline. Context cancellation is checked
through SQL, effect replay, historical replay, indexing, date iteration and
valuation entry/account loops. Budget/deadline errors return no partial bundle;
explicit caller cancellation remains cancellation. The serialized-input cap
is **not** a hard heap/RSS cap. See the measured memory and dense-history timing
in [P2a acceptance](../testing/historical-panorama-p2a.md).

The trend tab is enabled explicitly with a new whole-range capture. During
loading/failure no old financial data is labelled with the new range. Switching
metrics/tabs, selecting a point or inspecting the complete source table stays
local. Every chart point has a keyboard-accessible table control. Set-as-A/B
recaptures the whole new range; Open-day visibly leaves the series for a new P1
single-day capture. Ordinary date/mode controls also exit series mode. Obsolete
range queries cancel their Wails promise; stale completions cannot replace the
active capture. Focus/reconnect do not reread. History Back retains the bundle,
metric, selected point, tabs, filters, detail, focus and scroll position.

The existing shared ECharts wrapper renders unsmoothed isolated segments with
backend connectivity flags and explicit A/B/selection symbols. There is no
MarkLine registration assumption, interpolation, downsampling, frontend
financial arithmetic, share/subtotal replacement, contribution/return claim or
attribution widget. Exact strings remain in tooltips and the complete visible
source table. One-point ranges are labelled as one observation. Rendering uses
the existing light/dark theme and en/zh-CN/zh-TW strings. Native visual/keyboard
acceptance remains pending on the reviewed immutable head.

### Manual instrument valuation carry-forward

A manual instrument quote preference treats the latest eligible local price as
valid until replaced, including an Agent-supplied price accepted under that
preference. Age alone does not make this price stale, carried, provisional, or
missing historical coverage. Preserve the actual source and observation date;
no synthetic daily observations are created. A missing eligible price remains
missing, future-dated prices cannot satisfy earlier dates, and required FX
coverage is checked independently. Historical valuation and saved snapshot
presentation use the effective preference at the snapshot cutoff, rather than
the instrument's current setting. Provider and Agent preferences keep their
existing daily coverage requirements. Resolver policy v6 invalidates prior
derived completeness through the existing rebuild mechanism.

### Historical funding gaps

When cash replay is negative, omit the cash sleeve and retain a synthetic
missing-cash snapshot item. Account and household totals remain incomplete.
Only holdings acquired by cash removals within that unresolved deficit receive
funding-gap annotations; older unrelated holdings retain their known returns.
The diagnostic activity ID identifies the first negative balance, while replay
tracks every affected acquisition until cash recovers. Analysis still checks
both daily endpoints, including the resolution day's opening gap.

Gap-bearing snapshots use a versioned funding-attribution hash prefix. Existing
coverage checks and health repair rebuild older gap snapshots append-only from
local facts; requested-range reads do not backfill unrelated history. Ledger
facts, missing-price reasons, overflow handling and exact amounts are unchanged.

### Closed-day valuation coverage

Stocks, ETFs, mutual funds and ordinary bank investment products default to no
required price/NAV publication on Saturday and Sunday, independently of their
quote source. Crypto keeps daily requirements. A valid previous observation
covers a weekend or explicitly confirmed closed-day interval, without stale,
awaiting-update or provisional display quality solely because of its age.
Actual intervening trading-day gaps, incompatible quotes and missing opening
prices remain missing; required FX remains independently validated (with normal
weekend reference carry). The original quote date/source and exact price remain
unchanged. A real publication on a weekend remains eligible.

Agent imports accept per-instrument `kind: "closed"` facts for exchange holidays
or explicit no-NAV-publication dates. Supply `instrumentId`, `date`, optional
IANA `timezone`, `sourceTitle` and optional `sourceUrl`; omit currency, price,
quote time and corporate-action fields. Facts are dated, never market-wide, and
use the normal UUID/idempotent atomic import and source-audit path. Returned
`quoteIds` also identify these non-price facts for correction/retraction; the
Agent audit reports `kind: "closed"`, date, timezone and provenance without a
price. Corrections must retain the instrument and fact kind. An eligible actual
price on the same day takes precedence; a closure never creates an observation,
price-series point or opening price. Withdrawing a closure reopens any otherwise
unsupported date and invalidates derived snapshots. Revisions are append-only.
Schema 22 adds `agent_instrument_closures` with the same verified, read-only
pre-upgrade recovery policy as earlier upgrades; its financial source triggers
participate in revision tracking and publication guards. Resolver policy v6
invalidates derived completeness from the history origin on upgrade.
