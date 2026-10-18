# Historical review replay reproducer

This docs-only harness reproduces the PR101/PR102 investment-review preview
failure with a fresh, synthetic household on the current checkout. `seed.go`
creates onboarding, History, account, instrument, contribution, fee-bearing buy
and daily NAV observations through real `application.Service` methods. It does
not write SQLite rows directly. `preview.py` then enters a two-condition rule
through the real browser UI and records the actual Wails request, response,
console messages and screenshot.

The reproduced timeline is:

| Event | Effective time (UTC) | Created time (UTC) | Data |
| --- | --- | --- | --- |
| History Origin | 2026-10-01 00:00 | 2026-10-18 00:35:19.076 | UTC; empty household |
| Brokerage account | opened 2026-10-01 | 2026-10-18 00:35:37.890 | Created after History Origin; zero opening cash |
| Synthetic fund | — | 2026-10-18 00:35:47.462 | USD NAV instrument |
| Contribution | 2026-10-01 00:10 | 2026-10-18 00:36:21.100 | +USD 2,000 |
| Buy | 2026-10-01 00:20 | 2026-10-18 00:36:24.077 | 10 units; USD 1,000 gross; USD 5 fee |
| NAV import | 2026-10-01 through 2026-10-17 | 2026-10-18 12:00 | 17 synthetic daily closes at USD 100 |

The expected accepted ledger cash is USD 995 after the buy. Previewing
`Holding amount > USD 900` **and** `Cost > USD 1,002` for the synthetic fund
returns HTTP 422 with `history_update_failed` / `historical replay reached a
negative balance`.

## Run it

Use a new temporary directory and the same Go/Wails/browser prerequisites as
the [real-browser acceptance guide](../../../development/real-browser-acceptance.md).
Run the seeder from the repository root:

```bash
QA_ROOT="$(mktemp -d /tmp/nestworth-review-repro.XXXXXX)"
mkdir -p "$QA_ROOT/settings" "$QA_ROOT/xdg" "$QA_ROOT/tmp"

NESTWORTH_DATABASE_PATH="$QA_ROOT/replay/household.db" \
  go run ./docs/qa/post-merge-pr101-pr102-2026-10-17/repro/seed.go \
  -dir "$QA_ROOT/replay"
```

The seeder refuses an existing target directory, pins
`NESTWORTH_DATABASE_PATH` to that new synthetic database, and prints its IDs
and timestamps as JSON. Build the real server from this checkout if
`bin/nestworth-server` is not already available:

```bash
CGO_ENABLED=0 GOFLAGS='-mod=readonly -tags=server' wails3 task build:server DEV=true
```

In the same shell, point the server at the synthetic database and fresh
settings paths. Do not omit or inherit these paths from a personal profile:

```bash
export QA_ROOT
export NESTWORTH_DATABASE_PATH="$QA_ROOT/replay/household.db"
export NESTWORTH_SETTINGS_PATH="$QA_ROOT/settings/settings.json"
export XDG_CONFIG_HOME="$QA_ROOT/xdg"
export TMPDIR="$QA_ROOT/tmp"
export WAILS_SERVER_HOST=127.0.0.1
export WAILS_SERVER_PORT=18734
./bin/nestworth-server >"$QA_ROOT/server.log" 2>&1 &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true' EXIT
for attempt in $(seq 1 30); do
  curl --fail --silent http://127.0.0.1:18734/health >/dev/null && break
  sleep 1
done
curl --fail --silent --show-error http://127.0.0.1:18734/health
python3 docs/qa/post-merge-pr101-pr102-2026-10-17/repro/preview.py \
  --url http://127.0.0.1:18734/ \
  --output "$QA_ROOT/evidence"
rg -n 'history_update_failed|negative balance' "$QA_ROOT/server.log"
kill "$SERVER_PID"
wait "$SERVER_PID" 2>/dev/null || true
trap - EXIT
```

The Python script explicitly requests Playwright's default setting,
`chromium_sandbox=False`, to match the hosted container used for this report.
Pass `--chromium-sandbox` to request `True`; if Chromium refuses to launch,
record that environment limitation rather than retrying with a weaker setting
and calling it sandboxed. The screenshot and `preview.json` are written to the
output directory. A successful reproduction has a real HTTP 422, the
`historical replay reached a negative balance` backend log, no page exception,
and the error in the browser dialog. The missing `/favicon.ico` 404 may also
appear as a console error.
