# Repository guidance

Read `docs/development/engineering-guide.md` before changes and
`docs/development/local-workflow.md` for commands and platform gates. Preserve
financial, generated-artifact and isolated-data rules in those documents.
No repository `.agents/skills` is currently present; `skills/nestworth` is the
end-user ledger skill, not a development workflow.

For frontend work, read `frontend/AGENTS.md`. Discover existing components and
nearby tested call sites before creating controls. Fix recurring issues at the
owning shared component and add a practical regression or static guard. Keep
exceptions explicit; do not redesign unrelated screens or move business rules
into presentation code. Report native macOS verification separately from Linux
builds, jsdom tests and browser checks.

## Workspace and delivery

Inspect the branch, SHA and uncommitted changes before work. Preserve user and
concurrent work; never overwrite, reset or clean unrelated changes. Use isolated
workspaces for independent investigations; do not repurpose ongoing tasks.
Honor explicit stage boundaries (review, replacement, verification). A one-off
instruction is not permanent policy. Keep private personal, account and agent
coordination instructions and custom permission rules out of the repository.

Commit messages must follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/#summary):
a type, optional scope and description; mark breaking changes with `!` or a
`BREAKING CHANGE` footer.

PRs explain scope, environment, checks passed, unverified items and performance
impact. Tie test evidence to the exact head, not an older green commit.
Commit, push and PR creation do not authorize merging. Preserve necessary
regression assertions; explain invalid fixture assumptions when correcting them.

## Checks and performance evidence

Keep ordinary PR checks lightweight; race, stress and large performance evidence
belong in `perf`. During development, run affected checks first and expand when
needed for correctness. Do not mechanically repeat whole suites, race runs or
benchmarks. Use minimal effective synthetic fixtures and opt-in heavy experiments
without skipping necessary correctness coverage; explicitly requested performance
runs remain supported. See the guides for commands and CI boundaries.

Review CI elapsed time on PRs. When it materially increases, distinguish queue,
setup, build, cache and test time and identify slow jobs. Do not mask problems by
merely raising timeouts or deleting assertions. CI monitoring belongs to the
coordinator; hand off exact head/run links and remaining items instead of keeping
costly coding environments occupied solely waiting for CI.

Start performance exploration with small controlled paired samples for qualitative
direction; increase samples only when justified. Record configuration, environment,
data scale and actual path activation. Distinguish cold start, reopened databases
and same-process warm caches; report latency, allocation and RSS separately.
Microbenchmarks or small samples do not establish global or p95 claims. Mac claims
require native Mac evidence, not Linux substitution.

Performance changes must preserve exact financial results, provenance and date
semantics, transactions, invalidation, authentication and safe fallback. JSON,
cache and persistence changes must account for the exact bytes used by
fingerprints, HMACs, certificates and CAS. Never change fixtures or expected values
merely to bless a broken contract. Describe implemented, default-enabled,
actually-hit and deployed states separately. Validated positive optimizations
should reach approved normal usage; document specific reasons for disabled paths.
Never blindly enable incomplete, conflicting or unsafe experiments.

## UI consistency

Prefer existing shared date pickers, selects, inputs, buttons and formatters when
they suffice; explain justified custom or raw-browser-control exceptions. Review
component reuse and interaction consistency as well as functional output.
User-facing GUI labels use readable business names, not raw UUIDs, primary keys,
observation IDs or machine codes, except for deliberate diagnostic uses. MCP and
diagnostics may require identifiers. Shared money/numeric formatting follows
currency and scenario rules, not blanket two-decimal formatting; display rounding
must never alter stored exact values.

## Local test application identity and data isolation

All local testing, debug builds and native macOS QA must use the established
`build/darwin/Info.dev.plist` identity: bundle identifier `com.nestworth.dev`,
application name and display name `Nestworth` (bundle `Nestworth.app`). Do not
create task-specific QA names, random bundle identifiers or additional testing
identities. Production packaging retains its separate release identity.

Isolate each test run through explicit synthetic database/settings directories,
not a new app identity. Before launch, verify the bundle's dev identity, resolve
`NESTWORTH_DATABASE_PATH` and `NESTWORTH_SETTINGS_PATH` to the intended isolated
run directory, and verify a synthetic test marker/household in that database.
Follow the reusable launch and acceptance steps in
[local-workflow.md](docs/development/local-workflow.md#isolated-native-development-qa).
Use a launcher/configuration that preserves these paths when opened by native UI
tools; an app launch without verified overrides is prohibited. Never use or
modify a real household ledger for tests. Do not change system security settings
or automatically remove previously created QA applications.

If an accepted authorization flow stalls or repeats, stop requesting the same
permission. Preserve completed work and logs, mark blocked checks accurately,
and proceed with available verification and the authorized commit workflow.
