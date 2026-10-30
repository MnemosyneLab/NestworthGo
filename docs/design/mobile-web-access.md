# Private mobile web access

Nestworth serves a separate mobile React entry point from the existing desktop
process. It uses the same application service, write coordinator, providers and
SQLite owner as Wails and MCP. The feature is off by default. No database is
opened by the HTTP adapter, browser, proxy or second process.

## Connection and lifecycle

Desktop **Settings → Mobile access** configures the listener and authorizes
individual browsers. Connection settings are nonsecret and stored in
`mobile-access.json` beside the presentation settings file. Tokens, session
secrets and CSRF values are memory-only. The app must remain awake and running;
closing its last window retains the existing application exit behavior.

Supported boundaries:

| Mode | Listener | External address and prerequisites |
| --- | --- | --- |
| `tls` | An explicit private or loopback IP and port | HTTPS origin; absolute PEM certificate/key paths. The leaf must be current and match the origin hostname. The phone must trust its chain. |
| `loopback_proxy` | Explicit loopback IP and port | A trusted same-machine HTTPS proxy, reachable only through the owner's private network. It preserves the configured Host and browser Origin and supplies `X-Forwarded-Proto: https`. |

The external origin is an exact `https://host[:port]`, without a path, query,
fragment or credentials. Wildcard, unspecified and public listener addresses
are rejected. Public IP origins are rejected. A DNS name and a loopback proxy
do not themselves prove private reachability: network access restrictions and
trusted HTTPS termination remain external prerequisites. The app does not
configure a VPN, firewall, public ingress, certificate authority or daemon.

Changing connection settings closes the old listener and revokes its sessions,
grants, plans and cursors. Restore also cancels and joins mobile provider work
before replacing data. Shutdown closes access, quiesces application writes,
cancels provider workers, and then follows the existing database shutdown path.

## Browser authorization

Each desktop pairing action generates a 32-byte random token, displayed once.
It expires after ten minutes and is consumed atomically on successful exchange.
It is separate from MCP credentials and must be pasted into the HTTPS login
form, never a URL. Sessions have a seven-day absolute and 24-hour idle lifetime;
app restart requires pairing again. Up to five sessions are active at once.

The session cookie is `__Host-nw_mobile`, with Secure, HttpOnly, SameSite=Strict,
Path=/ and no Domain. Only digests of authentication secrets are retained by the
server. Permissions are `read`, `record`, `market_refresh`, and `manual_quote`;
read is mandatory, and manual observations require an explicit desktop choice.
Desktop device management includes individual/all revocation and last-use,
expiry and permission information.

The configured Host and any supplied Origin must match exactly. Mutations also
require a matching Origin, `X-Nestworth-Mobile: 1`, JSON content type, and a
session-bound `X-CSRF-Token`. Login exchange requires the Origin and custom
header before a session exists. The adapter has no credential CORS, debug,
filesystem, provider-settings or generic RPC surface.

API responses use no-store. Static assets have a same-origin CSP, no frames,
objects, remote scripts or financial service worker. Hash-named JS/CSS can be
cached and are served with gzip when accepted. Limits include 32KiB request
bodies, 500-character notes, 100-item maximum pages, four concurrent requests
per session, and bounded login/read/write rates. Logs and durable audit links
contain request/device/operation identifiers, not tokens, cookies or full bodies.

## Closed HTTP surface

All routes use `/mobile-api/v1` and the current server-owned household.

| Routes | Purpose |
| --- | --- |
| `POST/GET/DELETE /session` | Exchange token, read session/CSRF, sign out |
| `GET /bootstrap`, `/overview` | Capabilities, ledger time, valuation, bounded trend and distribution |
| `GET /accounts[/{id}]`, `/holdings[/{id}]` | Bounded account/holding views and independently nullable cost/gain |
| `GET /activities[/{id}]` | Filtered, stable-cursor ledger and readable business detail |
| `POST /record-previews`, `/record-confirmations` | Reviewed daily financial commands |
| `GET /operations/{id}` | Authorized durable receipt recovery; absence remains unknown |
| `GET /health` | Cached/versioned local scan, explicit rescan, bounded findings |
| `POST /quote-previews`, `/quote-confirmations` | Supported manual price/NAV/FX append |
| `POST /market-previews`, `/market-jobs` | Latest refresh or explicit instrument/FX history work |
| `GET /market-jobs/current`, `/market-jobs/{id}` | Process-local job status and bounded details |

Reads carry ledger timezone, as-of and opaque data version metadata. Cursors
bind the session, filters, stable position and application version. A changed
version requires reloading the first page. The application read gate includes
any existing derived maintenance and captures the version atomically on release.

Amounts, quantities, rates and prices remain decimal strings. Missing values
remain null rather than zero. The browser formats authoritative results and
does not sum, convert currencies or calculate financial returns. Civil input
uses the history timezone. Explicit `HH:MM` and `HH:MM:SS` are accepted, sharing
domain DST rejection. Unedited record time is resolved and frozen by the server.

## Daily recording and recovery

The closed mobile command set covers income, expense, transfer, card purchase,
card refund, debt principal/fees and current balance calibration. It maps to
existing domain commands and rejects unsupported, archived or manual-value
endpoints. Cross-currency transfers require both actual amounts; principal is
not expense, card refunds are not income, and calibration is remeasurement.
History must already be started on desktop.

Previews retain an immutable server command and show the reviewed before/after
effects. Confirmations submit only the plan and operation identifiers. Ordinary
records use guarded CAS and the existing atomic activity mutation receipt;
cash calibration reuses the existing absolute-current-cash plan. The mobile
mutation namespace is bound to the household. Receipt replay precedes expiry
or stale-preview refusal. Durable mobile audit association is committed in the
same activity transaction.

Manual quote previews require the currently selected manual source and ordinary
supported instruments/FX. Facts and a typed receipt are committed atomically in
the existing private configuration-backed business receipt pattern, without a
schema bump or source switch. Startup, backup verification and receipt reads
check receipt/fact consistency. Derived FX repricing cannot turn a committed
observation into an apparent failed append; recovery may report pending derived
work and health remains the authority for outstanding gaps.

Revocation is checked again inside the application write gate. Its authorization
lease is ordered after that gate and before persistence, so a revoke either
precedes a queued commit or follows an already admitted commit. Restore cannot
invert the authorization/write lock order.

The browser freezes an uncertain submission, retains its operation identity and
offers receipt lookup or retry of that same plan. Only the household and
operation IDs go into sessionStorage. There is no offline write queue, token
storage or persistent financial cache. Missing plans/receipts do not prove that
a write failed. After desktop verification, the user may explicitly discard a
recovery reference; this never saves, reverses or replays a financial operation.

## Health and market work

Local scans do not call providers. Supported manual findings open the relevant
quote form; supported balance findings open calibration. Provider history work
shows its actual supported ranges and prerequisites before confirmation.
Unmanaged configuration, Agent evidence, product and unsupported repairs remain
desktop work. A completed append/job does not automatically mark health clear:
the client rereads the report, including legitimate derived snapshot issues.

Latest refresh and historical sync are separate intents. Global overview refresh
uses latest missing/stale work; global historical repair is not exposed.
Instrument/FX history reuses existing sync planning and jobs. Latest calls across
desktop, MCP and mobile share application admission: equal work can attach,
different work conflicts, and sync/latest cannot bypass each other. There is no
unbounded queue or second provider implementation. Mobile latest jobs retain
progress, partial failure and safe target details; retries use supported scope
and existing freshness/provider limits. Reconfirming an accepted market plan
returns its original acceptance rather than launching another completed job.

Jobs belong to the desktop process, survive the initiating HTTP request, and
are intentionally not reconstructed as completed after restart. A missing job
is unknown; inspect current observations and rescan. Quote effective dates and
fetch/check times remain separate. Fetching an old close never makes it a new
market observation.

## Frontend and validation boundary

`mobile.html` / `src/mobile` are separate from the desktop shell. Canonical
Button, Input, Select, TimePicker and dialog controls are reused. DatePicker's
pure control is shared with a desktop preference adapter; formatters and the
calendar depend on the initialized i18n runtime, not desktop resource loading.
Mobile loads canonical locale resources and shared health/job translations.
All three supported languages retain locale coverage checks.

Mobile uses bounded holding cards rather than the desktop grouped table. This
is an explicit mobile presentation exception: the backend remains the authority
for amounts and no client grouping sums or financial calculations are added.
The initial trend is an accessible bounded date/value list to avoid shipping the
desktop chart engine. A URL-only QR convenience and richer chart presentation
can follow without changing authorization or financial contracts.

Linux builds, unit/jsdom tests and Chromium evidence do not establish native
macOS, iOS Safari, Android Chrome, screen-reader or real soft-keyboard acceptance.
Use the established dev application identity and isolated synthetic ledgers for
those later checks. No real ledger, credential, network configuration or release
artifact was created to validate this implementation.
