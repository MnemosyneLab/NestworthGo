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
kind, civil date and note. Account and instrument names, amount breakdowns,
filters, detail and write actions remain part of later History work. The
scoped Stage 2 workflows are implemented. Platform acceptance remains open.
Stage 3 includes synthetic startup, ledger ownership, household onboarding,
live Overview, account browsing and standard bank account creation. Stages 4–7
are planned. Use [navigation history](../../../docs/design/navigation-history.md),
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
Gio uses the same ownership boundary and blocks on a pending journal, leaving
it untouched. Gio still requires the pilot marker and has no ledger chooser.

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
| Open evidence | Wails GUI, native macOS and Wayland were not run. Production startup, ledger selection and recovery, broader account creation, account settings, ownership changes, archive and restore remain Stage 3 work. |

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
