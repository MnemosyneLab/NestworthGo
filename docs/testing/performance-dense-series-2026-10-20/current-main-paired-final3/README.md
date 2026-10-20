# Source-verified paired run

Baseline binary: current main `47c31ca88f1527f550e06f0c43d8f8854f3c0b08`.
Candidate binary: `f2fee931fa3d5041311468534ccb196332696067`.
The per-path raw logs and all 14 complete alternating pairs are retained here.
Only samples 1–10 per path in `analyzed-10-pairs.json` feed the reported
statistics; samples 11–14 are additional completed observations excluded from
the summary after the user capped the run at ten repetitions. The original
requested 30-pair loop was stopped before completion.

`fixture.log` records fixture creation using the baseline test binary. It was
reused read-only by both binaries. The parent folder's `provenance.json` records
source, binary, fixture and host identities. `SHA256SUMS` covers this folder's
evidence files.
