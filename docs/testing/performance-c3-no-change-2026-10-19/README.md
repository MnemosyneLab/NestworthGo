# C3a evidence index

- `provenance.json`: exact base/implementation/measurement/final source heads,
  toolchain, machine, isolation, validation scope and command exit codes.
- `commands.sh`: sequential final-source reproduction from the repository root;
  choose a Go1.26 compiler on PATH. Nonzero native checks are retained in the
  generated status file; this script does not turn them into passing gates.
- `performance.log`, `summary.json`, `summarize.py`: all120 warm invocation
  samples (60 pairs), separate n=1 workloads, full oracles and summary parser.
  Timings ran alone; no baseline production/default-window/native claim.
- `focused-unit.log`: original expanded-unit assertion failures from comparing
  row insertion order after reconstruction. Canonical-row sorting corrected
  that comparison; no financial/lifecycle field was dropped.
- `focused-unit-final.log`, `final-c3-unit.log`: passing focused C3 units before
  and after strict failed-tracking fallback. `initial-unit.log` is the earlier
  handoff-candidate run/toolchain module setup, not final evidence.
- `focused-race.log`, `overlap-race.log`, `fallback-race.log`: retained source/
  temporal/publication/C3 races, superseded-stage rejection, and final strict
  fallback coverage. `mcp-wails-race.log` covers both default and opt-in adapters.
- `full-unit.log`: full command with38 passing package results and the desktop
  native-dependency build failure. It is not a full green result.
- `internal-{vet,build}.log`, `final-internal-{vet,build}.log`: passing internal
  checks; success logs are empty. `full-vet.log`/`native-build.log` retain actual
  missing GTK/WebKit/GLib/libsoup failures.
- `main-sync-*.log`: integration checks after unchanged PR122/latest-main merge.
- `bootstrap-before.log`: reproduced incomplete-history recertification on
  PR123 head8bd8fae, including failed independent full oracles.
- `bootstrap-after-{unit,race}.log`, `bootstrap-parity-race.log`,
  `bootstrap-{vet,build}.log`: completeness bootstrap/expiry correction and
  full-rebuild recertification evidence. Prior performance logs are historical;
  corrected C3 fallbacks rebuild all dates and were not remeasured.
- `*-format.log`, `*-whitespace.log` (and initial names): empty successful
  formatting/whitespace checks. `ci-preservation.log` is an empty successful diff
  against latest main's workflows and CI scripts. No workflow was dispatched.
- `SHA256SUMS`: hashes for every retained artifact except this checksum file.

Validation runtimes are not performance samples. Original broad/focused jobs
overlapped; measurements began only after they exited. Final failed-tracking
fallback changes only paths rejected before valid confirmation; no timed valid
path changed. Later main changes are documented without relabelling earlier
logs. No full repository race run, native acceptance, frontend/bindings checks
or per-arm peak-RSS distribution is claimed. No real ledger/provider credentials
or native app was used. Candidate activation, merge and release remain separate.
