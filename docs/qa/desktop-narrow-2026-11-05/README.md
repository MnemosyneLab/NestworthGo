# Narrow desktop layout regression

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

Run the retained synthetic browser harness from the repository root:

```sh
PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs \
CHROMIUM_PATH=/usr/bin/chromium \
QA_OUTPUT=/tmp/nestworth-desktop-narrow \
node docs/qa/desktop-narrow-2026-11-05/browser.mjs
```

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
