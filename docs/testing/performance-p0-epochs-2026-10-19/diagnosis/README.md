# PR111 diagnostic evidence

See the [diagnosis](../../performance-p0-epochs-diagnosis-2026-10-19.md).

- `stages-epochs64.log`: instrumented n=30 paired no-op experiment at `3007340b97a3a56743578de2424920a6d4baf24a`, with complete epoch/full/durable oracle gates.
- `stage-samples.csv`: individual measured call/substage counters; `_ns` columns are nanoseconds.
- `stage-distributions.csv`: nearest-rank summaries; time metrics use `_ms` names and milliseconds.
- `original-noop-samples.csv`: extraction from the unchanged original `../after-real-epochs64.log`.
- `controls.json`: identical lifecycle-source SHA-256 at all relevant commits.
- `publication-regression.log`, `publication-race.log`: final direct assertions for publication, stable audit identities/timestamps, corrected old trigger vote and legitimate recovery.
- `diagnostic-gates.log`, `targeted-race.log`: selected full/durable/repeated-epoch gate results before the final correction-date strengthening; the final direct regression is verified separately above.
- `extract-original.py` and `extract-stages.py`: standard-library extraction/recomputation of the committed CSVs. Run them in place; the original raw log is read from the parent directory.
- `SHA256SUMS`: raw evidence, extraction scripts and control hashes. No databases or real financial/provider/credential data are included.

The instrumented results do not replace or clear the original p95 activation block. Production code/defaults remain unchanged. Parent handles remote CI.
