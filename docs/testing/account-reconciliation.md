# Account reconciliation validation

Use synthetic households, isolated temporary SQLite/settings paths and fake
providers. Never run these checks on a personal financial database. The approved
scope is in [the plan](../development/account-reconciliation-plan.md); implemented
contracts are in [the architecture](../development/account-reconciliation.md).

## Regression coverage

| Contract | Evidence |
| --- | --- |
| Start selected/all active scope; duplicate/foreign identities refused; equality stays pending | Application and MCP session tests |
| Pause/resume across service and physical database recreation | Application reopen and receipt tests |
| Explicit sourced confirmation, date-only precision, partial intent and outstanding counts | Application, MCP HTTP and real GUI date/time interaction tests |
| Last successful account completion, including fractional-second ordering | Application success/reopen/time-order tests |
| Immutable completion with a separate current recheck indicator | Application completion tests and localized GUI history tests |
| Mixed USD/EUR cash, ordinary holdings, managed contracts and complete scope | Application mixed-account test |
| Multiple removed holdings retain attempts and deterministic preview/commit order | Application removed-item regression |
| Guarded product valuation preserves confirmations/finish previews; terms/cash still require recheck | Fresh-schema and verified-v16-upgrade integration regression |
| Direct targets versus real records; missing instrument/product inventory; atomic renewal | Existing target/cost/trade suite plus linked inventory and MCP workflows |
| Linked financial facts, working-item capture and receipt rollback/recovery | Injected activity failure, restart replay and scope-refresh tests |
| Legacy unlinked target-plan receipt hash remains recoverable | MCP legacy receipt compatibility regression |
| Account-entry session filtering and cached scope isolation | Reconciliation query-hook test |
| Read-only/directory/ledger permission dispatch; cursor conflicts and wire budgets | MCP permission and encoded paging tests |
| Verified v15/v16 upgrades, read-only recovery, rollback/flush failure/retry, backup/restore | SQLite upgrade and backup/continuous-backup suites |
| Pending/unknown review retains identity and guards Escape/navigation; same-plan retry | Reconciliation GUI tests plus existing Record/Sheet recovery tests |
| Receipt-checked expiry permits fresh review; missing plans stay guarded; expired committed plans replay receipts | Application expiry/receipt and GUI dismissal/review regressions |
| Maintained Skill examples and portable bundle/installer behavior | Skill checker and Python tool suite |

MCP summaries default to 25/max 50 rows and reject encoded pages over 96 KiB.
Rows explicitly separate item detail/attempt history; no evidence is silently
truncated. Large scopes can require a smaller page size.

## Cloud commands

The task environment uses Go 1.26, the locked frontend dependencies and Wails
v3.0.0-beta.28. It has no GTK/WebKit headers or sudo. Supported local checks use:

```sh
export GOFLAGS='-mod=readonly -tags=server'
CGO_ENABLED=0 go test -p 1 ./...
CGO_ENABLED=0 go vet -p 1 ./...
CGO_ENABLED=0 go build ./cmd/nestworth
go test -race -p 1 -timeout 10m ./internal/application ./internal/infrastructure/sqlite ./internal/mcpserver -run '^TestReconciliation' -count=1
python3 tools/check-nestworth-skill.py
python3 -m unittest discover -s tools/tests -v
python3 tools/package-nestworth-skill.py
cd frontend
CGO_ENABLED=0 pnpm run generate:bindings
CGO_ENABLED=0 pnpm run check:bindings
CGO_ENABLED=0 pnpm run typecheck
pnpm run lint
CGO_ENABLED=0 pnpm run test --maxWorkers=2
CGO_ENABLED=0 pnpm run build
```

Run wall-clock performance tests without other heavy suites on the same worker.
The initial simultaneous Go/frontend run hit existing performance/deadline limits;
serialized Go and a bounded frontend run passed. No timeout or assertion budget
was raised. Full CI race checks use the repository's four application and two MCP
checksum partitions, infrastructure and remainder jobs, with the existing
10-minute process budgets and `-parallel 4` on split packages. The local remainder
race run excludes `cmd/nestworth` because native headers are unavailable; GitHub
CI installs those dependencies and covers it.

The complete [CI run for implementation commit e8178be](https://github.com/MnemosyneLab/NestworthGo/actions/runs/38697415779)
passed the main test/build job, all eight race shards and the aggregate. This is
pinned evidence for that commit; use PR #89's current-head checks and description
for subsequent scope-label/product-detail refinements and final test results.

## Remaining platform gates

Linux server-tag builds, generated bindings and jsdom do not prove native macOS
behavior. No Mac operations, package/sign/notarize/release, manual screen-reader
review or native window/process-close recovery acceptance was performed. Desktop
plans are in memory; pause/resume and financial/session receipts are durable, but
a lost desktop plan must be resolved by inspecting saved facts rather than
fabricating a new identity. Native Linux compilation is unavailable locally;
the pinned GitHub CI run above covers that gate for its source commit.

Vite builds successfully with large-chunk warnings for ECharts and the shared
display bundle. Generated bindings, bundles, binaries and the portable Skill
archive remain ignored artifacts. The development/debug bundle identifier is
unchanged. No release or public artifact publication is part of this PR.
