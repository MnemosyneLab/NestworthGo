# Real Browser Acceptance

This guide describes how to exercise the real Nestworth frontend and backend
through a cloud browser, without launching a native Wails window. Use it for
reviewable full-stack acceptance of a specific source revision. It does not
replace native GUI acceptance or the project test suite.

**Owner:** Engineering. **Status:** Maintained workflow. **Boundary:** This
verifies the HTTP server build, real application services, browser integration
and isolated persistence. It does not verify native Wails behavior.

The Wails `server` build tag is the supported browser path. It serves the real
embedded frontend and real Go application services over HTTP and WebSocket; it
does not substitute fixture API responses. Keep any custom fixture generator
temporary and call real `application.Service` methods. Never patch application
logic, weaken validation, write directly into SQLite to fabricate app behavior,
or use a production household.

## Prerequisites

Follow the setup and Wails binding instructions in the [Local Development and
Packaging guide](local-workflow.md). For Linux server mode, install Go, Node,
pnpm, and the Wails CLI. Native GTK/WebKit packages are not required for the
`server` build; they are required by the native `wails3 task dev` workflow.

The verified acceptance run for PR101/PR102 used:

| Tool | Version |
| --- | --- |
| Go | `1.26.0` (`linux/amd64`) |
| Wails CLI | `v3.0.0-beta.28` |
| Node | `v24.19.0` |
| pnpm | `12.2.1` |
| Vite | `8.2.1` |
| Python Playwright | `1.62.0` |
| Cloud Chromium | `151.0.7922.173` |

Check each command before starting. In one Linux cloud image, `/usr/bin/go` was
an unrelated Go board-game binary: `go version` returned `Go: Unknown option:
version`. Do not assume the executable name means it is the Go compiler. The
verified Go 1.26.0 archive had SHA256
`aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235`. For that
exact archive on Linux/amd64, an isolated install can be verified and used as
follows:

```bash
TOOL_ROOT=/tmp/nestworth-acceptance-tools
mkdir -p "$TOOL_ROOT"
curl --fail --location --output "$TOOL_ROOT/go1.26.0.linux-amd64.tar.gz" \
  https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
echo 'aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235  /tmp/nestworth-acceptance-tools/go1.26.0.linux-amd64.tar.gz' \
  | sha256sum --check
mkdir -p "$TOOL_ROOT/go1.26.0"
tar -C "$TOOL_ROOT/go1.26.0" -xzf "$TOOL_ROOT/go1.26.0.linux-amd64.tar.gz"
export GOROOT="$TOOL_ROOT/go1.26.0/go"
export GOPATH="$TOOL_ROOT/gopath"
export GOMODCACHE="$GOPATH/pkg/mod"
export GOCACHE="$TOOL_ROOT/go-build-cache"
export PATH="$GOROOT/bin:$GOPATH/bin:$PATH"
go version
```

The project CLI prerequisite is Wails `v3.0.0`, invoked as `wails3`.
When it is not already installed, install the CLI from the matching module
version:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
wails3 version
```

Use the pinned `pnpm` version and check `pnpm --version`; the tested container's
fallback pnpm wrapper was `11.19.0`, so it explicitly used pnpm `12.2.1`. For a
temporary npm-exec installation, verify the pinned package with:

```bash
npm exec --yes --package=pnpm@12.2.1 -- pnpm --version
```

Do not let a mismatched package-manager wrapper silently determine the build.
Use the cloud environment's browser executable. The verified Chromium was
`/usr/bin/chromium`. Playwright's `chromium_sandbox` launch option defaults to
`false`; omitting it does not enable Chromium's OS sandbox. For a sandboxed
run, pass `chromium_sandbox=True` explicitly and verify the launch succeeds
and Chromium reports sandboxing as active (for example, with
`chrome://sandbox`). See the [Playwright launch option documentation](https://playwright.dev/python/docs/api/class-browsertype#browser-type-launch-option-chromium-sandbox).
If the cloud container blocks sandbox startup, keep the failure as evidence;
do not retry by weakening the sandbox and call that run sandboxed. If the
browser checks are then continued with sandboxing disabled, record that mode
in the report and screenshots.

After installing the pinned Go, Wails and frontend tools, run the repository's
documented one-time setup in a clean checkout:

```bash
wails3 task setup
```

## Pin and record the source revision

Start from the intended commit, record the exact SHA, and confirm the branch
relationship before seeding data:

```bash
git status --short
git rev-parse HEAD
git ls-remote origin refs/heads/main
git log -1 --format='%H%n%P%n%s'
```

For a merged pull request, verify the expected merge commit or ancestor with
`git merge-base --is-ancestor <expected-commit> HEAD`. Record the output in the
acceptance report. Do not update or merge source while performing acceptance.

## Create isolated fixtures

Make a unique disposable run directory. Never point environment variables at a
normal Nestworth profile, a production ledger, credentials, or a cloud backup.
For broad analytics screens, the maintained synthetic seeder creates a real
SQLite household through the application layer:

```bash
QA_ROOT="$(mktemp -d /tmp/nestworth-browser-acceptance.XXXXXX)"
mkdir -p "$QA_ROOT/empty-data" "$QA_ROOT/settings" "$QA_ROOT/xdg" "$QA_ROOT/tmp"

NESTWORTH_QA_OUTPUT_DIR="$QA_ROOT/analytics" \
NESTWORTH_QA_SCENARIO=complete-cny \
NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
NESTWORTH_DATABASE_PATH="$QA_ROOT/analytics/data/nestworth.db" \
go run ./cmd/analytics-qa-seed
```

The seeder writes its database to `$QA_ROOT/analytics/data/nestworth.db` and
describes the deterministic accounts, instruments, dates, and expected cases
in [`cmd/analytics-qa-seed/README.md`](../../cmd/analytics-qa-seed/README.md).
Use one untouched database per run. `NESTWORTH_QA_RESET=1` can replace an
existing database only under the seeder's approved QA root; prefer a new
`QA_ROOT` instead of resetting an earlier run.

For maintained investment-review daily fixtures, use the dedicated seeder with
a new target directory:

```bash
EVAL_DATE="$(date -u +%F)"
NESTWORTH_DATABASE_PATH="$QA_ROOT/review/household.db" \
  go run ./cmd/investment-review-qa-seed -dir "$QA_ROOT/review" -date "$EVAL_DATE"
```

That helper creates a synthetic household and flat/loss/missing/distribution/
stale NAV cases. When a needed scenario is not available in a maintained
seeder, a temporary harness may onboard and seed through real
`application.Service` APIs (for example, start History before adding events,
then import synthetic market data). Keep it out of the permanent source tree
afterward; do not bypass service validation or fabricate repository state with
SQL.

For the PR101/PR102 cash-effect replay finding, a maintained docs-only
reproducer creates a minimal historical-review household through real
`application.Service` methods, then drives the rule preview through the actual
browser. See [`docs/qa/post-merge-pr101-pr102-2026-10-17/repro/README.md`](../qa/post-merge-pr101-pr102-2026-10-17/repro/README.md).

## Build and start the real HTTP backend

The root `Taskfile.yml` exposes `run:server`. The task builds the frontend,
generates the Wails TypeScript bindings, builds `cmd/nestworth` with the
documented server tag, and runs the actual application. It does not start a
native window. With the chosen synthetic fixture, set app state paths to
isolated locations:

```bash
export CGO_ENABLED=0
export GOFLAGS='-mod=readonly -tags=server'
export NESTWORTH_DATABASE_PATH="$QA_ROOT/analytics/data/nestworth.db"
export NESTWORTH_SETTINGS_PATH="$QA_ROOT/settings/settings.json"
export XDG_CONFIG_HOME="$QA_ROOT/xdg"
export TMPDIR="$QA_ROOT/tmp"
export WAILS_SERVER_HOST=127.0.0.1
export WAILS_SERVER_PORT=18724
wails3 task run:server
```

Use a different free `WAILS_SERVER_PORT` for each simultaneous fixture. Check
that the live backend is healthy and serving the application:

```bash
curl --fail --silent --show-error http://127.0.0.1:18724/health
curl --fail --silent --show-error -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18724/
```

Expected: the health response is `{"status":"ok"}` and the frontend returns
HTTP 200. If using a prebuilt server binary for parallel isolated runs, build it
from the same checked-out source with the repository's `build:server` task and
pass distinct database, settings, XDG and port values to each process.

## Drive the cloud browser

Open the app at `http://127.0.0.1:<port>/` in the execution environment's
Playwright-controlled Chromium. Do not use fixture data in the browser itself
or a frontend-only dev server as full-stack evidence. Listen for page errors,
console errors, failed responses and actual Wails transport traffic. A minimal
transport check using Python Playwright is:

```python
from playwright.sync_api import sync_playwright

with sync_playwright() as p:
    browser = p.chromium.launch(
        headless=True,
        executable_path="/usr/bin/chromium",
        chromium_sandbox=True,
    )
    page = browser.new_page(viewport={"width": 1440, "height": 1000})
    runtime_calls = []
    sockets = []
    failures = []
    console_errors = []
    page_errors = []
    page.on("request", lambda request: runtime_calls.append(request.url)
            if request.method == "POST" and request.url.endswith("/wails/runtime")
            else None)
    page.on("websocket", lambda socket: sockets.append(socket.url))
    page.on("response", lambda response: failures.append((response.url, response.status))
            if response.status >= 400 else None)
    page.on("console", lambda message: console_errors.append(message.text)
            if message.type == "error" else None)
    page.on("pageerror", lambda error: page_errors.append(str(error)))
    page.goto("http://127.0.0.1:18724/", wait_until="domcontentloaded")
    page.get_by_role("button", name="History", exact=True).click()
    assert runtime_calls, "No Wails runtime requests reached the backend"
    assert any("/wails/events" in url for url in sockets), "No Wails event socket"
    print({"failedResponses": failures, "consoleErrors": console_errors,
           "pageErrors": page_errors, "runtimeCalls": runtime_calls,
           "eventSockets": sockets})
    page.screenshot(path="/tmp/history.png", full_page=True)
    browser.close()
```

For each critical write, use the normal Preview → Confirm path, reload the
view, and check that the expected result remains exactly once. Capture the
request/response status and screenshot for both expected rejection and recovery
paths. A real backend request is normally a `POST /wails/runtime`; the Wails
event channel is `/wails/events`. These observations, together with the
isolated database path and post-run SQLite integrity check, establish that the
browser is exercising the actual backend.

## Scenario checklist

Choose the scenarios relevant to the changed code. Report actual observations
and separate browser coverage from unit/regression coverage.

- Onboard a synthetic household and inspect empty History and accounts; start
  History before backdated data if the scenario needs an earlier origin.
- Add accounts and synthetic holdings. Use positive, negative, zero and
  missing-quote or missing-FX data to check presentation boundaries.
- Record a contribution, buy with fee, sell with fee, dividend and cash
  transfer. Check preview values and final History wording. Exercise invalid
  amount/quote validation, cancel, correction and repeated confirm; verify a
  saved activity remains once after reload.
- Change the analysis period and account/instrument scope. Compare totals at
  the relevant parent and child scope; navigate Analysis → History and use the
  in-app Back action to check context restoration.
- Inspect asset changes, income and return analysis. Check ordinary date
  selection, the future-date boundary, and product fields whose valid dates
  legitimately extend into the future.
- Start History and archive a no-position instrument through the UI, then
  return to the mounted analysis/review views and confirm fresh reads. Also
  check that archiving an instrument with an open position is rejected.
- In investment review, preview a rule with multiple `Meet all` conditions,
  one-hit firing, a three-consecutive-day policy and the supported rolling
  threshold. Check To review/Acknowledged state and explanation/navigation.
  Use real daily quotes and preserve the origin-to-latest date range; record
  how many dates hit versus the threshold rather than describing that as an
  exact-threshold boundary test unless it is one.
- Compare filtered History with export where the OS Save dialog can be
  exercised. If export requires a native dialog, mark it NOT RUN under
  browser-only acceptance.
- Compare visible shared controls (date picker, dropdown, input, button,
  confirmation dialog) across the surfaces touched by the change. Record
  deviations; do not redesign during acceptance.

The permanent regression seeders and tests provide reproducible calculation
and date-boundary data. Use them where appropriate. Do not label a unit test as
browser coverage. If a numeric/date transition has exhaustive unit coverage,
use the browser to verify integration and presentation and say that explicitly.

## Evidence, failure recovery and cleanup

Keep a dated report with at least the tested source SHA, PR ancestry, exact
build and browser commands, tool versions, isolated fixture description,
scenario table (`PASS` / `FAIL` / `NOT RUN`), observed values, screenshots,
console/network failures, reproducible steps, severity and suggested fix.
Name screenshots by feature and state, and link them with paths relative to the
report. Record browser `Back`/`Forward` only if tested; do not infer it from an
in-app navigation button.

After stopping the app, validate the disposable database read-only (for
example, SQLite `PRAGMA integrity_check`) and remove only that run's temporary
directory. Never use broad deletion globs. Preserve the report and selected
screenshots in the repository's QA archive if the review requests retained
evidence.

HTTP-only browser acceptance cannot exercise native window lifecycle, system
menus, OS file dialogs, or native WebView rendering. It can prove the real
application services, frontend/backend transport, user-visible web flow, and
synthetic SQLite persistence. Keep those boundaries explicit in the result.

The worked example for this process is the [PR101/PR102 post-merge browser
acceptance report](../qa/post-merge-pr101-pr102-2026-10-17/REPORT.md).
