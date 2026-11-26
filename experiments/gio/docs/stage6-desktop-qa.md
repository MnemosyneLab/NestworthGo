# Stage 6 paired desktop acceptance

## macOS acceptance on November 26, 2026

Stock arm64 macOS Gio and Wails bundles used separate writable copies of each
synthetic fixture. Snapshot repair ran at `e250e551580409d62c1b4c3aad9fee87b044b10b`.
The Gio Wealth History and scoped Contribution runs, both portfolio Contribution
runs, and both provider settings runs used
`9c80ee9b3d4ee21ff8f2a49ce3cd664a98fded74`. The Wails Wealth History and
scoped Contribution references used `e250e55`; their application code is
identical at both revisions.

| Workflow | Packaged macOS result |
| --- | --- |
| Wealth History → Ledger | Both apps captured October 31 → November 25 and October 1 → captured current. The October 31 and November 25 Harbor Brokerage detail showed USD 225.43 at each endpoint. Ledger opened for Harbor Brokerage and those dates. Back restored the account evidence in both apps and focused the Ledger action in Gio. |
| Scoped Contribution → Ledger | October Dividend and interest for EURO, Harbor Brokerage, native EUR and excluded cash showed EUR 12.35. The detail omitted rate fields. Ledger showed the exact EUR 12.3456 receipt on October 2. Back retained the filters in both apps and focused the EURO row in Gio. |
| Currency Contribution → Ledger | October Total return grouped by currency showed EUR 94.47 and +27.76% for EUR. Both details disclosed that Ledger opens for the whole selected period. Ledger showed both brokerages' dividend records. Back restored the breakdown in both apps and focused the EUR row in Gio. |
| Dated snapshot repair | Escape left the snapshot, component, history state and financial revision readbacks byte-for-byte identical to the pre-preview readback. Confirmation appended only October 13 revision 2, complete at USD 125.25. All 45 prior revisions remained identical. Both apps reopened with only the adjacent October 14 finding. |
| Provider prerequisite | Both apps opened Settings → Market data from the CoinGecko finding. Gio focused the API key field, restored focus to Open provider settings when returning, and reopened the field with Return. The key stayed unset in both settings files. |

All three fixture pairs matched their source across 23 authoritative ledger,
quote and history-origin tables after the run. Automated independent-copy
tests exercise CoinGecko key save and removal with a synthetic token and zero
provider HTTP requests. Integrated provider authentication, VoiceOver, IME
and Wayland remain open.

## macOS card statements, market performance and refresh

The second arm64 macOS run used independent copies of four synthetic ledgers.
The stock Gio card build was `4b314536b6f8fc9e75a92a7f731b0a700210983c`.
The stock Gio instrument performance build was
`22ac62fa5deb3eed04c41058de67b32224e98ad8`. The stock Wails reference and both
Asset Changes bundles were built at `9c80ee9b3d4ee21ff8f2a49ce3cd664a98fded74`.
Both mixed-refresh bundles used `d8cad269f28f62160b6ab2df357b7ee349514531`
with in-process provider substitutes. The Wails application and shared quote
and analysis services were unchanged between these revisions.

| Workflow | Packaged macOS result |
| --- | --- |
| Credit-card statement → journal | Both account entries opened the same one-card session. Cancelling the draft or preview preserved the balance, activity, linked evidence and receipts. A USD 2.3456 synthetic purchase moved the signed balance from −USD 42.3456 to −USD 44.6912 with one additional linked activity. The other account stayed at USD 100. Reopen recovered both journal entries. Gio opened the exact new receipt, then Back restored the receipt row and the original card statement action. |
| Instrument market performance | Both stock apps showed PERF up 20.00% over October 26–November 25 and up 50.00% over all saved history. The EUR close table retained the exact 80 and 120 endpoints. The EUR 999 intraday observation stayed outside the performance series. Both showed unavailable 30-day performance for SPARSE alongside its saved observations. Gio Forward restored the selected All range and open table. From the Performance Brokerage holding, Back restored the initiating link and Forward reopened that scoped instrument. Performance and Asset Changes preserved the checked authoritative ledger and quote tables. |
| Current instrument and FX refresh | Cancel stored zero quotes. The next attempt saved USD 17.12345678 from Yahoo Finance and JPY/USD 140.4321 from Frankfurter, and reported SGD/USD failed. Retry added only SGD/USD 1.3456. The two earlier observations kept their original timestamps. Both apps showed the exact price and provider in Quote history and displayed both FX values after reopen. Each local substitute used its app's clock for the quote timestamp. |
| Return Calendar → Asset Changes → Contribution | Both returned the same October 2 portfolio price change and contribution of USD 93.75. The selected day survived both handoffs. Back restored the original October calendar and Gio restored focus to the price driver and the day button. |

A separate Gio regression fixes the clock at December 1, 2026 16:30 UTC
(December 2 in Singapore). With a November 20 History Origin, the shortcuts
select November 30, December 1 and November 20. A December 1 Origin clamps
the initial capture to December 1 and disables the month-end shortcut.

## macOS account-filtered Return and Ledger chain

Both stock arm64 macOS windows were built at
`8d9884410caab2b95928afe9cb66e200e6d38f3a`. Independent copies of the
`contribution` seed used Instrument scope, EURO, Harbor Brokerage, EUR,
native valuation and excluded cash. On October 2, the Return Calendar
showed EUR 37.35 with EUR 25.00 of price change and EUR 12.35 of dividends.
Asset Changes retained the day and all filters, and showed EUR 100.00 →
EUR 125.00. The price driver opened one-day Total return Contribution,
which retained the EUR 37.3456 exact shared result.

Both Ledger timelines retained October 2, Harbor and EURO, and displayed
only the EUR 12.3456 dividend. The receipt showed 21:00 Asia/Singapore and
the same note. Fourteen checked authoritative SQLite tables matched the
source. Back preserved each page and its scope in both apps. Gio restored
focus in turn to the receipt row, EURO contribution row, Price change driver,
and Calendar day. Wails restored the main content container on leaving a
closed analysis sheet.

The separate 665 × 480 Gio frame regression deliberately delays the Asset
Changes reload. The base lost its saved 647 px scroll position while loading.
At `7ee8f37` the viewport and visible driver focus recover when the data
arrives, and Return reopens the same driver. This timing regression uses
the paired shared Go results. Its artificial delay runs only in the frame
test.

## macOS standalone bank expense amendments

The stock Gio bundle at `dfadb661648e31305123b2265105f769f1d6d670` and
the Wails reference at `8d98844` used byte-identical copies of one
synthetic bank ledger. Wails opened Fix and Undo from the activity menu.
Gio opened Correct amount and Reverse activity from the exact receipt.
Cancelling a reviewed correction preserved the activity, effects,
correction groups, bank values and reconciliation links.

Replacing USD 12.3456 with USD 10.2345 retained the expense reason, note and
original effective instant. Reversing a separate USD 4.5678 expense appended
its exact inverse. SQLite readback after normal quit found three additional
activities, retained both original records and the unrelated linked
expense, and agreed on a final bank balance of USD 88.531. The independent
Go regression also verifies the intermediate USD 83.9632 balance.

## Reproduce the automated pair

From the repository root, run:

```sh
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run 'Test(Stage6PairedMixedLatestRefresh|Stage6PairedReturnCalendarFilteredKeyboardHandoff|Stage6PairedContributionFindsExactLedgerRecord|DataHealthSnapshotRebuildRequiresConfirmationAndAppendsOnlySelectedRevision|DataHealthSnapshotStopAndRetryKeepCompletedBatches)$')
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run 'Test(WealthCapture|Stage6PairedContributionShowsFullLedgerScope|DataHealthCredentialHandoffRechecksSameProviderOnBothDesktops)')
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run 'Test(CreditCardStatementHandoffReusesLedgerAndJournal|PortfolioInstrumentPerformanceMatchesWails|InstrumentPerformanceChartRespectsPriceBasisAndDenomination)')
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run '^TestStage6PairedAccountFilteredAnalysisRoundTrip$')
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run '^TestLinkedLockedProduct(Settlement|Terms|Renewal)MatchesWails$')
go test ./internal/wailsapi/marketdata ./internal/wailsapi/history ./internal/wailsapi/quote -count=1
go test ./internal/wailsapi/analysis -count=1
(cd frontend && TZ=Asia/Shanghai pnpm exec vitest run src/features/data-health src/features/marketdata)
(cd frontend && TZ=Asia/Shanghai pnpm exec vitest run \
  src/features/insights/ReturnAnalysisPage.test.tsx src/features/insights/AssetChangesPage.test.tsx \
  src/features/insights/AnalysisFilterBar.test.tsx)
(cd frontend && TZ=Asia/Shanghai pnpm exec vitest run \
  src/features/insights/insightTabs.test.tsx src/features/history/HistoryPage.test.tsx)
```

Each paired test checkpoints and closes a seeded SQLite database, then gives
Gio and the Wails Go adapter independent copies. The Data Health test compares
the date, superseded revision, exact USD 125.25 and component provenance after
normal reopen. The adjacent November 23 revision is identical to the source.
The scripted multi-batch test separately exercises stop, a failed batch and
retry. The exact-date repair uses stored local facts and makes no provider
requests.

The mixed latest test has a USD instrument, JPY/USD and SGD/USD. The first
JPY request waits for cancellation. A second run saves USD 17.12345678 from
Yahoo and a JPY/USD rate of 140.4321 from Frankfurter, and reports a provider failure
for SGD/USD. The retry fetches SGD/USD at 1.3456 and reuses the
successful quotes. The test compares each target status through the Wails
completion event, verifies Quote history provenance, and reopens both copies.
Both providers are local test doubles.

The Return Calendar pair uses the same synthetic fund in two accounts.
Harbor Brokerage holds 10 units and Second Brokerage holds 20. An explicit
EUR rate of 1.25 USD makes the native amount distinguishable from base
valuation. The exact October 2 native return and linked Asset Changes are
EUR 25 for Harbor alone. The Gio regression verifies the typed month and
day requests, focus after Escape and Back, and reopening the same day with
Return. The Wails frontend test verifies keyboard activation and the
navigation payload through a mocked desktop binding.

The Contribution pair adds dividend receipts for both accounts and a
cash-only record for Harbor. The exact Harbor dividend is EUR 12.3456.
Contribution and Ledger agree on the date, account, instrument and exact
receipt ID. The live hint has no activity-kind restriction for this metric.
The Wails frontend references also exercise carrying kinds when supplied.

## Stock macOS pair on synthetic data

Use the [macOS development prerequisites](../../../docs/development/local-workflow.md#prerequisites).
From the repository root, export fresh SQLite fixtures and build both apps:

```sh
qa=$(mktemp -d "${TMPDIR:-/tmp}/nestworth-stage6-qa.XXXXXX")
(
  cd experiments/gio
  NESTWORTH_STAGE6_QA_DIR="$qa" go test ./internal/desktop \
    -run '^TestExportStage6DesktopFixtures$' -count=1 -v
) | tee "$qa/export.log"
fixture=$(sed -n 's/.*fixtures=\([^ ]*\).*/\1/p' "$qa/export.log")
test -d "$fixture/snapshots/gio" && test -d "$fixture/snapshots/wails"
git rev-parse HEAD
wails3 task build
sh experiments/gio/preview-macos.sh --build-only
```

The exporter prints the selected and adjacent dates. It puts the selected
dirty date outside Overview's startup trend, which can materialize recent
derived snapshots. The named profiles below specify the expected synthetic
household.
Each scenario has a `source`
directory and separate writable `gio` and `wails` directories. Keep the
source closed in the apps so it remains a baseline.

Use Wails' established development identity and direct launch to pass the
synthetic paths to the bundled executable:

```sh
test -n "$fixture" && test -f "$fixture/snapshots/wails/nestworth.db"
wails_bundle="$qa/Nestworth.app"
gio_bundle="$PWD/bin/gio/Nestworth Gio.app"
mkdir -p "$wails_bundle/Contents/MacOS" "$wails_bundle/Contents/Resources"
cp build/darwin/Info.dev.plist "$wails_bundle/Contents/Info.plist"
cp bin/nestworth "$wails_bundle/Contents/MacOS/Nestworth"
cp assets/icons/icon.icns "$wails_bundle/Contents/Resources/icon.icns"
codesign --force --deep --sign - "$wails_bundle"

check_pair_identity() {
  test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$wails_bundle/Contents/Info.plist")" = com.nestworth.dev &&
  test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$gio_bundle/Contents/Info.plist")" = com.nestworth.gio
}

run_copy() (
  set -eu
  check_pair_identity
  scenario=$1 app=$2
  case "$scenario" in
    snapshots) household='Synthetic Stage 6 snapshots' ;;
    market-readback) household='Synthetic Stage 6 market' ;;
    provider-settings) household='Synthetic provider readiness' ;;
    return-calendar|contribution) household='Synthetic Stage 6 return calendar' ;;
    early-deposit-receipt) household='Synthetic term deposits' ;;
    funds-reservations) household='Synthetic funds' ;;
    card-statement) household='Synthetic Stage 6 card statement' ;;
    locked-settlement|locked-terms|locked-renewal|add-product|funds-product|account-product|standalone-locked-lifecycle|standalone-interest-terms|linked-cash-transfer|linked-debt-value|multi-account-reconciliation|check-account|deposit-corrections|product-operation-history) household='Synthetic first buy' ;;
    *) exit 1 ;;
  esac
  case "$app" in
    wails) executable="$wails_bundle/Contents/MacOS/Nestworth" ;;
    gio) executable="$gio_bundle/Contents/MacOS/Nestworth Gio" ;;
    *) exit 1 ;;
  esac
  copy=$(cd "$fixture/$scenario/$app" && pwd -P)
  export NESTWORTH_DATABASE_PATH="$copy/nestworth.db"
  export NESTWORTH_SETTINGS_PATH="$copy/settings.json"
  test -f "$NESTWORTH_SETTINGS_PATH"
  # The exporter checkpoints and closes the fixture. A first launch may
  # have a WAL-mode header before SQLite has created either sidecar.
  marker_db=$NESTWORTH_DATABASE_PATH
  if test ! -e "$NESTWORTH_DATABASE_PATH-wal"; then
    marker_db="file:$NESTWORTH_DATABASE_PATH?immutable=1"
  fi
  test "$(/usr/bin/sqlite3 -readonly "$marker_db" 'select name from households')" = "$household"
  exec "$executable"
)
```

- Run `run_copy funds-reservations gio` and `run_copy funds-reservations wails`.
  Open Available Funds, select a custom future date, and open Manage
  reservations from the asset list. Cancel an edit to Rent, then reopen it
  and save 50.1234 USD. Confirm Harbor Savings shows 75.3087 USD available
  after reservations on the same selected date. Add a 4.9234 EUR reservation
  on Travel Wallet cash and confirm 20.3266 EUR remains. Release Old reserve
  from the global manager and check that the unresolved item disappears.
  Compare the values after quitting and reopening each app. Keep the original
  screenshots of the manager, edit form, and source balances for each app.
- Run `run_copy funds-product gio` and `run_copy funds-product wails`.
  Open Synthetic locked product from Available Funds. Record a receipt of
  132.1234 USD with a fee of 1.2222 USD and keep the product detail open.
  Open Grouped undo, compare the cash preview, cancel and verify the receipt
  is still listed. Reopen and confirm the undo. The original operation is
  marked reversed, the contract reopens and the USD cash balance returns to
  156. Check these values after reopening each ledger and keep the screenshots.
- Run `run_copy snapshots gio` and `run_copy snapshots wails` one at a time.
  Open Data Health from Overview. Inspect the outdated snapshot and open
  the one-day rebuild sheet. Check its date against the exporter output.
  Dismiss before confirmation, then reopen the sheet from the same action.
  Confirm only that date, recheck, quit and start the same copy again.
  Gio supports stop after the current batch. Wails disables dismissal
  while a batch is running and offers Retry after failure. The automated
  tests cover the induced failure and retry.
- Compare the snapshots after each normal quit. The inspection query below
  should return one extra line for the printed repair date, with revision 2
  and USD 125.25. The adjacent date and all other revisions should be
  unchanged. Compare the results before confirming to verify that dismissal
  left the same rows. Persistent revision IDs and recorded times differ
  across independent apps.

```sh
for copy in source gio wails; do
  /usr/bin/sqlite3 -readonly "$fixture/snapshots/$copy/nestworth.db" '
    SELECT local_date, revision, net_worth_amount, currency, complete, resolver_policy_version
    FROM daily_valuation_snapshots ORDER BY local_date, revision;
  ' > "$qa/snapshots-$copy.tsv"
done
diff -u "$qa/snapshots-source.tsv" "$qa/snapshots-gio.tsv"
diff -u "$qa/snapshots-source.tsv" "$qa/snapshots-wails.tsv"
```

- Run `run_copy market-readback gio` and `run_copy market-readback wails`.
  Open Market Data and the synthetic instrument's Quote history. Verify the
  exact USD 17.12345678, saved Yahoo source and timestamp. Compare the
  JPY/USD observation and the empty SGD/USD row. Check Back and focus at
  normal and compact sizes, then reopen each app and compare the same quotes.
- The stock bundles register their ordinary production providers. The seeded
  readback profile supports the saved-quote walkthrough without provider
  calls. For cancellation, partial refresh and failed-target retry, use the
  automated pair above, or build dedicated QA binaries with only the
  provider registry replaced by the same deterministic doubles. The
  `market` fixture is the pre-refresh database for that test build. Keep
  the stock run to the saved-data screens and snapshot repair.

- Run `run_copy return-calendar gio` and `run_copy return-calendar wails`.
  Open Return Analysis and go to the previous calendar month. Select
  Instrument scope, Synthetic euro fund (EURO), Harbor Brokerage and EUR
  as the additional filters, Native currency, and disable Include cash.
  The exporter prints `return_calendar_day`. Focus that date and open it
  with Return. Confirm EUR 25 and 25% in the day details. Focus **View asset
  changes for this day** and press Return. Verify the same date in both
  date fields, the same filters, and a price change from EUR 100 to EUR 125.
  Go Back and check the original month, filters and day focus. Press Return
  again to verify the same day opens. Repeat at a compact window size.

- Run `run_copy contribution gio` and `run_copy contribution wails`. Set
  the same scope, previous month and filters as in the Calendar check.
  Open Contribution and choose **Dividend and interest**, grouped by
  Instrument. Select **Negative contributions** and search for `EURO` in
  the separate Details list. The positive EURO result remains in that
  list. Use the keyboard to open it, check the currency-filter warning,
  and activate **View in Ledger activity**. The previous month, Harbor
  Brokerage and EURO should select one cash dividend. Open that record
  and compare the date and EUR 12.3456 receipt. The exporter prints its
  `contribution_activity` ID for SQLite comparison. Return to the timeline,
  then go Back to Contribution and check that the search, ranking filter,
  focused row and effective range are still active. Press Return to reopen
  EURO and repeat at a compact window size.

- Run `run_copy locked-settlement gio` and `run_copy locked-settlement wails`.
  Open Harbor Brokerage's holdings reconciliation from Tasks. At Synthetic
  locked product, choose **Confirm actual receipt**. Enter USD 132.1234 as
  gross proceeds, USD 1.2222 as the fee and November 23, 2026 at 18:00 in
  Asia/Singapore. Check the cash move from USD 156 to USD 286.9012, then
  cancel and confirm the product is still open. Reopen the same action by
  keyboard and post after review. Check **Linked changes** for the settlement
  and its redemption, then reopen the app to verify the same receipt and
  settled contract. The automated comparison covers stale review and retry
  after a lost acknowledgement.

- Run `run_copy locked-terms gio` and `run_copy locked-terms wails`.
  In the same Harbor Brokerage Holdings session, open **Edit terms and rules**
  for Synthetic locked product. Enter a name and note, change settlement days
  to 2 and the normal exit fee to USD 0.0123. Verify the existing start date
  and fee currency in Review. Cancel once and return to the same action.
  Review and confirm on a fresh draft. Reopen the app and check the saved name,
  note, policy and linked terms receipt. Principal and cash should stay at
  USD 125.25 and USD 156 respectively.

- Run `run_copy locked-renewal gio` and `run_copy locked-renewal wails`.
  From the same open product checkpoint choose **Record actual renewal**.
  Enter USD 132.1234 gross, a USD 1.2222 receipt fee, and November 23, 2026
  at 18:00 Asia/Singapore. Set new principal to USD 101.2345 and the opening
  fee to USD 0.1275. Name the successor Synthetic successor. Set its start
  to November 23 and its maturity and unlock date to December 23, 2026.
  Review, cancel, then reopen the focused action. Confirm once at a compact
  size and reopen the app. Check USD 185.5392 cash, the settled predecessor,
  the new USD 101.2345 position and the renewal plus both Ledger links in
  Linked changes. The automated test also covers stale review and idempotent
  retry after the shared service saves the operation.

- Run `run_copy add-product gio` and `run_copy add-product wails`. In
  Harbor Brokerage's Holdings reconciliation, choose **Add product** on the
  completeness card. Open a USD term deposit with USD 50.1234 principal,
  USD 0.0123 opening fee, November 23 start and December 23, 2026 maturity.
  Review USD 105.8643 remaining cash, cancel once, then reopen the focused
  action and confirm on a fresh draft. Restart and check the cash, the new
  deposit and its operation and activity IDs in Linked changes. From an
  independent fresh copy also record an existing EUR locked product with
  EUR 40.1234 principal, EUR 39.2345 basis and EUR 41.3456 current value.
  Acknowledge that account cash excludes the position. USD cash should stay
  at 156, and the linked activity should retain the given basis.

- Run `run_copy deposit-corrections gio` and `run_copy deposit-corrections wails`.
  Open Harbor Brokerage's Holdings reconciliation and select Synthetic deposit
  checkpoint. Edit the terms to 3.1259% ACT/360. Review the old 2.5001% rate,
  cancel and reopen the focused action before confirming. Record USD 2.3456
  interest paid through November 23, 2026, effective November 23 at 12:00 in
  Asia/Singapore. Review for USD 108.2099 cash. Settle at 18:00 on the same
  date with USD 50.1234 returned principal, USD 1.2345 interest and USD 0.0123
  fee. Cancel once and confirm an unchanged position before reviewing again.
  Reopen and check USD 159.5555 cash, the settled contract, and the original
  product checkpoint's terms receipt, two product operations and three
  activity links.
  An independent copy can repeat the sequence with renewal instead. Use a
  USD 40 successor with USD 0.1111 opening fee, November 23 start and December
  23 maturity, and USD 3.4567 fixed maturity interest. Cash should be USD
  119.4444 and the successor should reference the predecessor and renewal.

- Run `run_copy funds-product gio` and `run_copy funds-product wails`.
  In Available Funds, choose **Add product**. The single eligible account is
  Harbor Brokerage. Use USD 50.1234 principal, USD 0.0123 opening fee,
  November 23 start and December 23, 2026 maturity. Review USD 105.8643
  remaining cash. Cancel and reopen the focused action, then confirm on a
  fresh draft. After closing and restarting the app, open the new product
  from its funding source and verify the exact amount and dates. On a separate
  fresh copy, record an existing EUR locked product with EUR 50.1234 principal,
  value 52.0123 and basis 49.7891. Acknowledge that cash excludes it and verify
  USD 156 cash is unchanged and the locked product opens from funding sources.

- Run `run_copy account-product gio` and `run_copy account-product wails`.
  Open Accounts, Harbor Brokerage and its Managed products section. Add the
  same term deposit as in the Available Funds scenario. The account should
  stay fixed. Return to the draft from review, cancel and use the focused Add
  product action for a fresh entry. Confirm, restart and open the new
  contract from the account. On an independent copy, use the existing-product
  mode with the EUR values above and verify the account cash remains USD 156.

- Run `run_copy linked-cash-transfer gio` and
  `run_copy linked-cash-transfer wails`. Open Tasks, Reconciliation and
  Harbor Brokerage's active Holdings session. From the USD cash checkpoint,
  start Cash transfer. Send USD 10.1234 to Dollar Savings with a USD 0.0123
  fee. Review USD 145.8643 remaining and USD 110.6234 at the destination.
  Return to the draft, then cancel and confirm the checkpoint action is
  focused. Post the transfer, then record an incoming EUR 20.125 transfer
  from Euro Manual Value, with EUR 0.125 fee and USD 22.2345 explicitly
  received. Use November 23, 2026 at 12:00 and 13:00 in Asia/Singapore for
  the outgoing and incoming entries. After restart, both receipts should
  belong to the same USD cash checkpoint. Verify USD 168.0988 in Harbor
  Brokerage, USD 110.6234 in Dollar Savings, EUR 390 in Euro Manual Value
  and the unrelated USD 12.34 unchanged.

- Run `run_copy standalone-locked-lifecycle gio` and
  `run_copy standalone-locked-lifecycle wails`. Open Synthetic locked
  product from Harbor Brokerage or Available Funds. Start valuation, enter
  USD 131.2345, cancel and reopen the focused action, and confirm. Restart
  and check the exact new value and unchanged USD 156 cash. Start Confirm
  actual receipt, enter gross proceeds USD 132.1234 and fee USD 1.2222, and
  review the USD 286.9012 cash and the named reservation release. Cancel
  once, return to the focused action, and review and post again. Restart
  and open the exact redemption in Ledger. The product is settled, the
  account shows the same cash, and the saved reservation is released.

- Run `run_copy standalone-interest-terms gio` and
  `run_copy standalone-interest-terms wails`. Open Synthetic locked
  product from Harbor Brokerage. Review interest of USD 3.4568 with USD
  6.4197 remaining, cancel once, and confirm a fresh review. Check the
  latest interest link and identical principal and contract value after
  restart. Open Edit terms and rules, cancel one draft, then save a new
  name, two settlement days and a USD 0.0123 exit fee. Restart and verify
  the exact policy, USD 125.25 principal, USD 123.4567 value, USD
  159.4568 cash and absence of any further activity.

- Run `run_copy early-deposit-receipt gio` and
  `run_copy early-deposit-receipt wails`. Open Winter Deposit from
  Available Funds. Record returned principal USD 987.6543, interest USD
  4.5678 and fee USD 1.2345, with an effective date before its December
  20 maturity. Back to details must retain input. Cancel once and
  confirm a fresh preview. Restart and open both Ledger receipt links
  from the settled deposit in Accounts. Verify exact USD 2090.9876
  account cash and the release of the named USD 25 reservation.

- Run `run_copy product-operation-history gio` and
  `run_copy product-operation-history wails`. Open Synthetic locked
  product and scroll to product operation history. Observe the grouped
  reversal and the reversed interest entry. Load the next page from the
  first 25 rows and verify all 29 rows, including the original recorded
  product. Refresh returns to the first page. No ledger write is needed.

- Run `run_copy card-statement gio` and `run_copy card-statement wails`.
  Open Synthetic Statement Card from Accounts. In Gio, open Statement
  reconciliation, select the in-progress record and choose Review checkpoints
  and journal. In Wails, choose Open account reconciliation and Continue
  session. Inspect the balance checkpoint and the linked journal. The existing
  receipt has note Synthetic statement purchase and owes USD 42.3456 after
  posting. Preview a USD 2.3456 card purchase on the balance checkpoint, cancel,
  and verify the previous balance and receipt remain. Reopen and confirm that
  purchase with a distinct note. Both copies should owe USD 44.6912. The
  journal should open the new exact receipt. Verify Back restores the
  initiating record and focus in Gio.
- Run `run_copy linked-debt-value gio` and
  `run_copy linked-debt-value wails`. In Tasks, Reconciliation, open the
  existing session for Harbor Brokerage, Synthetic Fixed Loan and Synthetic
  Manual Property. From USD cash, select **Debt draw** and choose Synthetic
  Fixed Loan. Preview USD 30.125 at noon on November 23, 2026 in
  Asia/Singapore. Go Back and check the retained draft, then Cancel and
  reopen the focused entry. Confirm a new review. Open **Record a change**
  on the loan balance and post a USD 10.1234 principal payment with a
  USD 0.0567 fee from Harbor Brokerage at 13:00. On the property balance,
  record a USD 424.5678 value with reason Reconciliation at 14:00. After
  restarting, open each exact activity from its checkpoint. Compare the
  three civil times and resulting USD 175.9449 cash, USD 100.0016 loan,
  USD 424.5678 property and unchanged USD 12.34 savings. The Manual value
  form offers no debt action.

- Run `run_copy multi-account-reconciliation gio` and
  `run_copy multi-account-reconciliation wails`. In Tasks, Reconciliation,
  start a new session beside the existing Synthetic Unrelated Savings
  session. The picker initially selects five active accounts and excludes
  Synthetic Archived Account. Filter to Bank account and deselect the two
  displayed rows. The three hidden accounts stay selected. Clear the filter,
  review the seven pending checkpoints, return to the picker and check that
  the chosen accounts remain. Cancel once and confirm the existing session
  is still the only saved session. Repeat and confirm. Quit and relaunch,
  then open the mixed session and inspect Harbor Brokerage cash and holding,
  Synthetic Fixed Loan balance and Synthetic Manual Property value. Verify
  that the two bank accounts and archived account are outside the new scope
  and that their balances and the existing session are unchanged.

- Run `run_copy check-account gio` and `run_copy check-account wails`.
  Open Harbor Brokerage's active Holdings reconciliation. Choose **Confirm
  account** and review the cash, ordinary holding, product and completeness
  evidence. Check that Review requires explicit acknowledgement. Use a
  synthetic source reference, a civil date in the account's History timezone
  and a note. Preview and cancel once, then return to the same focused action.
  Confirm after reviewing again and reopen the app. All four checks should
  be confirmed, with no financial activity and the session still in progress.

Record interactive macOS results separately, including VoiceOver announcements,
escape and native window close, keyboard focus and compact layout. These
checks remain open until observed in the packaged apps.
