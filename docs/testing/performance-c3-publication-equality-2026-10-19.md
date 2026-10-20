# C3 exact daily-publication equality

Current implementation: `68f419422387960f40a42a6eba52d9dfdf913429`.
First encoded-row diagnostic: `483f4060cda6bf26e540d9b2d9505bd6836662cd`.
Production remains C2; legacy/restart authority stays hard-disabled. This stays
on the same draft PR129 and does not enable a new production shortcut.

## Proof before implementation

Keep complete financial capture and full preparation. A proposed omission is
only a publication decision about a single daily input or result UPSERT; no
stored financial fact becomes a computation input and no suffix is planned.

Eligibility requires the existing certified source publication check to succeed
under the writer reservation, including full compiled table/index/FK and trigger
contracts, source checkpoint, physical identity/counters, provider lease and
configuration/handling guards. Additionally require `MayCertify` and an already
valid same-handle certificate authenticating the captured complete row seal.
A fresh initial publication, legacy/restart namespace, unknown/damaged state,
changed publication counters or a staged bounded publication cannot select this
optimization. A bounded experiment that falls back to its complete full result
may use the same exact full-publication comparison. Counts or an unsigned/new
checksum never grant eligibility.

Within that transaction, before the final clock fence, stream all persisted
daily input/result rows in the household, including their complete primary keys
and all scalar/serialized payload parameters used by the existing UPSERTs.
Retain private transaction-local SQL value images only for comparison at each
actual write. Complete keys use type-tagged, length-delimited bytes. Compare the
actual full stage's parameters by exact type and value, including every JSON
string byte, nullable scalar and selected FX. Reuse immutable scanned strings
without re-encoding/copying complete payloads; copy the scan tuple and any BLOB.
There is no caller-provided skip stamp, prefix inference or cross-call cache.

Exact equality of all non-timestamp parameters is stronger than each existing
UPSERT's `DO UPDATE ... WHERE` predicate. It proves that executing that UPSERT
would affect zero rows. `computed_at` and `evaluated_at` insertion values are
excluded from comparison precisely because the unchanged UPSERT preserves the
stored timestamps; `revised_at` is likewise untouched. Mismatched or missing
rows execute the existing SQL without modification. The input/result decisions
are independent, so one may be omitted while the other changes.

Consume each stored image on its first comparison, even on mismatch. A repeated
key later in the stage must execute the original UPSERT: an earlier write may
have changed it, and suppressing a later restoration against an old image would
be incorrect. Complete keys distinguish household/rule/condition/instrument/
timezone/date/input-policy namespaces; a common date or fingerprint is not a key.

The compiled writer contract establishes no hidden daily-row trigger or FK
action that can make a zero-row UPSERT perform protected writes. The writer
reservation prevents concurrent row/schema edits. Known publication writes
cannot mutate another retained daily-row image. No epoch, episode, handling,
evaluation-state, control, checkpoint, affected-row CAS or certification work is
omitted. Existing atomic rollback and final minimum temporal frontier remain.
Zero-row UPSERT omission preserves `total_changes()` and commit-counter semantics.

This is a correctness argument to be checked by focused tests and measurement,
not an activation approval or an assumed performance win.

## First encoded-row diagnostic

The following results are frozen for the first implementation. It eliminated
the extra zero-row UPSERTs but encoded/copied each scanned JSON payload into a
second complete byte buffer. The follow-up retains immutable SQL strings in
private copied tuples and compares exact values directly, preserving this proof
and actual-argument comparison without that second payload representation.

All ordinary `./internal/...` tests pass (application 122.501s, SQLite 27.205s),
with internal vet/build. Focused certified/equality tests pass (application
17.364s, SQLite 0.763s). Tests cover every non-timestamp parameter, complete-key
dimensions, SQL NULL versus non-NULL, timestamp preservation, consumed images
after equality/mismatch, repeated-key restoration, actual arguments changed in
the final clock callback, and independent full durable output including all
three daily timestamps. Existing dual full/bounded corruption, source/config/
provider/floor/clock/rollback/atomicity and future-activation matrices remain.

Serial matched small n=3 medians (10 instruments / 1 rule / 30 days / 310 quotes /
100 synthetic cash activities), with all independent full/durable oracles passing:

| Scenario | C2 | C3 exact publication comparison |
|---|---:|---:|
| same-day | 30.727ms | 4.086ms |
| clock-only | 37.714ms | 44.854ms |
| append | 59.912ms | 50.796ms |
| correction | 59.783ms | 48.673ms |

Clock-only still calculates/presents 310 complete pair-days; its C3 SQL execution
count is now 43, with 33 affected rows. The remaining small time regression is
18.9%; SQL-call reduction is not itself an acceptance verdict. Prior frozen
measurements are not a controlled cross-commit comparison.

Reproduce with official Go 1.26.1, SQLite 3.53.0, Linux amd64, `GOMAXPROCS=4` and
the isolated cache/module/temp directories in the evidence manifest:

```sh
NW_C3_CERTIFIED_EVIDENCE=1 NW_C3_PERF_SIZE=small NW_REVIEW_PERF_SAMPLES=3 \
  go test -tags=perf -p=1 ./internal/application \
  -run '^TestReviewCertifiedPerformanceEvidence$' -count=1 -timeout=8m -v
```

Large uses `NW_C3_PERF_SIZE=large`, `NW_REVIEW_PERF_SAMPLES=1`, `-timeout=12m`.
The main harness independently checks full preparation before publication and
paired durable output after both arms. The candidate uses a freshly initialized
private-origin fixture; the C2 arm shares exact synthetic financial IDs/data.
No real household, credentials or live provider is used. No race rerun, timeout
increase, workflow edit/dispatch, activation, merge to main or release occurs.

## Final direct-parameter implementation

After the storage-only change, both complete affected package suites were rerun
on the final implementation: application 124.312s, SQLite 25.949s. Internal
vet/build pass again. All other internal packages passed in the earlier complete
`./internal/...` run on `483f406`; the final rerun is explicitly the two affected
packages, not another claim of an entire current-source internal invocation.

Final small n=3 medians, all independent oracles passing:

| Scenario | C2 | C3 exact parameter comparison |
|---|---:|---:|
| same-day | 45.882ms | 4.210ms |
| clock-only | 42.081ms | 51.266ms |
| append | 65.384ms | 59.848ms |
| correction | 55.547ms | 57.818ms |

Final large n=1 uses the same 100 instruments / 20 rules / 30 history days /
100,000 quotes / 50,000 synthetic cash activities. It selects only the changed
clock/source publication workloads; the successful warm no-op never enters this
helper and is not remeasured in the final large invocation.

Clock-only is 5.830995185s / 7.918899192s (C2 / C3, +35.8%). Both execute 8,242
SQL statements and affect 6,242 rows. C3 still calculates/presents all 62,000
pair-days; C2 presents 2,000. The former 120,000 zero-row UPSERTs are gone.
C3 now adds two comparison queries: 646 total / 654,382 returned rows versus
C2's 625 / 513,151. `TotalAlloc` is 3,523,489,824 / 2,009,055,288 bytes
(C3 / C2), cumulative allocation rather than RSS.

Append is 10.404101690s / 7.988814464s. Both affect 6,342 rows; C2 executes
128,242 statements and C3 8,242. The added scan/comparison is measured rather
than presumed free, and the total clock-only regression remains unresolved.

Correction is 9.691885487s / 7.703464283s. Both affect 156 rows; C2 executes
124,002 statements and C3 4,056. All final independent full/durable oracles pass;
the selected large package elapsed time is 283.266s. Whole-process VmHWM is
1,083,168kB, including fixture generation, both arms and independent oracles,
not an isolated per-arm peak. The tuple representation reduces total allocated
bytes in this diagnostic against the first encoded-row prototype; its n=1
elapsed time is not a demonstrated cross-commit throughput improvement.

| Final large / n=1 | C2 | C3 exact parameter comparison |
|---|---:|---:|
| clock-only | 5.830995185s | 7.918899192s |
| append | 10.404101690s | 7.988814464s |
| correction | 9.691885487s | 7.703464283s |

The [manifest](artifacts/c3-publication-equality-2026-10-19/validation.json)
records exact source/commands, prototype versus final validation and measurement
scope. [SHA-256 checksums](artifacts/c3-publication-equality-2026-10-19/SHA256SUMS)
cover the retained raw logs. No p95 or isolated per-arm RSS acceptance is claimed.

Final large reproduction:

```sh
NW_C3_CERTIFIED_EVIDENCE=1 NW_C3_PERF_SIZE=large NW_REVIEW_PERF_SAMPLES=1 \
  go test -tags=perf -p=1 ./internal/application \
  -run '^TestReviewCertifiedPerformanceEvidence$/^large$/^(clock-only|append|correction)$' \
  -count=1 -timeout=12m -v
```

## Remaining implementation decision and activation gates

Exact write elision has a proved local equivalence and ordinary full durable
oracles. It does not remove the overall C3 clock-only regression. The extra
conditional daily statements have been removed; the remaining statement count
matches C2. Complete preparation is still 62,000 pair-days versus C2's 2,000,
and exact comparison requires reading the stored parameter images in addition
to existing complete certification work. These measurements do not isolate
each CPU cost and do not justify attributing the entire gap to one component.

The present proof cannot omit comparison reads from a global seal or narrow
fingerprints: it needs actual complete row values to prove the existing UPSERT
would be a no-op. The decision for any further C3 work is whether a separately
reviewed publication strategy can reduce this cost while retaining complete
preparation and exact equality; it cannot silently reuse computed facts, infer
an unchanged prefix from counts/fingerprints, or skip the explicitly retained
epoch/episode/handling/control/checkpoint work. No larger planner or persistent
security protocol was added here.

Production stays C2. Independent review of this new proof/code, acceptable
matched distributions and isolated per-arm RSS, native Apple Silicon validation
and the previously incomplete race gate remain separate activation blockers.
Race was not rerun and its timeout was not changed. The known Linux native
GTK/WebKit/libsoup dependency gap remains; internal build is not a root-command
or native package build pass. No merge to main or release occurred.

## SQLite trust boundary

The existing [C3 contract](performance-c3-certified-2026-10-19.md#authority-and-fallback)
trusts verified C1 schema and SQLite engine/connection semantics, and explicitly
excludes an authenticated protocol against privileged schema/file forgery. C1
likewise states that raw unverified connections can bypass enforcement. Ordinary
DML/concurrent edits, schema DDL/cookie rewinds and unknown FK/trigger definitions
already have concrete conservative fences and regression coverage here.

Unsafe `PRAGMA writable_schema` manipulation that forges `sqlite_master` text
independently of SQLite's cached parsed schema is within that excluded privileged
engine/schema-forgery category. The proposed reviewer scenario has not been
executed and is not claimed protected by the text contract. This checkpoint does
not broaden the threat model, reconstruct a persistent security protocol or
present raw schema text as engine attestation. Existing checks are not relaxed.
