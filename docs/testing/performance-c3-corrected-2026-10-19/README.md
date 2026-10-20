# Corrected C3 evidence

Production source is merged PR123 `ab06a3f7ca342419948ae7a986cd8e5a73440536`;
the frozen measurement harness is `6b5f7b33a807b16247b20ab736adfbb913633413`.
No production code or CI changes are included in this refresh.

- `warm.log`:30 alternating warm invocation pairs per fixture, distinct session
  bootstrap/fresh-projection diagnostics, n=1 changed-source diagnostics, final
  paired output and full oracles. Primary latency/allocation values come here.
- `fallback.log`:3 independent clone pairs per fixture/scenario. Bootstrap and
  repair compare to forced-full; clock-only compares to C2 bounded. Every pair
  compares durable output and a full oracle. n=3 has no p95 estimate.
- `rss.log`:3 isolated process pairs per fixture,3 operations per child. Raw
  sampled peak/ready RSS, lifetime HWM, polls, SQL and child durations. Setup and
  full bootstrap precede explicit GC/decommit and the sampled region. Sampling
  ends before JSON/SQL reporting; child timings are not primary latency data.
  Per-pair durable comparisons/full oracles run in the parent afterward.
- `small-smoke-race.log`:passing110.722s validation of fallback/RSS harness and
  process handshakes. These race timings/RSS are not accepted measurements.
- `vet-perf.log`, `internal-build.log`:empty successful internal checks.
  `native-build.log` retains missing GTK/WebKit/GLib/libsoup failures.
- `main-ci-{run,checks}.json`:read-only automatic main CI success for the exact
  PR123 merge. No workflow was edited or dispatched.
- `chronology.log`, `status.log`:serial execution window and three zero exits.
- `source-commit.txt`, `provenance.json`:exact sources, per-file hashes, compiler,
  environment, workload counts, method boundaries and unrun native gates.
- `commands.sh`:Linux reproduction; select Go1.26 on PATH. It records each check
  status, including native failure, rather than asserting every gate is green.
- `summarize.py`, `summary.json`:standard-library parser and exact summaries.
- `format.log`, `whitespace.log`, `ci-preservation.log`:empty successful checks.
- `SHA256SUMS`:hashes every retained artifact except itself.

Warm results do not describe cold/native performance. The RSS window measures
post-setup residency after an explicit decommit, not natural whole-app memory;
the separate lifetime HWM includes bootstrap. Sampled Linux VmRSS is not an
exact native operation-only peak or a30-pair rollout gate. All data is synthetic.
Old pre-correction evidence is retained untouched in its original directory.
