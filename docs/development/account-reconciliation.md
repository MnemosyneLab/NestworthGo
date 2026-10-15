# Account reconciliation architecture

The approved design is retained in [the handoff plan](account-reconciliation-plan.md).
This document describes the implemented contract; cloud validation and remaining
native gates are recorded on PR #89. Reconciliation changes the local ledger,
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
product terms use the existing product model, including closed contracts.

Checks store the ledger values reviewed, financial revision, explicit outcome,
reference value, source reference, civil as-of date, optional local time and ledger
timezone, note and actual server checked timestamp. Date-only evidence never
receives an invented source time. Skipped and unresolved outcomes require a note;
confirmation requires actual source and date. Prior attempts remain on each point,
with a maximum of 100 attempts per session item. At that bound, finish partially
and start another session rather than drop evidence. Notes are at most 2,000
bytes, source references 512 and reference values 256.

Start, pause, resume, checks and finish are preview/commit operations. The exact
plan, result and revisions are retained at the boundary; commit regenerates and
compares the result while holding the application write coordinator, then SQLite
validates session CAS and account financial revisions in the evidence/receipt
transaction. Same-plan recovery checks the durable receipt first, including after
expiry and completion. Changed payloads conflict. Uncommitted previews expire in
ten minutes. Desktop plan stores are in memory; MCP plans use the existing private
backup-owned plan store. A lost desktop plan is not proof of a failed commit.

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
Recorded revision order detects newly recorded backdated activity, rather than
using its effective date as a watermark. Separate change-log rows retain recorded
and effective timestamps. There is no prior watermark on a first session.

A changed working account refreshes its current items, retains earlier attempts,
surfaces removed points and requires renewed explicit review. Other accounts keep
their progress. Completed rows stay frozen; current revision differences are a
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

## Schema 17, backup and export

Schema 17 adds reconciliation session/account/item/link/receipt, financial
revision and recorded-change tables, indexes and revision triggers. New-database
DDL and the additive migration have equivalent verified definitions. v16 is
verified before a flushed, verified, read-only recovery copy is retained. DDL and
version advance in one transaction, followed by full verification. Existing v15
uses its original verified v15 → v16 step and then v16 → v17. Recovery copies
cannot be opened as writable live files; restore a copy to a new path.

Read-only inspection accepts verified v15/v16/v17 without upgrading the source.
Normal live open upgrades a restored copy. Unsupported versions, missing current
structures and malformed facts remain blocked. Backup containers and continuous
backup stream identity retain these exact compatibility boundaries. JSON export
format 2 adds explicit history datasets reconciliationSessions,
reconciliationAccounts, reconciliationItems and reconciliationLinks. Evidence is
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
