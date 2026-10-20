# Account reconciliation architecture

This document describes the implemented contract. Current release records own
acceptance status and remaining native gates. Reconciliation changes the local ledger,
never a financial institution or brokerage order.

## Authority and evidence

`application.Service` owns session state, capture, explicit outcomes and finish.
The Wails reconciliation adapter and MCP session tools call the same methods.
The working model has session, selected-account and per-item rows. Accounts are
fixed at start (one to 100 distinct active accounts; omission selects all).
Contents determine independent scope, native-currency cash, ordinary holding,
balance/manual-value and managed-product points. Every point starts pending,
including an empty account's completeness check. Quantities, money and cost are
canonical decimal strings. Ordinary cost evidence uses the existing cost replay;
product terms use the existing product model, including closed contracts. Cost
repository and integrity failures abort capture and preview/commit revalidation;
missing cost stays unavailable and prevents holding confirmation. Known zero
cost remains valid evidence.

Checks store the ledger values reviewed, financial revision, explicit outcome,
reference value, source reference, civil as-of date, optional local time and ledger
timezone, note and actual server checked timestamp. Date-only evidence never
receives an invented source time. Skipped and unresolved outcomes require a note;
confirmation requires actual source and date. Prior attempts remain on each point,
with a maximum of 100 attempts per session item. At that bound, finish partially
and start another session rather than drop evidence. Notes are at most 2,000
UTF-8 bytes, source references 512 and reference values 256. Shared inputs show
byte counts and field validation before preview; backend bounds identify the
specific note, source reference or reference value.

Start, pause, resume, checks and finish are preview/commit operations. The exact
plan, result and revisions are retained at the boundary; commit regenerates and
compares the result while holding the application write coordinator, then SQLite
validates session CAS and account financial revisions in the evidence/receipt
transaction. Same-plan recovery checks the durable receipt first, including after
expiry and completion. Changed payloads conflict. Uncommitted previews expire in
ten minutes. Only after durable receipt lookup proves there was no commit does
expiry return `stale_preview` with field `expiresAt`, allowing fresh review.
An unavailable desktop plan (`planId`) or unscoped stale response retains unknown
outcome recovery. New desktop previews register immutable private SQLite recovery material before
returning their business plan ID. Session IDs match their receipt identity;
adjustment/product plans retain their original canonical commands, token, hash
and expiry. Adapter/process loss therefore does not rebuild a financial command
from changed defaults. Receipt-only recovery is read-only and never executes an
unregistered identity. If a body exists, its hash/revision checks still apply.
MCP production plans and outer operation receipts also use private SQLite
configuration; files are a legacy/test fallback. Succeeded outer receipts replay
without plans. A missing body on a pending/unknown operation stays unknown;
older terminal `not_found` records may be inspected again for business receipts.
No polling or UI refresh automatically resubmits an unknown operation. Older
volatile desktop IDs cannot retroactively acquire a durable mapping.

Full success requires every point, including completeness, to be explicitly
confirmed at the current financial revision. Partial finish requires explicit
intent and a closing explanation; pending points remain pending. Account success
is computed from completed confirmed snapshots, ordered by actual completion time.
A partial session can contain a successful account and other unfinished accounts.
No unrelated selected or unselected account is implicitly confirmed. Prior
successful time is retained separately from a newly completed account success;
account history ordering compares actual instants, including fractional seconds.

## Financial revision and correction transactions

Schema-verified triggers maintain account financial revisions inside owning
financial transactions. Cash/value facts, holding quantity/scope, cost effects,
account archive/mode/currency, product contract and product-operation facts advance
the affected account. Provider prices, ordinary manual quotes, NAV and FX do not.
Pure managed-product value observations (`value_observation` with a `valued`
link) also leave the financial revision unchanged; principal, quantity, terms
and cash mutations retain their existing revision triggers. Derived value/cash
rows with `projection_kind='replay'` do not advance revisions or add changes;
authoritative baseline/event writes and activity facts retain their triggers.
Recorded revision order detects newly recorded backdated activity, rather than
using its effective date as a watermark. Separate change-log rows retain recorded
and effective timestamps. There is no prior watermark on a first session.

A changed working account refreshes its current items, retains earlier attempts,
surfaces removed points and requires renewed explicit review. Other accounts keep
their progress. All refreshed points, including removed points, use stable-ID
ordering so unchanged previews and commits compare the same evidence.
Removed points carry the current financial revision; their retained attempts
keep the historical revision originally checked. Completed
rows stay frozen; current revision differences are a
separate recheck projection. Historical successful timestamps remain available.
MCP session-only commits emit agent.reconciliation.changed and invalidate only
reconciliation reads; they do not use the broad agent.data.changed notification.
Descriptive instrument price/NAV performance remains independent of personal
ledger/reconciliation inputs, preserving merged PR90. No investment-review
rule-engine overhaul is part of this feature. The revision is deliberately
account-granular: a relevant edit requires that
account's points and scope to be checked again.

`reconciliationLink` is part of the immutable ledger/product command and request
hash. It names a session, selected account/item, session revision and financial
revision. The repository's shared transaction entry validates it before any
financial effect. Activity, product and guarded terms writers save exact linked
before/after evidence inside that same transaction; an unsupported linked write
fails closed. A newly surfaced working point is captured under the financial write gate and
persisted in that same transaction before linkage; evidence reads stay read-only.
The transaction advances session revision. Receipt replay happens
before session status/financial token checks, so a completed session cannot block
recovery of an already committed write. Unrelated writes during the same time
interval never gain a session link.

Direct balance/quantity/cost corrections reuse BuildReconciliationCommands and
existing atomic batch commits. Missing income/expense/buy/sell uses the existing
Record form and ledger tools. Added-quantity unitCost and whole-position totalCost
retain their distinct meanings. Managed lifecycle, terms and valuation use their
existing guarded services and financial invariants. Saving a correction leaves
its check pending. All scope/ownership and managed-position guards remain on the
backend. Financial mutations and evidence receipts are never best-effort paired.

## Schema 18, backup and export

Schema 17 adds reconciliation session/account/item/link/receipt, financial
revision and recorded-change tables, indexes and revision triggers. New-database
DDL and the additive migration have equivalent verified definitions. v16 is
verified before a flushed, verified, read-only recovery copy is retained. DDL and
version advance in one transaction, followed by full verification. Existing v15
uses its original verified v15 → v16 step and then v16 → v17 → v18. Recovery copies
cannot be opened as writable live files; restore a copy to a new path.

Read-only inspection accepts verified v15–v22 without upgrading the source.
Normal live open upgrades a restored copy. Unsupported versions, missing current
structures and malformed facts remain blocked. Backup containers and continuous
backup stream identity retain these exact compatibility boundaries. JSON export
format 2 adds explicit history datasets reconciliationSessions,
reconciliationAccounts, reconciliationItems and reconciliationLinks. Additive
format-2 datasets accountFinancialRevisions and reconciliationFinancialChanges
retain business revision and change evidence; consumers must accept additional
fields/datasets. closingNote is distinct from the opening note. Evidence is
structured JSON; private plan/receipt configuration is excluded. JSON export is
not a restorable application database or MCP session.

## API and interaction boundaries

Read permissions expose list_reconciliation_sessions, get_reconciliation_session,
get_reconciliation_session_page and get_reconciliation_item. Only ledger_write
registers preview_reconciliation_session and commit_reconciliation_session.
Directory permission never authorizes attestations. Existing commit_reconciliation
continues to mean a financial target adjustment, not session completion. Atomic
batches use one batch-level link and reject per-command/mixed links. Historical
replacement commands reject session links. Unlinked existing plans retain their
original durable receipt hash.

Headers omit account/item arrays. List filters status/account/start-date and sort;
start-date bounds are inclusive civil dates in the History Origin timezone, with
UTC fallback when no valid origin timezone exists. Invalid stored start timestamps
fail safely. GUI instant timestamps use the configured presentation timezone;
detail sections page accounts/items/attempts/changes/links with deterministic order.
Pages default to 25, cap at 50 and enforce a 96 KiB encoded budget with explicit
response_too_large. The client reduces limit instead of losing fields. Item rows
omit full detail and attempt arrays explicitly, exposing attemptCount; a complete
single-point read and pageable attempts retain the evidence. Cursors bind filters,
sort, session revision and content, producing stale_cursor on changed evidence.

The GUI uses existing date/time/select/sheet controls, canonical financial
formatters, localized labels and existing Record recovery. Session and correction
confirmation sheets register immutable plan identity before dispatch. Pending or
unknown outcomes guard dismissal and navigation; retry preserves the same plan.
Backend refusals require renewed review. Shared query invalidation refreshes the
session after financial changes. GUI labels use business identity and dates;
protocol IDs and revision tokens stay in control payloads and MCP responses.

Native macOS interaction, screen-reader/manual accessibility and packaging remain
separate from Linux server-tag builds, jsdom tests and generated bindings.


## Approved reconciliation follow-up

The v17 → v18 migration verifies the old DDL, publishes a flushed verified
read-only v17 recovery copy, then adds closing_note and replaces the product
revision triggers in one verified transaction. Fresh DDL and both upgrade paths
share the same final definitions. An unfinished v17 note is an opening note.
A completed v17 note was a closing explanation: migration moves it to closing_note.
An overwritten legacy opening note cannot be reconstructed. Existing receipts
remain original historical outcomes.

Actual OLD/NEW economic values drive revisions: principal, currency, state,
dates, interest and product access/fee terms matter. Name, note, updated_at,
revision and no-op financial assignments do not. Product value observations
retain their existing exemption. This does not change replay, move-account or
double-checkpoint rules.

Instrument archive checks all holdings, including other accounts and archived
rows, for nonzero exact quantities inside the shared write transaction. Holding
archive requires zero regardless of History origin. Legacy archived nonzero
positions stay visible with warnings; new archive operations cannot create them.

Links must match the reviewed holding ID, product ID or cash currency (or balance
target). Atomic companion legs are allowed when at least one leg matches;
missing-item corrections use the account completeness point. Removed rows keep
evidence and disposition, but reject new corrections. The repository validates
this within the financial/link/session transaction, including rollback on mismatch.

A reference is optional evidence. Explicit referenceComparable asserts matching
units/currency/current as-of basis, limited to current-day numeric cash, balance
or quantity values. Exact decimal comparison accepts equal numeric spellings.
A mismatch requires a correction, unresolved disposition, or a user explanation
before confirmation. Historical dates and descriptive references are retained
without comparison. Nothing copies values or confirms checks automatically.

Completed change lists retain the original revision interval and mark later rows
afterCompletion separately. Last-success timestamps remain historical facts with
a neutral later-bookkeeping indication. First-baseline copy comes from each
account's priorSuccessfulAt, never from whether a filtered changes list is empty.
Overlapping sessions remain independent and permitted; Continue appears before
new-session selection. Paused sessions may finish directly; finishing freezes
the completed record. AR19 and broad redesign remain deferred.

Upgrading an already-open session also surfaces legacy archived nonzero positions
when its financial revision has not changed. An omitted position is added as
pending; an old "removed" point is restored as a holding with its earlier attempts
retained. This read projection does not invent a financial revision or mutate a
completed record. The new pending point prevents a full finish until reviewed;
the next session action persists it through the existing evidence transaction.

### Current coverage beside frozen records

Completed reads retain persisted item evidence, account outcomes/counts, the
session revision, opening/closing notes and all timestamps. Counts come from the
saved account evidence, not a recount of today's inventory. The read model derives
`inventoryCoverageGap` and `uncoveredArchivedHoldings` separately from current
archived nonzero holdings missing from the recorded holding/product identities
(or represented only by a removed point). It does not infer whether a current
holding existed when that record completed. The record and account summary ask
for a new reconciliation; the historical success remains intact. Repeated reads
do not write or repair the completed record.

For unfinished sessions, coverage repair still adds/restores pending checks.
`financialRevisionChanged` identifies a real revision difference; inventory-only
repair and remaining pending reviews have their own copy and never claim that
bookkeeping occurred after completion. These current-context fields are stripped
before preview results are persisted, including receipts. They are not exported
as historical facts. `archivedHoldingIds` independently identifies currently archived holdings or
instruments, including zero quantities retained by legacy working records; it
does not label zero quantities as nonzero. Ordinary archived positions retain evidence/disposition and
partial-finish controls, but adjustment and missing-trade actions are hidden
until the holding or instrument is restored. Backend refusals remain authoritative.
Paused sessions may still finish directly.
