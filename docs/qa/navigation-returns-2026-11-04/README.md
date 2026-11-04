# Navigation return entry cleanup — 2026-11-04

Base: latest main at task start, `d470555`.
Only synthetic test data is used. No personal ledger, backup, account names or
user screenshot is read, copied or uploaded.

## Change and retained boundaries

- The desktop page bar is the single history Back/Forward entry. Names and native
  tooltips identify its actual adjacent destination, using Recent's existing
  business-label resolver and deleted-object fallback.
- Account detail displays **Accounts / current account**. Its parent action
  preserves the existing account-list/filter return and original cross-page
  source. It retains the shared Button, confirmation gate and final-focus ref.
- Historical overview's **View current overview** always navigates to Overview
  through the same confirmation gate. The historical entry/capture stays
  reachable through Back, Forward and Recent. The explicit current-overview
  action focuses main-content, which is mounted even while current reads reload.
- Mobile retains its separate navigation; account → holding → Back returns to
  that account before returning to the list. A fresh tab/session resets the
  optional account source. Form cancellation and pending/unknown recovery are
  unchanged.

The source audit retained reconciliation session-list return
(`ReconciliationPage`), category clearing (`HistoricalOverviewPage`), preview to
draft (`RecordChangeForm`, `AccountActionSheets`), form cancellation and wizard
steps, date navigation (`HistoricalOverviewPage`, `ReturnCalendarTab`) and
pagination. These remain business transitions, not history traversal.

## Reproduction

Use the pinned Go 1.27.3, Node 24.22.0, pnpm 12.14.0 and Wails 3.1.0 tools.
The Linux cloud worker lacks GTK/WebKit development headers. System package
installation failed with filesystem permission errors; `GOFLAGS=-tags=purego`
uses Wails' supported pure-Go compilation path for binding generation. This is
not evidence of a GTK desktop build or macOS GUI acceptance.

Run `pnpm run lint`, `pnpm run typecheck`, `pnpm run test`, and `pnpm run build`
from `frontend`. Generated bindings and bundles are not committed.

For browser acceptance, create a fresh root under `/tmp` or `/workspace/scratch`:

```bash
node docs/qa/navigation-returns-2026-11-04/browser-build.mjs "$QA_ROOT"
```

The builder derives the established same-page layout fixture: 61 synthetic
accounts and 40 synthetic holdings. It pins the backend clock to 2026-11-01
for seeding so the real host can read closed historical days. It uses the
actual production frontend and real services, with no frontend instrumentation.
It writes and checks a synthetic marker, refuses an existing database, builds
an HTTP server and restores the original generated frontend bundle afterward.

Launch `$QA_ROOT/server` with `NESTWORTH_DATABASE_PATH=$QA_ROOT/household.db`,
`NESTWORTH_SETTINGS_PATH=$QA_ROOT/settings.json`, isolated XDG directories,
`WAILS_SERVER_HOST=127.0.0.1` and `WAILS_SERVER_PORT=18744`. Run:

```bash
python3 docs/qa/navigation-returns-2026-11-04/browser-acceptance.py \
  --qa-root "$QA_ROOT" --base-url http://127.0.0.1:18744
```

The script changes only synthetic settings and reads synthetic accounts. It
checks empty history, list/detail/filter/real scroll and main-content focus
fallback, Forward, Recent object identity, Settings → historical → current
Overview, frozen-capture service-call reuse, 640px layout, menu Escape/focus,
all three languages and reload. The unchanged account-list row has no stable
focus ID; its established fallback is main-content. Retained analysis-trigger
focus restoration is separately exercised by App tests.

## Executed checks

Frontend evidence is tied to [the final source fingerprints](frontend-source-sha256.json).
The source files are unchanged between these checks and the delivered commit.

- Full lint and locale coverage: passed (15 locale coverage tests); affected-file
  lint passed again after test locator/focus changes.
- Full typecheck and final production build: passed. Wails generated 21 services,
  176 methods and 289 models. Existing large-chunk advisory remains.
- Full local Vitest invocation: 165 files / 1,734 tests; 1,731 passed and three
  newly added mobile cases failed because their Quick record locator matched
  both the account action and the bottom navigation action. The locator now
  scopes to the account hero; no runtime behavior or assertion was weakened.
- After that test fix: all 179 tests in App, MobileApp, navigation controls and
  historical overview passed. Coverage includes reconciliation drafts/guards,
  same-page objects, deleted objects, filter/scroll/focus restoration, empty
  stacks, onboarding, history traversal/Recent and all three languages.
- Browser acceptance exposed asynchronous Overview readiness: its body entry
  could be absent when focus restoration ran. The explicit current-overview
  action now targets the stable main-content landmark. The final App run passed
  61 of 62 tests, with one existing historical-capture case hitting its unchanged
  5-second deadline during local resource contention. A subsequent isolated run
  passed that case and the three explicit-overview/focus cases (4 passed). No
  timeout was raised. All other related tests have passing evidence above.
- Final actual-production HTTP Chromium acceptance: [8 cases passed](browser-results.json),
  zero page or application console errors. It verifies real scroll and focus,
  destination identity, frozen-capture read reuse, Recent, reload and 640px
  overflow/menu keyboard behavior in all three locales. The first browser
  sandbox launch failed because this worker's SUID helper is unavailable;
  the successful run used explicit `--allow-no-sandbox` and isolated browser
  XDG directories. No system security configuration was changed.

Browser screenshots: [English](account-640-en.png),
[Simplified Chinese](account-640-zh-CN.png),
[Traditional Chinese](account-640-zh-TW.png).

The final origin commit triggers the full regular native Mac CI. Its exact-head
run/check status is handed off separately; the staged local evidence above is
not described as one entirely green full-suite invocation.

## Acceptance limits

Linux HTTP Chromium and jsdom do not establish native macOS GUI, VoiceOver,
native keyboard/IME, OS window closing/restart, signing, packaging or physical
phone/touch acceptance. Those manual gates remain unverified. Pending/unknown
confirmation and reconciliation draft retention are covered by frontend
regressions; they are not injected into the real-service browser host.

No financial calculations, persistence schema or navigation storage is added.
The destination labels reuse existing account/instrument queries, with no new
polling or service request. No measured performance claim is made.
