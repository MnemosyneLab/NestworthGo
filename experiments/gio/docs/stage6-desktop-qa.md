# Stage 6 paired desktop acceptance

## Reproduce the automated pair

From the repository root, run:

```sh
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run 'Test(Stage6PairedMixedLatestRefresh|Stage6PairedReturnCalendarFilteredKeyboardHandoff|Stage6PairedContributionFindsExactLedgerRecord|DataHealthSnapshotRebuildRequiresConfirmationAndAppendsOnlySelectedRevision|DataHealthSnapshotStopAndRetryKeepCompletedBatches)$')
(cd experiments/gio && go test ./internal/desktop -count=1 -v \
  -run '^TestLinkedLockedProductSettlementMatchesWails$')
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
    locked-settlement) household='Synthetic first buy' ;;
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

Record interactive macOS results separately, including VoiceOver announcements,
escape and native window close, keyboard focus and compact layout. These
checks remain open until observed in the packaged apps.
