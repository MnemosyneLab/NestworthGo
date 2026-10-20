# H2a: conservative analysis quote dependencies

Baseline: `1615057a532a9577bf53071bb00254e30fa8dfc2` (G #139, H1 #140).
Default-enabled only for explicit account scope. The pure planner retains all
instruments in that account's captured historical/opening snapshots and all its
portfolio holdings, including zero positions. It also retains **every household
activity instrument**, trade/dividend/product context and both normalized/raw
holding effects. This deliberately over-approximates transfers, correction chains
and cost origins: classification precedes output filtering. It is not a complete
H2 account hydration closure and does not promise activity-heavy ledger savings.

All portfolio records, snapshots, activities, FX, source evidence and snapshot
maintenance remain full. The opening basis is constructed by the original full
historical loader before planning; its quote reads are not reduced. G's batch
implementation and candidate selection are unchanged, as are legacy loading,
memo size, source attribution, completeness and calculation rules.

Non-account scope, absent selected account, unknown activity kinds, unresolved
activity accounts/holdings or required instruments, and any snapshot with absent
historical account metadata retain the original instrument list. In particular,
clipping portfolio accounts while preserving household snapshots would mark days
incomplete in `resolveAnalysisUniverse`; this implementation never clips either.

## Synthetic evidence and limits

Linux amd64, Go 1.26.0. No real ledger, native Mac or deployed evidence.
`TestAnalysisQuoteBatchInputsResultsAndSQLCount` exercises actual service loading
through SQLite and G's batch: 12 requested instruments / 48 returned quote rows
become 2 / 8 for account scope, with both origin opening and later opening dates.
The repository wrapper records actual requested IDs and returned rows. Full
captured inputs are shared by the two computation arms; complete results are
compared, including coverage, missing states, provenance and attribution.

`TestAnalysisQuoteDependenciesEquivalence` includes 40 unrelated accounts with
holdings and historical snapshots: requested IDs and retained rows fall 41 to 1;
with the selected quote absent, rows fall 40 to 0 and complete results still match.
Transfer-in/out, sold, reversal/replacement and cost-effect cases compare complete
results for both ordinary and investment calculation. Unknown historical metadata
has an explicit full fallback regression. Synthetic correction tests compare the
captured-input kernel; they do not introduce a new ledger correction workflow.

These small checks demonstrate less quote hydration, not global latency, p95,
allocation or RSS gains. Unrelated snapshot/portfolio/FX loading and full origin
construction remain costs; retaining all activity instruments limits savings.

Validation: focused `go test ./internal/application -run TestAnalysis -count=1`;
application and SQLite package tests; application vet; gofmt and diff checks.
Race belongs to daily perf and was not run. No native build or broad dependency
installation. Exact head and remaining checks are recorded in the draft PR.
