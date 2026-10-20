# Agent instrument append follow-up: stop after bounded probe

Decision: do not ship this extension in PR149. Keep pure Agent instrument imports
on their existing full fallback. The original direct-FX date-movement change is
unchanged. No runtime, certificate, schema, journal, import API or normal CI test
change is retained from this probe.

## Reachable gap and candidate

At runtime base `67b1696487e8b83552435e566cdcb1583938e525`, production
`Service.ImportAgentMarketData` accepts 1–100 instrument items and appends both
`instrument_quotes` and `agent_quote_records`. D's ordinary quote planner rejects
the audit-table mutation. Existing FX provenance capture requires an eligible FX
description, so it cannot authorize ordinary no-FX instrument imports.

A temporary candidate used a separate versioned instrument/Agent description,
complete exact audit row seals, typed instrument quote IDs, and existing source
MAC/certificate binding. It required unchanged old quotes/audit records and exact
new quote/audit/journal associations, admitting only daily close/nav append with
`agent_supplied_v1` and `date_label`. Mixed FX, corrections/retractions, latest,
closures and unsupported metadata retained fallback. Missing old proof could not
bootstrap reuse. Candidate code was discarded after the latency gate below; it
was not accepted as a fully validated implementation.

Small production-import tests demonstrated independent persisted full-oracle
equality for no-FX single, multi-instrument/date, backfill and same-day losing
candidates, including existing episode/acknowledgement history and daily timestamps.
Old-audit mutation, forged dates/IDs, non-append, mixed FX, latest/closed and final
proof tampering rejected reuse. A missing journal entry rejected suffix capture;
deliberately corrupt journals cannot be reopened for the independent oracle.
An FX fixture initially failed to establish its required episode, so unchanged-FX
acceptance and remaining proof-version tests were not completed after the stop
decision. These are experimental observations, not shipped correctness claims.

## End-to-end diagnostic

Linux amd64, Go1.26.2, synthetic isolated SQLite only. Two same-process pairs,
alternating arm order, 90 closed days ×2 instruments, one simple amount rule,
27 Agent audit records. Production imports supply the baseline and mutation.
The changed quote is dated Dec25 with latest closed day Dec29. No prefix memo.

The comparator uses `WithFullReviewRecompute`, retaining the actual application
service and independent complete financial computation. This is the explicit full
rollback baseline, not an exact measurement of the pre-extension certified
fallback dispatcher. A preflight capture records source-capture/hash and decoded
prefix work; the subsequent timed `ReconcileInvestmentReviewsV2` repeats capture,
preparation and publication. Import/snapshot rebuilding and fixture/oracle setup
are excluded. Both arms use warm same-process state and preflight source reads.
Capture timings are separate diagnostics, not added to the service total.

| Pair | Arm | Capture including hashing / prefix decode (ms) | Service total (ms) | Prefix days | Recomputed days |
| --- | --- | ---: | ---: | ---: | ---: |
| 1 | Candidate suffix | 16.093 | 27.601 | 175 | 5 |
| 1 | Forced full | 4.748 | 21.254 | 0 | 180 |
| 2 | Forced full | 3.998 | 17.471 | 0 | 180 |
| 2 | Candidate suffix | 14.995 | 30.706 | 175 | 5 |

The work-count reduction is 175/180 financial/predicate days. It did not produce
a measured end-to-end win: candidate totals were about30% and76% higher than the
full rollback arm. Capture/description hashing, prefix decoding, complete row
sealing and full lifecycle replay remain real costs; these samples do not isolate
the cost of the new audit scan alone. An earlier exploratory pair also showed
higher candidate latency; the final pair above corrects a diagnostic that had
enumerated one instrument twice. No p95, global latency or native-Mac claim.

The conditional implementation gate therefore was not met. No larger matrix,
race run or repeated full suite was used to search for a favorable workload. New
correctness probes/diagnostics are not added to normal checks, so this follow-up
adds no CI runtime. Documentation-only final validation is `git diff --check`.
The coordinator retains CI ownership; no deployment or merge is authorized.
