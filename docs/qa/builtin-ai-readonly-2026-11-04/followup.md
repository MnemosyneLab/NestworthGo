# Period analysis, controlled navigation and recovery follow-up

Runtime/UI source: `753c193f1ef4d4a6b39986a498b2b7958ecd0bf3`.
This supersedes the initial stage's “period returns/model navigation unavailable”
limits. The [initial report](README.md) retains its own source-specific evidence.
No PR was merged. Main `e5a805a` (#223) is included; no #221 MCP handler/skill file
is changed. Environment and synthetic-data isolation remain as in the initial report.

## Enabled behavior

- Current overview and account reads retain their existing controlled handlers,
  budgets, exact amounts, capture/source times, missingness and paginated evidence.
- `compare_financial_attribution` now has a strictly read-only built-in host policy.
  It reconstructs daily valuations in memory with the shared historical replay and
  analysis kernels. SQLite `PRAGMA query_only=ON` acceptance forbids accidental
  maintenance/writes. External MCP retains its original maintenance behavior.
- Supported periods use household or one account, base currency and two available
  closed-date endpoints at most 365 days apart. The analysis interval is left +1
  through right in the history timezone. At most 20,000 snapshot items are rebuilt;
  shared capture/wire/ten-second-build and total run budgets remain. Oversized or
  incompatible periods report unavailable; they are not silently shortened or
  interpreted as zero. Net-worth movement, capital flows, residual and precision
  adjustments are not investment return. Current-day returns, multi-account sets,
  instrument/member filters and arbitrary analytics remain outside this vertical.
- The separate **Send and allow one page change** button explicitly authorizes one
  model-selected overview, account-list or disclosed real-account destination for
  that message. Ordinary Send exposes no navigation tools/grant. The model selects
  an opaque target, never a route/URL/coordinate/SQL/financial action.
- Navigation is offered only after a complete successful model turn. A backend
  two-minute page lease, host epoch/config, request, navigation generation, latest
  successful live conversation/run and actual object are validated. Consumption is
  once-only, including uncertain responses. Object validation occurs outside the
  runtime mutex, with all bindings rechecked afterward.
- New grants/turns, conversation creation/deletion, Stop, config/key changes, page
  publication/refresh, restore and shutdown revoke old authority. Deletion and new
  authorization during live-object validation also stop the pending action.
  Frontend page/publication generation and financial-review guards are checked
  before and after IPC, and application dispatch rejection is reported as blocked.
  Stop/new submissions/transport uncertainty withdraw pending frontend application.
  Grants/intents are memory-only; neither history nor restart recreates them.
- Local Status/List/Inspect/Messages use React Query `networkMode:always`. The
  browser offline hint pauses new provider submissions; it does not pause local
  history, pagination or Stop. No reconnect automatically resends a model request.
- A conversation-level in-process capability pool preserves frozen captures across
  live turns, without renewing their original five-minute TTL. Captures remain
  conversation/ledger/host-bound. Config/key changes, deletion, restore and shutdown
  revoke sessions. Nonblocking revoke and draining outside restore's gate avoid
  waiting for model/tool workers while holding application authority.
- Expired, evicted or restarted references return a fixed error to the model, with
  no financial facts or raw implementation error. The existing 8-round/16-call
  loop can recapture an initial read and restart relevant pages. Old/new capture
  IDs cannot be mixed; permission/disclosure does not widen and SDK retries remain
  off. Provider/framing/cancellation/invalid-result failures still fail closed.
- Checkpoint restore now refreshes only the first host-owned system policy after
  unmarshalling. It preserves assistant-native output groups/opaque reasoning.
  Existing successful conversations can use the newly installed period capability;
  old host text cannot suppress it or create navigation/financial authority.

## Verification

Final full frontend: **174 files, 1838 tests passed** on fixed source `753c193`,
365.30s. Full Go, vet, provider checks, targeted race, build, ESLint and TypeScript
also passed. Results are recorded in [checks.txt](evidence-followup/checks.txt).

- Root Go suite and vet passed on the fixed runtime/UI source, with purego for the
  Linux desktop dependency. Affected assistant/builtinai race checks passed.
  Provider `./check.sh` passed (tests, vet, build); purego desktop compilation and
  frontend production build passed. These are not native Mac launch evidence.
- Targeted assistant/frontend/translation acceptance passed 66 tests before the
  final full frontend run. Final full results are recorded in checks.txt.
- `TestReadOnlyFinancialAttributionDoesNotMaintainSnapshots` checks real SQLite
  query-only operation, unchanged snapshot count, exact full attribution parity
  with the existing maintained projection, income-vs-return and endpoint dates.
- `TestReadOnlyAttributionPriceReturnMissingAndBudget` checks an available nonzero
  return/rate, missing quote behavior and an unavailable oversized date range.
- Fake Responses/Eino integration verifies a real synthetic account's nonzero
  authoritative return reaches the model, with host evidence and no ledger writes.
- `TestLiveConversationPagingAndRestartRecapture` verifies live multi-turn frozen
  paging, unchanged original expiry, rejection after restart and bounded recapture
  with a new identity. No old snapshot is relabeled as current or revived.
- Runtime navigation tests cover ungranted typed-output forgery, explicit grant,
  once-only consume, page refresh/change during a run, expiry, config, restart,
  Stop, new grant/turn, deletion and deletion/new grant during object validation.
- Fake provider navigation exercises actual Configure/Publish/Grant/Start, shared
  account read, opaque inventory/target selection, successful offer and live typed
  consumption. A later ungranted wrapper cannot adopt the old native-history ref.
- The old-checkpoint test refreshes a deliberately obsolete host policy and verifies
  the actual next SDK request has current policy while encrypted reasoning and
  saved native output bytes remain unchanged. This is a synthetic compatibility
  gate, not a real-provider encrypted-signature acceptance claim.
- Frontend regressions cover local history and Stop while offline, normal Send
  without grants, explicit once-only dispatch, source-page change during IPC,
  pre/post financial-review guards, mobile denial and application rejection.
- [Chromium results](evidence-followup/results.json) exercise real settings/dock/Sheet
  components with synthetic IPC at 1440px English/light and 768px Chinese/dark:
  activation, read result, host evidence, offline Stop, explicit one-shot model
  navigation, protected ordinary navigation and no overflow/page errors. No real
  model or Wails host is used by this browser adapter.

## Mac acceptance and next stages

Use the isolated dev identity/path instructions in the initial report. Enter an
explicitly authorized test key locally in the UI, choose a model actually available
to that account and enable it. No real credential/ledger was requested or used here.

1. On synthetic data, request a current summary and a precise closed-date account
   or household period. Compare actual model explanations and evidence with the
   existing GUI/kernel; verify missing coverage is not presented as profit or zero.
2. Ask to open a disclosed account with normal Send and confirm no jump occurs.
   Use the separate authorization button and confirm one read-only jump. Change or
   refresh the page, create/delete a conversation, Stop, disable AI or enter a
   pending financial review during execution; stale intents must not navigate.
3. Reopen within capture TTL and continue pages; reopen after expiry/restart and
   confirm the model explains/refetches instead of presenting stale pages as valid.
   Disconnect the Mac externally and check that local history/pagination/Stop work.
4. Check actual provider/model/native encrypted-history compatibility, keyboard/
   VoiceOver, native focus/resize and isolated application backup/restore/power-loss.
   These remain unverified here. No financial writer or mobile model flow is added.

Explicit remaining scope: navigation to analytics date/filter state, arbitrary page
inspection/control, instrument/member/multiple-account period analysis, current-day
returns, mobile model principals and financial writes require further design/tests.

Memory/latency tradeoffs need native measurement: the pool can retain up to 20 live
conversation sessions with the existing independent read/context cache budgets
(16 MiB each per session); fixed TTL controls validity but is not a forensic-memory
purge guarantee. Read-only replay still takes the coordination permit and may
refuse large periods/households under its budgets. Do not raise budgets/timeouts
without measured correctness and resource evidence. Older binaries may reject new
navigation-run digest metadata; rollback must preserve the private sidecar and
follow the documented disabled-namespace reset procedure instead of editing hashes.
Database keys and full backups retain the accepted plaintext contract; no Keychain
or signing prerequisite is introduced. Richer native acceptance and broader product
scope remain separate stages, not a claim that arbitrary AI operation is complete.
