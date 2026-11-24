# Gio desktop migration roadmap

## Target and starting point

A household should be able to complete the same desktop journeys with Gio as
with Wails, using the same ledger, settings, financial results and recovery
guarantees. Wails is the shipping desktop and the reference for each comparison.
Develop Gio in the existing [standalone module](../README.md)
until a release candidate passes the cutover gates below.

At the planning baseline `c682277e`, Gio has the shell, three session
languages (zh-CN, en, zh-TW), two appearances, five accent themes, shared
editors/search/select/date/time controls and a development preview. Directory
can list, search and rename active institutions through the application
service. Overview contains sample balances. Other navigation labels do not
establish feature coverage. The [foundation design](foundation.md)
records the control contracts and the open macOS accessibility issue.

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
| 2. Shell and directory | Complete members, institutions and groups, including create, edit, archive, restore, icons and filters. Add navigation history, object links, retained drafts/scroll/focus, shared query states, durable preferences, system appearance/language and Japanese. | Reopen a saved change and preferences. Test keyboard-only navigation, focus restoration, IME composition and VoiceOver on a real Mac before migrating forms in bulk. |
| 3. Household, balances and accounts | Factor shared startup/blocked-state handling and lifecycle services. Implement onboarding, live Overview, institution-first account creation, account detail, ownership and valuation. | Compare populated, empty and partial valuations across shells. Verify restart, blocked/recovery paths and single-instance ownership using disposable ledgers. |
| 4. Ledger writes and history detail | Implement posting, preview and confirmation, History filters and detail, origin/starting-point workflows, repeat, correction and reversal. Add cross-page invalidation, including external writes. | Identical inputs produce the same preview, stored facts, provenance, linked corrections, balances and ordering. Test duplicate clicks, failures, retry and civil-date boundaries. |
| 5. Investments, products and tasks | Portfolio overview/holdings, instrument detail, trades and dividends. Available Funds, reservations and product lifecycle. Tasks for upcoming actions, investment review and reconciliation. | Compare lifecycle scenarios, valuation quality and reasons for unavailable actions. Follow links from a task or product through to its supporting facts. |
| 6. Analytics and maintenance | Wealth History, Return Analysis and Asset Changes, accessible charts and tables, market data and actionable Data Health. | Compare time windows, currencies, provenance, incomplete inputs and empty states. Ensure explicit provider refresh, keyboard access to chart data and authoritative repair results. |
| 7. Desktop integrations and distribution | Complete Settings, backup/restore/export, mobile pairing, MCP runtime, built-in assistant, credentials, native menus/dialogs, About and shutdown. Assemble the signed distributable and verify update and fallback. | Exercise each integration and restore/restart from a backup. Validate mobile assets, sidecar lifecycle and platform packaging on the supported target. |

Stage 1 is implemented on the experimental desktop. The current list displays
kind, civil date and note. Account and instrument names, amount breakdowns,
filters, detail and write actions remain part of later History work. Stages
2–7 are planned. Use [navigation history](../../../docs/design/navigation-history.md),
[history and form defaults](../../../docs/design/history-and-form-defaults-ux.md),
[account interactions](../../../docs/design/account-container-interaction.md) and the
[design index](../../../docs/design/README.md) for the detailed behavior behind each row.

### Stage 1 acceptance

| Check | Evidence |
| --- | --- |
| Storage and pagination | A synthetic checking account has six posted activities. SQLite integration tests reopen the fixture, walk it with a small page limit and check household dates and order. Model tests exercise the 50-row request, full continuation cursor, retry and refresh. |
| Desktop interaction | Gio frame tests open the route from the sidebar, exercise keyboard scrolling, switch languages, retry a failed append and check the retained rows. |
| Native macOS ARM64 | On macOS 27.2.1, the development bundle displayed all six rows, switched between Traditional and Simplified Chinese, reloaded them, and read the same six after quitting and reopening the verified fixture. The newest row, “Bank interest,” displayed November 12 in the household timezone. |
| Open evidence | The inspected macOS accessibility tree exposed window controls but did not expose the activity list or sidebar. VoiceOver parity remains open. The six-row native run does not measure large-list performance. Linux runtime acceptance and a paired Wails performance trial are pending for this revision. |

## Moving from the preview to a desktop build

1. During the pilot, run the Gio module's tests explicitly. The root
   `go test ./...` does not traverse the nested module. Build the existing
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
