# Historical data repair — 2026-09-18

## Problem and changes

Backdated deposits and first trades could precede the registration of their accounts,
instruments and holdings. Historical replay excluded those entities using creation
timestamps, even though their ledger facts established earlier economic existence.
A snapshot could consequently report completeness while omitting those components.

- Use activity evidence to include backdated entities, and immutable creation
  observations for their initial preferences and account settings. Later settings
  are not copied backward. Preserve the shared batch's activity slice across days.
- Before an instrument was registered, permit verified historical closes from an
  equivalent registered provider mapping when the initial route has no usable close.
  Require identical symbol, market and currency; reject disabled mappings,
  realtime quotes and future market dates. Normal post-registration as-of routing
  remains unchanged.
- Value the immutable History Origin as an in-memory opening basis when analysis
  begins on the starting date. Do not write a fictitious pre-history close or
  substitute zero for unknown origin valuations.
- Advance observation-slot check times when a provider returns an unchanged price
  or FX reference. Keep quote facts, revisions and input generation unchanged.
- Separate missing coverage from routine correction/no-observation rechecks in
  Data Health. Separate complete monetary amounts from undefined return rates in
  daily results, contribution groups and the UI.
- Attribute internal FX conversion execution differences to FX impact. The two
  currency legs are neutral only at a common reference notional; dropping their
  difference made the asset waterfall fail reconciliation.
- Bump the resolver policy to `market-date-daily-summary-v2`. Existing databases
  use the established policy migration and resumable rebuild, retaining ledger
  facts and prior snapshot revisions.

## Verification

- Regression tests cover a backdated first buy and deposit, multi-day rebuild
  idempotence, protection against later account-setting edits, opening balances,
  equivalent/invalid provider mappings, unchanged price/FX rechecks, zero-capital
  group completeness, frontend warnings, and executed FX spread reconciliation.
- `GOCACHE=/tmp/nestworth-review-gocache wails3 task check`: full Go tests, vet,
  application build, frontend build/bindings, lint, typecheck and frontend tests.
- A SQLite backup of the user's database was rebuilt first. All four closed dates
  (September 14–17) are complete; Data Health has no issues. Asset waterfall sum
  equals the period change exactly at the DTO precision; residual issue count is 0.
- The September 14 opening deposit is a reconciliation adjustment, not investment
  capital. Its return amount is known but its rate is undefined. Rate coverage
  remains 3/4 and is explicitly labeled separately from data completeness.
- Real database accounts, holdings, instruments, activities/effects, trade details,
  account balances, origin components and quotes were compared with the pre-repair
  backup and were unchanged. SQLite quick check returned `ok`.

## Local recovery artifacts

The original database backup is at
`~/Library/Application Support/Nestworth/backups/nestworth-before-history-repair-20260918-133204.db`.
The original installed app is retained at
`/private/tmp/Nestworth-before-history-repair-20260918-133204.app`.
Do not restore only the main file while an app has it open; use the supported
recovery workflow or close all instances before restoring a coherent backup.

Native app verification is recorded after the final package below. Developer ID
signing/notarization and live provider requests are outside this repair validation.

## Final local result

- Full `wails3 task check` passed after the FX attribution fix: all Go packages,
  vet/build, frontend build, bindings, lint, typecheck, and 51 test files / 387 tests.
- `wails3 task package` passed; the ad-hoc signed local v0.3.3 package was installed
  at `/Applications/Nestworth.app` and opened successfully.
- Native UI: Overview shows Data Health checked; Return Calendar restores known
  September 14–17 amounts and a neutral 3/4 rate-coverage explanation; Asset Changes
  no longer shows the waterfall reconciliation error. The final investment change
  includes the executed FX spread and displays CNY 97.50.
- Final copy verification: `AssetChange.Status=ok`, residual issue count 0,
  waterfall total and period change both `29183.0661`; Data Health issue count 0.
- Real database migration/rebuild completed using the app's normal analysis read
  path. All September 14–17 snapshots are complete under resolver policy v2.
- The intermediate application/data state was also backed up before the final
  update, with timestamp `20260918-133721`; use the earlier `133204` backup for the
  original pre-repair data.
- No provider HTTP repair, ledger edits, commit, push or public release was performed.
