# Available funds branch closeout — 2026-09-28

## Outcome

The available-funds page now leads with the amount available after reservations. Reservations remain optional and use one panel for viewing, adding, editing, and releasing an amount. Existing reservations are preserved.

The source table uses five columns, groups each asset with its account, and states that amounts are in the asset's native currency. A known future receipt is shown as unavailable by the selected date; it is no longer presented as an unknown amount. Filters follow the selected date, and refreshing an estimate preserves the controls and keyboard focus.

## Fixes

- Reservation forms show the source currency, explain their effect, validate positive decimal amounts, keep inputs after failure, and prevent duplicate submissions. Released entries are collapsed into history.
- Deposit forms keep advanced withdrawal rules when toggled. Existing locked products require an explicit current value and cost. Invalid fields receive focus, including fields inside collapsed sections.
- Product details show dates, interest terms, paid-through date, and the exact operation eligible for undo. Receipt previews show the cash, product value, and net-worth changes first; ledger details and optional transaction time are collapsed.
- Financial saves block dismissal and input changes until completion. Failed requests keep the form available for retry.
- Interest paid-through dates cannot move backward. Financial time validation covers the complete product history, including histories over 100 entries. Maximum-size operation pages return a next cursor correctly.
- A proven zero does not require a foreign-exchange quote. Positive amounts that round to zero still retain missing-rate warnings.
- Data health names the account and currency for incomplete cash snapshots, combines consecutive dates for the same component, preserves the issue count, and keeps historical gap navigation scoped to its component and date range.
- English, Simplified Chinese, and Traditional Chinese strings were updated together.

## Real-data validation

The installed database was opened read-only and copied with SQLite's backup API, including committed WAL contents. The baseline contained 2 accounts, 7 holdings, 11 activities, 1 managed product, and 1 reservation. Tests used a separate copy, with provider credentials removed from its settings.

The source database's logical dump matched the baseline after testing. Both passed `PRAGMA integrity_check`. The baseline file hash was unchanged. Private databases, settings, and screenshots remain outside the repository.

### Observed flows

1. **Available funds — PASS.** Opened the real portfolio and compared overview, today, and future amounts. Reviewed the source table, current versus future availability, currencies, filters, and optional reservation details.
2. **Reservations — PASS.** Tried empty submission, added an amount of 50, changed it to 75, and released it. Available funds changed by the exact reserved amount and returned to baseline. Existing reservations and holdings remained intact.
3. **Deposits — PASS.** Settled the existing deposit with interest and a fee, inspected the preview, recorded it, and undid the operation. Also opened a small new deposit and undid its opening. Cash and product value returned to the starting amounts. The automated real-data test additionally checks fees, repeated-request idempotency, read-only previews, and database integrity.
4. **Data health — PASS.** Inspected historical market-data gaps and incomplete snapshots. Improved their labels and grouping. Historical gaps remain genuine missing data; no provider repair or invented historical values were applied to the original database.

## Checks and evidence

- `GOCACHE=/tmp/nestworth-review-gocache wails3 task check` — PASS: frontend build, generated-binding checks, Go tests/vet/build, ESLint, TypeScript, 66 frontend test files / 503 tests, and diff whitespace checks.
- After the final snapshot-focus regression was added: TypeScript, targeted ESLint, production build, and the full frontend suite passed (66 files / 504 tests).
- `GOCACHE=/tmp/nestworth-review-gocache go test -race ./internal/application ./internal/domain ./internal/infrastructure/sqlite` — PASS.
- `NESTWORTH_QA_DATABASE=/tmp/nestworth-branch-qa-20260928/baseline.db GOCACHE=/tmp/nestworth-review-gocache go test ./internal/application -run TestRealDataLiquiditySmoke -count=1 -v` — PASS. The test is skipped unless a backup is explicitly provided, and it makes its own temporary working copy.
- Full frontend tests initially exposed timeouts under excessive jsdom concurrency. Bounding workers to four resolved those failures; the canonical check passed with that configuration.
- UI checks use the production Wails server build on loopback, with the real Go service and copied SQLite data. Native desktop window integration, packaging, and live provider repair were not exercised.
- The final build was inspected in Simplified Chinese, Traditional Chinese, and English. The normal desktop viewport had no page-level horizontal overflow. Snapshot focus was checked against the copied account's SGD component and retained exactly its six affected dates.

Local evidence directory: `/tmp/nestworth-branch-qa-20260928/`.

- `check-final.log`: canonical check.
- `real-data-smoke-final.log`: repeatable real-data smoke test.
- `frontend-tests-final.log` and `go-race-final.log`: final full frontend and race checks.
- `source-verification.json`: original-data and backup integrity checks.
- `screenshots/`: before/after UI observations; contains private portfolio information and is intentionally untracked.
  - `09-funds-final.png`: final available-funds page.
  - `10-funds-english-final.png`: English layout.
  - `11-data-health-final.png`: specific currency and historical-date focus.
  - `12-receipt-preview-final.png`: final receipt preview.

Changes are in the current branch. No push or release was made.
