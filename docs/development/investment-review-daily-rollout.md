# Daily investment review implementation checkpoint

The authoritative design is Library item
`libfile_da0cbd81b3c08191a4bdc7c9b6bee201`, version 2 (design 1.2),
114254 bytes, SHA256
`acddcc6e6eb82806c1d67156dd89d2e4e9b63af0a5e337df56a228ce1c34e56e`.
It was downloaded and read in full. This branch starts from main with PR97 and
commit `d23c01f4f9b59b64b7f83d4b246a8d6b37252f91`.

## Implemented on this branch

- Versioned, strict named RuleSpec with a bounded predicate tree, decimal
  thresholds, three-valued daily results, and single/consecutive/rolling
  trigger policies in a fixed IANA timezone. Missing civil days and missing
  inputs are unknown, not hits. Same-day evidence replaces the date's vote.
- Historical daily projection from dated position effects, cost, instrument
  type, quotes, corporate actions and FX. The current holding does not
  backfill a historical day. FX references and observed quotes retain their
  distinct date admission rules. Position epochs and T/U/F transitions
  preserve an acknowledged episode across U and resolve it on proven F.
- Schema 19 with a verified, flushed schema-18 recovery copy before upgrade;
  explicit 15–19 verification and full-backup compatibility. Valid v1
  preferences become one deterministic `legacy_v1` compatibility rule inside
  the upgrade transaction, with explicit true enrollment targets only. The
  original JSON and hash remain immutable; later v1 preference writes advance
  the rule's versions without rewriting that source. Malformed values remain
  quarantined for repair. Publication stages outside the write transaction, rechecks the
  authoritative source fingerprint, and commits days, states and episodes
  atomically with mutation receipts and same-day idempotence.
- Bounded read APIs for rules, current alert groups, one alert detail,
  positive-holding coverage and suppressions; explicit reconciliation,
  acknowledgement and suppression writes. Wails and MCP v2 expose these
  separately from the legacy v1 tools. The dedicated review page and rule
  editor offer grouped alerts, filters, local refresh, dated evidence, three
  trigger modes, acknowledgement and ignore management in en/zh-CN/zh-TW.
  The legacy panel remains available as a secondary section.

The general `.nestworth.json` export remains version 2 as specified. No
independent rule-template export/import format has been added.

## Lifecycle correction checkpoint (2026-10-16)

Independent review found three projection gaps. An edited condition no longer
closes an acknowledged episode solely because `conditionVersion` changed:
the complete new date-keyed policy interval must contain a proven F before
the old episode ends. U retains the episode and pauses actionability. Current
position epochs now replay authoritative effects through the actual capture
time, while daily votes still stop at the last closed date. A same-day clear
and re-buy therefore opens a distinct unacknowledged episode when the prior
closed rolling window is sufficient, without inventing a vote for today.
Finally, publication includes previously evaluated and still-open instruments
when a rule target shrinks, closes excluded episodes, and removes their stale
current evaluation state. Re-inclusion opens a fresh episode; repeated refresh
does not duplicate its resolution event.

Synthetic regression names: `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval`,
`TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes`,
`TestReviewSameDayClearClosesEpisodeBeforeNextDailyVote`,
`TestReviewAtomicSameDayTransferKeepsEpisodeAndAcknowledgement`, and
`TestReviewTargetRemovalClosesOldEpisodesAndReentryOpensFreshOne`.

The report now includes a same-transaction, publication-wide group summary
and filtered group count on every page. A separate read-only endpoint pages
persisted closed-day T/F/U evidence for one episode, binds its cursor to the
published control and episode revision, and exposes it through Wails, MCP and
the dedicated review page. Synthetic tests cover page boundaries, unchanged
read state and cursor expiration after acknowledgement. The evidence range
starts at the trigger window's first date, so a consecutive or rolling rule
retains the earlier votes needed to explain why it fired. Tests page through
consecutive, rolling, and same-day clear/rebuy episodes.

## Work still required for the design's release gate

The migrated compatibility rule now has an explicit conversion sheet. A pure
preview proposes separate low-yield and loss daily rules, states the changed
calendar-span semantics, and estimates new alerts and unknown outcomes. The
user chooses the daily trigger and may explicitly expand from old enrolled
investments to all current holdings. Save checks source and handling revisions,
then atomically archives the v1 rule and creates both new rules with a mutation
receipt. The old acknowledgements remain in history; original absolute snooze
dates become scoped suppressions. A retry uses the same IDs and receipt.
The conversion preview also carries a temporal token over the rule day,
history-origin day, and all-rule temporary-ignore days. Save rejects a token
that expired across any of those civil-midnight boundaries, even when no
financial row or revision changed; the user must preview again.

The dedicated page now offers a rule-scoped historical episode list and a
paged event timeline for each episode, including resolved and archived-rule
history. Acknowledgement and same-day evidence correction append meaningful
events; unchanged refreshes do not. Wails and MCP expose the same read-only
history APIs. Rule and suppression management page beyond the first 100.
The editor preserves nested
predicates while editing an existing rule and exposes their bounded all/any
JSON as an advanced field. Its ordinary type condition supports multi-select;
a visual builder for nested groups remains outstanding.

Reconciliation currently replays every historical date for each rule and
instrument. The new report summary also scans all current alert groups in
bounded SQL batches on each page read; its consistency is tested, but the
target first-page p95 budget has not been demonstrated. Dirty-range bounded
replay, checkpointing, long-gap progress and performance budget evidence are
outstanding. A complete independent rule
template import/export remains optional in design stage 5. The native macOS
Wails window, VoiceOver, Chinese IME, narrow/high-zoom layout, restart and
backup restore acceptance have not been exercised here. These are material
remaining gates; this checkpoint is for draft review, not merge or release.

## Parallel frontend ownership

The overall interaction task owns App routing, global navigation, shared
instrument detail and public filter context. This branch owns only the
dedicated `InvestmentReviewPage`, `ReviewRuleEditor`, their query hooks and
the backward-compatible optional disabled-item API in `ui/action-menu.tsx`.
The review page emits an `instrument-detail` target with `instrumentId` and
`section` (`holdings` or `performance`); the overall interaction task must
wire that target and retain the investment-review tab on return. No shared
App or navigation file is modified here.

All development tests use synthetic or isolated inputs. Application identity
remains `com.nestworth.dev` / `Nestworth`. No real ledger has been opened.

## Latest automated verification

Serial server-profile `go test -short -p 1 ./...` and `go vet ./...` passed
across all packages. The conversion, evidence and same-day rebuy regressions
also passed under the race detector. Generated Wails bindings were refreshed
with the pinned generator in server mode. Frontend TypeScript, focused ESLint
and conversion/editor tests passed. The previous draft head's remote CI
completed successfully; this new head requires its own CI result. Linux
server-mode compilation does not establish native macOS packaging.

The first remote CI run of the older draft head stopped at the standalone
user-skill check after Go tests passed: `skills/nestworth/SKILL.md` had 905
whitespace-separated words against its 900-word entrypoint budget. This
branch shortened the entrypoint without removing its MCP guidance, and
`python3 tools/check-nestworth-skill.py` now passes with all 76 JSON examples
linked to available tools. The companion installer test suite is also run
locally; remote CI must confirm the exact pushed head.
