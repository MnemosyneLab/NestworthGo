# Account reconciliation validation

Use synthetic households, isolated temporary SQLite/settings paths and fake
providers. Never run these checks on a personal financial database. Implemented behavior and financial safety boundaries are defined by the
[account reconciliation contract](../development/account-reconciliation.md).

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
| Guarded product valuation preserves confirmations/finish previews; financial terms/cash still require recheck; metadata does not | Fresh-schema and verified-v16-upgrade integration regression |
| Direct targets versus real records; missing instrument/product inventory; atomic renewal | Existing target/cost/trade suite plus linked inventory and MCP workflows |
| Linked financial facts, working-item capture and receipt rollback/recovery | Injected activity failure, restart replay and scope-refresh tests |
| Legacy unlinked target-plan receipt hash remains recoverable | MCP legacy receipt compatibility regression |
| Account-entry session filtering and cached scope isolation | Reconciliation query-hook test |
| Read-only/directory/ledger permission dispatch; cursor conflicts and wire budgets | MCP permission and encoded paging tests |
| Verified v15–v21 upgrades, read-only recovery, rollback/flush failure/retry, backup/restore | SQLite upgrade and backup/continuous-backup suites |
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
review or native window/process-close recovery acceptance was performed. At PR #89, desktop
plans were in memory; PR #96 adds durable private SQLite recovery as described
below. A missing body with no proven receipt remains unknown; never fabricate
a replacement identity. Native Linux compilation is unavailable locally;
the pinned GitHub CI run above covers that gate for its source commit.

Vite builds successfully with large-chunk warnings for ECharts and the shared
display bundle. Generated bindings, bundles, binaries and the portable Skill
archive remain ignored artifacts. The development/debug bundle identifier is
unchanged. No release or public artifact publication is part of this PR.

Archive regression: synthetic repositories reject archiving nonzero holdings or
an instrument with any remaining units, within the financial transaction and
without requiring a History origin. Legacy archived nonzero positions remain
pending checklist evidence and carry an explicit archive warning.

## Approved follow-up, 2026-10-16

Implementation branch: `fix/account-reconciliation-approved`, draft PR #96,
based on main `c33c735cea1beb3dfb77e75cd050be940103bcdd`. PR #95 remains
the source review, not the implementation branch.

| Evaluated scope | Regression evidence |
| --- | --- |
| AR01/02/21: historical success, Continue, independent overlapping sessions, per-account baseline | Application frozen-success/later-change tests and GUI continuation/baseline copy |
| AR03/06/14: holding trade defaults, early omissions, removed evidence and named Retry | GUI real-Sheet tests; selected account/holding/link assertions; removed actions hidden |
| AR04: transactional archive guards regardless of History, across every account, legacy visibility | Repository/application archive tests; synthetic legacy read fixtures retain valuation/gain coverage |
| AR05/07/09: comparable references, partial-finish intent, exact cost choice | Exact-decimal application cases, historical/text reference exclusion, real field-error/checkbox interactions and wire cost-field mapping |
| AR08/12/13: refresh known conflicts, unknown guard, action labels and immutable completion | Session/product recovery and dismissal tests; pause-to-finish backend test; destination-specific labels |
| AR10/11: conditional reused product fields, maturity versus receipt, valuation route and current/target | Real ProductCorrection test with canonical DepositFields/DatePicker; conditional receipt/valuation fields and neutral adjustment display |
| AR15/20: financial OLD/NEW triggers, separate opening/closing notes | Fresh and upgraded schema tests, metadata/no-op exclusion, principal/currency/state/date/interest changes; verified v17 recovery copy and legacy-note migration |
| AR16/17: frozen interval versus later changes, exports, concrete links and companions | Application business-export assertions and private-data exclusion; wrong-holding/cash correction rollback and successful atomic companion |
| AR18: durable desktop mapping, SQLite MCP recovery, unknown old commits | Adapter/process recreation, missing-body receipts, canonical five-command round trip, succeeded/pending/old-not_found operations, and old unknown plan after a newer plan/revision |

AR19, broad redesign, and unproven replay, account-move and double-checkpoint
changes are deferred. No global single-session database constraint was added.

Validation commands (all data synthetic; tool paths/caches are environment setup):

```sh
export GOFLAGS='-mod=readonly -tags=server'
CGO_ENABLED=0 go test -p 1 -parallel 4 ./... -count=1 -timeout 12m
CGO_ENABLED=0 go vet -p 1 ./...
CGO_ENABLED=0 go build -o /tmp/nestworth-reconciliation-server ./cmd/nestworth
CGO_ENABLED=1 go test -race -p 1 -parallel 4 ./internal/application ./internal/infrastructure/sqlite ./internal/mcpserver ./internal/wailsapi/reconciliation -run 'TestReconciliation|TestDesktopReconciliation|TestV17|TestProductRefusal' -count=1 -timeout 5m
CGO_ENABLED=1 go test -race -p 1 -parallel 4 ./internal/wailsapi/reconciliation -run 'TestDesktopReconciliation|TestDesktopAdjustment|TestProductRefusal' -count=1 -timeout 3m
python3 tools/check-nestworth-skill.py
python3 -m unittest discover -s tools/tests -v
cd frontend
CGO_ENABLED=0 pnpm run typecheck
pnpm run lint
pnpm exec vitest run --maxWorkers=2
pnpm exec vitest run src/features/reconciliation src/queries/reconciliation.test.tsx --maxWorkers=2
CGO_ENABLED=0 pnpm run build
```

Full frontend: 132 files / 1,368 tests passed with two workers. The initial
four-worker run had 1,367 passes and one existing historical-overview
`findByRole("button", {name:"Bank"})` failure. No historical-overview code/test
was changed; the complete suite was rerun with the explicit worker bound.
The Python tool suite passed 53 tests, and the Skill checker validated 76 JSON
examples. The four-package race invocation passed application, SQLite and MCP;
its desktop compile overlapped a final source edit and was rerun separately
against the frozen final source (including the new cost-field cases).
These are focused race checks, not a claim of full-repository race coverage.

CGO-disabled bindings and the server-tag build are Linux/cloud checks. The
initial CGO-enabled full Go invocation failed to find GTK4/WebKit development
packages. Native Linux GUI launch, native macOS build/window/process-restart
acceptance, screen-reader testing, signing and packaging were not performed.
The Vite bundle builds with the existing large-chunk warning. No user machine,
personal financial profile, real database or provider account was used.

The final legacy-inventory regression additionally emulates an already-open old
session with an omitted or persisted "removed" archived nonzero position at an
unchanged financial revision. Reads must restore a pending holding with the
archive warning and old attempts, without inventing a financial revision.
This case is also covered under the race detector:

```sh
CGO_ENABLED=1 go test -race -p 1 -parallel 4 ./internal/application -run 'TestReconciliationArchiveGuardAndLegacyVisibility|TestReconciliationSessionEvidenceDurabilityPartialAndImmutable|TestReconciliationRemovedItems' -count=1 -timeout 3m
```

The subsequent UI review fixes pass the selected item's `instrumentId` into the
shared missing-trade form, in addition to its holding/account/link context.
`ReconciliationRecordFlow.test.tsx` retains the real RecordChangeForm, Sheet,
selects and inputs; in en/zh-CN/zh-TW it verifies the known instrument is selected
and that a buy preview needs no instrument reselection.

Adjustment error coverage uses real correction fields with service-boundary
failures. Missing/both cost choices receive the choice message; malformed unit
or total costs, quantity reductions, cost-only corrections, zero-quantity costs,
currency mismatches and precision failures receive their corresponding localized
guidance. Unknown reasons retain the normal wire-error fallback. Editing an
unrelated cost does not clear a non-choice error. No backend financial validation,
decimal arithmetic or wire protocol changed in this UI follow-up.

UI review follow-up validation: `pnpm exec vitest run --maxWorkers=2` passed
133 files / 1,399 tests, including 31 new regressions. The initial default
four-worker `CGO_ENABLED=0 pnpm run test` had 1,398 passes and the same existing
historical-overview missing `Bank` button failure documented above; the complete
two-worker run passed without altering that test or its deadlines.
`pnpm run lint`, `CGO_ENABLED=0 pnpm run typecheck`, and
`CGO_ENABLED=0 pnpm run build` passed. Vite retains its existing chunk-size warning.
At UI review head `48e911dd8d486a1d8c5a4eb68a8d97e744bad438`, the Go source
was unchanged from `6f11e3f532a7038ec35648148986d32e39e02432`;
the full Go suite and focused race checks above were not rerun for this
frontend-only delta. `CGO_ENABLED=0 go build -o
/tmp/nestworth-reconciliation-server ./cmd/nestworth` passed again with the new
frontend bundle, using the server-tag GOFLAGS shown above.
Native GUI/macOS and manual accessibility gates remain unperformed.

## PR96 current coverage and v2 schema follow-up

Base: `48e911dd8d486a1d8c5a4eb68a8d97e744bad438`, same implementation branch
and draft PR. Completed historical evidence is not repaired in place. Synthetic
regressions cover holding-only, instrument-only and combined legacy archives;
positions omitted by older records and positions created after completion; equal
and changed financial revisions; repeated detail/last-success reads with identical
before/after exports; preserved timestamps, notes, outcomes, item evidence, counts
and revision; and a new pending session. Forced adjustments and explicit buy/sell
commands still reject archived targets. Evidence disposition and direct paused
partial finish continue to work.

Real GUI components cover en/zh-CN/zh-TW coverage warnings on the historical record
and account summary, current inventory versus real revision changes and pending
reviews, hidden archived correction actions, retained disposition and paused
partial-finish controls. The focused reconciliation/query suite passed 5 files /
100 tests, including archived zero-quantity legacy points. These use independent
current archive context, retain the evidence review action, and never show a
nonzero warning for zero quantity. No custom pickers, forms or dialogs were introduced.

The export schema tests validate bytes from `ExportJSONBytes`, including populated
session/account/item/link/revision/change datasets and Agent append/correction/
retraction provenance. Older v2 optional datasets and closing notes may be absent;
original required datasets and row/field types remain enforced. Current read hints
are not exported. The writer and format version remain unchanged.

Final-source validation commands (synthetic data only):

```sh
export GOFLAGS='-mod=readonly -tags=server'
CGO_ENABLED=0 go test -p 1 -parallel 4 ./... -count=1 -timeout 12m
CGO_ENABLED=0 go vet -p 1 ./...
CGO_ENABLED=1 go test -race -p 1 -parallel 4 ./internal/application ./internal/infrastructure/sqlite -run 'TestReconciliationCompletedCoverageIsDerivedAndReadOnly|TestReconciliationArchivedZeroHoldingReadContext|TestReconciliationArchiveGuardAndLegacyVisibility|TestReconciliationNotesReferenceBasisAndFrozenLaterChanges|TestReconciliationProductValuationPreservesChecksInFreshAndUpgradedSchemas|TestJSONExport|TestAgentMarketDataCorrectionRetractionAndSourceOnlyHistory' -count=1 -timeout 3m
python3 tools/check-nestworth-skill.py
python3 -m unittest discover -s tools/tests -v
gofmt -l cmd internal
git diff --check
cd frontend
CGO_ENABLED=0 pnpm run generate:bindings
pnpm exec vitest run src/features/reconciliation src/queries/reconciliation.test.tsx --maxWorkers=2
pnpm run lint
CGO_ENABLED=0 pnpm run typecheck
pnpm exec vitest run --maxWorkers=2
CGO_ENABLED=0 pnpm run build
cd ..
CGO_ENABLED=0 go build -o /tmp/nestworth-reconciliation-server ./cmd/nestworth
```

Full Go tests and vet passed. The final-source focused race run passed application
and SQLite; this is not full-repository race coverage. An earlier SQLite race
filter selected no tests and was corrected to the actual named regression before
the final two-package run. Full frontend: 133 files / 1,418 tests passed, including
the zero-quantity archive additions. Lint, regenerated-bindings/typecheck passed.
Python tools passed 53 tests; the Skill checker validated 76 JSON examples.
No real financial profiles, provider accounts or user machines were accessed.

The production frontend build and the server build embedding that bundle passed.
Vite retains its existing large-chunk warning. Server-tag/CGO-disabled Linux
checks and jsdom are not native desktop validation: no native Linux GUI launch,
macOS packaging/window/process-restart, browser layout or manual screen-reader
acceptance was performed. CI and independent review remain separate merge gates;
this delegated task commits/pushes only and does not merge. AR19, historical-
overview terminology, broad redesign and unproven replay/account-move/double-
checkpoint changes remain deferred.
