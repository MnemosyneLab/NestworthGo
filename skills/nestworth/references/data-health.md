# Diagnose and repair data health

For “检查数据健康” call `scan_data_health` with `{}` and explain affected dates,
instruments/currencies, missing/outdated/incomplete snapshots and actionable
prerequisites. Do not start repair for a request that only asks for inspection.

For an authorized repair:

1. Read the scan, then `preview_data_repair` with `{}`. Inspect estimated provider
   requests, snapshot days, targets and `unresolved` prerequisites. Both reads
   are local and make no provider HTTP requests.
2. Explain inputs requiring App provider settings, identity edits or user price/
   cost evidence. Starting repair replans; the preview is an estimate.
3. Call `start_data_repair` with a new UUID and empty input. It requires
   `ledger_write`, can contact configured providers, and persists market data
   and rebuilt snapshots. It does not change ledger facts/provider settings.
4. Read `result.job.jobId`, `attached`, `conflict` and `reason`. `attached` joins
   equivalent running work; `conflict` means no repair-all job was scheduled.
   Track the existing job before intentionally submitting new repair work.
5. Poll `get_data_repair_job` with `jobId`, initially every few seconds and back
   off on unchanged status. Track phase, outcome, request/target counts, rebuilt
   days, blockers and prerequisites. Accepted work continues if the MCP request
   disconnects; use the App Data Health UI for cancellation.
6. Report outcome (`succeeded`, `partial`, `failed`, `cancelled`) and fresh
   remaining health issues. Rebuilding snapshots is not proof that every price/
   FX input exists. Stop polling at a terminal outcome or when user input is
   required; do not repeatedly resubmit identical failing work.

Job status belongs to the current App process and is lost across restart. An
operation receipt durably records submission, not final completion. If a job
is `not_found`, rescan actual data before deciding what remains; don't mark it
failed/completed from that response alone. Unknown submission outcomes are not
automatically replayed; follow [recovery.md](recovery.md).

When supply is Agent-only, a provider repair cannot invent missing NAV/FX data.
Follow [market-data.md](market-data.md) to import real evidence, then rescan.
