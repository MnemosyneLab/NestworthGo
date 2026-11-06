# Mobile and desktop control browser checks

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

Run the retained synthetic browser harness from the repository root:

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

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
