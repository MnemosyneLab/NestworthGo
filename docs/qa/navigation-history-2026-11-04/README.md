# Desktop navigation history acceptance — 2026-11-04

Base: main `1701e5d5d9bb10ec9db846245766bab6f7a740e5`.
The follow-up frontend source fingerprint is recorded in
[frontend-source-sha256.json](frontend-source-sha256.json). These file hashes
must match the reviewed commit; generated bindings and bundles are not committed.
Earlier screenshots retain their original validation scope as identified below.

## Environment and isolation

Linux amd64, Go 1.27.3, Node 24.22.0, pnpm 12.14.0, Wails 3.1.0 and
Chromium 154.0.8037.57.
Native compilation and binding generation used the existing Debian GTK4 /
WebKitGTK development container. The full-stack browser used the actual
production frontend embedded in the Wails HTTP server, real application services
and a new `analytics-linux-qa-v3` / `complete-cny` synthetic household. No real
ledger or backup was used. The seeder used its established 2026-07-26 anchor.

The browser explicitly attempted OS sandbox startup, which failed in this cloud
container. The recorded journey uses the explicit `--allow-no-sandbox` flag.
This is Chromium HTTP acceptance, not native macOS GUI acceptance.

## Final checks

- Frontend lint and locale coverage: passed.
- Generated Wails bindings and TypeScript typecheck: passed.
- Complete local Vitest suite before the final CI-scroll regression: **164 files,
  1695 tests passed**, 265.69 seconds.
- After the CI-scroll fix: all **150 tests** in App, historical capture/overview
  and onboarding suites passed; lint/typecheck/build passed again. The complete
  macOS run [41779322441](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41779322441)
  passed all 1696 tests at `e3d1b76`. Follow-up results are tied to the latest [PR head](https://github.com/MnemosyneLab/NestworthGo/pull/200/checks).
- Production frontend build: passed. Existing large-chunk advisory remains.
- `CGO_ENABLED=0 GOFLAGS='-mod=readonly -tags=server -p=4' go test ./...`: passed.
- Same server configuration, `go vet ./...`: passed.
- `gofmt -l cmd internal`: empty.
- Linux GTK/WebKit native `go build ./cmd/nestworth`: passed, including a rebuild
  after the final frontend bundle. Wails GTK deprecation warnings remain.
- Production HTTP server build after the final frontend bundle: passed.
- `git diff --check`: passed.

An early local run overlapped native compilation and encountered two existing
App deadlines; the subsequent sequential full suite passed without increasing
timeouts. Separately, [original macOS CI 41775024213](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41775024213)
failed one of 1689 tests: the account-analysis return expected scroll 170 and
received 0. The artifact was inspected and a deterministic `flushSync` external
store reproduction failed the same way locally. Restore requests now match the
destination entry/page, survive intermediate commits and restore scroll before
paint; only focus restoration is deferred/cancellable. The original assertion
remains, and both the forced-commit regression and a real overflow browser
journey preserve scroll 170 and the archive filter.

Application regressions include cursor traversal/branch truncation, MRU
semantics, duplicate visits, unsupported one-time intents, repeated explicit
account creation, confirmation gating, deleted accounts, ledger identity reset,
source focus and scroll, actual instrument routes, month-boundary frozen
history, maintenance searches, liquidity request cancellation, and independent
reconciliation entries/drafts. Existing assertions that expected a nested
reconciliation workspace to be destroyed after Back now assert it remains
hidden for Forward, matching the new contract.

## Browser evidence

[Structured results](browser-results.json) record ten passing real-service
journeys and 119 runtime requests. No page errors, application console errors,
or failed application HTTP responses occurred. Chromium's automatic request for
the existing missing `/favicon.ico` is retained separately as a known warning.

The journeys cover empty history, same-page account detail/list navigation,
Back/Forward, branch truncation, Recent's business context and new-visit
semantics, Escape/keyboard focus, list filters, historical capture reuse without
another service call, independent historical headers, 640px width and popup
bounds, all three locales, and clearing history on reload.

- [Desktop Recent](desktop-recent.png)
- [640px desktop Recent](narrow-recent.png)
- [Independent retained Tasks sessions](retained-sessions.png)
- [Historical fork restored to its original date/capture](historical-fork.png)
- [Real account scroll restoration](scroll-restoration.png)

[Review-regression results](revision-results.json) add three passing full-stack
journeys: same-account A/B closing drafts remain reachable after forward
truncation and MRU deduplication; a Recent date-A fork edited to B does not alter
A's date or frozen capture in Recent/Back; first onboarding creates a real
synthetic household and lands on Accounts with empty history. A separate
[scroll journey](scroll-results.json) verifies actual scroll overflow at 170 and
archive-filter restoration. These bring final browser coverage to **14 flows**.

The draft regression also checks the confirmation gate on the recovery menu.
Historical regressions cover private refresh preserving a source capture,
removing an edited visit's obsolete MRU date, pending shared-read handoff, and
zero-time cache release after the last host unmounts.

Only synthetic data appears in these images. The interaction reference was
inspected privately; its unrelated branding/content is not included here.

## Reproduce the browser journey

Follow [real browser acceptance](../../development/real-browser-acceptance.md).
Create a fresh fixture and build the frontend before the server binary:

```bash
QA_ROOT="$(mktemp -d /tmp/nestworth-navigation-qa.XXXXXX)"
NESTWORTH_QA_OUTPUT_DIR="$QA_ROOT/analytics" \
NESTWORTH_QA_SCENARIO=complete-cny \
NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
NESTWORTH_DATABASE_PATH="$QA_ROOT/analytics/data/nestworth.db" \
  go run ./cmd/analytics-qa-seed
```

Launch the production HTTP server using **that fixture's** database and
`analytics/data/settings.json`, isolated XDG paths and localhost port 18741.
Do not run this journey against a personal server: it changes the fixture's
language preference and records screenshots.

```bash
python3 docs/qa/navigation-history-2026-11-04/browser-acceptance.py \
  --qa-root "$QA_ROOT" --base-url http://127.0.0.1:18741
```

For review regressions, launch another final-build HTTP server on port 18742
using a new, explicitly synthetic **empty** database/settings/XDG directory.
This second server must still show Onboarding; do not point it at an existing
household. Then run:

```bash
python3 docs/qa/navigation-history-2026-11-04/revision-acceptance.py \
  --qa-root "$QA_ROOT" --onboarding-url http://127.0.0.1:18742
python3 docs/qa/navigation-history-2026-11-04/scroll-acceptance.py \
  --qa-root "$QA_ROOT"
```

Use `--allow-no-sandbox` only when explicitly accepting and recording that mode.
The script verifies the synthetic fixture marker before exercising the app and
writes results/screenshots beneath the supplied QA root.

## Remaining boundaries

Native macOS window/keyboard/accessibility/VoiceOver acceptance, packaging,
signing and notarization were not run on Linux. Mobile uses its existing app and
navigation; its unchanged tests are included in the full suite. Navigation is
memory-only and does not survive reload/restart or a household change. The
current restore path requires restart. The conservative 20-Tasks-session limit
and unexposed page-local-state limits are described in the
[navigation contract](../../design/navigation-history.md).

## Same-page account restore review

The review edge was reproduced on `e3d1b76` before modifying production code.
Two forced-`flushSync` App regressions failed (900 became 0). A separate real
Chromium fixture uses 61 synthetic accounts and 40 holdings through application
services, with native CSS geometry and the browser's original scrollTop setter.
Both list→short-detail→Back and long-account→short-account→Recent lost position.
The short DOM had scrollHeight/clientHeight 844/844; the restored destinations
were 4367/844 and 4152/844. Writes into the short DOM were clamped to zero.

Restore requests now wait for the destination account ID and filter as well as
entry/page identity. The effect rechecks those props and performs no unsolicited
scroll reset when there is no pending restore. The same native browser paths
restore their captured list offset (1187) and account offset (900) exactly.
See [paired geometry results](same-page-results.json).

Reproduce with `node same-page-build.mjs /workspace/scratch/<fresh-directory>`
from the repository root (use this directory's full script path). This opt-in
builder seeds a fresh synthetic database and adds only a forced shared-store
commit hook to a separate test bundle; it restores production dist afterwards.
Launch its server with explicit isolated database/settings/XDG paths, then run
`same-page-acceptance.py --qa-root <same-directory> --base-url <local-url>`.
The script traces the original native setter without fabricating scroll sizes.
Production builds include no test hook. The prior fourteen browser journeys
were captured on `e3d1b76`; these two additional journeys verify this follow-up.

The concurrent main merge `073e0b7` is preserved. The follow-up is rebased onto
that commit; its final complete CI outcome is recorded in PR #200 for the exact
head. No deadline or scroll assertion was removed or increased.
