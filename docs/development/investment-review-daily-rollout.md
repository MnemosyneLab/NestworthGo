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
  explicit 15–19 verification and full-backup compatibility. Raw legacy review
  state and its hash are preserved, with malformed values quarantined for
  repair. Publication stages outside the write transaction, rechecks the
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

## Work still required for the design's release gate

The current schema preserves legacy configuration losslessly, but does not
create a transactional `legacy_v1` named compatibility rule or offer a
conversion preview. The old evaluator and its old settings remain isolated.
The page has no publication-wide summary/counts consistent with pagination,
no paged daily-evidence drilldown or historical episode view, and rule and
suppression management currently load only the first 100 records. The editor
does not expose all supported nested predicate and multi-type choices.

Reconciliation currently replays every historical date for each rule and
instrument. Dirty-range bounded replay, checkpointing, long-gap progress and
performance budget evidence are outstanding. A complete independent rule
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
