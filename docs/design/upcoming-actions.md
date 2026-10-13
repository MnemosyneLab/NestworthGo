# Upcoming actions

Owner: Nestworth maintainers. Status: Implemented on the feature branch;
release and native desktop acceptance pending. Scope: App-only, read-only MVP.
Baseline: `319ce0ba55244e769d454c5d0fe3d854aebe5438` (also current main when
implementation began). The independently reviewed Astra v2 source artifact has
SHA-256 `0915de6c56cb8d4f70be46817940fe15cf41a9e22dbcadb6386ac5cdde9f37c8`.

The App now also hosts a separate [investment review](investment-review.md)
panel and pending count. These opt-in, undated episodes do not alter the scheduled
action projection or its counts. Review preferences and acknowledgement use
separate explicit configuration mutations.

## Financial and date contract

`application.Service.UpcomingActions` captures the clock once, reads one
LiquiditySnapshot, freezes valuation clock/provider/TTL configuration for the
request, and evaluates normal routes. A pure Go projection classifies the
result; it performs no repository, provider, network, preview or write calls.
The existing Overview uses the same preparation helper and retains its output
contract. Separate Overview cards can represent different read instants.

The household History Origin timezone determines the civil date D; an unset
origin uses UTC. Invalid timezone names fail the read. The window is [D,D+30],
including 31 date labels. Calendar addition handles DST, leap days and year
boundaries. The UI never calculates financial phases from its own clock.

Contract maturity, original normal eligibility, estimated receipt and recorded
actual cash remain separate. Maturity takes precedence over an earlier unlock.
Only authoritative `dueUnconfirmed` creates a receipt-verification action;
past receipts remain visible beyond the window. Missing receipt timing leaves
a known maturity visible as blocked. A rolling estimate for requesting access
today cannot become an overdue receipt. Past explicit unlocks remain folded
under eligible items. Fixed recorded receipt overrides remain fixed. Weekday
settlement skips weekends, not public holidays; T+0 retains the same day.

Ordinary sources keep their explicit unlock/eligibility action. A fixed receipt
override remains in Available Funds details; it is not presented in this
projection as an estimate for a request today or on the unlock date, and cannot
create a managed receipt-verification action.

Money comes from existing liquidity evaluation, as decimal strings and currency.
Known zero differs from null. Unknown price/fee does not hide a known date.
Missing base FX preserves a known native shortfall. There is no mixed-currency
sum or automatic financial action, quote refresh, completion state, schema
migration, backup-format change or new MCP tool.

## Reservations and completeness

Reservations belong to their exact source and currency, with no payment date.
Comparison uses normal-route availability at D+30 regardless of Available
Funds filters. Coverage is classified before interpreting monetary results:
covered, short, currency mismatch, excluded source, due-route suppression,
not available by the end date, unknown amount, or unresolved source. Only a
known positive native shortfall produces an amount issue. Missing routes do not
produce a virtual full-reservation shortfall. Unresolved reservations remain
individual actions; resolved same-source reservations share one shortfall row,
with paired labels and IDs sorted together.

Incomplete financial inputs make completeness partial. Known scope constraints
(excluded sources, due-route suppression and not-yet-available routes) are
supplementary notes, not missing-data errors. Excluded known schedules retain
a supplementary date note. An attached data issue does not also become a second
standalone data issue. No empty state promises that all finances are healthy.

## Identity and presentation

Product lifecycle identity is `ua1/product/{id}/normal`; phase changes keep it.
Ordinary sources, source/currency shortfalls and unresolved reservation IDs have
separate stable identities. A SHA-256 occurrence key encodes date arrangements,
not names, amounts, revisions or rolling estimates. All nested collections and
items are deterministically ordered. Sorting uses priority, issue subtype, next
relevant date and ID; currencies are never ranked by amount. Receipt checks
precede reservation issues, blocked dates and ordinary upcoming schedules.

Overview shows at most three main actions and the full count. Upcoming actions
is reached from Overview and Available Funds, without another sidebar entry.
The full list filters receipt checks, reservation issues and schedules, renders
50 main rows initially and offers 50 more. Eligible items and supplementary
information are folded. Filters, focus and scroll survive session Back through
retained pages. Unknown wire types keep read-only text and have no default action.
English, simplified Chinese and traditional Chinese use the same resource keys.

The shared query is under the liquidity invalidation prefix. Active observers
refresh every 60 seconds and on focus/mount; hidden retained pages do not poll.
History Origin initialization also invalidates liquidity. Initial failures show
an error, background failures retain and label the last successful read/asOf.
Existing restore/reset restart barriers remain authoritative.

## Guarded navigation

Actions navigate to existing product detail, policy or reservation sheets.
A session request ID is consumed once after a fresh overview read; refetch does
not reopen a dismissed sheet, while another explicit click on the same target
can reopen it. Missing sources can still open the global reservation manager;
a reservation ID focuses the active or released card without editing/releasing.
Only same-page Available Funds object navigation omits a new trail entry;
other same-page focus changes preserve Back history. Failed navigation reads
stay pending for explicit Retry; leaving or superseding the request cancels it,
and late results cannot reopen its sheet.
Cross-page navigation hides old sheets and Back restores the prior trigger,
falling back to the page heading if the trigger disappeared.

Verify receipt opens the existing actual-amount → Preview → Confirm workflow.
Viewing, cancelling and rejected writes cannot complete an action. Interest
receipt leaves the contract open. Settlement removes it, undo restores it;
renewal creates a new identity and undo cancels the successor. These are reread
facts, not stored task status. Ordinary unmanaged legacy holdings never get
managed-product confirmation actions.

## Validation boundary

See [synthetic QA and remaining native gates](../testing/qa-upcoming-actions.md).
Automated Linux/headless checks do not establish macOS GUI, VoiceOver, zoom,
window resume or platform packaging behavior. No release version is assigned.
