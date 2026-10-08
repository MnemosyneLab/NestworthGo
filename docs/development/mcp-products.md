# Managed product MCP alignment

The read extension exposes the GUI's managed-contract, operation-history and
liquidity reads. Its dependent lifecycle extension adds five guarded record
operations. Terms and valuation writes remain App workflows pending their
separate extension. This document states the implemented contracts and the
remaining acceptance gates. It does not authorize a
tool, claim client installation, or establish that write alignment is complete.

## Delivery boundaries

| Review unit | Scope | Status |
| --- | --- | --- |
| Read parity | Contract list/detail, closed contracts, operation pages, household product/cash liquidity | Implemented in this change |
| Lifecycle parity | open, record_existing, receive_interest, whole-contract settle, latest safe group undo | Implemented in the dependent lifecycle change |
| Terms/value parity | Revisioned terms/policy updates and locked-product value observations | Follow-up; narrow previews and atomic stable receipts required |

Renewal is excluded until the GUI action is independently established. No unit
exposes reservation creation, modification or release. No releaseReservationIds
may be filled automatically. Preserve managed-position guards on generic trades,
imports, quote writes, reconciliation and corrections. Ordinary NAV holdings
remain a separate workflow when they are not managed contracts.

The implementation shares the existing application service and Wails liquidity
DTO adapters. Do not compose generic cash, trade or quote tools to simulate a
product operation. Tests use synthetic temporary SQLite households only; native
desktop, external-client and R2/live-ledger work are separate acceptance gates.

## Implemented minimal read interface

All modes, including read_only and directory_write, expose these four tools
with readOnlyHint=true, destructiveHint=false and openWorldHint=false. They do
not refresh providers, materialize analysis snapshots, create plans/receipts or
invalidate existing ledger previews. Product names, account IDs, notes and
reservation evidence are identity-bearing data; these are not frozen/minimal
financial-context captures.

| Tool | Input | Authority/result |
| --- | --- | --- |
| list_products | Optional accountId and includeClosed (default false) | ListProducts; GUI ProductDetailDTO array, including policy and active reservations |
| get_product | id (committed contract UUID) | Product; same detail, revision and GUI action reasons |
| list_product_operations | productId, optional cursor and limit | ListProductOperations; metadata page and nullable next cursor |
| get_liquidity_overview | Optional customHorizonOn and includeEarlyWithdrawal | LiquidityOverview; entire GUI overview, evidence, assumptions and unresolved reservations |

The server advertises managed_product_read and liquidity_read capabilities.
Discovery and schemas remain authoritative; GUI permittedActions are not MCP
permission or proof of an exposed write tool. The portable skill version is
1.5.0; updating repository files does not install it in a user client.

History pages retain newest-created-time/UUID ordering. The MCP checks that the
contract exists before returning history, accepts default/zero limit 25, rejects
negative limits or limits above 100, and limits cursor length to 128 bytes.
The cursor is the application cursor, not an authenticated frozen-package
cursor. Clients must retain productId with next and re-query after concurrent
GUI changes. Private product RequestJSON/ResultJSON are not exposed.

Inputs reject unknown fields and wrong types through SDK schemas. Product SDK
schema diagnostics are replaced with fixed validation errors, without echoing
request properties/values. Application domain errors retain safe codes; raw
SQL, paths and internal error details do not reach responses. HTTP responses,
including SDK text plus structured content and RPC envelope, are capped at
64 KiB; RPC IDs at 512 encoded bytes. Batches containing a product tool are
rejected before dispatch, including legacy protocol requests. Oversized output
fails as a whole, never truncated JSON or a silently incomplete subtotal.
An oversized list can be narrowed by account; an oversized single detail or
household overview requires the GUI. This bounds wire output, not database work
or the size of the complete GUI read model.

Money remains exact decimal strings with explicit original currency. Nullable
prices, routes, amounts and FX remain unknown. Liquidity preserves both
pre-reservation fullAvailable/knownAvailableSubtotal and post-reservation
fullUnreserved/knownUnreservedSubtotal. Source netNative is after route fees but
before reservations; unreservedNative subtracts appliedReserveNative. Preserve
requested/applied reserves, shortfalls, unresolved reservations and unknown
source counts. Known subtotals are not complete freely usable balances.

The current GUI ProductDetail application read does not populate
CurrentCostBasis, so the product DTO retains null even when holding gain has an
authoritative cost. This extension preserves that limitation; it does not infer
cost from principal/current value or change the shared read model.

Terms retain actual/365 and actual/360, civil dates, history timezone, effective
instants and paid-through dates. Forecast interest and early/normal receipt
routes are separate from current quote value, actual cash and actual returns.
due_unconfirmed changes display/availability, never automatically writes cash.

## Write interface and permissions

The lifecycle pair is registered by the dependent change. The terms and value
pairs remain proposed and are not yet exposed:

| Preview/commit pair | Accepted command |
| --- | --- |
| preview_product_operation / commit_product_operation | Exactly one tagged payload for open, record_existing, receive_interest, settle or undo |
| preview_product_terms / commit_product_terms | productId, expectedRevision, complete GUI terms and policy |
| preview_product_valuation / commit_product_valuation | Open locked product, explicit original-currency amount and actual observation time |

Only ledger_write may discover or invoke any of these previews or commits.
directory_write must never authorize financial or product-policy writes.
Previews are local no-financial-write operations that may persist a private
plan. Commit input is only operationId plus input.planId; no editable command,
state hash, reservation IDs or explanation text is accepted as authorization.
Reuse GUI application validation and DTO conversion for all effects.

Settling closes the entire contract. It is not partial redemption or an actual
bank instruction. Locks affect predictions, not the ability to record money
that actually arrived. With active reservations, return the existing protected
error with an explicit GUI-resolution message; never release reservations.
Undo must reject groups with reservation restoration/release effects as well
as unrelated later activity, unsafe replay or later contract/policy changes.

## Preview, commit and recovery contract

Persist a type-bound plan in the instance/database authority containing its UUID,
exact validated normalized command, resolved timestamps, current revisions,
reviewed dependency state hash, application preview token and fixed expiry.
Use a product-compatible chronological UUID for lifecycle business identity.
Persisted expiry is ten minutes and is checked after waiting for the application
write permit. Unknown fields, inactive union payloads, renewal, release fields,
wrong plan family and contradictory local/instant time inputs must be rejected.

Preview must serialize dependency collection with all writers and exclusive
backup/restore work. Resolve defaults and local times once using the history
timezone and the same application clock as validation; do not use the MCP host
clock for financial validation. Commit must post precisely that reviewed time
and effect, not silently change a default to its later now.

The guarded application entry points bind a server-owned reviewedAt, included
in the hashed command. They freeze defaults and resolve local time while under
the preview coordinator; ordinary GUI entry points reject that private field.
record_existing has no caller-supplied effectiveAt in the MCP schema and undo
accepts only the target operationId. Both keep the reviewed effective instant
while retaining actual activity creation time, historical replay and latest
safe group validation. Descriptive startOn is not a historical acquisition.
The existing GUI request/validation contract remains compatible.

Keep both the existing reviewed product state hash and a conservative
coordinator token that invalidates on writes, restart and exclusive restore.
Check state/contract/policy revisions, cash/holding/cost/quote dependencies,
reservation evidence and local date inside the same write permit as mutation.
Replay a matching committed business receipt before expiry or stale checks.
Wrong plan family must fail before acquiring a write permit. Uncommitted
expired/stale/restarted/restored plans require a fresh preview.

The plan UUID is the business mutation key; operationId identifies the outer
MCP receipt and hashes the exact tool and input. Reusing one operationId with
changed input conflicts. Repeating the same plan with a new operationId must
return the original business result, not post again. The mutation, business
receipt and all financial effects must commit in one transaction. Outer MCP
pending/unknown receipts may recover only by that atomic business key. A failed
receipt must not hide an already committed mutation after a post-commit derived
refresh failure. Report recorded facts and pending derived work separately.

Lifecycle CommitProductBundle already atomically saves product operations and
their effects. Terms currently use SaveProductTerms, which atomically updates
contract/policy revisions but has no business idempotency receipt. Refactor its
validation into a pure preparation shared by narrow preview and commit, and
extend persistence to save revision-checked terms/policy plus immutable result
and payload digest in the same transaction. Do not wrap the current direct
write in an independently saved MCP receipt and call it recoverable. Decide the
business receipt storage and export/restore/integrity contract in that review;
do not invent a product operation kind without updating schema-15 verification.

AppendProductValuation atomically stores a value_observation operation and quote,
but replay currently returns live Product detail. Add a pure observation preview
and guarded append with plan key, revision/state token and original timestamp.
Recover immutable quote/operation IDs and recorded value from a durable receipt;
reread current detail separately. Never quote a deposit with projected interest,
or import a managed quote through generic market-data tools. Later valuation or
terms changes must continue to block unsafe lifecycle undo.

After restore, distinguish retained receipts from current facts. Production
configuration, plans and business data must share the restored SQLite authority.
Never reuse external/legacy cached success to infer that a restored ledger still
contains a mutation. Reapplication of a missing fact is a new intentional action
after inspecting restored data; uncommitted old plans must not silently execute.

## Write acceptance gates

Require real Streamable HTTP, synthetic SQLite and executable skill examples
for each preview/commit pair. Verify complete schema discovery and permission
isolation; reject wrong family, malformed union, unexpected fields and renewal.
Test expired/stale/revision-conflicting plans, frozen defaults and local times,
history date/365/360 semantics, restart, exclusive restore and concurrent GUI
and MCP edits. Test same operation/plan retry, changed-input conflicts, same
plan/new operationId, pending receipt recovery and post-commit response failure.

Reconcile cash, holding quantity, cost, quote, net worth and actual returns
through open, record_existing (cash-excludes confirmation), interest, settle
and safe undo. Received interest must not be predicted twice or added to NAV;
maturity alone must not affect cash. Test whole settlement and actual early
receipt despite forecast locks. Inject failures through every bundle stage and
terms/policy/receipt stage: assert atomic rollback or safe original-result
recovery. Later operations, trades, terms, valuations and reservation edits must
block unsafe undo. Active reservations must remain unchanged on rejected MCP
writes, with no exposed mutation schema or implicit release.

Run the complete applicable repository gates: Go tests, race, vet, build,
gofmt, generated binding checks, frontend build/lint/typecheck/tests, skill and
installer checks, and diffcheck. Verify the remote PR SHA and CI after push.
Do not merge, publish, connect real financial data or enable R2 as validation.

Lifecycle undo preserves both physical attribution and investment performance:
reversed product interest is dividend/interest rather than external capital,
and reversed product fees compensate the original investment fee. The HTTP
regression compares no-interest versus interest-then-undo households/accounts
with cash included, nonzero stock gains, exact weighted capital and linked rate.
Guarded opening/existing-position undo blocked by active reservations returns
unresolved_reservation_release with GUI handling; ordinary GUI safety errors
retain their existing contract.
