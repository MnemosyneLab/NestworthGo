# C3 certified no-change and publication candidate (2026-10-19)

This records the intermediate default-off C3 implementation and frozen evidence.
The [current follow-up](performance-c3-followup-2026-10-19.md) adds trigger
integrity fixes, reduced repeated verification and restricted history-free
legacy/restart bootstrap; it supersedes the older bootstrap limitation below.
That bootstrap was subsequently [disproved and disabled](performance-c3-provenance-2026-10-19.md):
full daily replay cannot authenticate missing episode/ack history. Current
initial authority requires a separate process-private fresh-creation receipt.
The [current schema/dispatch checkpoint](performance-c3-dispatch-2026-10-19.md)
also covers internal FK writers. Ordinary C3 now performs one full preparation
after a missed confirmation; bounded auditing requires explicit lineage selection.
**Stage-two rollout acceptance is still blocked.** Production initialization
keeps C2. `WithCertifiedReviewCandidate()` is an internal synthetic experiment;
`WithFullReviewRecompute()` disables it. No UI/MCP setting enables it. There is
no schema migration, new trigger, general cache, dirty dependency/suffix planner,
write-trigger optimization, or CI change in this PR.

The branch starts at PR127 merge `2fa0a34f59e577b81e80c5cef81dc962315d6319`.
PR119 `c3867c626d3b6a0cdbcf669a0047ff8d07d4215f` and PR121
`09d6276bb0f0be6ebdbfa4afacfcca843c1e623a` are exact ancestors. PR126's
user-owned test deletion and main's workflows are preserved. Main's automatic
[check run](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39260731976)
succeeded on the PR127 merge; this task did not dispatch a workflow.
Main later advanced to PR128 `326688ce86d35216a9fe55a3bee63bcbb379683d`, changing
only three continuousbackup test files. It is incorporated unchanged in merge
`7853e7e423b704fd03e86c4baf56ec3cd03b79b9`; review production/test sources and CI
files are unchanged by that merge. The affected ordinary package is checked
separately; its provenance is recorded with final validation.

## Authority and fallback

A narrow lineage checksum and row counts cannot authenticate arbitrary
preexisting historical input/result JSON or frozen episode/event payloads.
The runtime shadow tests demonstrate that limitation against a clean clone
created **before** corruption/publication. A clone made after publication is
only an idempotence check and is never described as proof of repair.

The candidate uses a random private HMAC key owned by one open DB handle.
Neither key nor authority is exported, persisted as a secret, or reconstructed
from existing data. Only a previously empty six-table derived namespace with
no prior published/temporal checkpoint, or a valid same-handle authenticated
chain, may issue a `certified-full-review-v1` checkpoint. It authenticates the
complete temporal/source/handling certificate and a complete ordered row-byte
seal of control, current and historical rule configuration, targets,
suppressions, daily inputs/results, epochs, evaluation state, episodes and
events. A freshly computed checksum never blesses unknown history.

On the frozen `e112f58` implementation, existing/restarted/restored/corrupted projections **cannot bootstrap
no-op or bounded authority**. They retain full financial capture, full
preparation and conservative publication, including the original full source
recheck whenever tracking/proof is unavailable. A new coherent full capture
and full preparation can still use separately verified **source publication
CAS**, even when historical projection authority is absent: the source proof
is independent of trusting old evidence. It grants no no-op/bounded authority.
Full fallback does not claim
to repair every historical JSON/lifecycle defect; the original conditional
writer does not provide that guarantee. No automatic destructive reset is
provided. Epoch/tracking/semantic-policy changes, unknown metadata and damaged
optional authority fail closed. Malformed optional authority cannot prevent an
otherwise readable C2 temporal checkpoint from taking fallback.

Legitimate rule, suppression and acknowledgement commands preserve a valid
chain inside their existing transaction, marking it as needing reconciliation.
They cannot heal an invalid chain. Old counting-only certificates grant no
certified authority and the new marker cannot downgrade into that protocol.
The old constructor remains available for reproducible historical experiments,
with its full fallback unchanged; it is not the activation API.

## Source, temporal and concurrency fences

* Capture still reads and fingerprints all financial facts at one coherent
  source cutoff. Its private signed proof binds typed C1 checkpoint/floor,
  schema cookie, complete protected projection seal and configuration metadata.
* A pinned explicit provider selection carries a process-local revision. The
  service holds its read lease through no-op/publication **COMMIT**; same-key,
  ABA, registry replacement and exhausted revisions invalidate eligibility.
  Mutable registry defaults cannot establish shortcut authority. The staged
  provider key must equal the held selection. Provider changes also expire
  review read status through the clock context.
* Each certified operation (including freshness readers) pins its actual physical
  SQL connection. Capture samples counters **before** any financial snapshot read;
  a later commit cannot be absorbed as provenance for an older snapshot. `data_version`
  fences other-connection commits and `total_changes()` fences own-connection
  DML, including rolled-back writes (separately tested). These are
  [connection-local counters](https://www.sqlite.org/pragma.html#pragma_data_version);
  [total changes](https://www.sqlite.org/c3ref/total_changes.html) includes trigger
  and foreign-key writes but excludes REPLACE deletion counts. Replacement inserts
  still change the counter. Unknown/changed connection counters select
  full source recheck. Same-day no-op requires signed post-publication counters
  to match, so it need not read any quote/evidence JSON. Full tracking/schema
  verification and typed checkpoint, rule, handling and time checks still run.
* Publication takes the existing SQLite immediate write reservation before
  checking. Valid capture proof, exact typed checkpoint/floor and unchanged
  protected snapshot/counters allow source CAS to replace the second full
  financial reload/fingerprint. Unknown tracking selects full recheck;
  changed authoritative/protected state conflicts and retries at most three
  times. The Go mutex alone is never the database concurrency proof.
* Optional actual bounded publication requires authenticated prefix coverage,
  unchanged source/provider and complete independent full-oracle agreement.
  Every emitted date and typed financial/coverage/window field plus every
  lifecycle/epoch/ack field is compared. Only fresh episode UUID allocation is
  normalized. A frozen, unaliased full graph remains available for fallback;
  publication recomputes the comparison rather than trusting a matched stamp.
  Missing proof, missing oracle or comparison failure cannot authorize reuse.
* Full oracle preparation remains mandatory. The candidate saves repeated
  unchanged computation on no-op and can save the second financial source
  read/row attempts at publication; it does **not** eliminate full preparation
  on clock-only calls. Selected FX evidence additionally persists the private
  exact rate scalar for lossless reuse. Financial algorithms and source facts
  do not change.
* Control and temporal checkpoint CAS require exactly one affected row. Daily,
  lifecycle and checkpoint updates commit atomically; cancellation/ignored or
  aborted writes do not leave partial rows. C2 rechecks future transitions and
  samples the injected clock after all new prewrite reads. The persisted end
  remains **min(captured frontier, rechecked frontier)**, preserving PR121.

The trust boundary is the verified C1 schema and SQLite engine/connection
contract. Metadata rewinds are additionally tested on own/other connections.
Privileged SQLite file/schema-version manipulation, counter wrap combined with
intentional metadata forgery, or hostile in-process mutation of private keys
are not an authenticated hostile-database protocol. Unknown tracking or
connection provenance must never be interpreted as unchanged data.
In particular, unsafe `writable_schema` edits that forge stored SQL independently
of SQLite's cached parsed schema are excluded privileged schema manipulation;
the complete text contract does not attest engine internals. That unexecuted
reviewer scenario is recorded with the
[exact publication-equality proof](performance-c3-publication-equality-2026-10-19.md).

## Correctness and validation

Focused ordinary coverage includes full-output mutation rejection, complete
new-day sets, exact FX round trips, aliasing, clean-before-damage historical and
frozen JSON comparisons, same-count key changes, malformed/forged authority,
missing proof/oracle, legacy and restarted handles, rollback/recovery,
connection renewal, epoch/floor/policy changes, temporary schema, legitimate
rule/policy/timezone/ack/suppression handling, provider ABA/lease/registry
changes, concurrent source/projection changes, manual journal rewinds, final
clock crossing/cancellation, mandatory CAS/rollback, and full-oracle agreement.
Future exit/entry/clear-rebuy/transfer are checked at T-1ns, T, T+1ns with a
constant source token and no DML during activation. A separate regression moves
the rechecked frontier earlier and asserts that PR121's minimum is persisted.

Final command logs, exact source provenance and the deferred final race result
are recorded alongside the measurements in
[artifacts](artifacts/c3-certified-2026-10-19/). Development used ordinary focused
tests only. After the completed draft PR, **one** final related race invocation
ran on `53dbd180c3d75884a0132c9400a5a14f94015d7b`. Its application package hit the
**global 20-minute timeout**, at
`TestReviewTemporalFutureQuotesOriginAndHoldingFrontiers/holding` (that child
had run for only 1s). This is a package budget exhaustion, not evidence of a
hung child or an observed data-race assertion. New certified/window-oracle
application cases completed beforehand; the remaining application coverage is
incomplete. **The overall race gate is not passed.** No rerun is claimed and the
budget was not raised. Domain passed (1.052s), and SQLite passed (214.737s),
including the new unknown-connection fallback case. The whole command exited 1
in 1,457.805s. Per-case completion and actual outcomes are recorded in the
[coverage record](artifacts/c3-certified-2026-10-19/race-coverage.json), final
[race metadata](artifacts/c3-certified-2026-10-19/race.json)
and [raw log](artifacts/c3-certified-2026-10-19/final-relevant-race.log).
Native package/UI acceptance is a separate gate.

Final ordinary checks on frozen code `e112f58` passed: all `./internal/...`
tests (application 120.267s), internal vet/build, gofmt, diff and workflow
preservation. Exact commands/results are in
[validation.json](artifacts/c3-certified-2026-10-19/validation.json).
The native Linux build attempt at `28f2c8f` failed at pkg-config for GTK4,
WebKitGTK6, GLib/GObject/GIO and libsoup3; the [log](artifacts/c3-certified-2026-10-19/native-build.log)
is a failed gate, not an app build pass. No macOS build, package or UI run was
performed. Full-project/frontend/binding checks and full-backend race are not
claimed. The historical dense 10s full-backend race deadline is not waived.

Final paired measurements below were built at `c11030c`, with the review source
and tests identical to frozen implementation `e112f58`. The earlier
[fa1bd47 run](artifacts/c3-certified-2026-10-19/paired-prior-fa1bd47.log) is archived
ancestry evidence and is not used as the final result. Stage-two rollout
acceptance remains blocked.

## Reproduction and measurement contract

```sh
export PATH=/tmp/go/bin:$PATH
export GOCACHE=/tmp/nestworth-c3-cache GOMODCACHE=/tmp/nestworth-c3-mod
export GOPATH=/tmp/nestworth-c3-path TMPDIR=/tmp/nestworth-c3-scratch GOMAXPROCS=4
NW_C3_CERTIFIED_EVIDENCE=1 NW_REVIEW_PERF_SAMPLES=3 \
  go test -p 1 -tags=perf ./internal/application \
  -run '^TestReviewCertifiedPerformanceEvidence$' -count=1 -v -timeout=30m
NW_C3_CERTIFIED_EVIDENCE=1 NW_REVIEW_PERF_SAMPLES=3 \
  go test -p 1 -tags=perf ./internal/application \
  -run '^TestReviewCertifiedBootstrapActivationEvidence$' -count=1 -v -timeout=30m
```

Optional `NW_C3_PERF_SIZE=small|large` selects a fixture. Small is 1 account,
10 holdings/instruments, 1 rule, 30 days, 310 generated quotes and 100 cash
activities. Large is 1 account, 100 holdings/instruments, 20 rules, 30 days,
100,000 generated quotes and 50,000 cash activities. Both also have their
existing origin quotes and daily FX references; complete manifests are logged.
This is synthetic cash-background capacity evidence, not a realistic trade
mix or a 365-day/native acceptance result.

The four paired workloads compare normal C2 with the certified candidate. The
separate bootstrap/activation experiment uses an explicitly labeled **C2-full**
forced-full reference; it must not be interpreted as normal C2 activation cost.
It avoids relying on historical cached JSON that lost a private FX rate scalar.

Arms clone authoritative facts **before** any projection exists. An identical
full preparation supplies shared bootstrap episode identities, while every
handle captures/signs its own proof. C2 receives the same exact-FX bootstrap
codec, favoring C2 over its historical private-rate JSON loss. Newly allocated
bootstrap event UUIDs are compared through their unique event keys; other
persisted fields remain exact. The only removed JSON field is the additive
exact-rate codec field, whose exact typed values have a separate pre-publication
full oracle comparison. Timestamps normalized by the existing comparison are
computed/evaluated/revised timestamps, not economic or handling dates.

End-to-end timings include the service call through commit. Independent
pre-publication test oracles, fixture generation/clones/bootstrap, GC and
post-publication persisted-row comparison are outside those timing intervals;
the **runtime** full oracle is inside them. Arms rotate order by sample.
Bootstrap/activation use independent full pre-publication preparation and check
that activation did not change the source token. Every sample starts fresh;
there is no sequential-growth inference. Correction uses the existing helper's
deterministic sample variant: instrument 0, origin + (5 + sample % 20) days,
new price 250 + sample (zero-based). Both arms receive the identical mutation
within each pair; variants can produce different lifecycle write counts across
pairs. Their n=3 median is diagnostic across those variants, not an identical-
workload p95. Other paired scenarios apply the same mutation per fresh sample.
SQL counts are returned rows and driver
executions, not SQLite VM visited-key counts. Allocated bytes/allocs are
`runtime.MemStats` deltas, not RSS. Whole-process VmHWM includes setup/clones/
oracles and is explicitly unsuitable for per-arm RSS acceptance. n=3 medians
are diagnostic, with raw values retained; no p95 acceptance is inferred.

## Paired Linux results

The four-workload experiment passed all pre-publication typed full-oracle and
paired persisted-row comparisons (3 samples per arm/scenario/size). Binary
source is `c11030c5d0ede060d71e7edc0856b6c9038b75db`, review code identical to
`e112f58a56c21c56b9b82cec8a7fe9b6892d3c3a`. Exact per-check attribution is in
[environment/provenance](artifacts/c3-certified-2026-10-19/environment.txt).
Raw samples and allocated-byte/SQL diagnostics are in the
[paired log](artifacts/c3-certified-2026-10-19/paired-small-large.log) and
[generated summary](artifacts/c3-certified-2026-10-19/summary.md).

| Size | Scenario | C2 median | Certified median |
|---|---|---:|---:|
| Small | same-day warm | 38.366 ms | 12.322 ms |
| Small | clock-only | 39.208 ms | 113.056 ms |
| Small | append | 53.714 ms | 91.202 ms |
| Small | correction | 56.040 ms | 100.779 ms |
| Large | same-day warm | 5.676 s | 39.646 ms |
| Large | clock-only | 5.913 s | 12.312 s |
| Large | append | 10.059 s | 12.076 s |
| Large | correction | 9.582 s | 11.197 s |

**Changed/clock-only paths regress; default enablement is blocked.** Warm gains
are not a waiver for those regressions. The full runtime audit, repeated
tracking validation and complete row seals are intentionally still present.
Large clock-only publishes 2,000 pair-days while independently auditing 62,000;
its cumulative allocations are about 4,068 MiB versus C2's 1,915 MiB. No claim
is made that bounded publication eliminates full calculation. Source-changing
calls compute all 62,000 (append) / 60,000 (correction) pair-days. The candidate
uses verified source CAS in every measured non-no-op publication; small and
large warm samples assert that no quote/evidence JSON query occurred and use
only two confirmation row writes. Whole-process HWM is unsuitable for arm RSS;
no native p95/RSS or product SLA is accepted by these diagnostic samples.

The separate bootstrap/activation experiment also passed (3 samples per arm
and size, source `e112f58a56c21c56b9b82cec8a7fe9b6892d3c3a`). It preserves a
constant source token across the future event and uses pre-publication full
oracles. Its reference is **forced-full C2**, not ordinary C2.

| Size | Scenario | C2-full median | Certified median |
|---|---|---:|---:|
| Small | bootstrap | 62.571 ms | 82.379 ms |
| Small | intra-day activation | 55.399 ms | 98.094 ms |
| Large | bootstrap | 10.761 s | 12.109 s |
| Large | intra-day activation | 9.787 s | 11.577 s |

These are full-reconciliation costs: 300 pair-days for small and 60,000 for
large. Activation does not silently become a no-op or bounded-only calculation.
[Raw lifecycle measurements](artifacts/c3-certified-2026-10-19/bootstrap-activation.log)
and all raw samples remain available; no n=3 p95 or per-arm RSS claim is made.

## Activation gates and native handoff

Default enablement remains off. The gate status is:

1. Independent review of C1 coverage, temporal closure, authenticated provenance
   and provider/connection/atomic publication proof, including the follow-up's
   restricted complete-replacement bootstrap. Existing/restarted ledgers with
   frozen lifecycle history still fall back; a checksum cannot authenticate it.
2. Ordinary internal backend tests/vet/build passed, but the final related race
   hit its application package's global 20m budget. Race acceptance remains
   incomplete; this check is not marked passed.
3. Paired 30-sample p95 and isolated per-arm peak RSS on the target platform,
   with no unexplained >10% regression. Any measured changed-path overhead
   blocks default enablement; keeping full runtime audit is intentional until
   reviewers accept an equally strong alternative.
4. Native Apple Silicon acceptance and the unchanged cold 3s/warm 500ms/list
   150ms goals. Linux measurements cannot establish these goals.

On macOS, use only the established `com.nestworth.dev` / `Nestworth.app`
identity, explicit isolated `NESTWORTH_DATABASE_PATH` and
`NESTWORTH_SETTINGS_PATH`, and a verified synthetic household marker before
launch. Confirm no-op, clock-only, source mutation, future activation, handling,
provider change, cancellation and reopen/full fallback through the real app;
record end-to-end as-of/status, financial/lifecycle oracle output, p95/RSS and
cold/startup distinctions. No activation, merge or release is authorized here.
