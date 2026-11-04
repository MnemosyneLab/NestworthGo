# Remembered mobile login acceptance — 2026-11-04

Source: `fix/mobile-remember-login`, based on main `1f23e0f`. This report and
artifacts accompany the implemented source in the same PR. All data, accounts,
tokens and profiles used here were disposable synthetic fixtures.

## Implemented contract

- Unchecked **Remember this browser** explicitly opts in. Only the server's
  random bearer and its absolute expiry enter origin-scoped localStorage. The
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
| Full Vitest | 164 files, 1730 tests passed |
| Final mobile Vitest subset | 6 files, 53 tests passed |
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

Linux Chromium at 390 × 844 passed the ten assertions in
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
