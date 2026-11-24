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
durable General settings and the four-language sidebar. Overview still shows
sample balances. The stage records below track which workflows are available
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
Stages 3–7 are planned. Use [navigation history](../../../docs/design/navigation-history.md),
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
| General settings | Appearance, accent, language, timezone, date format, time format and decimal/grouping separators have draft, save, discard and confirmed reset. The page displays household currency and the origin timezone when it differs from the display zone. Ledger dates, preview pickers and Overview sample amounts use the applicable saved display formats. |
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
supported entity kind. Account, instrument and activity-detail links depend on
their later-stage pages. Back and Forward restore the state of the supported
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

#### First Stage 3 slice

Extract shared startup checks, blocked-state handling and single-instance
ownership behind a common Go boundary. Exercise usable, rejected and already
owned paths with disposable ledgers through the desktop entry point before
exposing ledger selection or account workflows.

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
