# Gio Investment Assets parity handoff · 2026-11-27

## Scope and evidence

The source comparison is the Library PDF *Nestworth-投资资产-Gio-Wails-差异核对.pdf* (25 pages; 33 checklist items; 20 screenshot groups). I read its extracted text but could not materialize or inspect its embedded screenshots. References below distinguish the PDF's baseline notes from Saul's newer independent X11 observations; no raw screenshots from that separate run were available to this executor.

That report used main `ef09d4cc` and isolated synthetic SQLite ledgers, with Gio on Linux X11 and Wails’ production frontend plus Go backend in Chromium at 1280×800 and 900×600. This executor launched the Wails production frontend with the real Go backend in Chromium against a separate synthetic SQLite ledger. Its usable viewport captures were 1200×855; this was not the requested paired 1280×800 / 900×600 run. The Gio native window still failed to render in this executor because of the X11/Mesa shared-memory error. Saul's independent Linux X11 results remain separately attributed below.

The implementation branch began from the requested PR #299 head `188b4413dabf251df61050d6e670a10471333e0b` and the then-current #299 lineage at `75b8069dac011a5d30ac26720702048d58baf3d8`. PR #299 was later squash-merged to `main` as `bdadfcb72a24b686f34a593bf1a1f98f80b04109`; PR #300 auto-retargeted to `main`. I merged that `main` into the existing task branch with a regular merge commit (`80e00c47`) and kept the newer account files from `main`. The diff against `main` is 15 files, all limited to the investment-asset increment. The panic was present at the requested `188b4413` head.

Feature commit: `9ba8c88540eba7d52d7bf5745c812e1934da141e`. Scroll preservation and its regression: `aa95a410793d5e7e6eb6599982eab6fadc84aae2`. Test assertion fixes: `c4ea8bea541619939694b453ea17c83f82fb2738`, `90de3c95067aa6a6999c0f45558eb8d313bdfc03`, and `dcfbf8ca7e550af312163100816b9cf3e67d3393`. Import-sheet focus and receipt fixes: `198eca5179adad09b136aa0fb42a3c8d03e25f6c`, `72b169abbcc753f19ba8645924a2c34e8671a9c0`, and `4cfafbde588d68c073406265a3b98617b4ec2035`. All implementation/test commits are by Walter Wang, `wangweichen1994@gmail.com`. Mainline merge: `80e00c47e76d2dac31806b77bf6fc9023b843beb`.

Existing PR: [#300](https://github.com/MnemosyneLab/NestworthGo/pull/300), open against `main` at `bdadfcb72a24b686f34a593bf1a1f98f80b04109`. The merged branch preserves the latest account work from `main`, and its platform PR diff is investment-only. Saul independently ran Linux X11 acceptance; no Slack or reviewer handoff is needed.

## Latest executor pass (2026-11-27)

The Wails comparison used its production frontend and actual Go server mode in Chromium, not a native Wails WebView. It used the synthetic `Nestworth Synthetic` household, `Harbor Brokerage` and `Second Brokerage`, and synthetic `QA-*` instruments only. The screenshot files capture the 1212×886 window (the app viewport was approximately 1200×855). The requested 1280×800 and 900×600 matched viewports were not achieved. The two saved raw Wails page captures are [`wails-account-details-1212x886.png`](evidence/wails-account-details-1212x886.png) and [`wails-allocation-trend-1212x886.png`](evidence/wails-allocation-trend-1212x886.png).

**My holdings.** The instrument summary and account-details views showed aligned quantity, cost, value, and gain columns, currency-separated group totals, and readable account labels. `QA-FULL` appeared under both brokerage accounts with distinct quantities and cost bases. The portfolio row actions use an anchored ellipsis menu in Wails; the Gio implementation now uses a menu anchored to each visible row rather than expanding a row inline. A focused regression asserts that opening it does not move the row at wide and compact test sizes. That Gio layout change has not been checked in a functioning native GUI.

**All instruments.** Searching `QA-BOTH` returned its matching instrument. The details view showed the instrument identity, account-scoped holdings, performance and data-record tabs, and the manual/provider/agent source filters. The unheld `QA-BUY` detail showed a clear no-holdings state; with its one manual price, the one-month range was empty and “Show all history” revealed its single point. The catalog and detail flows were observed in Wails, not yet paired against Gio native screenshots.

**Allocation & trend.** The Wails allocation donut showed the single valued allocation and the missing-valuation warning. The synthetic ledger had only one usable portfolio-trend date in the displayed table, so the trend line was blank; this data set does not establish whether a multi-day curve matches. Cash exclusion, date range controls, and another window size still need a paired run.

**Position import.** From the holdings page, the title action opened a contextual drawer that identified itself as recording an existing quantity and explicitly said it is not a buy and does not reduce cash. I selected Harbor Brokerage / `QA-BUY`, quantity `1`, and original per-unit cost `US$4`; the preview showed the effective local time and `0 → +1 → 1`. Confirm returned to the same My holdings / Account details page, displayed `QA-BUY` with quantity `1`, cost `US$4`, value `US$12.50`, and a “Change recorded” receipt. SQLite readback showed one new `position_transfer/reconciliation`, one `QA-BUY` holding for Harbor, unchanged existing holding quantities, the single original `US$100` cash row, and no `account_values` rows. This was a single synthetic write; duplicate-submit/idempotency was not exercised in this Wails run.

**Theme check.** The Settings selector briefly displayed Dark, but after navigating away it reverted to System and the assets page remained light. A stable dark-theme capture could not be established in this server-mode run. No dark-theme parity claim is made.

**Gio follow-up change and local checks.** The pending source change replaces the Gio holdings row's inline expanding controls with an anchored row menu while preserving account-scoped routes and menu focus restoration. It reports visible row origins through the shared column/list layout and tests stable row bounds around menu open/close in wide and compact layouts. The focused desktop tests `TestPortfolioMyHoldingsGroupedAmountsAndCompactNavigation`, `TestAccountPositionsFilterAndScopedInstrumentHistory`, and `TestAccountPositionDividendStartsWithExactHolding` passed locally; `TestPageTextSelectionHandlesInputMethodEditEvent` passed. `git diff --check` passed. A broader software-only desktop/UI/i18n run has known failures recorded below; it is not represented as green.

## Checklist results

“PDF baseline” means the status was recorded in the supplied comparison, not rechecked on this branch. “Implemented, UI unverified” means code and a regression assertion were added, but native UI behavior was not verified during this executor's run.

| # | Comparison item | Result |
| --- | --- | --- |
| 1 | Holdings total and quote date | PDF baseline records equal `US$3,272.27` totals and quote reference date; not rechecked here. |
| 2 | Holdings page density | Wails 1200×855 view shows summary, tabs, group totals, and four account rows in the first screen; Gio native comparison remains pending. |
| 3 | Holdings search | Reasonable Gio feature retained; not rechecked in a running app. |
| 4 | Group statistics and aligned amount columns | Wails 1200×855 totals and table columns are aligned; Gio native comparison remains pending. |
| 5 | Holding row identity hierarchy | Wails shows symbol, friendly name, and account beneath; Gio native comparison remains pending. |
| 6 | Instrument summary and per-account holdings | PDF baseline records both views and equal AAPL values; not rechecked here. |
| 7 | Holding row actions | Wails anchored menu observed. Gio row actions are now implemented as a row-anchored menu; focused layout tests pass, but native GUI verification is pending. |
| 8 | Chinese investment terminology | Follow-up; not rechecked. |
| 9 | 900×600 holdings layout | Not rechecked at the requested 900×600 viewport. |
| 10 | Instrument detail header | Wails identity and currency header observed; Gio native pair remains pending. |
| 11 | Holdings detail scope and account context | Wails detail showed account-scoped holdings and the unheld empty state; Gio native pair remains pending. |
| 12 | AAPL quantity, cost, value, and gain | PDF baseline records `12`, `US$2,204.00`, `US$2,424.00`, and `US$220.00`; not rechecked here. |
| 13 | Empty performance state | Wails `QA-BUY` detail shows no holdings; its one-month period says no local observations. Gio native state comparison remains pending. |
| 14 | Quick quote-maintenance access | Reasonable Gio shortcut retained in code; no quote was edited. |
| 15 | Quote-source filters and chart | Wails source filter choices and the single manual-price point are observed. A multi-day price curve is not established by this fixture. |
| 16 | Latest saved quote | PDF baseline records `AAPL US$202.00`, 11-23 08:00, manual; not rechecked here. |
| 17 | Daily close series, gaps, and range extremes | Follow-up; the supplied sample lacks daily close history. |
| 18 | Catalog sort and row layout | Not rechecked. |
| 19 | Catalog search fields | Wails `QA-BOTH` search returned the matching row; broader ISIN/market coverage and Gio search remain unverified. |
| 20 | Instrument creation field groups | Follow-up; no instrument was created. |
| 21 | Escape behavior while creating an instrument | Follow-up; the reported first-Escape behavior was not rechecked. |
| 22 | Instrument edit, quote, and closure layout | Existing Gio edit/maintenance entry points are unchanged; not rechecked. |
| 23 | Manual quote value 203 with blank date | Follow-up; the form was not opened or submitted in this run. |
| 24 | Archive, market closure, and provider/source changes | Follow-up; no changes were submitted. |
| 25 | Allocation amounts and percentages | Wails allocation showed one 100% valued allocation and a missing-valuation notice; PDF precision comparison and Gio rendering remain pending. |
| 26 | Portfolio value trend, dates, and cash exclusion | Wails showed one usable trend date and no drawn line; this fixture cannot validate a multi-day curve or cash exclusion. |
| 27 | Record-existing-holding entry | Wails drawer and successful submission were run on the synthetic ledger. Saul independently confirmed Gio scroll preservation at 900×600 on `15e22f64`; the later keyboard trap remains supported by code/tests but not this executor's native GUI. |
| 28 | Held-position filtering and single-candidate selection | Saul's independent X11 run confirmed the single-candidate explanation is visible. The test checks the complete note and asserts that one review semantic label contains the selected account and `QA-EUR`, the actually selected synthetic security. |
| 29 | Position-import preview | Wails preview showed account/security, effective local time, and quantity `0→+1→1`; its cost field was visible in the form. The Gio preview code/tests include cost and currency plus the no-cash notice; new Gio native review remains pending. |
| 30 | Save destination and receipt | Wails stayed on My holdings / Account details, refreshed the newly saved row, and displayed “Change recorded.” Gio's saved-in-place receipt and optional ledger activity action are covered by the branch test, not verified in a working native GUI here. |
| 31 | Position-import persistence and cash effects | Wails SQLite readback: 1 added `position_transfer/reconciliation`, 1 Harbor / `QA-BUY` holding at quantity `1`, existing quantities unchanged, cash row remains `US$100` (count 1), and `account_values` remains empty. Saul's earlier Gio acceptance separately read 3→4 with unchanged cash/value counts (`17`/`9`). These are isolated synthetic ledgers. |
| 32 | Text-selection panic from `key.EditEvent` | Fixed the unsafe type assertion in shared text selection. The new edit-event-then-Escape regression test passed in the software-only UI test configuration. The real AAPL price-form sequence was not run natively. |
| 33 | Full parity and regression boundary | Partial. Wails production frontend + real Go backend was visually inspected at 1200×855; Gio native rendering failed here. No full 33-row native pass, exact paired 1280×800 / 900×600 capture, dark-theme pair, or complete instrument-type regression is claimed. |

## Changes and checks

- `TextSelection.updateKeys` now recognizes Escape only when the event is a `key.Event`, then forwards other events such as `key.EditEvent` to Gio’s selectable router. Text selection/copy behavior was not deliberately changed.
- The portfolio import uses the existing guarded position-import workflow. Its review shows before/change/after quantities. Account-page position imports keep their existing title, candidate messaging, and review layout.
- The titlebar import no longer sets the shell's outer page scroll to zero. The new 900×600 test scrolls both the portfolio holdings list and outer stage, opens the drawer, then presses Escape and compares both offsets, the visible synthetic row, and restored titlebar focus.
- The portfolio import sheet now disables its hidden background and wraps Tab/Shift-Tab focus around its live controls. The focused regression starts at the account picker, cycles from Cancel to Close in both directions, and verifies Return cannot expand the hidden sidebar.
- Portfolio imports now use their own post-save callback, refresh the current holdings page, and expose an optional activity receipt. The account workflow keeps its existing post-save navigation callback.
- New import labels are translated in English, Simplified Chinese, Traditional Chinese, and Japanese.
- `git diff --check` passed before and after the mainline merge. At `15e22f64`, Linux CI exposed a stale `QA-BUY` assertion; the test imports the fixture's `QA-EUR` security, corrected at `02322acf`. Linux CI at `198eca51` showed the receipt lacked a ledger-open callback and that the focus test needed system Tab events. At `72b169ab`, CI confirmed the callback but found the return action needed to restore focus to the portfolio titlebar; the harness also needed to seed the native Tab destination because it doesn't model window-level focus traversal. The full Linux Gio suite then passed at `4cfafbde588d68c073406265a3b98617b4ec2035` (`gio-unit-tests`, job `140500809281`). The Darwin build failed before focused layout tests because `/var/folders/.../T/go-build` had no space. The broad check job also failed when the macOS runner could not write `actions-runner/_diag/Worker_20261127-122023-utc.log` because its disk was full.
- `go test -tags='nox11,nowayland,novulkan,noopengl' ./internal/ui -run TestPageTextSelectionHandlesInputMethodEditEvent -count=1 -v` passed.
- `go test ./internal/i18n -count=1` passed in `experiments/gio`.
- `go test ./internal/application -run TestBackdatedPositionImportPreviewsAndCommitsWithoutCash -count=1` passed at the repository root.
- The suggested `/workspace/shared/nw-ui` mount was not available in this executor. I used the extracted Go toolchain and EGL/XKB dependencies under `/tmp` to run the focused desktop and IME tests locally. At `02322acf`, Linux CI confirmed the corrected `QA-EUR` semantic label then exposed that the generic post-save callback navigated away from the portfolio; the portfolio now uses its dedicated in-place receipt callback. At `198eca51`, Linux CI showed the receipt's ledger activity action needed wiring. At `72b169ab`, the back action label needed the titlebar focus tag, and the test harness did not model normal native Tab traversal; both are corrected at `4cfafbde`. The Linux full suite passed at that SHA. macOS build and broad correctness checks remain blocked by the self-hosted runner's full disk.
- The full software-only UI suite has a pre-existing failure in `TestPageTextDragAndCopy`: clipboard output is `余额 CN¥1,022,706.5` instead of `余额 CN¥1,022,706.58`. I reproduced the same failure in a clean worktree at `75b8069d`, before these changes.

- The focused row-action regression passed at the current working tree. A full local software-only desktop/UI/i18n run did not pass: it reported `TestStandaloneProductSetupMatchesWails/account_record_existing`, `TestChartDataTableSizesColumnsToContent`, `TestPreviewPopoverFollowsAnchorAndRestoresFocus`, and the known `TestPageTextDragAndCopy` failure, then an async nil-DB panic during teardown. The three relevant holdings tests and the IME regression pass separately; this broader failure is not claimed as a green run.

## Remaining work

Remaining work: validate the changed Gio row menu and the complete three-page parity natively on Linux X11 at both 1280×800 and 900×600, with a stable light/dark pair; collect matching Wails captures at those exact sizes; verify multi-day trend data and cash exclusion with a suitable synthetic ledger; finish the unvisited PDF checklist items; and re-run the real AAPL price-form IME/Escape sequence. The report includes the two saved Wails page PNGs; the Wails import receipt was observed live but not saved as a repository PNG. Saul's scroll/focus and modal-trap evidence is independent of this executor and must not be attributed to a user-run test.
