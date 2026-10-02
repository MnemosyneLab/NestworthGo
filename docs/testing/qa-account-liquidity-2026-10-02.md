# Account and liquidity QA fixes — 2026-10-02

Base: `195a60cc7a7e10f7ce9a9fda8ae859959bb37f98` (main after PR30).
This change addresses QA01–03 from the bounded browser QA run on `defe754`.

## Fixes and regression evidence

| Finding | Cause and fix | Automated coverage | Browser / real IPC evidence |
| --- | --- | --- | --- |
| QA01, P1 | Reservation amount currency was also attached to an `account_value` source during SQLite hydration. Only `account_cash` now carries currency in its source identity. No schema change or stored-data rewrite. | All three source kinds round-trip through SQLite with valid references and unchanged amount currency. A real SQLite/service integration test closes and reopens the database, then checks reserve/edit/release values of 800/750/1000. | A copy of the original failing synthetic USD ledger starts successfully with its existing 200 reserve. Editing to 250 shows 750 available; release and reload show 1000. Account balance remains 1000. |
| QA02, P2 | A pending account update allowed further edits, dismissal and reopening; its completion could close a newer draft. Settings now remain in one pending session, with fields, dismissal, archive and duplicate submissions blocked until the request finishes. Opening a new session clears an old error. | Deferred success and failure cover disabled controls, Escape, repeated submission, preserved failure draft, cancel/reopen and a subsequent save. | A delayed genuine UpdateAccount call locks all tested controls and rejects Escape/backdrop dismissal. Success closes the original sheet; a newly opened draft stays intact. A transport abort preserves the failure draft and allows cancel/reopen/retry. |
| QA03, P2 | Annual-interest editing required a paid-through date hidden in collapsed advanced settings, without invalid-field metadata. The field is visible in the main edit form, explains its meaning, preserves an explicitly chosen date across mode switches, and receives backend-error focus. No date is inferred. | None→annual starts with a blank date; backend validation focuses and marks it invalid; an explicit date is sent on retry. Mode and year-basis changes retain the explicitly chosen date. | Real UpdateProductTerms returns 422 for a missing date and focuses the visible date picker; an explicitly chosen date saves with HTTP 200 and is read back. English and Simplified Chinese checked, including 960×700. |

Screenshots were captured and viewed; the portable evidence report includes screenshots,
ARIA DOM snapshots, IPC responses and queries from synthetic databases. No real ledger,
provider credentials or remote backups were used.

The latest-head icon Escape regression was also checked in Chinese at 960×700:
first Escape closes the icon picker and retains the account draft; the next Escape
closes the sheet; reopening restores saved account values. The full frontend suite
covers the previously fixed account, preview and keyboard flows.

## Local checks

- Frontend: 81 files / 672 tests passed; targeted account/product suites: 76 tests passed.
- Lint, typecheck, bindings generation/check, production frontend build passed.
- `CGO_ENABLED=0 go test -p 1 ./...` and `CGO_ENABLED=0 go vet ./...` passed with the server build tag.
- Server production build, gofmt, `git diff --check`, skill validation and 19 Python tests passed.
- The first concurrently loaded Go test run exceeded the existing cold analysis timing budget (13.57s vs 10s). The complete suite passed when rerun with package concurrency limited to one; no timing thresholds were changed.
- Full native builds and `go test -race ./...` require GTK/WebKit dependencies absent from this workspace. The PR's exact-head Linux CI installs those dependencies. Local affected-package race results and CI results are recorded in the handoff, separately from this document.
- Native macOS/WKWebView, IME and native window behavior remain unverified here.

## Native Mac acceptance

Use the PR's exact head SHA from the handoff. Run in a new test directory, keeping
both database and settings isolated. With the documented Go/pnpm/Wails toolchain:

```bash
NW_QA_DIR=$(mktemp -d "${TMPDIR:-/tmp}/nestworth-qa.XXXXXX")
export NESTWORTH_DATABASE_PATH="$NW_QA_DIR/nestworth.db"
export NESTWORTH_SETTINGS_PATH="$NW_QA_DIR/settings.json"
task dev
```

Keep providers and remote backups disconnected. Use the UI to onboard synthetic
USD data and create a balance-tracked bank account with 1000. Also create a
holdings-capable bank/brokerage account and a synthetic term deposit for QA03.

1. **Reservation persistence:** reserve 200 on the balance account; expect 800 available. Quit/relaunch with the same test paths; the application and reservation manager must open normally. Edit to 250 (750 available), release (1000), and relaunch again. Net worth/account balance must stay 1000 throughout. Smoke-test cash and holding reservations too; only cash source identities include currency.
2. **Pending settings:** edit the bank name and save while the real IPC request is delayed by a debugger or test harness. Name, ownership, inclusion flags, Save, Cancel, X and Archive must be disabled; Escape/backdrop cannot dismiss; Enter must not enqueue a second write. On success, reopen and enter a fresh draft; no earlier callback may close it. Repeat with a controlled transport failure: the draft and error stay visible, controls recover, and retry works. Cancel/reopen must clear the previous error and show saved values.
3. **Interest date:** open a deposit whose interest is not provided, switch to annual rate and enter 3.5. Paid-through must be visible while advanced settings remain collapsed, and remain blank until explicitly selected. Save should identify and focus that date. Choose an appropriate date within start/maturity, save, reopen and confirm persistence. Switch away/back without saving and change 365/360: preserve the explicitly selected date. Clearing it must again yield the visible date error; do not silently assume interest was received.
4. **Keyboard/window regression:** at approximately 960×700 in Simplified/Traditional Chinese, use Tab/Shift-Tab, Escape and Enter. Confirm footer actions remain reachable and date/field focus works. In account settings, open the icon picker with a Chinese unsaved name: first Escape retains the draft, second closes the sheet, reopening restores saved data. Check a Chinese IME composition commit followed by Enter does not submit unexpectedly.

Record macOS version, architecture, exact head SHA, pass/fail and a screenshot or
native IPC trace for any discrepancy. Cloud browser evidence is not a native Mac sign-off.
