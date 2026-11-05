# Preview expiry, interrupted writes and receipt recovery

For reconciliation session commits, `stale_preview` with field `expiresAt`
means durable receipt lookup proved the expired plan did not commit: reread and
obtain a fresh review. An unavailable `planId` or unscoped stale error does not
prove the outcome; retain the original operation/plan and inspect saved facts.

Keep the operation UUID, exact arguments, plan type/ID and returned receipt.
Use `get_operation` with `{"id":"<operation UUID>"}` to inspect execution.
Receipts can survive restarts, but describe past execution rather than current
balances; after restore they may refer to changes no longer present.

| Situation | Action |
| --- | --- |
| Uncommitted plan expired, App restarted, or state became stale | Read required current data and obtain a fresh preview; commit it with `commit_plan` |
| Single/batch/reconciliation/correction/product lifecycle/terms/value commit may already have posted | Retry the exact original operation/input/plan; atomic ledger mutation recovery prevents duplicate posting even after expiry/restart |
| A successful receipt exists | Read the affected current state; do not repost merely because today's state differs |
| A `commit_plan` receipt or `get_operation` shows a per-kind `tool` label (for example a name ending in change, batch or product_terms) | The label names the plan kind, not a tool to call; always commit with `commit_plan` |
| Agent quote import was interrupted | Reuse the same operation UUID and exact input; durable import batch identity returns original quote IDs; then inspect derived statuses/current quotes/health |
| Investment review rule/episode/suppression write outcome is unknown | Inspect `get_operation` and the relevant V2 rule, alert or suppression read; retry the same operation and input only when confirming that receipt |
| Daily v2 rule/alert/suppression/refresh outcome is unknown | Inspect `get_operation` plus the matching v2 list/detail tool; reuse the exact outer operationId and inner mutationId only for the same intended input |
| Directory create/update/archive outcome is unknown | Inspect operation receipt and actual definitions before a new operation; directory writes are not automatically replayed |
| Repair submission outcome is unknown or job disappeared after restart | Rescan health and inspect the App before a new intentional submission; do not replay blindly |
| Same operation UUID with changed input | Conflict; do not modify the old request to repurpose its UUID |
| Known terminal validation failure | Correct the input/intent and preview again where applicable; never retry unchanged failure in a loop |

Plans expire after ten minutes; some App reads materialize snapshots and stale
them. Check prerequisite balances/definitions/analysis before preview. Preview
activity and holding IDs are provisional. The server's stored normalized command
is the committed command; the commit accepts only `input.planId`, not a revised
command. Do not mix single, batch, reconciliation, correction and product-operation, product-terms or product-valuation plan types.

Historical fixes preserve original timestamps and replay later activity. A
returned resulting balance can describe the historical point; reread current
valuation. Snapshot rebuilding or dependent revaluation can remain pending
after facts commit. Recover the original operation first, then inspect health;
do not repost the financial record to repair derived data.

After backup restore, an intentionally reapplied missing change is a new intent
with a new UUID, after checking the restored facts. Backup and restore are App
workflows, not MCP tools; MCP mutation receipts are operation records, not
backup/restore commands or proof that restored data still contains the change.
The outer MCP execution receipt is a separate record from the business receipt
or plan, and the connected App's SQLite backup may include both. Neither receipt
proves that restored ledger facts still contain the change. Check the actual
restored facts before deciding whether to reapply an operation.

Managed product/history and liquidity reads do not create plans or receipts.
Re-query actual contracts, operations, cash and reservation evidence after GUI
writes or restore; maturity is not receipt. Product lifecycle commits use the
same plan-key recovery: identical retries or the same plan/new operation UUID
return original committed product/activity IDs, not a fresh product query.
Terms/value commits likewise return their original revision/value/IDs/times;
get_product reads current facts and full reservations. Business receipts, plans
and outer execution receipts share the SQLite backup authority. A restored older
ledger cannot recover absent facts from external cached success; a missing fact
requires fresh intent and preview after inspecting restored data. JSON household
export excludes private MCP and terms-recovery configuration; it is not a restored
MCP session. Active reservations and undo groups
that would restore reservations require GUI handling. See [product lifecycle](product-lifecycle.md).


An integrity_failed product receipt is not a failed financial transaction to
retry under a fresh intent. Inspect current facts and use the GUI to resolve
local evidence corruption; do not repost to repair metadata. Terms command/result
checksums and same-revision fact comparisons detect inconsistent local evidence,
but are not authentication against coordinated rewriting of a SQLite database.
Later revisions retain the original recorded result rather than substituting
current terms, policy, quote or value.

## Recovering when operationId was omitted

Prefer an explicit saved operation UUID for new commits. If an existing
`commit_plan` omitted it, use receipt `id` when available. If the response was
lost, compute the same ID locally from the original planId, without submitting
another write: UUID v5 under namespace `f1f65af0-bbb8-5d57-af9c-b624de715dcb`,
with the canonical lowercase hyphenated plan UUID as the UTF-8 name. This
namespace is UUID v5 of `nestworth:mcp:commit_plan:operation` under the RFC URL
namespace; do not hash the plan directly under the URL namespace.

Python's standard library gives an executable read-only recipe:

```python
import uuid
namespace = uuid.UUID("f1f65af0-bbb8-5d57-af9c-b624de715dcb")
plan_id = "PASTE_THE_ORIGINAL_PLAN_UUID_HERE"
operation_id = str(uuid.uuid5(namespace, str(uuid.UUID(plan_id))))
print(operation_id)
```

Test vector: plan `11111111-1111-4111-8111-111111111111` yields operation
`ea140a3e-eff3-54df-b11b-7a05f1d01270`. Use `get_operation` with that computed
UUID as `id`, then inspect current ledger/product/policy facts. Do not pass the
planId as the operation ID. If local UUID computation is unavailable, preserve
the plan and request help recovering the ID; do not invent one or resubmit just
to learn it. Missing/pending/unknown receipts do not prove non-execution. Keep
the original intent and follow the plan-family recovery rules; liquidity
save/reset must not be blindly replayed. A fresh preview is appropriate only
after outcome/state inspection establishes a separate intended write.

## Frozen capture and batch recovery

See [paging and batches](paging-and-batches.md). read_expired/read_revoked means
restart every section from a new initial capture; discard old pages. validation
on a cursor means preserve the original matching capture/section, never guess
an offset. too_large summary/single_item/capture requires an explicit narrower
workflow or a documented limitation, never silently omit financial evidence.

Atomic business batches still have a conservative outer-receipt crash window.
Pending/unknown requires `get_operation` plus fresh IDs/version/revision/state
before another intended write. Identical successful retries retain their short
receipt, which does not establish today's metadata, review state or coverage.

## Reconciliation session recovery

A reconciliation session commit through `commit_plan` uses the same receipt-first
recovery rule as financial commits. Retry the original operationId and planId after an unknown
outcome; never replace a user's confirmation with a new inferred one. After a
conflict, reread the session header and item financialRevision and request renewed
review. A completed session is immutable; changed current facts are a separate
recheck projection. Pause/resume preserves progress across App restart. Desktop
confirmation plans are in memory; a lost desktop plan remains an unknown outcome
until the saved session and financial receipts are inspected.


Production MCP plans and operations are stored in the connected App's SQLite
configuration. File paths describe only legacy/test storage. Succeeded operation
receipts replay without their plan. Missing bodies for pending/unknown commits
remain operation_outcome_unknown unless non-submission is proven; do not turn
that state into a new intent or repeatedly retry automatically. Reconciliation
session and target-adjustment business receipts may resolve a lost body on an
explicit identical retry. Unknown plan IDs never execute. A new plan after a
revision change is a distinct reviewed intent; changing an outer operation UUID
alone does not establish duplicate execution.


## Machine-readable MCP recovery hints

Safe external MCP tool errors retain isError and the original code/field/message
JSON text. the standard result metadata `_meta["nestworth.errorRecovery"]` additionally uses
schemaVersion mcp-error/1 with
error (the same fields) and recovery {action, automaticRetry, preserveIntent,
discardCapture}. automaticRetry is always false: hints never authorize replay,
new operation IDs, identity disclosure or permission changes.

restart_read/discardCapture applies to expired/revoked capture families.
inspect_operation_and_current_state with preserveIntent applies to unknown
outcomes and stale/integrity-sensitive writes: retain original IDs/input/plan,
then follow the receipt/state rules above. A hint does not prove a stale write
never executed. correct_input, retry_later, inspect_current_state and
inspect_limits_or_narrow_read are navigation hints, not successful outcomes.
HTTP authentication/protocol errors can arrive outside this tool-error shape.
Successful execution and recovery receipts remain complete, unmodified values.
