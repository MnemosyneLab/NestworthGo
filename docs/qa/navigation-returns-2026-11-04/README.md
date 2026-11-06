# Navigation return entry cleanup — 2026-11-04

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

For browser acceptance, create a fresh root:

```bash
QA_ROOT="$(mktemp -d /tmp/nestworth-navigation-returns.XXXXXX)"
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

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
