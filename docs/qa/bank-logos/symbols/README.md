# Bank symbol presentation review

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

Install Playwright outside the application (no runtime dependency is added),
then run Vite on port 9245. From the repository root, temporarily copy
`review.html` to `frontend/bank-review.html` and `review.tsx` to
`frontend/src/bank-review.tsx`. Create `/tmp/nestworth-bank-review` and run:

```sh
NODE_PATH=/path/to/playwright/node_modules node docs/qa/bank-logos/symbols/capture.cjs after
NODE_PATH=/path/to/playwright/node_modules node docs/qa/bank-logos/symbols/verify.cjs
```

The capture script renders both viewport sizes and themes. Capture `before`
from a checkout of `58e1630` with the same temporary harness, then run
`compare.cjs` to assemble the two comparison images. Remove both temporary
frontend harness files after reviewing; they are not production routes.

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../../testing/README.md) for current verification boundaries.
