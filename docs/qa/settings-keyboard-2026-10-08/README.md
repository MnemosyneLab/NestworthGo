# Workspace keyboard boundary — PR35

## Report and scope

The supplied Linux GUI reproduction loses keyboard focus after the final
Settings action (Restore all preference defaults, with Save disabled) and
after Directory → Members → Archive. It occurs on both main `8471593` and
PR35 `c727d6c`, on Debian 13/X11, GTK 4.18.6 and WebKitGTK 6.0 2.54.0.
Tab/Shift+Tab and returning from another window do not recover it; a mouse
click does. This report is not a native reproduction in this cloud workspace.

Source inspection found no workspace boundary/re-entry handling in AppShell.
Wails beta.28's `linux_cgo.c:setupWindowEventControllers` attaches key handling
to the WebView and focus handling to the window. Its Linux `focus()` presents
the window; it does not restore a particular DOM control. These observations
identify the unhandled application-content boundary, but do not establish a
WebKit defect or prove why this native host fails to re-enter after leaving it.
This workspace lacks GTK/WebKit development packages and Xvfb.

## Change

AppShell now has a focus sentinel before and after its content. Normal browser
Tab navigation reaches a sentinel only at the edge; it immediately focuses
the first/last currently tabbable workspace control. Entry from outside the
workspace uses the nearest edge. Disabled, hidden and inert controls are
excluded by the existing `tabbable` dependency. There is no document/window
keydown, blur or focus listener, shortcut interception, or forced native-window
activation. A modal-inert/aria-hidden workspace does not redirect focus.
Portal dialog focus management stays with Base UI. Toast access through the
existing Sonner keyboard shortcut remains outside this workspace scope.

The existing tab-panel Tab stop remains in place, with a visible focus ring
instead of an invisible `outline-none` stop. No native configuration or
backend/ledger/R2 behavior changes.

## Evidence

- Full frontend run: 86 files / 753 tests passed (`--maxWorkers=2`).
- Focused keyboard/settings run: 3 files / 37 tests passed. Lint,
  TypeScript, production build and generated-bindings verification passed.
- gofmt, Go tests/vet/build, the skill checker and 19 installer tests passed
  with local `GOFLAGS='-mod=readonly -tags=server'` and `CGO_ENABLED=0`.
  Native/race CI is recorded on the PR; native GUI acceptance is separate.
- Regression tests exercise real Settings and Directory inside AppShell with
  keyboard-only first/last traversal, forward wrap and reverse wrap.
- Focus-boundary tests cover input/select traversal, disabled/hidden/inert
  candidates, entry from outside, untouched modifier/native shortcut events,
  Portal confirmation cycling/cancel return, and kept-mounted inactive tabs.
- Chromium browser checks use the actual AppShell, Settings, Directory, Tabs
  and confirmation components with synthetic services. At 1400×700 and
  640×700, both pages wrap in both directions, their confirmations cycle and
  cancel back to the trigger, and the tab-panel stop has a visible CSS ring.
  `browser-results.json` records these four cases; screenshots show wrapped
  sidebar focus and focused tab panels. No real database or R2 service is used.
- A diagnostic browser comparison removes only the new sentinel spans from
  the rendered page: Tab after Restore then moves to BODY; with the spans in
  place it moves to Toggle sidebar. `browser-boundary-comparison.json` records
  this application-side boundary behavior. It is not a native reproduction
  of the reported failure to recover focus.
- These browser checks do not establish GTK/WebKit native acceptance. See the
  PR's exact head and CI result for the final checked build.

## Grok native verification handoff

Use the exact keyboard-fix commit named in PR35/the handoff response, rather
than an older PR build. In an isolated checkout with Go 1.26, pnpm 12.2.1,
Wails beta.28 and the GTK4/WebKit6/libsoup development dependencies installed:

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run build
FOCUS_QA_ROOT="$(mktemp -d /tmp/nestworth-pr35-focus.XXXXXX)"
CGO_ENABLED=1 GOFLAGS=-mod=readonly go build -o "$FOCUS_QA_ROOT/nestworth" ./cmd/nestworth
NESTWORTH_DATABASE_PATH="$FOCUS_QA_ROOT/household.db" \
NESTWORTH_SETTINGS_PATH="$FOCUS_QA_ROOT/settings.json" \
GTK_A11Y=none WEBKIT_DISABLE_DMABUF_RENDERER=1 \
WEBKIT_DISABLE_COMPOSITING_MODE=1 "$FOCUS_QA_ROOT/nestworth"
```

Use fresh synthetic onboarding data (one sample member is sufficient). Do not
copy a real database, settings, credentials, or enable R2. Verify:

1. Settings, Save disabled: Tab from Restore all preference defaults reaches
   the visible Toggle sidebar focus ring. Shift+Tab returns to Restore. Repeat
   several cycles with both sidebar states and at narrow/low window sizes.
2. Directory → Members: the same round trip from the last Archive control.
3. Initial Tab enters the first workspace control; initial Shift+Tab enters the
   last. Return after Alt+Tab and repeat the edge cycle without a mouse.
4. Settings/Directory tab-panel focus has a visible ring; the next Tab reaches
   its content. Inactive panels and disabled Save cannot receive focus.
5. Open Reset/Archive confirmation and cancel without executing it. Tab and
   Shift+Tab stay within the dialog and cancellation returns focus to its
   trigger. Repeat the already-working restore-preview cancel flow using only
   a synthetic backup, if available.
6. Text editing, native selects, combobox/menus and F10/Alt native-menu access
   retain their behavior. Escape from a native file picker still permits the
   next Tab, even if the platform does not immediately draw focus-visible.

Report exact commit, environment, focused control after each boundary, and
any failing sequence. Native acceptance remains pending until that report.
