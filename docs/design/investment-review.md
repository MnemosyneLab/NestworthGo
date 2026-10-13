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

Only existing, positive `agent_unit_nav_v1`, `close` quote facts with `date_label` or validated
`source_timestamp` evidence
with matching currency and current binding revision are comparable. Manual,
legacy, realtime, cumulative/adjusted NAV, and unknown provider price bases are
not inferred. This deliberately accepts the existing explicitly typed NAV import
path rather than adding a provider or official seven-day-yield feed. Raw facts
and correction/withdrawal audit records are never changed by this feature.
Same-day revisions collapse to one actual dated observation; the active latest
Agent revision wins. Forward-filled daily valuations are never read as NAV facts.

## GUI review and setup

Upcoming actions remains the review home. The Overview/Available Funds summary
always offers **Review investments and settings**, including with zero pending
reviews. Eligible holdings with no enrollment show an explanation and **Choose
investments**; the all-investment view exposes each instrument's explicit opt-in
checkbox. The current policy is visible before enrollment, and the master policy
editor remains under Review preferences.

The default list contains pending application-reported episodes, ordered by loss
before low yield, then display name and instrument ID. **All investments / setup**
keeps that pending group first and exposes other eligible holdings, including
acknowledged, ignored, stale and unavailable results. Separate badges show the
application's NAV condition and reminder handling. Acknowledgement suppresses the
action but does not imply healthy NAV or change episode identity or escalation.
No-eligible, none-enrolled, no-pending, insufficient-data and master-pause states
have explicit copy.

**View NAV** and **Reviewed** remain visible on actionable rows. Temporary-ignore,
resume and disable actions use guarded buttons inside Reminder controls; exact
source dates, actual span, observation counts, price return and the unit-NAV
formula/caveats remain in Calculation basis. Both disclosures use native details
and summary controls. Report-driven group changes retain logical keyboard focus
on the equivalent row control or its named heading; when the row leaves the
pending view, focus returns to the pending filter. A polite announcement names
the changed group. Focus is captured at the DOM commit, so a later refetch cannot
reclaim focus after the user moves elsewhere or the view becomes inactive.
Unsaved preferences survive unrelated report revisions.
This changes presentation only: calculations, eligibility, thresholds,
confirmation, acknowledgement, snooze precedence, persistence and MCP contracts
are unchanged.

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
source date labels without DST-hour effects. The existing import path validates
an optional actual timestamp against its supplied observation date/timezone;
that richer evidence remains eligible, including mixed timestamped/date-only
windows. The actual timestamp does not change the calendar-day denominator. Age is the household civil date
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
from low yield to loss remains the same episode. A candidate identity combines household,
instrument and first breach date of the run, independently of preferences. A
persisted acknowledgement whose date anchor still lies within that ongoing run
retains its original episode identity, even when earlier-history enrichment
moves the reconstructed first breach. A supported healthy window clears those
aliases, so a later breach run can produce a new episode. This resolution is
read-only and also honors acknowledgements already stored by the initial MVP.

One persistent App reminder row represents an episode. Acknowledgement suppresses
its pending count and button, survives refresh/relaunch, and never alters NAV or
ledger facts. A supported recovery followed by another sustained breach opens a
new episode. Preference edits do not clear an acknowledgement while its anchor
still belongs to the ongoing breach run under the selected criteria. Material
historical corrections/withdrawals or a supported healthy window can change the
reconstructed run. Initial-draft acknowledgement hashes remain readable; prior
canonical preference profiles are retained privately only for that compatibility.
There are no toasts, OS notifications, timers when closed, or promised background
monitoring. Active foreground views evaluate on mount/focus and every 60 seconds;
existing valuation/import/activity invalidation re-evaluates the shared query.
Hidden retained views do not poll. No external automation or outreach is used.

## Temporary and indefinite ignore

Reminder controls offers **Ignore for 7 days**, **Ignore for 30 days**,
**Resume now** while temporarily ignored, and **Disable reminders** while
enrolled. Thirty days means thirty calendar days, not a calendar month. Indefinite
ignore uses enrollment off; estimates and NAV navigation remain visible, and the
explicit enrollment checkbox re-enables reminders. Expiry never re-enables an
indefinitely ignored investment or overrides the master pause.

A temporary ignore preserves enrollment, estimates, preferences and episode
acknowledgements. From household civil date D, store the resume label D+7 or D+30.
The ignore is active exactly while the current household date is before that
label; it expires at that household day's start, including timezone/DST edges.
The UI shows the resume date and household timezone. At expiry, the next ordinary
foreground evaluation uses current actual evidence: only fresh, supported,
confirmed, unacknowledged criteria can create a pending action. No closed-app
notification or timer is introduced.

Refresh, quote backfill, episode reconstruction and preference edits never change
the stored resume date. A supported recovery during ignore suppresses the old
episode at expiry; a genuinely new sustained breach can later remind. Resume now
only removes the temporary ignore, so an existing acknowledgement, indefinite
opt-out or master pause still applies. Temporary ignore does not acknowledge the
episode. All ignore/enrollment/preferences edits use the same optimistic revision
and exclusive-write guards; stale UI reads disable the controls.

Policy, enrollment, temporary-ignore dates and acknowledgement live in the existing household SQLite
`app_configuration` table under `investment_review/{householdID}`. Evaluation
uses one coherent read transaction and does not write, including on read-only
DBs. Configuration mutations use the normal write/exclusive gate and policy/
enrollment revision checks. They are serialized with evaluations. Backups retain
configuration, including ignore dates and compatibility profiles; there is no
database migration or generated-binding commit.
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
   count is zero. Choose investments to open all holdings, then explicitly enable
   the flat and loss funds; both produce one sustained episode with actual
   dates, span, age, missing dates and estimated wording.
2. Refresh repeatedly, mark each Reviewed, quit/relaunch, and verify no repeat
   prompt/count. NAV values/history remain unchanged. View NAV opens the correct
   existing Market Data focus. No review receives a due date.
3. Ignore the flat fund for 7 and 30 days. Verify the explicit date/timezone,
   zero pending action while estimates remain visible, and persistence across
   restart, backfill and preference edits. Resume now must not acknowledge the
   episode. Disable reminders and re-enable explicitly; expiry must not turn
   indefinite ignore or the master pause back on.
4. Enable missing, distribution and stale funds. Verify reason/stale labels and
   absence of a reminder, with no fabricated zero return. Toggle 30/7 days and
   threshold/age/master controls; validate persisted settings and pause behavior.
5. Check Overview/Available Funds summary and Upcoming actions at narrow/normal
   widths in English, Simplified and Traditional Chinese. Keyboard-test summary,
   preferences, enrollment, refresh, Reviewed, NAV navigation and Back;
   check focus and VoiceOver labels. Close the app and verify no monitoring claim.

Automated tests additionally verify healthy recovery/new episodes, decimal
boundaries, actual elapsed days, revision collapse, withdrawal handling,
read-only SQLite, concurrent edits, snooze expiry/DST/backup, recovery during
ignore, legacy acknowledgement/backfill compatibility, exclusive-write rejection,
stale-read
UI and invalidation/foreground query behavior. Native layout, accessibility,
packaging and live-provider acceptance are not established by those tests.


## Deterministic regression coverage

Tests use synthetic SQLite households and injected clocks, with no live feeds:

| Test | Contract checked |
| --- | --- |
| `TestInvestmentReviewAcknowledgementSurvivesEarlierHistoryEnrichment` | Sep 1–Oct 9 flat NAV, acknowledge, Aug 31 backfill, restart/refresh retain identity; actual recovery/new breach renews it |
| `TestInvestmentReviewLegacyAcknowledgementSurvivesBackfillAndPreferences` | Initial draft's persisted acknowledgement remains readable without read-time writes, including after a preference edit |
| `TestInvestmentReviewDatedNAVTimestampVariants` | Date-only, timestamped, mixed and cross-UTC-date timezone NAV windows produce 0.3% / 3.65% over 30 civil days |
| `TestInvestmentReviewEpisodeAliasesStopAtSupportedRecovery` | Recovery excludes prior episode identities from the new breach run |
| `TestInvestmentReviewSnoozeExpiryAndFreshness` | Seven-day boundary, restart, enrolment/estimate preservation, unacknowledged expiry and stale suppression |
| `TestInvestmentReviewThirtyDaySnoozeAndBackupPersistence` | Exactly 30 civil days, backup/restore, before-boundary suppression and fresh-evidence expiry |
| `TestInvestmentReviewSnoozeUsesHouseholdCivilDateAcrossDST` | New York date differs from UTC; DST change does not move local expiry |
| `TestInvestmentReviewSnoozePreservesAcknowledgementBackfillAndPreferences` | Backfill, window/threshold changes, restart, expiry and manual resume preserve an existing acknowledgement |
| `TestInvestmentReviewSnoozeSurvivesPreferenceEditsWithoutAcknowledging` | Temporary ignore survives backfill/master/window edits; resume leaves the master pause and unacknowledged state intact |
| `TestInvestmentReviewRecoveryAndUnsupportedDataDuringSnooze` | Recovery and known distributions suppress expiry; a later confirmed breach can remind |
| `TestInvestmentReviewManualResumePermanentIgnoreAndConcurrentEdits` | Manual resume, indefinite disable/re-enable, revision conflict, exclusive-write rejection and invalid duration |
| `TestInvestmentReviewEpisodeAliasesStayInternal` / `TestInvestmentReviewSnoozeWireValidation` | Replay-only identities stay off the wire; invalid duration is wrapped in the stable error contract |

`InvestmentReviewPanel.test.tsx` covers all three locales, keyboard snooze/resume,
visible resume date and estimates, pending-count suppression, indefinite ignore/
re-enable, stale/pending controls and unsaved preferences across unrelated edits.
`investmentReview.test.tsx` covers exact guarded mutation payloads and invalidation,
as well as shared foreground-only evaluation.
