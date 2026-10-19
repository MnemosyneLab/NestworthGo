# PR114 conflict/retention follow-up evidence

See [report](../performance-c1-insert-fix-2026-10-19.md) and [current source coverage contract](../performance-c1-source-coverage-2026-10-19.md).

Measured production/test code is fixed at `0a7c3061adbd12d8bec5246c261388c28659cfbe`. Later changes add documentation and an extra stale-floor oracle test only. No initial PR114 artifacts in `performance-c1-2026-10-19/` were overwritten.

- `before-conflicts.log`: real-schema reproductions against old BEFORE INSERT definitions, deliberately failing expected revision counts/timing verification.
- `after-conflicts.log`, `conflicts-retention.log`: passing conflict/retention, every-column matrix, migration and verifier checks after correction.
- `focused-unit.log`: first expanded compile attempt, containing two test-harness compilation mistakes corrected before committing/measuring; `focused-app-restore.log` and final broad logs supersede it. Preserve failures accurately.
- `write-amplification.log`: unchanged paired n30 harness,120 samples, fresh cloned connections, alternating order. `extract-writes.py` rebuilds CSV/summary. These distributions do not establish production p95/cold/warm SLA.
- `provider-import.log`, `provider-refresh.log`: single complete production-call samples using synthetic provider fixtures/fakes, serially measured before validation. Source fingerprints/DB verification are outside timed intervals. Import retries exercise actual unchanged UPSERTs; refresh cached status still involves fake provider calls.
- `focused-race.log`: conflict, retention, every-column, concurrent writers, migration, restore and application oracle tests under detector. `stale-oracle-race.log` adds full fingerprint/recompute behavior below the floor.
- `unit-internal.log`: complete backend run. Any timing-gate failures are preserved; isolated reruns use the original gates/budgets and are recorded separately.
- `vet-internal.log`, `build-internal.log`, `format.log`: backend vet/build/format. Canonical native build/bindings limitations remain documented in the initial PR114 evidence; there are no frontend/service/DTO/generated-artifact changes in this follow-up.

Checksums cover all raw files after validation completion. This directory contains no real ledger, credential or live provider/cloud-backup material.
