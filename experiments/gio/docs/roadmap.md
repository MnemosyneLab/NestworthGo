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
- Use Wails' startup, locking, migrations and recovery contracts for Gio's
  own local data directory. Keep development and acceptance on separate
  synthetic copies. Production rollout and release acceptance remain gated.
  The shared instance lock coordinates writable ownership for both shells.
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

#### Startup and ledger ownership

Gio's ordinary launch opens its own `data/nestworth.db` and
`data/settings.json`. The default macOS directory remains
`~/Library/Application Support/com.nestworth.gio/data`. An empty database
opens household setup. A local Wails-format copy opens through the same
ordinary launch and accepts writes in the Gio copy. Stale files from
earlier preview builds have no role in the decision to open a database.
[Mac copy instructions](../README.md#use-a-local-wails-copy-on-macos)

Gio takes the [shared instance lock](../../../internal/infrastructure/backup/instance_lock.go)
and uses the [same restore reconciler](../../../internal/infrastructure/backup/journal.go),
SQLite schema and migrations, shared settings attachment and household
bootstrap as the [Wails entry point](../../../cmd/nestworth/main.go).
A failed startup releases the lock. The blocked window distinguishes lock,
recovery, unavailable and schema problems. The local backup chooser in
Wails' blocked window remains open in Gio.

| Check | Evidence |
| --- | --- |
| Ordinary copied ledger | A synthetic Wails-format database and settings were copied into the default Gio location under a fake config root. The automation retained the household ID, account ID, History timezone and full settings, exercised the shared lock, wrote an institution and found its same ID after reopening. It compared the Wails source files byte for byte and left old fixture metadata untouched. |
| Startup failures | Automated checks covered a fresh directory reaching onboarding with no household, a canonical lock through a symlink alias, and refusal of an empty, corrupt or unsupported existing database. Failures preserved the SQLite bytes and released the lock. |
| Linux X11 default launch | The packaged app launched with both path overrides unset. It read the synthetic AUD 214.79 valuation and ordinary household and member names. A Group added through Directory persisted after normal quit and reopen. Source database and settings SHA-256 hashes matched. A separate fresh launch showed household setup with zero stored households. A schema-99 file showed the newer-version notice and retained its exact bytes. |
| Build and tests | The full Gio module suite, Gio vet, Linux packaging, packaging tests and focused shared Backup, SQLite and Settings tests passed. |
| Release evidence | Native Mac, Wayland, VoiceOver and a real local copy remain unverified. Production rollout and cutover remain gated. |

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

#### Overview asset composition Sankey

The live Overview feeds the native Sankey from #288 with the shared
`OverviewAssetComposition` used by Wails. This preserves the real paths
from #287, including separate holdings classes in the same account and
unassigned institutions. Amounts are formatted in the household currency.
The valuation read date, quote and FX reference dates, and oldest carried
reference date have separate labels. Freshness, missing coverage, zero
components and negative amounts come from the same Overview result. A
failed reread keeps that result visible with Retry.

The chart and the complete keyboard-scrollable path list open the same
account ID. Back restores the originating control and scroll. A control
above the diagram jumps directly to the details so the list stays easy to
reach even with many chart rows. At compact widths Wails #287 keeps the
scrollable diagram and opens its complete details. Gio switches to labeled
bars grouped by level so the columns fit the window. Its complete list
keeps every class, institution, account and amount available by keyboard.

| Check | Evidence |
| --- | --- |
| Shared projection | The independent read-only comparison used a stale stored FX observation and unavailable provider. Gio matched Wails' AUD 123.51 asset display and exact AUD 98.411725 net worth, exposed the stale quote, and issued zero provider requests. The dense adapter test retained all 160 paths and their exact account targets. A separate Wails comparison preserved USD 123.45 cash and USD 91.34 stocks for the same account and the USD 0.25 account. The three exact amounts reconciled to USD 215.04. |
| Packaged Linux X11 | The 131-path synthetic household showed USD 567.04 known assets, one missing USD/AUD rate, and one zero component. The USD 123.45 cash and USD 91.34 stock paths each opened the same holdings account at USD 214.79. Account navigation opened the USD 0.25 path, and Back and Enter reopened it. End reached that path at 1260×1050 and 640×480. A second fixture at 640×480 showed AUD 214.79 and grouped cash and stock bars. Its November 25 valuation read date and November 1 quote and FX reference dates were separate. Both synthetic source database and settings hashes matched after quit. |
| Automated checks | Full Gio suite, Gio vet, native Linux package, focused shared Overview tests and 21 Wails Overview and Sankey frontend tests passed. |
| Open evidence | Native macOS, Wayland and VoiceOver remain open. Overview supporting cards and links and Portfolio allocation have separate parity work. |

#### Overview net-worth history and coverage links

Overview shows the shared net-worth series before allocation. The 30-day,
year-to-date, one-year and all-history presets use the shared trend
service. The chart and daily data keep a missing total separate from a
known subtotal and distinguish the current estimate from closed days.
The service may materialize derived daily snapshots, as in Wails.
The History entry opens Wealth History when a starting point exists and
Ledger activity's starting-point workflow otherwise.

Period and date coverage links open the selected local dates in Data
health. Current missing rows carry the source account, instrument or FX
pair when available. Data health uses its shared saved report, including a
matching collapsed finding, and offers a route back to all findings.
Back to Overview restores the original coverage control and position.

| Check | Evidence |
| --- | --- |
| Regression | The routed test distinguishes open and closed days, filters an unrelated date and FX pair, includes a matching collapsed finding, opens the precise daily scope, and restores keyboard focus on Back. |
| Packaged Linux X11 | Overview displayed USD 112.75 current net worth and 23 of 24 closed days valued. Data health opened November 1–24, including the November 4 snapshot gap and AUD/USD prerequisite. Back and Enter reopened that range. Wealth History opened from the Overview card. At 640×480 the period, currency, closed-day count and coverage action remained visible while scrolling. The source database and settings hashes matched after quit. |
| Follow-up | [Focused snapshot inspection and repair](#focused-snapshot-inspection-and-repair) uses this coverage handoff. Wails' arbitrary custom date ranges and native Mac, VoiceOver and Wayland verification remain open. |

#### Overview allocation summaries and filtered accounts

The Wails layout places the asset and liability categories after the history
card, then the Sankey and the institution, account type, member and group
breakdowns. Gio follows that order. The summary charts read the shared
amounts and basis-point shares and show unpriced components separately. Signed
bars keep negative category amounts visible when a ring cannot represent the
breakdown. The dimension rows open Accounts with the shared filter. Unassigned
institution and group rows match accounts without that assignment. Those
filtered results also scope the valuation read.

Gio adds **View allocation breakdowns** above the scroll area. It places
focus on the first breakdown link so keyboard users can reach the filters
without traversing every Sankey path. Pairs of cards sit side by side when
space permits and stack in a narrow window. Back from account detail restores
the scoped Accounts list. Toolbar Back restores the source Overview row.

| Check | Evidence |
| --- | --- |
| Gio regression | Shared SQLite fixture verified institution, account type, member and group destinations plus both unassigned filters. The compact frame verified focus order, exact account detail and both Back steps. Chart tests verify signed debt and a signed asset breakdown with the shared exact shares. |
| Wails reference | 148 frontend tests across Overview, Sankey, Net Worth Trend, Data Health and Accounts passed. Focused shared Go Overview, trend, health and Portfolio tests passed. |
| Packaged Linux X11 | At 1260×1050, both category cards showed USD 125.25 in assets and −USD 12.30 in liabilities. The Overview History action opened the Ledger activity starting point. The Sample bank link included Selected savings and excluded Unassigned cash. After opening the account, Back to accounts restored the scoped list. Toolbar Back and Enter reopened the same filter. At 640×480, Tab moved from Sample bank to Unassigned institution, which opened only Unassigned cash. Back and Enter reopened that filter. An empty synthetic household showed Open accounts and the Accounts empty state. All three synthetic sources retained matching database and settings hashes. |
| Checks | Full Gio tests, Gio vet and the native Linux package passed. Wails adapter tests passed for analytics, account and portfolio. Native macOS, Linux Wayland, VoiceOver and native IME remain open. |
| Remaining | Overview's home Tasks, upcoming actions, recent activity and provider repair actions remain open alongside custom history dates. Continue with focused Data Health snapshot breakdown and repair, then Market Data manual prices and FX rates with explicit save, retry and persisted evidence. The broader Stage 1–6 workflow gaps remain in the parity checklist. |

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

#### Interrupted startup recovery

Gio calls `backup.ReconcileOnStartup` under the instance lock, matching
Wails. The shared reconciler owns validation of journal paths, phase,
checksums, safety copies and restored settings. The fixture-specific
sealed recovery route has been retired.

A test seeds ordinary synthetic data through the shared service, installs a
backup with the USD 42.25 reserve over an outgoing USD 99.50 value, then leaves
the valid `replacement-installed` journal. The Gio session finishes the
journal, restores the saved Mint, Dark and day-first settings and reopens
with the same reserve value. A separate malformed-journal regression checks
that the existing database and journal stay byte-identical and the lock is
released. The shared Wails backup startup and lock tests passed.

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
| Posting, preview and confirmation | Contributions, withdrawals, same- and cross-currency transfers, Holdings cash and within-account FX, scoped balance targets, [card and debt posting](#card-purchases-refunds-and-debt-posting), and [cash-side debt entry and liability reasons](#cash-side-debt-entry-and-liability-reasons). | Expense categorization, broader account value updates and statement reconciliation, and additional backdated cases. |
| History, starting point and amendments | Stable Activity detail, filters and cursor paging, History and onboarding starting points, the [liability account Starting Point handoff](#starting-point-from-a-liability-account), repeat for supported cash movements, and linked cash corrections and reversals. | Multi-position opening and cost basis, Starting Point handoffs from more account action types, repeat list menu, broader amendment editors, within-account FX calculator and remaining statement interactions. |
| Cross-page and external invalidation | Confirmed writes refresh History, Account and Overview. History Activity links open the exact account and return to the existing visit. A write made through the same shared application service refreshes the active ledger pages without a page callback. | Exercise connected Mobile and MCP clients when their Gio lifecycles are wired. Keep production startup, target-platform acceptance and Wails GUI parity open. |

The [Holdings reads](#holdings-and-instrument-detail) and [first buy](#first-recorded-security-buy)
use isolated Holdings fixtures. Importing existing positions will require
separate evidence for their opening quantity and cost. The first-buy fixture
starts without holdings and verifies shared settlement, basis, valuation and
linked History after a duplicate confirmation, read failure and restart.

#### Card purchases, refunds and debt posting

**Accounts → balance liability → Record a change** follows the scope and
form order of [Wails account actions](../../../frontend/src/features/accounts/AccountActionSheets.tsx)
and the [Record Change form](../../../frontend/src/features/history/RecordChangeForm.tsx).
Credit cards default to Card purchase and offer Card refund. Balance
liabilities also offer Debt draw and Debt payment, with a required cash
endpoint and optional interest or fee on payments. Eligibility, persisted
activity and replay use the shared ledger. Card amount defaults to the card's
currency, and the shared command requires matching currencies for debt
principal. The form captures the current instant in the Origin timezone and
shows the editable local date and minute. Editing either field uses that civil
time. The confirmation shows exact amounts and signed before/after balances.

Gio uses a scrolling account page and wraps its controls at compact widths.
After confirmation it opens the exact recorded Ledger activity, with a Back
action that restores the selected account and the Record a change focus.
This gives Gio immediate readback where Wails closes the account action
sheet. Cancelling leaves the ledger unchanged. An out-of-date review returns
the amount and note to the form. An uncertain reply retains the frozen
submission for retry with its original mutation identity.

| Check | Evidence |
| --- | --- |
| Gio and Wails Go parity | Independent synthetic ledgers matched card purchase USD 12.3456 and refund USD 3.12, including the card reasons, signed values, notes, and effective instant and civil date. The debt draw of USD 20.125 and payment of USD 10.25 with USD 1.0001 fee matched Wails review rows and persisted effects. |
| Failure, keyboard and scope | The Gio harness checked review Cancel and focus restoration, invalid precision and fee, excess refund, stale preview, a duplicate click while a save was blocked, lost-reply retry, retrying a failed Ledger detail read and replay after reopening SQLite. The unrelated USD 105 account and its prior activity were unchanged. Wide and 620 dp viewports kept the same source actions and exact activity links. |
| Linux X11 and persistence | The standard package opened a synthetic profile copy at 1160×820 for card posting and 640×480 for loan posting. Cancel and Ledger Back restored Record a change, and Return reopened it. After quit and reopen, Ledger showed four postings. SQLite showed USD 39.2256 owed on the card, USD 59.875 owed on the loan and USD 108.8749 in Checking. The unrelated reserve retained USD 245 and its prior income. Source database and settings hashes matched. |
| Regression suites | Full Gio tests and vet, the standard Linux package, three focused Gio runs, the application, Wails History adapter and domain Go suites, and 129 Wails Accounts and Activity Detail frontend tests passed. |
| Open scope | [Cash-side debt entry and liability reasons](#cash-side-debt-entry-and-liability-reasons) have separate evidence below. Liability-specific amendment editors remain open. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver remain unverified. |

#### Cash-side debt entry and liability reasons

From an active Balance or Holdings cash account, **Record debt activity**
locks that cash account and offers Debt draw and Debt payment. Choose the
active liability account and enter principal in the cash account currency.
Payments allow optional interest or fees. The form uses the same debt
commands and eligibility as the [Wails Record Change form](../../../frontend/src/features/history/RecordChangeForm.tsx).
Gio gives this subset its own entry while the remaining cash-account
actions retain their existing placements. If History has no Origin, the
shared Starting Point form retains the initiating account action. It resumes
with the debt selector focused when the Origin has been verified.

On a liability account, the existing **Record a change** entry offers
the Wails increase and decrease reason sets, with card purchase and refund
as credit-card defaults and Other as the ordinary loan default. Outstanding
balance correction uses Reconciliation and accepts an exact zero target.
Review and Ledger detail show the effective reason and the direction of
the amount owed.

| Check | Evidence |
| --- | --- |
| Wails and Gio comparison | Independent synthetic profiles matched a cash-side draw, a Holdings payment and the liability Interest, Fee, Reconciliation decrease and zero-target correction. The Holdings payment used USD 10.125 principal and USD 0.0001 fee. It left USD 14.9999 and preserved the EUR 25.125 balance and the separate Balance cash account. |
| Guard, retry and focus | The compact Gio test cancelled and reentered Starting Point, restored the starting account and keyboard focus, excluded archived debt options and showed a currency mismatch. It kept the selected debt and principal across a stale review. A blocked duplicate click and a lost acknowledgement replayed the same operation, with USD 130.125 in the cash account and USD 70.125 owed after reopen. |
| Packaged Linux X11 | At 1160×820, cancelling the Starting Point review left zero origin rows and activities. At 640×480, the selected checking account resumed at the debt selector after confirming the Starting Point. Tab reached the principal and Return confirmed the preview. After a cash-side draw and payment, the wide loan form posted USD 1.2345 as Interest. Reopened SQLite held one Origin, three activities, USD 108.8749 in checking and USD 61.1095 owed on the selected loan. The card and reserve amounts and the synthetic source hashes matched. |
| Checks | Full Gio tests and vet, standard Linux packaging, focused application, domain and Wails History adapter tests, and 129 Wails frontend Accounts and Activity Detail tests passed. Interactive Wails, macOS, Wayland, VoiceOver and native IME remain unverified. |
| Open scope | Other account-action Starting Point handoffs, liability-specific amendment editors, broader account value actions and ledger entry shortcuts remain open. |

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
| Open evidence | The [liability account entry](#starting-point-from-a-liability-account) has separate evidence below. [One positive opening position](#opening-position-and-original-cost) is covered in Stage 5. Wails GUI, native macOS, Linux Wayland, native IME and VoiceOver were not run. The Stage 3 production startup and target-platform cutover gates remain open. |

#### Starting Point from a liability account

**Accounts → liability account → Record a change** opens the shared Starting
Point form when the household has no Origin, following the
[Wails account sheet](../../../frontend/src/features/accounts/AccountActionSheets.tsx).
The account stays visible in the form and the preview shows all household
opening balances. An origin read error offers Retry before displaying the
start form. Cancel restores focus to Record a change. After confirmation,
Gio reads back the persisted Origin before resuming the same card or debt
form with the selected account, its currency and the Origin timezone.

| Check | Evidence |
| --- | --- |
| Shared behavior | A synthetic ledger and an independent Wails reference both captured the Origin on November 22 in Asia/Singapore. The account origin read failure offered Retry. Cancel produced zero writes. A lost confirmation reply and failed verification blocked navigation until a read-only retry recovered the one Origin. The continued USD 1.2345 purchase matched Wails and survived reopen. |
| Linux X11 | In the packaged app, cancelling a review at 1160×820 left no Origin or activities. At 640×480, Return reentered the same account action and confirmed the Starting Point. SQLite showed four opening balances in the America/Vancouver Origin and zero activities. The resumed card form separately posted USD 2.3456. Back and Return restored Record a change. Restart showed one Origin and that one card purchase, and the synthetic source hashes matched. |
| Suites and remaining scope | The full Gio suite and vet, repeated focused Gio regressions, standard Linux package build, focused shared application and Wails History Go tests, and six Wails frontend account reference tests passed. Additional account-action handoffs remain open. See the [multiple opening positions checkpoint](#multiple-opening-positions-and-native-costs) for separate cost evidence. Interactive Wails, macOS, Wayland, native IME and VoiceOver remain unverified. |

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
| Open evidence | [Existing-position buys](#same-currency-buys-into-an-existing-holding) follow this milestone. [Foreign settlement](#trades-in-a-foreign-settlement-currency) and [opening positions](#multiple-opening-positions-and-native-costs) have later milestones. Backdated sale insertion, instrument maintenance and tax reporting remain separate. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun for this slice. |

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
| Open evidence | [Cash dividends](#same-currency-cash-dividends) and [opening positions](#opening-position-and-original-cost) follow this milestone. The [foreign settlement milestone](#trades-in-a-foreign-settlement-currency) covers a new instrument in a populated account. Backdated additions that change later cost replay and instrument maintenance remain open. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun for this slice. |

#### Trades in a foreign settlement currency

**Accounts → Holdings account → Record buy / Record sell** supports ordinary
instruments quoted in a different currency than the account's default. The
buy picker also offers unheld instruments on a populated account. A sale
requires a positive holding. The instrument fixes the native currency for
gross proceeds, fee and the signed settlement delta. The form names that
currency and points to the separate **Record conversion** workflow when
funding is needed. A buy spends the selected cash balance, including the fee,
at the effective time. The shared replay also checks later recorded balances.
The form carries an optional note and the precise effective time in the
History Origin timezone. [Wails trade form](../../../frontend/src/features/history/RecordChangeForm.tsx) · [Shared trade and FX rules](../../../internal/domain/change.go)

Preview shows native cash before and after, exact execution price and
applicable position cost. Review freezes one mutation. A competing change
requires another preview and preserves compatible input. Retrying an
uncertain confirmation returns the committed activity and existing Holding
ID. Cancel and Back from the linked Ledger receipt restore the entry action.
The compact form stacks the amount editors and retains the currency labels.

| Check | Evidence |
| --- | --- |
| Wails and Gio parity | Independent Wails-format synthetic ledgers first rejected an unfunded EUR buy. A distinct USD 120.25 to EUR 100.125 conversion charged USD 0.1234. After a competing EUR deposit invalidated the first preview, three units bought for EUR 60.0001 plus EUR 0.0002 recorded an execution price of EUR 20.00003333 and a EUR −60.0003 cash delta. Selling 1.25 units for EUR 30.1251 less EUR 0.0001 recorded EUR +30.125 and left 1.75 units. Wails Go agreed on the command, linked holding, ledger and wallet balances across restart. The domestic holding and the USD wallet were unchanged by the EUR trades. |
| Packaged Linux X11 | Posted a separate conversion and a buy at 1160×820, followed by a sale at 640×480. Selected the instrument and confirmed trades by keyboard. Back from each Ledger detail restored the initiating action. After quit and reopen, read-only SQLite showed exactly one conversion, one buy and one sale. The two trades shared a Holding ID, retained both EUR fees and left EUR 70.2497 and USD 79.6266. The unrelated domestic holding and its USD 40 quote remained intact. The untouched synthetic source database and settings matched their original hashes. |
| Regression checks | The Gio tests covered cancel, stale review, note and civil-time handling, unchanged domestic positions, historical shortfall, duplicate confirmation and a lost response. The full Gio suite and vet, focused application/domain/Wails Go tests and four Wails frontend reference cases passed. |
| Remaining coverage | Wails' optional market-price calculators in the trade form, more backdated additions or sales that change later cost replay, inline instrument creation, and foreign dividends remain open. Interactive Wails, native macOS, Wayland, VoiceOver and native IME were unrun. |

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

The initial milestone proved **Accounts → Holdings account → Record existing
position** with one positive, same-currency position. The
[multiple opening positions checkpoint](#multiple-opening-positions-and-native-costs)
now extends the entry to several securities.
The Wails account form calls `CreateHolding`. Its pre-History branch saves
the quantity observation and leaves the optional cost argument unused. Gio
reviews the quantity before calling that same branch and checks the persisted
holding when a reply is uncertain. [Wails account action](../../../frontend/src/features/accounts/AccountActionSheets.tsx) · [Wails Go holding adapter](../../../internal/wailsapi/holding/holding.go) · [Shared holding command](../../../internal/application/portfolio.go)

Next, **Ledger activity → Start recording history** asks for the original cost
per unit. Enter zero when the original cost is zero. The shared planner uses
the entered basis and preserves any market quote separately. An explicit
cost also allows capture when a market quote is missing. The review includes the quantity, original
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
| Open evidence | [Multiple positions and native cost currencies](#multiple-opening-positions-and-native-costs) have separate evidence below. Managed products and the History-enabled position-import operation remain distinct write paths. Wails GUI, native macOS, Wayland, native IME and VoiceOver remain unrun. Stage 3 production startup and target-platform cutover remain open. |

#### Multiple opening positions and native costs

Before History starts, add another ordinary security in the same or a different
Holdings account. The picker labels each instrument with its quote currency and
omits an already-held pair. Each confirmed quantity remains an opening
observation. The shared Starting Point draft then shows the account,
instrument, quantity and a separate cost field for each positive position.
Costs use the instrument's currency. Gio asks for an explicit cost on each row,
preserving the original-cost review even when Wails offers a market quote as an
editable default.

The guarded application review freezes the complete map of exact costs and
previews the same household Origin and components used by Wails. A stale
quantity or review reloads the matching fields and keeps independent costs
where the holding and its currency are unchanged. Confirm creates a single
Origin with its opening components. Retry reads back that Origin.

| Check | Evidence |
| --- | --- |
| Wails and Gio comparison | Separate synthetic ledgers captured 2.5 USD units with an explicit zero cost and 4.125 EUR units at 12.34567891 per unit. Gio rejected omitted USD cost despite a known market quote. Reopened SQLite matched Wails quantity, cost, Origin date and timezone, with zero activities. |
| Compact and failure coverage | The Gio suite verified a distinct accessible input for each position, duplicate pair exclusion, a changed quantity invalidating review, and retained cost input. A lost reply plus failed read was resolved by read-only Retry. |
| Packaged Linux X11 | At 1160×820, Accounts saved 2.5 units of a USD security and 3.125 of a EUR security in the same brokerage. At 640×480, Tab moved between the costs. Cancel left both quantities and zero Origin rows. Confirmation and normal reopen retained one Origin with USD 0 and EUR 12.34567891 per unit, the USD 5.25 opening cash and zero activities. The USD 40 quote and the synthetic source hashes matched. |
| Checks | Full Gio tests and vet, the standard Linux package, shared application vet, focused application and Wails History and Holding Go suites, and nine Wails frontend reference tests passed. |
| Open scope | Managed and unavailable opening holdings, importing positions after History starts, and in-form instrument creation remain open. Interactive Wails, macOS, Wayland, VoiceOver and IME remain unverified. |

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

The experimental shell now supports Holdings and instrument detail, domestic
and foreign-currency settlement for ordinary buys and sales, cash dividends in
the default account currency, and multiple pre-History positions with native costs. Available Funds has a Today view
with bank reservation creation and release. Term deposits support read-only
terms and actual maturity receipt with linked History. Tasks covers term deposit
schedules, published pending investment checks, and supported open card
statement sessions.

Broader investment work still includes position imports after History starts,
trade quote calculators, instrument catalog maintenance and wider lifecycle coverage. Product gaps
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

**Insights → Wealth history → Trend** starts on the most recent closed day in
the History timezone. Use the shared date pickers to read 1 through 31 consecutive
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

#### Wealth History account captures

The Accounts lens reads the shared historical endpoint. Choose a single
closed day, compare two closed days, or compare a day with the current stored
valuation. Two-date comparisons preserve A and B order, including when
swapped. The capture controls precede the read time and endpoint summaries,
followed by the lens selector, changed-only filter and account rows. Account
rows expand to balance, cash and holding components with exact shared values,
missing currency pairs and available quote evidence. A failed read keeps the
previous capture and offers Retry. Trend keeps its loaded range when switching
lenses. Trend remains the initial lens while Allocation and Change are open.

| Check | Evidence |
| --- | --- |
| Shared/Wails | Independent read-only SQLite copies matched the complete historical account capture for dated and current comparisons, including the missing AUD/USD pair and the stored manual rate `1 USD = 1.2345 AUD`. Both services made zero provider calls. The Wails Historical Overview interaction suites passed 85 tests. |
| Gio | Frame checks covered date swap, changed-only filtering, expanding the account, both sides of the comparison, switching lenses with retained state and keyboard traversal into the Trend table. The full Gio module suite, including the independently merged chart component tests, and `go vet ./...` passed. |
| Linux X11 | The packaged app opened a verified synthetic profile and compared November 24 with current. Both sides showed USD 125.00 known net contribution, named the missing rate for Synthetic Australian cash and expanded Synthetic broker to its fund. At 700×480 the summaries stacked. Logical SQLite fingerprints matched before launch and after quit. |
| Open | Allocation, Change, full account detail and links to Ledger activity remain open, along with the Wails capture shortcuts and expanded evidence. Native Mac, VoiceOver, native IME and Wayland remain unverified. |

Shared chart components are available through #286. Return Trend is the first
page integration. Each chart keeps a keyboard-readable table and needs its
own page and platform evidence.

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
tracks the cross-page workflows. The Trend chart builds on the same filters
and projections.

| Check | Evidence |
| --- | --- |
| Wails workflow reference | The 30 tests in `ReturnAnalysisPage.test.tsx` passed. The Gio frame counterpart moved from September to August 2026. Both Trend and Contribution received August 1–31, the selected instrument and account filter, USD, native valuation and excluded cash. Returning to Calendar kept August and the effective range. |
| Shared Go comparison | Independent SQLite fixtures matched the Wails Go Trend amount and ratio, point count, and the first Contribution row's exact identity and amount. Both copies made zero provider requests. |
| Linux X11 | The packaged preview on the verified synthetic portfolio selected the named instrument and native valuation. Trend and Contribution showed USD 25 and +25% and retained November 1–24. Calendar retained the date range on return. At 700×480 the shared date fields wrapped to a second row, and Escape followed by Tab and Return moved from Scope to the Valuation picker. |
| Build and tests | The full Gio module suite, Gio vet, independent native Linux preview build and focused shared application/domain/Wails analysis suites passed. |

#### Return Trend chart and daily data

The display choice precedes the period amount, linked period rate and
coverage. A line chart and keyboard-scrollable daily table follow the
summary, with return sources below. The display switches among cumulative
amount, linked return rate and daily amount while retaining the effective
range and filters. Cumulative mode prepends a real zero on the civil day
before the range and includes that date in the chart viewport. Linked-rate
and daily-amount modes start on the first requested day. The chart and table
format the shared decimal amounts and rates. Geometry uses floats.

An explicit valuation gap is a dated, selectable point with its reason.
The cumulative service value can carry across that gap. Gio interrupts the
line and marks the specific missing observation in the table and readout so
it remains visible between valid points. The chart supports arrow keys, Home,
End, Return and Escape. Its compact version shortens the chart and keeps
the data table in the page scroll.

| Check | Evidence |
| --- | --- |
| Projection | The focused fixture starts August 1 with USD 5.25, has a missing valuation on August 2, and records −USD 2.50 on August 3. Cumulative mode draws a true zero on July 31 and shows USD 2.75 on August 3. Linked-rate and daily-amount modes have no synthetic baseline. Every mode keeps the dated gap. The shared line test checks that missing values split strokes and real zero stays in the line. |
| Wails and Gio | 66 Wails Return Analysis and insight tab tests passed. The focused root Go projection and Wails request tests passed. Gio frame tests exercised the normal shell, chart selection and clearing with the keyboard, linked mode, and the keyboard-readable table at 700×480 in dark appearance. |
| Build and tests | The full Gio module suite, `go vet ./...` and the packaged native Linux launcher passed on the combined tree. |
| Linux X11 | The packaged preview on a verified synthetic portfolio showed the October 31 zero and November 2 USD 5.25. Keyboard selection of November 4 showed Missing valuation and Unavailable. Daily mode selected November 6 at −USD 2.50. Linked-rate mode started November 1 with no cumulative baseline. At 1180×1000 the gap interrupted the chart; after reopening the synthetic fixture in dark appearance, the plot and daily table remained readable at 700×480. |
| Open | Asset Changes and Wealth History chart adapters remain separate milestones. Native Mac, Wayland and assistive technology acceptance remain open. |

#### Contribution ranking chart and detail

Contribution follows the Wails order of summary, direction and count, main
ranking, then searchable details. The signed bars share a centered zero axis
and reuse the same exact magnitude ordering and limit as the ranking table.
The chart has one tab stop and supports arrows and Enter. Gio keeps a separate
keyboard-readable list of the ranked rows alongside the independent Details
search. It also keeps the established detail and Ledger activity actions.

Selecting a bar opens the same Wails-backed Contribution detail as selecting
the corresponding text row. Closing the detail restores its trigger. Opening
Ledger activity carries the supported account, instrument, civil dates and
activity kinds. Back restores the chart focus and selected contribution, and
Enter can reopen the same detail.

| Check | Evidence |
| --- | --- |
| Projection and interaction | The signed fixture preserves USD 5.25, −USD 2.50 and an actual zero in magnitude order. A row without an amount stays out of the ranking. The Wails-backed Gio interaction checks the chart and text-row detail, the effective Ledger activity filters, and Back and Escape focus. |
| Linux X11 | The packaged app displayed both signed funds. Selecting Loss fund opened −USD 2.50. Escape and Enter restored the detail. Ledger activity retained Sample broker, Loss fund and November 1–24. Back plus Enter reopened Loss fund. The chart and amounts remained visible at 700×480 after scrolling. |
| Build and tests | Full Gio tests and vet, the packaged Linux launcher, focused shared Contribution Go tests and 39 Wails Contribution and Insight Tabs tests passed. Native Mac, Wayland and VoiceOver remain open. |

#### Asset Changes and the Return day link

**Insights → Asset changes** opens with Asset trend, Change drivers and
Categories tabs followed by the same filter card as Return Analysis. The
first trend view uses the daily net worth projection. The user can switch
among net worth, assets, liabilities or net change and day, week or month.
Change drivers shows the beginning and ending values, the net change, rate
and the shared driver groups. Categories shows the chosen shared category
total and attributed rows. The trend and category tables support keyboard
scrolling. The layout follows the Wails section order. The following
checkpoints cover Change Drivers and Asset Trend. Category details remain open.

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

#### Change Drivers waterfall and breakdown

Change drivers shows the value summary, native waterfall, and grouped
attribution in Wails order. The adapter uses the shared beginning, signed
drivers and ending. It accumulates levels with decimal arithmetic and warns
when exact inputs disagree at Wails' comparison precision. The reported
ending remains visible. Missing beginnings and unavailable driver values
remain missing.

A chart driver or grouped row opens its breakdown with the effective civil
dates, exact total and the shared account and instrument projections. Failed
reads can be retried. Escape restores focus to the chart or row. Residual
details can open Ledger activity at the affected day and dimensions. The
Return Analysis link and the residual link's end-to-end acceptance remain
open.

| Check | Evidence |
| --- | --- |
| Projection and interaction | Gio checks signed steps, the missing opening, the reported ending and the ties-to-even discrepancy threshold. A Calendar day link used the Wails-backed Price change driver read for the selected day. A failed first read retried in place, matched the breakdown, and restored chart focus on Escape. |
| Linux X11 | The packaged app displayed USD 300.00 beginning, USD 2.75 Price change, and USD 302.75 ending. The chart opened a sheet showing Gain fund USD 5.25, Loss fund −USD 2.50 and Sample broker USD 2.75. Escape and Enter reopened that sheet. All three steps and their labels remained readable at 700×480. |
| Build and tests | Full Gio tests and vet, the Linux launcher, focused shared Go Asset Changes and driver tests, and all 43 Wails Asset Changes and Return Analysis reference tests passed. Native Mac, Wayland and VoiceOver remain open. |

#### Asset Trend chart and period coverage

Asset Trend offers all fifteen shared Wails metrics and day, week and month
frequencies. Level summaries lead the metric controls and chart. The chart
and keyboard-readable table keep the full selected range. Week keys begin
Monday and month keys begin on the first, with their first plotted anchors
clamped to the selected range. Rate and flow summaries follow the detailed
series. Liability levels use the Wails display sign.

Gio exposes missing periods as selectable dated markers and interruptions in
the line. The markers share the keyboard order of available observations.
The table and readout keep a known subtotal, provisional status and available
quote provenance when the shared valuation supplies them. These states keep
missing totals outside the chart geometry. The dated link to Data health
remains open.

| Check | Evidence |
| --- | --- |
| Projection and interaction | Tests cover clamped week and month boundaries, an absent week, an unplotted known subtotal, provisional quote evidence, negative debt levels, and a missing or exact rate. The Calendar-linked day moved to the Return rate metric with the keyboard and matched the Wails Go value at +25% and 1/1 rated days. Return Trend's existing viewport, gap and focus tests pass through the shared chart frame. |
| Linux X11 | The packaged app selected November 4 in the net worth chart and displayed an unavailable total and USD 105.25 known net contribution. Weekly retained the chosen dates, then Return rate showed the 3/7 uncovered week and later 0% weeks with 7/7 coverage. The compact table remained readable at 700×480. |
| Build and tests | Full Gio tests and vet, the packaged Linux launcher, focused shared Go trend tests and 20 Wails Asset Trend and Asset Changes tests passed. Native Mac and Wayland remain open. |

#### Wealth History Allocation and category drill-down

Wealth History defaults to Allocation at the prior month end within the
recorded History range. As in Wails, if History started later, the first
closed History date is used. Capture controls and endpoint totals precede
the lens tabs and the class or currency breakdown. The plot uses a donut
for positive allocations with nonnegative shares, signed bars for mixed
amounts and text for all-zero or unavailable values. The plot and list
display the original formatted amounts and shares, with a valued-only
notice when some included amounts are missing.

Selecting a category opens the matching Accounts component rows while
retaining the capture. The filter can be cleared in Accounts, or Return to
allocation restores the source focus and page position. A/B and current
comparisons use the shared category amounts, known subtotals, shares and
supported changes. Liabilities use signed levels and the opposite sign for
increases in debt. Partial, absent and excluded states remain distinguishable.
The comparison uses paired value cards that wrap as the window narrows.
The paired chart and Change lens are covered in the following milestone.

| Check | Evidence |
| --- | --- |
| Wails Go and Gio controls | Independent read-only copies matched the Wails default capture and date comparison in the History timezone. The test verifies that selecting AUD shows Local cash and omits Foreign cash and debt, that the original scroll and keyboard focus recover, and that partial FX and signed liabilities are distinguishable in comparison. The chart projection keeps negative bars and missing geometry separate. |
| Linux X11 | The packaged app used the standard Gio directory under a fake config root. Allocation showed AUD 214.79, with AUD 123.45 cash at 57.47% and AUD 91.34 stocks at 42.53%. Selecting the stock arc opened Pine Brokerage. Return to allocation and Enter reopened the same selection. The chart and values stayed readable at 700×480 after page scroll. Both synthetic Wails source file hashes matched after quit. |
| Build and tests | Full Gio tests and vet, the packaged Linux launcher, focused shared historical Go tests and 92 Wails Historical Overview frontend tests passed. Native macOS, Wayland, VoiceOver and IME remain open. |

#### Wealth History Trend and date actions

The Trend chart uses the bounded shared historical series for net worth,
assets and signed liabilities. It keeps the requested civil date viewport.
Missing full values interrupt the line, with any known subtotal and named
missing inputs available in the readout and daily table. The chart follows
the service's metric-specific connection flags and shows provisional
observations with dashed edges.

Select a date from the chart or the table to set range endpoint A or B, or
open Accounts for that day's capture. Gio groups the actions after the chart
readout so they stay available to chart and keyboard users while keeping the
compact daily table focused on values and evidence. The captured range
and prior retry behavior remain intact.

| Check | Evidence |
| --- | --- |
| Projection and workflow | The adapter test verifies the explicit viewport, signed debt, unavailable valuation with a known subtotal, basis change and provisional boundary. A keyboard selection opened the exact Wails Go capture on independent read-only databases. The original bounded-range, keyboard table and retry regressions still pass. |
| Linux X11 | The packaged app at the default synthetic Gio path opened November 1–24 and selected November 4 by keyboard. The readout showed unavailable net worth, USD 105.25 known net contribution and the named missing AUD exchange rate. Opening Accounts used November 4 and retained the same evidence. Selecting November 4 as B loaded November 1–4, with the selected date, gap and known value readable at 700×480. The synthetic Wails source database and settings hashes matched after quit. |
| Combined tree | The branch retains the Sankey component, localization and Component Preview from #288. Those components are available for separate business-page wiring. Verified full Gio tests and vet, the packaged Linux launcher and 91 Wails Historical Trend and Overview frontend tests. Native Mac, Wayland, VoiceOver and IME remain open. |

#### Paired Wealth Allocation and Change

The pair chart places adjacent A and B rows on a shared signed scale, with
the dates and currency in the readout. A row links to the same category
filter as its detail card. Partial or excluded cells have text states and
absent geometry. This layout lets both amounts and labels remain readable
when the rows wrap in narrow windows.

Change ranks comparable account balances by absolute change and separates
missing, excluded or otherwise incomparable accounts for individual review.
The list includes the full comparable set alongside the initial top-ten chart.
The account sheet exposes the two endpoint values, source dates, child
components and the scoped Ledger activity link. Back returns to the same
sheet and action. Closing the sheet restores its chart or row control.
The reconciliation summary and evidence use the shared endpoint bridge,
including exact residuals, component principal, known records and reasons.

| Check | Evidence |
| --- | --- |
| Gio and shared Go | Read-only Wails comparison matched both endpoint results. Tests exercise the paired chart’s keyboard handoff, focus return, missing FX, signed debt, scoped Ledger activity and navigation Back. |
| Packaged Linux X11 | November 1–24 showed USD 12.75 net-worth change, USD 2.75 supported, and USD 10.00 residual. The broker had USD 100.00 and USD 102.75, and the second account remained separately inspectable. Ledger activity retained the broker and both dates. Back and Escape restored the detail and chart. Paired Allocation showed both stock amounts at 1220×1050 and 700×480, with the absent A cash value explicit. The synthetic Wails source hashes matched after quit. |
| Tests | Full Gio suite, Gio vet, focused shared historical Go tests and 101 Wails frontend reference tests passed. Native Mac, VoiceOver and Wayland remain open. |

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
Forward preserve the tab, search and scroll.
[Manual price and FX entry](#manual-instrument-price-and-fx-entry) and
[explicit provider refresh](#explicit-current-refresh-and-historical-sync)
follow this read milestone. Quote history and inline source configuration
remain open. Reload saved data refreshes the local view.

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
originating button. [Focused snapshot inspection and repair](#focused-snapshot-inspection-and-repair)
adds snapshot scope and adjacent-day grouping. [Manual quote actions](#manual-instrument-price-and-fx-entry)
follow these milestones.

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
and joined on quit. Detailed per-target results remain open.

| Check | Evidence |
| --- | --- |
| Wails comparison | The Gio plan matched the Wails Go plan. All 24 frontend checks across Data Health, Snapshot Repair and Market Data Sync Bar passed. Focused shared Go tests covered repair preview, attach/conflict, cancellation and historical gap clearance. |
| Gio interaction | Ran the repair test three times. The first confirmed request blocked at a deterministic provider and was cancelled. The retry saved one exact provider rate of 140.1234. The fresh Gio report matched the Wails report and kept the historical gap visible after the latest quote was saved. The pre-History test verified the repair prerequisite. |
| Linux X11 | The packaged launcher opened the plan on a marked household and displayed one estimated provider request at 1100×900 and 700×480. Escape and Return restored the plan without storing a new FX quote. A separate native build with only a deterministic provider substituted at build time confirmed the plan, cancelled the first request, then saved the exact 140.1234 rate on a second confirmation. It showed Repair completed and the remaining gaps, and the isolated fixture was verified after shutdown. |
| Build and tests | The full Gio suite, Gio vet and vanilla Linux package build passed. Live providers, Wails GUI, native macOS, VoiceOver and IME remain unverified for this workflow. |

#### Focused snapshot inspection and repair

From Overview coverage, **Inspect snapshot finding** narrows Data Health to
the same component and dates. Adjacent incomplete days group only when the
component and diagnosis match. A valued day keeps the surrounding findings
separate. The focused report shows blocking inputs alongside collapsed
snapshot evidence. Scan, filtering and preview read saved data.

For an executable finding, **Preview snapshot repair** shows the exact closed
dates inside the current focus. **Confirm and rebuild** uses the same shared
historical snapshot operation as Wails. It processes up to 31 dates per call.
**Stop after current batch** pauses new work and keeps completed revisions.
Retry starts at the failed batch. Recheck refreshes the report on completion,
failure and pause. Close returns focus to the preview control when that
finding is still actionable, or to Recheck when it has cleared. The existing
global Repair all keeps its broader scope. The found rows are grouped when a
report or focus changes, with no benchmark run for this milestone.

| Check | Evidence |
| --- | --- |
| Gio and shared Go | Scoped read compared the report to the Wails adapter. Opening, inspecting, previewing and dismissing changed no synthetic snapshot rows. Confirmation appended one superseding USD 125.25 revision for November 22 and preserved the other dates. A deterministic 64-day sequence verified a stop after the first 31 days, failure with two committed revisions in the next batch, retry from that batch and completion of the remaining dates. No provider requests. |
| Wails reference | All 19 tests across Data Health, Snapshot Repair and the Overview net-worth card passed. Focused shared application, History and Market Data adapter snapshot/health/repair tests passed. |
| Packaged Linux X11 | At 1260×1050, Overview's November 1–24 coverage link opened the report and inspected the November 4 Synthetic Australian cash finding. Back and Return reopened that finding. The blocked flow left snapshot rows unchanged. On a second synthetic profile at 640×480, Escape and Return restored the October 12 repair preview without writing. Confirmation added one USD 125.25 revision for October 12. October 13 and the authoritative account, activity, holding, quote and History origin rows were identical. The focused recheck cleared October 12. Both source databases and settings hashes matched. |
| Build and tests | Full Gio tests, Gio vet and the native Linux package passed. |
| Follow-up | [Manual instrument price and FX entry](#manual-instrument-price-and-fx-entry) covers linked Data Health manual prerequisites. Detailed sync evidence and Wails GUI, native macOS, Wayland, VoiceOver and IME verification remain open. |

#### Manual instrument price and FX entry

In **Manage → Market data**, an eligible instrument's **Set price** action
opens a right-hand form for that instrument. Gio places this action on the
instrument row while the broader Wails instrument editor remains open work.
**Add manual FX quote** is the FX tab's page action, including an empty FX
list. A Data Health manual entry starts with the finding's instrument or
ordered currency pair and its date within the selected focus. Unfocused
entry leaves the date blank to use the current time. Date-only input uses
the saved History timezone, or UTC before History.

Saving uses the same application operations as Wails. The domain validates
the canonical decimal, including a real zero instrument price and a positive
FX rate. Validation keeps the draft. Escape or Close leaves without a write.
While a save is pending, the form blocks another submission and waits for the
result. If a rate committed but subsequent metal repricing failed, Gio shows
the saved observation and the follow-up failure. The existing Wails Go
adapter keeps its current error contract.

After save, the active view rereads stored evidence and restores focus to
the originating control when present. Quote entry leaves the configured
source and unrelated diagnostics in place. The receipt identifies the saved
observation separately from the currently selected quote.

| Check | Evidence |
| --- | --- |
| Shared Go and Wails | Focused application, quote and Market Data adapter tests passed. All 83 Wails Market Data, Instrument Management, Data Health and Snapshot Repair frontend tests passed. A synthetic repository failure after the FX insert verified that the committed observation remains readable by its ID and that the legacy API still returns its original error result. |
| Gio regression | An unquoted instrument and a focused JPY/USD prerequisite opened with the expected defaults. Escape and invalid attempts appended nothing. A stored USD 12.34567891 price and 1 JPY = 0.006789012345 USD reopened through the Wails quote adapter with their Asia/Singapore dates. Their exact shared known subtotal was USD 31.480370165. The SGD prerequisite and existing snapshot rows remained unchanged. A scripted failure verified keyboard Retry, disabled overlapping submissions and the committed-rate warning. The two Gio manual tests and the Ledger refresh regression passed 15 consecutive runs. The Ledger fixture exercises the live valuation read independently from background trend publication and keeps its five version assertions. |
| Packaged Linux X11 | At 640×480, Escape cancelled a typed price. The form retained the rejected nine-decimal text, then saved USD 12.34567891 for November 24 selected by keyboard. An injected SQLite insert failure retained 1 JPY = 0.006789012345 USD. A double-click on Retry appended exactly one manual observation. On a normal reopen, both quote values were readable in Market Data. Overview showed USD 31.48 known and the missing SGD rate. Authoritative account, activity and holding rows and snapshot rows matched before and after entry. The untouched source database and settings retained their hashes. |
| Checks and remaining work | Full Gio tests and vet and the packaged Linux launcher passed. Native Mac, Wayland, VoiceOver, IME and interactive Wails remain open. Instrument and source management and quote history remain open. The Overview home action placement is separate. No benchmark was run. |

The focused Data Health follow-up preserves the finding's own date when it
is undated or lies outside the current coverage scope. An overlapping focus
can narrow a dated finding. Escape and the toolbar Back area dismiss the
form and restore the same finding and control. A counted Gio test verified
both closures performed zero scans, and a successful save performed one.
Packaged Linux at 640×480 verified the focused price and USD/JPY forms from
Overview, Escape, Back and keyboard reopening on a copy with zero stored
quotes. Both quote tables remained empty and the source hashes matched.
All 47 Wails Data Health, Snapshot Repair and Market Data reference tests
passed for this checkpoint.

#### Explicit current refresh and historical sync

Market data shows **Update latest data** above the Instruments and FX rates
tabs. **More actions** contains **Sync and repair**, **Force refresh all**
and the shared Market data settings shortcut. Provider-backed instrument
and FX rows can update their own quote or preview scoped historical repair.
The row keeps the saved source, and refresh uses the configured provider.
Gio presents instrument sync with the same preview as FX and global repair
so the target, ranges and estimated work are visible before confirmation.
The Wails instrument editor starts its row sync directly.

The global preview can be recalculated with **Force historical recheck**.
Preview is local and only **Start sync** starts the shared historical job.
Current refresh uses the shared per-request cancellation and retries the
same scope. Historical retry previews the same scope again. Both paths
show progress, safe per-target results and remaining prerequisites. A
terminal operation rereads the active saved quote or health evidence.
Historical snapshot progress shows the number rebuilt and the initial
estimate separately because newly fetched inputs can dirty additional days.

| Check | Evidence |
| --- | --- |
| Gio and shared Go | A real SQLite application service with deterministic providers covered current cancellation without a quote, overlapping-operation fencing, retry with one success and one failure, a forced refresh, and readback of exact rates and provider provenance. Instrument-only refresh stored USD 17.12345678 without making an FX request. The scoped instrument and JPY/USD previews matched the Wails Go adapter. Escape and keyboard reopening restored the same scoped preview, global forced preview recalculated locally, and cancelling and retrying JPY/USD committed a dated 139.123456 rate while leaving SGD unresolved. Settings Back restored the FX tab and control. |
| Wails reference | The 47 existing frontend tests across Market Data, the sync bar, Data Health and Snapshot Repair and the focused shared application and Wails adapter refresh, sync and health tests passed. |
| Packaged Linux X11 | At 1260×1050, cancelling and retrying current refresh displayed one updated and one failed target. JPY/USD showed 140.4321 with Frankfurter provenance and SGD/USD stayed missing. At 640×480, Escape dismissed the JPY/USD historical preview and Return reopened it. After confirming, cancelling and retrying the same historical job, the copy had 11 dated JPY/USD observations at 139.123456 and the original latest observation. The shared job reported unresolved findings and four rebuilt snapshot days versus an initial estimate of zero. Source database and settings hashes matched. This launch substituted a deterministic provider at build time. |
| Checks and remaining work | Full Gio tests, vet and the standard Linux package build passed. [Quote history](#selected-instrument-and-fx-quote-history) follows this checkpoint. Inline instrument and FX source management, Agent evidence and provider repair launched directly from a Data Health finding remain tracked in the parity checklist. Wails GUI, macOS, Wayland, VoiceOver and IME remain open. No benchmark was run. |

#### Selected instrument and FX quote history

**View quote history** opens a right-hand sheet for the selected instrument or
currency pair, including objects without saved observations. The Wails quote
series APIs supply the canonical values, filtered observations and chart
points. The date shortcuts and adjacent windows use the recorded History
timezone, with UTC before History to match the shared query's civil-date
boundaries. Quote history may predate the household's Starting Point.
**Show all dates** is available when the selected source has older or newer
local observations outside an empty window.

The sheet shows the configured source, the selected current quote as stored,
and the historical evidence for All, Manual, Provider or Agent. Swapping an
FX pair changes only the historical query direction. Current quote text
keeps the direction of its stored evidence. Historical closes, NAV, manual
observations and daily references have separate legend entries. Latest and
intraday observations are dots. The saved-observations count describes the
dates represented in the selected range. Data Health is the source for
coverage findings. The keyboard-readable table retains all matching
observations, including other currencies and same-time facts omitted from
the chart. It shows source, effective date or timestamp basis, and delay
when present. Gio places the table jump near the filters for compact
windows and offers a copyable CoinGecko attribution link.

From a quote history sheet, **View data health** carries the selected
instrument or pair and date window. Back restores the quote sheet, source,
direction, dates and its list position. A finding's **View quote history**
opens its object and dated range directly. Close or Escape returns to that
finding and its control without a new scan.

| Check | Evidence |
| --- | --- |
| Gio and shared Go | A SQLite fixture matched the Wails instrument and reversed FX series, including USD 12.34567891, a delayed flag, and the exact reciprocal 0.142857142857. It exercised empty versus outside-range states, local failure and retry, Home and Escape, focused Data Health and Back. A projection test kept date-only NAV apart from timed provider observations, isolated intraday marks, and kept a nonmatching currency off the plot. Reads left the ledger version unchanged. |
| Wails reference | All 40 tests across Quote History, Market Data and Data Health passed. Focused shared application and Wails quote-adapter tests passed. |
| Packaged Linux X11 | At 1260×1050 the default instrument window was empty and the selected quote showed USD 13.98765432. Show all dates revealed two observations. At 640×480 View data table reached the exact delayed USD 12.34567891 entry. Escape and Return reopened the instrument. FX history kept the stored USD/JPY reference while the historical direction switched. The empty SGD/USD sheet carried scope to Data Health, opened the November 21–24 missing-rate finding, and restored the selected sheet on Back. Both source hashes matched. The copy's instrument quotes, FX quotes and FX preferences matched the synthetic source rows. The packaged test launch substituted a deterministic provider. |
| Checks and remaining work | Full Gio tests, vet and the standard Linux package build passed. [FX sources and focused quote repair](#fx-sources-and-focused-provider-repair) follow this checkpoint. Interactive Wails, macOS, Wayland, VoiceOver and IME remain open. No benchmark was run. |

#### FX sources and focused provider repair

**Change FX source** opens the selected pair's Manual, Provider and Agent
settings. The same selector covers pairs that have no saved preference.
Gio uses an explicit Save for its compact sheet, with Close or Escape
before submission. A pending write disables duplicate submission and
dismissal. Failure retains the selected source for retry. The shared write
preserves recorded quotes. As in Wails, saving Provider follows with the
shared missing-or-stale refresh. The sheet states this consequence before
Save. Manual and Agent changes reread local data.

Executable provider findings in Data Health open the existing historical
repair preview for the exact instrument or pair. Plans follow the shared
request scope and may include opening-anchor dates outside the displayed
finding. Dismissing a preview restores the finding and starts no job.
Confirmation starts the existing cancellable job. The summary, per-target
details and scoped retry stay available on Data Health, with a local report
recheck after the job terminates. A scoped finding can clear even when the
shared job reports other unresolved coverage. Snapshot-date repairs still
use their existing selected-date confirmation.

| Check | Evidence |
| --- | --- |
| Gio and shared Go | Tests matched the JPY/USD plan with the Wails adapter, verified zero new scans or requests on preview dismissal, and verified one recheck per terminal job. A cancelled request retried the same scope and committed quotes only for JPY/USD. A separate FX source test covered compact dismissal, overlapping submission, a local storage error with retained choice, a successful Agent save without refresh, and a Provider save that read back 140.4321. The other pair kept its source. |
| Wails reference | All 48 Quote History, Market Data, sync bar, Data Health and Snapshot Repair frontend tests passed. Focused shared Go and Wails Market Data adapter tests passed. |
| Packaged Linux X11 | Overview opened the JPY/USD finding. At 640×480 Escape and Return reopened its preview. Confirming, cancelling and retrying committed 11 dated JPY/USD observations at 139.123456 and cleared the focused finding while the job retained other unresolved coverage. At 640×480, a Manual draft closed with JPY/USD still on Provider. An SGD/USD Agent selection saved at 1260×1050 and reopened on a new launch. SQLite contained only the selected JPY/USD historical quotes and the two expected source preferences. Source database and settings hashes matched. The packaged test substituted a deterministic provider. |
| Checks and remaining work | Full Gio tests, vet and the standard Linux package build passed. [The instrument source editor](#instrument-quote-source-and-provider-binding) follows this checkpoint. Interactive Wails, macOS, Wayland, VoiceOver and IME remain open. No benchmark was run. |

#### Instrument quote source and provider binding

**Edit quote source** opens from a Market data instrument row or a Data
Health binding prerequisite. The sheet shows the configured source and the
instrument's currency. Manual, Provider and Agent are explicit choices.
Provider exposes the available provider, its symbol and the market and
country routing fields. Metal templates keep their fixed futures binding.

The lookup uses the same application API as Wails. It is available for
stocks and ETFs through Yahoo Finance and for cryptocurrencies through
CoinGecko. Gio uses a **Look up instrument** button before sending the typed
query. A result is identified by symbol, name, venue and currency. The
source editor applies the provider binding and routing fields and keeps the
existing catalog name, type, currency and other metadata. Results with a
different reported type or currency can be inspected with their mismatch
label. The selected source can also be configured with a known provider
symbol without a lookup.

Save uses the shared instrument writer under the ledger lock. Missing
Provider fields show validation before submission. Failed lookups and saves
can be retried, and failed saves retain the draft. A save rereads the active
view and restores focus to the originating row or finding when it remains
visible. Changing to Manual or Agent preserves the saved provider identity
and existing quote observations. Lookup and save are separate actions.

| Check | Evidence |
| --- | --- |
| Gio and shared Go | The compact interaction fixture typed into the lookup editor, verified the query stayed local until submission, retried a failed lookup, disabled a mismatched result, and used the compatible result. The first save failed with no write and the draft remained for retry. Wails readback matched Yahoo Finance, DEMO, NASDAQ and US. The USD 12.34567891 manual quote, name, alias, ticker, currency, note and sort order were preserved. A Data Health fixture verified dismissal restored the same finding without rescanning and Save rechecked that same scope. |
| Wails reference | All 82 Instrument Management, Market Data and Data Health frontend tests passed. Focused shared application and Wails instrument and Market Data adapter tests passed. |
| Packaged Linux X11 | At 1260×1050 saving a blank Provider draft showed the binding validation. At 640×480 actual keyboard text reached the lookup, an injected provider error retried, and the USD result was selectable beside the disabled EUR result. The native test caught that the sheet's shortcut listener was consuming editor events. The listener now reads only its shortcut keys. Save restored the originating row's focus. Return reopened the same instrument, and quit and reopen showed Yahoo Finance and DEMO. SQLite confirmed NASDAQ and US, the original USD 12.34567891 manual quote and untouched catalog metadata. Source database and settings hashes matched. The packaged test used deterministic providers. |
| Checks and remaining work | Full Gio tests, vet and the standard Linux package build passed. [Local Agent evidence and closed dates](#local-agent-coverage-evidence-and-closed-dates) follow this checkpoint. Interactive Wails, macOS, Wayland, VoiceOver and native IME remain open. No benchmark was run. |

#### Local Agent coverage evidence and closed dates

**Agent evidence** opens the saved evidence for a Market data instrument or
FX pair. An Agent prerequisite in Data Health opens the same view initially
limited to the finding's dates. **Show all dates** exposes the selected
target's other evidence. Coverage findings come from the shared Data Health
scan. The evidence list uses the local Agent audit, showing exact stored
amounts, civil dates, source titles and URLs, closures and superseded or
withdrawn facts. FX evidence keeps the stored direction. A source URL is
displayed as provenance.

An active instrument can record a closed date using the same command as the
Wails instrument editor. The confirmation stores a dated closure fact and
preserves the saved quotes. A failed response retains the date and operation ID for
retry. The view rereads stored evidence after success, reports a pending
snapshot recheck separately when the shared result requires it, and rechecks
the originating Data Health scope on save. Close or Escape returns to the
same row or finding. These workflows use the local ledger and its saved
Agent evidence.

| Check | Evidence |
| --- | --- |
| Gio and shared Go | A compact Data Health fixture switched from a dated finding to all evidence while isolating the scope to the selected instrument. It retained the exact USD 12.34567891 NAV and its source. A simulated lost acknowledgement retried the same Wails operation ID and read back one closure with the original quote history intact. A separate FX projection preserved the stored direction and the withdrawal of the selected pair. |
| Wails reference | All 60 Instrument Management, Quote History and Data Health frontend tests passed. Focused shared application and Wails quote adapter Agent and closure tests passed. |
| Packaged Linux X11 | At 1260×1050 the synthetic fund showed its USD 17.12345678 NAV and source. At 640×480 the date picker saved November 24 and showed the closed-date receipt. Escape and Return reopened the selected row. Quit and reopen showed the closure alongside the original NAV. SQLite retained both original price rows and one added closure receipt. Source database and settings hashes matched. The packaged test used deterministic providers. |
| Checks and remaining work | Full Gio tests, vet and the standard Linux package build passed. This view loads the shared household audit only when opened and filters to the selected target. No benchmark was run. Wails GUI, macOS, Wayland, VoiceOver and native IME remain open. |

#### Follow-up order across Stages 4 to 6

Continue Stage 5 with position imports after History starts and broader
Holdings actions. Stage 4 still has additional Starting
Point handoffs, value updates and liability-specific amendments. Put additional Agent audit filters,
large-history paging, and possible local correction and withdrawal authoring
after these account and investment workflows. The broader catalog editor
remains part of investment parity. External enrollment and permissions stay
paused with Stage 7.

#### Additional analysis filters and effective dates

Return Analysis and Asset Changes show scope and the effective dates in
the primary row. More filters shows the valuation and cash options with
account, currency, asset class, member and instrument. A summary shows the
active choices while the controls are collapsed. The compact header uses
one summary row when it fits. The bulk clear action preserves the date range,
scope, valuation and cash setting. Editing one date keeps the other date and
respects the recorded History period. The Return day link carries the exact
filters into Asset Changes. Contribution detail explains that Ledger activity
cannot apply an additional member filter.

| Check | Evidence |
| --- | --- |
| Wails comparison | All 90 tests across Analysis Filter Bar, Return Analysis, Asset Changes and Insight Tabs passed. Focused shared Go query and analysis tests passed. On independent synthetic ledgers, Gio and the Wails Go adapter matched the contribution with USD, Stocks, the member and the named instrument selected. |
| Gio interaction | August 1–31 stayed effective across Return tabs. Calendar day → Asset Changes retained the additional instrument, member, asset class and earlier account, currency, native valuation and cash choices. Back restored focus and the originating filters. Clearing the additional set retained other choices and focused More filters. |
| Linux X11 | The packaged preview selected Stocks, Sample owner and Synthetic fund for November 1–24 and retained them in Contribution and at 700×480. The contribution remained USD 25 and +25%. Its detail showed separate member and currency or asset-class notices before Ledger activity. |
| Build and tests | The full Gio suite, Gio vet and native Linux package build passed. Interactive Wails, macOS, VoiceOver and IME remain open. |

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

#### Contribution ranking and search

Contribution shows the shared total, positive and negative summaries, then
Main contributions and All contribution details. The ranking uses the exact
absolute amounts and supports positive and negative filters and 10, 20 or
all rows. Gio uses keyboard-scrollable rows while chart integration is
paused. Count buttons use a separate wrapping row so the choices stay visible at compact sizes.
The details search is independent of the ranking's direction and limit.
Total Return can sort details by the shared return rate, and supported
metrics can group by asset class. Metric changes reset incompatible group
or sort choices. The result controls and link focus survive the History
round trip.

| Check | Evidence |
| --- | --- |
| Wails comparison | All 36 Insight Tabs frontend tests and focused shared Go Contribution tests passed. Gio preserves the shared amount and rate, and the Wails direction, limit and absolute-value order. The ranking test distinguishes magnitude 1.00000001 from 1, breaks ties by key, and excludes missing amounts without changing the details order. |
| Gio interaction | With one positive USD 25 item, Negative reduced the ranking to zero. Searching independently hid and recovered the item in Details. Show 20 and the search survived Contribution → Ledger activity → Back. |
| Linux X11 | The packaged preview showed zero of one in the negative ranking, found Synthetic fund in Details, and opened the same November 1–24 Ledger activity context. Back and Return reopened that detail. At 700×480 the search and result remained reachable after scrolling. |
| Build and tests | The full Gio module suite, Gio vet and native Linux package build passed. Interactive Wails, macOS, VoiceOver and IME remain open. |

#### Analysis date shortcuts and individual filter clearing

Return Analysis and Asset Changes share the day, week, month, quarter,
six-month, year and all-history shortcuts above More filters. Presets end
on the last closed date in the History timezone. The previous and next
controls step to adjacent inclusive ranges and respect the History start
and last closed date. Additional filter chips remove one choice at a time
and return focus to More filters. Dates, scope, valuation and other
choices remain selected.

| Check | Evidence |
| --- | --- |
| Wails reference | The 41 Analysis Filter Bar and Return Analysis tests and two date-range tests passed. Gio frames exercised a historical week and its adjacent periods. The date regression also covers leap-day month ends, custom adjacent windows and clipping at History start. |
| Gio interaction | In the filtered August Calendar and Asset Changes round trip, removing Stocks preserved the other four additional filters and dated selection. Clearing the remainder preserved scope, valuation and cash. |
| Linux X11 | Last week selected November 18–24. Previous selected November 11–17. Removing the Sample owner chip preserved those dates. Asset Changes showed November 18–24 at 700×480 after choosing the same shortcut. |
| Build and tests | The full Gio module suite, Gio vet and native Linux package build passed. Interactive Wails, macOS, VoiceOver and IME remain open. |

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
