# Bounded capture/equality SQL work and memo rollout

Before: `76b63ebb9365aacff8403ab08e8ecb23c9c062d7`.
After production/test commit: `a9c0dd8a1acfac7dc42ba5c215e5f2865e55ece1`.
Same branch and draft PR131, preserving all previous work and user merges.
Previous evidence-head CI39375100284 passed; it does not validate this new commit.

## Changes and safety

The six capture projection COUNTs now run only when initial-publication absence
could matter, or inside the separately gated bootstrap branch. Established
chains skip them. Initial eligibility still requires the fresh-handle genesis
connection, no previous certificate/fingerprint, an unissued key, six empty
projections and the existing receipt/legacy absence check. Bootstrap provenance
remains disabled; no restored/restarted/legacy authority is repaired. Confirmation
and post-publication counts are unchanged.

The equality reader now uses requested rule/version/instrument/timezone/policy
and exact date ranges, rather than every household row since a global minimum.
Repeated keys are deduplicated, sorted dates become disjoint consecutive ranges,
and date gaps remain gaps. Batches contain at most150 ranges:751 input or901
result parameters, below999. Empty requests issue no equality queries. CROSS JOIN
keeps requests outside the indexed row lookup; EXPLAIN shows existing primary-key
range searches. No schema/index change. Results remain private maps, and the
publication/UPSERT order is unchanged. All13 input and9 result columns retain the
same *any scan, SQL types, NULL and complete JSON/exact-FX byte comparison.
Repeated comparisons still consume images once. Missing images retain ordinary
UPSERT. The complete unique retained/emitted partition, source/control/checkpoint
CAS, provider lease/time fences, full source capture, origin replay and integrity
seals remain unchanged.

The user explicitly approved enabling the bounded prefix memo. GUI initialization
now uses `WithDefaultReviewOptimizations()`; MCP receives the same application
service/repository (`mcpserver.New(service, ...)`), so both use the same setting.
`NESTWORTH_REVIEW_PREFIX_MEMO_DISABLE=1` disables only memo storage/reuse, retaining
D1/D2. `NESTWORTH_REVIEW_FULL_RECOMPUTE=1` takes precedence and selects the existing
full path. Neither changes persisted settings. Direct NewRepository retains its
baseline configuration; explicit WithReviewPrefixMemo(false) is available.

The existing one committed/one pending snapshot limits remain65536 entries and
32MiB encoded payload per snapshot (at most64MiB plus bounded metadata across
both), not an RSS cap. Authentication, generation fences, ownership, invalidation
and cold fallback are unchanged. The earlier matched memo experiment found medium
warm improvements12.9–17.5%, cold fill+1.6%, retained heap+4.3–4.5MB and process
RSS+13.6–14.4MB. Default enablement is a user-authorized tradeoff, **not global
D1/D2 performance acceptance**, p95/native acceptance or O(new-days) computation.

## Minimal qualitative measurements

Linux amd64, Go1.26.1, SQLite3.53.0, GOMAXPROCS4/four-CPU quota. Serial matched
synthetic sources/UUIDs and independent fresh-genesis authority. Existing perf-only
harnesses; no heavy default test or broad matrix added. Before executable is the
existing final memo binary from ac46204 production content; ac46204..76b63eb changes
only docs/evidence/local guard tests, not production or these measured harnesses.
Binary/source hashes are in the manifest. After executable matches a9c0dd8 source.

One before/final medium clock journey (50 instruments,3rules,30days) with memo on
both sides isolates the SQL change. Setup, noop and independent full/durable/all-
three-timestamp oracles are outside measured reconciliation. SQL counters are
sampled before oracle reads.

| Transition | Before→after elapsed, ms | Queries | Returned rows |
|---|---:|---:|---:|
| Cold fill |464.882→448.882|191→185|21912→21906|
| Warm1 |365.378→375.843|191→185|17862→17856|
| Warm2 |400.187→379.408|191→185|18162→18156|

Six COUNT queries/rows are avoided in each measured reconciliation. This current-
namespace clock fixture has no unrelated historical equality images to discard.

One small historical-correction sample (10 instruments,1rule,30days), explicit
D2 without memo on both sides:66.362→70.006ms,163→157queries,4718→4712returned
rows,24write statements/14affected rows unchanged. The existing harness also ran
one bounded-C2 control per invocation; that control is not a shipped-default
comparison. Source-changed correction emits the full current namespace, so it
does not demonstrate equality row reduction. Allocated bytes rose19.497→20.822MB;
scoped request/query construction has overhead when every namespace is requested.

The focused ordinary scoped-reader fixture demonstrates the actual row filtering:
old global-minimum reads161input+161result images; new reads154+154, excluding
unrequested versions, instrument, timezone, rule and date gaps. The fixture spans
two chunks, repeats scopes/dates, includes absent dates and a second household,
and compares complete expected row images. It is correctness evidence, not a
large-ledger throughput or native memory claim.

The first candidate used ordinary JOIN/100-range batches and showed slower clock
publication despite fewer returned rows (484.247/427.528/445.964ms). Its logs are
retained, not discarded. Inspection led to CROSS JOIN/indexed request lookups and
150-range batches; the final single sample above followed that material change.
One initial invocation used an incorrect source-directory env name and generated
unmatched synthetic facts; its log is retained but excluded. No further sampling.

Conclusion: six redundant capture queries are removed and equality reads exclude
unrequested namespaces/dates. End-to-end timings are mixed; **no general latency
improvement is established**. This limited evidence is sufficient for the bounded
qualitative checkpoint, not a performance acceptance gate.

## Validation and limits

Focused SQLite comparison/default/rollback/memo tests pass(.032s), including every
column and NULL/type differences, duplicate consumption, empty/missing/gapped/
multichunk requests and query plans. Focused application full/durable-oracle,
legacy/restart/deleted-history/authority and default memo checks pass(6.344s).
After the final query-plan adjustment, equality and default-memo application tests
pass again(1.521s); all measured full/durable/timestamp oracles pass. Scoped vet and
`go build ./internal/...` exit0. No race run, workflow edit or manual CI dispatch.

No native Mac run; Linux still lacks GTK/WebKit/libsoup prerequisites for the
GUI build. No native/p95/global stage acceptance, release, merge or real ledger/
settings/provider operation. Existing provenance restrictions and rollback remain.
[Raw logs, manifest, samples and checksums](artifacts/d1-d2-bounded-sql-2026-10-20/).
