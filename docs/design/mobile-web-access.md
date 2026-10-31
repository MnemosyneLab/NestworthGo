# Private mobile web access

Nestworth serves a separate mobile React entry point from the existing desktop
process. It uses the same application service, write coordinator, providers and
SQLite owner as Wails and MCP. The feature is off by default. No database is
opened by the HTTP adapter, browser, proxy or second process.

## Connection and lifecycle

Desktop **Settings → Mobile access** configures the listener and authorizes
individual browsers. Connection settings are nonsecret and stored in
`mobile-access.json` beside the presentation settings file. Tokens, session
secrets and CSRF values are memory-only. The computer hosting Nestworth must
remain awake, with Nestworth running; closing its last window retains the
existing application exit behavior on every supported desktop platform.

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
The desktop QR contains only an address: the backend origin for the built-in
page, or the trusted frontend origin with a `#backend=` address hint. It never
contains a pairing token, session credential or durable credential. The browser
removes the hint when displaying connection settings and requires an explicit
save before it changes an existing endpoint.

The session cookie is `__Host-nw_mobile`, with Secure, HttpOnly, SameSite=Strict,
Path=/ and no Domain. Only digests of authentication secrets are retained by the
server. Permissions are `read`, `record`, `market_refresh`, and `manual_quote`;
read is mandatory, and manual observations require an explicit desktop choice.
Desktop device management includes individual/all revocation and last-use,
expiry and permission information.

The configured Host must match exactly. Without `frontendOrigin`, any supplied
Origin must match the backend exactly. Mutations also require an allowed Origin, `X-Nestworth-Mobile: 1`, JSON content type, and a
session-bound `X-CSRF-Token`. Login exchange requires the Origin and custom
header before a session exists. The adapter has no credentialed CORS, debug,
filesystem, provider-settings or generic RPC surface.

API responses use no-store. Static assets have a same-origin CSP, no frames,
objects, remote scripts or financial service worker. Hash-named JS/CSS can be
cached and are served with gzip when accepted. Limits include 32KiB request
bodies, 500-character notes, 100-item maximum pages, four concurrent requests
per session, and bounded login/read/write rates. Logs and durable audit links
contain request/device/operation identifiers, not tokens, cookies or full bodies.

## Separately hosted frontend

Build the repository's real `mobile.html` entry and its assets, and serve them
from a frontend you trust. Hosting an older disconnected/demo adapter does not
activate this protocol; this change does not publish or update any hosted site.
The backend remains on the owner's existing private network. Both endpoints
must use HTTPS trusted by the browser. No public backend, tunnel, certificate
exception, browser flag or system networking change is created by this feature.

1. Configure and verify the private backend HTTPS origin using one of the modes
   above. Keep the desktop running and verify the phone's private-network route.
2. In desktop **Mobile access**, optionally set **Trusted frontend origin** to
   the exact HTTPS origin, such as `https://mobile.example`, with no path or
   trailing slash. Empty preserves same-origin access only. Saving configuration
   revokes sessions, grants, previews and cursors. Trusting a frontend gives its
   JavaScript access to the authorized data and pairing exchange: do not authorize
   arbitrary third-party pages. The single-origin setting has no wildcard mode.
3. Configure the separate frontend host's CSP `connect-src` to permit the exact
   backend HTTPS origin. The built-in backend page keeps `connect-src 'self'`;
   use that page with its own backend, or use the separately hosted build for
   server switching. A host that prevents external connections cannot be fixed
   by entering another address in the app.
4. Scan the address-only QR or enter the backend origin in phone **Connection
   settings**, test it, then choose **Use this server**. Blank uses the current
   site's backend. Paths, credentials, queries, fragments, wildcard hosts and
   plaintext backend addresses are refused. The test is anonymous and exposes
   only a service/version/authentication marker, with no household data.
5. Generate the one-use token on desktop and enter it separately in the phone's
   password field. Cross-origin exchange returns an origin-bound bearer session
   and CSRF token in the response body. They remain only in JavaScript memory:
   no URL, cookie, persistent storage, log, referrer or backup contains them.
   Reloading a separate frontend requires pairing again. In-page **Reconnect**
   validates the current session and reloads reads without replacing a draft.
6. Sign out to revoke the session. Switching servers cancels stale requests,
   drops the local session and financial cache, and requires fresh pairing; it
   does not send credentials to the new server. An old remote session may remain
   until expiry, so sign out before switching or revoke it on desktop. Pending
   recovery references are kept separately per normalized server in sessionStorage.

Cross-origin requests use `credentials: omit`, an Authorization bearer header,
`X-Nestworth-Mobile`, and the session's CSRF value for mutations. Cookie sessions
cannot authorize cross-origin reads; bearer sessions cannot authorize same-origin
cookie mode or another frontend origin. CORS allows only the configured exact
origin, GET/POST/DELETE, and the four explicit headers. There is no
`Access-Control-Allow-Credentials`. Preflights from that authorized origin can
receive the legacy private-network opt-in header; this is not a guarantee of
browser reachability. Secure HttpOnly Strict same-origin cookies remain unchanged.
Redirects are refused, fetches use no-referrer, and no connection probe carries
a session or pairing token.

Browser local-network permission, TLS trust, network routing and host CSP remain
independent prerequisites. See [Chrome's Local Network Access guidance](https://developer.chrome.com/blog/local-network-access),
[MDN CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS) and
[mixed content](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Mixed_content).
Do not assume a public HTTPS page can universally reach private HTTP, or bypass
a browser warning. A fetch rejection cannot reliably distinguish DNS, TLS,
network, CSP and CORS failures. The UI explains those checks without claiming a
domain rejection or confirmed write outcome. HTTP functional tests with isolated
synthetic fixtures do not constitute production HTTPS or phone acceptance.

Only server, language and theme preferences enter localStorage. Financial drafts
and query results stay in memory; page reload or session revocation discards
unsubmitted drafts. Active drafts guard page exit, disable server switching and
sign-out, and require explicit discard. The page owns the endpoint/session
change; an already-open settings panel is also locked while a draft or recovery
reference is active, including re-pairing after session expiry. Unknown
confirmations retain their original recovery identity. A timeout/cancel of an
HTTP request does not prove that an accepted write failed.

## Closed HTTP surface

All routes use `/mobile-api/v1` and the current server-owned household.

| Routes | Purpose |
| --- | --- |
| `GET /connection` | Anonymous service/version/authentication probe |
| `POST/GET/DELETE /session` | Exchange token, read session/CSRF, sign out |
| `GET /bootstrap`, `/overview` | Capabilities, ledger time, valuation, bounded trend and distribution |
| `GET /accounts[/{id}]`, `/holdings[/{id}]` | Bounded account/holding views and independently nullable cost/gain |
| `GET /activities[/{id}]` | Filtered, stable-cursor ledger and readable business detail |
| `POST /record-previews`, `/record-confirmations` | Reviewed daily financial commands |
| `GET /operations/{id}` | Authorized durable receipt recovery; absence remains unknown |
| `GET /health` | Cached/versioned local scan, explicit rescan, bounded findings |
| `POST /quote-previews`, `/quote-confirmations` | Supported manual price/NAV/FX append |
| `POST /market-previews`, `/market-jobs` | Latest refresh or explicit instrument/FX history work |
| `GET /market-jobs/current`, `/market-jobs/{id}` | Shared process-local job status and bounded details |
| `POST /market-jobs/{id}/cancel` | Request cancellation of mobile-owned work; retained observations are not rolled back |

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
operation IDs go into sessionStorage, in a namespace for the selected backend.
If storage is unavailable, the page retains recovery in memory and asks the user
to keep it open until the original result is confirmed. There is no offline
write queue, token storage or persistent financial cache. Missing plans/receipts do not prove that
a write failed. After desktop verification, the user may explicitly discard a
recovery reference; this never saves, reverses or replays a financial operation.

**Confirm and add another** remains an explicit final confirmation. Success clears
amounts, received amount, fees, note and custom date/time; it keeps the chosen
accounts, currencies and operation kind and focuses the amount. A fresh operation
ID and fresh preview are required for the next entry. An uncertain result stays
frozen and retains that continuation choice through same-plan retry or receipt
lookup. Balance calibration keeps the single-entry flow.

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

The phone reads desktop/MCP latest-refresh and history-sync snapshots through the
same task read model. Mobile-owned work adds cancellation requests; external work
is observed without taking ownership of cancellation. Attaching to an equivalent
history task preserves its original creator; attachment cannot grant mobile
cancellation rights. Global desktop history repair can be viewed but remains
desktop-only for retry. The panel reports real
preparing/running/partial/failed/succeeded/cancelled states, phase, counts, readable
targets, bounded domain error codes, history ranges and next-eligibility time.
Provider messages and paths are never returned as task detail. Page errors offer
an explicit status reload. Paging pins the job identity; navigation and reconnect
reread process state without starting work. Hidden pages stop polling.

A retry opens a fresh explicit preview using saved observations and existing
provider/freshness limits (`all` latest retries become `stale`). It is not a
failed-target-only force refresh: eligible work can be reused or retried by the
existing application rules. A lost acceptance response freezes its immutable
plan and offers same-plan retry, which returns the original acceptance. It cannot
be dismissed into a second start while the acceptance outcome is unknown.

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
The net-worth trend is a decorative inline SVG line drawn from the backend's
points, paired with an accessible date/value list ("Show values"), so the
desktop chart engine is not shipped. The desktop address-only QR uses a local
SVG encoder with no remote image or credential service.

The mobile visual language ("Ledger Pocket") lives in `src/mobile/mobile.css`
and reuses the desktop Ledger tokens (`--font-display`, `--color-*`,
`--radius-*`) so light, dark and system themes follow the shared palette. It is
touch-first: a bottom tab bar with a central quick-record action, 44–48 px
targets, safe-area padding, `interactive-widget=resizes-content` for virtual
keyboards, and a sticky bottom action bar for record, quote and market reviews.
The tab bar is hidden while a flow is open so draft protection and the leave
guard stay in control. Below 360 px two-column grids collapse and row icons are
hidden. Motion is limited to short entrance and progress transitions and is
disabled under `prefers-reduced-motion`. Presentation changes must not add
client-side amount math or weaken preview, operation-id or recovery guarantees.

Linux builds, unit/jsdom tests and Chromium evidence do not establish native
macOS, iOS Safari, Android Chrome, screen-reader or real soft-keyboard acceptance.
Use the established dev application identity and isolated synthetic ledgers for
those later checks. No real ledger, credential, network configuration or release
artifact was created to validate this implementation.
