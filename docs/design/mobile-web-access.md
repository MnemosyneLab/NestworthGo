# Private mobile web access

Nestworth serves a separate mobile React entry point from the existing desktop
process. It uses the same application service, write coordinator, providers and
SQLite owner as Wails and MCP. The feature is off by default. No database is
opened by the HTTP adapter, browser, proxy or second process.

## Setup and lifecycle

Desktop **Settings → Mobile access** has one enable switch and two inputs:

1. **LAN listen address**, e.g. `192.168.1.20:9443`. Use the computer's actual
   private IP and a free port. IPv6 ULA uses brackets, e.g. `[fd12::1]:9443`.
2. **Mobile token**. Set 8–64 ASCII letters/digits, or choose **Generate token**
   for a ten-character random token. Apply saves the draft and signs out phones.

On a phone on the same LAN, open `http://192.168.1.20:9443` (or scan the
address-only QR), enter the token, and use the mobile page. There is no phone
backend-address input or second pairing step. The app must stay running and
the computer awake; closing the last window exits Nestworth.

Access stays off by default. Listeners require a concrete private/loopback
numeric IP and port 1–65535; public, wildcard, unspecified, DNS and scoped IPv6
addresses are rejected. Loopback is useful for local testing but is unreachable
from another device. No network, firewall, router or device security settings
are changed. HTTP sends the token, session credential and financial data in
plaintext. It is intended only for a trusted LAN, with no confidentiality claim.

Apply closes the old listener and revokes sessions, previews and cursors,
including when the token or address changes or access is disabled. Restart
retains the shared token but drops sessions; phones sign in again with the same
token. Restore closes access and cancels/joins mobile provider work before data
replacement. Shutdown uses the existing application/database shutdown path.

## Token and browser session

The ten-character generator uses `crypto/rand` and base32 (50 random bits).
Custom eight-character tokens are accepted for convenience; their strength
is chosen by the user, and a short predictable value has no guaranteed entropy.
The existing login limit is five attempts per source IP per minute. No additional
configuration is required. This limit reduces online guessing, but cannot
protect a token observed on the unencrypted network.

`mobile-access.json` beside presentation settings is now version 2 and contains
`version`, `enabled`, `listen` and `token`. The token is persisted in plaintext
so desktop settings can show/edit it, using an atomic replacement with mode
0600. It is separate from MCP credentials and is not part of Nestworth backup
archives (database plus presentation settings). OS-level copies of the app's
settings directory may include it. Sessions and CSRF credentials remain memory
only. No token is put in URLs, QR codes, logs or mobile persistent storage.

A successful token login returns a fresh 32-byte random bearer credential and
CSRF value. The shared token remains reusable; it is not an API bearer token.
Sessions expire after seven days or 24 hours idle, with at most five active
sessions. A new login replaces the least recently used inactive session when
that limit is reached, so page reloads do not permanently exhaust the slots.
All logins receive the closed mobile API's read, daily-record,
market-refresh and supported manual-observation capabilities. Per-device names,
permission choices and desktop device-management UI were removed. Phone sign-out
revokes that session; applying settings signs out all phones.

The phone always calls relative same-origin routes with `credentials: omit`;
there are no cookies or selectable transport modes. Page reload requires the
shared token again. In-page **Reconnect** checks the existing session while
preserving drafts and immutable financial confirmations. Supplied Host and
Origin must match the address derived from the listener (port 80 is normalized
for browser headers). Mutations require Origin, `X-Nestworth-Mobile: 1`, JSON
content type and the session's CSRF value. Cross-origin requests and preflights
are refused. There is no CORS, trusted-forwarded-header or anonymous connection
probe implementation.

API responses use no-store. Static assets retain the same-origin CSP, no frames,
objects, remote scripts or financial service worker. Hash-named JS/CSS are cached
and gzip is supported. Existing bounds remain: 32KiB request bodies,
500-character notes, 100-item maximum pages, four concurrent requests per session
and bounded login/read/write rates. Durable audit links use operation/session
identifiers rather than tokens or full request bodies.

Only language and appearance preferences enter localStorage. Financial drafts
and query data remain in memory. Active drafts guard page exit and sign-out;
unknown confirmations retain their original recovery identity. The browser uses
`crypto.getRandomValues` for operation UUIDs, which works without a secure context;
it does not depend on `crypto.randomUUID`, clipboard APIs or service workers.
A request timeout/cancel cannot prove a submitted confirmation failed.

## Removed capabilities and migration

Direct TLS serving, PEM certificate/key loading and validation, HTTPS proxy
mode/forwarded-header trust, separate frontend authorization/CORS, saved backend
selection and QR backend hints are removed. There is no supported HTTPS or
separately hosted mobile frontend path. Existing hosted frontends are not changed
or redeployed. Old tokens from the one-use pairing flow cannot be used here.

Existing unversioned configurations, including old HTTP LAN configurations,
load **disabled**, retain only their listen address as a draft, and show a
localized migration message. The original file is left unchanged until the user
applies settings; then it is replaced by version 2 without origin, mode,
certificate, key or frontend-origin fields. Invalid saved JSON also stays off.
No certificate files, ledger data, database schemas, provider settings or
presentation preferences are modified by this migration. Future unknown versions
also stay disabled. Version-2 enabled configurations resume with their saved
token and a fresh empty session set.

Retired phone server preferences are ignored. Requests always use the page's
own server, and old per-server recovery references are not redirected to a new
household. Same-origin recovery references keep their existing key and can be
checked after signing in again.

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
operation IDs go into sessionStorage, in the page origin’s sessionStorage.
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
