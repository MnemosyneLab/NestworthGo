# E1: bounded shared financial fact shadow

E1 adds schema **23**, following the actual schema-22 main baseline
`dc199c1127e0b457aa969e01cee1245be7f60c0b`. It does not migrate production
readers, remove old tables, backfill history at startup, implement checkpoints,
or change rule evaluation and episode handling. Existing per-publication memory
sharing is unchanged and is not an E1 gain.

## Activation and bounds

Normal initialization does **not** enable this shadow. The internal repository
copy constructor `WithReviewFinancialFactShadow(ruleDays)` admits at most 128
emitted rule-days per successful publication (nonpositive values disable it).
There is no production environment flag or GUI control. The first emitted
rule-days are sampled; retained suffix days and no-change publications do not
acquire historical refs. Skipped IANA civil dates (for example 2011-12-30 in
Pacific/Apia) have no civil cutoff and are omitted only from shadow sampling.
Their authoritative unknown slot, `nonexistent_local_date` reason and empty leaf
evidence remain unchanged; the shadow never evaluates a replacement result for
these dates. Other key/cutoff errors still abort the publication. This is
intentionally incomplete diagnostic coverage.

Admission reserves worst-case new versions and refs for the entire sample before
inserting anything. A sample exceeding 256 KiB of estimated encoded columns,
4,096 lifetime fact rows, 16,384 lifetime reference rows, or 8 MiB lifetime encoded
columns is skipped in full. Counts and byte sums include all households in the
local store. These are diagnostic limits, not a SQLite file-size guarantee:
indexes, record headers, free pages, WAL and recovery copies are additional.
An admitted comparison or write failure aborts the publication; it never produces
a successful diagnostic verdict for rejected or cancelled publication.

## Facts, versions and semantic isolation

`review_financial_daily_facts` stores stable fact IDs, exact serialized semantic
keys, source-version/provenance bytes, payload and coverage. Household and
instrument are separate lookup fields and restrictive foreign keys. The only
supported account scope is the entire historical household aggregate. Adding an
account filter requires a new explicit scope namespace; current-account scope
must never stand in for historical scope.

The key includes economic date and actual UTC civil cutoff, timezone/cutoff and
schedule policy, data/calendar/admission policy, metric policy, historical
resolver policy, FX provider selection, algorithm namespace, and sorted distinct
financial requirements `(metric, currency, FX mode, lookback)`. Rule IDs,
conditions, operators, thresholds, target selection and trigger windows are not
financial identity. Two targets selecting the same instrument still use the
same household aggregate. Requirements are deliberately conservative: different
metric requirement sets do not share even their compatible base subset in E1.

The calendar namespace uses the existing temporal clock epoch. The application
pins financial timezones within a process; a new DB session/rollback changes
that epoch. No persisted fact is assumed calendar-compatible across sessions.
Price/FX basis and provider candidate metadata are additionally versioned in
source provenance. The fixed algorithm namespace must be revised if replay,
cost-basis or quote-selection semantics change.

Each sampled day has a `base_v1` fact for quantity/type/native currency/amount/cost
and a `metrics_v1` fact for performance by lookback and selected FX by currency.
These payloads are separate rather than a mutable union of every rule's needs.
The existing complete-input codec preserves private exact FX scalars explicitly;
all runtime JSON uses `internal/jsoncodec` (Sonic). Coverage and the original
input fingerprint remain in the store/reference. Provenance retains the source
checkpoint and historical fingerprint, plus economic-date price candidates with
exact price scalars and their source/provider/binding/basis metadata. Selected
FX records and exact rates are retained with the metric payload. This diagnostic
is not an independently sufficient source-history archive or a reader proof.

Version lookup compares **all actual namespace, source, payload and coverage
bytes**, not just a hash. New source or payload content appends a new stable ID.
Application writes never update/delete fact rows or prior references. There is
no GC in E1. Refs retain rule/condition/instrument/timezone/day, original input
fingerprint and exact fact ID. The old per-rule inputs/results remain the sole
production authority, including their existing fingerprint compatibility rules.

## Transaction and comparison boundary

Shadow insertion executes within the existing successful-publication transaction,
after daily writes and all source/rule/handling/time/suffix guards, before
control, final certification and temporal commit. Rollback/cancellation removes
both authoritative and shadow writes. No independent connection, staging commit,
startup migration replay or asynchronous writer publishes partial facts.

The adapter reloads persisted payload/coverage bytes, reconstructs the complete
financial input, compares exact encoded input bytes, and evaluates the unchanged
rule against the reconstructed input. Exact result bytes must match the staged
rule result. Thresholds, three-valued votes, reasons, rolling/consecutive policy,
episodes and acknowledgements stay rule-specific. Existing exact SQL parameter
comparisons, MAC/CAS checks and final row seals are unchanged. E1 rows are not
added as computation authority or as a shortcut to any existing proof.

### Opening evidence versus immutable fact versions

The baseline deliberately replays same-condition financial corrections and can
update an existing episode's opening payload while retaining episode identity
and acknowledgement. E1 preserves that behavior. “Immutable facts” means an old
referenced shared version is never overwritten or aliased; it does **not** impose
new frozen-opening semantics on existing episode payloads. The correction fixture
checks shadow-off/on equality over actual persisted daily inputs/results,
episode evidence, opening/correction events and acknowledgements, while retaining
both original 20% and corrected 30% shared metric versions. No lifecycle changes
are included.

## Migration, rollback and future E2

Fresh databases install empty tables. Verified schema-22 databases take a verified,
flushed read-only `original-v22.sqlite` recovery copy before an additive upgrade.
Earlier supported schemas still upgrade sequentially. Read-only verification
accepts 22 without upgrading; schema/compiled writer-contract verification knows
both new table definitions and the lookup index. Current SQLite snapshots preserve
facts, refs and existing episode evidence/acknowledgements.

Algorithm rollback disables the internal shadow constructor and retains the new
tables for diagnosis; it does not downgrade schema. An old binary cannot open a
schema-23 live ledger. Recovery means restoring a verified pre-upgrade copy to a
new live path, accounting separately for any later financial writes; never
silently replace the current ledger. Peak disk can include **old tables + new
facts/refs/index + WAL + complete recovery copy** (and an explicit backup). The
encoded-column cap cannot predict that peak. Migration creates no all-history
backfill, but the mandatory recovery copy itself needs disk space.

E2 must separately authorize reader cutover, exact compatibility/reconstruction,
join and index costs, bounded migration and atomic activation, old-reference
retention/GC, and restore/import behavior. E1's sampled refs are not a complete
migration frontier. No old-table deletion or F checkpoint is authorized.

The [E2a reader-cutover assessment](review-shared-financial-facts-e2a.md) identifies
the missing current-reference and complete-proof boundary. It is design evidence,
not an implemented or enabled E2 production path.

## Synthetic observations and validation

Linux amd64, Go 1.26; synthetic temporary SQLite databases only; no providers or
real ledgers. Two thresholds, one instrument, eight civil days:

- 16 authoritative per-rule inputs; 16 unique shared facts (8 base + 8 metrics),
  32 refs. Payload + coverage: old 4,366 bytes, shared 4,237 bytes. Including
  fact identity/key/provenance/metadata columns and reference columns, additional
  encoded data is 40,819 bytes (34,323 facts + 6,496 refs), excluding SQLite
  headers/index pages/WAL. E1 adds 48 inserts to the 32 daily input/result writes,
  a 2.5x daily-row mutation count including the unchanged writes; lifecycle and
  control work are excluded. Duplicate fact lookup/read/validation adds work too.
- Correcting close 120 to 130 appends versions: 32 facts total, all original 16
  rows remain exact, and the 20% and 30% metric each has one version shared by
  both thresholds. Source-version isolation conservatively duplicates unchanged
  sampled base facts too. This is not a disk-saving or latency claim.

The exact FX fixture compares off/on output and checks preservation of
`2.123456789012`. Key tests separate timezone, currency, FX mode, lookback,
provider, calendar session, resolver and data policy. Fault tests reject a late
publication write failure and cancellation, and compare both old and new rows;
snapshot/reopen preserves versions and acknowledgement. Migration tests cover
recovery, flush failure and missing-index rejection. The previous uncommitted
frozen-opening probe was converted into the differential test above.

Commands/results and platform limitations are recorded in the PR. No race,
repeated performance suite, native Mac, GUI, export-format migration or p95/RSS
claim is part of this slice. **E1 does not improve production performance before
reader cutover and temporarily adds writes when opted in.** CI elapsed time review
is handed to the coordinator; the normal workflow was not expanded.

Validation commands (tool environment supplied Go 1.26, `GOFLAGS=-mod=readonly
-tags=server`, `GOMAXPROCS=5`; caches outside the repository):

```sh
go test ./internal/application -run '^TestReviewFinancialFactShadow' -count=1 -v
go test ./internal/infrastructure/sqlite -count=1
go test ./internal/application ./internal/infrastructure/backup ./internal/infrastructure/continuousbackup \
  -run '^(TestReviewFinancialFactShadow.*|TestPopulatedV15FinancialUpgradeAndAliasReload|TestPopulatedV15CloudInspectConfirmAndUpgrade|TestCloudStageAcceptsVerifiedV15IdentityAndUpgradesOnlySyntheticCopy|TestSourceTrackingRestoreRotatesLiveEpochBeforeResume|TestReviewPublicationFailureAfterFinancialCommitDoesNotHalfWrite|TestReviewHistoricalQuoteActivityAndFXCorrectionsInvalidateAndRecompute)$' -count=1 -v
go vet ./internal/application ./internal/domain ./internal/infrastructure/sqlite
go build ./internal/application ./internal/domain ./internal/infrastructure/sqlite
gofmt -l cmd internal
git diff --check
```

The SQLite package passed once in 76.507s. The final focused E1 application tests
passed in 0.524s. Selected existing correction/atomicity, populated-v15 upgrade,
backup source-epoch restore and continuous-backup upgrade tests passed. The
combined focused command initially failed only the new FX fixture because it
used an invalid leaf at the predicate root; grouping that leaf as required by
the existing rule contract fixed the fixture, and the final focused E1 run above
passed without weakening any financial expectation. Vet, relevant package build,
format and diff checks passed. The canonical `go build ./cmd/nestworth` was
attempted but cannot compile the Wails desktop dependencies without GTK4,
WebKitGTK6, GLib/GObject/GIO and libsoup development pkg-config files in this
Linux environment. No libraries were installed for this task. The entire
application suite, frontend checks, native Mac, race and perf suites were not run;
normal CI remains responsible for its unchanged full check set.

PR145 review follow-up: the synthetic Pacific/Apia origin 2011-12-29 / now
2012-01-01 fixture confirms off/on publication equivalence including the skipped
2011-12-30 unknown slot with empty leaves. Only December 29 and 31 receive shared
facts (four base/metric rows total). The same focused E1 application command above
passed all three tests in 0.561s after this correction; no broad suite or race was
repeated. Sampling skips only errors recognized by the existing domain
`ReviewNonexistentCivilDate` classifier.
