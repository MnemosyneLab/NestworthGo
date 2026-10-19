# C3 exact daily-publication equality

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
changed publication counters or explicit bounded experiment cannot select this
optimization. Counts or an unsigned/new checksum never grant eligibility.

Within that transaction, before the final clock fence, stream all persisted
daily input/result rows in the household, including their complete primary keys
and all scalar/serialized payload parameters used by the existing UPSERTs.
Retain private transaction-local byte images only for comparison at each actual
write. Compare the actual full stage's SQL parameters with type-tagged,
length-delimited bytes, including exact JSON, nullable values and selected FX.
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
