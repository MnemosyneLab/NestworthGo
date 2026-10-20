# D1/D2 follow-up: enabled path, measured overhead and remaining decision

Production head measured: `577d00388504638bdb953826e3720a036880442e`.
The final evidence commit adds only tests and documentation. The user's main
merge `94538f775e946564fdfdddde247cbe2f9e5382ff` is retained as an ancestor.
All runs use isolated synthetic data. No real settings, credentials or ledgers
were used. PR131 remains a draft, unmerged and **not performance accepted**.

## Changes supported by profiles

The CPU/alloc harness is opt-in under the existing `perf` build tag. It wraps
one actual reconcile, excluding setup and independent oracles. The existing
repository phase wrapper reports confirm/capture/publish elapsed time; residual
includes planning, orchestration, locks and provider work, not pure preparation.
CPU cumulative stacks overlap and must not be summed as independent durations.

The original private-scalar walker boxed large composite values simply to test
whether they represented financial scalars. It now checks immutable Go type
traits first, cached by `reflect.Type`, preserving the exact old stream. The
first uncached-trait experiment reduced allocations but increased clock elapsed
to 18.046s; it is retained as negative evidence. No financial cache was added.
Suffix hashing now streams the exact original top-level JSON representation
instead of allocating the whole prefix JSON. Independent original-walker and
original-JSON oracles verify bytes/digests, including private FX rates, nil/empty
collections and escaped strings. Future incompatible root shape uses the generic
encoding. The prefix lookup stores indices into the existing authenticated
slice rather than duplicate evidence structs.

Within one verified coherent capture, unchanged exact historical fingerprint
and unchanged exact source token permit reuse of the previous **authenticated**
nonquote/quote descriptions. Changed sources compute and compare nonquote and
quote descriptions once, independently checking actual changed quote buckets
against the contiguous journal. The descriptions carried to signing are bound
by the existing private source MAC and checked again before writes. Full
historical capture, physical counters, exact schema/trigger contracts, protected
row seals, source/control/checkpoint CAS, provider lease, temporal fences and
transaction rollback remain. This does not authorize legacy/restarted/restored
history or synthesize acknowledgement history.

The reviewer candidate concerning provider replacement did not reproduce using
the supported setter: it invalidates the clock epoch before capture, chooses
full authenticated preparation and preserves the certified chain. A regression
checks the persisted MAC/provider, next noop and next clock suffix against the
full durable oracle. No production rebinding mechanism was added.

## Paired diagnostics

Linux amd64, Go 1.26.1, SQLite 3.53.0, four-CPU quota, GOMAXPROCS=4. Each
sample freshly creates synthetic data and clones paired arms. Large n=1 is a
diagnostic, not p95 or a controlled cross-commit acceptance experiment.

| Final large clock-only, unprofiled n=1 | Full C2 | Enabled D2 |
|---|---:|---:|
| Elapsed | 5.825027363s | 7.960744014s |
| Total allocated bytes | 1,682,139,576 | 2,389,760,368 |
| Queries | 625 | 667 |
| Rows returned | 513,151 | 714,382 |
| Executions / affected rows | 8,242 / 6,242 | 8,242 / 6,242 |
| Write-reservation elapsed | 3.057534801s | 3.606589613s |

D2 remains **36.7% slower** than its paired full arm. Compared descriptively
with the earlier 10.241296187s / 4,060,713,464-byte D2 diagnostic, elapsed is
22.3% lower and cumulative allocation 41.1% lower. These separate runs do not
prove a controlled source-identical cross-commit improvement. Every staged/full
and durable-output oracle passed; clock recomputes 2,000 pair-days and reuses
60,000, while policy/lifecycle and complete publication still span history.

Final profiled D2 is 7.977231608s / 2,392,460,992 bytes. Disjoint wall phases
are capture 4.119322015s, confirm .036287679s, publish 3.550138970s and residual
.271482944s. CPU cumulative samples include prefix capture 1.60s, daily
publication 1.74s, prefix decode .86s, signer .78s (row seal .76s), SQL equality
read .64s, source fingerprint .65s and suffix digest .63s. These overlap.
Quote-day seal construction is absent from this unchanged-clock profile.
Sampled allocation profiles can lag GC cycles; TotalAlloc is the primary exact
allocation count. Neither measure is peak RSS.

| Small n=3 median, milliseconds | C2 | D2 |
|---|---:|---:|
| Warm unchanged | 36.764470 | 10.762118 |
| Clock-only | 37.756980 | 65.254205 |
| Quote + FX append, full fallback | 59.338905 | 64.046801 |
| Correction | 54.404166 | 73.278098 |
| Quote-only append | 59.533656 | 73.258296 |

## Ordinary validation and normal CI cost

Targeted ordinary suites passed: application 23.435s, domain .005s and SQLite
.710s. Internal vet/build passed. The full 30-case policy/change durable-oracle
matrix passed locally in 6.150s. To avoid adding that cross-product to every
normal check, seven representative policy/correction/clock/delete/move/restart
cases remain normal tests; the complete matrix requires `-tags=perf` and
`NW_D2_LOCAL_ORACLES=1`. Authority, rollback, calendars, lifecycle/handling,
source-description and provider regression tests remain ordinary correctness
coverage. No workflow was changed or manually dispatched. No race was rerun.

Comparable split-workflow runs are main47 `39355709532`, user merge945
`39355749201`, and production577 `39357969811`. Their creation-to-completion
elapsed times are 287/350/267s, respectively. The945 run spent about89s before
jobs were created. This scheduling delay is distinct from job execution.
The final source commit's check result will be recorded in the PR without
another evidence-only source push.

| Test step seconds, main47 / merge945 / production577 | Main | Merge | Production |
|---|---:|---:|---:|
| Application shard1 + backup | 67 | 115 | 100 |
| Application shard2 + remainder | 96 | 104 | 149 |
| MCP + SQLite | 93 | 92 | 94 |
| Meta vet | 49 | 47 | 48 |

The Go step increase is significant even though total workflow got faster.
Per-test JSON identifies 11.50s of introduced tests at577: .71s in shard1 and
10.79s in shard2, including 6.50s for the full matrix now made local opt-in.
Common unchanged tests also slowed: median elapsed ratios1.60/1.728 by shard;
unrelated backup12.562→17.801s. Go steps additionally include compilation and
harness work. Native setup varied30–54s and frontend tests69–96s. These limited
runs show variance and introduced cost separately; they do not establish runner
variance as the sole cause. Raw job JSON is retained, without signed URLs.

## Remaining design boundary and acceptance

D2 currently computes financial suffixes but emits the complete daily
publication and retains C3 exact argument-to-current-SQL comparison for every
candidate UPSERT. It still decodes/authenticates the prefix, recomputes complete
policy/lifecycle and seals all protected rows. Reducing these costs further
requires an explicit next design: either a compact resumable lifecycle proof,
or a suffix-publication contract proving precisely which full-oracle daily
outputs can be omitted and validating that the retained SQL prefix equals those
outputs under the reserved transaction. D1 dependency scope alone is not that
output-equality proof. This iteration does not silently replace C3 argument
comparison with a financial fingerprint, carry SQL images between transactions,
or introduce E/shared persistent facts. The current enabled regression is
reported for the parent's design decision; it is not a successful end-to-end
performance acceptance.

Normal check and correctness review are the PR readiness gates specified by the
user. Race belongs to scheduled/local perf, not a normal readiness gate. The
historical single application race timed out with no warning and is not a pass;
no timeout increase or repeat was made. Thirty-sample p95, isolated per-arm RSS,
native Apple Silicon and native package/restore checks remain uncompleted.
Linux lacks GTK/WebKit/libsoup development dependencies for native desktop
build; internal build succeeds and native-dependency CI build succeeds. Native
QA must retain com.nestworth.dev / Nestworth.app and synthetic path isolation.

[Raw evidence, manifest and SHA256SUMS](artifacts/d1-d2-followup-2026-10-20/).
Rollback remains `NESTWORTH_REVIEW_FULL_RECOMPUTE=1` before normal initialization,
then explicit reconcile; it changes no persisted user setting.
