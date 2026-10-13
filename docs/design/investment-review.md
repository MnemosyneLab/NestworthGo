# Lightweight investment review

Status: implemented on the feature branch; independent review and native Mac
acceptance pending. Baseline: main `5e8824a96b992f76ffc048541c5c99851ee71d53`.

## Scope and assumptions

The App shows a review panel on Upcoming actions and a count beside the upcoming
summary in Overview and Available Funds. Review episodes have no due dates and
never enter the scheduled liquidity action counts. UpcomingActions remains a
read-only projection. Reviews invite checking an investment and its NAV data;
they never execute a financial action or prescribe selling.

Each instrument is **opt-in**, initially unmonitored. This MVP covers active,
positive holdings in non-archived holdings accounts, for mutual funds and bank
investment instruments. It measures the instrument's unit NAV price change,
not an owner's realized return. Multiple accounts share one instrument episode.
Managed contracts with total-value observations, balance accounts, stocks and
other unsupported instruments are outside this MVP. Account portfolio/net-worth
inclusion does not change whether an owned fund may be enrolled.

Only existing, positive `agent_unit_nav_v1`, `close`, `date_label` quote facts
with matching currency and current binding revision are comparable. Manual,
legacy, realtime, cumulative/adjusted NAV, and unknown provider price bases are
not inferred. This deliberately accepts the existing explicitly typed NAV import
path rather than adding a provider or official seven-day-yield feed. Raw facts
and correction/withdrawal audit records are never changed by this feature.
Same-day revisions collapse to one actual dated observation; the active latest
Agent revision wins. Forward-filled daily valuations are never read as NAV facts.

## Calculation and coverage

The default selected window is 30 days; 7 days is optional. Anchor at the latest
actual daily observation, even when it is several days old. Set target to that
source date minus the selected calendar window. Pick the nearest actual baseline
on or before target. A baseline more than **7 calendar days before target** is
insufficient; no return is published. Thus actual spans are bounded to 30–37 or
7–14 days, and the UI labels both the selected window and actual span.

For supported windows:

    period percent = (latest unit NAV / baseline unit NAV - 1) × 100
    estimated annualized percent = period percent × 365 / actual calendar days

This is simple annualization, explicitly estimated. Go uses decimal division
(24 fractional places for the ratio, 12 for annualization), never binary floats.
The UI displays authoritative decimal strings. Calendar-day subtraction uses
source date labels without DST-hour effects. Age is the household civil date
minus the source date, with UTC used before History Origin exists. Future dated
observations cannot become the anchor.

The UI shows baseline, latest and target dates, actual span, selected window,
data age, distinct dated observations, and calendar dates with no observation.
Missing calendar dates include non-publication days and are disclosed rather
than forward filled. Sparse data or unsupported comparability shows unavailable
with a reason, not zero return. A supported old window may still show its estimate
with a stale label, but cannot issue a reminder.

Any nonzero split/distribution metadata or known ledger cash dividend in the
inclusive window makes it unsupported. The MVP does not add distributions to
NAV, so double counting cannot occur. Ambiguous holding quantity adjustments
also suppress windows. Ordinary buys, sells, cash flows and transfers cannot
change the NAV calculation; trade fees are excluded and explicitly disclosed.
Fund expenses already embedded in supplied unit NAV remain embedded. Unknown or
unrecorded corporate actions cannot be detected; checking source data is part of
the prompt. There is no new TWR/XIRR engine or holding-balance return inference.
Currency and quantity unit are immutable under existing instrument edit rules.

## Preferences and episodes

Preferences apply to enrolled instruments and remain user choices, not rules by
asset class. Defaults: low estimated annualized return **below 1%**, window price
loss **at or below -1%**, confirmation **7 days**, maximum data age **7 days**.
The master switch pauses reminders without erasing enrollment. Allowed ranges:
window 7/30; low threshold 0–10%; loss threshold -50% to below 0%; confirmation
2–30 days; maximum age 1–30 days. Percent inputs are exact canonical decimals
with up to eight fractional digits; equivalent numeric spellings normalize.

At least two distinct supported rolling-window endpoints must breach either
threshold across the confirmation period. Every intervening supported endpoint
must also breach; an observed healthy window resets the run. Gaps over seven
days or unsupported windows break new confirmation. An established episode
survives an unsupported interruption until a supported healthy window proves
recovery; unavailable/stale latest windows do not show a reminder. Escalation
from low yield to loss remains the same episode. Identity combines household,
instrument, canonical preference values and first breach date of the run.

One persistent App reminder row represents an episode. Acknowledgement suppresses
its pending count and button, survives refresh/relaunch, and never alters NAV or
ledger facts. A supported recovery followed by another sustained breach opens a
new episode. Explicit preference changes create a new review context; material
historical corrections/withdrawals may change reconstructed episode identity.
There are no toasts, OS notifications, timers when closed, or promised background
monitoring. Active foreground views evaluate on mount/focus and every 60 seconds;
existing valuation/import/activity invalidation re-evaluates the shared query.
Hidden retained views do not poll. No external automation or outreach is used.

Policy, enrollment and acknowledgement live in the existing household SQLite
`app_configuration` table under `investment_review/{householdID}`. Evaluation
uses one coherent read transaction and does not write, including on read-only
DBs. Configuration mutations use the normal write/exclusive gate and policy/
enrollment revision checks. They are serialized with evaluations. Backups retain
configuration; there is no database migration or generated-binding commit.
Invalid configuration/read/write failures remain explicit, and stale UI results
disable configuration and acknowledgement actions.

## Synthetic GUI acceptance

From the reviewed commit on Mac, create a **new** temporary child directory:

    go run ./cmd/investment-review-qa-seed -dir /tmp/nestworth-review-qa-UNIQUE

The seed refuses any existing directory and creates synthetic flat, loss,
missing-baseline, known-distribution and stale funds, all unmonitored. Its default
date is today's UTC date; `-date YYYY-MM-DD` makes offline replay deterministic.
Keep the acceptance launch near that date. Launch the exact reviewed build with
`NESTWORTH_DATABASE_PATH` pointing at that directory's `household.db` and
`NESTWORTH_SETTINGS_PATH` at that directory's `settings.json`; leave backup and
MCP disabled. Use no real household or provider data.

1. In Upcoming actions, verify all funds start unmonitored and the pending review
   count is zero. Enable the flat and loss funds; both produce one sustained
   episode with actual dates, span, age, missing dates and estimated wording.
2. Refresh repeatedly, acknowledge each, quit/relaunch, and verify no repeat
   prompt/count. NAV values/history remain unchanged. Check NAV opens the correct
   existing Market Data focus. No review receives a due date.
3. Enable missing, distribution and stale funds. Verify reason/stale labels and
   absence of a reminder, with no fabricated zero return. Toggle 30/7 days and
   threshold/age/master controls; validate persisted settings and pause behavior.
4. Check Overview/Available Funds summary and Upcoming actions at narrow/normal
   widths in English, Simplified and Traditional Chinese. Keyboard-test summary,
   preferences, enrollment, refresh, acknowledgement, NAV navigation and Back;
   check focus and VoiceOver labels. Close the app and verify no monitoring claim.

Automated tests additionally verify healthy recovery/new episodes, decimal
boundaries, actual elapsed days, revision collapse, withdrawal handling,
read-only SQLite, concurrent enrollment, exclusive-write rejection, stale-read
UI and invalidation/foreground query behavior. Native layout, accessibility,
packaging and live-provider acceptance are not established by those tests.
