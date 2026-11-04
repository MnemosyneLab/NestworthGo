# Remembered mobile login acceptance — 2026-11-04

Source: `fix/mobile-remember-login`, initially based on main `1f23e0f`, now
integrating main `85435e919ef038fe308b081319fd914002c42094` (#204 and #208). This report and
artifacts accompany the implemented source in the same PR. All data, accounts,
tokens and profiles used here were disposable synthetic fixtures.

## Implemented contract

- Unchecked **Remember this browser** explicitly opts in. Only the server's
  random bearer, absolute expiry and opaque selection metadata enter
  origin-scoped localStorage. The
  shared desktop token, CSRF, query data and financial drafts are never saved
  there. The native checkbox is the permitted shared-control exception; there
  is no canonical checkbox wrapper.
- Seven-day absolute expiry and 24-hour idle expiry remain server authorities;
  the form states both, HTTP plaintext, trusted device/network requirements and
  the XSS exposure of browser storage. This adds neither JWT nor cookies/CORS.
- The private 0600 registry stores SHA-256 bearer digests, bounded lifecycle
  metadata and a database/configuration scope HMAC. Its independent random key
  lives only in private access configuration, preventing the leaked registry
  from checking guesses of a weak shared token. Normal restart retains valid
  remembered sessions and rotates CSRF; it never reloads previews or writes.
- Apply intentionally keeps its existing visible **save and sign out phones**
  meaning, including unchanged settings; it rotates the scope key. Sign-out,
  sign-out-all, token change, disable and supported restore durably revoke.
  Restore uses a distinct revocation hook and refuses replacement on storage
  failure. File identity binds permissions to the selected database.
- Each accepted remembered request synchronously saves recent use under the
  existing authorization lock. This adds bounded small-file I/O; no latency or
  throughput improvement is claimed. Write authorization remains inside the
  application gate. No existing financial or HTTP boundary is widened.
- Storage failures do not acknowledge successful durable revocation. They fence
  authorization and return 503, preserving browser credentials for recovery.
  A failed best-effort removal leaves revocation uncertain until storage repair.
  Browser storage denial falls back to page memory with a visible explanation.

## Checks completed

| Evidence | Result |
| --- | --- |
| Final Linux native-container `go test ./...`, `go vet ./...`, `go build ./cmd/nestworth` | Passed, including GTK/WebKit native compilation |
| `go test -race ./internal/mobileapi` | Passed; affected lifecycle/crash/concurrency paths were rerun after the final registry binding change |
| Frontend ESLint and `tsc --noEmit` | Passed |
| Full Vitest | 169 files, 1770 tests passed |
| Final mobile/navigation/assistant integration subset | 12 files, 150 tests passed |
| Production Vite build | Passed; existing large-chunk advisory remains |
| Wails v3.1.0 binding generation | Passed in the Linux native container; generated files are not committed |
| Skill/tool-name consistency and fast tooling | Passed; 97 tooling tests |
| gofmt and `git diff --check` | Passed |

Environment: Go 1.27.3, Node 24.22.0, pnpm 12.14.0, Linux with the existing
`nestworth-native:trixie` GTK/WebKit container. The bare host lacked those native
headers; those initial wrapper attempts failed before generation. A concurrent
source edit also invalidated one intermediate vet import snapshot. The final
frozen-source native test/vet/build sequence completed successfully. Neither
failure was waived or treated as passing evidence.

Backend regressions cover remembered login/restart, fresh CSRF and refusal of
old CSRF, memory-only session non-restoration, exact idle/absolute expiry,
durable recent-use renewal, individual/all revocation, unchanged Apply, token
change, disable, restore fencing, database switch and same-path replacement,
registry corruption and write denial, five-session LRU admission and in-flight
protection, concurrent requests/revocation, and process termination after durable
login/revocation writes. The crash test kills a subprocess after file+directory
fsync; it does not simulate filesystem or hardware power-loss faults. Existing
Host/Origin, cross-origin, CSRF and authorization/write-order tests remain in the
full suite. Existing uncertain-confirmation tests retain original recovery
identities and receipts.

## Browser evidence

Linux Chromium at 390 × 844 passed the eleven assertions in
[browser-results.json](browser-results.json): explicit opt-in, credential shape,
reload, tab close/reopen, browser-process close/reopen using a persistent profile,
503 on initial API validation followed by explicit Reconnect, no automatic
confirmation for a retained pending-operation identity, port isolation, sign-out
clearing storage, and storage-denied login fallback. Before sign-out there was
one shared-token exchange across reload/reopen/reconnect, and zero automatic
confirmation requests. API transport failures and malformed/expired browser
storage additionally have jsdom regressions. Actual desktop-service restart and
revocation persistence have backend integration evidence, not browser-process
restart as a substitute.

[Login and risk disclosure](login.png) · [Restored synthetic session](restored.png)

## Remaining acceptance boundaries

GitHub's native Mac `test` workflow is triggered by the branch push; its exact
head, run URL and observed status are recorded in the delivery message, rather
than predicting success here. Native macOS GUI, actual iOS Safari/Android devices,
real soft keyboards and assistive technology remain unverified. Chromium and
jsdom do not replace those checks. There is no release, merge or real-ledger QA.

Nestworth backups exclude the registry/access configuration. OS/browser-profile
copies and whole-machine rollback are outside that exclusion. This is not a
claim of encrypted local storage or protection from a local settings writer.
Remembering is unavailable on platforms without the implemented durable file
identity; ordinary memory-only login remains available there.

## Follow-up review regressions

The exact initial head `fb5437200e053afe496b2a3581b403a96a68eef5` reproduced both
old-tab 401 and old-tab successful-logout deletion of a newer remembered login
(two failing shared-storage tests before the fix). Credential invalidation now
captures the request/tab bearer and removes only matching immutable v2 entries.
A separate random selector contains no bearer and is never cleared by expiry or
logout, avoiding localStorage's non-atomic compare/remove race and preventing
fallback to old credentials. This includes failed/concurrent publication,
failed anonymous login, same-tab late responses after explicit restore, legacy
selection, expiry cleanup interleaving and delayed storage-event regressions.
Storage events never change the active tab's authorization or replay operations.

[Two-tab Chromium evidence](tabs-results.json) records six checks with actual
shared-origin pages and real server logout/401 responses. The newer login
survived the old tab's successful logout and subsequent old-session 401, including
refreshing the surviving tab; credentials were not logged. Physical mobile
acceptance remains unverified.

A separate Go regression reproduced registry replacement on read admission at
120, write admission at 20 and four active requests despite HTTP 429. Admission
now precedes recent-use renewal and persistence in the existing lock. Regression
checks compare registry file identity/bytes and in-memory idle/last-use values;
a real 120-request HTTP sequence accepts all 120, refuses request 121 and leaves
last-use durably at request 120. The accepted request, concurrency, LRU,
revocation and write-gate tests remain. A lifecycle test run also exposed the
existing explicit-listener-close/Shutdown race; the already-owned `net.ErrClosed`
is now treated as a successful close while all other shutdown errors propagate.

Final local full native Go test/vet/build, frontend full test/lint/typecheck/build,
mobile regressions and focused admission/revocation race checks passed after
these changes. The original PR-head native Mac and Eino CI runs both passed;
the follow-up head's exact CI results are reported in the delivery message.

## Main integration and return navigation

Main `85435e919ef038fe308b081319fd914002c42094` merged automatically without text
conflicts. The integration retains #208's account -> holding -> source account ->
account-list returns and #204's closed page-guide gates. The remembered-login
refresh/remount regression now exercises that complete return path with one
token exchange and zero confirmations; the three-language navigation tests remain.

A separate regression failed on the automatic merge: after `GET /session`
observed a new server incarnation, the new guide provider's React key remounted
the entire financial subtree and discarded an existing reviewed draft. Only the
guide component now also resets on incarnation changes; the provider resets its
subtree only for a different device and already revokes/replaces its host on
principal-property changes. The regression checks
that the exact amount input and Confirm button stay mounted, the amount and
original preview remain, the guide UI is recreated and disabled by the active
financial flow, and no new preview/confirmation is dispatched. No backend
plan, CSRF, preview or permission is reconstructed by this UI retention.

Wails bindings were regenerated for the newly integrated assistant service;
generated output remains uncommitted. Initial desktop tests before generation
failed on that missing generated import and were rerun after generation.

On the integrated source, the final local Go test/vet/native build and frontend
ESLint/typecheck/production build passed. Full Vitest passed 169 files / 1770
tests, and the targeted mobile/navigation/assistant/App subset passed 12 files /
150 tests. Chromium passed eleven checks, including restored-session holding ->
source account -> list returns, with one token exchange and zero automatic
confirmations. The earlier `7d93c71` Mac and Eino runs both passed; the merge
head's new run links/status are reported separately with its exact SHA.
