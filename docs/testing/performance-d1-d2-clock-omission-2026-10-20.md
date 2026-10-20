# Integrated D2 clock-only publication omission

Production baseline `057ef31e464ebd831b8dd470084778ac3d295646` preserves the
user's merge of main `3d202a2a1c8a3c60401717b121f52f473c79589c` / PR133.
Production candidate `7eb1e5cad44bf011d99455c0e2538c381bcf0df1` adds the bounded
D2 omission proof below. Subsequent evidence commit changes no production code.
Earlier 5.825/7.961s measurements precede this integration and are historical.

## Contract and implementation

[The design](../design/review-dependency-suffix.md) now specifies exact output
attestation, beyond financial dependency equality. On unchanged complete source
fingerprint **and exact source token**, one coherent reader obtains every13
input and9 result SQL payload column, decodes the prefix and compares it against
the shared canonical publication argument builder. Keys, types, NULL, scalar
columns, coverage/reason, JSON bytes and exact selected FX must all match.
Only an attestation bit/membership and typed evidence leave this transaction;
no SQL images cross transactions. The existing typed-prefix digest/source MAC
binds them. Representation mismatch emits that day through the normal comparator.
Changed-source/correction/fallback paths retain complete publication comparison.
The shared builder also avoids the old discarded ordinary JSON serialization
before producing the complete certified input codec; output bytes are unchanged.

Preparation retains full policy/position/lifecycle replay and latest/frozen
handling output. Only attested daily SQL output is omitted. Before comparison
and again after the final clock, the writer derives eligible scopes from the
capture and enforces a unique origin-to-latest partition: every expected daily
key is either authenticated retained output or an emitted day. Missing/duplicate
pairs, extra dates, gaps, overlap, duplicate days and fabricated target exclusion
reject atomically. Actually excluded pairs have zero expected daily output.

Existing physical counters, schema/trigger/FK contracts, source/control/checkpoint
CAS, pinned unchanged provider lease, temporal guards, final typed-prefix MAC
check and post-publication complete row seal remain. Equality images are read
inside the writer reservation from the earliest emitted date, a conservative
superset of attempted keys; absent images execute the original UPSERT, and
consume-once semantics remain. Without attestation the reader covers all dates.
No new historical authority, lifecycle checkpoint, E/shared facts or schema
migration is added. Legacy/restarted/restored ledgers retain full fallback.
Normal GUI/MCP initialization remains enabled; rollback remains
`NESTWORTH_REVIEW_FULL_RECOMPUTE=1`, then explicit reconcile.

## Correctness and cost controls

Ordinary targeted domain/SQLite/application suites passed .010/.833/24.252s.
Final focused SQL-image, partition/atomicity and exact-FX tests passed
.064/1.079s (SQLite/application). Local opt-in30-case policy/change matrix passed
6.122s. Internal vet/build passed on final production source. Tests compare an
independently captured forced-full plan, complete durable output and **all three
daily timestamps** (computed_at/evaluated_at/revised_at). The clock path covers
all policies in the local matrix; the light ordinary guards cover missing,
duplicate and overlapping days, missing/falsely-excluded pairs and late
attestation mutation. The exact-FX clock test explicitly requires attestation.
SQL-image tests use SELECT placeholders, not mutations to certified rows; they
exercise scalar/NULL/JSON mismatch and changed-source rejection.

The exhaustive matrix remains local opt-in (`-tags=perf NW_D2_LOCAL_ORACLES=1`).
The matched-fixture helper and measurement harness are perf-only. No race was
run, no timeout increased and no workflow edited/dispatched. Race remains a
scheduled/local performance check, not a normal PR gate. Final normal-check
result and actual step/per-test durations are recorded in the PR without an
additional evidence-only source push.

## Matched separate-build experiment

Three matched repetitions total, below the user's ten-repetition cap; no
30-sample run or p95 claim. Linux amd64, Go1.26.1, SQLite3.53.0, GOMAXPROCS4,
four-CPU quota. Each build reads the **same saved unpublished synthetic facts**
(UUIDs included), then creates fresh genesis clones and earns independent
same-handle certification; no signed authority is imported. Baseline production
is unchanged apart from the identical opt-in fixture helper and one-line source
selection patch. Binary/helper/source checksums, order and raw samples are in
[the manifest](artifacts/d1-d2-clock-omission-2026-10-20/manifest.json).

Order is baseline1, candidate1, candidate2, baseline2, baseline3, candidate3;
runs are serial, unprofiled, with setup and full/durable oracles outside timed
reconcile. Config:100 instruments/holdings,20 rules,30 historical days,
100,000 quotes,50,000 cash activities. Each timed arm has a paired existing
**C2 bounded clock control**, which is distinct from the forced-full oracle.

| Matched repetition | Baseline C2 / D2 seconds | Candidate C2 / D2 seconds |
|---|---:|---:|
| 1 | 5.794713 / 7.835409 | 5.530950 / 7.215158 |
| 2 | 5.892859 / 8.024530 | 5.488562 / 7.076847 |
| 3 | 5.765275 / 8.382097 | 5.966874 / 7.260141 |

Baseline/candidate D2 medians: **8.024530 /7.215158s** (−10.1%), ranges
7.835409–8.382097 /7.076847–7.260141s. Their paired C2 medians also shift
5.794713→5.530950s, so the elapsed delta is not attributed wholly to code.
Median per-sample D2/C2 ratios are **1.361738→1.289381**: enabled D2 still
trails its control by about29%. This is a small diagnostic distribution, not
end-to-end acceptance or a claim about other workloads.

D2 median total allocations are2,389,810,456→2,247,259,856 bytes (−6.0%).
Median writer reservation is3.688832→2.064208s (−44.0%). Returned rows fall
714,382→594,382 (120,000 fewer); queries remain667, executions8,242, affected
rows6,242. Actual publication falls62,000→2,000 pair-days; independent full
oracle still audits62,000, financial recomputation2,000 and prefix reuse60,000.
The old C3 comparator already elided the prefix's no-effect writes; omission
saves image reads/publication work, not additional affected-row writes.

Every paired plan/full and durable-output oracle passed. TotalAlloc is cumulative
allocation, not peak RSS. Raw whole-process VmHWM includes setup, both arms and
oracles; it is **not** isolated per-arm RSS or a native acceptance measurement.
No integrated append/correction speedup is claimed from historical measurements.

The remaining gap is measured rather than forced into a pass. Full source
capture, prefix decode/canonical attestation, digest authentication, policy and
lifecycle replay, and complete protected-row sealing remain real costs. No
claim is made that all remaining D2-only opportunities are exhausted. p95,
isolated per-arm RSS and native Mac/package/restore validation remain unproved.
Linux lacks native GTK/WebKit/libsoup development dependencies; CI native-dep
build is separate. Native QA must retain com.nestworth.dev / Nestworth.app and
explicit synthetic database/settings paths. No real ledgers/settings/provider
credentials, release or PR merge were touched.
