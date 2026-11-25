# Stage 6 paired desktop acceptance

## Reproduce the automated pair

From the repository root, run:

```sh
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run 'Test(Stage6PairedMixedLatestRefresh|Stage6PairedReturnCalendarFilteredKeyboardHandoff|Stage6PairedContributionFindsExactLedgerRecord|DataHealthSnapshotRebuildRequiresConfirmationAndAppendsOnlySelectedRevision|DataHealthSnapshotStopAndRetryKeepCompletedBatches)$')
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
derived snapshots. The expected households are `Synthetic Stage 6 snapshots`,
`Synthetic Stage 6 market`, `Synthetic Stage 6 return calendar` and
`Synthetic first buy`.
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
    return-calendar|contribution) household='Synthetic Stage 6 return calendar' ;;
    locked-settlement|locked-terms|locked-renewal|add-product|funds-product|deposit-corrections) household='Synthetic first buy' ;;
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
  test "$(/usr/bin/sqlite3 -readonly "$NESTWORTH_DATABASE_PATH" 'select name from households')" = "$household"
  exec "$executable"
)
```

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

Record interactive macOS results separately, including VoiceOver announcements,
escape and native window close, keyboard focus and compact layout. These
checks remain open until observed in the packaged apps.
