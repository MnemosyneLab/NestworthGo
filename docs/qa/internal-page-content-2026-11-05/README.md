# Internal semantic page reads

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

Run the retained synthetic browser harness from the repository root:

```sh
PLAYWRIGHT_MODULE=/path/to/playwright-core/index.mjs \
PAGE_CONTENT_OUTPUT=/tmp/nestworth-page-content-browser \
node docs/qa/internal-page-content-2026-11-05/browser.mjs
```

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
