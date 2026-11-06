# Selected icon visual QA

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

Reproduce by temporarily copying `review.tsx` to
`frontend/src/coverage-review.tsx` and `review.html` to
`frontend/coverage-review.html`, starting Vite on 9245, making
`/tmp/nestworth-icon-review`, then running `review.cjs` with Playwright available
through NODE_PATH and Chromium at `/usr/bin/chromium`. Remove the temporary
frontend files afterwards. These are component screenshots, not native Wails
acceptance. The real directory and account edit Escape checks are documented
in [the bank regression evidence](../bank-logos/symbols/README.md).


Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
