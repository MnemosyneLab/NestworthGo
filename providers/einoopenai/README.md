# Completion-gated Eino Responses adapter

This module resolves the two provider gates from [experiment PR #192](https://github.com/MnemosyneLab/NestworthGo/pull/192).
It implements Eino's `model.AgenticModel`, independently of the capability bridge,
MCP, SQLite, Wails and assistant UI. It is **not installed in the application**.
No writer becomes available and no existing external MCP behavior changes.

## Baseline and dependency order

Created from main `1701e5d5d9bb10ec9db846245766bab6f7a740e5` on 2026-11-04.
At inspection, #192 was draft at `c13313237f279dc56d9237047e187d1787882e95`;
its `experiments/eino-v0922/README.md` and source were read. The capability
prerequisite [#199](https://github.com/MnemosyneLab/NestworthGo/pull/199) was also
unmerged/draft at `192323f67d0a4f94ba2007533be018bbe90982fa`. Its
[design](https://github.com/MnemosyneLab/NestworthGo/blob/192323f67d0a4f94ba2007533be018bbe90982fa/docs/architecture/builtin-ai.md)
requires provider completion checks separately from trusted host approval.

This PR neither stacks on nor copies #199. The nested module keeps root runtime
dependencies unchanged and uses the existing repository JSON wrapper via a local
module replacement. It can merge before or after #199; a later runtime integration
must depend on both, choose the root dependency/module layout, integrate the host
lifecycle and durable review flow, and pass end-to-end acceptance. #192 is evidence,
not a prerequisite to merge. No merge is requested here.

The Go module proxy's `@latest` still resolved Eino **v0.9.22** and agenticopenai
**v0.2.4** on 2026-11-04. This module pins those releases and OpenAI Go SDK
**v3.44.0**, matching the experiment. It does not claim an upstream fix.

## Actual path and guarantees

`New` returns the guarded model; the underlying unsafe model is private. Eino's
actual typed agent/runner is used in acceptance tests, with a synthetic mutation
counter and no approval wrapper. Both `Generate` and `Stream` go through the same
bounded transport before the SDK or Eino receives a response. Stream requests
use real SSE on the wire, but deliberately return one complete Eino message.
Live display-token streaming is deferred; this increases time to first display.

1. The adapter reuses agenticopenai's request builder and **nonstream** response
   converter. It never invokes that release's unsafe streaming converter.
2. The transport reads through EOF, including trailing framing, and validates
   the complete JSON or SSE response. A successful HTTP status, valid tool
   argument object, `output_item.done` or `[DONE]` alone is insufficient.
   It requires one successful completed response, consistent response/item IDs,
   monotonic sequence numbers, completed items and agreement between item-done
   snapshots, observed argument/text fragments and the complete terminal output.
   Failure, incomplete/cancelled/error events, bad framing/JSON, duplicate names,
   unsupported events/items, inconsistent ordering and read errors fail closed.
3. Only the validated final response object goes to the official SDK and the
   upstream nonstream converter. Eino never receives provisional executable
   arguments. Cancellation and bounds are checked before returning a message.
   An already dispatched tool's own transaction/outcome semantics still belong
   to the capability layer; this adapter does not provide rollback or approval.
4. Complete native **output groups** are retained in a versioned JSON-string
   sidecar in `AgenticMessage.Extra`: reasoning, function calls and whole assistant
   messages. This preserves encrypted content, IDs, order, nullable fields,
   message `phase`, refusal parts, multipart message boundaries, future fields
   and exact numeric lexemes. On the next request, private per-round placeholders
   in the upstream builder are replaced with these captured items before any
   HTTP request. This bypasses that release's lossy history conversion, which
   drops phase, rewrites refusals as output text and splits multipart messages
   into multiple same-ID messages. Placeholders never reach the provider.
   Missing/duplicate/unmatched state or edited/reordered Eino content is refused;
   a digest binds native output and converted content for consistency, never
   approval authority. The
   `nestworth.responses.output.v1` sidecar must survive serialization/checkpoints.
   Earlier draft sidecar formats are refused rather than replayed lossily.
   Native state is **never display text**. UI consumers must select visible text
   or refusal fields explicitly, never render the entire model message,
   `Reasoning`, metadata or `Extra` as chat text.
5. The wire boundary enforces `store:false`, encrypted reasoning inclusion,
   disabled automatic truncation and zero SDK retries. Previous-response IDs,
   remote conversations/background execution, explicit remote cache keys/retention
   and provider/server tools are refused. HTTPS origin/path is pinned and
   redirects are refused before a second request. Errors do not contain response
   bodies, URLs, provider error messages or credentials. No exporter is installed.
   The host must also avoid installing payload-logging Eino callbacks.

The default per-round limits (overrides may only reduce them) are 4 MiB of request
and response body, 4,096 SSE data frames, 128 output/native-history items and 60
seconds including waiting for EOF. Function arguments are object JSON capped at
64 KiB, JSON nesting at 64, and observed content-part indices below 128. The HTTP
transport must obey Go request cancellation (as `http.DefaultTransport` does).
Buffers/decoding make multiple copies; there is no bounded-RSS or latency claim.

Supported outputs are host function calls, plain assistant text/refusals and
native reasoning. This deliberately conservative subset refuses annotations,
provider-executed tools, multimodal outputs and unknown event families. It can
reject otherwise valid provider extensions; extend it only with acceptance
fixtures and explicit review. Model choice and credentials are host-owned;
`Config.APIKey` is an integration input, not credential storage or a frontend API.
The public constructor accepts HTTPS only; tests use a private loopback exception
and a transport that rejects every other host.

## Provider contract and synthetic coverage

The official [streaming contract](https://developers.openai.com/api/reference/resources/responses/streaming-events#response.output_item.added)
marks added reasoning ciphertext as provisional; done is the authoritative item
for continuation. Added ciphertext may be absent, null, partial or already final.
A differing added/done value is therefore accepted, while done and the terminal
completed response must still agree. The [reasoning/message schemas](https://developers.openai.com/api/reference/resources/responses)
allow optional nullable encrypted content and assistant phase; these are preserved
exactly. Reasoning `content` is documented as an optional array, without null;
`content:null` remains unsupported and is refused. No real provider capture or
request was used to infer additional nullable behavior.

Tests exercise all provisional-ciphertext forms followed by success, failed,
incomplete and truncated responses. The latter still produce zero dispatch.
Stream and nonstream tests also round-trip absent/null final ciphertext and
preserve multipart text/refusal grouping and commentary/final_answer/null phase
through four actual adapter requests, with serialization between rounds.

## Reproduce and evidence

With Go 1.27.3 or newer, run from this directory:

```sh
./check.sh
```

This runs formatting, all default tests (including seeded parser fuzz cases),
vet and a separate build. The dedicated `Eino provider acceptance` workflow runs
the same command on trusted origin pushes; ordinary root `check` remains intact.
Module download is the only required non-loopback network access for these tests.
All requests, keys, opaque reasoning and counters are synthetic. Tests never open
a ledger or make a paid/provider request.

The default suite strictly requires zero dispatch for broken streams and failed
nonstream responses. Positive controls dispatch exactly once and carry reasoning
through subsequent real runner/model requests. Separate labelled characterization
cases retain the **unmodified** upstream failures: complete arguments dispatch
once after EOF before item done, EOF before response completed and `response.failed`;
done-only encrypted reasoning is dropped. These cases cannot substitute for the
strict corrected-adapter checks.

Coverage includes nonstream responses, malformed event/data shapes, duplicate JSON
names and identities, sequence/index errors, conflicting terminal output, incomplete
SSE delimiters, post-completion errors, text-part consistency, resource caps, timeout
while waiting for EOF after completion, cancellation before/during a response,
read failures after completion, HTTP errors, redirects, disabled retries, request
option overrides, full native-history serialization/replay and concurrent round
isolation. Workflow guards retain the exact active-workflow inventory, nonrunning
Linux rollback archive and the dedicated gate's trusted-origin/macOS ARM64 rules.

No real OpenAI compatibility, encrypted-signature validity, arbitrary compatible
provider, model quality, credential/keychain, database migration/recovery, built-in
read/write UI, native GUI/a11y, Wails packaging or complete assistant acceptance is
claimed. Completion validation is only one independent gate; host-owned review,
scopes/principals, durable state and the #199 follow-ups remain required before
activating the full assistant.
