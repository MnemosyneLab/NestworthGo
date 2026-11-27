# Gio Investment Assets parity handoff · 2026-11-27

## Scope and evidence

The source comparison is the Library PDF *Nestworth-投资资产-Gio-Wails-差异核对.pdf* (25 pages; 33 checklist items; 20 screenshot groups). I read its extracted text but could not materialize or inspect its embedded screenshots. References below distinguish the PDF's baseline notes from Saul's newer independent X11 observations; no raw screenshots from that separate run were available to this executor.

That report used main `ef09d4cc` and isolated synthetic SQLite ledgers, with Gio on Linux X11 and Wails’ production frontend plus Go backend in Chromium at 1280×800 and 900×600. This executor did not launch either application or capture new evidence. Saul independently ran part of the Gio workflow on Linux X11 at 900×600, as recorded below; Wails and the 1280×800 comparison remain unverified here.

The implementation branch began from the requested PR #299 head `188b4413dabf251df61050d6e670a10471333e0b` and the then-current #299 lineage at `75b8069dac011a5d30ac26720702048d58baf3d8`. PR #299 was later squash-merged to `main` as `bdadfcb72a24b686f34a593bf1a1f98f80b04109`; PR #300 auto-retargeted to `main`. I merged that `main` into the existing task branch with a regular merge commit (`80e00c47`) and kept the newer account files from `main`. The diff against `main` is 15 files, all limited to the investment-asset increment. The panic was present at the requested `188b4413` head.

Feature commit: `9ba8c88540eba7d52d7bf5745c812e1934da141e`. Scroll preservation and its regression: `aa95a410793d5e7e6eb6599982eab6fadc84aae2`. Test assertion fixes: `c4ea8bea541619939694b453ea17c83f82fb2738`, `90de3c95067aa6a6999c0f45558eb8d313bdfc03`, and `dcfbf8ca7e550af312163100816b9cf3e67d3393`. Import-sheet focus and receipt fixes: `198eca5179adad09b136aa0fb42a3c8d03e25f6c`, `72b169abbcc753f19ba8645924a2c34e8671a9c0`, and `4cfafbde588d68c073406265a3b98617b4ec2035`. All implementation/test commits are by Walter Wang, `wangweichen1994@gmail.com`. Mainline merge: `80e00c47e76d2dac31806b77bf6fc9023b843beb`.

Existing PR: [#300](https://github.com/MnemosyneLab/NestworthGo/pull/300), open against `main` at `bdadfcb72a24b686f34a593bf1a1f98f80b04109`. The merged branch preserves the latest account work from `main`, and its platform PR diff is investment-only. Saul independently ran Linux X11 acceptance; no Slack or reviewer handoff is needed.

## Checklist results

“PDF baseline” means the status was recorded in the supplied comparison, not rechecked on this branch. “Implemented, UI unverified” means code and a regression assertion were added, but native UI behavior was not verified during this executor's run.

| # | Comparison item | Result |
| --- | --- | --- |
| 1 | Holdings total and quote date | PDF baseline records equal `US$3,272.27` totals and quote reference date; not rechecked here. |
| 2 | Holdings page density | Follow-up; the summary spacing and first-screen density were not rechecked. |
| 3 | Holdings search | Reasonable Gio feature retained; not rechecked in a running app. |
| 4 | Group statistics and aligned amount columns | Follow-up; not rechecked. |
| 5 | Holding row identity hierarchy | Follow-up; not rechecked. |
| 6 | Instrument summary and per-account holdings | PDF baseline records both views and equal AAPL values; not rechecked here. |
| 7 | Holding row actions | Follow-up; Gio’s inline expansion still needs comparison with an anchored menu. |
| 8 | Chinese investment terminology | Follow-up; not rechecked. |
| 9 | 900×600 holdings layout | Follow-up; no narrow-window capture was made. |
| 10 | Instrument detail header | Follow-up; not rechecked. |
| 11 | Holdings detail scope and account context | Follow-up; not rechecked. |
| 12 | AAPL quantity, cost, value, and gain | PDF baseline records `12`, `US$2,204.00`, `US$2,424.00`, and `US$220.00`; not rechecked here. |
| 13 | Empty performance state | Follow-up; the supplied ledger has no daily closes, so non-empty performance remains untested. |
| 14 | Quick quote-maintenance access | Reasonable Gio shortcut retained in code; no quote was edited. |
| 15 | Quote-source filters and chart | Follow-up; source labels and non-empty series were not rechecked. |
| 16 | Latest saved quote | PDF baseline records `AAPL US$202.00`, 11-23 08:00, manual; not rechecked here. |
| 17 | Daily close series, gaps, and range extremes | Follow-up; the supplied sample lacks daily close history. |
| 18 | Catalog sort and row layout | Follow-up; not rechecked. |
| 19 | Catalog search fields | Existing broader ISIN/market search is retained; search behavior was not rechecked. |
| 20 | Instrument creation field groups | Follow-up; no instrument was created. |
| 21 | Escape behavior while creating an instrument | Follow-up; the reported first-Escape behavior was not rechecked. |
| 22 | Instrument edit, quote, and closure layout | Existing Gio edit/maintenance entry points are unchanged; not rechecked. |
| 23 | Manual quote value 203 with blank date | Follow-up; the form was not opened or submitted in this run. |
| 24 | Archive, market closure, and provider/source changes | Follow-up; no changes were submitted. |
| 25 | Allocation amounts and percentages | PDF baseline records equal amounts with different display precision; not rechecked here. |
| 26 | Portfolio value trend, dates, and cash exclusion | Follow-up; not rechecked. |
| 27 | Record-existing-holding entry | Saul's independent Linux X11 run at 900×600 on `f425a440` reproduced a 23 px background jump: the valuation card moved from y≈106 to y≈129 after opening and dismissing the titlebar drawer. On `15e22f64`, Saul confirmed the scroll fix: the valuation card and ES3/QQQ row key pixels matched before and after, and Escape restored titlebar focus. This follow-up adds a focus trap and keyboard regression; native recheck of that part is pending. |
| 28 | Held-position filtering and single-candidate selection | Saul's independent X11 run confirmed the single-candidate explanation is visible. The test checks the complete note and asserts that one review semantic label contains the selected account and `QA-EUR`, the actually selected synthetic security. |
| 29 | Position-import preview | Implemented, UI unverified: portfolio preview adds quantity before/change/after, account and instrument context, unit cost and currency, effective time, and no-cash notice. |
| 30 | Save destination and receipt | Implemented, UI unverified: save refreshes holdings in place and leaves an optional “View ledger activity” receipt. Back navigation recognizes the receipt button as its source. |
| 31 | Position-import persistence and cash effects | Existing application test `TestBackdatedPositionImportPreviewsAndCommitsWithoutCash` passed. In Saul's independent synthetic X11 acceptance, importing US Brokerage / ES3 quantity `1` at unit cost `4 SGD` previewed `0→1`; SQLite readback showed one `position_transfer/reconciliation`, holdings `3→4`, and unchanged `account_values` / `account_cash_values` row counts (`9` / `17`). This is separate user-run evidence, not a new measurement by this executor. |
| 32 | Text-selection panic from `key.EditEvent` | Fixed the unsafe type assertion in shared text selection. The new edit-event-then-Escape regression test passed in the software-only UI test configuration. The real AAPL price-form sequence was not run natively. |
| 33 | Full parity and regression boundary | Partially verified by Saul's 900×600 X11 run and this branch's targeted CI tests. No full 33-row native pass, Wails run, paired 1280×800/900×600 captures, or complete asset-type regression is claimed. |

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
- This executor cannot access the suggested `/workspace/shared/nw-ui` mount or a working Go toolchain, so it cannot rerun the desktop package locally. GitHub CI provisions the Linux Gio dependencies. At `02322acf`, Linux CI confirmed the corrected `QA-EUR` semantic label then exposed that the generic post-save callback navigated away from the portfolio; the portfolio now uses its dedicated in-place receipt callback. At `198eca51`, Linux CI showed the receipt's ledger activity action needed wiring. At `72b169ab`, the back action label needed the titlebar focus tag, and the test harness did not model normal native Tab traversal; both are corrected at `4cfafbde`. The Linux full suite passes at that SHA. macOS build and broad correctness checks remain blocked by the self-hosted runner's full disk.
- The full software-only UI suite has a pre-existing failure in `TestPageTextDragAndCopy`: clipboard output is `余额 CN¥1,022,706.5` instead of `余额 CN¥1,022,706.58`. I reproduced the same failure in a clean worktree at `75b8069d`, before these changes.

## Remaining work

Ask Saul to repeat the 900×600 Tab/Shift-Tab scenario on `4cfafbde588d68c073406265a3b98617b4ec2035`. Saul's native recheck already confirmed the scroll correction on `15e22f64`. The import readback covers synthetic no-cash behavior at 900×600, but no raw evidence was delivered to this executor. The Wails comparison, 1280×800 captures, remaining PDF rows, and real AAPL crash sequence remain unverified here.
