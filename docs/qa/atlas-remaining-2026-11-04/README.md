# Remaining Atlas acceptance: D1–D4 and F2

Date: 2026-11-04. Base: `82dd9321b8080523c3bae4640f913708292b1966`
(main at the start of this task). Functional source:
`4590e74595297219d7ce0a05a3fb60e2d4476a8d`. The subsequent acceptance
commit contains documentation, browser runners and synthetic evidence only.
Delivery: [Draft PR #223](https://github.com/MnemosyneLab/NestworthGo/pull/223).
No merge, release, real-household testing, #218/#219 reimplementation, or
historical-attribution backfill is included.

## Reproduction and fixes

| Item | Reproduced cause | Change and regression |
| --- | --- | --- |
| D1 | A refused shared-token exchange used the same `session` error as an expired grant. Mobile rendered HTTP codes through the domain-only catalog and constructed field keys instead of using the canonical map. | Wrong tokens now return `401 invalid_token`, field `token`; invalid grants retain `401 session_expired`. The original fifth-attempt budget and sixth-attempt 429 remain. A mobile transport catalog supplies all three locales, including `rate_limited`, `csrf_expired`, `forbidden` and `session_changed`. Known field labels prefix messages; unknown fields add no label or trailing separator. Three new login locale cases failed before the fix and passed afterwards. The Go regression checks actual statuses, code distinctions, unchanged attempt budget, unknown operation state and absence of credential echoes. |
| D2 | Native `required`, `minLength` and `pattern` prevented `onSubmit` from reaching the application validation path. | The owning settings form uses `noValidate`, retaining input semantics and the authoritative Go validation. Twelve cases (empty address/token, short token and invalid token characters, in three languages) failed before the fix. All pass with real shared Input/Button/FieldError components, associated inline alerts, `aria-invalid` and descriptions. Disabling access with incomplete fields remains covered. |
| D3 | The settings component stored `displayError(...)`'s already translated string. | Retain the stable WireError, recover callService's original payload, and translate when rendering. The new language-switch test failed with retained English text before the fix, then passed through Simplified Chinese, Traditional Chinese and English without another Configure call, lost draft or cleared invalid state. |
| D4 | `common.loading` was absent in every locale; equal-key-set checks could not detect a key missing everywhere. | Add the shared label in all three base resources. Three explicit missing-key cases failed before the fix. Desktop resources and lazy mobile resources are now checked, and a held real HTTP session read displays the translated status in the production bundle. |
| F2 | Evidence summary had line-height-only geometry; shared Sheet Close had a 32px minimum at every width. | Evidence is 44px on mobile/narrow surfaces and retains its original 20px desktop line at xl. Shared Sheet Close is 44px below xl and 32px on the normal desktop; mobile headers explicitly retain 44px even at wide viewport widths. Existing Base UI focus, Escape and confirmation guards are preserved. Six pre-fix regression assertions failed; the assistant/Sheet suites passed after the fix. |

Authentication comparison, token storage, CSRF, permission gates, rate budgets,
session generation fencing, unknown financial operation recovery and closed AI
activation gates are unchanged. The HTTP change only distinguishes an existing
refusal. No financial rules moved into the frontend.

## Browser evidence

Linux Chromium `154.0.8037.57`. Every matrix covers English, Simplified Chinese,
Traditional Chinese, light and dark themes. All data, values and credentials
are synthetic.

- [24 component cases](evidence/component-browser-results.json): mobile at
  360/1440px and desktop at 768/1440px. Tests exercise actual settings,
  MobileAssistant, DesktopAssistant, AssistantConversation and shared Sheet
  components, with explicit synthetic Wails/status adapters. The adapters
  return structured rejections; they do not implement product validation.
- [Six 320px cases](evidence/mobile-320-results.json): additional narrow layout
  and keyboard coverage.
- [Six 320px touch-emulation cases](evidence/touch-browser-results.json):
  actual emulated taps toggle the evidence disclosure and dismiss the panel;
  the keyboard review/return-focus checks and layout checks also pass.
- [Six real HTTP cases](evidence/http-browser-results.json): production mobile
  bundle plus the existing opt-in `TestMobileBrowserFixture`. Each fixture
  creates its own synthetic household/database/settings. Real token refusals,
  real 429 admission and expired-grant responses are checked. Initial session
  reads are held to inspect translated loading status before continuing to
  the real server. No product endpoints or authentication paths are mocked.

For each component case, Enter submits invalid settings to the adapter and the
localized inline alert becomes available; `aria-invalid` and described-by IDs
are checked. Retained errors update on language change without resubmission.
Evidence opens/closes by Enter and Space. Review dismissal and focus restoration
are checked; Escape closes the containing narrow panel and returns focus to its
trigger. Measured Close rectangles stay inside the viewport, titles do not
intersect Close, and the assistant/document have no horizontal overflow.

| Surface | Evidence summary | Review Close | Panel Close |
| --- | ---: | ---: | ---: |
| Mobile 320/360/1440px | 44px | 44px | 44px |
| Desktop 768px | 44px | 44px | 44px |
| Desktop 1440px | 20px | 32px | Existing dock icon control |

Screenshots: [320px Traditional Chinese dark](evidence/mobile-320-dark.png),
[mobile review](evidence/mobile-review-dark.png),
[desktop density](evidence/desktop-density.png),
[real HTTP 429](evidence/mobile-429-dark.png).
The [HTTP accessibility snapshot](evidence/mobile-429-accessibility.yml) retains
the alert and localized control names.

Reproduce after installing the locked frontend dependencies and generating
bindings with the repository's supported toolchain:

```sh
# PLAYWRIGHT_MODULE may point at an installed playwright package's index.mjs.
node docs/qa/atlas-remaining-2026-11-04/browser.mjs
ATLAS_BROWSER_CASES='[["mobile",320]]' \
  node docs/qa/atlas-remaining-2026-11-04/browser.mjs
ATLAS_TOUCH=1 ATLAS_BROWSER_CASES='[["mobile",320]]' \
  node docs/qa/atlas-remaining-2026-11-04/browser.mjs
# Requires a prior production frontend build and the Go toolchain on PATH.
node docs/qa/atlas-remaining-2026-11-04/http-browser.mjs
```

The runners use temporary frontend entries and remove them on completion.
`ATLAS_BROWSER_OUTPUT` selects a separate output directory; `CHROMIUM_PATH`
selects the installed browser. The existing fixture remains opt-in and is never
added to product startup.

## Automated checks and limits

Environment: Linux amd64, Go 1.27.3, Node 24.22.0, pnpm 12.14.0.
The host lacks GTK/WebKit development headers; the existing Debian native
build container supplies them for genuine Wails v3.1.0 binding generation and
Linux compilation. Bindings and frontend dist are generated, ignored outputs.
After one successful generation, frontend consumers reuse those exact bindings.

Passed checks:

- `go test ./internal/mobileapi -count=1` (9.281s); `go vet ./internal/mobileapi`.
- Canonical `go build ./cmd/nestworth` completed in the Linux native-dependency
  container (output redirected to a scratch binary, not launched). Upstream
  Wails GTK/X11 deprecation warnings remain; no build error occurred.
- `pnpm run lint`, including canonical locale coverage.
- `pnpm exec tsc --noEmit` and `pnpm exec vite build --mode production`.
  The existing 588.40kB ECharts vendor-chunk warning remains; no bundle-limit
  or timeout setting was changed.
- The final functional source passed all **13 affected frontend files / 175
  tests** with one worker, including mobile HTTP-client/session fencing tests,
  mobile app flows, settings controls, Sheet, assistant, privacy and locale
  regressions. No automatic retries or increased timeouts.
- Focused regressions during development: MobileAccessSection 23 tests after
  D2; MobileAccessSection + wire helpers 39 after D3; assistant/Sheet 32 after
  F2; loading/locale checks 21 after D4; three mobile login locales after D1.
- `git diff --check`; gofmt on the changed Go files.

The full local frontend suite was attempted, then interrupted after seven
reported files (459 tests, 60 failures) to avoid continuing an already failing
broad run. Failures included timeouts and missing role/name assertions in
HistoryPage, App, HistoricalOverviewPage, AccountsPage, ReconciliationPage,
HoldingDetailPanel and the global keyboard suite. This is **not a green full
suite**. Early resource contention may affect timings; it does not establish
that every failure is environmental.

An isolated worktree at the unmodified base reproduced five selected failures
in four of those files with one worker; the exact assertions are retained in
[the baseline sample](evidence/baseline-sample.txt). This demonstrates those
samples predate the patch, not that every full-suite failure has been triaged.
Unrelated fixtures, expectations and test timeouts were not changed. Native
Mac CI remains the required full regular gate; see the PR for exact-head run
status rather than inferring success from an older green commit.

The native Mac regular gate passed for the exact functional head `4590e74`:
[run 41849232282](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41849232282),
[job test](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41849232282/job/126643895025).
The run waited about 9m23s for its job; the job itself took 3m36s, with 2m40s in
correctness lanes, 4s binding generation and 3s frontend build. The queue delay
must not be interpreted as a test-runtime increase. This passing automated Mac
gate does not establish manual GUI or screen-reader acceptance. The final
acceptance-only push starts another exact-head check; its state is reported in
the PR rather than reusing the functional-head result as a final-head green.

This environment cannot establish native macOS app/VoiceOver acceptance,
Safari/WebKit behavior, actual iOS/Android devices or physical touch accuracy.
Chromium keyboard, DOM accessibility descriptions, accessibility snapshots and
emulated touch are supporting evidence, not an actual screen-reader audit.
No native application was launched, and no production or real household data
was accessed. Full Go/domain suites, race/performance runs and packaging were
not repeated for this scoped presentation/HTTP-error change.

Performance: additional locale lookups during error rendering and CSS-only
geometry changes; no additional provider/model calls or background reads were
introduced. No latency or p95 improvement is claimed.
