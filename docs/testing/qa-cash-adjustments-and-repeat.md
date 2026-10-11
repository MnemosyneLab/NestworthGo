# Cash review and ordinary Repeat validation

Base: `d38cffcfb5bc25af8a5780ccf4929cfde9f626db` (main after PR #51).
Fixtures: synthetic SQLite databases under test temporary directories; mocked
frontend services. No user databases, exports or backups were used.

## Implemented boundary

Cash review covers Holdings-account cash before and after History starts.
Current/target/delta/currency/time come from Go; preview never persists a fact.
After History starts, optional reason text becomes the reconciliation note.
Repeat allows only `cash_in` / `income` and `cash_out` / `expense`, excluding
product, correction and reversal identities. Source eligibility is authoritative
across filters/pages and rechecked under preview/commit gates; the source ID
is read-only metadata and never becomes new transaction identity. Ordinary account/currency validation
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
  changed account currency; unavailable session plan; specialized commands rejected;
  missing/specialized/reversed sources rejected; source-type switching rejected;
  fresh expense; source reversal after opening/preview; receipt replay after source
  reversal; current-token confirmation still checks source eligibility.
- Frontend: cash review fields and optional note; positive delta sign;
  back preserves input; edits discard preview; stale confirmation requires a new
  preview; Escape/cancel/reopen reset drafts; keyboard Preview activation;
  Repeat defaults to a fresh current time; edits remain editable; double-click
  rejection guard; uncertain IPC retry uses the same plan; deleted account blocks
  preview; explicit allowlist excludes unsupported history; filtered and unloaded
  reversals suppress Repeat; source reversed after draft opening cannot preview;
  locale-key coverage.

## Commands and results

Tooling uses `/workspace/.nestworth-tools/env.sh` (Go 1.26, pinned dependencies,
`GOFLAGS='-mod=readonly -tags=server'`). Native Linux GTK/WebKit development
libraries are absent. CGO-disabled commands therefore validate the server
configuration and generated bindings, not a native desktop build.

- `go test ./internal/wailsapi/holding ./internal/wailsapi/history` — passed.
- Follow-up `go test ./internal/wailsapi/history ./internal/application
  -run 'TestRepeat|TestCashPlan' -count=1` — passed, including the new
  source eligibility and receipt regressions.
- Follow-up complete `CGO_ENABLED=0 go test ./... -p 1`, `CGO_ENABLED=0
  go vet ./...`, and server build — passed after the lifecycle fix. Application
  tests completed in 45.307s; no wall-clock budget failure.
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
- `go test -race ./internal/application ./internal/wailsapi/history ./internal/wailsapi/holding ./internal/wailsapi/preview` — History and Holding passed (83.498s / 26.685s); application reached the default 10-minute whole-package timeout under load. Active existing tests included historical overview, legacy snapshot coverage/repair and liquidity review. The new cash-plan race test passed independently. The follow-up CI-style application shards below supersede this incomplete run.
- `CGO_ENABLED=0 go vet ./...` — passed.
- `CGO_ENABLED=0 go build -o /tmp/nestworth-server-check ./cmd/nestworth` — passed.
- Frontend full `vitest run --maxWorkers=2` — 822/830 passed, 90 files.
  Eight failures included two incorrect expectations and six timing failures
  while frontend and Go/race/build jobs competed for CPU.
  All five affected suites rerun sequentially with `--testTimeout=20000` —
  146/146 passed. Feature keyboard/recovery rerun — 3/3 passed. Final allowlist, locale and
  feature rerun — 25/25 selected tests passed (110 skipped).
- Follow-up full `pnpm --dir frontend exec vitest run --maxWorkers=1` —
  833/833 tests, 90/90 files passed in 415.70s, default 5-second test timeout.
  No competing Go jobs ran. Node 24.19.0, pnpm 12.2.1; 4-CPU / 16-GiB container.
- `tsc --noEmit`, `eslint .`, `vite build --mode production` — passed.
- `CGO_ENABLED=0 pnpm run check:bindings` — passed. Generated bindings/bundles
  are excluded from the commit according to the engineering guide.
- `gofmt -l cmd internal`, `git diff --check` — clean.

## Initial frontend failure investigation

The initial run used two workers while Go/race/build jobs also ran. The
checkout fixes the two expectation failures; no production change was needed
for the six timing failures. The individual failure names and symptoms were:

| Suite | Test | Initial symptom |
| --- | --- | --- |
| App | retains the upcoming filter and restores its trigger after Back | Async `Reservation issues` button was not found. |
| keyboardOnly | cycles past the final settings action in AppShell without losing keyboard focus | Default 5-second timeout. |
| AccountsPage | completes the CMB mixed-account first fund buy journey | Obsolete `Save` expectation; cash now uses Preview → Confirm. |
| AccountsPage | opens cash dividend from a holding row with that holding preselected and submits the existing command | Default 5-second timeout. |
| AccountsPage | opens cash dividend from the account action and only offers that account's holdings | Default 5-second timeout. |
| HistoryPage | validates adjustment unit costs before IPC and accepts an exact eight-place correction | Default 5-second timeout. |
| HistoryPage | Repeat opens a fresh editable draft, invalidates edits/back and confirms the latest plan once | New test expected an empty effective timestamp; existing form defaults use a fresh exact timestamp. |
| InstrumentManagement | selects CoinGecko coin IDs and a non-USD quote currency | Async Bitcoin option was not found. |

Controlled follow-up: all seven existing failure cases passed on base
`d38cffc` (5 files; 7 selected, 136 skipped; 46.84s). The new Repeat case has no
base counterpart. The complete updated-head run above passed all eight cases,
including the updated cash journey and corrected Repeat assertion. The six
timing failures did not reproduce in either controlled run; contention is
consistent with their symptoms, rather than a demonstrated product regression.

The original broad race command omitted CI's `-p 1`, `-parallel 4` and two-part
application selection. At its 600.346s timeout, active stacks were SQLite
migration/schema verification (historical overview and liquidity review),
historical snapshot batch reads/repair, and a parent waiting on its batch
subtest. Many other tests were waiting to enter `t.Parallel`. No race report
was emitted. `.github/workflows/check.yml` documents the same known baseline
whole-package timeout in PR #44 (600.030s) and splits application and MCP into
two independently timed processes; unrelated packages use their own shards.
The follow-up uses the same selector and flags, sequentially with no competing
workload:

```sh
for part in 0 1; do
  regex=$(bash tools/race-shard-select.sh "$part" 2 ./internal/application)
  go test -race -p 1 -parallel 4 -run "$regex" ./internal/application
done
```

Both application parts passed: 234/497 runnable targets in 301.514s and
263/497 in 224.201s, covering all 497 targets without overlap. No race report
was emitted. Full API race checks also passed with
`go test -race -p 1 ./internal/wailsapi/history ./internal/wailsapi/holding
./internal/wailsapi/preview`: History 47.386s, Holding 8.515s; the generic preview
package has no standalone tests and is exercised through both API suites.

## Remaining acceptance

Native Wails IPC and macOS desktop launch were not exercised. Manually verify
cash preview and Repeat in en, zh-CN and zh-TW: keyboard/focus return, narrow sheet
layout, exact effective-time readability, Cancel/Back/reopen, stale balance/account
changes, and rapid Confirm/retry. No merge, tag, release packaging or release
publication is part of this change. Independent review, CI verification and
native GUI acceptance remain outstanding.
