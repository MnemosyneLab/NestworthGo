# Built-in AI Responses follow-up investigation — 2026-11-05

Source baseline: main `2b292ae` (latest fetched at investigation start).
Environment: Linux amd64, Go 1.27.3, Node 24.22.0, pnpm 12.14.0;
Wails 3.1.0 checks use the existing Linux GTK/WebKit dependency image.
All ledger tests use isolated synthetic SQLite fixtures. Provider requests go
only to a loopback fake with a synthetic key. No personal ledger, screenshot
amounts, API key, or paid provider request was used.

## Findings and scope

A successful greeting proves one request worked. A later failed run with no new
model history does not establish whether the next request, response validation,
local tool, subsequent tool-output request, or persistence failed. The previous
implementation collapsed errors at the transport, model, engine and runtime
boundaries, making these distinctions unavailable to the user. Successful-turn
checkpoints remain the intended persistence boundary.

The ordinary synthetic greeting -> financial context -> result -> answer path
passed before the parser change. A second fixture replaced the argument delta
with `response.function_call_arguments.done` containing `{}`. It failed before
the fix, before the controlled financial read ran. The parser required a prior
delta, although the documented done event contains finalized arguments.
The fixed parser accepts that complete snapshot; if partial deltas exist they
must match. The same argument bytes must match the done output item and terminal
response. Duplicate done, late delta, conflicting item, malformed arguments,
failed/incomplete terminal response, missing completion and trailing corrupt data
still fail closed, before tool execution. Both `{}` and nonempty object snapshots
have regressions.

This is a reproduced compatibility defect, **not proof that the user's actual
provider response had this event shape**. No live stream was captured. Official
reference: [Responses streaming events](https://developers.openai.com/api/reference/resources/responses/streaming-events).

Actual requests from the pinned Eino/OpenAI SDK omitted `strict` on functions.
The shared capability schema deliberately makes current-context fields optional
(`asOf`, `scope`, `disclosure`); `{}` is a valid minimal current read. Responses
may implicitly normalize schemas when strict is omitted. Requests now explicitly
send `strict:false` and preserve the existing schema and local validation. This
removes an implicit-contract dependency; a live provider regression caused by
normalization was not demonstrated. See [OpenAI strict-mode guidance](https://developers.openai.com/api/docs/guides/function-calling#strict-mode).

The full fake request path verifies greeting text and encrypted reasoning survive
checkpoint serialization, the next request includes the native function call and
controlled financial result, optional fields remain optional, and the final
checkpoint/evidence succeed. Existing provider tests also cover native replay,
paging, controlled refusal recovery, revocation, write exclusion and completion
before dispatch. Missing coverage remains missing financial evidence, not zero.

## Diagnostics and recovery

Only fixed host-owned categories cross SDK wrapping, engine and runtime:
authentication/access (401/403), rate/quota (429), request rejection, provider
unavailability (5xx), connection failure, response rejection, history rejection,
tool-result rejection, resource budget and timeout. Unknown errors keep the old
generic category. Status codes identify classes, not a verified provider-internal
cause. Raw error bodies/headers, request parameters, credentials and financial
results are never reflected into diagnostics. No new logging or automatic retry
was added. English, Simplified Chinese and Traditional Chinese messages give
safe next steps. Unknown UI codes fall back to generic text.

Synthetic canary tests verify HTTP categories survive the actual SDK and engine,
wrappers are discarded, failed completion content is discarded, the prior
checkpoint stays unchanged, and persisted codes reopen successfully. Existing
old failures remain generic; their causes cannot be recovered retrospectively.

## Acceptance boundary

Automated checks and exact final head are recorded in the PR. Initial direct
host Wails checks failed because GTK/WebKit headers were absent; the configured
Linux dependency image supplies those headers for subsequent checks.
No macOS GUI, packaging, signing, real-provider model availability, rate/quota,
encrypted-reasoning signature acceptance, or actual gpt-6-luna stream was verified.
Native Mac CI and an owner-run synthetic-ledger greeting/current-finance/next-turn
smoke remain separate acceptance gates. If it fails, report only the fixed
category and app version; do not request a key or ledger dump.

Performance impact: bounded argument snapshot buffering and fixed-error routing;
function metadata gains explicit strict flags. No financial calculation or query
algorithm changed. No performance benchmark or native latency claim is made.
