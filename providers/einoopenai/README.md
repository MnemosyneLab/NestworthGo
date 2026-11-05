# Completion-gated Eino Responses adapter

This module resolves the two provider gates from [experiment PR #192](https://github.com/MnemosyneLab/NestworthGo/pull/192).
The guarded `model.AgenticModel` remains independent of MCP, SQLite and Wails.
The module also supplies `CapabilityTools` for direct model-facing local calls
and a synthetic ledger/host-review acceptance gate. It is **not installed in the
application**; no model writer or App assistant entry point becomes available.
Existing external MCP behavior is unchanged.

## Baseline and dependency order

Created from main `1701e5d5d9bb10ec9db846245766bab6f7a740e5` on 2026-11-04.
At inspection, #192 was draft at `c13313237f279dc56d9237047e187d1787882e95`;
its `experiments/eino-v0922/README.md` and source were read. The capability
prerequisite [#199](https://github.com/MnemosyneLab/NestworthGo/pull/199) was also
unmerged/draft at `192323f67d0a4f94ba2007533be018bbe90982fa`. Its
[design](https://github.com/MnemosyneLab/NestworthGo/blob/192323f67d0a4f94ba2007533be018bbe90982fa/docs/architecture/builtin-ai.md)
requires provider completion checks separately from trusted host approval.

The original provider stage (#201) and capability stage (#199) are now merged.
The host-review integration stage uses main
`76722b33477f45aed4babffaa0d33664098c3f7d`, including desktop navigation #200.
The nested module still uses the repository JSON wrapper via a local replacement and does not add root runtime
dependencies. Its additional indirect SQLite/MCP dependencies are used by the
synthetic shared-handler acceptance fixture; it does not construct an MCP
transport. The executable still needs a module/dependency decision, durable
runtime/principals, credentials and trusted desktop/mobile UI before activation.

The Go module proxy's `@latest` still resolved Eino **v0.9.22** and agenticopenai
**v0.2.4** on 2026-11-04. This module pins those releases and OpenAI Go SDK
**v3.44.0**, matching the experiment. It does not claim an upstream fix.

## Actual path and guarantees

`New` returns the guarded model; the underlying unsafe model is private. Eino's
actual typed agent/runner is used in acceptance tests, with a synthetic mutation
counter and no approval wrapper. Both `Generate` and `Stream` go through the same
bounded transport before the SDK or Eino receives a response. Stream requests
use real SSE on the wire, but deliberately return one complete Eino message.
The desktop runtime installs a progress observer: Engine Generate then selects
SSE and publishes provisional text snapshots while frames arrive. Without an
observer Generate retains JSON mode. Stream still returns one verified message.
All modes retain bounded reads, terminal validation and clean EOF before dispatch.
Tool start/end events carry real, bounded input/output through the runtime;
only successful complete replies advance the native checkpoint. See the
[built-in runtime contract](../../docs/architecture/builtin-ai-runtime.md#provisional-streaming-and-tool-details).
No latency/quality equivalence with a live provider is claimed.

1. The adapter reuses agenticopenai's request builder and **nonstream** response
   converter. It never invokes that release's unsafe streaming converter.
2. The transport reads through EOF, including trailing framing, and validates
   the complete JSON or SSE response. A successful HTTP status, valid tool
   argument object, `output_item.done` or `[DONE]` alone is insufficient.
   It requires one successful completed response, consistent response/item IDs,
   monotonic sequence numbers (SSE), successful terminal items (function status may
   be absent; an explicit final status must be completed) and agreement between item-done
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

## Direct capability adapter and host review gate

`CapabilityTools(session)` accepts only the model-facing `Definitions`/`Call`
interface. It rejects writes, open-world/provider capabilities, unknown effects,
missing annotations, duplicate names and unsupported/lossy schema conversions.
Returned metadata is detached. Arguments and results are bounded to 64 KiB;
local dispatch retains exact decimal strings and the session's own typed
validation, permission checks, paging, cancellation and revocation. Arbitrary
implementation error text is not reflected into model history. The host must
use the actual in-process session; it remains household-wide and is not suitable
for current mobile/scoped principals.

The interface contains no confirm, approve, broad writer dispatch or key input.
Single ledger write reviews live separately in the root host coordinator; see
[the integration contract](../../docs/architecture/builtin-ai.md). A model-created
preview cannot be adopted as approval. The host obtains and displays a new
authoritative preview, then deliberately confirms its immutable identity/digest.

`TestCapabilityRunnerLedgerHostReviewLoop` runs the real guarded model and typed
Eino runner against a local-only fake provider and real synthetic SQLite. It
checks local account evidence, a completed proposal, a distinct host preview,
explicit synthetic host confirmation, one exact 12.34 posting and authoritative
readback. Complete proposal arguments followed by EOF before item done, EOF
before response completed, failure or incomplete completion invoke no preview
handler. Done-only opaque reasoning survives the actual next request. A separate
positive variant uses real Eino summarization middleware with deliberately false
99999 balance/blanket-approval text: authoritative cash stays 0 before host
confirmation, and a fresh host preview still binds the requested exact 12.34.
Root host tests separately exercise real child-process restart and a lost outer
success receipt; they do not implement an Eino checkpoint store. Existing
#201 acceptance still checks zero synthetic mutation dispatch independently of
host approval, full native history serialization/replay and error/key canaries.

This is a backend integration test, not a UI flow. Model tool choice, explanation
quality, native key storage, mobile permissions, durable conversation/run state,
structured page-context capture, semantic navigation and real-model compatibility
are unimplemented or unverified. No API key or paid provider is needed to run it.

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
the same command on GitHub-hosted Ubuntu 24.04 on trusted origin pushes;
ordinary root `check` remains intact.
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
Linux rollback archive and the dedicated gate's trusted-origin/hosted-Linux rules.

No real OpenAI compatibility, encrypted-signature validity, arbitrary compatible
provider, model quality, credential/keychain, financial-schema migration, full
database/backup/restore acceptance, built-in read/write UI, native GUI/a11y, Wails
packaging or complete assistant acceptance is claimed. The bounded host recovery
tests above do not establish those gates. Scopes/principals, durable runtime and
conversation state, trusted UI and the #199 follow-ups remain required before
activating the full assistant.
