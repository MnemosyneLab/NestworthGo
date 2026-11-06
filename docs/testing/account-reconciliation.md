# Account reconciliation validation

Use synthetic households, isolated temporary SQLite/settings paths and fake
providers. Never run these checks on a personal financial database. Implemented behavior and financial safety boundaries are defined by the
[account reconciliation contract](../development/account-reconciliation.md).

## Regression coverage

| Contract | Evidence |
| --- | --- |
| Start selected/all active scope; duplicate/foreign identities refused; equality stays pending | Application and MCP session tests |
| Pause/resume across service and physical database recreation | Application reopen and receipt tests |
| Explicit confirmation with optional source reference, date-only precision, partial intent and outstanding counts | Application, MCP HTTP and real-component date/time interaction tests |
| Empty/omitted source reference survives preview, commit and saved reads for scope and balance checks; malformed dates/times and source byte limits remain enforced | Application optional-reference durability, desktop adapter and MCP HTTP regressions |
| New cutoff defaults to ledger today across timezone differences, midnight and reopen; saved/manual dates and intentional clearing stay unchanged | ReconciliationPage tests with actual shared controls and synthetic clocks (jsdom, not native GUI) |
| Last successful account completion, including fractional-second ordering | Application success/reopen/time-order tests |
| Immutable completion with a separate current recheck indicator | Application completion tests and localized GUI history tests |
| Mixed USD/EUR cash, ordinary holdings, managed contracts and complete scope | Application mixed-account test |
| Multiple removed holdings retain attempts and deterministic preview/commit order | Application removed-item regression |
| Guarded product valuation preserves confirmations/finish previews; financial terms/cash still require recheck; metadata does not | Fresh-schema and verified-v21-upgrade integration regression |
| Direct targets versus real records; missing instrument/product inventory; atomic renewal | Existing target/cost/trade suite plus linked inventory and MCP workflows |
| Linked financial facts, working-item capture and receipt rollback/recovery | Injected activity failure, restart replay and scope-refresh tests |
| Legacy unlinked target-plan receipt hash remains recoverable | MCP legacy receipt compatibility regression |
| Account-entry session filtering and cached scope isolation | Reconciliation query-hook test |
| Read-only/directory/ledger permission dispatch; cursor conflicts and wire budgets | MCP permission and encoded paging tests |
| Verified 21→22 upgrade, read-only recovery, rollback/flush failure/retry, backup/restore | SQLite upgrade and backup/continuous-backup suites |
| Pending/unknown review retains identity and guards Escape/navigation; same-plan retry | Reconciliation GUI tests plus existing Record/Sheet recovery tests |
| Receipt-checked expiry permits fresh review; missing plans stay guarded; expired committed plans replay receipts | Application expiry/receipt and GUI dismissal/review regressions |
| Maintained Skill examples and portable bundle/installer behavior | Skill checker and Python tool suite |

MCP summaries default to 25/max 50 rows and size pages against the complete
65,536-byte JSON-RPC response, including both SDK representations. Rows explicitly
separate item detail/attempt history; no evidence is silently truncated. Large
pages shrink automatically, with cursors advancing after delivered rows. A
single oversized item fails with too_large rather than being skipped.

## Running checks

Use the pinned toolchain and commands in the [local workflow](../development/local-workflow.md).
Choose the reconciliation suites in `internal/application`,
`internal/infrastructure/sqlite`, `internal/mcpserver` and
`internal/wailsapi/reconciliation`, plus `frontend/src/features/reconciliation`
and `frontend/src/queries/reconciliation.test.tsx` for the affected boundary.

Include expiry/recovery after process restart, edited drafts and late responses,
multiple accounts and currencies, managed-product replacement/renewal, and
liability purchase/refund/repayment. The [UX/liability matrix](reconciliation-ux-and-liabilities.md)
adds keyboard, focus and grouping checks. Use supported-schema fixtures;
historical schema-version test names are not current compatibility promises.
Current support lives in [storage recovery](../architecture/data-and-ipc-contracts.md#database-support-and-storage-recovery).

Native macOS focus/scroll, actual IPC recovery and three-locale presentation
require a separate isolated native run. Passing synthetic tests or an older
release's acceptance does not close those gates for a new revision.
