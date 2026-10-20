# Engineering Guide

## Current baseline

Nestworth `0.3.8` is the current release, published 2026-10-14, for this Go
1.26 module, with a Wails v3 desktop shell and a React + TypeScript frontend.
The Go domain and application layers own
financial validation, persistence, valuation, replay, cost basis, gains, and
provider routing. The frontend owns presentation, interaction state,
localization, accessibility, and chart rendering.

## Prerequisites

- Go 1.26 or newer;
- Node.js with pnpm;
- Wails CLI `v3.0.0-beta.32`;
- macOS 27.0 or newer on Apple Silicon, with a macOS 27 SDK, for the primary
  desktop and packaging target.
- Linux compile of `./cmd/nestworth` also needs `gcc`, `pkg-config`,
  `libgtk-4-dev`, `libwebkitgtk-6.0-dev`, and `libsoup-3.0-dev`.

Dependency versions are defined by `go.mod`, `go.sum`,
`frontend/package.json`, and `frontend/pnpm-lock.yaml`.

## Setup and daily commands

For the complete copy/paste workflow, see the [Local Development and Packaging
guide](local-workflow.md). The short form is:

From the repository root:

```bash
wails3 task setup
wails3 task dev
```

`frontend/bindings/` contains only generated, gitignored files. After a clean
checkout, generate it with `wails3 task setup` or `wails3 task
generate:bindings`. The direct frontend `dev`, `build`, `typecheck`, and `test`
scripts regenerate it before importing bindings. Run `generate:bindings`
before `check:bindings`: the check requires existing output and compares it
with fresh generator output. The Wails build tasks generate bindings before
building the frontend.

Run the normal automated checks with a writable cache:

```bash
GOCACHE=/tmp/nestworth-go-0.3.8 go test ./...
GOCACHE=/tmp/nestworth-go-0.3.8 go test -race ./...
GOCACHE=/tmp/nestworth-go-0.3.8 go vet ./...
gofmt -l cmd internal
go build ./cmd/nestworth
cd frontend && pnpm run lint && pnpm run typecheck && pnpm run test
```

Generate Wails bindings after changing a bound Go service:

```bash
wails3 task generate:bindings
```

Bindings and frontend bundles are generated artifacts. Do not hand-edit them
or commit them.

Build and verify the primary-target package:

```bash
wails3 task package:release
```

The expected outputs are `dist/macos/Nestworth.app`, the versioned arm64
DMG and ZIP, `dist/macos/SHA256SUMS`, and the standalone skill bundle with
its checksum. Use isolated database and settings paths for a launch smoke;
never point tests at real financial data.

## Repository layout

```text
cmd/nestworth/          Wails process entry point
internal/wailsapi/      Bound services and wire DTOs
internal/application/   Use cases, transactions, and read models
internal/domain/        Entities, value types, invariants, and calculations
internal/infrastructure/SQLite repositories and providers
internal/settings/      Presentation preferences and provider selection
frontend/               React application and generated binding consumer
build/                  Wails Taskfiles and platform packaging metadata
assets/                 Brand and native icon sources
docs/                   Maintained product and engineering contracts
testdata/               Sanitized deterministic compatibility fixtures
```

Keep dependencies pointed inward. The application layer defines orchestration
and ports; infrastructure implements them. Bound services adapt application
results to stable wire DTOs without embedding business rules.

## Financial implementation rules

- Never use binary floating point for persisted values or financial formulas.
- Preserve exact decimal money, quantities, rates, and percentages.
- Missing quotes yield incomplete or unavailable results; never substitute
  zero.
- Multi-row mutations validate and commit atomically.
- Posted financial changes are immutable; correction and reversal operations
  append linked records.
- Derived gain and analytics values are recomputed from authoritative facts.
- Keep current valuation, historical valuation, provider refresh, and replay
  as distinct authorities with explicit tests.

## Frontend rules

See the [shared-control contract](frontend-controls.md) for component discovery,
date semantics, interaction conventions, enforced boundaries and exceptions.

- Pages call generated Wails services through the query/mutation helpers.
- Pages do not open SQLite, construct SQL, call providers, or recalculate
  financial totals.
- Query states must distinguish loading, empty, error, unavailable, and stale
  data where applicable.
- Charts map authoritative values to pixels; they do not derive those values.
- Every interactive feature adds keyboard and localized-label coverage.
- After a mutation, invalidate or reload the authoritative query.

## Persistence and settings

The current SQLite schema is verified before business writes. Startup enables
foreign keys and checks structural integrity. New databases use schema 21.
Verified v15–20 databases receive additive upgrades in order: short names (16),
reconciliation (17), reconciliation evidence/revisions (18), investment review
(19), shadow financial source revision/journal metadata (20), and temporal
review publication checkpoints (21). Each step
retains a verified read-only recovery copy. Schema20 keeps the full financial
fingerprint/recheck algorithm authoritative; see the
[source coverage, recovery and rollback contract](../testing/performance-c1-source-coverage-2026-10-19.md).
C2 shadow capture/publication also compares typed source checkpoints alongside the
full fingerprint; see the [C2 validation boundary](../testing/performance-c2-2026-10-19.md).
This does not enable a source-token fast path or remove full source rechecks.
The [C2 temporal contract](../testing/performance-c2-temporal-2026-10-19.md)
adds a final injected-clock fence, session-bound calendar/rollback invalidation,
and stale status for expired reads. Full financial and future-source rechecks
remain authoritative; C3 is not enabled.
An internal [C3a no-change candidate](../testing/performance-c3-no-change-2026-10-19.md)
is opt-in only for reviewed synthetic experiments. It confirms a complete,
unchanged source/review/temporal checkpoint under the SQLite write reservation,
without reading financial/evidence JSON or rewriting daily/lifecycle rows.
Production initialization keeps C2; forced full recomputation disables the
candidate. Changed-source publication retains all existing rechecks.
The [corrected C3 benchmark evidence](../testing/performance-c3-corrected-2026-10-19.md)
separates bootstrap/full-fallback cost from warm confirmation and measures RSS
in isolated synthetic arm processes. It does not establish native rollout gates.
An opt-in [completeness-lineage shadow](../testing/performance-c3-lineage-shadow-2026-10-19.md)
compares narrow historical coverage and a separate bounded window with a full
preparation oracle, then revalidates under the publication reservation. It
forces full preparation and grants no reuse authority. Normal initialization
remains unchanged; full recomputation disables the internal candidates.
The [certified C3 candidate](../testing/performance-c3-certified-2026-10-19.md)
adds a separate default-off internal constructor, `WithCertifiedReviewCandidate()`.
It binds complete projection integrity to one DB handle, fences explicit provider
selection through commit, and verifies source publication CAS with full fallback.
Same-day confirmation avoids financial/evidence JSON. A missed confirmation uses
one full preparation; `WithCertifiedReviewCandidate()` does not select the separate
lineage/window experiment. Explicitly combining it with
`WithReviewCompletenessLineageShadow(...)` retains the independent full runtime
oracle for bounded experiments. Observation alone can use
`WithReviewLineageObserver(...)` without changing dispatch. Certification verifies
the complete compiled table/index contract as well as all user triggers, including
FK and deferred-action definitions; unknown schema objects disable authority.
Full preparation may omit an individual daily UPSERT only after an authenticated
same-handle publication recheck and exact comparison of its actual SQL parameter
image with the persisted row under the writer reservation. This transaction-local
comparison never supplies computation facts or omits lifecycle/checkpoint work.
Unsafe `writable_schema` forgery of SQLite's stored versus cached schema is outside
the documented SQLite engine/connection trust contract; this is not engine
attestation or an authenticated hostile-database protocol.
Legacy/restarted/damaged projections
cannot acquire reuse authority from a fresh checksum; legitimate handling preserves
only an already valid chain. Production initialization still uses C2, and native,
performance and review gates remain blocking. The old counting-only constructor
is retained for historical experiments, not activation.
Stage-three [dependency/suffix preparation](../design/review-dependency-suffix.md)
is enabled in normal desktop initialization. It uses authenticated quote
OLD/NEW invalidation and exact daily-prefix reuse, with full policy/lifecycle
replay. Non-quote/unknown dependencies and legacy/reopened/restored namespaces
retain full fallback; they do not gain certification. Set
`NESTWORTH_REVIEW_FULL_RECOMPUTE=1` before launch for the full recomputation
rollback path. Earlier default-off statements above describe the historical
C1/C2/C3 slices; they are superseded by this explicitly authorized dispatcher
change, not by a claim of native or stage-two acceptance.
No other legacy migration is supported.
See [reconciliation storage](account-reconciliation.md#schema-18-backup-and-export). Other older, unversioned, future, or invalid databases remain blocked.
See the [narrow storage upgrade](investment-short-names.md#narrow-storage-upgrade)
for verification, transaction, recovery-copy and v15/v16 backup boundaries. Missing current tables and old
constraints fail verification rather than being repaired. Durable settings and
provider credentials live in SQLite; the separate versioned JSON file stores
presentation preferences.

## Validation strategy

| Layer | Required evidence |
| --- | --- |
| Domain | Decimal edges, invariants, and deterministic calculations |
| Application | Use-case validation, transaction outcomes, and read models |
| Infrastructure | Schema, repository ordering, integrity, and provider bounds |
| Frontend | Page state, localization, error handling, and keyboard flows |
| Integration | Isolated SQLite data plus application-level workflows |
| Release | Build metadata, package smoke, and named manual gates |

Use sanitized deterministic fixtures. Provider tests use fakes and must not
depend on live network responses.

GitHub Actions workflow `.github/workflows/check.yml` runs on pushes to
`main`, on pull requests, and on manual dispatch. Its gates are gofmt, binned
non-race `go test`, vet, `go build ./cmd/nestworth`, frontend
build/lint/typecheck/test, and `git diff --check`. Go tests are three parallel
jobs and Vitest is two shards, sized from measured whole-job times so each
lands near 4–5 minutes. The required aggregate check name is `test`. Race detection is not part
of that workflow. `.github/workflows/perf.yml` runs daily at 21:00 UTC and on
manual dispatch. A scheduled run skips the perf and race jobs when the
default branch is still at the last successful scheduled run; the first
scheduled run and every manual dispatch execute them. Performance evidence
is compiled only with `go test -tags perf`
in that workflow (scale, 365-day pipeline, paired samples, and epoch
diagnostics as separate jobs). The same workflow runs the sharded
`go test -race` suite without the perf tag. The race aggregate is `race`.
[ci-pause.md](ci-pause.md) records an earlier manual-only pause; this branch
restores automatic `check` triggers. Native `.app`/DMG packaging, signing, and notarization stay
manual. Wails v3 is pinned at `v3.0.0-beta.32` (`@wailsio/runtime` at
`3.0.0-beta.32`); upgrade those versions only in a dedicated change.

## Documentation and change gate

Update the owning architecture or product document when a stable contract
changes. Maintained documentation is English. Before committing:

```bash
gofmt -l cmd internal
GOCACHE=/tmp/nestworth-go-0.3.8 go test ./...
GOCACHE=/tmp/nestworth-go-0.3.8 go vet ./...
go build ./cmd/nestworth
git diff --check
```

Record unexecuted manual accessibility, signing, notarization, or publication
checks explicitly; passing unit tests does not prove those gates.
