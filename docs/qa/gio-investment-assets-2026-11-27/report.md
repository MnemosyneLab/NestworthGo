# Gio Investment Assets parity handoff · 2026-11-27

## Scope and evidence

The source comparison is the Library PDF *Nestworth-投资资产-Gio-Wails-差异核对.pdf* (25 pages; 33 checklist items; 20 screenshot groups). I read its extracted text. I could not materialize or inspect the embedded screenshots in this environment, so all references below to the earlier AAPL, ES3, valuation, and layout checks are labeled as PDF baseline observations, not as new verification.

That report used main `ef09d4cc` and isolated synthetic SQLite ledgers, with Gio on Linux X11 and Wails’ production frontend plus Go backend in Chromium at 1280×800 and 900×600. This run did not launch either application, create or copy ledgers, or capture new screen evidence. The development image lacks the native build dependencies needed to start Gio and its Wails/Chromium test path.

The implementation branch began at PR #299 head `188b4413dabf251df61050d6e670a10471333e0b`. PR #299 was still open and had advanced to `75b8069dac011a5d30ac26720702048d58baf3d8`; that head descends from `188b4413`. The task branch was fast-forwarded to the live account-parity head before applying these changes. The intended stacked PR base is `feat/gio-account-parity`. The account changes are therefore upstream history, not part of this branch’s change set. The panic was still present at the requested `188b4413` head.

Implementation commit: `9ba8c88540eba7d52d7bf5745c812e1934da141e` (author and committer Walter Wang, `wangweichen1994@gmail.com`).

## Checklist results

“PDF baseline” means the status was recorded in the supplied comparison, not rechecked on this branch. “Implemented, UI unverified” means code and a regression assertion were added, but the desktop package could not be built here.

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
| 27 | Record-existing-holding entry | Implemented, UI unverified: the portfolio remains mounted behind a contextual side panel. Cancel still uses the existing draft cancellation and focus-restoration path. |
| 28 | Held-position filtering and single-candidate selection | Existing per-account filtering and explicit account choice remain. A message now explains when the sole eligible security is auto-selected. Desktop interaction test added, not run. |
| 29 | Position-import preview | Implemented, UI unverified: portfolio preview adds quantity before/change/after, account and instrument context, unit cost and currency, effective time, and no-cash notice. |
| 30 | Save destination and receipt | Implemented, UI unverified: save refreshes holdings in place and leaves an optional “View ledger activity” receipt. Back navigation recognizes the receipt button as its source. |
| 31 | Position-import persistence and cash effects | Existing application test `TestBackdatedPositionImportPreviewsAndCommitsWithoutCash` passed. The new portfolio UI readback test did not run; this run did not count cash rows or remeasure portfolio value. |
| 32 | Text-selection panic from `key.EditEvent` | Fixed the unsafe type assertion in shared text selection. The new edit-event-then-Escape regression test passed in the software-only UI test configuration. The real AAPL price-form sequence was not run natively. |
| 33 | Full parity and regression boundary | Follow-up. No full 33-row native pass, Wails run, paired 1280×800/900×600 captures, instrument writes, or complete asset-type regression is claimed. |

## Changes and checks

- `TextSelection.updateKeys` now recognizes Escape only when the event is a `key.Event`, then forwards other events such as `key.EditEvent` to Gio’s selectable router. Text selection/copy behavior was not deliberately changed.
- The portfolio import uses the existing guarded position-import workflow. Its review shows before/change/after quantities. Account-page position imports keep their existing title, candidate messaging, and review layout.
- The portfolio save now refreshes the current page and exposes an optional activity receipt; the account workflow’s posted-navigation callback remains unchanged.
- New import labels are translated in English, Simplified Chinese, Traditional Chinese, and Japanese.
- Formatting and `git diff --check` passed.
- `go test -tags='nox11,nowayland,novulkan,noopengl' ./internal/ui -run TestPageTextSelectionHandlesInputMethodEditEvent -count=1 -v` passed.
- `go test ./internal/i18n -count=1` passed in `experiments/gio`.
- `go test ./internal/application -run TestBackdatedPositionImportPreviewsAndCommitsWithoutCash -count=1` passed at the repository root.
- The desktop import test could not build: standard Gio prerequisites (`xkbcommon`, `wayland-client`) are absent; disabling those platform backends still stops at missing EGL development metadata (`egl.pc`) and headers. No system packages were installed.
- The full software-only UI suite has a pre-existing failure in `TestPageTextDragAndCopy`: clipboard output is `余额 CN¥1,022,706.5` instead of `余额 CN¥1,022,706.58`. I reproduced the same failure in a clean worktree at `75b8069d`, before these changes.

## Remaining work

Run the desktop test and same-ledger Gio/Wails interaction pass after provisioning the native dependencies. Capture raw synthetic evidence at both requested sizes, including cancel/no-write, import review, persisted readback, unchanged cash entries, quote maintenance, catalog Escape, and holdings row actions. Recheck the 33 PDF rows against the latest `feat/gio-account-parity` base. The added UI behavior has not been accepted on native Linux, and the real AAPL crash path was not reproduced again.
