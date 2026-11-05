# MCP Wave 1 A baseline

Measurement record for stream A in
[mcp-remaining-work.md](mcp-remaining-work.md). This file is separate from the
headless Go SDK pairing table in
[mcp.md](mcp.md#paired-scenario-measurement-headless-synthetic-data). Those
figures used `payloadRecorder` and an in-process Go client. The figures below
come from an external TypeScript MCP client and from GitHub Actions.

| Item | Verdict |
| --- | --- |
| A1 CI phase timing | PARTIAL |
| A2 Real MCP client measurement | PARTIAL |
| A3 Native macOS verification | BLOCKED |

## Source and environment

MCP server behavior was measured from source `482d1a5b`
(`fix(assistant): remove deleted financial page tool labels`). The branch tip
while this document was written, `0168c86f`, differs from that commit only in
`tools/tests/test_ci_macos.py` (the workflow inventory now includes
`package-macos.yml`). `git diff 482d1a5b..0168c86f -- internal/mcpserver` is
empty, so the server under measurement is the server at `0168c86f`. This
document does not change server code.

Host: Linux x86_64, Go 1.27.3, Node v22.23.4. The repository pins Node 24.22.0
for the app; Inspector 2.10.0 accepts Node >= 22.19.0, and v22.23.4 was the
interpreter that actually ran the client. No macOS SDK, no `com.nestworth.dev`
bundle, and no private Mac worker were available.

Data: a temporary SQLite file under the measurement process directory, household
name `SYNTHETIC wave1 A`, base currency USD, one member `Synthetic owner`,
accounts Checking, Savings, Brokerage, and Credit card, plus one manual
instrument `Example stock`. The process clock was fixed at
`2026-11-05T12:00:00Z`. The market-data registry was empty, so the run made no
provider requests. No real household ledger, credential, or `~/.config/Nestworth`
path was opened.

The server was `mcpserver.Service` in `ledger_write`, loopback Streamable HTTP,
stateless, JSON responses. It reported server name `Nestworth`, version
`3.0.0`.

## A1 — CI

PARTIAL. One check on this branch ran its steps far enough to separate queue,
setup, build, cache, and test time. That run's recorded conclusion is
`cancelled`. The branch tip's own run had not started a step when this document
was written.

### Run that executed steps

| Field | Value |
| --- | --- |
| SHA | `482d1a5bf564895c76f4e41406fb932611de7657` |
| Workflow | `check` / job `test` |
| Run | https://github.com/MnemosyneLab/NestworthGo/actions/runs/42004678818 |
| Job | https://github.com/MnemosyneLab/NestworthGo/actions/runs/42004678818/job/127168340936 |
| Created | `2026-11-05T09:43:10Z` |
| First step | `2026-11-05T09:43:54Z` (`Set up job`) |
| Run conclusion | `cancelled` |
| Annotation | `The run was canceled by @bghtyu.` |
| Checkout | log shows the workspace moved to `482d1a5b` |

GitHub Actions cache restore is off in `.github/workflows/check.yml`
(`actions/setup-go` `cache: false`, `actions/setup-node`
`package-manager-cache: false`). There is no cache-restore step and therefore
no cache-restore duration. The step `Verify native environment and select
persistent caches` selects the self-hosted `GOCACHE`, `GOMODCACHE`, and pnpm
store. On this run its start and completion timestamps are both
`2026-11-05T09:44:01Z`, so that selection was under one second. That duration
is not a hit rate.

Queue is `created_at` until the first step, 44 seconds. A queued job can
already have `started_at` set before any step exists, so `started_at` alone is
not treated as runner execution.

| Phase | What was timed | Elapsed |
| --- | --- | --- |
| Queue | run created `09:43:10Z` → `Set up job` started `09:43:54Z` | 44s |
| Setup | `Set up job` `09:43:54Z`–`09:43:56Z`; checkout `09:43:56Z`–`09:44:01Z`; setup-go, setup-node, pinned pnpm, persistent-cache selection, and pinned Wails CLI all timestamped within `09:44:01Z` | about 7s through Wails |
| Build | pnpm install `09:44:01Z`–`09:44:02Z` (1s); Wails bindings `09:44:02Z`–`09:44:05Z` (3s); frontend production build `09:44:05Z`–`09:44:08Z` (3s). Inside the lanes, the Go `build` lane reported 5.675s and does not add to the wall clock below | install + bindings + frontend build = 7s, overlapping the 5.675s Go build lane |
| Cache | no Actions cache-restore interval; persistent-cache selection < 1s | no restore time to report |
| Test | `Run independent correctness lanes` `09:44:08Z`–`09:46:44Z` | 156s wall |

The lanes overlap. Their own reported seconds, first start/end pair only:

| Lane | Outcome | Seconds |
| --- | --- | ---: |
| go-tests | success | 145.927 |
| frontend-tests | success | 125.308 |
| tooling | failure | 123.13 |
| lint | success | 22.029 |
| typecheck | success | 14.621 |
| build | success | 5.675 |
| vet | success | 1.667 |
| format | success | 0.432 |
| whitespace | success | 0.055 |

The failing assertion is `AssertionError: Items in the first set but not the
second: 'package-macos.yml'` in the workflow inventory test. The gate step
`Require every correctness gate` ended `failure` at `09:46:44Z`. Its JSON
lists `tooling` and `lanes` as `failure` and every other gate, including
`go-tests`, `build`, `frontend-tests`, and `go-package-accounting`, as
`success`. Artifact upload then ran `09:46:44Z`–`09:46:50Z` (6s). The job
object's `completed_at` is `09:47:05Z` with conclusion `cancelled`.

`0168c86f` adds `package-macos.yml` to that inventory assertion. It does not
change the MCP server.

### Runs that did not produce phase times

| SHA | Run | State at `2026-11-05T09:47:05Z` |
| --- | --- | --- |
| `0168c86f` (tip before this document) | https://github.com/MnemosyneLab/NestworthGo/actions/runs/42004987255 / job https://github.com/MnemosyneLab/NestworthGo/actions/runs/42004987255/job/127170169412 | `queued`, 0 steps. Build, cache, and test elapsed are not available. `started_at` is `09:47:05Z` while status is still `queued`. |
| `13af9c3b` | https://github.com/MnemosyneLab/NestworthGo/actions/runs/42003512505 | `cancelled`, job has no steps |
| `cb6f88ff` | https://github.com/MnemosyneLab/NestworthGo/actions/runs/42003299463 | completed `failure`; same gate shape (`tooling` / `lanes` failure, other listed gates success). Not the tip. |

The Eino provider workflow on `cb6f88ff` succeeded:
https://github.com/MnemosyneLab/NestworthGo/actions/runs/42003299469.
It is not the `check` job.

## A2 — real MCP client

PARTIAL. Discovery overhead, call counts, wire bytes, and tokenizer counts were
measured with a real external client. Both `structuredContent` and text were
present on every successful tool result and compared equal as JSON. Nothing in
this environment forwarded either copy to a model, so the Wave 2 question of
whether a host puts both copies in a prompt is still unanswered.

### Clients

Primary session: `@modelcontextprotocol/client` 2.2.0, the client package
Inspector 2.10.0 depends on. `StreamableHTTPClientTransport`,
`versionNegotiation.mode: "legacy"`. Negotiated protocol `2025-11-25`, era
`legacy`. One session for discovery and all four tasks. Authorization was
`Authorization: Bearer <redacted>` on loopback. The client name sent at
initialize was `wave1-a-typescript-client` version `2.2.0`.

Second client, separate process, not included in the call counts above:
Inspector CLI 2.10.0 (`inspector-cli`), Node v22.23.4, invoked as

```text
--transport http --protocol-era legacy --format json --quiet
--header "Authorization: Bearer <redacted>"
```

That is the CLI form of the documented `"type": "http"` config. The catalog and
client-config paths were pointed at the temporary run directory. Inspector was
not given a model, and this script was not given one either.

Tokenizer: `gpt-tokenizer` 3.4.0. `o200k_base` and `cl100k_base` counts are
sums of per-message counts. They are not a provider bill, not a packed-prompt
count, and not a claim about a model context window. Wire tokens count each
HTTP response body. Combined tokens count `text + "\n" + JSON.stringify(structuredContent)`
for each tool result, then sum those counts.

### Discovery

`listTools()` walked every page. The server listed 90 tools in `ledger_write`.

| HTTP exchange | Count | Response bytes | Max response | o200k | cl100k |
| --- | ---: | ---: | ---: | ---: | ---: |
| `initialize` | 1 | 2,500 | 2,500 | 511 | 511 |
| `notifications/initialized` | 1 | 0 (HTTP 202) | 0 | 0 | 0 |
| `GET` (SSE open) | 1 | 19 (HTTP 405, body `Method Not Allowed\n`) | 19 | 4 | 4 |
| `tools/list` | 6 | 127,287 | 35,950 | 28,469 | 27,239 |
| Discovery total | 9 | 129,806 | 35,950 | 28,984 | 27,754 |

The six `tools/list` response sizes were 19,192, 19,428, 22,970, 18,011,
35,950, and 11,736 bytes. The largest page is under 64 KiB. The 405 is the
client opening a standalone SSE stream against a JSON-only handler. It is not
a tool page.

Server instructions on that connection: 2,300 bytes, 448 o200k / 449 cl100k.
A host-shaped catalog of `{name, description, inputSchema}` for all 90 tools,
as one JSON array: 103,513 bytes, 22,509 o200k / 21,599 cl100k. The same
catalog plus each tool's `outputSchema`: 117,561 bytes, 25,676 o200k / 24,619
cl100k. Those catalog figures are projections of what `listTools()` returned.
They are not the wire bodies, and this run does not show which projection a
model host would send.

### Tasks

The script followed the skill's tool names. It did not ask a model to choose
them. Identity lookups and post-write reads are included. That is why these
call counts are higher than the headless pairing script, which already held
fixture IDs and did not count the same reads.

All 28 `tools/call` results had at least one text block and `structuredContent`.
Canonical JSON comparison was equal on all 28. Where the o200k counts of the
text and of `JSON.stringify(structuredContent)` differ, the difference is
encoding, not a second payload: net worth text was 1,294 o200k and the
structured JSON was 1,288 o200k, and those two values still compared equal.

| Task | Tool calls, in order | Response bytes | Wire o200k / cl100k | Text o200k | Structured o200k | Both-in-one-message o200k |
| --- | --- | ---: | --- | ---: | ---: | ---: |
| Check net worth | `get_financial_context` | 8,938 | 2,658 / 2,723 | 1,294 | 1,288 | 2,582 |
| Record a buy | `list_accounts`, `list_instruments`, `list_holdings`, `get_account_snapshot`, `get_read_item`, `preview_change`, `commit_plan`, `preview_change`, `commit_plan`, `list_holdings`, `get_account_snapshot`, `get_read_item`, `get_read_page` | 53,621 | 19,934 / 20,275 | 9,690 | 9,690 | 19,391 |
| Reconcile | `list_accounts`, `get_account_snapshot`, `get_read_item`, `get_read_page`, `preview_reconciliation`, `commit_plan`, `get_account_snapshot`, `get_read_item`, `get_read_page` | 32,381 | 11,557 / 11,625 | 5,594 | 5,594 | 11,196 |
| Import fund NAV | `list_instruments`, `create_instrument`, `import_market_data`, `get_market_data`, `scan_data_health` | 8,977 | 3,254 / 3,210 | 1,543 | 1,543 | 3,090 |

Outcomes, synthetic amounts only:

- Net worth, before any write: `get_financial_context` with `asOf: current`,
  household scope, `disclosure: minimal`. `content.summary.netWorth` was
  `{value: "0", currency: "USD"}`. One call. No detail pages were requested.
- Buy: the brokerage had no USD cash component before funding (missing, not a
  stored zero). A holdings trade cannot settle on Checking (`trackingMode`
  rejects it), so the script added `1000` USD to Brokerage with
  `money_added` / `contribution` at `2026-11-04T14:00:00Z`, committed, then
  bought 10 shares of `Example stock` for gross `200` USD plus fee `5` USD at
  `2026-11-04T15:00:00Z`. Both commits returned `succeeded`. Cash afterward was
  `795`. The holding quantity was `10`.
- Reconcile: Savings was read as `0`, `preview_reconciliation` targeted `1250`
  USD, and `commit_plan` returned `succeeded`. The following read was `1250`.
  The preview expanded to one command.
- Fund NAV: `list_instruments` for `Example fund A` returned no row.
  `create_instrument` created a CNY `mutual_fund` with `quoteSource: agent`.
  `import_market_data` stored one unit NAV, `1.0197` on `2026-09-28` Asia/Shanghai,
  from the synthetic title `Synthetic issuer unit NAV publication`. The receipt
  reported `currentValuationStatus: updated` and
  `snapshotStatus: rebuilt_check_health`. `get_market_data` returned unit price
  `1.0197` in CNY. `scan_data_health` returned `delivery.selectionComplete: true`.

### What the clients did with the two copies

The TypeScript client handed this script both `content[].text` and
`structuredContent` on all 28 calls. The script read `structuredContent` to
continue the task. It did not send text or `structuredContent` to a model.

Inspector `tools/list` exited 0 and wrote a JSON object whose only top-level
key was `tools`, with 90 tools and no `nextCursor` (the CLI aggregated pages).
That list payload is not the wire log above.

Inspector `tools/call` of `get_financial_context` exited 0. The result object's
top-level keys were `content` and `structuredContent`. One text block. Canonical
JSON of the text and of `structuredContent` matched. This call ran after the
four tasks, on the already modified synthetic ledger. Its `netWorth` was
`{currency: "USD", status: "incomplete", value: null}`. That is not the
pre-write `"0"` result. It is evidence about the result shape only.

Inspector `--strict` on `tools/list` exited 6. Stderr contained 47 lines
starting with `Error:` and 271 starting with `Warning:`. One warning is the
known portable-schema note: `analyze_period` input
`query.accountId` uses `type: ["null", "string"]`. One error is
`acknowledge_investment_review_alert` output schema using bare `true`. Those
counts are Inspector's portability report. They are not a failed ledger call.

No Cursor, Claude, or other model host was configured against this server.
Wave 2 item G still has no evidence that a model sees both copies, one copy,
or only a projected catalog.

## A3 — native macOS

BLOCKED. This environment is Linux x86_64. It does not have the macOS 27 SDK,
a `com.nestworth.dev` / `Nestworth.app` bundle, or a private Mac worker. The
isolated native QA steps in
[local-workflow.md](local-workflow.md#isolated-native-development-qa) were not
run. No MCP connect or write through the native app was attempted, and no
result is invented here. The self-hosted macOS Actions job above is the
repository `check` workflow. It is not that isolated app QA.

## What this record does not claim

- It does not replace the headless pairing byte table, and it does not restate
  those before/after comparisons.
- Wire bytes and `gpt-tokenizer` counts are not model-token savings.
- The script's tool sequence is not evidence of which tool an agent would pick
  for net worth. C3 still needs an actual agent choice.
- Native macOS behavior, latency percentiles, allocation, and RSS were not
  measured. The HTTP `ms` samples from this single run are not reported as
  timings.
