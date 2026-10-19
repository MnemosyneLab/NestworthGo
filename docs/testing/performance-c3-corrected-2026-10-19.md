# Corrected C3 bootstrap, fallback and memory evidence

Production base: PR123 squash merge
`ab06a3f7ca342419948ae7a986cd8e5a73440536`. Measurement harness:
`6b5f7b33a807b16247b20ab736adfbb913633413`, branch
`perf/c3-corrected-evidence`. This is an independent evidence PR: no production
algorithm, candidate activation, financial/source schema, cache, dirty planner,
workflow or CI script changes. The candidate remains default-off.
Before delivery, main advanced to CI-only PR124
`604d7faf3b87d4962a167a38e9449cfb8dc80c2f`. That commit was integrated unchanged;
no production or measured Go source changed. Workflows and CI helper scripts
match latest main exactly. All timing/CI evidence below retains its original
source, rather than being relabelled as a run of the later CI-only integration.

The merge commit's automatic push checks passed all7 jobs in
[run39244315587](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39244315587).
This task only read their status; it did not dispatch or edit any workflow.

The [earlier C3 report](performance-c3-no-change-2026-10-19.md) and its raw
artifacts remain intact. Their pre-bootstrap-correction distributions are not
renamed or reused here. In particular, corrected C3 fallback reconstructs all
dates after missing/expired/untrusted proof, including older missing rows.

## Method and comparison boundaries

All inputs are test-created synthetic SQLite snapshots with deterministic
arithmetic source generation, fixed clocks, and UUID-identical `VACUUM INTO`
clones. No real ledger, provider network/credentials, native app, or cloud backup
was accessed. Schema21, Go1.26.1, SQLite3.53.0; full environment and exact source
heads are recorded in the accompanying provenance.

| Fixture | Accounts | Instruments/holdings | Rules | Days | Generated quotes + seed quotes | Cash-only activities | FX |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Small | 1 | 10 | 1 | 30 | 310 +10 | 100 | 120 |
| Large/R30 | 1 | 100 | 20 | 30 | 100,000 +100 | 50,000 | 120 |

Warm latency is30 alternating invocation pairs per fixture. Both arms use the
bounded C2 evidence-window configuration. C2 still captures/rechecks full
financial sources; the explicitly opted-in C3 arm confirms the unchanged
checkpoint. A new DB session's full bootstrap is timed separately and excluded
from warm samples. Setup/cloning/session prime, output comparisons, oracles,
storage/memory diagnostics and SQL-log formatting are outside invocation timing.
This is not a comparison to the production-default P0 baseline-window query.
Logical clocks advance by nanoseconds within a verified finite interval, not by
the real elapsed runtime of this controlled experiment.

Paired bootstrap/fallback diagnostics use3 independent clone pairs per scenario
per fixture, alternating order. No p95 estimate is inferred from n=3:

- Bootstrap starts from a published C2 snapshot reopened in a new session. The
  reference explicitly forces full recomputation; C3 must mint its first
  trusted completeness proof through full preparation.
- Clock-only advances one day after both sessions are primed. The reference is
  ordinary C2 bounded-window reuse; corrected C3 must rebuild every date.
- Proof repair removes the earliest input/result slots outside the rolling
  window and removes the C3 review proof, without source DML. Both the reference
  forced-full path and the corrected C3 path must restore those older rows.

Each pair compares all durable projection/lifecycle rows, normalizing only
computation timestamps and insertion order, and uses the existing independently
captured full-recompute typed daily oracle. The repair case asserts the expected
rule×instrument count on the restored earliest day. Separate n=1 quote correction
and market append diagnostics retain their full oracles and are never mixed
with no-change or clock-only samples.

SQL counters include driver executions, matched affected rows, returned rows,
Query-to-Rows.Close consumption (not pure SQL), and successful non-readonly
transaction reservation through Commit/Rollback. Readonly capture transactions
are excluded from that reservation counter even though the driver uses immediate
transactions. Returned count-query rows do not count visited historical index
entries. B/op-like allocation totals are process counter deltas, not live heap
or peak RSS. No source/evidence JSON reads are permitted in warm C3 confirmation.

## RSS isolation and limits

RSS has its own serial experiment, separate from primary latency. Each fixture
has3 alternating child-process pairs,3 warm operations per child. Each child
opens a verified synthetic clone, completes its same-session full bootstrap,
then runs one explicit GC plus `debug.FreeOSMemory` before signalling readiness.
The parent samples that child's `/proc/<pid>/status` VmRSS at a requested1ms
interval from readiness/start through an end-of-operation handshake. Sampling
ends before SQL summary/JSON reporting allocations. Setup, fixture generation,
bootstrap and parent-side oracles are outside this sampled region.

`SampledPeakRSSKiB` is the largest observed VmRSS, a sampled lower bound rather
than an exact kernel operation-only maximum. Poll counts and raw per-process
values are retained. Ready RSS and process-lifetime VmHWM before/after operations
are reported separately; lifetime HWM includes startup/full bootstrap. No kernel
counter is reset and no privileged process/system setting is changed. The
requested sampling interval can jitter under CPU pressure; small differences
between independently read kernel counters are not treated as precise allocator
measurements. RSS child timings include a polling parent and are not mixed into
the primary warm latency distribution.

This method isolates post-setup algorithm residency after an explicit decommit;
it does not estimate natural whole-desktop residency after bootstrap. Three
Linux process pairs cannot establish the proposed native30-pair10% RSS rollout
gate. Native Apple Silicon verification remains separate.

## Validation and results

The small fallback/RSS harness passed under race110.722s, including process
handshakes, paired output/oracle checks and restoration of older missing rows.
Perf-tag internal vet and internal build passed. The canonical desktop build
failed because GTK4/WebKitGTK6/GLib/libsoup development packages are unavailable.
No native or full-suite success is inferred from these focused checks.

Formal latency runs passed: warm361.852s and paired fallback563.886s. All60 warm
invocation pairs completed, with equal durable output at each arm's start/end
and a final independent full oracle per fixture. Each of the18 fallback clone
pairs passed its own durable comparison and full oracle; repair restored the expected
earliest rule×instrument rows. Primary warm results (nearest-rank p95, sample29):

| Warm no-change, n=30/arm | C2 median | C3 median | C2 p95 | C3 p95 |
| --- | ---: | ---: | ---: | ---: |
| Small | 35.260ms | 9.469ms | 41.664ms | 12.843ms |
| Large/R30 | 5.493s | 40.250ms | 6.004s | 48.068ms |

Median allocated bytes per warm call are8.124MB →1.324MB small and1,943.044MB
→2.566MB large (decimal MB). These are cumulative allocation deltas, not RSS.

Paired fallback medians below have n=3/arm, not p95 estimates:

| Scenario/reference | Small reference → C3 | Large reference → C3 |
| --- | ---: | ---: |
| Bootstrap / forced-full | 59.426 → 72.192ms | 10.047 → 10.712s |
| Clock-only / C2 bounded | 39.874 → 65.780ms | 6.339 → 11.292s |
| Proof repair / forced-full | 52.586 → 64.621ms | 12.481 → 11.254s |

Clock-only emits10 versus310 pair-days in the small fixture and2,000 versus
62,000 in large: outputs agree, but corrected C3 deliberately prepares all
dates. The large clock-only median is about78% higher than ordinary C2 in this
small diagnostic sample. Bootstrap/repair include a forced-full reference and
must not be interpreted as C2 incremental comparisons. These results support
keeping default-off and investigating completeness lineage, not weakening
bootstrap/expiry validation. They do not establish a native regression estimate.

The separate large n=1 clock-only/correction/append observations were
11.830s /10.614s /11.416s; the paired scenario tables are the primary fallback
comparison. These n=1 observations are not p95 estimates.

The isolated RSS run passed314.441s, including paired durable comparisons and
full typed oracles after each process pair. Median counter values in KiB:

| Metric, n=3 processes/arm | Small C2 → C3 | Large C2 → C3 |
| --- | ---: | ---: |
| Sampled warm-region VmRSS peak | 29,828 →28,016 | 584,484 →46,372 |
| Ready VmRSS after bootstrap/decommit | 25,700 →25,824 | 44,092 →44,668 |
| Process-lifetime VmHWM (includes bootstrap) | 29,680 →29,688 | 639,192 →679,672 |

The large sampled region median is570.8 →45.3MiB. Its lifetime HWM medians are
624.2 →663.7MiB; the bootstrap peak is not removed by cheaper confirmation.
Warm-region peak ranges are547,632–601,208KiB C2 and44,376–47,112KiB C3. Neither
these small Linux samples nor lifetime counters establish a native30-pair
regression threshold. Actual per-process ready/peak/HWM values, poll counts and
three operation durations are retained, without treating child timings as main
latency observations.

Formal runs were serial from11:34:12Z to11:55:04Z on2026-10-19. Exact
commands, raw logs, per-arm RSS values, summaries and checksums are retained in
[`performance-c3-corrected-2026-10-19/`](performance-c3-corrected-2026-10-19/).

## Next bounded milestone and outstanding gates

Keep this evidence independently reviewable. PR123 implements opt-in unchanged
confirmation and conservative fallback; it does not complete the plan's C3
publication-token/CAS replacement for full-source rechecks. D1/D2 suffix work
depends on that transactional/effective-time foundation and B's independent
bounded-window validation. Do not treat a merged default-off candidate as
permission to activate it or advance directly to suffix/shared-fact storage.

The measured clock-only full-rebuild cost makes completeness lineage a useful
next bounded C3 implementation, **in shadow mode first**: validate a current
trusted manifest before considering bounded window reuse, carry its coherent
checkpoint/provenance through preparation, and revalidate it before writes under
the publication reservation. Do not restore an early return before count/proof
validation. Missing/legacy proof, gaps, changed namespaces or concurrent damage
must still select full reconstruction. Preserve every bootstrap/expiry repair,
direct-caller and incremental-C2-publication regression and compare all outputs
to the forced-full oracle. No runtime shortcut should activate from that shadow
work without its own review and paired-workload gates.

The remaining C3 publication-validation shadow/CAS slice must separately compare
verified source namespaces inside the write reservation while retaining the full
fingerprint/frontier oracle, handling/episode/rule CAS, cancellation and
clock/activation failure proofs. Neither implementation is bundled into this
measurement PR. Independent G
analysis quote batching remains an alternative under its A/input-equivalence
gates, not an excuse to expand this evidence slice.

Default activation still requires reviewed source/time/coverage completeness and
representative native paired latency/RSS across bootstrap, no-change, clock-only,
append, correction and future activation; the plan's proposed >10% regression criterion is not
waived by a good no-change result. Product targets3s cold/500ms warm and the cold
definition (process/OS pages/UI) require separate Apple Silicon measurement.

Actionable native gates: use macOS27/Apple Silicon with its SDK and the pinned
Wails toolchain; build the exact source for local QA with the established
`Info.dev.plist` / `com.nestworth.dev` identity. Production packaging retains its
separate release identity. Launch local QA only after explicit synthetic
DB/settings paths and marker checks. Run
the paired latency/RSS matrix, cancellation/logical-as-of and archive payload
launch/restore checks; record model, storage and cold definition. Native
VoiceOver/IME acceptance remains unrun. Installing Linux GTK/WebKit development
packages would unblock this host's desktop compilation but would establish none
of those Mac gates. No merge, release or activation is performed here.
