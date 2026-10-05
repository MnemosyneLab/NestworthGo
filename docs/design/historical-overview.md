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
