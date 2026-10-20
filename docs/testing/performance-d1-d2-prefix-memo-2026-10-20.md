# Private prefix memo experiment: qualitative result

Production candidate: `ac46204` (full SHA in artifact manifest). Validation head:
`032b7e85fe5065f275e95ee42c5decf1b440973f`. This is the same PR131 branch,
preserving1359 and user merges. The memo is **opt-in and not enabled in normal
initialization**. Existing D2 enablement/rollback is unchanged. No cumulative-wave
comparison was run here; that remains with Luna.

The [bounded contract](../design/review-prefix-memo-experiment.md) was written
before implementation. The memo stores immutable lossless typed-evidence
encodings, including exact FX, not SQL tuples or caller-owned objects. Full
financial capture/fingerprint, full-origin policy/position/lifecycle replay,
final typed-prefix authentication, exact retained/emitted partition, source/
control/checkpoint CAS, provider lease, physical schema/trigger/FK/counters,
temporal/final-clock checks and full post-publication row sealing remain.

Only canonical22-column-reader attestations seed it. Private candidates are built
before capture exposure. Successful certified commit can rebind only the matching
private candidate; no newly emitted UPSERT is treated as equality evidence. The
next clock reads/attests the uncovered tail. Certified noop writes a checkpoint,
so its successful commit rebinds only existing content, adding no entries. Both
capture and postcommit actions are generation-fenced; a stale read/hook cannot
replace newer state. No mutex spans SQLite commit. Cache failure/eviction has no
certification authority and cannot repair legacy/restart/restore provenance.

One committed/one pending snapshot, each at most65536 entries and32MiB encoded
payload, plus bounded metadata. This is an eviction limit, not an RSS bound or
routing threshold. Reuse authenticates private content/MAC, then decodes fresh
owned public values: **all prefix entries are still decoded**. This is not
O(new-days) total reconciliation, a lifecycle checkpoint or E/shared facts.

## Evidence and limitations

Final small and medium probes each use n=3 matched journeys, serial one-arm
processes on the same saved unpublished synthetic facts/UUIDs, fresh independent
genesis authority, same binary/toolchain. Linux amd64, Go1.26.1, SQLite3.53.0,
GOMAXPROCS4/four-CPU quota. Baseline selects observation-only no-store/no-reuse;
candidate selects the memo. These are explicit D2 configurations, not shipped
currentmain defaults. Each journey measures cold fill, noop, and two subsequent
clocks; setup/noop/oracles are outside the clock timings. Initial/superseded
probes are retained. Ten matched journeys total including those earlier probes;
no further sampling is needed for this qualitative conclusion.

| Median elapsed, ms | Small baseline / memo | Medium baseline / memo |
|---|---:|---:|
| Cold fill | 64.034 /64.594 | 432.218 /439.323 |
| Warm clock1 | 68.647 /57.253 | 440.743 /383.859 |
| Warm clock2 | 64.838 /61.199 | 443.320 /365.629 |

Small:10 instruments,1rule,30 historical days,310 quotes,100 activities. Medium:
50 instruments,3rules,30days,1550 quotes,500 activities. Warm medium avoids SQL
decode/canonical argument encoding for4500 then4650 cached entries, reading and
attesting150 tail rows per clock instead of4650/4800. Total typed decodes remain
4650/4800. Small reads/attests10 tail rows instead of310/320. All counts are from
the actual reconcile; final SQL statistics exclude the subsequent oracle reads.

Medium warm allocation medians129.594→124.637MB and132.215→126.654MB;
allocation counts1,506,899→1,100,755 and1,544,129→1,124,943. Cold allocation
127.882→143.199MB. Medium writer-reservation medians are176.110→194.039ms
(warm1, worse) and190.133→178.631ms (warm2). This does not establish a writer-
reservation improvement; capture savings and overall timings must stay separate.

Memory tradeoff is material: after two GC and FreeOSMemory outside timing,
medium retained Go heap is1.488→5.807MB and1.491→5.941MB on warm clocks.
Single-arm process VmRSS medians are42564→56196kB and42776→57204kB. Small
retained heap rises about0.3MB, RSS about1.4–1.5MB. These are isolated single-arm
process point measurements, not peak RSS, pure memo attribution or native Mac QA.
No p95, broad performance acceptance or asymptotic improvement is claimed.

Qualitative result: warm SQL/encode work and measured warm elapsed improve, at
cold-fill/allocation/retained-memory cost. The costs do not erase the measured
warm benefit in these two fixtures, but **default rollout remains rejected/
unaccepted** pending representative/native tradeoff review. The experiment stays
opt-in. It does not establish gains for other sizes, sources or invalidation
rates; the bounded cache may decline larger prefixes entirely.

## Correctness and executor recovery

Independent full-planner/durable-output/all-three-timestamp oracles passed for
all measured calls. Light default tests cover cold/warm/noop/next-clock, public
pointer mutation isolation, missing/unattested coverage, and stale generation
callbacks. The local opt-in matrix covers cancellation/CAS failure/stale
promotion, eviction, source/provider/rule/handling change, warm acknowledgements,
suppression expiry/frozen evidence, missing SQL rows, clock rollback, reopen,
exact-FX alias isolation, future activation without journal change, and memory/
entry bounds. Final local matrix4.526s application; focused ordinary checks
.426s application/.008s SQLite; broader proof-focused suites24.608/.816s passed.
Internal vet/build passed. No race run or workflow edit/manual dispatch occurred.
Actual final normal-check duration/per-test cost is recorded in the PR.

An executor disk limit caused Go linking to fail with ENOSPC. No old executable
was used as the new candidate. No Go build/benchmark was running when this task's
6GB build cache was cleared; artifacts/source were preserved. The affected builds
and checks then succeeded. The linker blocker is resolved. The parent's earlier
reduced-sampling steering around05:06 was not delivered here; the first delivered
notice arrived after these probes. No more experiments/builds were started after
that notice. Only evidence persistence and existing CI observation remained.

[Manifest, raw logs, samples and SHA256SUMS](artifacts/d1-d2-prefix-memo-2026-10-20/).
No release, merge, real ledger/settings/provider credentials or native UI launch.
Native QA must retain com.nestworth.dev / Nestworth.app and synthetic paths.
