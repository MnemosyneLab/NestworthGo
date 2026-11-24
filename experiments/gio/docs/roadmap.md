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
filters, detail and write actions remain part of later History work. Stage 2
is in progress. Stages 3–7 are planned. Use [navigation history](../../../docs/design/navigation-history.md),
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
| Institutions lifecycle | The pilot adds an institution with a name and type, renames it, asks for confirmation before archiving, and restores it from **Show archived**. The default view lists active institutions. **Show archived** includes both states and name search applies to both views. See the milestone evidence below. |
| Remaining | Institution icon customization and display, members, groups, navigation history, object links, and wider navigation-state restoration are still open. Native VoiceOver and full keyboard traversal remain open. A macOS accessibility client can now navigate via the sidebar, edit a search field and choose a select option. Linux desktop interaction for the other Stage 2 workflows remains open. AI, MCP and backup workflows belong to stage 7. |

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
