# Optional investment short names

Base: verified main `c84910971e3bf30c110e1e88f74686679172e3a0`.
This change uses a separate branch. After PR #56 and PR #57 merged, the branch
integrates main `74778dbc2f93f253be54ec65c0115112f4b489f9` with a merge commit.
The feature diff against that main remains focused on optional short names.

## Integration with account composition and historical series

The only textual conflict was `frontend/src/features/accounts/accountComposition.ts`.
Main's backend account-composition projection and frontend panel replace the old
client-side donut builder. The resolution preserves that replacement and applies
the universal display helper to the backend's instrument detail labels. Canonical
type grouping, exact totals, basis-point allocation, stable detail references,
top-ten/remainder behavior and chart interaction fixes stay intact. The alias-edit
regression checks unchanged totals, amounts, shares, types and detail references.
Detail projections also carry optional full identity text into the data table,
so duplicate aliases remain distinguishable without replacing the primary label.
The shared chart changes only those optional table cells; renderer selection,
hover, reset behavior and category calculations are unchanged.

Main's historical-series and dense-entry changes merged cleanly. Their bounded,
read-only snapshot paths, chunked activity hydration and civil-day replay remain
intact; the shared instrument query includes the new alias column. Bindings are
regenerated from the combined Go model and services.

## Display and identity contract

Every primary instrument label uses the same order:

1. Trimmed, nonempty `shortName`.
2. Trimmed symbol/code.
3. Trimmed full name.

Go and React share this rule, including Unicode whitespace, NEL and BOM. There
are no market or instrument-type exceptions. Full-name subtitles retain the
existing `InstrumentLabel` layout and classes. Selectors append identity text
(full name, symbol, market, currency and ISIN) and continue to use IDs as values.
Duplicate aliases are allowed, including across share classes and archived
instruments. Sorting and labels do not change stable row/navigation keys.

The optional field permits at most 120 Unicode code points after trimming.
An explicit empty or whitespace-only string clears it. An omitted field keeps
an existing alias, including an older complete-form request that predates this
field. New records default to an empty alias. The field does not change symbols,
provider bindings, ISINs, IDs, quantities, exact money values or transaction
identity. Existing provider bindings are retained when a form edits metadata
on an instrument using Manual or Agent quotes.

The alias is propagated through Wails/MCP requests, instrument DTOs, valuation
and missing-input components, grouped holdings, overview/activity labels,
change preview endpoints, historical hierarchy rows, gain summaries, data
health and sync progress. Current queries and analysis caches are invalidated
after metadata edits. Frozen historical records and financial facts are not
rewritten to substitute an alias for an identity.

Managed products have one persistent alias: `instruments.short_name` on the
contract's linked internal instrument. The product projection reads it from
that instrument. Contract-term edits update the linked instrument's full name
and alias in the same revision-checked transaction, including the guarded MCP
terms path. General instrument editing continues to reject managed instruments.
The optional alias in sealed contract JSON omits empty values, preserving older
receipts' encoding. Product details retain the symbol and legal name.

## Narrow storage upgrade

Schema 16 adds only `instruments.short_name TEXT NOT NULL DEFAULT ''`.
There is one supported predecessor, schema 15; this is not a general migration
chain. Version zero, schemas 1–14, unknown schemas, future versions, and malformed
v15/v16 schemas continue to fail their version/integrity guards.

For an existing v15 file, `sqlite.Open`:

1. Opens a read-only probe and verifies the complete v15 schema, constraints,
   indexes, financial/history invariants, integrity and foreign keys.
2. Creates a consistent `VACUUM INTO` copy in an adjacent private
   `.nestworth-v15-recovery-*` directory. The retained file is
   `original-v15.sqlite` (directory 0700, published file 0400).
3. Verifies the recovery file read-only and flushes its data and directory
   entries before changing the live schema. Failure to create, verify or flush
   recovery stops the upgrade. The snapshot keeps an unpublished
   `.snapshot-v15.partial` name until verification, file flush and read-only
   permissions succeed. Partial artifacts are cleaned on failure; published
   copies remain read-only even if a later directory flush or transaction fails.
4. Acquires the existing immediate SQLite transaction, checks `data_version`
   against the pre-snapshot generation, and verifies v15 again. A concurrent
   committed change aborts rather than proceeding with a stale recovery copy.
5. Adds the column and changes `user_version` to 16 in that transaction, runs
   the complete v16 verifier, and commits. DDL, version and verification errors
   roll back together. Interrupted transactions use SQLite's existing recovery.
6. Continues the normal initialization and permissions/WAL checks. If a later
   startup step fails after commit, the committed database is valid v16 and a
   retry can open it; the verified v15 recovery copy remains available.

There are no financial-row conversions, rebuilds, decimal coercions, deleted
records or bypassed version guards. Read-only backup verification recognizes
validated v15 and v16 without migrating the inspected file. Backup package
restore checks that the database version matches its manifest before the live
session is touched. The existing restore journal and safety-copy behavior remain
in use; the installed v15 copy upgrades when opened normally.

Cloud recovery recognizes v15/v16 stream identities and checks the staged
file's version against that identity. Inspection leaves v15 staged files and
source stream metadata unchanged. New streams use v16; existing stream metadata
is never overwritten. No external connection/provider behavior changes.

Old schema-15 binaries deliberately reject a schema-16 live database. Rollback
copies the retained pre-upgrade v15 file to a **new live path** (file 0600)
for the old binary, or uses a verified backup. Never open the retained original
as a live database; it is read-only and the new binary refuses to upgrade it
in place. It must not reset `user_version` on the upgraded file. The recovery copy reflects
pre-upgrade state: financial writes made afterwards require preservation before
choosing that older state. This change does not implement a v16-to-v15 downgrade.

## JSON export compatibility

`com.nestworth.export` format version 2 is a deliberately versioned financial
projection, not the restorable application database. It does **not** export
short names. This change leaves that contract unchanged. Alias-preserving
backup/restore uses the SQLite backup package or the cloud database snapshot.

## Review follow-up

- Holding selectors, starting-point unit-cost inputs and preview endpoints keep
  the universal primary label first and append code, legal name and available
  ISIN/market/currency/unit/provider identity. Regression cases use duplicate
  aliases in the same account with equal quantities and verify selected IDs.
- Reservation source selection also resolves the instrument identity through
  existing local queries; colliding source descriptions retain source IDs.
- MCP `list_instruments` query matching covers alias, legal name and code,
  preserving duplicate results and Unicode whitespace/case matching.
- Cloud candidates carry the actual verified schema into Wails packaging.
  Populated v15 synthetic cloud objects pass real Manager staging, Wails
  inspect/confirm, journal reconciliation and open/upgrade; every table, exact
  value and ID is compared. Mismatched/future schemas, corrupt LTX, invalid
  confirmation and failed quiesce preserve the original live session and data.
  The local replica backend is injected only through a test-only export.
- v15 schema columns are filtered by name. Tests additionally cover a real
  committed second-connection write between snapshot and upgrade, file flush
  and both directory-flush failures, and attempted in-place recovery upgrade.

## Synthetic evidence

All database tests use temporary synthetic copies. No real financial database
is read or opened. `sqlite/testdata/schema_v15.sql` is the exact schema from the
base commit, not the new schema with a changed version number.

- `TestPopulatedV15UpgradeRecoveryAndReopen`: populated v15, every old column in
  every table compared exactly, empty defaults, reopen, recovery permissions,
  recovery-copy verification and restore into another synthetic destination.
- `TestPopulatedV15FinancialUpgradeAndAliasReload`: copies every old column into
  the immutable v15 schema from a synthetic ledger with funded cash, holdings,
  trades, cost basis, product contracts/operations, history origin and sealed
  terms evidence. Every table/row/value/ID is compared before and after upgrade.
  It edits a managed alias through the guarded terms path, verifies unchanged
  financial history, closes/reopens, and restores an alias-bearing v16 snapshot.
- Failure tests: malformed v15 rejected without writing; recovery directory
  creation failure leaves the original file unchanged; failed post-DDL schema
  verification and cancelled commit restore the complete v15 schema and rows.
- Backup tests: a populated v15 archive reads/restores/upgrades while the
  archive stays byte-identical; manifest/database mismatch fails before install.
- Cloud test: verified v15 stream recovery stages read-only, upgrades only its
  synthetic copy and preserves original stream metadata.
- Domain/application/Wails/MCP tests cover Unicode, whitespace, length limits,
  duplicate share-class aliases, archived instruments, clear/omit/edit/reload,
  immutable identity and values, projections and managed-instrument edit guards.
- React tests cover the universal rule, full-name subtitle classes, duplicate
  aliases/identity-rich selector labels, search, and keyboard editing/clearing
  in English, Simplified Chinese and Traditional Chinese.

## Validation commands and platform limits

The environment lacks native GTK4/WebKitGTK/GLib development dependencies.
Headless verification uses the repository's server build tag and
`CGO_ENABLED=0` for full Go tests, vet, Wails binding generation, TypeScript and
production web/server builds. The server build also uses `-buildvcs=false`, as
in the repository task, because the environment's parent Git mount prevents
automatic VCS stamping. Focused race tests keep CGO enabled.

```sh
GOFLAGS='-mod=readonly -tags=server' CGO_ENABLED=0 go test -p 1 ./...
GOFLAGS='-mod=readonly -tags=server' CGO_ENABLED=0 go vet ./...
GOFLAGS='-mod=readonly -tags=server' CGO_ENABLED=0 go build -buildvcs=false ./cmd/nestworth
go test -race -p 1 -parallel 4 ./internal/domain ./internal/application ./internal/infrastructure/sqlite ./internal/infrastructure/backup ./internal/infrastructure/continuousbackup ./internal/mcpserver ./internal/wailsapi/instrument ./internal/wailsapi/wire -run 'ShortName|V15|UniversalDisplay|SupportedManifest|StreamIdentity|AccountComposition|HistoricalSeries|HistoricalOverviewDense|HistoricalOverviewHydration|HistoricalActivityHydration|StandaloneActivitiesUntil' -count=1
cd frontend
CGO_ENABLED=0 pnpm run test --maxWorkers=2
CGO_ENABLED=0 pnpm run typecheck
pnpm run lint
CGO_ENABLED=0 pnpm run check:bindings
CGO_ENABLED=0 pnpm run build
```

Bindings are generated from Go, including preexisting main P1 types missing
from main's checked-in generated exports. No hand-edited binding shortcuts.
The production web build retains the existing large ECharts vendor-chunk
warning. Native packaged builds and visual/mobile GUI acceptance remain for
parent-coordinated review. Directory flush failures fail closed; native checks
must verify support on each target platform rather than skipping that safety
step silently.

Combined validation against integrated main on 2026-10-12:

| Check | Result |
| --- | --- |
| Full Go suite in server/headless mode | Passed; 37 tested packages |
| Go vet and gofmt | Passed |
| Relevant race suites for aliases, v15 upgrade/recovery, manifests, stream identity, composition, series and activity hydration | Passed across all eight listed packages; application 343.135s |
| Frontend suite with two workers | 100 files / 990 tests passed |
| TypeScript, ESLint and generated-binding consistency | Passed |
| Production frontend and headless server builds | Passed; existing ECharts chunk-size warning |
| Native packaged build and visual/mobile acceptance | Pending parent review; native dependencies absent |

The initial concurrent runs hit existing interaction/performance deadlines;
completed runs use bounded frontend workers and avoid overlapping the full Go
performance suite with race instrumentation. An actual equal-primary-label
sorting regression was fixed with full-name/ID tie-breakers and verified by
`InstrumentHoldingsTable` tests. Subsequent full frontend and Go runs pass.

The combined normal suite retains main's mandatory 1,000-leaf/25,000-activity
dense-entry regression. The race run retains main's existing 20-leaf/8,300-activity
representative, SQLite parameter-boundary coverage and unchanged runtime-deadline
criteria. No new skips or relaxed deadlines were introduced. The original schema
fixture still has Git blob `f0c7bd99fae589368b1ad94d6bf2670a46824ae9`.

## Native QA checklist (pending parent acceptance)

Use a fresh isolated application-data directory and disposable populated v15
copies. Never point the app at the user's financial database.

- [ ] Packaged native build opens populated v15; recovery file exists, is
  private/read-only; a copy at a new live path reopens with the old binary. The live copy reopens as v16.
- [ ] On every supported native OS, backup/directory flush succeeds; disk-full,
  read-only directory and interrupted startup preserve recoverable original data.
- [ ] Primary short name/code/name order holds for all instrument types without
  market exceptions; the small full-name subtitle keeps its position and style.
- [ ] Edit, clear, close/reopen and reload in all three locales. Test tabs,
  Unicode whitespace, emoji/CJK, and 120/121-character limits.
- [ ] Keyboard Tab order, input focus, submit/error focus, screen-reader labels,
  and clearing to fallback work; 320/375px mobile layouts do not overflow.
- [ ] Duplicate aliases, two share classes, archived records, selectors/details,
  account-composition detail tables,
  alias/code/ISIN search, account navigation, insight filters, overview, history,
  data health, historical rows and sync progress identify the intended ID.
- [ ] Managed-product contract editing changes the one shared instrument alias;
  lifecycle, settlement/undo, reservations and ordinary instrument edit guards
  retain financial identity and values.
- [ ] Export a v16 backup, restore it, and verify aliases and exact financial
  records. Inspect/restore a v15 backup and a local synthetic v15 cloud stream.
- [ ] Independent Astra/Atlas review covers storage recovery, projection paths,
  old-client omission semantics and immutable history/transaction identity.
