# Built-in AI complete response contract follow-up — 2026-11-05

Baseline: main `3d93dbcab847b11e4c60906e427d15c3590746d9`, including #227.
Linux amd64; configured Go 1.27.3 / Node 24.22.0 / pnpm 12.14.0 / Wails 3.1.0.
All tests use local fake HTTP providers, synthetic SSE/JSON, synthetic keys and
isolated synthetic SQLite ledgers. No real key, personal ledger, screenshot
amount, paid request or app launch was used.

## Established defects versus incident hypotheses

The original #227 no-delta repair is not the explanation assumed here. The new
report's safe response-rejection category narrows the layer but still does not
identify a particular rejected provider event or SDK conversion.

The current [official function-calling stream example](https://developers.openai.com/api/docs/guides/function-calling#streaming)
omits function item `status` in added/done snapshots. The pinned official Go SDK
also marks function `status` optional; its converter supports absence. Main's
validator nevertheless required `in_progress` in added and `completed` in final
function items. Full synthetic three-user-message fixtures failed on the second
message with main's validator; they pass after the fix. An isolated copy using
the exact baseline validator independently reproduced that red result for both
Generate and Stream. This is a demonstrated compatibility defect; optional type
metadata/examples do **not** prove that the user's live response omitted status.
No live provider response was captured.

The initial function snapshot could also contain an argument prefix or complete
object; main rejected any nonempty initial arguments. Accumulation now starts
from that bounded snapshot and appends deltas. Finalized arguments must agree
with observed bytes, item.done and response.completed. Official Node
[ResponseStream tests](https://github.com/openai/openai-node/blob/main/tests/lib/ResponseStream.test.ts)
include completed snapshots through added; these are SDK compatibility tests,
not production captures. Partial arguments in response.created.output were not
misidentified as evidence about output_item.added. Partial/sparse response-frame
support is not introduced by this patch.

Absent function status is distinct from explicit null, empty, mistyped,
incomplete or in-progress terminal status. Those explicit terminal values still
fail even if the outer response is completed. A nonempty provisional name/call ID
cannot change. Empty string provisional identities may acquire complete terminal
identities; missing/null/mistyped required string fields are rejected. The optional
name on argument.done, if supplied, must agree with the final call. Event item
IDs remain required for correlation and stateless replay despite their optional
API type declaration. Unknown events are rejected, never silently discarded.

A separate SDK mapping issue was reproduced: complete refusal content lives in
`AssistantGenText.OpenAIExtension.Refusal.Reason`, with empty `Text`. The Engine
previously treated that as an empty/failed response. It now displays the complete
refusal and preserves its untouched native item for later turns. This is another
compatibility fix, not an asserted cause of the user's asset-summary failure.

## Transport decision and guarantees

The production Engine now calls Generate. Its prior Stream call buffered the
entire SSE response and yielded one message; the UI never received provisional
tokens. Nonstream JSON removes the handwritten SSE state machine from ordinary
desktop usage without removing a user-visible streaming feature. It still reads
through clean EOF, validates strict JSON and successful outer/item states,
checks body/item/history/tool budgets, uses the actual SDK's nonstream converter,
and preserves complete native output groups at their original positions.
No retry, fallback, previous_response_id, server-side conversation, write or
navigation authority is added. Stateless store:false and encrypted reasoning
requests remain unchanged. No performance or live transport-equivalence claim is
made. True incremental display would need a separate future design.

The public Stream adapter is retained and corrected. The same Engine orchestration
is exercised through a test-only Stream adapter as well as production Generate.
The three-user-message matrix performs five provider requests: greeting, tool
call, tool-output follow-up, another tool call and another follow-up. It verifies
all native groups, encrypted reasoning, exact serialized checkpoints, user order,
tool-output count and real controlled financial-context results. Both paths
require success before execution and discard failed histories.

`TestGenerateEngineCompletionGate` checks malformed/trailing JSON, explicit
incomplete/failed outer or function states and cancellation while a valid JSON
body has arrived but EOF has not. A synthetic marshal hook cancels after response
verification/SDK conversion and before dispatch; it verifies zero local reads,
empty new history and `context.Canceled`, fixing that window's old generic error.

## Stream evidence and diagnostics

The new `contractEvents` fixture does not rely on the old simplified `events()`
helper. It emits created/in_progress, reasoning summary/content parts with all
text events, message text/refusal parts with all events, argument delta/done,
item.done and response.completed. The adversarial matrix proves zero dispatch
for changed identities, conflicting argument/name/text/refusal/summary snapshots,
missing part completion, wrong item correlation, invalid explicit statuses,
unsupported events, missing terminal completion, inconsistent terminal response
and trailing corruption. Prefix/full initial snapshots have positive controls.

Fixed code-owned stage/rule diagnostics distinguish request contract, HTTP body
and media type, terminal JSON/item validation, SSE added/function/arguments/parts/
ordering/framing/EOF/unsupported event, SDK conversion and engine inventory/empty
text. Raw errors, provider content, field values, ledger facts and keys are never
included. The whitelist survives SDK wrapping, runtime persistence/reopen and
localized UI. Unknown diagnostic strings are not displayed. Existing broad codes
remain valid; old failures cannot be retrospectively classified.

## Runnable synthetic reproduction

Use the configured toolchain, then run from the repository root:

```bash
cd providers/einoopenai
go test -count=1 -run 'TestFullStreamContractThreeUserTurns|TestFullSSEContractAdversarialGate|TestGenerateEngineCompletionGate|TestCancellationAfterResponseValidationBeforeToolDispatch|TestCompleteRefusalIsDisplayableAndReplays|TestFixedProtocolDiagnosticsSurviveSDKAndEngine' -v ./...
./check.sh
```

These fixtures use the real pinned request builder/SDK converter and loopback-only
transport. They require no personal settings, database or provider credential.
Final exact-head checks and their results are recorded in the PR. Affected
runtime/state/IPC/architecture tests and frontend conversation/locale tests are
included; unrelated full financial/UI/race/performance suites are not repeated.
The existing Linux GTK/WebKit dependency image supplies native build/vet and
binding generation. Linux/jsdom checks do not establish macOS GUI, packaging,
provider signatures, model access/quality or the actual uncaptured incident cause.
No output-only `created_by` replay change is made without evidence; a provider
request rejection there would be a different category. CI follow-up belongs to
the coordinator.
