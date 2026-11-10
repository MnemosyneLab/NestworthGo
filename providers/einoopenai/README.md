# Completion-gated Eino Responses adapter

This nested module supplies the completion-gated Responses model, direct local
capability adapter and Engine used by `cmd/nestworth/assistant_runtime.go`.
The root module references it through a local replacement. Desktop activation,
credential storage, permission modes and page-operation authority are owned by
the [runtime contract](../../docs/architecture/builtin-ai-runtime.md); the provider
adapter does not independently grant financial or UI authority.

Dependencies are pinned in this module's `go.mod` and `go.sum`. Its synthetic
acceptance suite exercises the real pinned SDK request builder and response
converter through loopback fake providers. Historical experiment/PR stage order
is available in Git history, not a prerequisite for current activation.

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
   snapshots, observed argument/text fragments and the complete terminal output,
   except for the narrow reasoning ciphertext refresh described below. The
   validated terminal snapshot is authoritative for conversion, sealing and replay.
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
64 KiB, with a 256 KiB envelope for the bounded `operate_page` batch. JSON nesting
is capped at 64, and observed content-part indices stay below 128. The HTTP
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
interface. It accepts the current host-granted shared surface: Read-only excludes
mutations, snapshot-maintaining analysis and provider calls; Full also admits
shared writes and provider search. It rejects unknown effects, missing annotations,
duplicate names and unsupported/lossy schema conversions. The session rechecks
current permission on every call. Metadata is detached. Ordinary arguments and
all results are bounded to 64 KiB; `operate_page` arguments allow 256 KiB;
local dispatch retains exact decimal strings and the session's own typed
validation, permission checks, paging, cancellation and revocation. Arbitrary
implementation error text is not reflected into model history. The host must
use the actual in-process session; it remains household-wide and is not suitable
for current mobile/scoped principals.

The interface contains no confirm, approve, broad writer dispatch or key input.
Full writes use the root host coordinator and shared MCP contracts; see
[the integration contract](../../docs/architecture/builtin-ai.md). A model-created
preview cannot be adopted as approval. The Full convenience writer obtains a new authoritative preview and submits
its immutable identity under the session grant, without a human confirmation step.

`TestCapabilityRunnerLedgerHostReviewLoop` runs the real guarded model and typed
Eino runner against a local-only fake provider and real synthetic SQLite. It
checks local account evidence, a completed proposal, a distinct host preview,
synthetic Full submission, one exact 12.34 posting and authoritative
readback. Complete proposal arguments followed by EOF before item done, EOF
before response completed, failure or incomplete completion invoke no preview
handler. Done-only opaque reasoning survives the actual next request. A separate
positive variant uses real Eino summarization middleware with deliberately false
99999 balance/blanket-approval text: authoritative cash stays 0 before Full
submission, and a fresh host preview still binds the requested exact 12.34.
Root host tests separately exercise real child-process restart and a lost outer
success receipt; they do not implement an Eino checkpoint store. Existing
#201 acceptance still checks zero synthetic mutation dispatch independently of
host approval, full native history serialization/replay and error/key canaries.

This is a backend integration test, not UI or live-model acceptance. Desktop
conversation state, page context and navigation are implemented by the owning
runtime; their behavior is not established by this test alone. Mobile model
access remains closed. No API key or paid provider is needed to run it.

## Shared business recipes

The host appends compact business recipes from `internal/agentguidance/recipes.json`
to selected optional entry-tool descriptions. Existing group loading supplies them
only with those tool schemas, in the same deterministic order and with a fresh
turn-local catalog. The portable skill contains generated copies of the same
neutral text and illustrative rule format. There is no installed-skill read,
filesystem lookup, new tool or additional model round. Connection, permission,
unknown-outcome recovery and desktop-page policy remain owned by each adapter.
Tool results remain untrusted data. See the [runtime contract](../../docs/architecture/builtin-ai-runtime.md#shared-business-recipes).

## Provider contract and synthetic coverage

Function schemas explicitly send `strict:false` so optional capability fields
retain omission semantics. Complete refusals are displayable and replay their
native items. Fixed stage/rule diagnostic codes cross SDK/runtime boundaries;
raw provider errors, keys and ledger content never become diagnostic text.
Unknown codes remain generic. A synthetic compatibility reproduction is not
proof of the cause of an uncaptured live incident.


The official [streaming contract](https://developers.openai.com/api/reference/resources/responses/streaming-events#response.output_item.added)
marks added reasoning ciphertext as provisional. Added ciphertext may be absent,
null, partial or already final, so a differing added/done value is accepted.
The `response.output_item.done` item and `response.completed` output must still
agree, with one narrow exception: a reasoning item's nonempty `encrypted_content`
string may refresh to another nonempty string. Both snapshots must independently
validate; all other fields, including IDs, type, status, summary, reasoning text
and unknown native fields, must match. The exception does not apply to messages
or function calls, and does not allow missing, null, empty or malformed ciphertext
transitions. The validated terminal response's original ciphertext bytes are
authoritative for SDK conversion, checkpoint sealing and replay after clean EOF;
no decryption or plaintext-equivalence claim is made. Missing completion events,
trailing events and incomplete EOF still prevent history publication and tool
dispatch. The [reasoning/message schemas](https://developers.openai.com/api/reference/resources/responses)
allow optional nullable encrypted content and assistant phase; these are preserved
exactly. Reasoning `content` is documented as an optional array, without null;
`content:null` remains unsupported and is refused. No real provider capture or
request was used to infer additional nullable behavior.

Tests exercise all provisional-ciphertext forms followed by success, failed,
incomplete and truncated responses. The latter still produce zero dispatch.
Terminal-refresh tests verify exact terminal snapshot replay and zero dispatch
for changes outside the exception or missing/invalid completion.
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
All requests, keys, opaque reasoning and counters are synthetic. Any ledger fixtures are isolated and synthetic; tests never open a personal
ledger or make a paid/provider request.

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

These synthetic checks do not establish real-provider compatibility, encrypted
signature validity, model quality, native GUI/accessibility or packaging. The
installed desktop's credential, persistence, permissions and UI gates require
their owning runtime/application tests and relevant native acceptance. A passing
provider suite alone does not establish complete assistant acceptance.
