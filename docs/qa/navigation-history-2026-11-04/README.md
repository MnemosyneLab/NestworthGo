# Desktop navigation history acceptance — 2026-11-04

Base: main `1701e5d5d9bb10ec9db846245766bab6f7a740e5`.
The exact frontend source used for final checks and screenshots is recorded in
[frontend-source-sha256.json](frontend-source-sha256.json). These file hashes
must match the reviewed commit; generated bindings and bundles are not committed.

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
- Complete Vitest suite: **164 files, 1689 tests passed**, 272.90 seconds.
- Production frontend build: passed. Existing large-chunk advisory remains.
- `CGO_ENABLED=0 GOFLAGS='-mod=readonly -tags=server -p=4' go test ./...`: passed.
- Same server configuration, `go vet ./...`: passed.
- `gofmt -l cmd internal`: empty.
- Linux GTK/WebKit native `go build ./cmd/nestworth`: passed, including a rebuild
  after the final frontend bundle. Wails GTK deprecation warnings remain.
- Production HTTP server build after the final frontend bundle: passed.
- `git diff --check`: passed.

An early run overlapped native compilation and encountered two existing App
5-second deadlines. Those cases passed in isolation, then all 49 App tests and
the complete frontend suite passed in the final sequential run. No assertions
were removed and no timeouts were raised for this change.

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
journeys and 129 runtime requests. No page errors, application console errors,
or failed application HTTP responses occurred. Chromium's automatic request for
the existing missing `/favicon.ico` is retained separately as a known warning.

The journeys cover empty history, same-page account detail/list navigation,
Back/Forward, branch truncation, Recent's business context and new-visit
semantics, Escape/keyboard focus, list filters, historical capture reuse without
another service call, independent historical headers, 640px width and popup
bounds, all three locales, and clearing history on reload.

- [Desktop Recent](desktop-recent.png)
- [640px desktop Recent](narrow-recent.png)

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
