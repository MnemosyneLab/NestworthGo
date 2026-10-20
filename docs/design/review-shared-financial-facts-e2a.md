# E2a reader cutover assessment

Status: **design prerequisite only; E2a production implementation is not delivered**.
Inspected baseline: `7a25274252f6cd5846648a23da340cbe031f118a` (PR145).
This document does not activate a reader, change a schema, or add another shadow.
The [E1 diagnostic contract](review-shared-financial-facts-e1.md) remains current.

## Concrete blocker to an E1 reader switch

The missing component is an **authoritative current input-reference format and
resolver integrated with the existing complete projection proof**. A switch to
the existing E1 tables is not that component:

| Existing boundary | What a direct switch would miss |
| --- | --- |
| `review_result_fact_refs` primary key includes `fact_id` and `input_fingerprint` | Several versions can coexist for one rule/day. There is no current base/metric pair selector or publication identity. Joining by rule/day can combine versions. |
| `publishReviewFactShadow` samples emitted days and can skip the entire sample on admission limits | Neither absence nor presence proves complete coverage. Retained suffix days have no new refs. The sample limit cannot be used as a production frontier. |
| `certifiedProjectionSeal` in `review_certified.go` seals the old daily inputs/results and lifecycle rows | Neither shared payloads nor their refs are part of that seal. A newly computed shared-table checksum cannot establish the missing provenance chain. |
| `captureReviewSuffix`, `scanReviewSuffixOutput`, and `reviewDailyPublicationArguments` compare full old SQL images | Removing an input or substituting a reference changes the exact parameter contract. Keeping these reads preserves the repeated work. |
| `noChangeProjectionCounts`, completeness lineage, and publication equality operate on old input keys | Missing shared refs can be invisible to current completeness counts. Old row counts cannot certify a mixed format. |
| `reviewAlertGroupsForIDs` in `review_alert_read.go` reads latest amount/cost from old inputs | Merely omitting old input writes can leave displayed amounts stale after a correction. |
| `reviewWindowBaselineSQL` and the suffix reader start their date universe from old inputs | Missing old rows can silently remove false/unknown dates, not just financial JSON. |

These are coupled correctness boundaries, not a missing environment flag.
The current source/MAC/CAS/provider/calendar fences must remain. Making E1 refs
authoritative without the resolver would violate them. Retaining every old
input write/read plus E1 production writes would not reduce that work, so it
is not proposed as a performance delivery.

The blocker is architectural, not an approval denial or an assertion that
normalized storage is impossible. Implementing the authority format below is
necessary before a production speedup can be demonstrated. This assessment
stops before changing that format; it must not be reported as completed E2a.

## Smallest proposed production subset

Start with newly published, fully prepared rule/instrument scopes whose complete
date interval fits an explicit transaction budget. Admit only groups with at
least two rules with the same exact E1 financial semantic key for each supported
day, differing in thresholds/operators. Requirements, source versions, timezone,
calendar session, resolver and provider selection must match exactly. Count
potential savings before admission; two separate fact kinds and their reference
metadata can outweigh sharing for small groups.

Keep legacy inline inputs for all unadmitted scopes and nonexistent local dates.
In particular, Pacific/Apia 2011-12-30 remains an explicit unknown slot with
`nonexistent_local_date` and empty leaf evidence. Never skip a production date
because a fact cannot be keyed. Rules with different metric requirements,
currencies, FX modes, lookbacks, schedules or financial policies do not share.
No historical migration, startup backfill, GC or checkpoint is needed to begin
this subset. No existing E1 row acquires authority merely through admission.

### Required persisted boundary

Use a separate production namespace; leave E1 diagnostics unchanged and bounded.
The format must represent one unambiguous active record for every admitted
`(rule, condition, instrument, timezone, on_date)`. It must bind household,
input fingerprint, exact coverage, semantic/source namespace, publication
version and exactly one base ID and one metric ID. Restrictive foreign keys
must prevent cross-household/instrument or wrong-kind pairings, in addition to
application validation. Historical versions remain immutable; the active
selector changes atomically, and old referenced versions remain retained.

A membership manifest must describe the full admitted key set and its format
version. It must distinguish legacy inline dates from normalized dates without
inferring coverage from a minimum/maximum date or row count. A gap, duplicate,
wrong pair or unknown format is failure, never permission to omit evidence.

### Transaction and proof integration

1. Prepare the full supported scope from authoritative sources. Existing source,
   rule, handling, provider, temporal and suffix fences remain mandatory.
2. Under the existing publication transaction, compare all namespace/source/
   payload/coverage bytes when selecting an immutable version. Write each new
   version once and write the complete admitted references and membership.
3. Resolve the persisted representation and compare exact complete input bytes
   and unchanged per-rule results before activation. Preserve private exact FX
   scalars using `encodeCompleteReviewInput`/`decodeReviewInput` and
   `internal/jsoncodec` only. Hash equality alone is insufficient.
4. Publish results, lifecycle, current selectors, membership, control and temporal
   state atomically. Cancellation or any late failure rolls all of them back.
   No separate staging commit is visible to readers.
5. Include authoritative membership, active selectors and referenced fact bytes
   in the complete row seal and completeness format. Version the proof so an
   old certificate/memo cannot authorize the new layout. Extend exact SQL-image
   attestation rather than dropping it. Retain source MAC/CAS and row-seal checks.

The normal admitted path must stop writing the full old per-rule financial
payload. Its resolver should fetch/decode each exact fact version once per
bounded read transaction and supply independent owned input values to each
rule. Mutable maps must not alias across rules. Daily results, reasons,
three-valued votes and lifecycle evidence remain rule-specific. Memo eviction
must affect work only, not authority.

All input consumers in the table above need the same explicit mixed-format
resolver contract, including alert amount/cost and the full-recompute reader.
The existing proof cannot certify a subset based on an unmodified old-row seal.
Restart/restore must continue to lose same-handle reuse authority; loading the
new format for display or recomputation does not grant certification.

### Corrections, rollback and failure

Same-condition corrections append shared versions and atomically replace the
active pairs. Preserve baseline episode identity and acknowledgement while
allowing its opening evidence to update as today. Old shared versions cannot be
updated or repointed in place. Policy or calendar-session changes use distinct
namespaces and full safe preparation where reuse is unproved.

A production off switch must disable new normalized writes and financial reuse
while keeping the new format readable. After normalized writes, the old input
rows alone are potentially stale: switching a flag to an unchanged old reader
is not safe rollback. Full source preparation must reconstruct and atomically
publish legacy inline inputs before that reader can be selected. It must retain
all handling history, not attempt to reconstruct acknowledgements from prices.
If reconstruction fails, retain the previous complete readable projection and
its stale/repair status; do not expose a partly converted scope. Missing or
damaged normalized data must force authoritative recomputation, not reuse stale
old inputs. A failed refresh must report unavailable evidence explicitly.

Algorithm rollback does not downgrade the schema or make an old binary able to
open it. Preserve the schema-upgrade recovery-copy/export contract and all
post-upgrade financial writes. No tables or historic versions are deleted.

## Evidence and exit criteria

No new production path was enabled or hit in this assessment. No latency,
allocation, RSS, disk-saving or decoding-saving result is claimed. There is no
new paired runtime observation because no implementation was exercised.

The existing E1 observation is useful negative evidence, not a new benchmark:
two thresholds over eight days produced 16 old input rows, 16 shared fact rows,
32 refs, and 40,819 additional encoded-column bytes; 48 inserts were added to
the unchanged 32 input/result writes. Its shared payload/coverage was 4,237
bytes versus 4,366 old bytes. Source/key/ref overhead dominates that small
sample. See E1 for the measurement environment and exclusions. E2 must measure
total encoded columns and SQL/decoding work, not just deduplicated payload.

After the authority-format implementation, use a small threshold-sharing
fixture and one non-shareable variant. Compare inline-only and normalized
publication and readback for exact decimals, selected FX and provenance,
results, episode opening corrections, identity and acknowledgement. Cover one
late transactional fault/cancellation, reopening with loss of certification,
and mixed-format/off-switch fallback. Reuse the existing Apia fixture rather
than introducing a broad matrix. A complete normalized scope must also reject
a missing or mismatched active pair without silently dropping its date.

One or two small paired runs should record admitted/actually-read scopes,
legacy/normalized row counts, **total** encoded columns including references,
queries/rows consumed, fact versus per-rule decodes, and latency. Distinguish
cold/reopened from warm memo hits. Require an observed reduction in normal
admitted work before activation; leave the switch off with a concrete blocker
if overhead wins. No race, repeated full suites or broad performance runs are
needed for this assessment. Native desktop acceptance remains separate from
Linux checks. CI elapsed-time review belongs to the coordinator.
