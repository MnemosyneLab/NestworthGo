# Account reconciliation implementation plan

Base: main at 759b9882782a3081fa8cf6aac8fe2663d489d60d (PR #88).

## Product contract

A persisted session selects active accounts (all by default), captures cash by
currency, ordinary holdings and managed products from actual contents, and lets
the user explicitly confirm, skip or leave each point unresolved. External
reference/as-of time differs from completion time. Only fully confirmed,
unchanged accounts advance their last successful reconciliation. Completion
freezes the evidence; subsequent edits show recheck indicators without changing
the historical record. Quotes and FX are informational, never confirmation gates.

Corrections use existing audited cash calibration, position adjustment (with
explicit cost input when adding quantity), and transaction/product operations.
Explicit session linkage attributes only intended writes. Agent APIs cannot
invent external evidence or infer confirmation from matching numbers.

## Incremental delivery

1. Plan, dedicated branch, push and immediately open draft PR.
2. Domain/application/storage: additive verified migration; durable sessions,
   per-account scope and item snapshots, revision guards, idempotent mutations,
   immutable completion and content/revision based recheck detection.
3. Wails/MCP: shared application orchestration, bounded/filterable history and
   detail reads, preview/commit and ledger permission parity, explicit correction
   links using current ledger mutation services.
4. GUI: dedicated start/resume/history/detail workflow, account entry points,
   shared controls and formats, explicit reference/evidence/outcome actions,
   existing transaction and calibration forms with return-to-session behavior.
5. Maintained skills/nestworth/SKILL.md, MCP references/examples, GUI and
   architecture docs; integrated regression and supported cloud checks.

Each coherent stage is committed and pushed to the draft PR. No merge, release,
production data, external agent contact or native Mac operations.

## Acceptance coverage

Synthetic isolated SQLite and UI fixtures cover pause/reload/resume, selected
scope, mixed account contents, multiple cash currencies, explicit confirmation,
partial/skipped outcomes, last-success timestamps, frozen snapshots, edits after
confirmation (including backdated changes), adjustment versus transaction routes,
cost invariants, explicit mutation linkage, revision/conflict/idempotency,
pagination/response bounds and MCP read-only/directory/ledger permission parity.

Run Go test/vet/build, focused race tests, frontend lint/typecheck/tests/build,
generated binding checks and maintained skill checks. Document unavailable gates
honestly: Linux builds, jsdom and cloud integration do not constitute native macOS
GUI acceptance. Preserve the existing development/debug bundle identifier.
