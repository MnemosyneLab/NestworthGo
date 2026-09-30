# Preview expiry, interrupted writes and receipt recovery

Keep the operation UUID, exact arguments, plan type/ID and returned receipt.
Use `get_operation` with `{"id":"<operation UUID>"}` to inspect execution.
Receipts can survive restarts, but describe past execution rather than current
balances; after restore they may refer to changes no longer present.

| Situation | Action |
| --- | --- |
| Uncommitted plan expired, App restarted, or state became stale | Read required current data and obtain a fresh preview; use its matching commit tool |
| Single/batch/reconciliation/correction commit may already have posted | Retry the exact original operation/input/plan; atomic ledger mutation recovery prevents duplicate posting even after expiry/restart |
| A successful receipt exists | Read the affected current state; do not repost merely because today's state differs |
| Agent quote import was interrupted | Reuse the same operation UUID and exact input; durable import batch identity returns original quote IDs; then inspect derived statuses/current quotes/health |
| Directory create/update/archive outcome is unknown | Inspect operation receipt and actual definitions before a new operation; directory writes are not automatically replayed |
| Repair submission outcome is unknown or job disappeared after restart | Rescan health and inspect the App before a new intentional submission; do not replay blindly |
| Same operation UUID with changed input | Conflict; do not modify the old request to repurpose its UUID |
| Known terminal validation failure | Correct the input/intent and preview again where applicable; never retry unchanged failure in a loop |

Plans expire after ten minutes; some App reads materialize snapshots and stale
them. Check prerequisite balances/definitions/analysis before preview. Preview
activity and holding IDs are provisional. The server's stored normalized command
is the committed command; the commit accepts only `input.planId`, not a revised
command. Do not mix single, batch, reconciliation and correction plan types.

Historical fixes preserve original timestamps and replay later activity. A
returned resulting balance can describe the historical point; reread current
valuation. Snapshot rebuilding or dependent revaluation can remain pending
after facts commit. Recover the original operation first, then inspect health;
do not repost the financial record to repair derived data.

After backup restore, an intentionally reapplied missing change is a new intent
with a new UUID, after checking the restored facts. Backups/restores themselves
are App workflows, not MCP tools.
