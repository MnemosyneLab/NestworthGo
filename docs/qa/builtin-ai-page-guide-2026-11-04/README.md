# Built-in assistant page guide acceptance — 2026-11-04

Use the checkout’s pinned toolchain, locked frontend dependencies and generated
bindings. Install Playwright separately and select an available Chromium.
Use isolated synthetic data; never connect these runners to a personal ledger.

Follow [real browser acceptance](../../development/real-browser-acceptance.md).
Create a new maintained synthetic fixture and verify its marker, generate
bindings/build the frontend from `frontend/`, then build the server with
`CGO_ENABLED=0 GOFLAGS='-mod=readonly -tags=server -p=1'`. Start it with explicit
isolated database/settings/XDG/tmp paths and port 18861. Start the opt-in
`TestMobileBrowserFixture` using `NESTWORTH_MOBILE_BROWSER_FIXTURE` below the same
QA root and `NESTWORTH_MOBILE_BROWSER_BIND=127.0.0.1:18862`. It expires after five
minutes; start the browser only after `connection.json` exists. Run
[browser-acceptance.py](browser-acceptance.py) with `--root` and `--desktop`;
the sandbox fallback requires its explicit flag. Stop the temporary services
afterward. Never use a personal server/database for this journey.

Save results outside the repository and record the exact source SHA. Historical
outputs are available in Git history. Run the harness against the intended revision;
browser/component results do not establish native macOS,
accessibility, physical-device or live-provider acceptance. See the
[testing guide](../../testing/README.md) for current verification boundaries.
