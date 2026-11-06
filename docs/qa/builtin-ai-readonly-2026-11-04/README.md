# Desktop built-in AI read-only acceptance — 2026-11-04

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

`browser.mjs` mounts real React components with synthetic IPC responses. It does not start a Wails host or call a provider.

```sh
PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs \
AI_BROWSER_OUTPUT=/tmp/nestworth-ai-browser \
node docs/qa/builtin-ai-readonly-2026-11-04/browser.mjs
```

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
