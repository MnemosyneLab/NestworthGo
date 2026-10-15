# Account reconciliation implementation plan and handoff

## Status and execution contract

**Approved plan; implementation is now progressing in the same PR #89.**
See [the implementation contract](account-reconciliation.md) for delivered
behavior and the PR for exact commits and validation. The original planning
handoff below is retained as the approved scope. The owner stopped the earlier
planning task and reassigned implementation. Exploratory,
uncommitted implementation edits were removed; this branch changes only this
plan. All proposed names and schemas below are design decisions for subsequent
implementation, not existing APIs or passing acceptance claims.

- Repository: `MnemosyneLab/NestworthGo`.
- Inspected base: `759b9882782a3081fa8cf6aac8fe2663d489d60d`, merged PR #88.
- Branch: `feat/account-reconciliation`.
- Draft PR: <https://github.com/MnemosyneLab/NestworthGo/pull/89>.
- First plan commit: `92c727a`; pushed and draft PR opened immediately afterward.
- Repository-local identity: `Walter Wang <wangweichen1994@gmail.com>`.
- Requested development environment/model: cloud, GPT-6 Astra, high reasoning,
  fast service where the execution platform supports that selection. This plan
  does not attest which model/service tier the platform assigned.

The successor should read `AGENTS.md`, `frontend/AGENTS.md`,
`docs/development/engineering-guide.md`, `local-workflow.md`, and
`frontend-controls.md`, then inspect the current main/branch delta. Continue this
branch and draft PR if assigned; if a fresh branch is requested, preserve the
base requirement and open its draft PR immediately after its first commit.
Commit and push each meaningful tested increment. Do not merge or release.
Preserve the current development/debug bundle identifier. Use synthetic isolated
fixtures only; no real user data, local Mac operations, Atlas/external agent
contact, or source/report/log/screenshot uploads to public services. Report
through the authorized GitHub PR and task conversation.

## Product contract

A household user starts a session, chooses active accounts (all selected by
default), checks each against bank/broker records, resolves differences or
explicitly leaves them pending, and saves a readable record. This is a local
ledger workflow: no banking connection, brokerage order or real-world transfer.
Do not add accounting periods, ledger locks, approval hierarchies or enterprise
closing machinery.

Required experience:

1. A dedicated Reconciliation page with Start, saved in-progress sessions,
   pause/resume, history and immutable completed detail. Account selection allows
   a subset; no unseen account is silently confirmed. Each account shows progress
   and its previous successful completion time. Account detail links to start,
   resume and relevant completed records.
2. Derive sections from contents, not mutually exclusive account classifications.
   An account can have cash, ordinary holdings and managed products together.
   Check each native cash currency separately. Balance/manual-value accounts
   retain their existing domain semantics rather than being forced into holdings.
3. Holdings show instrument identity/completeness, quantities, cost evidence and
   access to transactions. Include an explicit scope/completeness check so matching
   known positions cannot conceal a missing instrument or currency. Managed
   deposits/products show principal, terms/maturity and lifecycle state using the
   existing product model, including relevant closed/unsettled records.
4. Each point starts pending. The user explicitly confirms it, skips it with a
   note, or leaves an explained unresolved difference. Never infer confirmation
   from numeric equality. A confirmed point requires source evidence and a current
   reviewed revision; saving a correction does not itself confirm the point.
5. Keep external source/reference, source as-of date/time and actual checked/
   completion timestamps separate. Support date-only references without inventing
   a source time; use the ledger timezone when resolving a supplied local time.
6. A session may finish partially. Show confirmed, skipped and unresolved counts
   plus account outcomes; never describe a partial session as all passed. Only an
   account with complete scope and every required point explicitly confirmed at a
   still-current revision advances its last successful completion time.
7. Quote, daily NAV and FX delays are informative valuation differences. They do
   not invalidate cash/quantity attestations or block their confirmation. Pending
   settlement can remain an explicitly explained difference.
8. Completed records retain their original scope, business labels, checked
   values, evidence, outcomes, notes, timestamps and explicitly linked writes.
   Later relevant edits show a recheck indicator alongside the original record;
   they never overwrite it or erase the historical successful-completion date.

## Confirmed architecture and reuse points

| Layer | Existing entry points | Planned use |
| --- | --- | --- |
| Application | `internal/application/reconciliation.go` | `BuildReconciliationCommands` already resolves current balance/quantity/cost targets into audited domain commands. Reuse it for direct adjustments. |
| Cash calibration | `internal/application/cash_preview.go`, `internal/wailsapi/holding/cash_preview.go` | Existing `PreviewCashBalance`/`ConfirmCashBalance`, immutable reviewed amounts and mutation recovery. Extend linkage instead of writing balances directly. |
| Position/cost domain | `internal/domain/change.go`, `cost_basis.go` | Existing `PositionAdjustmentInput` and `PositionCostAdjustmentInput`; no new adjustment domain operation is presently necessary. |
| Transactions | `internal/application/change_service.go`, `change_batch.go`, `change_mutation.go`; `internal/wailsapi/history/{history,command}.go` | Existing Record, preview, immutable activities, batch atomicity, corrections/reversals and mutation receipts. |
| Products | `internal/domain/product_contract.go`, `product_operation.go`; application product services and Wails liquidity services | Preserve managed-position guards; edit terms/valuation and receipt/redemption/renewal through existing operations. |
| Concurrency | `internal/application/write_coordinator.go`, `exclusive.go` | Existing write/preview gates, ledger lock order, backup/restore exclusion and preview fencing. |
| Storage | `internal/infrastructure/sqlite/{database,schema_verify,readonly,upgrade_v15}.go`, `schema.sql` | Current schema 16; existing verified v15-to-v16 upgrade with a retained recovery copy. Extend deliberately. |
| MCP | `internal/mcpserver/{reconciliation_tools,ledger_tools,batch_tools,correction_tools}.go` | Existing `preview_reconciliation`/`commit_reconciliation` and `preview_change`/`commit_change`; retain their meanings. Add session tools separately. |
| GUI | `frontend/src/features/accounts/{AccountDetail,AccountActionSheets}.tsx`; `features/history/RecordChangeForm.tsx`; `features/liquidity/Product*` | Reuse calibration, transaction and product forms, including guarded unknown-outcome recovery and focus handling. |
| GUI wiring | `frontend/src/App.tsx`, shared layout/navigation, `queries/`, `lib/money.ts`, `lib/time.ts` | Add navigation/query keys, invalidation, localized labels and authoritative DTO rendering. |
| Maintained Skill | `skills/nestworth/SKILL.md`, `references/reconciliation-and-corrections.md` | Update the actual user Skill and runnable examples, not only tool descriptions. |

Existing direct holding correction semantics must survive unchanged: increasing
quantity requires a cost decision; `unitCost` prices added units, while `totalCost`
with instrument currency sets the remaining position's entire cost. Do not mix
these fields. A cost-only correction uses total cost; a quantity decrease must
not silently reset cost. Existing managed-product guards disallow ordinary
position adjustments on managed holdings. Existing adjustment plans cap targets
and expanded commands at 100; retain those bounds.

## Proposed persistence and application design

Keep the model small: session, selected account, check item, explicit write link
and mutation receipt. Prefer separate indexed rows for pageable evidence rather
than one ever-growing household JSON document. Exact decimal values remain
canonical strings; no binary floating-point persisted financial arithmetic.

- Session: household, status (`in_progress`, `paused`, `completed`), monotonically
  increasing revision, started/completed timestamps and optional note.
- Selected account: frozen business identity, source/reference and as-of precision,
  note, prior successful record reference, scope revision and completion outcome.
- Item: stable account/currency/holding/product identity; kind; initial and checked
  value/cost/terms/lifecycle evidence; pending/confirmed/skipped outcome; note,
  reference values, confirmation timestamp and reviewed financial revision.
- Write link: session/account/item, explicit operation/mutation identity, committed
  activity/product-operation IDs, operation family and available before/after
  evidence. Multi-leg transfers link only the selected affected accounts, without
  claiming unrelated legs were checked. No attribution by wall-clock overlap.
- Receipt: operation ID, complete request hash, result identity and revision so
  identical retries recover durably and changed payloads conflict.

Session scope is fixed after start. On a ledger correction or external edit,
refresh the affected account's working check scope: surface new/removed points,
retain prior attempts as evidence, and require affected points plus completeness
to be checked again. Do not erase unrelated accounts' progress. Separate mutable
working data from the immutable completed snapshot and transient recheck projection.
Allow separate sessions to coexist; their revision checks prevent one session's
old evidence from being silently reused after another changes the ledger.

Use account/item financial change revisions (or equivalent durable fact identities)
that include newly recorded backdated activities, corrections, reversals, cost
changes, scope/archive changes and product term/lifecycle revisions. Effective
transaction date alone is insufficient. Quotes/FX/provider refresh must not form
the revision for a quantity/cash check. The process-local global preview token is
suitable for short-lived mutation plans, not a persisted reconciliation watermark.
If existing persisted facts cannot supply a reliable revision, add an account
financial revision maintained atomically by the owning write paths; do not invent
parallel financial recomputation in the frontend.

Show changes since the last successful check using recorded revision/cursor order,
with recorded and effective dates both visible. First sessions have no prior
watermark. Page activity/term details separately instead of repeating full account
history. A session's linked changes are a distinct subset of these account changes.

Serialize capture/confirmation/finish with the existing application write gate.
Repository transactions must atomically validate expected session and reviewed
financial revisions, save evidence and persist receipts. Finish rechecks every
account; a changed confirmed point becomes unresolved/recheck-required rather
than passing. A session with never-reviewed items may finish only with explicit
partial intent and a closing explanation; those items stay pending, not skipped
or confirmed. Compute account last-success from completed confirmed snapshots (or
an atomic cache), never from start/pause time or source as-of time.

### Migration and recovery

Propose schema 17 with additive reconciliation tables/indexes only. Verify v16
before upgrading, retain a flushed verified read-only recovery copy, apply changes
in one transaction, then verify the resulting schema. Preserve v15 support through
its existing verified upgrade followed by the new verified step. Do not broaden
support to arbitrary older/unversioned/future/malformed files or silently repair
missing current tables. Review backup manifest version checks, read-only backup
inspection, restore installation, continuous-backup compatibility and JSON export
contracts alongside the schema change. A restored v16 copy upgrades on normal
open; inspection itself remains read-only. Recovery copies must never open as
writable live databases. Test failures before/within upgrade and retry safety.

## Correction routes and explicit linkage

Both GUI and MCP must offer these alternatives:

| Difference | Direct correction | Missing real ledger record |
| --- | --- | --- |
| Cash/balance | Existing target-balance calibration/reconciliation operation with reviewed delta and reason | Income, spending, transfer, FX, dividend, interest, fee or other supported transaction |
| Ordinary holding | Existing target-quantity/cost adjustment with explicit cost decision; no fabricated buy/sell | Existing buy/sell, position import or transfer as appropriate |
| Managed product | Existing product terms/valuation correction when supported | Existing receipt, redemption, renewal and other lifecycle operations |

Persist intended session linkage in the immutable preview/command envelope and
request hash. Validate selected account/item ownership before preview and commit;
reject closed sessions and cross-household links. Capture linkage and ledger
receipts atomically with the existing financial commit, so crashes cannot produce
an unlinked committed correction or falsely linked failed write. This likely
requires extending existing commit envelopes/repository transactions, not wrapping
a successful write with a separate best-effort session save. Retain receipt-first
retry behavior even if the session subsequently completes or the preview expires.
A precommit session/financial conflict requires fresh review; unknown outcomes
retry the same operation identity, never a new command.

GUI forms retain their existing submitting/unknown-outcome dismissal and navigation
guards. On success, close once, invalidate authoritative queries, return to the
same session/account/item and show the linked result. The user then checks and
confirms explicitly. Cancellation returns without changing the item's outcome.
No transient routing state may be the sole durable record of progress or linkage.

## GUI delivery

Add `features/reconciliation/`, query helpers and a thin Wails reconciliation
service registered in `cmd/nestworth/main.go`. Generate bindings; never hand-edit
or commit `frontend/bindings` or generated bundles.

The main page provides account selection, in-progress session cards and pageable
history. Session detail shows per-account counts and source as-of fields; account
review presents mixed check sections with scoped correction actions, recent
changes and clear Confirm / Leave pending / Skip controls. History detail renders
saved snapshots and linked before/after changes, with current recheck status
visually separate. Account detail includes the last-success timestamp and relevant
Start/Resume/View record links. If multiple sessions exist, let the user choose.

Use shared DatePicker/TimePicker, NativeSelect/FilterableSelect, Input, Button,
Sheet, AlertDialog, PageChrome/PageState and existing typography. Use canonical
money/quantity/quote formatters; preserve exact payloads and show precise audit
values where needed. GUI labels use business names, currencies, instruments and
dates, never UUIDs, revision hashes or opaque protocol tokens. Implement existing
supported locales, loading/empty/error/retry states, keyboard flow, narrow layouts
and focus return. Explain locally: changes update Nestworth, not the bank/broker.

## MCP and maintained Skill parity

Proposed new session surface (names to finalize against repository conventions):

- `list_reconciliation_sessions`: filter by status/account/date; stable cursor,
  deterministic sort and bounded summaries.
- `get_reconciliation_session`: header, scope/outcome counts and revision.
- Separate pageable account/item/change/link reads plus a single-item evidence
  read; avoid embedding every account and its history in the header.
- Session mutation preview/commit for start, pause/resume, source/reference/note
  updates, explicit per-item outcomes and finish. Require expected revisions and
  idempotent operation IDs. A preview displays exact intended outcomes/counts.
- Existing transaction, adjustment and product previews/commits gain optional
  validated session linkage. Do not duplicate financial mutation tools or change
  `commit_reconciliation` into a session-finish operation.

Read-only mode can inspect all permitted session evidence. Session writes and
ledger-correction previews/commits require `ledger_write`; `directory_write`
must not gain an attestation or ledger-writing loophole. Test registration,
permission dispatch, preview stores, request hashes and durable receipt recovery.
An MCP preview is not evidence that the user checked anything: explicit outcomes
must carry user-provided reference/attestation provenance. The Skill must ask for
missing evidence and never manufacture a source, number, note or confirmation.

Use existing bounded-read conventions and byte-budget tests. Start with page
sizes default 25/max 50 for summaries/items (tune against measured wire tests),
full identity and revisions on each item, stable cursors tied to session revision,
and explicit stale-cursor errors. Bound note/reference lengths on input. Page
large item evidence separately with counts/continuations; never silently drop
fields or truncate evidence to fit. Put business identity on page rows so agents
do not need full directory reads simply to label them.

Update `skills/nestworth/SKILL.md`, its reconciliation, recovery, paging, cash,
positions and product references as applicable, `docs/development/mcp.md`, and
`docs/user/nestworth-skill.md`. Add GUI user documentation and the feature's final
architecture contract. Update AGENTS only if this creates a recurring convention,
not as a substitute for product docs. Follow existing Skill version/package rules.
Add runnable examples and workflow tests for:

1. Start/read selected scope and revisions; capture real source/as-of evidence.
2. Preview/commit a linked direct correction **or** a missing transaction; inspect
   committed receipt and resulting values.
3. Read refreshed items, explicitly confirm the checked points, or record a
   skip/unresolved explanation. No automatic post-correction confirmation.
4. Preview/commit full or partial finish; read immutable completed evidence,
   account last-success dates and all outstanding counts.
5. On stale/conflicting data, reread and request renewed review; on unknown commit
   outcome, recover the original receipt before considering a new operation.

## Implementation increments and acceptance

The plan-only commits already exist. Subsequent development should be delivered
in coherent tested commits, each pushed to the draft PR:

| Increment | Touched layers | Required acceptance before calling the stage complete |
| --- | --- | --- |
| 1. Session/storage core | Domain session model; application ports/orchestration; SQLite schema, migration and repositories | Durable start/pause/resume/restart, fixed selected scope, mixed contents, partial finish, immutable snapshots, account last-success, migration/recovery tests |
| 2. Safe linked mutations | Existing change/calibration/product envelopes and atomic commit paths; revision tracking | Direct adjustment versus transaction semantics, cost preservation, cross-account/household rejection, atomic linkage/receipts, backdated edits and concurrency |
| 3. API parity | Thin Wails adapter/registration; MCP read/mutation tools and permission/budget contracts | Same application behavior, preview/commit recovery, explicit evidence, pagination/filter/sort and complete identity/revision/evidence |
| 4. GUI workflow | Reconciliation feature, navigation/account entries, queries/invalidation, existing form integration/locales | Start through finish/readback, pause/reload/resume, scope/section states, correction return path, keyboard/shared-control interaction and unknown-outcome guards |
| 5. Skill/docs/integration | Maintained Skill/examples, user/architecture docs, cross-layer regression | Executable end-to-end examples, both correction routes, partial outcomes, stale/retry guidance, supported cloud checks and explicit native limitations |

Minimum synthetic regression matrix:

- Select all/subset, reject archived/foreign/duplicate identities; an empty account
  still requires an explicit completeness check; no unseen account passes.
- One account containing USD/EUR cash, ordinary holdings and a managed product;
  each native balance stays separate and product ownership prevents generic trades.
- Explicit equality confirmation, skip-with-note, pending settlement, partially
  completed sessions and unchanged last-success for every nonpassing account.
- Pause/resume across service/database recreation; lost-response retry; same ID
  same payload returns original result and changed payload conflicts.
- Cash calibration versus income/expense/transfer; quantity adjustment versus
  buy/sell; added-quantity unit cost versus whole-position total cost; no fabricated
  trades, raw balance writes or bypassed product invariants.
- Newly recorded backdated activity, reversal, cost edit, product terms/lifecycle
  and scope changes after confirmation; reconfirmation required only where relevant.
  Quote/NAV/FX-only refresh does not invalidate quantity/cash evidence.
- Simultaneous confirm/correct/finish, stale sessions and preview expiry; session
  and ledger receipts/linkage atomic under injected failures and restart retries.
- Frozen historical labels, values, scope, source/as-of/completion times, outcomes
  and before/after detail after later edits; separate recheck indicator; unrelated
  writes in the same interval never appear as linked session corrections.
- Changed-since-prior pagination includes backdated new writes; deterministic
  paging without omissions/duplicates and explicit revision-bound cursor conflicts.
- `read_only` / `directory_write` / `ledger_write` permission matrix, oversized
  inputs, complete per-item evidence and measured MCP response budgets.
- Verified v15/v16 upgrades, current-schema reopen/verification, readonly backup
  inspection, restore upgrade, corrupted/future schema rejection and recovery
  failure rollback; no modification of source backups.
- GUI load/error/empty states, localization, shared date/select interactions,
  keyboard completion, form return and pending/unknown mutation guard behavior.

Run focused tests per increment, then supported repository gates:

```sh
GOCACHE=/tmp/nestworth-reconciliation-go go test ./...
GOCACHE=/tmp/nestworth-reconciliation-go go vet ./...
go build ./cmd/nestworth
gofmt -l cmd internal
git diff --check
cd frontend
pnpm run lint
pnpm run generate:bindings
pnpm run check:bindings
pnpm run typecheck
pnpm run test
pnpm run build
```

Use the documented CI race partitions/budgets for the full suite and focused race
checks during development. Run `python3 -m unittest discover -s tools/tests -v`
and applicable maintained Skill validation/package checks. All financial fixtures
must use temporary isolated database/settings paths; providers are fakes.

Record exact commands, results and source commit in the PR. If Linux native
headers/tooling or another gate is unavailable, report it as blocked, not passed.
Linux compilation, jsdom and cloud integration are not macOS native acceptance.
The successor's final handoff must identify commit links, test evidence, remaining
native/manual limits and the unmerged PR. No implementation tests were run or
claimed for this planning-only handoff.
