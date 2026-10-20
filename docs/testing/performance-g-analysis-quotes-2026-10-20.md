# G: batched analysis quote loading

Baseline: `b4db6e44d39959565e46947d168ca8d012e8b67a` (main, Sonic PR #137),
verified by fetching `origin/main` in the saved Linux cloud environment.
Scope: performance plan G only. B remains off; no H, E, F or D changes.

## Contract

`AnalysisService.loadInputs` uses the optional `AnalysisQuoteBatchRepository`
port when available. Other adapters keep the existing per-instrument path,
which also provides the differential oracle. A batch error propagates; it does
not silently retry through the legacy path.

SQLite reads under one read-only transaction. It validates the household once,
then validates instrument ownership and loads quotes in chunks of at most 900
distinct IDs (901 parameters including the ownership query's household ID).
For I distinct instruments this is `ceil(I/900)` quote SELECTs plus
`1 + ceil(I/900)` validation SELECTs, rather than I quote SELECTs. An empty
request performs only the household validation. For one or two instruments,
validation can outweigh the saved round trips.

All dates, manual/provider/agent candidates, price/provenance fields and agent
withdrawal exclusion match `ListInstrumentQuotes`. Both paths use the existing
scanner. The batch groups candidates back into requested instrument order,
including repeated IDs, with `quoted_at DESC, created_at DESC, id DESC` within
each instrument. Empty histories stay empty. Unknown IDs or IDs outside the
household fail without returning a partial batch. Errors and cancellation
release the transaction; all chunks and ownership checks see the same read
snapshot. Other input-loading read boundaries are unchanged; this does not
claim a single transaction for the entire analysis.

The instrument universe still comes from the complete household portfolio,
including archived entities and cleared positions. There is no account scope
narrowing, date clipping, schema/index change, cache policy change or financial
algorithm change. H1's `AccountRecord`/`listAccountRecords` ownership/latest-value
helpers in `sqlite/repository.go` are untouched. JSON remains on the existing
`internal/jsoncodec`/Sonic implementation; no codec changes are needed here.

## Focused evidence

Synthetic temporary databases only; no providers, credentials or real ledger.

- `TestAnalysisQuoteBatchMatchesLegacyCandidates`: full candidate structs and
  order agree with legacy SQL, including ties, repeated/empty IDs, empty
  histories, years-old candidates, manual/provider records, provider metadata,
  withdrawn agent quotes, invalid household/unknown instrument and cancellation.
- `TestAnalysisQuoteBatchBoundsQueriesAndRetainsReadSnapshot`: 901 instruments
  produce **2 quote SELECTs, 5 total SELECTs**, maximum **901 parameters**.
  A second connection changes a later chunk's quote after the first chunk;
  the batch retains the original value and a subsequent read sees the write.
  Later-chunk failure/cancellation returns no partial prefix.
- `TestAnalysisQuoteBatchInputsResultsAndSQLCount`: 12 instruments, 48 quote
  candidates (including pre-origin and future provider candidates), two starting
  holdings in CNY/USD, manual FX, origin costs and a full disposal. Complete
  inputs and `ComputeAnalysis` results agree for origin/later ranges,
  household/account scopes, base valuation and CNY-filtered native valuation.
  Only fresh in-memory opening snapshot IDs/item IDs and creation time are
  normalized; financial values, content hash, quote IDs and provenance remain
  compared. Base results must be complete, avoiding an empty-result oracle.

The existing driver decorator counts **every SELECT through Rows.Close** in
`loadInputs`, including portfolio hydration, origin opening and snapshot quote
evidence. Thus this measures net query reduction, not a relocated loop:

| Range | Legacy total SELECTs | Batch total SELECTs | Quote SELECTs |
| --- | ---: | ---: | ---: |
| Aug 1–3, including origin opening | 86 | 77 | 12 → 1 |
| Aug 2–3, after origin | 44 | 35 | 12 → 1 |

Two small qualitative observations from the successful focused run on Linux
amd64 / Go 1.26.1, with durable daily snapshots prebuilt and no memo or snapshot
ensurer in the measured calls:

| Range | Legacy whole `loadInputs` | Batch whole `loadInputs` | Driver query consumption, legacy → batch |
| --- | ---: | ---: | ---: |
| Aug 1–3 | 16.961081 ms | 9.344950 ms | 16.381664 → 8.877072 ms |
| Aug 2–3 | 5.020377 ms | 5.005565 ms | 4.832491 → 4.150060 ms |

These are one old-then-new observation per range, with warm-cache/order bias,
SQL instrumentation and tiny synthetic histories. They are not a distribution,
p95, native macOS result, end-to-end page time or kernel speedup. No allocation,
RSS or large-scale latency claim is made. All candidates remain materialized;
chunking bounds parameters, not history size or peak memory.

## Validation

Executed successfully with `GOCACHE=/tmp/nestworth-g-cache` and
`GOPATH=/tmp/nestworth-g-path`, using `/tmp/go/bin/go` (Go 1.26.1):

```sh
go test ./internal/domain ./internal/infrastructure/sqlite -count=1
go test ./internal/application -run '^(TestAnalysis|TestReviewF0|TestReviewF1)' -count=1
go test ./internal/infrastructure/sqlite -run '^TestAnalysisQuoteBatch' -count=1 -v
go vet ./internal/application ./internal/infrastructure/sqlite ./internal/domain
go build ./internal/application ./internal/infrastructure/sqlite ./internal/domain
```

The focused batch test additionally ran verbosely to record the observations
above. `gofmt` and `git diff --check` were checked before commit.

`go build ./cmd/nestworth` was attempted and failed because the cloud image
lacks GTK4/WebKitGTK 6 development headers (`pkg-config` cannot find `gtk4` or
`webkitgtk-6.0`). Full `go test ./...`, full `go vet ./...`, frontend checks,
race, perf matrix, native macOS/package checks and live-provider checks were
not run. There are no wire/UI changes. Race remains owned by daily perf CI;
normal exact-head CI is monitored separately by the parent task. No merge or
deploy was performed.
