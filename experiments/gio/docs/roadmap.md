# Gio desktop migration roadmap

## Target and starting point

A household should be able to complete the same desktop journeys with Gio as
with Wails, using the same ledger, settings, financial results and recovery
guarantees. Wails is the shipping desktop and the reference for each comparison.
Develop Gio in the existing [standalone module](../README.md)
until a release candidate passes the cutover gates below.

At the planning baseline `c682277e`, Gio had the shell, three session
languages (zh-CN, en, zh-TW), two appearances, five accent themes, shared
editors/search/select/date/time controls and a development preview. Directory
could list, search and rename active institutions through the application
service. Since that baseline, the desktop has gained Ledger activity,
durable General settings, the four-language sidebar and live Overview reads.
The stage records below track which workflows are available
and which still require acceptance. [Foundation design](foundation.md)

## Implementation boundaries

- Gio owns presentation, focus, drafts, route state and conversion of
  authoritative values to human-readable text or pixels. Use existing Go
  application queries and commands. Where a desktop adapter also performs
  authorization, repeat eligibility, validation or lifecycle work, trace and
  reuse the owning contract before exposing the operation in Gio.
- Keep I/O outside the frame loop. Pages retain widget and query state,
  invalidate the window when work completes, and distinguish first load,
  empty, failure, retry and existing results. Cancel and join outstanding work
  before closing the database. Financial mutations reread authoritative data.
- Keep the experimental launcher on its verified synthetic household. Introduce
  production startup, paths, locking, migrations and recovery together through
  the existing desktop owners before allowing a real ledger to be selected.
  The two shells must never concurrently own writable access to the same ledger.
- Share services, storage format, calculation rules and settings across shells.
  Keep React assets needed for the separately served mobile experience. See
  the [desktop entry point](../../../cmd/nestworth/main.go) and
  [production navigation model](../../../frontend/src/app/navigation.ts).

## Delivery sequence

Track page capabilities, layout alignment and verification in the
[Gio and Wails parity checklist](parity.md).

Each row ends in a user journey exercised against an isolated fixture. Begin
with read paths, then use the same data to validate writes and reconstruction.

| Stage | User workflows and dependencies | Exit evidence |
| --- | --- | --- |
| 1. Read ledger activity | Open History from the shell. Load the first 50 activities through `ListActivityPage`, browse older entries with its complete keyset cursor, refresh from the first page and recover from a failed read. Show localized activity labels, the recorded local date and notes. | On an isolated SQLite fixture, compare ordered activities and cursor boundaries to the service. Exercise loading, empty, error, retry and end of history. The slice has no posting actions. |
| 2. Shell, General settings and directory | Persist General settings, follow system appearance and language, and provide all four languages in the sidebar. Complete members, institutions and groups, including create, edit, archive, restore, icons and filters. Add navigation history, object links, retained drafts/scroll/focus and shared query states. | Reopen saved preferences and a directory change. Test keyboard-only navigation, focus restoration, IME composition and VoiceOver on a real Mac before migrating forms in bulk. |
| 3. Household, balances and accounts | Factor shared startup/blocked-state handling and lifecycle services. Implement onboarding, live Overview, institution-first account creation, account detail, ownership and valuation. | Compare populated, empty and partial valuations across shells. Verify restart, blocked/recovery paths and single-instance ownership using disposable ledgers. |
| 4. Ledger writes and history detail | Implement posting, preview and confirmation, History filters and detail, origin/starting-point workflows, repeat, correction and reversal. Add cross-page invalidation, including external writes. | Identical inputs produce the same preview, stored facts, provenance, linked corrections, balances and ordering. Test duplicate clicks, failures, retry and civil-date boundaries. |
| 5. Investments, products and tasks | Portfolio overview/holdings, instrument detail, trades and dividends. Available Funds, reservations and product lifecycle. Tasks for upcoming actions, investment review and reconciliation. | Compare lifecycle scenarios, valuation quality and reasons for unavailable actions. Follow links from a task or product through to its supporting facts. |
| 6. Analytics and maintenance | Wealth History, Return Analysis and Asset Changes, accessible charts and tables, market data and actionable Data Health. | Compare time windows, currencies, provenance, incomplete inputs and empty states. Ensure explicit provider refresh, keyboard access to chart data and authoritative repair results. |
| 7. Desktop integrations and distribution | Complete advanced Settings workflows, backup/restore/export, mobile pairing, MCP runtime, built-in assistant, credentials, native menus/dialogs, About and shutdown. Assemble the signed distributable and verify update and fallback. | Exercise each integration and restore/restart from a backup. Validate mobile assets, sidecar lifecycle and platform packaging on the supported target. |

Stage 1 is implemented on the experimental desktop. The current list displays
kind, civil date and note. Stage 4 adds History filters and activity detail,
plus reviewed contributions, withdrawals, cash transfers and currency
conversions. Eligible cash movements, including Holdings cash and internal FX,
support amount corrections and linked reversals. Bank accounts can reconcile a
native balance, and Holdings cash can be set or reconciled by currency.
History can capture an opening baseline for an existing household, including
[one eligible positive position](#opening-position-and-original-cost). The scoped Stage 2
workflows are implemented. Platform acceptance remains open.
Stage 3 includes synthetic startup, ledger ownership, household onboarding,
live Overview, account browsing, and creation and maintenance across the
account Catalog. Production startup and target-platform acceptance remain
open. Stage 5 includes Holdings and instrument reads, same-currency buys,
partial sales, liquidations, cash dividends, a positive opening position and
the Today view of Available Funds with standard bank-account reservations.
Stage 6 includes the [read-only Wealth History table](#read-only-wealth-history)
and the [Return Analysis calendar](#read-only-return-analysis-calendar).
Stage 7 is paused. Use [navigation history](../../../docs/design/navigation-history.md),
[history and form defaults](../../../docs/design/history-and-form-defaults-ux.md),
[account interactions](../../../docs/design/account-container-interaction.md) and the
[design index](../../../docs/design/README.md) for the detailed behavior behind each row.

### Stage 1 acceptance

| Check | Evidence |
| --- | --- |
| Storage and pagination | A synthetic checking account has six posted activities. SQLite integration tests reopen the fixture, walk it with a small page limit and check household dates and order. Model tests exercise the 50-row request, full continuation cursor, retry and refresh. |
| Desktop interaction | Gio frame tests open the route from the sidebar, exercise keyboard scrolling, switch languages, retry a failed append and check the retained rows. |
| Native macOS ARM64 | On macOS 27.2.1, the development bundle displayed all six rows, switched between Traditional and Simplified Chinese, reloaded them, and read the same six after quitting and reopening the verified fixture. The newest row, “Bank interest,” displayed November 12 in the household timezone. |
| Open evidence | The AppKit bridge now exposes the activity list and sidebar in the macOS accessibility tree. End-to-end VoiceOver parity remains open. The six-row native run does not measure large-list performance. Linux runtime acceptance and a paired Wails performance trial are pending for this revision. |

### Stage 2 progress

| Area | Current state |
| --- | --- |
| General settings | Appearance, accent, language, timezone, date format, time format and decimal/grouping separators have draft, save, discard and confirmed reset. The page displays household currency and the origin timezone when it differs from the display zone. Ledger dates, preview pickers and Overview use the applicable saved display formats. |
| Sidebar and shared controls | The collapsible sidebar has a four-language menu and a system/light/dark appearance button. Both write through the same preferences adapter as Settings. The shell and pages share the editors, selects, pickers, popovers, buttons and visual tokens in the [ui package](../internal/ui). |
| Persistent preview | The Mac launcher builds `Nestworth Gio.app` with ID `com.nestworth.gio`. A normal quit and direct Launch Services reopen used the same synthetic household, database file and settings contents in the [default Gio profile](../README.md#persistent-test-data). |
| Automated coverage | Frame tests exercise sidebar actions, draft retention, validation, save failure and retry, timezone filtering, compact scrolling and reset confirmation. Storage tests reopen an isolated fixture and check that save and reset preserve unrelated settings. Changing the display timezone and date format through Settings retains the original recorded ledger day in History. |
| Native macOS ARM64 | On macOS 27.2.1, the isolated development app saved Mint and DD/MM/YYYY from Settings and switched to Light and Traditional Chinese from the sidebar. The November 12 interest entry displayed as 12/11/2026. After a normal quit and reopen of the same fixture, Overview displayed the saved language and palette. The settings file retained the day-first date format. |
| Institutions lifecycle | Add an institution with a name, type and icon. Edit its name and icon together, confirm archive, and restore it from **Show archived**. The default view lists active institutions. **Show archived** includes both states and name search applies to both views. The list and object view display saved icons. |
| Members and groups | Directory opens on Members and offers Institutions and Groups tabs. Members and groups support creation, atomic name and icon edits, archive confirmation, restore, name search and archive filtering. The two member archive guards and the retention of existing group references follow the application service. |
| Navigation | The page bar supports Back, Forward and Recent. Directory member, institution and group rows open read-only object destinations. Returning restores directory tab, filters and scroll, other migrated pages' scroll, and available source focus. Mounted form drafts and ledger paging results survive traversal. |
| Platform acceptance | Full macOS keyboard traversal, native IME and VoiceOver remain open. Prior AppKit checks reached the sidebar, search editor and select options. Linux Wayland and an interactive Wails comparison remain unrun for the directory and navigation milestones. |

#### Institutions lifecycle milestone

The [Wails directory](../../../frontend/src/features/directory/DirectoryEntityList.tsx)
and [Go service](../../../internal/application/service.go) define the reference
workflow. Gio uses the shared type catalog and leaves name normalization,
validation, default icon assignment and archive timestamps to the same service.
Restore acts on the same ID. The pilot uses an inline creation form and keeps
the search and archive filter in the page's session state. A new session
starts with an empty search and the active list.

| Check | Evidence |
| --- | --- |
| Gio and SQLite | On Debian 13 amd64 with Go 1.27.4, Gio module tests and vet passed. The compact Gio frame regression used the normal Directory route with a verified synthetic SQLite fixture. It selected a non-default type by keyboard, checked required-name feedback, and verified the stored name, type, default icon and ID. A canceled archive left the row active. Confirmed archive removed the row from the active search, and the archived view found it without clearing the search. A failed post-write read offered a reload without a duplicate write. A failed restore kept the archived row available for retry. The test reopened the fixture after archive and after restore. |
| Wails reference | Ran `go test -mod=readonly -count=1 ./internal/wailsapi/directory` against the existing Wails adapter. Production code and schemas were outside the change. |
| Linux X11 | Built with `sh experiments/gio/preview-linux.sh --build-only`. In the actual preview window on Xvfb with Mesa software rendering, used a separate verified synthetic profile to add an Exchange, cancel and confirm archiving by keyboard, find it again with the same search, and restore it. Normal quit and reopen verified the persisted archive and subsequent restoration. |
| Open evidence | Interactive Wails comparison, Linux Wayland, and native macOS acceptance for this milestone remain unrun. Full keyboard traversal, native IME and VoiceOver are still Stage 2 gates. |

#### Members and groups milestone

The [Wails directory](../../../frontend/src/features/directory/DirectoryPage.tsx)
defines the operations and [its icon picker](../../../frontend/src/components/forms/IconPicker.tsx)
defines the available icon choices. Gio uses the same application commands and
passes name and icon to one atomic edit. Its name search and archive filter
belong to each tab. The inline editor temporarily hides the search controls
so the form and its buttons fit the minimum window size.

| Check | Evidence |
| --- | --- |
| Members | The Gio frame regression used the normal Members tab and a verified SQLite fixture. It checked both archive guards, name validation without a partial icon write, save and reload retry without a second commit, search across archive and restore, and the same member ID and icon after reopen. |
| Groups | The Groups frame regression edited name and icon, then archived a group referenced by a synthetic account. The account retained its group ID and display name. The application rejected an explicit assignment to the archived group. Switching tabs preserved the search and the pending read retry. Retry issued no second archive, and restoration persisted under the same group ID. |
| Catalog and service | Gio module tests and vet passed on Debian 13 amd64 with Go 1.27.4. The tests compare every available icon key and label to Wails in all four languages. The Wails directory adapter and the application member archive tests also passed. |
| Linux X11 | Built with `sh experiments/gio/preview-linux.sh --build-only`. In the actual 700 × 460 preview window on Xvfb with Mesa software rendering, added members and groups with icons, edited both, rejected archive of the synthetic checking account's sole active owner, and archived and recovered the new rows using their own search and archive filters. Normal quit and reopen of a separate verified synthetic profile confirmed the archive states, restorations and edited icons. |
| Open evidence | Interactive Wails comparison, Linux Wayland and native macOS were not run. Full keyboard traversal, native IME and VoiceOver remain Stage 2 gates. Navigation history and object links have a separate milestone. |

#### Navigation and directory-object milestone

The [Wails navigation design](../../../docs/design/navigation-history.md)
defines the cursor, branch, confirmation and restoration contracts. Gio keeps
the 40-visit limit and uses a read-only directory object for each currently
supported entity kind. The Stage 3 Accounts milestone extends this history to
accounts. Instrument and activity-detail links depend on their later-stage
pages. Back and Forward restore the state of the supported
pages.

| Check | Evidence |
| --- | --- |
| Visits | Gio frame tests exercised Back and Forward through Component Preview, Settings, Directory and Ledger activity. They checked draft text and selection, saved-versus-draft settings, directory tab and archive/name scope, scroll, focus, retained activity pages, the 40-visit bound and forward-branch replacement. |
| Object reads and writes | On verified synthetic SQLite fixtures, opening a group read its exact ID. Back after opening restored the source button. Forward read a renamed and later archived object. A failed list reload after a committed edit remained retryable and issued no second write. Tests also covered blocked traversal during confirmation, retained object data after a read error, a missing or malformed object, and a late response to an abandoned visit. |
| Linux X11 | The full Gio module tests, vet and Linux preview build passed on Debian 13 amd64 with Go 1.27.4. The actual preview on Xvfb with Mesa software rendering used a separate verified synthetic profile. At 700 × 460, Back returned to a filtered group list and pressing Return reopened its source object. Forward showed the new name and archive state after explicit writes. The confirmation blocked Back. In the wide window, typing after Back continued the retained preview draft, and visiting Ledger activity discarded the forward branch. A normal quit and reopen cleared the trail and retained the group's stored archive state. |
| Open evidence | Linux Wayland, interactive Wails comparison and native macOS were not run for this milestone. Full keyboard traversal, native IME and VoiceOver remain Stage 2 gates. |

#### Institution icons and Recent milestone

The institution chooser includes the generic options and all 272 selectable
bank symbols from the [Wails picker](../../../frontend/src/components/forms/IconPicker.tsx).
It shows the type default until the user chooses an icon. The list and object
view also render the 33 historical wordmarks. Editing only the name preserves
the original key. The list refresh after a successful edit only rereads data.

Recent follows the [Wails visit rules](../../../frontend/src/app/navigationHistory.ts)
for pages and object kinds Gio exposes. It records departures in most recent
order, keeps up to 20 distinct destinations and Directory scopes, and lives
for the window session. Selecting an entry opens a visit that participates in
Back and Forward. The menu keeps entries from a discarded forward branch.
Object visits reread the exact ID and update the label when it is unavailable.

| Check | Evidence |
| --- | --- |
| Automated | Gio module tests and vet passed on Debian 13 amd64 with Go 1.27.4. Catalog tests compared every entry to the source presentation set, including aliases, historical wordmarks and asset digests. Frame and synthetic SQLite tests exercised type defaults, explicit icons, atomic validation, reopening, and retry after a committed edit. Recent tests covered order, deduplication, distinct filter scopes, the 20-entry bound, draft restoration, invalid objects, confirmations, discarded branches, a fresh session and the absence of navigation writes. The shared domain, Wails directory adapter and application icon tests passed. |
| Linux X11 | Built with `sh experiments/gio/preview-linux.sh --build-only`. In the actual preview on Xvfb with Mesa software rendering, used a fresh verified synthetic profile. At 700 × 460, changed the type and saw its default icon, searched for Standard Chartered by alias and kept that choice through a later type change. Edited the saved name and icon to BOCHK. Forward reread the object. In the wide window, chose the object from Recent by keyboard and used Back and Forward between the object and Settings. Choosing Overview after Back discarded Forward, and the object stayed in Recent. After a normal quit and reopen, Recent was empty and the renamed institution still showed BOCHK in its editor. |
| Open evidence | This milestone did not run Linux Wayland, a paired interactive Wails session or native macOS. Full Mac keyboard traversal, native IME and VoiceOver remain open. |

### Stage 3 progress

#### Synthetic startup and ledger ownership milestone

Gio uses the [shared instance lock](../../../internal/infrastructure/backup/instance_lock.go)
on the canonical database path before opening SQLite for writes. It validates
the synthetic marker on a private copy to keep SQLite WAL-index writes away
from rejected profiles. After open, it attaches shared settings and runs the
shared application bootstrap. Shutdown joins UI work before checkpointing,
closing SQLite and releasing the lock. The blocked window covers an occupied
ledger, an invalid profile and a pending restore.

The [Wails entry point](../../../cmd/nestworth/main.go) reconciles interrupted
restores under the instance lock and offers recovery on database failure.
This ownership milestone blocked every pending restore journal. The later
[disposable recovery milestone](#disposable-startup-recovery-milestone)
introduces a verified fixture. Gio still requires the pilot marker and has
no ledger chooser.

| Check | Evidence |
| --- | --- |
| Automated | On Debian 13 amd64 with Go 1.27.4, the Gio module tests and vet, preview packaging tests, and the shared backup cross-process and directory-alias lock tests passed. Gio tests checked simultaneous fixture publication, an occupied profile, lock release after a pending restore, a journal with the live file moved, and byte-for-byte preservation of rejected profiles with a mismatched marker or a committed marker change in WAL. |
| Linux X11 | The Linux preview build ran on Xvfb with Mesa software rendering using a fresh isolated fixture. One process opened Overview. A second process using the same path displayed the occupied-ledger notice and exited with a failure status when dismissed. After the first process quit normally, another process opened the same ledger while the blocked notice was still open. The shared backup probe could not acquire the lock through a directory symlink, and Gio refused the symlink as a profile root. Invalid presentation settings and a pending restore journal produced separate blocked states. Moving the live file aside with the journal present kept the pending-restore state. File lists and SHA-256 hashes stayed unchanged during the rejected launches. |
| Linux X11 failure paths | In a separate synthetic profile, a durable settings attachment failure released the lock before the blocked notice closed. The CLI could verify the same fixture while the notice was open, and Return activated Quit. A marker change committed only to the WAL was detected, and the rejected launch left the files and their hashes unchanged. |
| Open evidence | This slice did not run native macOS or Wayland, a Wails/Gio paired launch or production restore recovery. Real ledger selection, onboarding and account workflows remain later Stage 3 work. |

#### Live Overview milestone

[Wails Overview](../../../frontend/src/queries/portfolio.ts) calls
`PortfolioService.Overview({})`. Gio calls the same unfiltered
`application.Service.Overview` and uses its display totals, known subtotals,
quality and missing inputs. It retains the result and read time on a failed
reload. Refresh rereads stored data using the configured FX provider key and
quote freshness policy. It performs no provider requests.

The Go comparison used independent equivalent copies from
[analytics-qa-seed](../../../cmd/analytics-qa-seed) with the seed anchor fixed
at `2026-07-26T00:00:00Z`. The live resolver uses the latest stored quotes.
To exercise the seed's gap in a current valuation, the copies retained
quotes through August 17, omitted the derived daily snapshots, and retained
the seeded current ledger facts. An empty onboarded household provided the
fifth case. The Gio-side copies carried the synthetic household marker and
passed the normal pilot guard. The shared valuation and display clocks were
fixed at `2026-09-18T13:14:00Z`, which displayed as `2026-09-18 21:14` in
`Asia/Singapore`.

| Scenario | Exact shared Go result in AUD | Gio window |
| --- | --- | --- |
| Complete | Net worth and assets `59374.42884`, liabilities `0` | Current estimate `59,374.43` |
| Missing prices | Known net contribution and assets `54668.065`, no net total | `54,668.06`, missing AAPL, QQQ and ES3 prices |
| Missing FX | Known net contribution and assets `18285`, no net total | `18,285.00`, missing USD and SGD rates against AUD |
| Both | Known net contribution and assets `18285`, no net total | Both kinds of missing input, with present amounts labelled as known contributions |
| Empty | 0 accounts, AUD base | Empty state without balance cards |

| Check | Evidence |
| --- | --- |
| Go service and adapter | Every copy agreed on exact amounts, display projection, availability, and missing inputs. A separate SQLite regression used a fixed clock, a stale persisted FX observation and an unavailable provider. It checked exact values and the signed liability, retained the stale quote disclosure, and recorded zero calls to either provider endpoint. |
| Gio frame and build | The Gio module tests, vet and Linux preview build passed on Debian 13 amd64 with Go 1.27.4. Frame tests exercised first load, empty and all-unavailable results, navigation back to a prior result, and a failed reload followed by Retry. The Wails portfolio adapter, shared valuation/provider and formatting tests passed. |
| Linux X11 | The fixed-clock preview ran the five verified profiles on Xvfb with Mesa software rendering. It displayed the shown amounts, both missing-input kinds in the same result, and no empty-household balances. The 700 × 460 combined case also supported reload and scrolling. Post-run comparisons found the same recorded account, holding, activity and market data rows in both copies. |
| Linux X11 read failures | A separate fixed-clock QA build injected two read failures at the Overview service boundary. The first showed loading followed by an error with no balances. Retry ran the real query. A failed reload then kept the incomplete figures and original read time visible. A second retry restored the normal state. The clock and fault overrides were temporary build overlays. |
| Open evidence | Wails GUI, native macOS and Wayland were not run. The comparison covered the shared current valuation and Wails Go adapter, with Gio rendering observed separately. Historical replay, provider refresh and account workflows remain separate. |

#### Read-only Accounts milestone

The [Wails account interaction contract](../../../docs/design/account-container-interaction.md)
and [account Go adapter](../../../internal/wailsapi/account/account.go) define
record and valuation reads. Gio groups rows by institution, keeps unassigned
accounts last, and uses signed base values for ordering. It opens detail by
the exact account ID. The amount text comes from the authoritative native
and household-base projections. The view labels missing prices, missing
exchange rates, stale quotes, known subtotals and unavailable values.

**View accounts** in an institution detail opens the matching Accounts scope.
Back, Forward and Recent preserve that scope and the archive filter. Back
to accounts restores the preceding list and its scroll and available focus.
Archived references keep their names and archive status. As in the Wails Go
adapter, archived accounts have records and no current valuation. A simple
account can still display its recorded balance in the matching currency.
Metadata and holdings are read-only. Group subtotals and the product, gains,
reconciliation and mutation surfaces are outside this slice.

| Check | Evidence |
| --- | --- |
| Go adapter | On independent copies of a verified synthetic ledger, compared the full account and valuation DTOs for default and archived lists, institution-scoped lists and exact-ID detail. The valuation and UI clocks were fixed at `2026-11-21T17:00:00Z`. Cases include a stale stored USD/AUD rate, missing SGD conversion, an unpriced holding, a recorded zero and archived references. |
| Gio frame and build | The Gio module tests, vet, Linux preview build, Wails account adapter tests and focused shared account valuation tests passed on Debian 13 amd64 with Go 1.27.4. Frame tests exercise Back, Recent, restored focus, empty and unavailable targets, and failure followed by Retry. An abandoned read returns after a newer account is visible, and the newer account remains selected. |
| Linux X11 | The fixed-clock preview ran on Xvfb with Mesa software rendering. From Directory, opened a named institution, selected its account, reloaded, went back, and used Return to reopen the focused row. The account showed USD `125.50` and AUD `188.25` with a stale quote notice. An SGD account retained `75.00` with AUD unavailable. A mixed account showed a known subtotal, both missing-input kinds and a holding quantity with no price. Scrolled to the holding at 700 × 460. The unassigned account showed a true zero. The archived account showed its recorded balance and archived institution, and returning kept the archive filter. Comparison of 11 account, directory and ledger tables before and after the run found identical rows. |
| Open evidence | Wails GUI, native macOS and Wayland were not run. The comparison covers Go records and valuations. Production startup, onboarding and account writes remain later work. |

#### Institution-first bank account creation milestone

Open an active institution from Directory, choose **View accounts**, then
choose **Add bank account**. This form creates a bank asset account that
tracks its total balance in one currency at an existing institution. It
defaults to the household currency and selects the sole active owner when
there is one. Multiple selected owners share equally. The review shows the
exact opening amount, up to three owners with their shares and a count of any
additional owners, and whether the account contributes to net worth. Group,
icon and dates use the shared service defaults.

The [Wails create wizard](../../../frontend/src/features/accounts/AccountCreateWizard.tsx)
and [Go account adapter](../../../internal/wailsapi/account/account.go) both
reach the same `application.Service.CreateAccount` used by Gio. The service
validates the selected institution and owners at confirmation and records the
opening balance according to the ledger's History state. An archived or
missing institution cannot start a form. If a reference becomes unavailable
during review, the form retains the other inputs and rereads active references.

Successful confirmation opens the account by its stored ID, invalidates the
scoped list and refreshes Overview on its next visit. Back restores the scoped
list and available source focus. If account detail fails after the write, the
confirmation remains visible and **Retry** only rereads the saved account.

| Check | Evidence |
| --- | --- |
| Go adapter and Gio frames | Independent verified synthetic ledger copies with a fixed clock produced the same exact bank balance, ownership, currency, institution and account flags through the Gio form and Wails Go adapter. Frame tests cover validation, canceling review, reference changes between review and submit, a repeated confirmation, a successful write followed by a failed detail read, read-only Retry, Back, Overview refresh and a persisted reopen. The Gio module tests, vet, Linux preview build, Wails account adapter tests and shared application tests passed on Debian 13 amd64 with Go 1.27.4. |
| Linux X11 | With the clock fixed at `2026-11-21T17:00:00Z`, ran the native preview on a verified synthetic profile under Xvfb with Mesa software rendering. At 700 × 460, a negative balance showed validation. Correcting it to USD `321.09` let the user review the sole owner's `100.00%` share and double-click confirmation. The app opened the new detail. Back showed the original and new accounts in the institution's list, and Overview showed USD `2,055.22`, up from `1,734.13`. After a normal quit and reopen, the new detail and Overview displayed the same values. |
| Linux X11 read failure and storage | A separate QA build and equivalent synthetic copy forced the first exact-ID detail read to fail after the real write. The native window showed the saved notice and load error together. Retry loaded the same USD `321.09` detail. Both closed profiles held exactly one new bank account, one owner and one opening contribution with one effect. Pre-existing rows in 12 account, directory, ledger and market tables matched the untouched copy. The fixed clock and fault were temporary build overlays. |
| Open evidence | Wails GUI, native macOS and Wayland were not run. Creation with custom group, icon, dates or ownership shares remains outside this form. |

#### Household onboarding milestone

The [Wails onboarding form](../../../frontend/src/features/onboarding/OnboardingPage.tsx)
and [Go household adapter](../../../internal/wailsapi/household/household.go)
define the form and storage contract. Gio uses `Bootstrap` and
`CompleteOnboarding` from the same application service. The form defaults to
CNY, asks for a household name and one or more initial members, and leaves
History off by default. The language preference applies before creation.
Enabling History uses today in the resolved display timezone unless the user
picks an earlier civil date. The service stores the History origin at local
midnight and creates the default institution and group.

An optional [empty onboarding fixture](../README.md#empty-onboarding-fixture)
is generated only in a fresh explicit run directory. A private random receipt
and a matching SQLite marker bind it to that canonical path. The same proof
remains usable after creation. This lets the initial household be absent while
the instance lock, private-file checks and read-only verification still apply.
The existing sample profile keeps its household-bound marker and startup path.

A successful Bootstrap without a household shows setup. An initialized but
empty household opens the ordinary shell. Read errors show **Retry**. Once a
submission begins, every outcome is followed by Bootstrap, including an error
that may have occurred after the household transaction. Retry only reads when
the outcome is still unknown or the write was confirmed. A failed write with a
successful read showing no household returns to the retained form. Successful
onboarding opens Accounts and refreshes the preferences' household context.

| Check | Evidence |
| --- | --- |
| Go adapter and Gio frames | Independent empty fixtures used the same fixed clock and form values. Gio and the Wails Go adapter agreed on SGD, member order and metadata, the default institution and group, and the start of `2026-11-20` in `Asia/Singapore` (`2026-11-19T16:00:00Z`). Frame tests covered initial read failure, validation, retained drafts after a failed write, duplicate clicks, a committed write followed by a failed read, and an error returned after the household transaction. Reopening retained stored IDs and the origin. A separate case used default CNY and stored no origin. |
| Profile guard | Gio tests checked the old sample marker and shared instance lock, explicit fixture preparation, unchanged files on a rejected launch, linked receipts, copied fixtures at a different path, and unmarked synthetic databases with and without a household. Shared backup cross-process and alias tests passed. |
| Linux X11 | The Gio module tests, vet, Linux preview build, Wails household adapter and targeted application tests passed on Debian 13 amd64 with Go 1.27.4. Native Gio on Xvfb with Mesa software rendering used `2026-11-21T17:00:00Z` from a temporary clock overlay. At 700 × 460, the history picker defaulted to `2026-11-22` in `Asia/Singapore`. The form validated a missing household name, accepted SGD and two members with a `2026-11-20` start, and opened empty Accounts after a double click on Create. After a normal quit, reopening displayed empty Overview and both members in Directory. A second process was blocked at the empty fixture without changing its files. A prior sample fixture still opened directly to its populated Overview. |
| Linux X11 read failure and storage | A second native build with a temporary fault overlay failed the first Bootstrap after the real write. The window displayed the saved notice and a read error together. Retry opened empty Accounts. Both closed and reopened profiles contained one household, two ordered members, one correctly zoned History origin, and the default institution and group. They had no accounts, holdings, postings or opening components. Both retained their original path-bound proofs. |
| Open evidence | Wails GUI, native macOS and Wayland were not run. Production startup, ledger selection and recovery, and broader account workflows remain Stage 3 work. |

#### Standard bank account maintenance milestone

Gio implements the [Wails account settings contract](../../../docs/design/account-container-interaction.md#97-account-settings)
for bank assets tracked as one total balance. The detail settings can change
the name, institution, group, icon, net-worth inclusion and ownership.
The form supports equal or exact percentage shares. Type, balance-sheet
role, tracking method, currency and recorded balance stay fixed in this form.
A settings save sends one shared `UpdateAccount` call with the changes.
Untouched setting fields use the [Go adapter's patch semantics](../../../internal/wailsapi/account/account.go).
Existing archived references can be retained. New references must be active.

Archive and restore are separately confirmed `ArchiveAccount` calls. The
archived detail keeps its stable ID and recorded balance and offers restore.
The active list and live valuation exclude it. Back and Forward keep the
original list scope and archive filter. A committed change updates the
account label on refresh and invalidates Overview's cached read. A failed
refresh keeps the previous result and the saved notice, with changes disabled
until Retry rereads the same ID.

| Check | Evidence |
| --- | --- |
| Go adapter and Gio frames | Matched the stored records on independent copies of the same synthetic ledger at `2026-11-21T17:00:00Z`. The flow renamed and moved a bank account, retained its archived group, replaced an archived owner with an active one, stored an exact `75.25%` / `24.75%` split and changed net-worth inclusion. A stale group rejected a combined update and left the record unchanged. Repeated clicks and forced post-commit read failures during edit and archive yielded one mutation each. Retry, Back, Forward, filtered rediscovery, restore, Overview invalidation and reopen passed. The balance and six activities still matched the original account ID. |
| Archive guard | A valid synthetic Holdings account with an open managed contract was rejected by the Wails Go archive adapter. Gio showed the maintenance scope on the read-only account and offered no mutation. The standard bank-balance archive and restore used the shared writer. |
| Linux X11 | On Debian 13 amd64 with Go 1.27.4, the Gio module tests, vet, Linux preview build, Wails account adapter and focused shared application tests passed. At 700 × 460 under Xvfb with Mesa rendering, changed the name, default group and Wallet icon, then archived the account, found it through Show archived and restored it. Back kept the archive filter active. Overview and the detail showed USD `1,734.13` after restoration. The activity list and a normal quit and reopen showed the original ledger and updated account. In an independent two-member profile at 1120 × 840, saved `75.25%` / `24.75%`, reopened and matched both exact shares against SQLite. |
| Linux X11 read failures and storage | A separate native build forced the detail read to fail after both the settings update and archive. The saved notices and stale-result warning appeared, and Retry recovered the same ID. The fault probe counted one update, one archive and one restore. Both closed profiles retained every balance, owner and history row, including six effects-linked activities. Each recorded three state observations for maintenance. The fixed clock and read faults used temporary overlays. |
| Open evidence | Wails GUI, native macOS and Wayland were not run. Other account types, compatible type changes, production startup, ledger selection and recovery remain Stage 3 work. |

#### Total-balance credit card and loan milestone

The [Wails create wizard](../../../frontend/src/features/accounts/AccountCreateWizard.tsx)
uses [Catalog combinations](../../../internal/domain/account_type.go) to
select a liability role and a single-currency total for credit cards and
loans. Both use `CreateAccount` with a nonnegative opening amount in the
account currency. Before History starts, a nonzero opening amount is a
stored balance baseline. With History started, the same service records its
opening contribution and account effect. A zero opening has no activity.
Loan proceeds, repayments, card purchases, refunds and reconciliation are
separate ledger actions.

Gio adds the two types from the accounts list with an optional existing
institution, default household currency and equal shares for chosen active
members. The institution starts unassigned in this entry point. The
institution-scoped bank form remains available. A newly archived institution
requires another choice or an explicit clear. The same settings, archive and
restore paths use `UpdateAccount` and `ArchiveAccount`. Detail and Overview
use the shared read models. A missing exchange rate leaves the native debt
visible and the household-currency value unavailable.

| Check | Evidence |
| --- | --- |
| Go adapter and Gio frames | At `2026-11-21T17:00:00Z`, independent verified copies matched the Wails Go adapter on a USD `321.09` card with History, an unassigned SGD `43.21` loan before History with missing SGD/AUD FX, and a zero USD loan with History. The card produced one opening contribution; both loans produced no posting in those cases. Records, net-worth inclusion, native and base amounts, missing-input evidence, maintenance, archive and restore were compared. Frame tests covered negative amounts, an archived institution, a failed write retaining the draft, and a committed write followed by read-only Retry. Back returned focus to the source and the same account ID reopened. |
| Linux X11 | The full Gio module, vet and Linux preview build passed on Debian 13 amd64 with Go 1.27.4. Focused Wails adapter, shared application and domain tests passed. At 700 × 460 on Xvfb with Mesa rendering, created a USD `321.09` card at Harbor Sample Bank, including validation of a negative input and repeated confirmation. Overview showed USD `1,413.04` net worth with USD `321.09` liabilities. Added an unassigned SGD `43.21` loan and saw the native debt with the USD conversion unavailable. Archive kept the recorded debt, and restore returned the missing-FX state. A normal quit and reopen retained both accounts. The standard bank account flow separately passed creation, Back, Overview refresh and reopen on X11, and its maintenance regression passed in Gio frames. |
| Linux X11 read failure and storage | A temporary QA overlay failed the credit-card submission before writing, then failed the first exact-ID read after the successful retry. The draft survived the write failure, and the created notice and **Retry** recovered the detail after the read failure. The probe counted two card submissions, including the deliberate pre-write failure, and one loan submission. The two closed profiles each held exactly one card and one loan, one opening contribution per account and unchanged pre-existing account, balance, owner, directory and activity rows. The clock and fault overlays are outside the application. |
| Open evidence | Wails GUI, native macOS and Wayland were not run. The Wails comparison covers the Go adapter and shared contracts. |

#### Complete total-balance Catalog milestone

**Add asset account** covers cash on hand, bank account, digital wallet,
receivable, and Other asset. **Add liability account** covers credit card,
loan, and Other liability. Both entry points derive their choices from the
[shared Catalog](../../../internal/domain/account_type.go). The selected
entry point fixes the role. The institution-scoped bank entry stays
available. Opening amounts and history evidence still come from the shared
[account writer](../../../internal/application/service.go).

Settings offers a type change within the existing role and total-balance
tracking mode, matching [Wails's compatibility filter](../../../frontend/src/features/accounts/accountCatalog.ts).
It sends only the changed account type to the shared writer for a type-only
edit. The form and the fresh-account check reject incompatible requests.
Role, tracking and currency stay fixed. The recorded balance, ownership,
saved inclusion flags, stable ID and ledger activities remain intact. The
shared reader may classify the account differently based on its new type.

| Check | Evidence |
| --- | --- |
| Fixed-clock Go comparison | On paired synthetic ledgers at `2026-11-21T17:00:00Z`, compared all five new combinations with the Wails Go adapter, including opening amounts, ownership, inclusion defaults, opening activity and zero behavior. Bank-to-receivable and credit-card-to-loan conversions matched and survived reopen. An incompatible creation could not enter review. Cross-role and non-balance destination edits were rejected, and the Gio probes found zero updates or added observations for those attempts. The original balance and activity references survived valid conversions. |
| Linux X11 | The Gio module tests, vet, Linux preview build and focused Wails adapter, application and domain tests passed on Debian 13 amd64 with Go 1.27.4. At 700 × 460 under Xvfb with Mesa, created all five additions, including a zero Other asset and a positive Other liability, and converted Harbor Checking to Receivable. Native reopen showed all five accounts, USD `1,761.18` net worth, and the original USD `1,734.13` balance on the converted account. Closed SQLite confirmed four opening activities and unchanged values, ownership and history for the original account. Separate X11 runs passed the bank create and reopen path and the credit-card and loan create, archive, restore and reopen path. |
| Linux X11 invalid requests | The asset settings picker excluded liability and manual-value destinations. A temporary QA overlay then supplied a Loan and a Property to the asset form at submission. The real form rejected both requests, and read-only verification found identical account, value, owner, activity and observation rows before and after. The clock and invalid-input overlays are outside the application. |
| Open evidence | Wails GUI, native macOS and Wayland were not run. The Wails comparison covers the Go adapter and shared contracts. |

#### Manual valuation and Holdings account creation milestone

**Add valued or holdings account** is available in Accounts and at an active
institution. The scoped form retains that institution. The shared Catalog
supplies all nine asset types for manual valuation and all eight for Holdings.
The form accepts an opening valuation for the manual mode and sends zero when
that field is blank, following the [Wails wizard](../../../frontend/src/features/accounts/AccountCreateWizard.tsx).
A Holdings account starts empty. The shared writer receives an empty opening
amount and creates no cash, positions or activities. Cash on hand in Holdings
mode displays only cash by currency. Mixed Holdings also displays investments.
An existing manual account with a missing value displays the shared valuation
service's unavailable state.

Creation accepts an active group, with the same initial group selection order
as the Wails wizard. Once History has a Starting Point, the optional opening
date uses the interval from that day through today in the stored History
timezone. The [shared application service](../../../internal/application/service.go)
owns the account, ownership, state observation and any opening contribution
in one transaction. The form returns to the saved account ID and retries
failed detail reads without submitting again. Mode selection applies to new
accounts. The existing total-balance settings guard limits conversions to
types in the same role and tracking mode.

| Check | Evidence |
| --- | --- |
| Fixed-clock adapter comparison | On independently opened copies of a marked synthetic ledger, Gio frame interactions and the Wails Go adapter produced matching records, ownership, exact display/base values and composition for a dated property estimate, explicit zero, empty brokerage and empty multi-currency cash. The nonzero estimate produced one account-value contribution at the selected civil day's effective instant. Zero and empty Holdings produced no activities. A separate marked ledger without History exercised a stored baseline and the missing-value state. |
| Validation and retry | The frame tests rejected an illegal type, a Holdings request with an opening amount, and an opening date without History before calling the writer. Each accepted write survived a simulated failure rereading the saved ID. Retry only read details, and Back restored institution scope and focus. The total-balance creation, settings, archive and incompatible-conversion regressions ran in the Gio suite. |
| Linux X11 | In a 700×460 Xvfb window, the native app opened Harbor Sample Bank and created a USD 1,234.56 property estimate dated November 20, an empty brokerage, and a cash-only Holdings account. Detail showed the recorded estimate and the appropriate empty component sections. After a normal quit, the same verified synthetic profile reopened and all three details were revisited through the institution. Read-only SQLite checks confirmed one dated account-value contribution, zero new cash/position rows and the same opening evidence for the estimate. A temporary build overlay fixed the clock at November 21, 2026, 17:00 UTC. |
| Automated checks | The full Gio Go test suite, Gio `go vet`, Linux preview build and focused shared domain, application and Wails Go adapter tests passed. |
| Open evidence | The Wails comparison covers the Go adapter. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun for this milestone. |

#### Manual valuation and Holdings maintenance milestone

Account settings use the same single update as the
[Wails Go adapter](../../../internal/wailsapi/account/account.go). Active
manual-valuation accounts require a recorded value, including a recorded zero.
Active Holdings accounts can be edited with or without cash and positions.
The same settings edit the name, icon, institution, group, net-worth inclusion
and ownership shares. Conversion preserves role, tracking and currency, and
the Holdings type picker keeps cash-only accounts within their own boundary.
Existing archived affiliations and eligible existing owner shares follow the
shared writer's validation.

Archive and restore keep the account ID and recorded financial facts. The
shared archive guard rejects open managed contracts, and Gio surfaces that
result after confirmation. A manual account with missing valuation can still
be archived and restored. A successful mutation followed by a failed read
keeps the success notice and exposes a read-only retry. Changes to estimates,
cash balances and positions belong to the Stage 4 and 5 ledger workflows.

Every create flow now uses the same optional group, opening date, icon and
ownership controls. The date is offered once History has a Starting Point.
Custom shares use the shared basis-point parser and appear in the review.
Group, date, icon and ownership travel with the account in a single create.
The Wails wizard supplies portfolio and liquidity flags from the Catalog;
Gio uses those defaults as well.

| Check | Evidence |
| --- | --- |
| Fixed-clock adapter comparison | Gio frames and the Wails Go adapter used independent copies of the same marked synthetic ledger. Property converted to collectible, populated and empty brokerage converted to investment account, and cash-only Holdings kept its type while saving settings. The suite compared saved records and valuations, exact ownership, archive/restore state, existing archived group links, and the unchanged manual value, cash in USD and SGD, position, and linked activities. Both paths refused incompatible conversions. Closing and reopening retained the same IDs and financial facts. |
| Disabled and retry paths | An open managed deposit blocked archive through the real Gio confirmation and the Wails adapter. A manual account with legacy missing valuation exposed archive/restore and blocked settings. Frame tests exercised incompatible input in both form and worker, a settings save and archive each followed by an injected read failure, and a read-only Retry. Existing bank, credit card and loan tests ran with the new controls. |
| Create fields | A compact Gio frame created a bank account with a selected active group, civil opening date, Vault icon and a 75.25/24.75 ownership split. An invalid allocation was rejected before writing. The saved record matched an independent Wails Go create. |
| Linux X11 | At 900×660 on Xvfb, opened three accounts through Harbor Sample Bank. Converted the USD 1,234.56 property to a collectible and saved 75.25/24.75 owner shares. Converted a populated brokerage to an investment account and confirmed that the cash-only type control stayed fixed. Archived and restored all three, then quit normally and reopened the same verified synthetic profile. Detail still showed the estimate, USD 15.25 and SGD 18.75 balances, and the brokerage's 2.5-unit position. Read-only SQLite comparisons retained the account IDs and every recorded estimate, cash, position, activity and effect row. A temporary build overlay fixed the clock and seeded the fixture through the application service. |
| Automated checks | Full Gio Go tests, Gio `go vet`, Linux preview build, and focused shared domain, application and Wails Go adapter tests passed. |
| Open evidence | Wails GUI, native macOS, Wayland, native IME and VoiceOver were not run for this milestone. |

#### Account creation recommendations and inline institution milestone

The [Wails account wizard](../../../frontend/src/features/accounts/AccountCreateWizard.tsx)
supplies the preferred institution types. All account create forms default to
the first active group by directory sort order and ID. Unscoped forms list
matching institution types first, select the first eligible institution by
the same order, and allow search across all active institutions by name or
type. The unscoped draft recomputes an untouched suggestion when its account
type changes. Gio preserves explicit institution and group choices, including
an explicit empty choice, through reference refresh. Gio also preserves an
explicit institution across type changes because its selectors share a form.
Wails's separate type step resets that choice on a type change. Creation from
an institution still keeps the originating institution fixed.

Unscoped creation can save an institution using the same application service
as the [Wails Directory adapter](../../../internal/wailsapi/directory/directory.go).
The form starts with the first preferred institution type or Other and picks
the default icon for that type until an icon is explicitly selected. Canceling
the institution editor returns to the account draft. Saving the institution
starts a separate commit, selects its stable ID and refreshes the available
references. If the refresh fails, Retry reads references only. The saved
notice explains that canceling the account leaves the institution in Directory.
Canceling or failing the account write cannot roll back an earlier successful
institution write. The directory list refreshes on its next visit.

| Check | Evidence |
| --- | --- |
| Defaults and search | Gio frames checked property with no recommendation, brokerage across eligible bank and brokerage types, crypto exchange with competing active names and an earlier archived candidate, name filtering, and preservation of explicit choices or absence after type changes and reference refresh. Total-balance, liability and scoped bank regressions exercised the shared default group. |
| Partial success and retry | Gio frames canceled a named institution draft and retained the account fields. A simulated institution write failure preserved name, type and icon. A pending submission suppressed duplicate writes and navigation. A known successful institution write followed by a reference failure exposed read-only Retry; canceling the account kept the institution and refreshed an already visited Directory list. A later account write failure and committed account detail read failure were exercised independently. |
| Go adapter and persisted records | At a fixed clock, independent marked copies produced equivalent institution metadata and account association, group, ownership and opening amount through the Gio form and the Wails Directory/Account Go adapters. Reopening the same synthetic profile retained the selected IDs. Full Gio Go tests, Gio `go vet`, Linux preview build, and focused shared services and Wails Go adapter tests passed. |
| Linux X11 | In a 900×660 Xvfb window, the liability draft suggested Harbor Sample Bank. The institution editor rejected empty and overlong names, and canceling it retained the account name and amount. Saved a synthetic exchange with the Vault icon and canceled the liability. A later crypto exchange draft recommended that saved exchange. The searchable picker found it, and review showed the default group. A normal quit and reopen retained the account and institution link. Read-only SQLite comparison found exactly one new exchange, one empty Holdings account and the same existing value, cash, holding and activity rows. A temporary overlay fixed the clock. |
| Open evidence | Wails GUI, native macOS, Wayland, native IME and VoiceOver remained unrun for this milestone. |

#### Disposable startup recovery milestone

The [fixture command](../README.md#disposable-startup-recovery-fixture)
creates a fresh synthetic run and uses the shared application backup service
and `InstallRestore`. It leaves an installed replacement and a complete
safety copy at the `replacement-installed` phase, before startup finalization.
The outgoing synthetic reserve is USD 99.50, and the backup recorded USD
42.25. The backup also carries a distinct presentation configuration.

Gio takes the canonical instance lock, then validates the exact journal it
will pass to the shared reconciler. It requires the dedicated receipt bound
to the canonical run root, fixed live and staging names, a generated safety
name, private owned files without links, an absent staging group and absent
SQLite sidecars. It verifies the journal and outgoing settings bytes, and
checks the sealed SHA-256 hashes and synthetic markers for both databases on
private read-only copies. This guard relies on the private profile directory
and cooperating writers honoring the instance lock. The ordinary launcher
and `--verify-pilot-fixture` continue to reject a pending journal.

After finalization, startup reloads presentation settings and verifies the
profile again before opening the business database. The same lock remains
held through the session and is released after checkpoint and close. The
[Wails entry point](../../../cmd/nestworth/main.go) continues to use the
shared startup recovery path. Gio accepts the bounded disposable fixture
phase. Installing a staged replacement at startup, recovering from a second
interruption during finalization and opening production profiles remain open.

| Check | Evidence |
| --- | --- |
| Shared startup comparison | Independent copies of the same pending synthetic layout agreed on USD 42.25 and recovered Dark, Mint and day-first settings. The reference executed the Wails entry point's shared lock, reconciler, settings reload and database open order. The ordinary verifier left the pending files unchanged. After Gio finalization, the journal was gone and the original backup and USD 99.50 safety copy were byte-for-byte intact. A normal close and reopen retained the recovered value. |
| Rejection and ownership | Go tests refused external journal paths, unsupported phases, link substitutions, unexpected SQLite sidecars, changed settings, a different synthetic database with a matching checksum and a copied receipt at another root. Rejections conserved the file inventory and bytes and released the lock. A separate process took the shared lock through a directory alias and fenced Gio before recovery. |
| Linux X11 | The native blocked window appeared while the alias process held the lock. SHA-256 and file lists stayed identical. Releasing the lock allowed a fresh launch to show USD 1,776.38 net worth and `21/11/2026` in History. Normal quit, read-only verification and reopen preserved the recovered profile. A separate pending journal with an external staging path displayed the recovery-blocked window and preserved both the fixture and its outside sentinel. |
| Automated checks | Gio module tests, Gio `go vet`, shared backup package tests and vet, and the Linux preview build passed. |
| Open evidence | Wails GUI, native macOS, Wayland and VoiceOver were not run. Production path selection, migration and restore phases, package acceptance and the target-platform lifecycle remain release gates. |

#### Remaining account capabilities by dependency

| Wails type or capability | Gio gap | Stage dependency |
| --- | --- | --- |
| Movements on existing accounts | Recorded value changes, opening cash and later cash by currency, FX, transfers, debt draws and payments, card purchases and refunds, account-total reconciliation, and related History detail. | Stage 4 preview, confirmation, and ledger writes. |
| Positions and products | Investment opening, trades, grouped products and Portfolio workflows for Holdings. | Stage 5. |
| Production profile and recovery | Ledger selection, production startup and recovery on the intended platform. | Stage 3 startup ownership and cutover gates. Stage 7 backup and restore interactions. |

Stage 3 closes with production startup and target-platform restart, blocking,
ownership and Wails parity evidence. Stage 4 uses separate verified synthetic
profiles and the established startup and ownership path.

### Stage 4 progress

#### Stage 4 exit review

| Gate from the delivery sequence | What the pilot supports | What remains |
| --- | --- | --- |
| Posting, preview and confirmation | Contributions, withdrawals, same- and cross-currency transfers, Holdings cash and within-account FX, and scoped balance targets. The milestone comparisons cover exact native amounts, posting kinds, civil dates, balances and duplicate handling. | Debt draws and payments, card purchases and refunds, expense categorization, broader reconciliation and account value updates, and backdated cases remain outside the migrated forms. |
| History, starting point and amendments | Stable Activity detail, filters and cursor paging, History and onboarding starting points for eligible cash/liability balances, repeat for supported cash movements, and linked cash corrections and reversals. | Positive position opening and cost basis are investment prerequisites when importing existing holdings. The account action-sheet shortcut, repeat list menu, broader amendment editors, within-account FX calculator and remaining statement interactions can follow the first investment read. |
| Cross-page and external invalidation | Confirmed writes refresh History, Account and Overview. History Activity links open the exact account and return to the existing visit. A write made through the same shared application service refreshes the active ledger pages without a page callback. | Exercise connected Mobile and MCP clients when their Gio lifecycles are wired. Keep production startup, target-platform acceptance and Wails GUI parity open. |

The [Holdings reads](#holdings-and-instrument-detail) and [first buy](#first-recorded-security-buy)
use isolated Holdings fixtures. Importing existing positions will require
separate evidence for their opening quantity and cost. The first-buy fixture
starts without holdings and verifies shared settlement, basis, valuation and
linked History after a duplicate confirmation, read failure and restart.

#### Cash contribution milestone

**Ledger activity → Record contribution** selects an active cash-balance asset
with a recorded value. This slice supports balance-mode bank, cash-on-hand and
digital wallet accounts. The receiving account sets the currency. The shared
date and time pickers collect a civil date and minute in the immutable History
Origin timezone, using the shared service to validate the effective instant.
Review shows the exact balance before, contribution and resulting balance.
Returning to details discards the preview. An intervening ledger write expires
the guarded review and requires a fresh preview.

Gio uses the [Wails History Go request](../../../internal/wailsapi/history/command.go)
to resolve the command and hash its immutable mutation envelope. The guarded
application writer shares the mutation replay and atomic commit implementation
with `RecordChangeWithMutation`. After an uncertain reply, the form offers a
retry of the frozen command and the same mutation ID. After acknowledgement it
opens the exact posted activity, reloads History and invalidates Overview.
Retry on a detail failure reads the saved record. Visiting Accounts rereads the
balance. This contribution detail is part of the History page. The form keeps
an unresolved submission in memory during the window session. The durable
application receipt can replay the same known ID after a restart.

| Check | Evidence |
| --- | --- |
| Go adapter and exact facts | Independent copies of a verified fixture at `2026-11-21T17:00:00Z` matched the Wails History Go preview, stored activity and effect for USD 25 on November 21 at 01:00 in `Asia/Singapore`. The recorded instant was November 20 at 17:00 UTC. One added `account_value` effect was classified as `external_inflow`, and the balance moved from USD 1,734.13 to USD 1,759.13. |
| Draft and failure paths | Gio frame tests used the normal History entry point to cancel a review, edit USD 50 to USD 25 and preview again, reject zero and an unavailable account, and require a new review after the account was archived. The tests exercised missing History, a timestamp before the Origin, and a repeated confirmation while the writer was pending. |
| Replay and read retry | A fault discarded the first successful response. The identical command, mutation ID, payload hash and token returned the original activity. A later detail read failed after confirmed success; Retry read the activity without calling the writer again. On reopen the shared receipt replayed the original ID, while USD 50 with that ID conflicted. One business activity and one effect remained. |
| Linux X11 | At 900×660 on Xvfb, canceled a USD 50 review, then reviewed USD 50 again, edited it to USD 25 and reviewed the new amount. A double click confirmed one contribution and left the posted detail visible. History reloaded from six to seven entries, and Accounts and Overview showed USD 1,759.13. After normal quit and reopen, History still opened the same contribution detail. Read-only SQLite comparison found one added activity, one USD 25 external-inflow effect and one mutation receipt. A temporary build overlay fixed the clock. |
| Automated checks | Gio module tests and vet, the Linux preview build, and focused Wails History adapter and shared application preview/mutation regressions passed. |
| Open evidence | Wails GUI, native macOS, Wayland, native IME and VoiceOver remained unrun. |

#### Cash withdrawal and same-currency transfer milestone

**Record withdrawal** and **Record transfer** reuse the contribution workflow
for the History Origin clock, exact review, explicit confirmation, immutable
mutation replay and refreshes History and Overview. Withdrawal uses Wails
`money_removed` with reason `other`. The shared writer posts an external
outflow. The cash account scope remains bank, cash-on-hand and digital wallet
assets in balance mode with a recorded value.

For a same-currency transfer, both endpoints are different active eligible
accounts, and Wails uses the same amount for sent and received principal.
The guarded preview validates both accounts again. Changing the source clears
a destination that becomes the same account. A blank fee is
omitted. A positive fee is deducted separately in the source currency and
shown alongside both resulting balances. Saved detail identifies the sending
and receiving accounts, amounts, fee and resulting balances.

| Check | Evidence |
| --- | --- |
| Exact Go parity | Separate copies of a verified synthetic profile used the same clock and History timezone. Wails Go and Gio matched preview endpoints, stored activities and effects, exact account balances and Overview net worth. A USD 25.125 withdrawal reduced USD 1,934.13 to USD 1,909.005. A USD 40 fee-free transfer kept net worth at USD 1,909.005. A USD 30.25 transfer with a USD 1.125 fee left USD 1,637.63 and USD 270.25 in the two accounts and USD 1,907.88 net worth. Each principal leg was classified as `internal_transfer`. The fee was a separate `fee` effect. |
| Invalid choices and review | Gio frames exercised source changes, a missing destination, and an incompatible currency. Guarded review rejected the same account, a foreign-currency account, an archived account and an unknown ID. A zero fee and a balance insufficient for principal plus fee prevented preview. Editing a reviewed fee required a new preview. |
| Replay and read retry | A double click during a paid transfer did not create a second write request. A deliberately discarded response replayed the identical envelope and returned the original activity. A following detail load failure offered a read-only retry. After reopening, withdrawal and both transfers replayed to their original IDs. A changed fee with the same ID conflicted. |
| Linux X11 | At 900×660 on Xvfb, withdrew USD 25.125, verified an EUR source could not choose a USD destination, and changed a reviewed USD 2 fee to USD 1.125 before confirming the USD 30.25 transfer. A double click on each confirmation posted one activity. Overview displayed USD 1,907.88. Both saved details opened after normal quit and relaunch. Read-only SQLite comparison found four new effects and two mutation receipts. A temporary build overlay fixed the clock and prepared the verified synthetic fixture. |
| Automated checks | The Gio module suite, Gio vet, Linux preview build, and focused Wails History, domain movement, household attribution and application idempotency regressions passed. |
| Open evidence | Wails GUI, native macOS, Wayland, native IME and VoiceOver remained unrun. Production startup, target-platform cutover and expense categorization remain open. See the cash correction and reversal and repeat milestones for later progress. |

#### Cross-currency cash transfer milestone

**Record transfer** accepts different currencies on the same eligible
balance-mode cash assets as the same-currency workflow. The source and
destination set the currencies from their account defaults. The user enters
both actual principal amounts. Changing an account currency clears the amount
bound to that currency, including the fee when the source currency changes.
Same-currency transfers retain a single principal input.

Gio uses the [Wails History Go command](../../../internal/wailsapi/history/command.go).
The [shared domain](../../../internal/domain/change.go) validates the native
amounts, deducts the optional fee in the source currency, and derives the
executed rate in the sent-to-received direction. Review and detail show the
two principal legs with their currencies, the executed rate, the fee and the
native resulting balances. Both principal legs are internal transfers.

Wails offers an optional amount calculator that requires a market quote.
Posting an explicit pair works without one. The
[cross-currency calculator milestone](#cross-currency-transfer-reference-calculator)
covers the Gio draft helper. Overview still needs a supported valuation quote for a
complete base total. The executed rate stays on the activity.
The shared analyzer’s [execution-spread bucket](../../../internal/application/analysis_classifier.go)
currently applies to the distinct `fx_conversion` kind. A cross-account
`cash_transfer` has no separate spread result in its posting preview or
stored effects.

| Check | Evidence |
| --- | --- |
| Exact Wails Go parity | Independent verified fixture copies with a fixed Singapore History clock matched USD 125.625 sent, EUR 100.5 received, and USD 1.2505 fee. The two native balances became USD 1,607.2545 and EUR 142.5. The preview and stored detail matched the executed rate of EUR 0.8 per USD, two internal principal effects and one fee effect. |
| Missing market FX | Neither fixture had a USD/EUR quote. Both accepted the transfer and returned matching incomplete Overview valuation for the EUR account. A later independent EUR→USD quote of 1.2 made net worth USD 1,978.2545 on both paths with the existing USD 200 reserve. The stored transfer retained the executed rate and native amounts. |
| Draft and retries | Gio frames rejected blank and zero received amounts, checked that changing destination currency clears only its amount, and edited a reviewed receipt before confirming. Double confirmation made one write attempt, loss of the reply replayed the original ID, and a detail failure after acknowledgement retried only the read. Reopening replayed the receipt, and a changed received amount with the same mutation ID conflicted. Contribution, withdrawal and same-currency frame regressions also passed. |
| Linux X11 | On Xvfb, the minimum 700×460 window showed the missing-receipt validation and the preview button. At 900×660, edited the reviewed receipt from EUR 99.25 to EUR 100.5, double-clicked confirm, checked incomplete Overview and reopened the saved detail after a normal exit. Read-only SQLite comparison found one new activity, three exact effects, one mutation receipt, both native balances and no market FX quote. A temporary overlay fixed the clock and prepared the verified fixture. |
| Automated checks | The Gio module suite and vet, the Linux preview build, and focused Wails History, domain transfer, application attribution and mutation regressions passed. |
| Open evidence | Wails GUI, native macOS, Wayland, native IME and VoiceOver remained unrun. Stage 3 production startup and target-platform cutover gates remain open. |

#### Holdings cash endpoints and currency conversion milestone

After History starts, contributions, withdrawals and transfers can use a
currency within an active Holdings asset account. Each Holdings endpoint
selects a supported currency. Balance-mode endpoints retain their account
currency. The [Wails History Go commands](../../../internal/wailsapi/history/command.go)
accept the explicit cash currencies, including for a transfer between
Holdings and balance mode. Wails' transfer form locks both currency fields
to the account defaults.
A valid inflow can create a currency balance. Outflows require sufficient
cash in that currency at the effective time, including any fee.

**Record conversion** uses a single Holdings account and separately entered
sold and bought currencies and amounts. A transfer requires different
accounts even when the currencies differ. A conversion requires different
currencies in the same account. Both use the shared guarded review and
mutation receipt. The preview and saved detail show native effects and
balances, the executed rate and the optional fee in the sold or sent
currency.

The [shared execution-spread rule](../../../internal/application/analysis_classifier.go)
applies to `fx_conversion` with base valuation and both endpoints in scope
when the native legs can be valued. It compares executed principal at
reference FX and attributes the difference separately from fees. Missing
reference FX leaves valuation incomplete and the native cash write valid.

Wails uses a distinct [target-balance plan](../../../internal/application/cash_preview.go)
for opening or reconciling cash. Before History it stores an opening
observation without a business activity. After History it reconciles the
delta at the reviewed instant. The tests use that contract to seed and
preserve opening facts. The Holdings cash target-balance milestone adds the
Gio form. The calculator for within-account FX conversion remains open.

| Check | Evidence |
| --- | --- |
| Exact Wails Go parity | On equivalent isolated fixtures, USD 125.625 sent from a balance account opened EUR 100.5 in Holdings. Converting USD 40 into EUR 32 with a USD 1.25 fee left USD 58.75 and EUR 132.5. The native endpoints, executed rate of EUR 0.8 per USD, two internal conversion effects and separate fee matched Wails. |
| Opening cash and money movements | Two independently generated onboarding fixtures recorded and replayed a Wails USD 100 opening baseline without an activity. Gio History blocked writes until History started. After starting History, a SGD 20 contribution and SGD 8.125 withdrawal matched Wails cash effects and ended at SGD 11.875 while preserving each original USD baseline. |
| Spread and missing FX | The shared Asset Change query was incomplete without a reference quote and exposed no execution spread. With a historically applicable EUR→USD reference of 1.2, the conversion day's household analysis showed USD −1.60 execution spread and a separate USD −1.25 fee, with zero external flow. Both amounts came from the shared analyzer. |
| Validation and retry | Frames and guarded app calls rejected the same account for a transfer, the same currency for conversion, an unsupported currency, a non-Holdings conversion account and principal plus fee above the available balance. The second confirmation click made no additional call. A lost reply replayed the original ID, the acknowledged detail load retried only reads, and a reopened copy replayed the same receipt. |
| Linux X11 | In a 900×660 Xvfb window, chose EUR on the Holdings endpoint of a bank transfer, reset a same-currency conversion choice, rejected insufficient cash for principal plus fee, and double-clicked conversion confirmation. Both details reopened normally. Read-only SQLite comparison found two added activities, five exact effects and two receipts, with unchanged positions and market quotes. A temporary overlay fixed the clock and prepared the verified fixture. |
| Automated checks | The Gio module suite, vet and Linux preview build passed, along with focused Wails History, Holdings cash, domain conversion and application attribution and mutation regressions. |
| Open evidence | Stage 3 production startup and target-platform cutover gates remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remained unrun. Holdings investment positions and trades remain Stage 5. |

#### History filters and activity detail milestone

**Ledger activity** combines activity type, account, instrument, and inclusive
History-local date bounds using the [Wails Go query contract](../../../internal/wailsapi/history/history.go).
Archived accounts and instruments remain selectable by stable ID. The account
filter matches either endpoint of a transfer. Each filter change starts at
the first page with a new cursor. Pages use effective time, creation time
and activity ID in server order. The model retires a replaced query's response
channel so a late response cannot append rows or replace the active status.

Each row opens its detail by activity ID. The detail uses recorded
effects, original currencies, executed FX rates, fees and resulting balances
where available. Non-cash records can show stored instrument and quantity
effects and trade price. Internal transfer principal, external flows and fees
use the recorded classifications. The list distinguishes no matches,
read failure and History without a Starting Point. Details distinguish a
missing activity from a retryable read. Read retries do not call a writer.

Back and Forward preserve the timeline's filters, loaded pages, scroll and
visible row focus within the window session. A newly posted activity opens
by its saved ID even if the filter excludes it. Timeline refresh keeps the
current filter. On failure, the existing rows remain and Retry reads the
same query again.

| Check | Evidence |
| --- | --- |
| Wails Go parity | On independent copies of a verified fixture, the 50-row first page and 17-row continuation matched Wails IDs and order, including identical timestamps. Account, type and instrument/date intersections matched Wails. The fixture contained contributions, withdrawal, same- and cross-currency transfers, FX conversion and a buy. |
| State and fault handling | Frame tests restored the loaded page position and stable row focus on Back, reopened the same detail on Forward, and recovered a filtered visit after visiting another timeline. A deliberately late page and a late detail could not replace their successors. A missing detail showed unavailable, and a transient detail and failed post-confirmation list refresh retried reads only. |
| Linux X11 | In a 900×660 Xvfb window, loaded all 67 records, selected an archived account, opened an internal transfer, and used Back and Forward. Verified the empty intersection and a buy selected by instrument and both date bounds. Reload retained its filters. At the end of the second page, opened an older record and returned to the same scroll position. Read-only SQLite comparison after normal quit found identical activities, effects, cash and account observations, holdings and mutation receipts. A temporary overlay fixed the clock and seeded the verified fixture. |
| Automated checks | The Gio module suite, vet and Linux preview build passed, along with focused Wails History, shared application and SQLite filter, pagination and mutation regressions. |
| Boundaries | Wails GUI, native macOS, Wayland, native IME and VoiceOver remained unrun. Wails date presets and product actions remain open. See the cash correction and reversal, Holdings cash target-balance and repeat milestones for eligible actions. Stage 3 production startup and target-platform cutover gates remain open. |

#### Cash correction and reversal milestone

History details offer **Correct amount** and **Reverse activity** on ordinary
single-currency contributions and withdrawals on active, supported
total-balance asset accounts. Correction keeps the account, currency, note
and original effective instant. Its review shows the exact replacement and
the balance at the original instant. The shared correction service appends
the linked reversal and replacement, then replays later balances. A standalone
reversal reviews the exact inverse and freezes the current effective instant
through confirmation. History displays that instant in the History Origin zone.

Gio follows the financial commands in the [Wails History Go adapter](../../../internal/wailsapi/history/history.go)
and uses the [shared guarded correction and reversal entry points](../../../internal/application/correction_guard.go)
for explicit confirmation and mutation replay. Linked details open the original,
reversal and replacement by stored ID. The UI identifies corrected originals,
reversed originals and replacements, and explains the original external flow
that an inverse effect offsets. The original activities and effects are retained.
Managed activities, terminal originals, reversal rows and unsupported movements
offer read-only detail. The following milestone extends these actions to
transfers, Holdings cash and internal FX.

An intervening ledger change requires a fresh review. When a write reply is
lost, the same submission reuses its reviewed request and mutation ID. A
known saved correction can also report a failed historical snapshot refresh.
Gio shows both outcomes and reloads the saved detail, current History query
and Overview. Retry on a failed detail or timeline read uses the read path.

| Check | Evidence |
| --- | --- |
| Wails Go and exact facts | Independent copies of a verified synthetic profile used `2026-11-21T17:00:00Z` and History Origin `Asia/Singapore`. Correction of USD 25 to USD 40 matched the adapter preview and stored linked effects at the original November 20 01:00 instant. The preview showed USD 1,774.13 at that instant. With later activities replayed, Harbor Checking held USD 1,761.13. Reversing a USD 9 withdrawal matched the Wails Go command and left USD 1,770.13. Shared household Asset Changes reported USD 40 net external flow on both copies, with the internal transfer excluded. |
| State and fault handling | Gio frame tests rejected an unchanged amount, required a fresh preview after an edit and an intervening ledger write, and blocked a duplicate click and Back during confirmation. A simulated snapshot refresh error accompanied a real saved ID, then detail Retry recovered that ID with no new write. A deliberately lost reversal reply replayed the identical request. Reopening returned both saved IDs from their receipts. Already corrected, already reversed, unsupported transfer and missing activity paths were rejected. |
| Linux X11 | On Debian 13 amd64 with Go 1.27.4, ran the native Gio window at 900×660 on Xvfb with Mesa software rendering. Canceled and reopened the correction review, double-clicked both confirmations, and followed linked details from the saved replacement to the original and correction reversal. Overview showed USD 1,774.13 before and after normal quit and reopen. Reopened History showed 12 records and linked the latest reversal back to the original. Read-only SQLite comparison preserved every original activity and effect and found three added activities, three exact USD effects, one correction group and two mutation receipts. A temporary overlay fixed the clock and seeded the verified profile. |
| Automated checks | The Gio module suite, vet and Linux preview build passed, along with focused Wails History, shared application correction/reversal and mutation, and SQLite query regressions. |
| Open evidence | Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover gates remain open. Broader correction forms remain open. See the repeat and balance reconciliation milestones for later progress. |

#### Transfer, Holdings cash and FX history amendments

**Correct amount** and **Reverse activity** now cover same- and cross-currency
internal transfers, ordinary Holdings cash contributions and withdrawals, and
currency conversion within one Holdings account. Corrections retain the
original kind, accounts, native currencies, note and effective instant. Same-currency
replacement principal is shared by the two legs. Cross-currency replacements
require each exact native principal. Review shows both endpoints, source fee,
resulting balances and the executed rate returned by the shared preview. The
standalone reversal review shows the direction of each inverse effect.

The [Wails History Go commands](../../../internal/wailsapi/history/history.go)
and [shared historical correction contract](../../../docs/design/history-and-form-defaults-ux.md#historical-correction-evidence)
own the financial rules. Blank transfer fees retain the original, explicit zero
removes it, and positive fees use the source currency. Blank fees on an
account-internal conversion remove the fee. Historical correction can use its
original archived endpoints and preserves their archive status. Standalone
reversal needs active endpoints. The shared preview also refuses insufficient
balances and replacements that invalidate later events. The same guarded
confirmation, linked activity navigation, mutation replay and read-only Retry
apply to these operations.

| Check | Evidence |
| --- | --- |
| Exact Wails Go parity | Separate marked SQLite copies used `2026-11-21T17:00:00Z` and the `Asia/Singapore` History Origin. The cross-currency replacement recorded USD 62.5 sent, EUR 49.25 received, USD 1.5 fee and the exact executed rate of EUR 0.788 per USD. It matched the Wails preview, replacement, correction reversal and standalone reversal. The remaining correction and reversal paths matched Wails for a same-currency transfer, EUR Holdings contribution and withdrawal, and USD-to-EUR Holdings conversion. After offsetting all movements, the two Holdings cash balances were USD 120 and EUR 60 on both copies. |
| Reference rate and attribution | The shared household report kept cross-currency transfer principal outside external flow. With a manual reference of USD 1.2 per EUR, the report showed USD 12.30 net external flow from the two Holdings cash movements while the corrected transfer was active. After reversing those cash movements, the report showed zero external flow. The active replacement conversion of USD 50 to EUR 41 contributed a USD 0.80 spread loss and a separate USD 1.75 fee. The reference rate did not supply a replacement principal. |
| Failure and replay | A zero destination principal could not advance to confirmation, and insufficient source cash was rejected. The fee regression covered blank retention and explicit zero removal. The archived destination accepted correction, preserved archive metadata and refused standalone reversal until restored. After a known saved correction and failed detail read, Retry issued no write. A lost reversal reply replayed the identical request, including after reopen. A different request hash with the same mutation ID was rejected. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, rejected zero destination principal, reviewed both native amounts and the fee, double-clicked confirmation, followed the original and replacement links, and reversed the replacement. Normal quit and reopen retained 17 activities and USD 1,955.08 net worth. A separate profile exercised conversion correction and reversal, including scrolling to the balances at 700×460. Reopen showed USD 1,954.68 net worth. Closed read-only SQLite verified three appended activities, nine exact effects and two mutation receipts on each profile. Original activities, effects and market quotes were retained. A temporary overlay fixed the clock and seeded the verified profiles. |
| Automated checks | Gio module tests, vet and Linux preview build passed, alongside focused shared Wails History, domain, correction, reversal, FX and mutation tests. |
| Open evidence | Broader Wails edits such as changing original endpoints, a reference calculator within correction forms, and product history remain open. See the repeat and balance reconciliation milestones for later progress. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover gates remain open. |

#### Start history for an existing household

Wails exposes [Start History](../../../frontend/src/features/history/StartHistoryForm.tsx)
in [History](../../../frontend/src/features/history/HistoryPage.tsx) and in
[account action sheets](../../../frontend/src/features/accounts/AccountActionSheets.tsx)
when an event needs a Starting Point. Its Go adapter calls `StartHistoryOnDate`.
Gio onboarding can already create an empty Starting Point, and Gio account
creation can already write a pre-History balance or use its existing dated
opening-entry contract after History starts. The new **Ledger activity → Start
recording history** action captures balances across the existing household.

Gio defaults to today in the resolved display timezone and shows the native
amounts that the shared planner will capture. Populated ledgers use the actual
confirmation instant on that day. An earlier date requires zero balances,
cash and holdings and uses midnight in the selected timezone. Existing account
values, per-currency cash, archived account state and ownership become opening
facts. Capturing the Starting Point creates no ledger activities. The
[Stage 5 opening-cost slice](#opening-position-and-original-cost) extends this
form to one positive same-currency holding.

The preview and confirmation reuse the same
[application planner](../../../internal/application/history.go) as Wails.
The reviewed civil day and timezone are frozen. An intervening write requires
a new review. The persisted household origin is the durable confirmation result.
After a confirmation or a lost reply, Gio verifies the origin before allowing
another write. When verification fails, Retry rereads the saved state.

| Check | Evidence |
| --- | --- |
| Wails Go parity | Independently prepared marked profiles used `2026-11-21T17:00:00Z` and `Asia/Singapore`. The populated case captured USD 125.5 in savings, an archived USD 31.25 loan, and Holdings cash of USD 15.125 plus EUR 20.5 with the same account state and ownership. Both rejected an earlier start with balances. The empty case matched `2026-11-19T16:00:00Z` for the selected `2026-11-20` local day. |
| State and fault handling | Gio frame tests covered cancel, stale preview after cash changed, positive positions with a valid Wails draft price, duplicate confirmation, failed reads after both known and ambiguous writes and read-only Retry. A first USD 5 contribution left USD 130.5. Replay after normal reopen returned the same origin and preserved the recorded activity and opening facts. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, rejected an earlier date with balances, reviewed native balances and double-clicked the confirmation. History was empty immediately after start. A subsequent USD 5 contribution brought savings to USD 130.5. Normal quit and reopen showed that single activity and the same balance. The empty profile accepted November 20 at midnight, then reopened with empty History. Closed read-only SQLite checks found exactly one origin in each profile, four opening components in the populated profile and none in the empty profile. The populated file retained original balances and contained only the separate contribution and its mutation receipt. A temporary overlay set the clock and seeded the verified profiles. |
| Tests | The Gio module suite and focused application, Wails History adapter and SQLite History tests passed. |
| Build and vet | The Linux Gio preview build and Gio and shared application vet checks passed. |
| Open evidence | The account action-sheet Starting Point entry remains open. [One positive opening position](#opening-position-and-original-cost) is covered in Stage 5. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver were not run. The Stage 3 production startup and target-platform cutover gates remain open. |

#### Holdings cash target balance

**Accounts → Holdings account → Set cash balance** follows the Wails
[account detail](../../../frontend/src/features/accounts/AccountDetail.tsx),
[cash form](../../../frontend/src/features/accounts/AccountActionSheets.tsx)
and [Go adapter](../../../internal/wailsapi/holding/cash_preview.go). It uses
the same `PreviewCashBalance` and `ConfirmCashBalance` plan. An active
Holdings account can set a single supported currency to a nonnegative exact
target with up to four decimal places. The service assigns the effective
instant. Before History it uses UTC and appends an opening cash observation.
With History it uses the immutable Origin timezone, accepts an optional reason
and posts the difference under `reconciliation` with a `remeasurement` effect.
The account receipt opens that exact activity. The timeline calls it **Cash
reconciliation**. Other currencies retain their balances.

The read-only review shows the prior balance, target, signed adjustment and
exact effective timestamp. A matching target makes no write. The shared
plan rejects intervening writes and later winning observations. Gio keeps the
same plan for an uncertain reply. After acknowledgement, account retry only
reloads the detail. Overview and History refresh from the shared queries.

| Check | Evidence |
| --- | --- |
| Exact Wails Go parity | On independent marked fixtures, choosing EUR in a USD-default Holdings account before History stored exactly EUR 20.5 without an activity. With History, a USD 150 target after an intervening adjustment to USD 110 posted an added USD 40 cash effect. Lowering the target to USD 80 posted a removed USD 70 cash effect. Both matched the Wails Go preview, signed effects, attribution, note, effective time, and final observation. Final net worth was USD 104.6 with EUR 20.5 unchanged. Shared Asset Changes reported net adjustment of USD −20 and zero external flow on both fixtures. |
| State and fault handling | Normal Gio frame interactions covered negative and unchanged targets, a fresh review after the cash balance changed, double-click confirmation and Back while pending. A lost reply replayed the same plan, returned the original activity and did not duplicate it. A failed detail read after acknowledgement retried only the query. Reopening preserved the balance and the same receipt. Separate shared tests checked expiration and effective/audit order conflicts. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, an unchanged target was refused, then a USD 100 balance was raised to USD 150 and lowered to USD 80. Double-clicks produced two total activities. The first receipt opened the remeasurement and its note. Reopen retained both History rows, EUR 20.5 and USD 104.60 net worth. The separate pre-History profile selected EUR, saved EUR 20.5 and reopened with that cash balance. Closed read-only SQLite checks matched the reviewed effective instants and kept the original baselines, quotes, and Holdings facts. The History profile added two cash effects and two receipts. The pre-History profile added one opening observation and no activities. A temporary overlay seeded the verified profiles and supplied an advancing synthetic clock. |
| Tests | The Gio module suite passed, along with the focused shared cash-plan, Wails Go cash adapter and SQLite cash observation tests. |
| Build and vet | The Linux Gio preview build and Gio vet passed. |
| Open evidence | Statement reconciliation, positive Holdings opening costs, and securities actions are outside this cash slice. The next milestone covers bank account balances. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover gates remain open. |

#### Bank balance reconciliation

**Accounts → bank account → Update bank balance** follows the Wails
[account action sheet](../../../frontend/src/features/accounts/AccountActionSheets.tsx)
for active, balance-tracked bank assets with an existing observation. Enter a
nonnegative target in the fixed account currency. A matching target makes no
write. Both modes use the shared money precision and the current-time path.

Before History, the Wails [account Go adapter](../../../internal/wailsapi/account/account.go)
uses `AppendAccountValue`, which stores a balance observation at confirmation.
Gio checks the reviewed observation ID and whether History is still off under
the write permit before calling that same method. The append has no durable
request receipt. When its reply is uncertain, Gio reads the recorded balance
and reports the uncertainty. No confirmation retry is offered for that append.

With History, Wails uses the [History Go adapter](../../../internal/wailsapi/history/history.go)
and the shared `value_update` command. Gio freezes the reviewed instant in
the History Origin timezone, uses the `reconciliation` reason, and confirms
with the guarded command and the Wails mutation envelope. The recorded
`account_value` effect is a `remeasurement`. A stale review requires a new
preview. The same mutation can resolve an uncertain reply. Once a write is
acknowledged, refreshing only reads. The account receipt links to the saved
activity and Overview reflects the new balance.

See [credit-card balance reconciliation](#credit-card-balance-reconciliation)
for the liability workflow.

| Check | Evidence |
| --- | --- |
| Wails Go parity | Independent marked fixtures matched the baseline observation, including the native amount and timestamps. After History, an intervening adjustment from USD 100 to USD 110 made the first review stale. Both then increased the target to USD 150 with one USD 40 remeasurement and lowered it to USD 80 with one USD 70 inverse-direction remeasurement. Final household net worth was USD 50, including the unchanged USD 30 credit-card liability. Shared Asset Changes reported USD −20 adjustment and zero external flow on both copies. |
| State and fault handling | Gio frames covered negative and unchanged targets, a changed balance, double-click and navigation during confirmation, a lost unkeyed baseline reply, a lost keyed reply and read failure after acknowledgement. Baseline recovery only read the saved balance. Keyed recovery and service reopen returned the same activity ID. Starting History while a baseline preview was outstanding invalidated that preview. The bank action was absent on a credit-card detail. |
| Linux X11 | In a 900×660 Gio window on Xvfb, rejected an unchanged target, raised a bank balance from USD 100 to USD 150 and lowered it to USD 80. Followed the saved detail link and confirmed the remeasurement. Two double clicks produced two activities, and normal reopen showed USD 50 net worth and both History rows. A separate pre-History run set USD 125, then reopened with USD 125 in the bank and USD 95 net worth. Read-only SQLite comparison verified a single new baseline without any activities on that profile. The History profile had two exact remeasurement effects and two mutation receipts. Original observations, History Origin and the credit-card balance were retained. A temporary overlay seeded the verified profiles and provided an advancing synthetic clock. |
| Tests | The Gio module suite passed, along with focused shared domain, application and Wails Go account and History tests. |
| Build and vet | The Linux Gio preview build and Gio vet passed. |
| Open evidence | Backdated and non-reconciliation Wails value updates and other balance-account types remain open. See the credit-card statement milestone for session coverage. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver remain unrun. The Stage 3 production startup and target-platform cutover gates remain open. |

#### Credit-card balance reconciliation

**Accounts → credit card → Update card balance** follows the Wails
[account detail action](../../../frontend/src/features/accounts/AccountDetail.tsx#L418)
and [simple balance form](../../../frontend/src/features/accounts/AccountActionSheets.tsx#L385).
The pilot requires an active, balance-tracked credit-card liability with an
existing observation. The target amount owed is a nonnegative magnitude in the
account currency. Zero is supported. The shared money parser rejects negative
targets, including an overpaid card balance, with the same four-place precision
as the other balance forms. Review shows the amount owed alongside the signed
account balances. An unchanged target makes no write.

Before History, the shared `AppendAccountValue` path stores a current balance
observation. After History starts, the Wails
[liability form](../../../frontend/src/features/history/RecordChangeForm.tsx#L638)
selects the `reconciliation` reason for `value_update`. Gio takes the reviewed
current instant and displays it in the History Origin timezone. The
[shared command](../../../internal/domain/change.go#L1533) records an
`account_value` remeasurement. Increasing debt lowers the signed account
balance and net worth. The activity detail names the increase or decrease in
debt and the resulting amount owed. The
[statement session](../../../frontend/src/features/reconciliation/AccountReconciliation.tsx)
is a separate Wails Tasks workflow.

The bank balance form and card form share guarded confirmation. A ledger
change invalidates the preview. An uncertain pre-History append only reloads
the balance because the append has no receipt. With History enabled, an
uncertain reply can replay the same mutation, including after reopen. Once
acknowledged, a failed refresh can only retry the read.

| Check | Evidence |
| --- | --- |
| Wails Go parity | Independent marked fixtures used `2026-11-21T17:00:00Z` and `Asia/Singapore`. Both Go paths rejected a negative target and stored an exact zero observation before History. With History, both raised debt from USD 30 to USD 35 between previews, then to USD 50.125 and finally to zero. The signed review, exact effects and timestamps matched. Net worth moved from USD 70 to USD 49.875 after the two increases and recovered to USD 100. Asset Changes reported USD +30 in adjustments and zero external net flow on both copies. |
| State and fault handling | Gio frame tests covered an unchanged target, an intervening balance change, double confirmation, a lost unkeyed reply, a lost keyed reply, and a read failure after acknowledgement. Durable replay, including after reopen, returned the original activity. The original observation and three intended activities survived. The bank controls and write paths also passed their regressions. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, rejected a negative target, raised debt from USD 30 to USD 50, then set it to zero. Opened both saved details and checked the remeasurement and amount owed. Double-clicking each confirmation produced two total activities and two receipts. Normal reopen showed both History rows and USD 100 net worth. A separate pre-History profile set the debt to zero and reopened with zero debt, one new baseline observation and no activity. Read-only SQLite checks confirmed exact effects and retained origin and baseline facts. A temporary overlay seeded the verified synthetic profiles and supplied an advancing clock. |
| Tests | The Gio module suite and focused shared domain, application, Wails Go account, History and Asset Changes tests passed. |
| Build and vet | The Linux Gio preview build and Gio vet passed. |
| Open evidence | Backdated value updates, other liability types and the account-level Starting Point entry remain open. The next milestone covers statement sessions. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver were unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Credit-card statement reconciliation

**Accounts → credit card → Statement reconciliation** opens the card's
existing reconciliation records and can start an independent session for that
card. The [Wails account entry](../../../frontend/src/features/reconciliation/AccountReconciliation.tsx)
opens the same records through Tasks. The
[Wails Go adapter](../../../internal/wailsapi/reconciliation/reconciliation.go)
owns the persisted preview and receipt for Gio's start, account confirmation
and finish. An existing paused single-card session can continue. Completed
records are read-only. After later bookkeeping, their original evidence
remains visible with the shared recheck status.

The Wails [whole-account check](../../../frontend/src/features/reconciliation/ReconciliationPage.tsx#L1138)
covers both the recorded card balance and account completeness. It uses one
source as-of date, optional source time and reference, and an optional check
note. No opening and closing dates define a billing period in this contract.
The date defaults to today and is capped at today in the History Origin
timezone. A date from before the origin can be recorded as evidence. Before
History, the calendar uses UTC and the shared contract requires a History
Origin for a source time. The check time is recorded separately. The balance
checkpoint is the current ledger amount, with the amount owed shown alongside
the signed balance. The whole-account action records evidence and leaves
financial History, balance, net worth and cash flows unchanged.

The [application session contract](../../../internal/application/reconciliation_session.go#L454)
keeps individual numeric source comparisons and linked financial adjustments
separate. A comparable amount for an individual check requires the current
ledger day. Billing evidence does not authorize a backdated ledger change.
This Gio entry can complete a single card's balance and completeness checks.
Multi-account records and checks requiring individual evidence handling are
available here to view.

| Check | Evidence |
| --- | --- |
| Wails Go parity | Independent marked ledgers used a fixed review instant in Asia/Singapore. Compared the reference and note, the November 15 source date and 01:00 source time, the separate actual check timestamp, the signed USD −35 ledger balance and positive USD 35 debt magnitude. Both stored one attempt per checkpoint and a completed record with two confirmations. The only financial change was a separate USD 5 control adjustment used to invalidate the first review. Both retained USD 65 net worth after that adjustment and zero external flow. |
| Review and recovery | Gio frames verified that completeness must be affirmed, editing preserves the evidence, a future date cannot be selected and an intervening ledger write requires a fresh review. A duplicate click did not trigger another confirmation. A lost response retried the same plan, the Go adapter replayed its receipt after reopen, and a failed detail read after acknowledgement retried only the query. Gio reopened completed evidence from the card's records. Later bookkeeping preserved both original checkpoints with the shared recheck indicator. A Wails-created multi-account session opened read-only. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, started a card session, selected the November 15 source date, set 01:00 and supplied a reference. Reviewed and confirmed the two checkpoints, then finished and reopened the completed record. The synthetic card stayed at USD 30 owed and Overview at USD 70. Read-only SQLite checks verified one completed session, three receipts, the original observations and zero financial activities. A temporary overlay seeded the verified fixture and supplied an advancing clock. |
| Tests | The Gio module suite and focused shared application, SQLite reconciliation and Wails Go reconciliation and Asset Changes tests passed. |
| Build and vet | The Linux Gio preview build and Gio vet passed. |
| Open evidence | Multi-account editing, individual source comparisons and exceptions, partial finish and linked financial adjustments remain open in Gio. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver were unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Repeat an ordinary cash activity

**Ledger activity → Details → Repeat as new draft** opens a one-time draft.
Wails has the same action in the History list menu and
[detail sheet](../../../frontend/src/features/history/ActivityDetailSheet.tsx).
The [Wails draft mapper](../../../frontend/src/features/history/activityToCommand.ts#L192)
and [shared eligibility check](../../../internal/application/repeat_source.go)
cover ordinary, single-account cash additions and removals. The inbound
reasons are income, interest, contribution, gift and other. The outbound
reasons are expense, fee, tax, interest and other. Transfers, holding-linked
fees or taxes, investment and product activities, reconciliation, corrections
and reversed activities are outside that contract. This flow creates a new
activity after explicit confirmation.

Gio copies account, native amount, currency, reason and note, with date and
time defaulting to the new draft's current instant in the History Origin
timezone. The default retains milliseconds. Editing the civil date or time
uses the selected local minute through the same History contract. The current
cash picker supports active bank, cash-on-hand and wallet balances and
Holdings cash. If the historical account is unavailable, the user can choose
another supported account. That selection uses the account currency and resets
the reason to Other. Holdings cash can select a supported native currency.

The [Wails Go repeat adapter](../../../internal/wailsapi/history/repeat.go)
validates the source again, creates a fresh mutation identity, and freezes
the command for confirmation. The source, activity and effect IDs are
independent of the new write. A changed ledger can require a fresh preview.
An uncertain reply retains the same plan for retry. A missing plan offers
read-only inspection of History. Plans are scoped to the running process.
After a confirmed write, the saved detail opens by its new ID and a failed
detail query can only retry the read.

| Check | Evidence |
| --- | --- |
| Wails Go parity | Independent marked fixtures started with USD 25 income and USD 4.50 expense. Gio and Wails Go matched the editable USD 17.25 new income, the repeated USD 4.50 expense, exact effects, dates, currency, reasons, notes and USD 103.25 final net worth. Both original activity and effect IDs and audit fields were preserved. For the manually selected earlier expense time, both services reprojected the later expense's resulting balance to USD 116. |
| State and recovery | Gio frames verified that opening, reviewing and canceling made no write, and editing required a fresh preview. Duplicate confirmation made one submission. Lost acknowledgement retried the same plan and returned the same activity. An acknowledged save followed by a failed detail read only retried the query. A reversal invalidated its source even when opened from an older draft. A missing plan allowed read-only History inspection, and reopening kept both new records. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, opened the original income, checked the new default date, changed the amount and note, reviewed the result and double-clicked to confirm. Normal reopen showed both the original and repeated income. The bank balance was USD 137.75 and net worth USD 107.75. Read-only SQLite checks retained the original audit rows and found one new activity, one effect and one new mutation receipt. A temporary overlay seeded the verified fixture and provided an advancing clock. |
| Tests | The Gio module suite and focused shared application and Wails Go repeat and mutation tests passed. |
| Build and vet | The Linux Gio preview build and Gio vet passed. |
| Open evidence | Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver were unrun. Gio's list menu shortcut and repeat for accounts outside its supported cash picker remain open. Stage 3 production startup and target-platform cutover remain open. |

#### Cross-currency transfer reference calculator

**Ledger activity → Record transfer → Fill from reference rate** follows the
[Wails transfer calculator](../../../frontend/src/features/history/RecordChangeForm.tsx#L1089)
and [decimal rules](../../../frontend/src/features/history/CalculateAmounts.tsx#L27)
for an ordinary transfer across two different currencies. Enter one native
principal and leave the other blank or zero. The calculator fills the other
field. Two positive amounts are preserved. Invalid input and results that
round to zero or exceed the supported amount leave the draft unchanged.

The [Wails quote adapter](../../../internal/wailsapi/quote/quote.go#L56)
and Gio read the same [application query](../../../internal/application/portfolio.go#L222).
It returns the latest local observation eligible for the pair's configured
source and retains the stored orientation. A missing preference or eligible
observation yields no quote. Gio shows the stored direction, source and
quote time or civil observation date. The selected transfer date controls
posting. The calculator uses the latest saved reference for the pair,
including when the transfer date is earlier. It multiplies along the stored
quote or divides in reverse, with exact half-even rounding to the target
currency's minor units.

A pair change discards the previous pending read. A read error offers a
read-only retry, and explicit amounts remain usable when the quote is missing.
Calculation only fills a field. Review confirms both editable native
principals and the source-currency fee, and the shared posting command
derives the executed rate from the confirmed pair. Quote source selection,
manual quote entry and provider refresh stay outside the Gio transfer form.

| Check | Evidence |
| --- | --- |
| Wails parity | On separate synthetic copies, Gio and the Wails Go adapter selected the stored EUR→USD reference of 1.2. USD 125.625 divided to EUR 104.69. Reversing the calculation from EUR 104.69 multiplied to USD 125.63. Exact arithmetic probes covered even and odd ties in both directions and zero-place JPY. Both posting paths confirmed USD 125.625 and EUR 100.50 with a USD 1.2505 fee, giving an executed rate of EUR 0.8 per USD. |
| Draft and reads | Frame tests covered empty and filled sides, a sub-minor-unit estimate, no quote, a read failure and retry, and a late old response after the pair changed. A transfer date earlier than the quote retained the same current observation. A date-labelled quote retained its civil date. Editing the estimated amount required a fresh review, and double confirmation wrote once. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, filled EUR 104.69, reviewed it, replaced it with EUR 100.50 and reviewed before confirming. At 700×460 both calculator and Preview were visible. After quit and reopen, the detail showed EUR 0.8 per USD and native balances USD 1,607.2545 and EUR 142.50. Read-only SQLite verified one added transfer and receipt, both principal effects and the fee, and unchanged quote and preference rows. A temporary overlay seeded the verified profile and fixed the clock. |
| Tests | The Gio module suite and focused shared application, Wails Go quote and History tests passed. |
| Build and vet | The Linux Gio preview build and Gio vet passed. |
| Open evidence | The within-account FX and correction-form calculators remain open. The Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver were unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Post-commit refresh and History account links

A confirmed ledger write retires superseded reads in History, Account and
Overview. Account facts, valuation and Holdings rows share an application read
scope. History reads the activity, links and references in one scope and queries
each timeline page under the same coordinator. Overview uses the existing
portfolio snapshot. While the ledger version is stable, the three destinations
show the same committed revision. Later writes through the shared application
service advance that revision and trigger new reads. The Wails client similarly
invalidates its affected query families after a write. [Wails invalidation](../../../frontend/src/queries/invalidation.ts) · [Shared read coordinator](../../../internal/application/write_coordinator.go)

The acknowledged Activity opens by its saved ID even when the active History
filter excludes it. Its account links use the effect account IDs and include
each named account once. Back restores the detail visit and link focus.
**Back to ledger activity** restores the nearest timeline visit with its filters,
scroll and source focus. A timeline refresh rebuilds its loaded range from fresh
cursors. A failed post-commit read offers a read-only retry.

| Check | Evidence |
| --- | --- |
| Synthetic shell and Go service | A frame test posted USD 25 in filtered History. It checked the saved Activity ID and result, USD 120 in Account and USD 90 in Overview at one shared application revision. An older Overview read captured before the write arrived after the new result. Injected History and Account read failures retried without submitting again. The test also exercised Back, Forward, both focus targets, a later write through the Wails Go adapter and a database reopen. |
| Timeline paging | A refresh with a newly inserted row retained the loaded range through the original anchor, even after a second-page failure and retry. Appending then used the anchor's full cursor and returned the next older activity once. |
| Linux X11 | In a 900×660 native Gio window on Xvfb, double-clicked Confirm on a USD 25 contribution from the Money removed view. The saved Activity opened Harbor Checking at USD 145.50. Back retained the original filter and one matching withdrawal. Overview displayed USD 115.50. Normal quit and reopen retained three activities, the same contribution and both totals. A read-only SQLite comparison found one new activity, effect and mutation receipt and retained both pre-existing activities. A temporary build overlay supplied the verified synthetic fixture and clock. |
| Tests | The Gio module suite and focused shared application and Wails Go History tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | Connected Mobile and MCP client paths, the Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

### Stage 5 progress

#### Holdings and instrument detail

**Investment assets** displays current positions, searchable by instrument
name or symbol. Gio reads the shared quantity, cost basis and native market
value across Holdings accounts. **Portfolio value** uses the shared
base-currency valuation and its quality, with a known subtotal when some
inputs are missing. Portfolio value covers securities. Household net worth in
Overview also reflects cash and other eligible balances. A funded account can
show no investment holdings. Zero-quantity positions are omitted from the
current list. [Wails Portfolio view](../../../frontend/src/features/portfolio/PortfolioPage.tsx) · [Shared holdings read](../../../internal/application/instrument_holdings.go)

Instrument detail shows the shared aggregate and each account's position.
It reads current price and the stored market FX quote independently and shows
the quote's currency direction and date. The local FX reference follows the
configured source. A position with a price can retain its native value when
household FX is missing. Missing prices preserve the cost basis and make
market value unavailable. A failed read offers an explicit retry. Ledger
version changes retire pending results before rereading the current page.
[Wails instrument view](../../../frontend/src/features/investments/InstrumentDetailPage.tsx) · [Wails Go quote adapter](../../../internal/wailsapi/quote/quote.go)

Account and Activity detail link to the security by ID. Security detail links
to its accounts and a History visit scoped to that instrument. Back restores
the prior visit, including search, scroll and available source focus.

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Separate synthetic profiles at a fixed clock agreed on quantity 3, EUR 60.36 cost, EUR 90.3702 native value, EUR 30.1234 price and EUR→USD 1.2. An instrument without FX kept GBP 13 of native value. Fixtures covered missing price, missing FX, both gaps and empty positions. The complete portfolio returned USD 108.4442, while the mixed fixture returned the same known subtotal with an incomplete valuation. The Overview comparison also checked the household cash and exact net worth of USD 208.44424 in the complete case. |
| Shell tests | Exercised Account and Activity links by stable ID, scoped History, returning to the filtered list and source focus, empty and nonexistent results, read retry, a superseded quote response, and normal storage reopen. |
| Linux X11 | Native Gio at 900×660 on Xvfb displayed complete, mixed and empty portfolios. The mixed profile showed missing price, missing FX and both gaps separately. The window exercised search, account navigation, instrument-filtered History and normal quit and reopen. A read-only SQLite comparison retained all financial and quote rows. A temporary overlay seeded the verified synthetic profiles and fixed the clock. |
| Tests | The Gio module suite and Wails Go Portfolio, Quote and Analytics package tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | Portfolio grouping and allocation, performance and market-data maintenance, position import, broader trade support, dividends and products remain separate slices. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun for this slice. |

#### First recorded security buy

**Accounts → Holdings account → Record first buy** records an already executed
trade. This pilot starts with a funded, active investment Holdings account
with no holding records. Choose an existing ordinary security in the
account's default currency. The account pays from that same-currency cash
sleeve. History must be enabled, and the chosen date and time use the immutable
History timezone. The picker excludes managed, archived and foreign-currency
securities for this slice.
[Wails trade form](../../../frontend/src/features/history/RecordChangeForm.tsx) · [Wails Go trade command](../../../internal/wailsapi/history/command.go)

Enter the actual quantity, gross before fees and optional fee. The shared
trade preview derives execution unit price and validates the amount available
at the effective time, including the fee. The preview shows exact execution
and settlement figures and the fee-adjusted average acquisition cost from
shared cost replay. Current quote remains separate in instrument detail.
Confirmation atomically creates the first Holding with its Activity and
effects. [Shared trade and mutation handling](../../../internal/application/change_service.go) · [Cost basis replay](../../../internal/domain/cost_basis.go)

An uncertain submission keeps the same command, mutation ID and preview
token. A stale preview whose receipt was absent returns to review. A confirmed
submission opens its saved Activity and invalidates Account, Overview,
History and Portfolio reads. A subsequent read failure can be retried from
that page. The Activity links to the affected account and instrument.

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independently seeded profiles at a fixed clock bought quantity 2.5 with USD 75.1234 gross and USD 1.2345 fee. The derived unit price was USD 30.04936, cash became USD 123.6421, and the position basis was USD 76.3579. The stored USD 40 market quote gave a USD 100 valuation and USD 223.6421 household net worth. Both paths recorded the same civil day and separate principal and fee effects. |
| Shell tests | Exercised quantity, gross and fee precision, insufficient cash including the fee, a different settlement currency, a current zero cash balance, cancel, draft edits, a stale preview, a double click and replay of a lost response. A failed read of the acknowledged Activity retried without writing. Reopening retained one buy and the shared service replayed the same receipt. |
| Linux X11 | Native Gio at 900×660 on Xvfb rejected gross plus fee above cash, then reviewed and double-clicked confirmation for the valid buy. History linked to the position and the cash account. Overview displayed USD 223.64. A normal quit and reopen preserved the buy and its fee. A read-only SQLite comparison verified one Holding, one Activity, three effects, one receipt and the exact cash and quantity projections. Opening balances, History Origin and quote rows were preserved. A temporary build overlay seeded the verified synthetic profile and fixed the clock. |
| Tests | The Gio module suite and focused trade, cost basis and mutation tests in the shared application, domain and Wails Go History packages passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | [Same-currency sales](#same-currency-security-sales) and [existing-position buys](#same-currency-buys-into-an-existing-holding) follow this milestone. Cross-currency settlement, positive opening positions, instrument creation and provider actions remain separate slices. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun for this slice. |

#### Same-currency security sales

**Accounts → Holdings account → Record sell** records an already executed
partial sale or liquidation. Choose a positive holding in an active
investment account and settle in its default-currency cash sleeve. Enter the
executed quantity, gross proceeds and optional fee. The shared trade command
derives unit price, checks quantity at the effective time, and previews cash
and quantity endpoints. The source picker uses existing Holdings and
excludes archived, managed and foreign-currency instruments.
[Wails trade form](../../../frontend/src/features/history/RecordChangeForm.tsx) · [Wails Go trade command](../../../internal/wailsapi/history/command.go)

Gio also reads the prospective sale's realized gain or loss and remaining
native cost from an additive application query under the same guarded read.
It uses the shared weighted-average cost replay, including acquisition and
sale fees. The preview labels missing cost evidence as unavailable and
reports an actual zero remaining cost when quantity reaches zero. This
slice accepts times strictly after the holding's latest affected cost or
quantity activity. The audit check includes reversals. Market quotes are
separate from the executed amount. [Application trade cost preview](../../../internal/application/trade_cost_preview.go) · [Cost basis replay](../../../internal/domain/cost_basis.go)

Confirmation uses the same immutable command, mutation ID and preview token
on retry. The saved sale opens by Activity ID and refreshes History, Account,
Portfolio and Overview. Liquidation preserves the Holding ID and immutable
History. The current-position list omits the zero position.

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Separate synthetic profiles acquired five units in two buys with fees, giving an average cost of USD 27. Selling two units for USD 70.125 gross with a USD 1.125 fee left three units, USD 81 basis, USD 15 realized gain and USD 134 cash. Selling the other three for USD 60 with a USD 2 fee left zero quantity and basis, a USD 23 loss on the sale, cumulative realized loss of USD 8 and USD 192 cash and net worth. Compared trade previews, effects, the Wails Go holding projections and the shared gain read. |
| Shell tests | Covered oversell, later audit events even after reversal, double confirmation, lost response replay, and a read-only retry after a known commit. Reopen preserved both sales and buys. Replaying both sell plans after restart returned their saved IDs. The first-buy workflow passed its existing parity and failure tests. |
| Linux X11 | Native Gio at 900×660 on Xvfb rejected an oversell, then confirmed a partial sale with a double click and navigated through the saved activity to the instrument and account. A second preview showed zero remaining quantity and cost and the USD 23 loss. After liquidation and normal reopen, Overview showed USD 192 and History retained four activities. Read-only SQLite confirmed two sales, one receipt per sale, exact cash and quantity projections, and unchanged Holding identity, buys and quote rows. A temporary build overlay seeded the verified synthetic profile with a controlled advancing clock. |
| Tests | The Gio module suite and focused shared application, domain and Wails Go trade, gain, cost and mutation tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | [Existing-position buys](#same-currency-buys-into-an-existing-holding) follow this milestone. Backdated sale insertion, cross-currency settlement, opening positive positions, instrument maintenance and tax reporting remain separate. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun for this slice. |

#### Same-currency buys into an existing holding

**Accounts → Holdings account → Record buy** records an executed buy against
an existing unarchived holding in the account's default currency. An
unarchived zero-quantity holding is eligible after liquidation and keeps
its ID on reentry. The first-buy entry remains available on an account
with no holding records. The existing-position picker lists holdings for
ordinary, active securities already on the account. [Wails trade form](../../../frontend/src/features/history/RecordChangeForm.tsx) · [Wails Go trade command](../../../internal/wailsapi/history/command.go)

The preview shows the executed quantity, gross and fee, together with the
holding quantity, cost basis and cash before and after. It also shows this
buy's fee-adjusted unit cost and the resulting weighted average. History's
shared cost replay retains the previous acquisition facts and realized gains.
Reentry starts from zero basis. Missing cost evidence is displayed as
unavailable. The effective time must be later than the holding's last cost
or quantity event. [Application trade cost preview](../../../internal/application/trade_cost_preview.go) · [Shared cost replay](../../../internal/domain/cost_basis.go)

Cash validation uses gross plus fee. A change to the same Holding after
preview requires a fresh review. A lost response retries the frozen
command and mutation ID. A read failure after acknowledgment retries
only the read. Account, Portfolio, History and Overview refresh after
confirmation.

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent synthetic profiles ran a first buy, a concurrent buy, the newly reviewed addition, a partial sale, a liquidation, and a buy back into the same holding. Five units had USD 135 basis and USD 27 weighted average. Subsequent sales realized USD 15 and a USD 23 loss. Reentry bought four units with USD 100 fee-inclusive basis, leaving USD 92 cash and the prior cumulative USD 8 realized loss. Compared Wails Go trade previews and holdings, and shared gain and valuation reads. |
| Shell tests | Exercised cash shortfall including fee, a stale plan after a competing buy on the same Holding, double confirmation, a lost response, a read-only retry following acknowledgment, and replay after restarting the shared service. First-buy and sell regressions passed. |
| Linux X11 | In native Gio at 900×660 on Xvfb, rejected a buy above the available cash including the fee. Double-clicked confirmation on the addition, sold part, liquidated, and used **Record buy** from the closed position. A normal quit and reopen showed five trade records, USD 100 position basis, USD 160 market value and USD 252 net worth. Read-only SQLite verified the original Holding ID, original acquisition and quotes, exact cash and quantity projections, and one receipt per new trade. A temporary build overlay seeded the verified synthetic profile with a controlled advancing clock. |
| Tests | The Gio module suite and focused shared application, domain and Wails Go trade, gain, cost and mutation tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | [Cash dividends](#same-currency-cash-dividends) and [opening positions](#opening-position-and-original-cost) follow this milestone. Buying a new instrument in a populated account after History starts, backdated additions that change later cost replay, foreign-currency settlement and instrument maintenance remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun for this slice. |

#### Same-currency cash dividends

From a Holdings account, **Record cash dividend** lists its ordinary,
unarchived holdings whose quote currency matches the account's default
currency. An unarchived holding remains eligible after liquidation. Enter the
positive amount actually credited, with up to four decimal places. The Wails
dividend command accepts one amount. Separately posted withholding or fees use
distinct Wails cash-removal operations and remain outside this slice. Gio uses
the History Origin timezone and the shared rules for effective times from the
origin through the current time. [Wails entry](../../../frontend/src/features/accounts/AccountActionSheets.tsx) · [Wails form](../../../frontend/src/features/history/RecordChangeForm.tsx) · [Wails Go command](../../../internal/wailsapi/history/command.go)

Review shows the cash balance before, the exact net credit and the resulting
balance at the effective time. Confirmation records one cash dividend, opens
its History detail with links to the account and instrument, and refreshes the
shared ledger views. The cash effect is income and the investment return is
attributed to the holding. The shared ledger leaves quantity, cost basis,
realized trade gain and external flows unchanged by the dividend.
[Dividend effect](../../../internal/domain/change.go) · [Investment attribution](../../../internal/application/analysis_classifier.go)

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent marked profiles used a fixed clock and `Asia/Singapore` History Origin. The USD 12.3456 dividend produced one cash income effect and the same account and holding links. After a separate USD 7.125 contribution, cash was USD 153.4706 and net worth was USD 273.4706. The three units retained USD 81 basis, USD 120 native market value and USD 15 realized trade gain. Closed-day analysis attributed USD 12.3456 to dividend income and investment return and only the contribution to external flow. |
| Shell tests | Rejected zero, negative and overprecision amounts, dates outside History and an archived holding. Accepted an active zero-quantity holding in preview. A change to the cash balance invalidated a reviewed plan. Double-click did not submit twice, a lost response replayed the original ID, a detail failure after acknowledgment retried only the read, and reopening preserved the receipt. |
| Linux X11 | In a 900×660 Xvfb window, rejected zero, reviewed USD 12.3456, double-clicked confirmation, and followed the History links to the instrument and account. Overview showed USD 266.35 with no separate contribution in this fixture. A normal quit and reopen retained the four records and the exact dividend. Read-only SQLite verification found one added activity, one income effect, one mutation receipt and USD 146.3456 cash in both event and replay projections. The holding, prior trades and market quotes were unchanged. A temporary build overlay seeded the verified synthetic profile and controlled the clock. |
| Tests | The Gio module suite and focused shared application, domain and Wails Go dividend, gain and mutation tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | Dividend notes, separate withholding or fees, foreign-currency dividends, reinvestment and capital-return operations remain outside this slice. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Opening position and original cost

Before History starts, **Accounts → Holdings account → Record existing
position** records a positive quantity in an existing ordinary security. The
picker offers unused, active securities in the account's default currency.
This entry is limited to one positive holding across the household, so the
same ledger can complete the Starting Point form.
The Wails account form calls `CreateHolding`. Its pre-History branch saves
the quantity observation and leaves the optional cost argument unused. Gio
reviews the quantity before calling that same branch and checks the persisted
holding when a reply is uncertain. [Wails account action](../../../frontend/src/features/accounts/AccountActionSheets.tsx) · [Wails Go holding adapter](../../../internal/wailsapi/holding/holding.go) · [Shared holding command](../../../internal/application/portfolio.go)

Next, **Ledger activity → Start recording history** asks for the original cost
per unit of one active, ordinary same-currency holding. Enter zero when the
original cost is zero. The shared Starting Point planner requires an existing
selected market quote. Gio takes the opening basis from the entered cost and
preserves the quote separately. The review includes the quantity, original
cost, cash and capture date. A populated start captures the current instant in
the selected History timezone. Confirmation stores opening facts. Later sales
use the shared cost replay from those facts. [Wails Starting Point form](../../../frontend/src/features/history/StartHistoryForm.tsx) · [Shared Starting Point planner](../../../internal/application/history.go)

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent verified profiles used `2026-11-21T17:00:00Z` and `Asia/Singapore`. They observed 2.5 units, then captured USD 12.3456 per unit and USD 210 opening cash. The market quote remained USD 40, basis was USD 30.864 and History was empty at capture. A later 1.25-unit sale for USD 25 with a USD 1 fee left USD 234 cash, USD 15.432 basis and USD 8.568 realized gain. Reopen retained the origin, position ID and the single sale. |
| Shell tests | Negative quantity and negative or blank cost were blocked. Explicit zero cost passed preview. Competing cash writes invalidated both review types. Double-click, a lost reply and failed verification used one position write and read-only recovery. Starting Point was verified by read after a lost reply, and an old quantity plan was rejected after History started. Existing cash-only starts and the multiple-holding guard passed. |
| Linux X11 | In a 900×660 Xvfb window, rejected a negative quantity, saved 2.5 units, and verified the single-position limit on a second attempt. Reviewed USD 12.3456 per unit and USD 200 opening cash and double-clicked both confirmations. After normal quit and reopen, net worth was USD 300, History remained empty, and the account showed the original position. Read-only SQLite verification found one holding and one origin with two opening components. The quote and cash observation rows were unchanged. A temporary build overlay seeded the verified synthetic profile and controlled the clock. |
| Tests | The Gio module suite and focused application, Wails Go holding and history, and SQLite history tests passed. |
| Build and vet | The independent Linux Gio preview build, Gio vet and shared application vet passed. |
| Open evidence | Multiple positions, cross-currency cost and managed products remain outside the slice. The History-enabled position-import operation in Wails is a separate write path. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Read-only Available Funds

**Available funds** opens the Today horizon used by the default Wails query.
The report uses the civil day in the ledger's History timezone, with UTC as
the shared default before History. Both shells use the same Go projection
for funding sources, normal access, fees, reservations and coverage. Gio
displays each native currency separately and labels the converted base-currency
total. A complete total, a known subtotal and an unavailable amount are
distinct display states. [Wails page](../../../frontend/src/features/liquidity/AvailableFundsPage.tsx) · [Wails Go adapter](../../../internal/wailsapi/liquidity/liquidity.go) · [Shared read](../../../internal/application/liquidity_read.go)

Each source shows its current native value and the shared estimate of funds
available today, before and after applied reservations. The detail also shows
requested amounts, shortfalls, unresolved reservations, and coverage reasons
such as missing access rules or base exchange rates. Account and ordinary
instrument links use stable IDs. Returning to Available Funds re-reads the
report and restores the list position and source focus. [Shared availability projection](../../../internal/domain/liquidity_evaluate.go)

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent marked SQLite profiles used `2026-11-21T17:00:00Z`. The USD bank contributed USD 125.4321 before, USD 40.125 reserved and USD 85.3071 after reservations. EUR 25.25 stayed complete in its native group while the base conversion remained partial with missing FX. A zero bank balance was known zero. An ordinary instrument with missing access rules and price retained unavailable amounts. An archived source appeared with an unresolved USD 5 reservation. A separate household with no sources produced the shared known zero. |
| Shell tests | A failed read retried successfully. Account and instrument links resolved by ID, and Back restored the account-source scroll position and keyboard focus. Forward returned to the same account. A closed and reopened profile matched the Wails Go figures. The view, retry and navigation left the ledger version unchanged. |
| Linux X11 | In a 900×660 Xvfb window, viewed the USD and EUR amounts and missing-FX explanation, opened the saved account and instrument sources, returned to the scrolled report, and found the unresolved reservation. Normal reopen repeated the same amounts. A separate profile showed the no-source message and known USD zero. Read-only SQLite comparison found the same rows across 13 financial tables in both profiles. A temporary build overlay seeded the verified profiles and controlled the clock. |
| Tests | The Gio module suite and focused application, domain and Wails Go liquidity tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | [Bank-account reservations](#bank-account-reservations) and [read-only term deposits](#read-only-term-deposits) follow this milestone. Additional horizons, early-withdrawal estimates, broader reservation and policy editing, product lifecycle actions and Tasks remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Bank-account reservations

The Today view offers **Manage reservations** on an active, balance-mode bank
asset. The form binds a purpose and a positive native amount to that single
account, using its default currency. Wails also supports other source kinds.
The Gio slice calls the same `SaveLiquidityReservation` with revision zero for
creation and `ReleaseLiquidityReservation` with the active row's ID and
revision. A release keeps the row with a release time and bumped revision.
[Wails reservation form](../../../frontend/src/features/liquidity/ReservationSheet.tsx) · [Wails manager](../../../frontend/src/features/liquidity/ReservationManager.tsx) · [Shared commands](../../../internal/application/liquidity.go)

The review shows the request and the current available amount. The shared
availability projection sets the applied amount, remainder and any shortfall
after confirmation. A request can exceed availability. A reservation for a
zero-balance source can be saved and appears as unresolved until the source is
positive or the reservation is released. These operations write reservation
metadata. Balance observations, net worth and the ledger timeline retain their
prior state. [Shared projection](../../../internal/domain/liquidity_evaluate.go)

Gio confirms against the reviewed ledger version. A lost create response is
checked by ID when acknowledged, or by finding exactly one new matching row
relative to the review. Release is checked by ID and revision. Retry during
verification only reads. A completed read that finds the old state requires a
new explicit review. Released rows remain visible in the source manager.

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent synthetic profiles started with USD 125.4321 cash and a USD 40.125 rent reservation. A new USD 100.5 request raised requested reserves to USD 140.625, with USD 125.4321 applied, zero available and a USD 15.1929 shortfall. Release restored USD 85.3071 available and retained the released row through reopen. Net worth and activity rows matched their initial state. A USD 9 request against a zero-balance account matched Wails as unresolved. |
| Shell tests | Covered zero input, a stale confirmation, a write failure with an identical pre-existing reservation, double confirmation, save and release replies lost after commit, failed verification reads, and read-only Retry. Release of the zero-balance reservation cleared its unresolved state. |
| Linux X11 | In a 900×660 Xvfb window, entered USD 100.5, double-clicked both confirmations, and viewed the USD 15.1929 shortfall and restored USD 85.3071 availability. Normal reopen showed the released row and the same available amount. Read-only SQLite comparison found one new reservation at revision two with a release time, both original reservations intact and the same rows in 12 other financial tables. A temporary overlay seeded the verified synthetic profile and controlled the clock. |
| Tests | The Gio module suite and focused application, domain and Wails Go liquidity tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | Editing active reservations and managing other source kinds, including archived sources, remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Read-only term deposits

**Available funds** links term-deposit sources to details by contract ID.
Holdings account detail lists them under **Managed products**, including
closed contracts. Details link back to the owning account by account ID.
Back and Forward restore the prior visit, including its list position and the
focused link. Managed holdings are separate from ordinary positions and their
trade actions. An existing managed instrument target points to the supported
product while shared account and portfolio totals continue to include the
managed value where eligible. [Wails source menu](../../../frontend/src/features/liquidity/SourceTable.tsx) · [Wails account cards](../../../frontend/src/features/liquidity/AccountProductsSection.tsx) · [Shared ownership lookup](../../../internal/application/portfolio.go)

The details use the shared product read for principal, current native
contract value, start, maturity and product state. Expected receipt date
and net proceeds come from the normal liquidity route and carry estimate
labels. Even after the modeled receipt date, an open contract can still
have an unconfirmed receipt and make no cash available. Missing timing
keeps the estimate unavailable. The receipt and interest action statuses
report the shared permissions. A closed contract explains why receipt
confirmation is unavailable. The statuses are read-only. [Wails product sheet](../../../frontend/src/features/liquidity/ProductDetailSheet.tsx) · [Wails Go adapter](../../../internal/wailsapi/liquidity/liquidity.go) · [Shared liquidity projection](../../../internal/domain/liquidity_evaluate.go)

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent marked SQLite profiles used `2026-11-21T17:00:00Z` and the Asia/Singapore civil date November 22. Shared account lists and product details matched for locked, due-unconfirmed, missing receipt timing and settled contracts. USD 1,000 contract value had USD 1,011.1111 estimated net proceeds after the modeled fee. A due USD 600 contract showed USD 606.25 estimated proceeds and no selected Today route. The incomplete product retained its USD 300 value and an unavailable estimate. Actual cash stayed USD 1,100. |
| Shell tests | Covered failed product read and Retry, navigation from funds and account cards, Back and Forward with restored focus, an unavailable contract ID, and separation from ordinary positions. Reopen returned the same product states and cash. Ledger version and activities were unchanged. |
| Linux X11 | In a 900×660 Xvfb window, opened the locked term deposit from Available Funds, followed its account link, opened the settled deposit, and navigated back to the original scroll position. Checked the due and incomplete contracts, then quit and reopened normally. After the existing first-launch preference initialization, a read-only SQLite comparison found identical rows in all 77 tables across both browsing sessions. A temporary build overlay seeded the verified synthetic profile and fixed the clock. |
| Tests | The Gio module suite and focused application, domain and Wails Go liquidity and product tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | [Confirming a term deposit receipt](#confirm-actual-term-deposit-receipt) follows this milestone. Other managed product types and actions, policy and broader reservation editing, and Tasks remained open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Confirm actual term deposit receipt

On an open term deposit at or after contractual maturity, **Confirm actual
receipt** follows the Wails due-receipt entry for a product whose modeled
normal receipt is due and unconfirmed. The shared display state and settle
permission are re-read before preparing the command. The draft uses the
original principal as the initial value and zero for interest and fee.
Enter the actual amounts from the receipt. The unpaid maturity interest and
normal net proceeds remain separate estimates. [Wails due receipt entry and form](../../../frontend/src/features/liquidity/ProductDetailSheet.tsx) · [Shared due-state projection](../../../internal/domain/liquidity_evaluate.go)

The user can specify the actual date and time in the History timezone.
Leaving both fields empty freezes the time of review. The receipt date must
be at or after contractual maturity. The shared command enforces acceptable
amounts, the latest financial operation, and the current-time ceiling.
The preview shows returned principal, actual interest and fee, both cash
balances, and the change in the contract value. Any active reservations on
that holding are listed for release as part of the same settlement, per Wails.
[Wails Go adapter](../../../internal/wailsapi/liquidity/liquidity.go) · [Shared operation service](../../../internal/application/product_operations.go)

Confirmation sends the reviewed command, state hash and one mutation ID.
A stale review returns to the draft. An unknown outcome can replay the same
confirmation. Once acknowledged, all retries refresh reads. The settled
product links to its posted principal and optional interest activities.
The links use the persisted operation receipt and remain available after
reopen. Each linked History record returns to the product. [Shared receipt read](../../../internal/application/liquidity.go)

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent marked profiles used Asia/Singapore. A USD 606.25 forecast remained separate from USD 595.125 returned principal, USD 4.25 actual interest and USD 0.375 fee. Shared preview and posting agreed on exact cash (USD 1,100 to USD 1,699), zero remaining quantity, the settled contract, and the two linked activities. Today recognized USD 1,699. Asset change attributed USD 4.25 to dividend and interest, with zero external flow. |
| Shell tests | Covered not-yet-mature and settled products, invalid amounts and dates, frozen default time, reservation changes invalidating review, and release of both reviewed reservations. Double confirmation sent one request. After a lost committed reply, the same command, ID and hash resolved to the original receipt. A failed refresh after acknowledgement retried only reads. Checked links in both directions, normal reopen, and persistence of the receipt. |
| Linux X11 | In a 900×660 Xvfb window, checked the locked product, entered actual amounts on a due deposit, reviewed the cash increase, and double-clicked the posting control. Followed the History links and reopened normally. Read-only SQLite verification found one new settlement operation, two linked activities, zero remaining quantity, USD 1,699 cash, and unchanged earlier activity, effect and quote rows. A temporary build overlay seeded the verified synthetic profile and fixed the clock. |
| Tests | The Gio module suite and focused shared application, domain and Wails Go liquidity tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | [Term deposit Tasks](#term-deposit-tasks) follows this milestone. Early redemption, renewal, opening products, standalone interest recording and other managed product types remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Term deposit Tasks

**Tasks & checks** presents the term deposit schedules from the shared
`UpcomingActions` read used by the Wails **Upcoming** tab. The projection
supplies the History timezone, the 30-calendar-day forward window, attention
and scheduled groups, order, and reasons. A due receipt can stay on the list
past its estimated date. A blocked schedule shows which inputs are missing.
Missing dates and amounts stay unavailable. All proceeds on the list are
estimates. [Wails Tasks](../../../frontend/src/features/tasks/TasksPage.tsx) · [Wails upcoming list](../../../frontend/src/features/upcoming-actions/UpcomingActionsPage.tsx) · [Shared read](../../../internal/application/upcoming_actions.go) · [Shared projection](../../../internal/domain/upcoming_actions.go)

Each supported row opens the exact product by its shared target ID.
**Verify receipt** requests the existing confirmation draft only for a
due receipt. Gio checks the freshly read product and its settlement
eligibility before opening the draft. If the product has already settled,
the shortcut shows the updated details. Back restores the list scroll
and surviving link focus. Forward to a product does not repeat a consumed
receipt intent. Browsing and opening the draft are read-only. After an
explicit settlement, the list refresh follows the shared projection and
omits the settled receipt. [Wails receipt navigation](../../../frontend/src/features/upcoming-actions/upcomingActionPresentation.ts) · [Existing Gio receipt](#confirm-actual-term-deposit-receipt)

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent synthetic profiles fixed the read at `2026-11-21T17:00:00Z`. Both projections showed November 22 through December 22 in Asia/Singapore. The December 22 boundary was included, December 23 was omitted, and overdue, same-day receipt, blocked, awaiting and upcoming items agreed. The overdue estimate was USD 606.25. The blocked row kept its maturity date and had no estimated receipt date or amount. |
| Shell tests | Exercised an empty ledger, failed list and product reads with Retry, exact product targets, Back with focus and scroll, and Forward after opening a draft. A stale receipt link after settlement opened the settled facts without a new draft. An explicit settlement through the existing form removed the task on return and after reopening. Read-only interaction preserved activities and ledger version. |
| Linux X11 | In a 900×660 Xvfb window, opened the due receipt draft, checked upcoming and blocked products, returned to Tasks, quit and reopened. After first-launch preference initialization, the SQLite database was byte-for-byte identical across the read-only run. A separate explicit confirmation posted USD 600 principal and brought cash to USD 1,700. The settled item disappeared on Back and after another normal reopen. SQLite showed one new settlement operation and one linked redemption activity. A temporary build overlay seeded the verified synthetic profile and fixed its clock. |
| Tests | The Gio module suite and focused shared application, domain and Wails Go Upcoming Actions and liquidity tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | The [investment and reconciliation tabs](#investment-review-and-card-reconciliation-tasks) follow this milestone. Other Upcoming item types, standalone data checks and reminders remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Investment review and card reconciliation Tasks

**Investment review** lists actionable rule episodes from the shared published
review with the Wails `pending` filter and cursor contract. It shows the
published date, rule name, trigger date and evaluated window, and opens the
instrument's existing holdings detail by instrument ID. The published review
can be out of date. Gio displays the shared stale or repair status and
distinguishes a missing publication from a complete publication with no
pending items. Reload reads the publication. Rule evaluation and publication
remain separate service operations. [Wails investment review](../../../frontend/src/features/investment-review/InvestmentReviewPage.tsx) · [Published review read](../../../internal/application/review_alert_read.go)

**Reconciliation** lists open sessions for one active balance-mode credit card
from the same read as Wails. Each card uses the saved start date, open status
and checkpoint counts. Opening it selects the exact saved session in the
existing statement screen. The screen rereads that session on entry. Its latest
status governs what can be continued. Back restores the task tab, position and
link focus, and Forward can reopen the same session. [Wails reconciliation list](../../../frontend/src/features/reconciliation/ReconciliationPage.tsx) · [Shared session read](../../../internal/application/reconciliation_session.go) · [Existing Gio account statement workflow](#credit-card-statement-reconciliation)

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent marked ledgers triggered the published cost rule on November 21 in Asia/Singapore. Both reads agreed on the rule, trigger, window and daily evidence. Moving the clock one day forward surfaced the same stale status on the retained report. Both ledgers had two supported open sessions for the same card and two out-of-scope sessions. The paused and in-progress records retained their own IDs, counts and start time. |
| Shell tests | Covered unpublished review and no eligible card sessions, a late review read after switching tabs, stable instrument and card-session targets, Back focus, Forward to the exact paused record, and term-deposit regression. Activity records and the ledger version stayed unchanged. Both reads survived a normal reopen. |
| Linux X11 | In a 900×660 Xvfb window, followed the review task into the instrument's three-share holding and the paused card task into the saved statement with USD 30 owed. Returned to both task lists, revisited Upcoming, and repeated the two jumps after normal reopen. Once preferences were initialized, the full synthetic SQLite file was byte-for-byte identical before and after browsing. A temporary build overlay seeded the fixture and fixed its clock. |
| Tests | The Gio module suite and focused shared investment-review publication, Upcoming Actions and reconciliation tests passed. |
| Build and vet | The independent Linux Gio preview build and Gio vet passed. |
| Open evidence | Rule management, refresh publication, acknowledgement, suppression, evidence and coverage browsing, and the broader reconciliation workspace remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. |

#### Stage 5 review checkpoint

The experimental shell now supports Holdings and instrument detail, same-currency
buys including additions, partial sales and liquidation, cash dividends, and
one pre-History opening position with cost. Available Funds has a Today view
with bank reservation creation and release. Term deposits support read-only
terms and actual maturity receipt with linked History. Tasks covers term deposit
schedules, published pending investment checks, and supported open card
statement sessions.

Broader investment work still includes cross-currency securities, instrument
catalog maintenance, and wider investment lifecycle coverage. Product gaps
include new and existing product setup, early redemption, renewal, standalone
interest, and other managed product types. Tasks still has the broader Upcoming
item types and data checks, review management, general reconciliation, and
reminders. Production startup and target-platform acceptance also remain open.

The first independent read-only Stage 6 slice is Wealth History for a fixed
date range and base currency, with a keyboard-readable table. The existing
historical Overview series supplies daily values, missing inputs and coverage.
Return Analysis, Asset Changes, provider operations and repairs can be evaluated
separately.
[Wails historical series adapter](../../../internal/wailsapi/analytics/historical_series.go)

### Stage 6 progress

#### Read-only Wealth History

**Insights → Wealth history** opens on the most recent closed day in the
History timezone. Use the shared date pickers to read 1 through 31 consecutive
civil date labels, inclusive, starting at or after the recorded History
starting point. The range uses the saved household base currency. The loaded
range, currency, History timezone and read time remain visible. A failed
reload retains the previous capture. A replacement read or navigation away
cancels the outstanding query. Reload reads the ledger and stored quotes
through the same bounded replay as the Wails Trend view.
[Wails Trend](../../../frontend/src/features/historical-overview/HistoricalTrend.tsx) · [Shared historical series](../../../internal/application/historical_series.go)

Tab into the daily table and use the arrows, Page Up, Page Down, Home and End.
Each date shows net worth, assets and signed liabilities. Partial values are
labeled as known contributions or subtotals, and provisional totals are
identified. Each date shows its coverage counts, missing named inputs,
available observation dates and carried or stale quote counts. Changes in
valuation basis or inclusion scope and civil dates that did not occur in the
History timezone are called out separately. The shared service owns the
decimal values and coverage decisions. The table uses the usual currency and
display preferences.

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent read-only SQLite copies used AUD and Asia/Singapore. August 1 had missing USD FX, one of two asset components valued, an unavailable net total and a known AUD 24.90 net contribution. August 2 had exact net worth AUD 148.411725 and displayed AUD 148.41. The complete returned series matched the Wails adapter, including exact amounts, coverage, missing inputs and endpoint evidence. Both reads made zero provider calls. |
| Gio interaction | Frame tests covered History not started, the starting day before any day has closed, 31-day and timezone bounds, a skipped civil date, retained results after budget failure, Retry, cancellation after a replacement request or leaving the page, and keyboard movement through the table. Focused regression checks covered the existing sidebar popovers after page scroll, page-picker anchors, compact layout, Settings tabs and window-size persistence. |
| Linux X11 | The packaged preview launched from `preview-linux.sh` on Debian 13 amd64 with Go 1.27.4, Xvfb and Mesa software rendering. The verified synthetic profile showed USD 1,700.25 on November 3 and USD 1,734.13 on November 24. The keyboard selected and loaded the range, reached the table and moved between its first and last rows. Back and Forward kept the loaded range at 700×480. Normal reopen restored that size and loaded the same values when the range was selected again. The synthetic SQLite file had the same SHA-256 before and after the reopen and read. |
| Build and tests | The full Gio module suite, Gio vet, the independent Linux preview build, and focused shared application, domain and Wails analytics Go tests passed. The Component Preview popover regression checks placement below the footer at 1120×1000, above it at 1120×840, and after scrolling the compact window. It compares the actual anchor bounds and checks focus throughout. The native Linux preview reproduced those placements and Escape returned focus to the trigger. |
| Open evidence | Wails GUI, Linux Wayland, native macOS, VoiceOver and native IME remain unverified for this slice. Production-profile startup, release acceptance and cutover remain separate gates. |

The Wealth History endpoint drill-down, detailed source evidence, market data
refresh and Data Health repair remain open. Chart development and integration
are paused pending separately coordinated shared components.

#### Read-only Return Analysis calendar

**Insights → Return analysis** starts in Calendar. Previous month, Next month
and Today change the visible period in the History timezone. The monthly
summary precedes the seven-column grid and contributors, matching the Wails
information order. The summary and the grid use separate periods so an adjacent
week never changes the visible month's return. The grid respects the saved
week start. Select a closed day to open the right-hand detail with its return,
coverage, composition, contributors and available issues. Escape, Close and
clicking outside dismiss it and restore the selected day button. Reload and
Retry use the recorded month. Navigating back restores month and scroll.
[Wails Calendar](../../../frontend/src/features/insights/ReturnCalendarTab.tsx) · [Shared Go return projections](../../../internal/application/analysis_return_projections.go)

The first slice uses the Wails default household investment scope, base
valuation and cash inclusion. Gio presents shared precise return values and
rates with the usual display preferences. The shared read can maintain derived
historical snapshots locally, as it does for Wails. It does not post ledger
entries or refresh providers. Shared filters, year, Trend, Contribution and
cross-page actions are tracked separately in the [parity checklist](parity.md#return-analysis-and-related-navigation).

| Check | Evidence |
| --- | --- |
| Wails Go comparison | Independent synthetic SQLite copies closed February 1–3 in Asia/Singapore. The period and February 2 results matched at USD 25 and a decimal ratio of `0.25`, with the same rate coverage and one price component. Both copies made zero provider requests. |
| Gio interaction | Frame tests exercised failure and Retry for both the month and day reads, Monday/Sunday week boundaries, an open current day, and day-sheet focus restored after Escape and an outside press. |
| Linux X11 | The packaged `preview-linux.sh` launched a verified synthetic household on Debian 13 amd64 using Xvfb and Mesa. At 1120×900 it showed November 1–24, USD 25 return, +25%, and 24/24 rated days. November 2 opened Price change and its named contributor. Escape and Return closed and reopened that day. At 700×480 the detail stayed readable and dismissed from the backdrop. The activities, holdings, instrument quotes, account values and History origin tables matched before and after the verified run. |
| Build and tests | The full Gio module suite, Gio vet, native Linux launcher build and focused application/domain/Wails analysis projection tests passed. |
| Open evidence | Wails GUI, Wayland, native macOS, VoiceOver and native IME remain unverified for this slice. |

#### Return Analysis filters, Trend and Contribution

Calendar, Return trend and Contribution tabs sit above the shared filter card
and active content. Portfolio, account and instrument scope, valuation
currency, date endpoints, cash inclusion and additional account and currency
filters form one typed query. Leaving Calendar freezes its effective closed
range for the other projections. An explicit range overrides the return
summary. The calendar grid keeps its own visible weeks. Selected filters and
month survive tab changes. A native request that requires base valuation uses
the shared application's fallback.

Trend shows the shared period amount, linked period rate, coverage,
composition and the selected daily values. Contribution shows the shared
summary and ranking, with return type, grouping and ordering. Both begin with
keyboard-scrollable lists so the supplied daily values and ranked rows are
readable at small window sizes. The [parity checklist](parity.md#return-analysis-and-related-navigation)
tracks remaining presets and detail workflows. Chart development and
integration are paused.

| Check | Evidence |
| --- | --- |
| Wails workflow reference | The 30 tests in `ReturnAnalysisPage.test.tsx` passed. The Gio frame counterpart moved from September to August 2026. Both Trend and Contribution received August 1–31, the selected instrument and account filter, USD, native valuation and excluded cash. Returning to Calendar kept August and the effective range. |
| Shared Go comparison | Independent SQLite fixtures matched the Wails Go Trend amount and ratio, point count, and the first Contribution row's exact identity and amount. Both copies made zero provider requests. |
| Linux X11 | The packaged preview on the verified synthetic portfolio selected the named instrument and native valuation. Trend and Contribution showed USD 25 and +25% and retained November 1–24. Calendar retained the date range on return. At 700×480 the shared date fields wrapped to a second row, and Escape followed by Tab and Return moved from Scope to the Valuation picker. |
| Build and tests | The full Gio module suite, Gio vet, independent native Linux preview build and focused shared application/domain/Wails analysis suites passed. |

#### Asset Changes and the Return day link

**Insights → Asset changes** opens with Asset trend, Change drivers and
Categories tabs followed by the same filter card as Return Analysis. The
first trend view uses the daily net worth projection. The user can switch
among net worth, assets, liabilities or net change and day, week or month.
Change drivers shows the beginning and ending values, the net change, rate
and the shared driver groups. Categories shows the chosen shared category
total and attributed rows. The trend and category tables support keyboard
scrolling. The layout follows the Wails section order. Charts are paused
and the driver and category details remain open.

The Return Calendar detail action **View asset changes for this day** opens
Change drivers with that single date and the existing supported filters. Back
returns to the originating Calendar, leaves its month and filters intact,
and restores focus to the day button.
[Wails destination and filters](../../../frontend/src/features/insights/AssetChangesPage.tsx) · [Shared projections](../../../internal/application/analysis_projections.go)

| Check | Evidence |
| --- | --- |
| Wails reference | The 43 frontend tests across `ReturnAnalysisPage.test.tsx` and `AssetChangesPage.test.tsx` passed. The Go fixtures compare the linked day's amount, trend point and category total to the Wails adapter. |
| Gio interaction | A closed August day carried the selected instrument, additional account and USD, native valuation and excluded cash into Asset Changes. Back retained the August range and focused the originating day. |
| Linux X11 | The packaged Linux preview followed November 2 into Change drivers with the selected date in both fields. The synthetic value went from USD 100 to USD 125 and Price change showed USD 25. Back and Return reopened the same day. |
| Build and tests | The full Gio suite, Gio vet, native Linux preview build, and focused application, domain and Wails analysis Go tests passed. |

#### Contribution detail and Ledger activity

Selecting a ranked contribution opens a right-hand detail with the amount,
rate, rated days, contribution components and account or instrument
breakdown. It uses the same effective range and grouping as the ranking.
The Ledger activity action carries the shared hint's civil dates, account,
instrument and supported activity kinds. This includes a hint with
multiple kinds. The detail explains which currency and asset-class filters
Ledger activity cannot apply before leaving the page. Closing the detail
returns to the ranking. Back from Ledger activity restores the selected
ranking row and its keyboard focus.

| Check | Evidence |
| --- | --- |
| Wails reference | The 79 tests in `insightTabs.test.tsx`, `ReturnAnalysisPage.test.tsx`, and `AssetChangesPage.test.tsx` passed. The Gio fixture compared exact contribution values and the History hint against the Wails Go adapter. |
| Gio interaction | The Contribution sheet reported the active currency filter it could not carry. Ledger activity used the hint dates, account and instrument. Back restored the originating ranking row's focus. A History regression verified that multiple exact activity kinds survive copy and reload and that changing the activity type replaces the linked set. |
| Linux X11 | The packaged Gio preview opened Synthetic fund and showed USD 25, +25% and 24/24 rated days. The action opened Ledger activity with November 1–24, Synthetic broker and Synthetic fund. The fixture had no matching activities. Back followed by Return reopened the same contribution. The isolated synthetic fixture was verified again on exit. |
| Build and tests | The full Gio suite, Gio vet, the native Linux package build and focused shared application, domain, Wails analysis and Wails History Go tests passed. Interactive Wails, macOS, VoiceOver and IME verification remain open. |

#### Stored Market data

**Manage → Market data** opens on Instruments, with FX rates beside it. The
page puts its local reload action above the tabs and instrument search above
the type groups. It excludes instruments owned by managed products, as the
Wails Market data catalog does. The FX list combines saved source preferences
with currency pairs from Overview's missing FX inputs. Each row distinguishes
the configured source from the selected local observation and shows amount
or rate, quote direction, as-of information and explicit absence. Back and
Forward preserve the tab, search and scroll. Quote history, manual entry,
source configuration and provider operations remain open.
The current reload action refreshes the local view.

| Check | Evidence |
| --- | --- |
| Wails reference | All 23 tests in `MarketDataPage.test.tsx` passed. The Gio fixture matched the Wails Go selected quote at exactly USD 12.34567891 with delay evidence and the stored direction 1 USD = 0.92345678 EUR. JPY/USD showed a missing quote and source. The app made zero requests to the fixture provider. |
| Gio interaction | A rendered frame narrowed the list to Unquoted bond, retried a saved FX read failure, and restored the FX rates tab and the instrument search on Back. The Settings and Manage regressions distinguish the new Market data route from the Settings tab with the same visible name. |
| Linux X11 | The packaged preview showed the same exact instrument and FX values and the JPY/USD missing row. At 700×480 the amount wrapped and remained readable. Back restored FX rates, and Instruments retained the filtered bond. A second marked fixture launch compared all SQLite tables. The only row changes came from initial Settings configuration and its financial-source revision bookkeeping. |
| Build and tests | The full Gio suite, Gio vet, packaged Linux build and focused shared application, Wails quote and Wails market data tests passed. Interactive Wails, macOS, VoiceOver and IME verification remain open. |

#### Local Data Health diagnosis

The Market data introduction opens **Data health** without adding a separate
sidebar group. The local scan shows coverage, gap and prerequisite counts
before the executable, prerequisite and reported sections. Findings display
their severity, date range and available evidence. Related findings collapsed
by the shared report stay under their causes. Recheck repeats the local read
and a failed scan offers Retry. Provider prerequisites link to the Market data
tab in Settings. Account-value findings can open Accounts. Back restores the
originating button. Focused issue navigation, compaction of adjacent snapshot
days and manual-quote actions remain open.

| Check | Evidence |
| --- | --- |
| Shared diagnosis | The Gio interaction test compared the complete report against the Wails Go adapter. One unresolved manual JPY/USD prerequisite cleared after the fixture stored exactly 1 USD = 140.1234 JPY. Both scans made zero provider requests. Focused shared Go scan and plan tests passed. |
| Gio interaction | Injected failure exposed Retry. Recheck switched to the healthy summary after the stored quote. Another frame verified the provider-settings tab and returned keyboard focus to the issue action. |
| Linux X11 | The packaged preview on a marked synthetic profile opened Data health from Market data, displayed one blocking JPY/USD gap, rechecked, and retained the finding at 700×480. Back then Return reopened Data health. Logical row fingerprints matched for every SQLite table. |
| Build and tests | The full Gio suite, vet, packaged Linux build, focused application and Wails Go market data tests and the 16 Wails Data Health frontend tests passed. Interactive Wails, macOS, VoiceOver and IME verification remain open. |

#### Review, cancel and verify a Data Health repair

**Repair all** opens a right-hand plan with estimated requests, historical
targets, latest checks, snapshot work and prerequisites. Gio explains that
History must be started when the report identifies this prerequisite. The
plan is local and Escape returns focus to its button. **Start repair** begins
the shared application job. The report shows live progress and supports
cancellation. A fresh scan follows terminal outcomes so job success and
outstanding coverage stay visible independently. Work that survives page
navigation stays attached to the running preview process and is cancelled
and joined on quit. Focused repair actions, detailed per-target results and
snapshot-specific review remain open.

| Check | Evidence |
| --- | --- |
| Wails comparison | The Gio plan matched the Wails Go plan. All 24 frontend checks across Data Health, Snapshot Repair and Market Data Sync Bar passed. Focused shared Go tests covered repair preview, attach/conflict, cancellation and historical gap clearance. |
| Gio interaction | Ran the repair test three times. The first confirmed request blocked at a deterministic provider and was cancelled. The retry saved one exact provider rate of 140.1234. The fresh Gio report matched the Wails report and kept the historical gap visible after the latest quote was saved. The pre-History test verified the repair prerequisite. |
| Linux X11 | The packaged launcher opened the plan on a marked household and displayed one estimated provider request at 1100×900 and 700×480. Escape and Return restored the plan without storing a new FX quote. A separate native build with only a deterministic provider substituted at build time confirmed the plan, cancelled the first request, then saved the exact 140.1234 rate on a second confirmation. It showed Repair completed and the remaining gaps, and the isolated fixture was verified after shutdown. |
| Build and tests | The full Gio suite, Gio vet and vanilla Linux package build passed. Live providers, Wails GUI, native macOS, VoiceOver and IME remain unverified for this workflow. |

#### Additional analysis filters and effective dates

Return Analysis and Asset Changes show scope and the effective dates in
the primary row. More filters shows the valuation and cash options with
account, currency, asset class, member and instrument. A summary shows the
active choices while the controls are collapsed. Gio uses that summary and
the pickers for individual clearing, which keeps the compact header to one
summary row when it fits. The bulk clear action preserves the date range,
scope, valuation and cash setting. Editing one date keeps the other date and
respects the recorded History period. The Return day link carries the exact
filters into Asset Changes. Contribution detail explains that Ledger activity
cannot apply an additional member filter.

| Check | Evidence |
| --- | --- |
| Wails comparison | All 90 tests across Analysis Filter Bar, Return Analysis, Asset Changes and Insight Tabs passed. Focused shared Go query and analysis tests passed. On independent synthetic ledgers, Gio and the Wails Go adapter matched the contribution with USD, Stocks, the member and the named instrument selected. |
| Gio interaction | August 1–31 stayed effective across Return tabs. Calendar day → Asset Changes retained the additional instrument, member, asset class and earlier account, currency, native valuation and cash choices. Back restored focus and the originating filters. Clearing the additional set retained other choices and focused More filters. |
| Linux X11 | The packaged preview selected Stocks, Sample owner and Synthetic fund for November 1–24 and retained them in Contribution and at 700×480. The contribution remained USD 25 and +25%. Its detail showed separate member and currency or asset-class notices before Ledger activity. |
| Build and tests | The full Gio suite, Gio vet and native Linux package build passed. Presets, individual filter chips, interactive Wails, macOS, VoiceOver and IME remain open. |

#### Return Calendar year and month detail

Calendar offers Month and Year after the period controls. Year displays the
shared year summary and a responsive grid of monthly return amounts, linked
rates and rated-day coverage. A selection can override the summary range
while month cards retain the visible year's scope. A month card opens the
selected month and moves keyboard focus to the Month control. Months outside
the recorded History period show their missing state and are disabled.

| Check | Evidence |
| --- | --- |
| Wails comparison | The year frame used the same shared February month fold as the Wails Go adapter. Amount USD 25, rate +25% and rated-day coverage matched. All 30 Wails Return Analysis frontend tests and the focused shared Go calendar tests passed. |
| Gio interaction | A failed Year read exposed Retry. The recovered result opened February and focused the Month control, after which the existing day-detail and Asset Changes interaction regressions passed. |
| Linux X11 | The packaged preview showed the November Year card at USD 25, +25%, 24/24. At 700×480 the cards wrapped and the November result was reachable by scrolling. Selecting it showed November 2026 and November 1–24 in Month. The synthetic fixture verified after quit. |
| Build and tests | The entire Gio module suite, Gio vet and native Linux package build passed. Native macOS, VoiceOver and IME remain open. |

### Follow-up Linux acceptance

Independent Linux QA on an earlier preview found a Directory caption,
Tasks navigation behavior and database changes across a combined analysis
tour. The packaged preview repeated those workflows on isolated profiles.

| Workflow | Disposition and evidence |
| --- | --- |
| Onboarding → Directory | A new synthetic household saved **Atlas Willow household** and **Willow Sample**. Directory showed the fixed synthetic caption even though the entered name was saved. Directory now uses the household name from the authoritative startup read. After quit and reopen the packaged preview displayed both entered names. A Gio regression compares onboarding against the Wails Go adapter, then reopens the stored profile and checks the Directory labels. |
| Account → Investments → Tasks | Opened Synthetic broker, which showed Synthetic fund under Investments. In Tasks, Investment review showed no published review and Reconciliation showed no open statements. Back returned to the same brokerage detail and Forward restored Reconciliation. Wails updates the Tasks tab within one navigation entry, so both surfaces return to the previous page. The nine Wails Tasks tests and seven matching App navigation tests passed. |
| Wealth History evidence | On a fresh priced fixture, November 24 showed USD 125 known net contribution, assets coverage 1/2, quote date November 1 and the named missing AUD exchange rate for Synthetic Australian cash. A read-only query through the Wails Go adapter matched the subtotal, quote date, missing account and coverage. |
| First launch and analysis writes | Fingerprinted SQLite rows before launch, after the initial Overview, after Wealth History and after Return Calendar. First launch attached the shared Settings store and added the durable settings row and its financial-source revision record. Wealth History changed no SQLite table. Return Calendar materialized 24 daily snapshots and 48 items and advanced the snapshot state through the shared analysis service. Accounts, activities, holdings, source quotes and settings matched across both analysis reads. |
| Platform verification | Packaged Linux X11 and the full Gio module suite with vet passed. Native macOS, VoiceOver and IME acceptance remain open. |

## Moving from the preview to a desktop build

1. During the pilot, run the Gio module's tests explicitly. The root
   `go test ./...` does not traverse the nested module. Regular Mac CI runs
   the Gio internationalization tests separately. Build the Gio
   development bundle to check actual macOS interactions. Keep framework
   dependencies in the Gio module while comparing the two shells.
2. As startup and integrations migrate, move shared non-UI orchestration
   behind Go entry points used by both shells. Keep toolkit-specific window,
   dialog, file-picker and menu adapters at the edge. Introduce an opt-in
   Gio packaging target that uses these owners and the existing app identity.
3. Run the release candidate and reference build sequentially on equivalent
   copies of each fixture. A parity record should name the action and expected
   outcome, such as correcting an entry, reopening its account and following
   the linked reversal. Classify any discrepancy as a defect, an intentional
   UX improvement with acceptance evidence, or an open release gate.

## Parity and cutover decision

For each migrated journey, record the source revision, the reference Wails
path, the synthetic scenario, the Gio result and any remaining gap. Compare
decimal values, missing data, household-local dates, IDs used for internal
links, provenance and resulting stored state. Run destructive recovery
scenarios on separate synthetic copies. Exercise localization, errors, focus,
window resizing and restoration through the actual desktop entry point.

Measure first actionable window, first populated list, page load and scroll
frame times, and steady/peak process memory using the same Mac, data,
page size, build mode and interaction script on both shells. Include Wails'
attributable WebKit helpers. Separate cold launch, reopen and warm navigation.
Start with small paired trials and publish the measured range and method before
claiming a responsiveness or memory improvement.

Switch the default only after the feature inventory has acceptance evidence,
keyboard/VoiceOver and IME pass on the supported Mac, and upgrade, backup,
restore, signing, notarization and rollback to Wails have been rehearsed.
Automated frame tests, a successful native build and an interactive Mac run
are separate evidence. Retain a compatible Wails build through the cutover.
Use the [isolated native QA workflow](../../../docs/development/local-workflow.md#isolated-native-development-qa)
for every development launch.
