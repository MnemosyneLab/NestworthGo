# Second review batch acceptance — 2026-10-19

Implementation: `4f32a7a67cba2ed51f97baa6110c3b767ea1cce4` on
`fix/review-batch-two`, based on main
`2c585d1c3a26a55c94a5152d2c3f94275969d6b6`. Both PR106
(`79263e58653c005099a7d1a145f2a2944860295a`) and PR107 are verified
ancestors. The following evidence-only commit does not change application code.
Final draft-PR head and exact-head CI are recorded in the PR.

## Confirmed problems and design

- **F12:** field edits invalidated successful previews by sequence, but only
  reset mutations already in error. A pending obsolete mutation could retain its
  pending state and later publish an error. The shared RecordChangeForm now
  resets all three preview observers on draft invalidation, including pending
  observers. Existing sequence, command-identity and unmount guards remain.
  Record, Fix and Repeat share the fix; confirmation recovery is unchanged.
- **F5:** row values and aggregates already come from authoritative read models
  and use two display decimals. Aggregates retain their exact-sum-then-round
  contract. Confirmed paths are holdings (including Account detail), analysis
  rows/totals through the shared AnalysisFilterBar, historical overview, and
  available-funds comparison. The first two use the existing InfoPopover via a
  small shared MoneyRoundingHelp component; the latter two extend existing help
  content. All three translation catalogs are updated. No accounting formula,
  persisted value, API precision, or submitted payload changes.
- **F18:** no production defect reproduced. Both ListCostBasisEvents and
  ReadGainSnapshot already apply validCostBasisEvents before acquisition FX.
  New isolated SQLite tests compare both reads and exercise Undo, acquisition
  correction at a later FX rate, a valid later acquisition, sale, and realized
  gain parity across household/account/instrument/intersection scopes. USD/CNY
  and EUR/CNY cases originally asserted quantity-weighted acquisition FX.
  The 2026-10-29 F03 correction supersedes that arithmetic: native cost
  `24+40 = 64`, base cost `24*7+40*8 = 488`, current value `4*10*9 = 360`,
  unrealized base gain `-128`. Selling one unit leaves cost `48` native /
  `366` base, and still realizes `(30-16)*9 = 126` CNY.

## Automated checks

Commands use Go 1.26.0, Node 24.19.0 and pnpm 12.2.1. Go was installed under
`/tmp` after identifying `/usr/bin/go` as the unrelated board-game executable;
archive SHA256 matched the maintained real-browser guide. Locked dependencies
were installed without lockfile changes. Generated artifacts are not committed.

- F12 controlled deferred promises: failure/edit/retry, pending edit with and
  without a replacement request, old success/error versus newer success/error,
  repeated preview, and unmount in Record/Fix/Repeat. Existing Sheet close,
  command identity, date edits and confirmation recovery coverage remains.
  Six pending-edit cases failed against the original implementation because
  the obsolete pending observer still owned the Preview button. The broad
  baseline name filter also selected an existing date test that timed out under
  concurrent load; that timeout is not counted as a reproduced defect.
- F5 focused formatter/holdings checks: 58 tests passed; positive/negative
  subcent rows and totals retain their canonical values. Additional backend
  tests check exact positive `0.008` and negative `-0.008` gain aggregates from
  two `±0.004` rows. Keyboard tests open/dismiss rounding help and restore focus.
- F18 and subcent backend tests passed, including a targeted race run:
  `go test -race -p 1 ./internal/application -run
  'TestGainReversedAndCorrectedAcquisitionsKeepValidLaterFX|TestSubcentGainRowsKeepExactAggregate'
  -count=1`. This is targeted race evidence, not a full race-suite pass.
- Full frontend suite: **137 files / 1,531 tests passed, no unhandled errors**
  (`node_modules/.bin/vitest run`; [full log](logs/frontend-full.log)). The first
  full run passed all assertions but failed with six unhandled errors from an
  incomplete reconciliation mutation mock. Its reset methods were added; the
  whole suite was rerun successfully, without ignoring errors or weakening tests.
- ESLint, TypeScript, production Vite build and generated-bindings freshness:
  passed. Bindings use `CGO_ENABLED=0 GOFLAGS=-tags=server` in this
  image, matching the documented browser workflow; that generation/check has
  no native-type warnings. Vite reports the existing large-chunk advisory.
- `gofmt -l cmd internal` and `git diff --check`: clean.
- Canonical `go test ./...`, `go vet ./...`, and `go build ./cmd/nestworth`
  were attempted. Native builds are blocked by missing GTK4/WebKit6/libsoup/gio
  development libraries. The full Go test run also failed the existing cold
  performance budgets: 403ms versus 300ms for one month, and 18.9s versus 10s
  for three years, while duplicate broad Go checks and frontend work competed
  for this four-CPU container. An attempted `-tags server` run with CGO still
  enabled retained the native-library block and also exceeded those budgets.
  These are failed full runs, retained in the evidence, not full-suite passes.
  All other Go packages passed, including SQLite and MCP.
- `CGO_ENABLED=0 go test -tags server ./cmd/nestworth`,
  `CGO_ENABLED=0 go vet -tags server ./...`, and
  `CGO_ENABLED=0 go build -tags server,production ... ./cmd/nestworth`: passed.
  Isolated performance diagnostic passed all three unchanged budgets in 3.78s
  (cold month 0.07s; cold three years 3.70s). This supports contention as the
  explanation for the broad-run budget failures; it is **not** a full-suite
  rerun/pass. See [isolated diagnostic](https://github.com/MnemosyneLab/NestworthGo/blob/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d/docs/qa/review-batch-two-2026-10-19/logs/performance-isolated.log).

See [native full-run log](logs/go-full-native.log),
[targeted race log](logs/gain-targeted-race.log), and
[bindings check](logs/bindings-server-check.log). Full exact-head native and
race CI remains a parent review gate; no tests or budgets were weakened.

## Real browser / backend acceptance

The production server binary embeds the actual built frontend and runs real
application services. No frontend API responses were mocked. Temporary seeding
used application.Service APIs against an isolated `/tmp` SQLite database:
**Synthetic second review batch**, two synthetic brokerage accounts holding
one ETF unit each, and a synthetic Checking account initially holding USD 100.
History began before changes. Initial ETF price was 0.0079 and current manual
price 0.004: each value is 0.004 and gain -0.0039; aggregate value is 0.008 and
gain -0.0078. No real ledger, credentials, provider or cloud-backup data was used.
Explicit isolated database/settings/XDG paths and the synthetic household marker
were verified before the accepted run. Database integrity_check returned `ok`.

Chromium 151.0.7922.173, Playwright, 1440×1000, English. Sandboxed launch failed
because the container's Chromium SUID helper is not configured. Browser checks
then explicitly used `chromium_sandbox=False`; no system sandbox configuration
was changed. These checks are not described as sandboxed or native acceptance.
HTTP `/health` returned `{"status":"ok"}`; actual POST `/wails/runtime` calls
and `/wails/events` WebSockets were observed.

| Scenario | Observed result |
| --- | --- |
| Holdings help with keyboard Enter/Escape | PASS; concise rounding explanation, focus restored |
| Exact positive/negative subcent rows | PASS; two visible zero value rows, total 0.01; negative gain hints and total -0.01 retained |
| Record rejected preview → edit → fresh preview → confirm | PASS; invalid -1 rejected, error clears on edit, USD 2 recorded |
| Fix rejected preview → edit → fresh preview → confirm | PASS; -1 rejected, error clears, USD 2 corrected to USD 3 |
| Repeat rejected preview → edit → fresh preview → confirm | PASS; ordinary USD 5 source repeated as USD 4 after invalid -1 recovery |
| Reload / persistence | PASS; USD 4 repeat appears once; five immutable activities (2, reversal, corrected 3, new 5, repeat 4), final cash 112 |
| Analysis rounding help | PASS; shared keyboard-accessible popover visible |
| JavaScript page errors | None in accepted flows |
| Transport failures | Expected HTTP 422 for rejected previews; browser also logged a favicon 404 |

A first automation selected an account option without its currency suffix; a
later automation tried Repeat on a corrected source, which is ineligible.
Those harness attempts were corrected, without changing application behavior.
Pending/out-of-order completions are controlled in jsdom tests, not induced
through browser latency. Existing tiny-value hints can crowd the holdings table
(visible in the row screenshot); row layout is outside this explanation change.

Screenshots:

- [Holdings rounding help](evidence/02-holdings-rounding-help.png)
- [Authoritative subcent rows and aggregate](evidence/03-subcent-rows.png)
- [Rejected Record preview](evidence/04-record-preview-failure.png)
- [Fresh Record preview](evidence/05-record-fresh-preview.png)
- [Fresh Fix preview](evidence/06-fix-fresh-preview.png)
- [Fresh Repeat preview](evidence/07-repeat-fresh-preview.png)
- [Analysis rounding help](evidence/08-analysis-rounding-help.png)

## Remaining gates

Native macOS WebView, window/process close, packaging, accessibility, signing
and release checks were not run. No native bundle was created; subsequent native
QA must use `com.nestworth.dev` and explicit synthetic database/settings paths.
Browser testing does not establish native focus/rendering behavior or OS-dialog
acceptance. No merge or release was performed; parent owns final review and CI.
