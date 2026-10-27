# Investment workflows acceptance — 2026-10-27

Base: latest `main` at `0621583b03d3146a6a9bbe82369bd96415a46c36`.
Evidence applies to the implementation delivered with this report. The PR records
its final commit. All fixtures and retained screenshots are synthetic. The private
Library screenshots were not materialized or copied; repository UI and the supplied
description provided the reference.

## Environment and automated checks

Linux amd64; Go 1.27.3, Wails 3.0.0, Node 24.19.0, pnpm 12.2.1,
Chromium 151.0.7922.173, Python Playwright. Node/pnpm are the supplied environment
versions, below the guide's 24.22.0/12.14.0 pins; pinned-version reproduction is
not established by this run.

`source /workspace/.nestworth-tools/env.sh` selected the working Go/cache tools.
`CGO_ENABLED=0`, `GOFLAGS='-mod=readonly -tags=server'` and a writable
`XDG_DATA_HOME=/workspace/.cache` were used. The initial default binding attempt
failed for missing GTK4/WebKit development libraries; the supported server build
then generated bindings successfully. This is Linux HTTP/server evidence, not
native Linux or macOS desktop acceptance.

| Check | Result |
| --- | --- |
| `wails3 task check` | PASS |
| Production frontend build | PASS; existing >500 kB chunk warning remains |
| Generated bindings freshness | PASS |
| `go test ./...` | PASS |
| `go vet ./...`, `go build ./cmd/nestworth` | PASS with server tag |
| Frontend lint and locale coverage | PASS |
| Frontend typecheck | PASS |
| Frontend suite | PASS — 149 files, 1,538 tests |
| gofmt and `git diff --check` | PASS |
| Race/performance suites | NOT RUN; not the ordinary PR gate |
| Native macOS GUI, package/signing/release/deployment | NOT RUN |

Initial full checks exposed the new table's absence from the certified schema
allowlist and the old backup fixtures' schema22 assumptions. The final change
includes the priority table in the complete schema contract and row seal,
preserves explicit schema21/22 fixtures, and passes the full checks. Old UI
assertions were updated for the intentionally repeated aggregate/member figures
and structured evidence. Necessary navigation and value assertions remain.

## Browser checks

The real `cmd/nestworth` HTTP server served the production bundle on loopback
port 18726. Runtime requests used `/wails/runtime`, not fixture responses.
The isolated database was `/tmp/nestworth-req-full3/household.db`, with a separate
settings/XDG directory and verified household marker **Synthetic four-feature
acceptance**. A temporary application-service seeder created two owners, two
institutions, USD/SGD investment accounts, a JPY balance account, a shared fund,
an ETF, a missing-quote position, a zero position, dated Agent observations and
three priority rules containing the same two performance metrics. It used normal
onboarding, holding, history, quote-import, Record and review APIs, never direct
financial SQL. The temporary seeder was removed from the source tree.

Chromium with `chromium_sandbox=True` failed because the container's SUID sandbox
helper was not configured. No system security settings were changed. The actual
run explicitly used `chromium_sandbox=False`; it is not sandboxed-browser evidence.
Screens were exercised at 1280×960 and 390×844. Dark theme tokens were activated
in the browser; this does not establish native theme-setting persistence.

| Scenario | Result |
| --- | --- |
| Deselect all; repeated filtered select; disable duplicate bulk action | PASS |
| Combine owner, account type and institution; empty results; clear filters | PASS |
| Preserve hidden selections; show total, visible and hidden selected counts | PASS |
| Partial selection and reconciliation preview → Escape → restored selection | PASS |
| Identify two account members with owner/institution and all backend figures | PASS |
| USD-denominated fund in a SGD-default account retains USD figures | PASS |
| Account entry and Back restore the instrument's aggregate/member scope | PASS |
| Shared fund and ETF show available annualized price/NAV changes | PASS |
| Missing quote remains unavailable with no invented annualized result | PASS |
| Zero/closed position reachable with Show zero holdings | PASS |
| P0/P1/P2 visible in priority order; existing P2 editor opens as P2 | PASS |
| Rule-editor Escape closes without saving | PASS |
| Summary and daily history distinguish 7/30-day comparisons and observations | PASS |
| OR rule hit contains an explicitly non-matching condition | PASS |
| Light/dark and narrow layouts; no document horizontal overflow | PASS |
| Page exceptions / failed HTTP responses | 0 / 0 (85 runtime calls in main run) |
| Final isolated SQLite integrity / foreign-key check | `ok` / no violations |

The retained screenshots contain only the synthetic household:

- [Per-account holdings](holdings-light.png)
- [Condition evidence in a narrow dark window](review-dark-narrow.png)
- [Account selection in a narrow dark window](reconciliation-dark-narrow.png)

## Calculation and compatibility boundaries

Bank investment products, mutual funds and ETFs share explicit eligibility for
simple annualized price/NAV change: interval percentage × 365 / actual civil
days. Review retains its existing carry/freshness/baseline-gap policies; details
end at the latest actual quote. Both use the existing exact-decimal projection.
Invalid dates, missing history, zero baselines and unsafe evidence remain
unavailable. This is not personal holding-period performance or a product yield.
Existing cost replay remains authoritative for multi-trade, dividend, fee and
transfer accounting. Account figures are not recomputed or converted by the UI.

`warning` maps to P1, `notice` to P2, and P0 is highest. Old API aliases and
receipt request hashes remain accepted. Schema23 adds a separate priority table;
schema21/22 upgrades retain verified read-only recovery copies. Priority edits
retain condition versions and invalidate the projection through the ordinary
revision/CAS path. Alert grouping/status filters are retained; groups and rules
sort by handling status, priority and stable identity. Pagination holds only a
bounded page-plus-one result and carries its complete ordering key. No benchmark
or broad performance improvement is claimed.
