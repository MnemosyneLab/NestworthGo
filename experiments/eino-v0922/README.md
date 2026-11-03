# Eino v0.9.22 suitability experiment

**Result: Go embedding is feasible; the pinned Responses adapter is not accepted
for unrestricted ledger writes.** The core runner and existing Nestworth MCP
interoperate, but deterministic tests reproduce two provider-adapter defects.
Do not interpret the green characterization suite as provider safety acceptance.

## Scope and exact baseline

- Date: 2026-11-03, Linux amd64, Go 1.27.3.
- Fetched `origin/main`: `f81d3cd3bc975ded45be7d1014e12f9af89c171a`.
- Harness source commit: `0dc2203` (subsequent commits retain evidence/docs).
- Dedicated branch: `experiment/eino-v0922`.
- This nested module replaces Nestworth with `../..` so tests can import its
  internal application, MCP and JSON packages. It changes no normal application
  dependencies, CI, runners, generated frontend files or product behavior.
- All ledgers, MCP credentials/plans/receipts and checkpoints are synthetic,
  created under `/tmp`; none is included in this directory. No real key, paid
  request, private ledger, GUI launch, release or merge was used.

The requested pins resolve together without an alpha or main override:

| Component | Resolved version |
| --- | --- |
| Eino core | v0.9.22 |
| agenticopenai | v0.2.4 |
| officialmcp | v0.1.1 |
| OpenAI Go SDK | v3.44.0 |
| Official MCP Go SDK | v1.7.0 (Nestworth requires this; adapter minimum v1.6.1) |
| Sonic | v1.15.4 (Nestworth requires this; adapter minimum v1.15.0) |

`go.mod`, `go.sum`, [resolved-modules.txt](resolved-modules.txt) and
[evidence/build-info.txt](evidence/build-info.txt) preserve the actual resolution.
First-party JSON goes through Nestworth's `internal/jsoncodec`; third-party
libraries retain their own codecs.

## Reproduce

Install Go 1.27.3 or newer and run from this directory:

```bash
./run.sh
```

The script sets `GOWORK=off` and `TMPDIR=/tmp`, runs the targeted tests/vet and
prints resolved modules. Initial module downloads need network access; runtime
model and MCP traffic stays on loopback. This environment installed a separately
checksum-verified Go 1.27.3 toolchain under `/tmp/eino-toolchain/go` because its
original `/usr/bin/go` was a different program. Validation used:

```bash
export PATH=/tmp/eino-toolchain/go/bin:$PATH
export GOPATH=/tmp/eino-gopath GOMODCACHE=/tmp/eino-mod GOCACHE=/tmp/eino-cache
export GOWORK=off TMPDIR=/tmp
go test -count=1 -timeout=120s -json ./... > evidence/tests.jsonl
go vet ./...
go test -c -o /tmp/eino-runtime.test
go version -m /tmp/eino-runtime.test
go list -m all
```

A separate command enforces the **unmodified provider's** acceptance criteria and
is deliberately red for these pinned versions:

```bash
EINO_REQUIRE_PROVIDER_SAFETY=1 go test -count=1 -timeout=60s -v \
  -run 'TestRawStreamFailureCharacterization|TestReasoningArrivingOnlyAtItemDone' ./...
```

See [provider-acceptance.txt](evidence/provider-acceptance.txt). Three broken-stream
subcases mutate the synthetic ledger, and done-only encrypted reasoning is lost.
The default tests instead assert and label those observed limitations so future
changes cannot silently erase the evidence.

## Actual integration versus stubs

**Real:** Eino typed ChatModelAgent/Runner with `schema.AgenticMessage`, the pinned
Responses converter, officialmcp tool/schema conversion and reconnecting session,
Nestworth `application.Service`, appports, SQLite schema/repositories, local
Streamable HTTP service, actual previews, commits, receipts and bounded reads.
Tests discover all 103 tools exposed in `ledger_write`; ordinary runner tests
select a small allowlist. Native Wails is not launched.

**Synthetic:** an HTTP/SSE Responses server, invented opaque reasoning payloads,
a deliberately false summarizer, synthetic approval decisions and a transport
that loses a successful commit response. The fake model does not validate real
OpenAI protocol acceptance or reasoning signatures. It is deterministic, not a
model quality assessment. The local completion gate and approval tool are
experiment prototypes, not installed product features.

Every model request is checked for `store:false`, explicit
`include:["reasoning.encrypted_content"]` and no `previous_response_id`.
`EnableAutoCache:false`, fake API key, a local-only HTTP client and zero provider
SDK retries are explicitly configured.

## Results and ownership of guarantees

| Requirement | Observed result | Owner / qualification |
| --- | --- | --- |
| Two tool rounds and streaming | 3 Responses requests, 3 streaming events, 5 total Runner events; actual `get_context` and `get_catalog` results fed back | Eino runner + provider + MCP adapter work together |
| Reasoning continuity | IDs and encrypted contents round-trip when present in `output_item.added`; encrypted contents arriving only at `output_item.done` are dropped | **Provider adapter acceptance fails**; Eino state cannot preserve fields it never receives |
| Discovery/schema/content/errors/auth | 103 tools; required schemas present; structuredContent retained; isError becomes typed `server_tool_error`; wrong bearer rejected | Existing MCP server + officialmcp; set `IncludeStructuredContent:true` explicitly |
| Malformed/truncated stream | Malformed event JSON and incomplete argument JSON produce no ledger mutation; clean EOF after complete arguments, EOF after item.done, and response.failed each still produce one raw write | **Provider adapter acceptance fails**; raw completion status is not an execution gate |
| Completion gate prototype | All five broken cases yield zero writes and an error; a valid case commits once | Experiment wrapper buffers entire stream and requires completed status before releasing chunks |
| Cancellation | During SSE and before approval resume: zero writes/no later model round. Cancellation immediately after successful commit: one durable write/no later round | Eino/context scheduling + server transaction semantics; cancellation is not rollback |
| Pending approval and process restart | Eino file checkpoint and real preview persist; prepare child abruptly exits with `os.Exit(0)`; a new child opens the same SQLite and restores the pending tool | Eino checkpoint works; **old preview cannot be committed after full app restart** |
| Safe restart fallback | Restored old preview gets `stale_preview`, zero writes; fresh preview + new explicit approval commits exactly once | Nestworth process nonce intentionally changes; do not bypass or reuse old approval |
| Reject/implicit resume/expiry/new arguments | Rejection, untargeted resume, edited expiry and fresh changed-plan run do not commit; changing an already-used operation's arguments is a conflict | Experiment approval gate + Nestworth expiry/operation binding; preview alone is not approval |
| Lost response + duplicate request | Upstream commit succeeds, response replaced by HTTP 404, adapter reconnects; model repeats call; 3 commit attempts / 2 session discoveries / 1 SQLite mutation and recovered success receipt | Retry is officialmcp; idempotency and receipt recovery are Nestworth's, not exactly-once transport |
| Long-context summary | Real Eino middleware compresses 12 messages with a stub false balance/approval summary; runner rereads actual account snapshot; write remains pending; paged authoritative USD cash is still 0 | Host policy + read tools + approval gate; a summary is not financial or authorization authority |
| Exact money | Approved synthetic 12.34 remains the exact authoritative decimal; conflicting reused operation for 99999 is refused | Nestworth ledger, not LLM arithmetic |
| Telemetry | No exporter/global callback configured; runtime default HTTP rejects external hosts, provider client allows only its fake endpoint; zero unexpected external HTTP attempts in passing tests | Harness traffic control + inspected defaults, not a comprehensive network security audit |

## Important adapter and persistence details

1. In `responses_event_convertor.go` at agenticopenai v0.2.4,
   `receivedStreamingResponse` accepts ordinary EOF without requiring completion
   and handles `response.failed`/`response.incomplete` as metadata. The runner can
   therefore dispatch a function call with complete argument JSON. Tool argument
   validity is insufficient to establish successful model completion.
2. `itemAddedEventReasoningToContentBlock` converts encrypted reasoning, whereas
   `itemDoneEventReasoningToContentBlock` only updates item ID/status. The
   done-only fixture reproduces that distinction. This corrects the earlier
   static-audit conclusion: converter support alone was not sufficient evidence
   of streaming preservation. No claim is made about how every live model emits
   these fields; this valid event-placement case is not covered by the adapter.
3. The conservative completion wrapper releases chunks only after reading EOF
   and observing final completed metadata; it caps at 1,000 chunks. It sacrifices
   live token latency and is only tested in streaming mode. It does **not** repair
   lost reasoning. A production solution should gate dispatch and let separately
   validated display text stream independently, with explicit byte/time limits.
4. Checkpointing required registering `map[string]any` and `[]any` with Eino's
   serialization registry. Without that, the real MCP tool schema metadata
   caused `gob: type not registered for interface: map[string]interface {}`.
   Both processes perform the same registration. The file adapter uses 0600
   temporary files and rename and implements deletion; it is not encrypted,
   fsync/power-loss tested, concurrent-writer tested or version-migration ready.
5. Nestworth `WriteCoordinator.previewToken()` contains a process-specific nonce.
   Reopening the application invalidates an uncommitted plan even with unchanged
   SQLite facts. The experiment preserves this boundary and demonstrates fresh
   preview/approval rather than weakening it for the test.
6. officialmcp retries CallTool once for terminal connection/session errors.
   Its session layer is not a transaction manager. The injected 404 happens only
   after fully consuming the successful real commit response. The resolved MCP
   SDK uses `server/discover` for this protocol version, hence session discovery
   counts rather than assuming an `initialize` RPC.
7. The summary test uses a single forced token-threshold trigger and a stub
   summarization model. It checks real financial read/approval boundaries; it
   does not prove arbitrary real models will explain monetary values correctly.
   The host must compute/render monetary answers from authoritative results.
   Bounded account details can initially contain zero rows with `hasMore:true`;
   the cash verifier follows the cursor and checks contentHash continuity.
8. Eino callbacks can receive prompts/tool payloads if an exporter is installed.
   Inspection found no default exporter in the exercised core/provider/MCP code.
   Existing application errors log code/caller; the synthetic expiry case emits
   `stale_preview`. No payload logger was installed. The HTTP guards cover the
   exercised clients, not arbitrary future custom transports or non-HTTP SDKs.

## Validation and remaining work

Recorded source `0dc2203`: 12 top-level tests passed, one subprocess helper skipped
in the parent run, 31 passing leaf cases (35 pass events including parents), and
both child process phases exercised. The final targeted run took 4.974 seconds;
that is suite execution evidence, not an app latency or memory measurement.
Compilation, `go vet`, formatting and whitespace checks passed. Raw provider
acceptance deliberately fails for the four limitations described above. Evidence
is retained in [tests.jsonl](evidence/tests.jsonl),
[provider-acceptance.txt](evidence/provider-acceptance.txt) and build/module files.

**Recommendation:** retain Eino core v0.9.22 as a viable embedded runtime candidate,
but do not ship this adapter combination as an unrestricted OpenAI ledger agent.
Resolve the provider completion/failed-state handling and done-only encrypted
reasoning preservation, then rerun the red acceptance command against the fixed
adapter. Implement a host-owned durable approval contract bound to the exact
preview, workspace/version, expiry and operation; all write capabilities,
including directory writes, need that policy. The string approval gate here is
synthetic test control, not a product security protocol. Re-preview and reapprove
after a full application restart.

No private data/access blocker or denied operation remains. Remaining technical
limits are the reproduced adapter defects and restart semantics, plus untested
native Go/Wails packaging/UI and real provider compatibility. No benchmarks,
RSS/size claims, full product test suite, native GUI, live API or CI run is claimed.
