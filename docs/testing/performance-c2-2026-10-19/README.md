# C2a evidence

See [stage report](../performance-c2-2026-10-19.md). Production and test implementation is fixed at `5ababa4028fdd771d1edde95f086d2cd150604ec`, based on merged main176bfb0. Later documentation does not modify the implementation.

`focused-unit.log`: initial source checkpoint, strict publication, temporal/floor/epoch/readonly and retained publication/oracle checks. `review-unit.log`: broader review/domain/SQLite plus JSON export checks. `focused-race.log`: second-connection writer-reservation check, negative control, publication concurrency/atomicity and full-recompute/lifecycle oracle under instrumentation. `domain-final-race.log`: final household namespace/floor check. `vet.log`/`build.log`/`format.log` record backend checks. All use isolated synthetic data; no native app/provider/cloud-backup access.

C1 failed-CI diagnosis and base/current controlled workload samples are in the separate [CI evidence directory](../performance-c1-ci-2026-10-19/). These are not C2 latency measurements and do not establish a canonical successful CI run. No workflow or shard selector is modified here.
