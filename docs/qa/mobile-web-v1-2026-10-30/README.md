# Mobile web v1 implementation acceptance

Date: 2026-10-30. Base: `beea4e366807d84be36d54e20b59effe849eb33d`.
Implementation contract: [private mobile access](../../design/mobile-web-access.md).

The complete 423-line, 50,193-byte canonical design was read before implementation.
Its SHA-256 was verified as
`09881521a184900c3c938e319afe7d4016ed0b0adaa77368c0db83a788579a26`.
The source design was not overwritten. The existing PR179–181 fixes remain in
the branch base.

## Design coverage

| Area | Status | Evidence / boundary |
| --- | --- | --- |
| One database/process; independent mobile entry | Done | Same application Service injected into the closed HTTP adapter; `mobile.html`; no browser Wails transport. |
| Overview, accounts, holdings, ledger | Done | Bounded responses/cursors, filters, native currency/price, nullable cost and separate realized/unrealized gain; trend is a bounded accessible date/value list. |
| Income, expense, transfers, card purchase/refund, principal/fees, calibration | Done | Domain commands, immutable preview, exact before/after detail, guarded CAS and existing atomic ledger receipts. |
| Uncertain submission recovery | Done | Same-plan retry, authorized durable lookup, minimal recovery identity, explicit desktop-verified discard; no automatic new intent. |
| Manual price/NAV/FX and actionable health | Done | Source checks, atomic fact/receipt, receipt integrity/reopen tests, rescan including an empty healthy report. Unsupported/configuration/product/Agent work stays on desktop. |
| Latest updates versus historical sync | Done | Distinct previewed intents, instrument/FX history scopes, progress, dedupe/conflicts, partial failures and bounded task details. |
| Shared provider coordination | Done | Desktop/MCP/mobile latest admission and existing sync jobs cannot bypass each other; no second provider stack or unbounded queue. |
| Global latest-job visibility | Partial | The mobile current-job panel tracks mobile-started/attached work. A separate latest refresh initiated only from desktop/MCP can cause an explicit conflict without its full desktop progress appearing in this panel. |
| Token/session/CSRF/Host/Origin/revocation | Done | Ephemeral synthetic TLS HTTP tests, permission denial, single-use/expiry/restart and both sides of revoke/commit ordering; no persistent real credentials. |
| Desktop settings and conservative defaults | Done | Off by default, explicit TLS/private binding, pairing/revocation and device metadata; sessions end on restart; last-window exit preserved. |
| URL-only QR convenience | Not done | The address is displayed as a usable link; manual token login is implemented. |
| en / zh-CN / zh-TW; narrow, zoom and dark UI | Done for automated/browser checks | Real locale resources, shared controls, 320px and 200% root-font checks, dark screenshot; native screen-reader/soft-keyboard checks remain below. |
| Performance budgets | Partial | Measured cold English JS transfer below 250KiB in local Chromium. The proposed 100-account/1,000-holding/50,000-activity latency and contention benchmarks were not run. |
| Native/real-phone launch acceptance | Not done | No native Mac, iOS Safari or Android Chrome device was available. No packaging, signing, deployment or public exposure. |

## Automated checks

The environment used Go 1.27.3, Node 24.22.0, pnpm 12.14.0 and Wails 3.0.0.
Native Linux compilation used the existing isolated GTK4/WebKitGTK6 container.

| Check | Result |
| --- | --- |
| Full `go test ./...` | Passed twice during implementation; latest application package run 236.587s. Subsequent narrow HTTP/read additions and final new tests received affected checks below. |
| Full `go vet ./...` and native `go build ./cmd/nestworth` | Passed. |
| Mobile/application/SQLite affected tests | Passed, including all daily operations, concurrent same-ID replay, stale CAS, atomic receipt rollback, manual health repair and verified SQLite reopen. |
| Mobile/application targeted `-race` | Passed, including HTTP authorization, revocation ordering and provider job coordination. |
| Frontend full suite | Passed: 157 files, 1,604 tests. |
| Frontend lint/typecheck/build | Passed; final holding-detail and session-ownership additions also received focused type/lint/test/build checks (34 affected tests; a final four-file, 13-test subset also passed). |
| Wails bindings regeneration and consistency | Passed; generated bindings and production bundles are not committed. |
| gofmt and `git diff --check` | Passed. |

An earlier full frontend run had one outdated six-tab assertion and four
five-second timeouts while other heavy work was running. The assertion now
expects the added seventh tab. The affected three files passed serially (207
tests), and the subsequent complete suite passed without raising timeouts or
removing assertions. Existing Wails browser-preview and ECharts zero-size jsdom
warnings remain distinct from application/runtime errors.

The browser checks found and fixed an explicit-time transport mismatch and a
healthy-report null-array crash. Regressions cover second-level/DST handling,
empty reports, lost-response lookup/retry, locale-specific repair entry, known holding cost rendering and stale responses arriving after a new login.

## Actual browser evidence

Chromium 154.0.8037.57 ran headlessly against a real ephemeral HTTPS handler,
application Service and synthetic SQLite ledger. The household marker is
`SYNTHETIC MOBILE TEST`; all accounts, observations, credentials and operations
are synthetic. The self-signed test certificate was accepted only in this
isolated browser fixture. This does not establish phone certificate trust.

The browser completed token login, expense preview/confirm, same-plan replay,
manual FX and NAV appends, health rescan to zero findings, language switching,
holding-price and known-cost inspection, narrow layouts, root-font enlargement and dark mode.
The exact replayed amount stayed `1.2345`; pending recovery storage cleared after
success. There were no page runtime errors. See [machine-readable results](browser-results.json).

The measured English cold JS response bodies totaled 211,500 bytes (approximately 207KiB) over
gzip. This is local transfer evidence, not a real-phone latency, p95, large-ledger
or desktop-contention claim. The 200% check changes root font size in Chromium;
it is not native mobile accessibility zoom or screen-reader acceptance.

- [390px overview](overview-390.png)
- [390px exact record review](record-review-390.png)
- [390px holding with native unit price](holding-price-390.png)
- [320px Chinese health view](health-zhCN-320.png)
- [320px form at 200% root font size](form-200percent-320.png)
- [390px dark form](form-dark-390.png)

## Remaining setup and manual gates

The owner must supply a trusted private HTTPS address, trusted certificate chain
or same-machine private proxy, and suitable private-network access. These are
deployment prerequisites, not automatically performed configuration.

Before enabling a real ledger, complete isolated native Mac launch/exit/restore,
iOS Safari and Android Chrome, soft-keyboard, screen-reader, sleep/reconnect and
real-device trust checks. Use `com.nestworth.dev` and explicit synthetic paths
according to the repository workflow. No result here represents those checks or
authorizes merge, release, public networking or use of a real household ledger.
