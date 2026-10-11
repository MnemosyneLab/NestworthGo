# Cash review and ordinary Repeat validation

Base: `d38cffcfb5bc25af8a5780ccf4929cfde9f626db` (main after PR #51).
Fixtures: synthetic SQLite databases under test temporary directories; mocked
frontend services. No user databases, exports or backups were used.

## Implemented boundary

Cash review covers Holdings-account cash before and after History starts.
Current/target/delta/currency/time come from Go; preview never persists a fact.
After History starts, optional reason text becomes the reconciliation note.
Repeat allows only `cash_in` / `income` and `cash_out` / `expense`, excluding
product, correction and reversal identities. Ordinary account/currency validation
still belongs to the application. Neither feature changes schema or adds templates.

Confirmation retains a server-owned plan. Preview is fenced by the application
write revision and process nonce. Posted retries use durable ledger receipts;
pre-History baseline retries use the same immutable observation ID. Plans are
session-local, expire after ten minutes and have bounded in-memory retention.
Restart recovery requires checking History for an uncertain prior outcome.

## Automated coverage

- Cash: preview does not append; current 100 → target 80 gives -20; baseline
  and History paths; concurrent double confirmation; identical retry;
  intervening cash change; fresh delta after staleness; archived account;
  expiry; committed retry after expiry; baseline confirmation keeps the reviewed
  effective time and records the actual audit time; caller note mutation cannot alter the
  retained command. Post-History confirmations return the persisted observation.
- Repeat: preview does not post; concurrent confirmation returns one Activity;
  another preview creates a distinct Activity without correction/reversal links;
  incoming mutation/holding identity is dropped; archived/missing account;
  changed account currency; unavailable session plan; specialized commands rejected.
- Frontend: cash review fields and optional note; positive delta sign;
  back preserves input; edits discard preview; stale confirmation requires a new
  preview; Escape/cancel/reopen reset drafts; keyboard Preview activation;
  Repeat defaults to a fresh current time; edits remain editable; double-click
  rejection guard; uncertain IPC retry uses the same plan; deleted account blocks
  preview; explicit allowlist excludes unsupported history; locale-key coverage.

## Commands and results

Tooling uses `/workspace/.nestworth-tools/env.sh` (Go 1.26, pinned dependencies,
`GOFLAGS='-mod=readonly -tags=server'`). Native Linux GTK/WebKit development
libraries are absent. CGO-disabled commands therefore validate the server
configuration and generated bindings, not a native desktop build.

- `go test ./internal/wailsapi/holding ./internal/wailsapi/history` — passed.
- `go test ./internal/application -run TestCashPlanExpiryAndImmutableReason` — passed.
- `go test -race ./internal/application -run TestCashPlanExpiryAndImmutableReason` — passed.
- `CGO_ENABLED=0 go test ./...` — all functional packages passed; existing
  `TestAnalysisPerformanceBudgets` failed under concurrent load: cold month
  363ms / 300ms budget, cold 3Y 16.37s / 10s budget. A targeted rerun
  passed cold month but cold 3Y remained 15.65s while other tests were running.
  A final quiet rerun of all performance-budget subtests passed (10.061s total).
  A subsequent complete run again passed functional tests but exceeded the
  existing wall-clock budgets (cold month 340ms; cold 3Y 24.03s). Serial package
  execution (`CGO_ENABLED=0 go test ./... -p 1`) passed the complete suite.
- `go test -race ./internal/application ./internal/wailsapi/history ./internal/wailsapi/holding ./internal/wailsapi/preview` — History and Holding passed (83.498s / 26.685s); application reached the default 10-minute whole-package timeout under load. Active existing tests included historical overview, legacy snapshot coverage/repair and liquidity review. The new cash-plan race test passed independently; full application race acceptance remains outstanding.
- `CGO_ENABLED=0 go vet ./...` — passed.
- `CGO_ENABLED=0 go build -o /tmp/nestworth-server-check ./cmd/nestworth` — passed.
- Frontend full `vitest run --maxWorkers=2` — 822/830 passed, 90 files.
  Eight failures included old/updated expectations and timeout contamination.
  All five affected suites rerun sequentially with `--testTimeout=20000` —
  146/146 passed. Feature keyboard/recovery rerun — 3/3 passed. Final allowlist, locale and
  feature rerun — 25/25 selected tests passed (110 skipped).
- `tsc --noEmit`, `eslint .`, `vite build --mode production` — passed.
- `CGO_ENABLED=0 pnpm run check:bindings` — passed. Generated bindings/bundles
  are excluded from the commit according to the engineering guide.
- `gofmt -l cmd internal`, `git diff --check` — clean.

## Remaining acceptance

Native Wails IPC and macOS desktop launch were not exercised. Manually verify
cash preview and Repeat in en, zh-CN and zh-TW: keyboard/focus return, narrow sheet
layout, exact effective-time readability, Cancel/Back/reopen, stale balance/account
changes, and rapid Confirm/retry. The performance-budget rerun passed after other suites finished. The full
application race suite still needs a dedicated run without the shared-load
timeout. No merge, tag, release packaging or release
publication is part of this change. Independent review and final-SHA comprehensive
acceptance are coordinated by the parent.
