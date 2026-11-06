# PR98 integration and verification evidence

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

The retained `pr98-pr99-browser.html`, `.tsx` and Playwright runner are historical synthetic component fixtures. The runner has environment-specific Playwright/output paths and expects a preview server on port 4178. It is not a portable current acceptance command. Use current App and investment-review tests and the [real-browser workflow](../../development/real-browser-acceptance.md) for new verification. Retain this fixture when changing its owning tests.

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
