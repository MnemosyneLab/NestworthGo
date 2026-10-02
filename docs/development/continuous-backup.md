# Continuous backup and explicit recovery

This feature is being implemented for unreleased v0.3.6 / build 7. It backs up
one running machine; it is not cloud synchronization or a cross-machine writer
election. It is off by default and has no daemon after app exit. No live R2
credentials, household upload, or native Mac acceptance has been used here.

The backend embeds `github.com/benbjohnson/litestream` at v0.5.17 behind
`internal/infrastructure/continuousbackup`. The SQLite driver rises from
modernc v1.44.3 to v1.49.1 to match the library. The business schema remains 15;
no migrations are added. Litestream itself creates `_litestream_seq` and
`_litestream_lock` bookkeeping tables. Current Nestworth schema verification
still verifies all required tables, constraints, indexes, integrity and foreign
keys; it accepts these extra library tables.

`backup_config.db`, beside the business database, stores R2 configuration,
credentials, a stable random backup ID and status. It is a separate private
SQLite file and is never included in business backup, export or MCP. R2 keys
are plaintext locally, as authorized. Existing provider credentials and AI / MCP
configuration remain in the business database and are included in backups.
There is no new credential API in MCP or the portable Nestworth user skill.

A fresh random stream is created at each app start or reconfiguration, under
`nestworth/v1/<backup-id>/<stream-id>`. No two runs continue the same stream.
This deliberately avoids making an unproved continuation/ownership claim.
Each stream has a small `stream.json` containing application ID, source version/
build, schema and stream ID. It contains no credentials. Conditional creation
refuses an existing different identity. Listing/restore require this identity
and full candidate verification; unmarked prototype streams cannot be restored
through the UI. The record does not represent remote backup success. A separate target-scoped
local stream ledger records ownership before the worker starts. It records a
seal only after the worker is joined, the application SQLite connection is
drained, a final snapshot is remotely confirmed, and Litestream closes
successfully. The optional sealing attempt has a 500 ms budget; failure or
cancellation leaves the stream unsealed and protected. Crash-abandoned open
rows are never promoted on restart. Sealing evidence does not change the
global last-successful-backup status. R2
conditional creation is supported by the [S3 API](https://developers.cloudflare.com/r2/api/s3/api/).
An OS instance lock is acquired before startup journal reconciliation or opening
SQLite. It fences duplicate processes using the same database path locally.
The lock is released by the OS after a crash; its file can remain.

The app owns one serial replication worker. Library DB/replica monitors,
compaction and retention monitors are disabled. Every scheduled/manual attempt
uses `DB.SyncAndWait` and a remote `DB.SyncStatus` confirmation. Status writes
only touch the separate configuration DB. The last attempt and last successful
backup are distinct. Equality of captured LTX positions does not prove that
later business writes have been scanned. A connection test lists the bucket;
it does not write an object, prove upload permission, or set a successful backup
time. Backend/provider errors are replaced with generic summaries, without
credentials, responses, SQL or paths.

Lazy Litestream initialization also runs inside the drain adapter: its remote
L0 query occurs after acquiring a read lock, and initialization errors close
SQLite descriptors. The initial `DB.Sync` and all failure cleanup finish while
application queries are queued; retries use freshly reset library handles.
After initialization succeeds, ordinary incremental sync does not reserve the
application pool slot. Initialization can briefly delay local queries, up to
the operation timeout; there is no callback that borrows that slot recursively.

Disable and target changes first cancel cleanup and join the replication worker. Litestream must
not close POSIX descriptors while app SQLite connections exist. The narrow
SQLite adapter reserves the sole pool slot, waits for active rows/transactions,
closes the pinned modernc driver handle, then closes the Litestream store. New
queries remain queued until `driver.ErrBadConn` releases the slot for a fresh
connection. The sql.DB and repository references stay stable. The adapter
relies on the pinned driver's idempotent Close and the pool's one-connection
limit; both are covered by tests. It never returns while a timed-out background
close continues touching SQLite. A failed drain during configuration change
or restore pause retains the old configuration, resumes its worker, and shows
retrying instead of a stale confirmation. Shutdown does not resume a worker.

Shutdown first permanently fences/drains application write permits, cancels
and joins market-data workers, then closes/drains MCP. This uses the central
write coordinator without holding `changeMu` or the SQLite pool slot while
joining workers; it also works after restore retained its exclusive permit.
Backup shutdown stops its worker, attempts final remote confirmation with the shutdown
context, then performs synchronous cleanup with a separate five-second drain
context. Network operations use ten-second contexts. This is bounded for the
context-aware file/S3 clients; a backend that ignores cancellation can block
shutdown until it returns. A separate hanging-backend probe reproduces that
interface limitation. Production has no configurable custom backend or insecure
TLS mode. R2 uses a validated account ID to construct its HTTPS endpoint,
region `auto`, explicit static credentials and normal TLS verification.

Cloud inspection downloads to a private temporary directory, validates the
full current schema nondestructively, and creates a temporary local package
for the existing token-based restore preview. Empty/not-found and incompatible
candidates are errors. Installation requires typed `RESTORE` plus acknowledgment
and reuses the existing exclusive-write, checkpoint, close, safety-copy, atomic
swap, rollback and startup-journal recovery path. Both local and cloud restore
pause replication and persist backup disabled before installation. Restart and
explicit re-enable produce a fresh isolated stream. Failed operations never
turn into an automatic restore of a valid local database.

## Optional history retention (phase one)

Cleanup is **off by default**, preserving permanent remote retention. Settings →
Continuous backup → History retention offers 30 or 90 days. Enabling cleanup,
changing its enabled retention period, and executing a manual cleanup require
an explicit irreversible-deletion acknowledgment. Turning cleanup off never
deletes anything. A preview is read-only and works while cleanup is off.

Ownership lives only in `backup_config.db`, scoped to the normalized R2 account,
bucket, stable BackupID and exact stream UUID. Existing remote identities are
not adopted into the ledger. Legacy, foreign, unknown and crash-abandoned
streams remain protected. The current stream and the newest two sealed streams
are protected. On each scan those two streams must actually restore to private
temporary databases and pass the existing schema-15, integrity and foreign-key
verification. If either fails, cleanup is skipped; an older unverified stream
is never substituted. Temporary verification files are removed on completion,
failure or cancellation. Verification honors cancellation without weakening
schema checks.

Age is based on the locally recorded final confirmed backup/sealing time, not
`StartedAt`, remote object ages or global last-successful-backup status. Only
whole expired sealed streams are eligible; individual L0 files are never aged
out. There is no compaction, calendar thinning or periodic stream rotation.
A long-running current stream has **no hard storage bound**. Protected and
unsealed streams can also accumulate indefinitely.

Retention enumerates every object page under the exact configured BackupID,
with no delimiter, stream cap or recovery-point cap. It accepts only canonical
stream UUID paths, `stream.json`, and phase-one L0 filenames. Unexpected paths,
duplicate keys, missing/changed owned identities or changed target inventories
abort the operation. Other BackupIDs are outside cleanup's listing and deletion
scope. The recovery UI still limits discovery to 100 recent L0 points per stream
and 512 streams; those display limits are not used by cleanup.

The preview shows scanned and eligible bytes, scan time, eligible stream IDs
and protection reasons. Manual preview tokens are single-use and expire after
15 minutes; automatic cleanup defers while a preview is unexpired. Execution repeats inventory, identity, actual survivor restoration
and protection checks; changed configurations, survivor sets, candidate sets or
inventories require a new preview. Application restore, target changes and
cleanup share the manager operation lock. A successfully staged recovery
candidate pins its source stream for the rest of the app session, including
when the user closes the preview; this deliberately favors preservation.

Before removing any object, the control database durably records `deleting`
and the exact key/size/ETag manifest. Recovery discovery and staging reject
local deleting/deleted streams, including stale recovery-point requests. Each
successful deletion is checkpointed. A lost response or process interruption
can leave a partial stream; retries use the persisted manifest and only accept
an unchanged subset of its objects. Data objects go first, `stream.json` last.
The final inventory must confirm absence before the ledger becomes `deleted`.
Unknown/new/changed objects stop resumption. Completed tombstones retain
ownership information, but discard the no-longer-needed object manifest.

R2 requests use the configured static credentials and exact bucket/key.
Deletion first checks size and ETag with conditional HEAD and sends `If-Match`
on DELETE; failures never cause a retry without the precondition. Cloudflare's
[S3 compatibility table](https://developers.cloudflare.com/r2/api/s3/api/)
documents conditional HEAD, but does not explicitly document DELETE conditional
headers. Isolated live acceptance must establish their behavior; this code does
not claim to provide remote writer fencing. No other app, lifecycle rule or
external writer should modify locally owned stream prefixes during cleanup.
The control ledger must not be cloned to a second active writer. Live R2
behavior and deletion permissions were not tested with real credentials.

Background work starts on a timer one minute after manager startup, outside the
startup critical path. It checks eligibility hourly, with at most one attempt
per 24 hours persisted **before** network I/O, including failed attempts. Safe
resumption occurs on a later daily attempt or an explicitly confirmed manual
run. Operations have a five-minute budget. Configuration changes, restore and
shutdown cancel cleanup before acquiring the operation lock; shutdown cancels the
scheduler, serializes control-database closure, and waits for scheduler exit
before returning. There is no daemon when the app
is closed. Cleanup results, errors and timestamps are stored separately from
backup status; cleanup never creates a successful-backup claim.

Only synthetic file storage and fake S3 request interfaces are used by automated
tests. Coverage includes full pagination, current/two-survivor/legacy/foreign/
crash protections, post-cleanup restoration of retained data, denied deletes,
partial outcomes and lost checkpoints, restart/resumption, stale preview and
concurrent configuration/restore/shutdown, redacted errors, and repeated,
interrupted, keyboard-only and localized UI flows.

## R2 setup and later acceptance

The user creates a dedicated bucket and bucket-scoped Object Read & Write API
credentials. Keep those credentials out of source control, diagnostics,
telemetry, browser storage and screenshots. Account ID and bucket name are
nonsecret; the configured credential fields expose only fixed masks. Replace
both R2 keys together, or explicitly remove the pair with backup disabled.
Blank inputs keep the stored pair. Save and test the configuration before
opting into backup. A successful test confirms listing, while the first
successful backup confirms upload.

A real R2 test requires explicit user credentials and an isolated safe data
source. Native Mac acceptance must cover enable/disable, network interruption,
restart, target replacement, backup status, cloud recovery previews, cancellation,
confirmation, safety-copy retention and re-enable after restore. These are later
gates; local file-store tests and CI cannot establish live R2 or native UI behavior.
