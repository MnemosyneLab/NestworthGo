# D-FX direct reference suffix proof (2026-10-20)

Baseline: `9c21b2de808430e7837dfccecf8edbb30fbb0113`, verified against
`origin/main` before this independent branch. This D-only extension uses the
existing suffix dispatcher, certificate JSON, source journal and daily layout.
There is no schema migration, new codec, persistent raw-rate store or H1 change.

## Eligible scope

Changed FX may reuse a financial daily prefix only when the complete OLD and
NEW captured inventories contain one existing direct base/comparison currency
pair of agent `daily_reference` candidates with valid civil effective dates,
nonzero value-effective timestamps, and nonempty source-policy/timestamp-basis
metadata. Every candidate has the same source key, source-policy version and
timestamp basis; those values must remain unchanged. Portfolio FX inputs must
be an exact subset of the complete historical candidate inventory. All captured
instruments use one of the pair's currencies; every currency-bearing rule leaf
names the comparison currency. Identity conversion is harmless. Inverse/cross
routing, manual/timestamped and provider-quality branches are outside v1.

Eligible journal operations are insert, deletion, and correction of only the
rate/revision of an existing ID. An update cannot change any admission/selection
metadata, pair or date. Only one mutation of each ID may occur between captures;
intermediate states otherwise require a separate proof. Deleting the last
candidate, an empty inventory, mixed dependency families and missing or malformed
coverage use full preparation. Generation-only history-state updates may
accompany FX writes after the independent invariant agrees.

Provider preferences, provider lease, rule versions, accounts/activities,
ownership, financial policy, timezone/cutoff/calendar, and every other non-FX
captured input must be unchanged. The non-FX invariant retains instrument quotes,
so a combined instrument-quote/FX mutation cannot pass. Full-source equality is
still authoritative; this is no substitution of a narrow token for capture.

## Complete candidate and date proof

`DescribeReviewFX` separately versions a non-FX invariant (only both FX arrays
and non-economic input generation are excluded) and a complete location/pair/date
bucket inventory. Each bucket contains candidates ordered by ID. Each candidate
fingerprint covers **every** captured FX field and exact private rate scalars
through the existing historical fingerprint codec. A second fingerprint excludes
only rate/revision, making admission/selection metadata changes ineligible.

The descriptions are generated in the coherent source-capture transaction and
bound by its existing source MAC, including full captures that cannot reuse a
prefix. A normal authentic full publication carries that captured description
into the existing certificate MAC. Publication never derives new FX authority
from caller-modified historical fields. Exact unchanged-source captures may reuse
the prior authentic deterministic description. Legacy/missing/incompatible
FX descriptions cannot authorize changed-FX reuse; restart/restore/key loss never
acquire historical authority from a newly calculated checksum.

The planner compares the union of complete OLD/NEW historical candidates, not
only selected rates. Every changed candidate requires exact journal ID, pair and
OLD/NEW civil-date coverage against those independently captured inventories.
Insert/delete absence and update scope equality are checked. Missing journal
coverage, forged dates and hidden date/pair moves fail closed. Both complete
portfolio inventories are checked against their corresponding historical
inventories, including exact payload and selection fingerprints. Their latest
view membership may change only among those exact historical candidates:
review's historical reconstruction and `portfolioForDay` replace current
portfolio FX with complete historical facts. Thus deleting a selected latest
candidate can expose an older unchanged portfolio candidate without incorrectly
moving the historical dirty date back to that older candidate's date.

Agent reference selection admits candidates only on/after their effective date;
all competing candidates, including missing/deleted/unselected alternatives, are
in the authenticated historical inventory. Under the stable direct-pair policy,
no candidate change can affect an earlier civil date. The planner invalidates
**every known rule/instrument**, including explicit/evaluated/open-episode targets,
from the earliest affected OLD/NEW date through latest closed day. There is no
early stop. Existing complete prefix validation, final prefix digest/MAC, row
sealing, source/control/checkpoint CAS, physical connection counters, provider
lease and final injected-clock fences remain in force. Policy and lifecycle
replay still starts at origin with all three-valued votes and handling history.

## Synthetic oracle evidence

`TestReviewDependencyFXMutationFullOracle` uses one USD holding, a CNY cost
predicate, valid agent reference candidates, stable manual preference and an
acknowledged episode. Every case first proves unchanged-source suffix reuse with
selected FX. Eligible changed-source cases additionally require a nonempty
prefix, `direct_fx_suffix`, fewer computed days, and changed **admitted financial
FX evidence**:

| Mutation | Reused days | Recomputed days / full days | Admitted rate change |
| --- | --- | --- | --- |
| Historical insert on Oct 6 | 5 | 6 / 11 | 1 → 3.123456789123 |
| Correction on Oct 8 | 7 | 4 / 11 | 2 → 3.123456789123 |
| Selected-candidate deletion on Oct 8 | 7 | 4 / 11 | 2 → 1 |

The same matrix requires full fallback for date movement, inverse/cross pairs,
mixed instrument/FX changes, an unselected provider candidate, forged journal
dates, a tampered persisted candidate seal, preference changes, selection metadata
changes and reopen. Domain tests independently cover canonical ID ordering, exact
private rate changes, both captured locations, omitted facts, duplicates,
unsupported observations/pairs, incomplete or legacy descriptions and unexplained
source differences. A final-clock mutation of the captured FX description must
reject atomically through the existing source-MAC guard.

`assertSuffixOracle` compares preparation against the full pipeline oracle and
publishes a separate synthetic `VACUUM INTO` copy with `WithFullReviewRecompute`.
Durable comparisons cover daily financial/predicate evidence, episodes, events,
handling and identities. Existing identities stay exact; only newly allocated
independent UUIDs map through their complete semantic keys. The shared row helper
strips `computed_at`, `evaluated_at` and `revised_at`; the separate
`publicationDailyTimes` comparison restores exactly daily-input `computed_at` and
daily-result `evaluated_at`/`revised_at`. This does **not** claim equality of those
timestamps on other tables. No fixture or financial/lifecycle assertion was
removed to enable reuse.

## Validation and performance limits

Focused Linux validation uses Go 1.26.1 with isolated toolchain/cache paths under
`/tmp`. Commands and final results are recorded in the PR. Normal check stays
light; no race run, benchmark matrix, desktop/native build or full-suite campaign
is added here. The initial evidence-only fixture incorrectly combined manual
source with daily-reference observations and failed its required episode
assertion; it was corrected to valid agent candidates without dropping the
assertion.

The counts above prove saved financial/predicate calculations, not latency or
memory targets. Full capture/fingerprinting, additional complete candidate seals,
full protected-row sealing, prefix decoding, exact SQL-image comparison and full
policy/lifecycle replay remain costs. Changed FX invalidates all instruments,
even those with identity/no FX dependencies; long corrections can still require
nearly full history. No p95, RSS, native performance or broad FX-routing acceptance
is claimed. Full recomputation remains the process rollback path.
