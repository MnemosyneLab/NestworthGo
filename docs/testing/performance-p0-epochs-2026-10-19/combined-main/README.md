# PR111 combined-main verification

Tested merge commit: `93ddfd19147482ffa29a587a201928545e5a2fd7`.
Normal, non-rewriting merge; parents:

- Reviewed PR111: `3fd8b65d68124cbfa3fa70d325f7a9fb7e89ad9a`.
- Latest main / PR112: `1ee80acea6659b75ee994d9c675d52eaefdb0e45`.

There were no conflicts. `integration.json` records byte-for-byte preservation
of both PRs' changed files in this merge. Only this verification record is added
after the tested merge; no production/test code is changed afterward.

| Combined-tree checks | Application | Domain | SQLite |
| --- | --- | --- | --- |
| Focused normal tests | Passed, 7.246s | Passed, 0.005s | Passed, 0.367s |
| Focused race tests | Passed, 39.388s | Passed, 1.017s | Passed, 11.345s |
| Vet | Passed | Passed | Passed |

The normal selection includes the repeated-epoch full/durable oracle, emitted-day
mutation rejection gates, exact midnight publication/recovery assertions, bounded
window equivalence/scope, snapshot provenance checks, and PR112's FX regressions.
The race selection includes the mixed review full oracle, exact midnight case,
window equivalence/scope and all three new FX precision regressions. Those cover
28-place bounds, unchanged native precision, inverse-FX exact storage/trend/health
and persisted database reopen. Exact commands are in `commands.sh`; raw logs and
exit status are retained alongside this file. Go 1.26.0, GOMAXPROCS=4, isolated
synthetic temp databases; no native app or real ledger/provider/backup is used.

This is correctness/integration verification, **not a new performance benchmark**.
Original baseline/window measurements remain at their recorded pre-PR112 commits;
the stage diagnosis remains at `3007340`. No measurements are rerun or relabelled
for this combined tree. The original p95 review block and baseline production
default remain unchanged. No C1 work or PR111 merge is authorized here. Parent
handles combined-head CI and the subsequent merge decision.
