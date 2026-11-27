# Synthetic existing-history fixtures

The product never imports this package. Fresh product ledgers must use
`application.Service.CompleteOnboarding`, whose repository transaction creates
an empty immutable origin. Ordinary mutation tests should use that entry point
and recorded changes.

Some replay/analysis/QA fixtures require exact, already-existing opening
balances, quantities, costs, preference observations and timestamps, with no
activities preceding the fixture origin. Build those facts in a disposable
database through the synthetic fixture helpers. The fixture registers its own
temporary DB connection, including SQLite's normal transaction wrapper. Staging
writes form the source facts; an empty synthetic origin is present even during
setup so production methods never rely on an initialized ledger without an
origin. Finalization replaces **only this explicitly registered synthetic
setup** with the requested populated origin in a transaction. The origin and
all subsequent recorded changes are read through the production repository and
replay code. No production migration, compatibility mode, or repair path uses
this mechanism.

The fixture's raw writer is also used for targeted storage/integrity tests to
construct precise synthetic rows. Tests that exercise a real write or reopen
must use a complete valid origin (except explicit rejection tests).
