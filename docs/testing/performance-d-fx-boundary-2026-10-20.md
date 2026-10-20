# D-FX candidate coverage boundary (2026-10-20)

Baseline: `9c21b2de808430e7837dfccecf8edbb30fbb0113` (verified against
`origin/main`). This slice adds synthetic regression evidence only. Runtime
behavior, schema, source authentication and dispatch remain unchanged. No FX
mutation becomes eligible for suffix reuse and no speedup is claimed.

## Evidence and blocker

C1 records FX OLD/NEW scopes, but `readReviewSuffixChanges` decodes only
instrument quote scopes into `ReviewQuoteDependency`. `PlanReviewInvalidation`
rejects FX dependencies. Independently, `ReviewNonQuoteFingerprint` retains FX
in the complete historical batch. `captureReviewSuffix` requires that signed
fingerprint to match before it even plans quote invalidation. These are separate
fail-closed boundaries, not redundant checks to remove.

The authenticated temporal certificate has complete instrument/date quote bucket
seals but no equivalent complete FX candidate description. A selected FX rate in
a daily prefix proves the selected input's integrity, not the absence or contents
of competing candidates. Journal pair/date metadata cannot supply that proof.
An FX planner case or removal of FX from the non-quote fingerprint would therefore
leave an unauthenticated dependency gap. This bounded slice stops before runtime
enablement rather than weakening either boundary.

## Proposed eligibility for a later implementation

The first candidate should permit only one existing direct base/comparison pair,
with explicit valid civil `effective_date` and `daily_reference` observations,
using the existing authoritative selector and admission policy. Provider keys,
preferences, rules and data policy, accounts, activities, ownership, timezone,
cutoff/calendar and all other captured non-FX inputs must remain exactly unchanged.
No inverse/cross route, timestamp-derived economic date, pair/date movement,
selection-policy change, mixed dependency family or incomplete capture is eligible.
Missing candidate coverage and unrecognized metadata must use full preparation.
This is a proposed proof boundary, not a currently enabled policy.

Before eligibility can be enabled, a same-handle authentic full publication must
bind a canonical complete FX candidate inventory from **both** captured FX fact
locations (historical facts and portfolio inputs), including private exact rate
scalars and every captured selection/eligibility field. Compare the complete OLD
and NEW inventories, including deletions and previously unselected candidates;
prove all differences covered independently of the journal. Do not manufacture
this authority from a current checksum after restart, legacy load or restore.
Candidate selection/routing must be proven stable under that comparison, otherwise
fall back. A conservative frontier may invalidate every rule/instrument from the
earliest changed effective date through latest closed day, with no early stop.

Any extension must bind its descriptions and plan through the existing capture
MAC and final prefix MAC, retain complete row sealing, source/physical-connection
CAS, provider lease and injected-clock fences, and replay policy/lifecycle from
origin. It must preserve exact decimals, unknown slots, frozen evidence and
acknowledgement association. A full-oracle shadow must exercise the proposed
planner before it supplies reuse authority. That shadow/planner is not implemented
here; the tests compare current fallback with an independent full oracle.

## Synthetic regression coverage

`TestReviewDependencyFXMutationFullOracle` uses a USD holding, CNY comparison,
agent direct daily-reference candidates with unchanged manual preference, and an acknowledged episode. Each case
first proves an actual unchanged-source suffix hit containing selected FX. It
then changes exactly one FX candidate (plus an instrument quote in the mixed
case), requires no suffix, and checks the existing complete non-quote fingerprint
changed. Cases: insert, exact historical rate correction, deletion, effective-date
movement, inverse pair, cross pair, mixed quote/FX and an unselected provider
candidate. The last case explicitly rejects a fixture that selected the losing
provider candidate in any reused day.

`assertSuffixOracle` compares preparation with the full pipeline oracle, publishes
both the candidate and a separate synthetic `VACUUM INTO` copy using
`WithFullReviewRecompute`, and compares durable rows including daily financial
and predicate evidence, episodes, events, handling and daily timestamps. Existing
identities remain exact; only newly allocated independent UUIDs map through their
complete semantic keys. No financial/lifecycle fields are excluded for this test.

## Validation and limits

On Linux with an isolated Go 1.26.1 toolchain/cache under `/tmp`, these passed:

```sh
go test ./internal/application -run '^TestReviewDependency(FXMutationFullOracle|SuffixExactFXPrefix|SuffixAuthorityAndRollback|SuffixFullDurableOracle)$' -count=1
go test ./internal/domain ./internal/infrastructure/sqlite -run 'Test(PlanReviewInvalidationOldNewAndFallback|ReviewSuffixFingerprintIncludesPrivateFXScalar|ReviewFXEconomicDateBranches|ReviewFXReferenceAgesAndReciprocal|ReviewSuffix)' -count=1
go vet ./internal/application ./internal/domain ./internal/infrastructure/sqlite
gofmt -l cmd internal
git diff --check
```

The application group passed in 8.671s; domain/SQLite in 0.071s/0.083s.
The new eight-case matrix also passed alone in 2.836s before adding the explicit
selected-candidate-ID assertion verified in the final application group. An
initial invalid manual/daily-reference fixture failed its required episode
assertion; it was corrected to valid agent candidates without removing assertions.
Full-suite, desktop build and native gates were not run for this evidence-only
change; focused checks follow the task's light validation budget.

There is no changed-source FX fast path,
no performance sampling, benchmark matrix or race run in this slice. Full source
capture, daily recomputation and complete policy/lifecycle replay remain required
for every FX mutation. Native macOS, packaging, restore and performance acceptance
are not established by this Linux synthetic evidence.
