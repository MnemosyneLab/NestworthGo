# README screenshot provenance

The three retained illustrations were captured on 2026-10-17–18 from source
`5cb76c849ec0cc5af0c467ac9903aa74a11fb55a`, using Linux Chromium
151.0.7922.173 and the real Wails HTTP server with isolated synthetic households.
They show an older UI and do not establish current or native macOS acceptance.

- [Historical overview](evidence/08-historical-overview.png)
- [Investment assets](evidence/04-investment-assets.png)
- [Available funds](evidence/03-available-funds.png)

The original run was a partial pass: export/native dialogs were not exercised,
and a contribution → fee-bearing buy → review preview reproduced a historical
negative-balance replay failure. That observation applies only to the named
source, not a current bug verdict. The [historical reproducer](repro/README.md)
is retained; its expected-failure assertion is not a current correctness gate.
The original report, request captures and logs remain in Git history.

For new checks use the [real-browser workflow](../../development/real-browser-acceptance.md)
and the current application regression tests. The historical browser ran with
Chromium sandboxing disabled after the container refused sandbox startup.
