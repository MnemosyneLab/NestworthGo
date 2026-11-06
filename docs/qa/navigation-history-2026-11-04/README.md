# Desktop navigation history acceptance — 2026-11-04

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

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

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
