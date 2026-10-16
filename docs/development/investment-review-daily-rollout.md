# Daily investment review implementation checkpoint

The authoritative design is Library item
`libfile_da0cbd81b3c08191a4bdc7c9b6bee201`, version 2 (design 1.2),
114254 bytes, SHA256
`acddcc6e6eb82806c1d67156dd89d2e4e9b63af0a5e337df56a228ce1c34e56e`.
The current executor's Library search/materialize flow returned that exact
file ID and version. It was materialized at
`/workspace/NestworthGo-investment-review-design-2026-10-16.md`; byte count and
SHA256 match the values above. The repository had no saved exact-name copy.
The original rollout checkpoint was based on main with PR97 at commit
`d23c01f4f9b59b64b7f83d4b246a8d6b37252f91`. This continuation verified the
existing PR98 worktree and retains its earlier implementation.

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

## Conversion flow, history UI, nested editor, and incremental replay

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
The rule editor now recursively edits nested all/any groups, instrument
conditions, and multi-select type conditions without exposing raw JSON. It
checks the 20-leaf/two-group limit and keeps Save disabled when a stored
predicate contains malformed or `null` nodes. The historical event renderer
also treats a JSON `null` payload as absent optional counts rather than
crashing.

Published daily inputs and results now act as reusable date-slot checkpoints.
Capture reads them only when the last published historical source fingerprint
exactly matches the current consistent source snapshot and the current rule,
condition version, timezone version, instrument, and date match. A missing
slot is computed; a changed source fingerprint disables reuse and causes a
full safe recomputation. Per-publication caches share same-day portfolio
snapshots and compatible daily inputs across rules, index quantity activities
by affected instrument, and calculate all policy windows in one O(1)-per-day
rolling pass. Tests assert zero snapshot/input projections on unchanged
source, exact invalidation on source change, and unchanged atomic-transfer
epoch evidence.

This is incremental reuse for unchanged history and new date slots. Quote
selection is indexed once per instrument for a full replay, so same-date
deduplication and sorting are not repeated for every daily vote. The opt-in
SQLite measurements use the design-sized 100-holding/20-rule/100k-quote/
50k-activity synthetic fixture. A pre-query-rewrite 30-day clock-only run measured
7.290s warm p95 and 25.696ms first-alert-page p95; a pre-query-rewrite 365-day run
measured 17.262s warm p95 and 47.570ms page p95. Both page results are below
150ms, while cold and warm budgets fail. A daily refresh with 100 new quotes
and 4 FX references, and an in-place historical quote correction, each changed
the source token and safely forced a full 62,000-day replay. The exact-decimal
values inside quote/rate types are now included in that token; regression tests
cover in-place corrections. Full fixture counts, SQL traces, phase timings and
the proposed schema-level performance work are in
`docs/testing/investment-review-daily-acceptance-2026-10-16.md`.
Native macOS Wails, VoiceOver, Chinese IME, high-zoom layout, restart and
backup-restore acceptance have not been exercised in this Linux environment.

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

Local verification after the quote-series and activity-hydration changes:
Go 1.27.2 `go test -short -p 1 ./...`, `go vet -p 1 ./...`, and server-profile
`go build ./cmd/nestworth` passed. Direct Vitest execution passed all 137 files
and 1,457 tests. TypeScript no-emit checking and the production Vite build
passed, ESLint passed, and the generated Wails bindings check passed against
v3.0.0-beta.28. The synthetic nested-editor interaction and invalid-value tests
passed in Vitest; a Chromium screenshot of the real editor component is stored
at `docs/qa/investment-review-daily-2026-10-16/nested-condition-editor.chromium.png`.
This Linux server-profile verification does not establish native
macOS Wails packaging or accessibility acceptance.

The older draft head CI run 38900264828 completed successfully. The final
commit still needs its exact-SHA CI result. Native macOS Wails, VoiceOver,
Chinese IME, zoom/theme/layout, restart and backup-restore remain unrun.

The first remote CI run of the older draft head stopped at the standalone
user-skill check after Go tests passed: `skills/nestworth/SKILL.md` had 905
whitespace-separated words against its 900-word entrypoint budget. This
branch shortened the entrypoint without removing its MCP guidance, and
`python3 tools/check-nestworth-skill.py` now passes with all 76 JSON examples
linked to available tools. The companion installer test suite is also run
locally; remote CI must confirm the exact pushed head.
