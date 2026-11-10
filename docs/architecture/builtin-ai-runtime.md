# Built-in AI desktop runtime

The desktop right-hand assistant supports explicit OpenAI Responses model requests,
controlled local financial reads, run feedback, cancellation and paged persistent
history. Activation defaults off. Desktop Settings → Built-in AI credentials saves
a write-only key, an enabled model set and an explicit enable flag.
The sidebar selects the per-request model and supported reasoning effort; see
[model selection](ai-model-selection.md) for the offline catalogue, history and
account-access rules. There are no startup,
reconnect or implicit retry model calls. Desktop writes default off. The UI permission modes are Read-only and Read & Write (可读写 / 可讀寫); the latter retains the internal Full permission and does not require per-change confirmation. Mobile model access remains closed. The existing deterministic page guide remains available on mobile.

The enabled vertical covers current overview, accounts, closed-period attribution
and one-shot model-selected navigation under a current desktop page lease,
without a separate per-message approval.
Full exposes the existing MCP ReadWrite registry, including preview/commit,
directory, investment, reconciliation, market-data and repair capabilities, plus
the scoped generic page operations described below. Unrestricted UI automation
and mobile model access remain unavailable. Existing explicit
account buttons retain their page-host guards; model navigation has independent
backend and frontend fences described below.

## Ownership and authority

`internal/assistant` owns private state, transitions, model/credential/storage
ports, fixed errors and the future page-host contract. The executable derives
the namespace from the canonical database path and household UUID. The runtime
gets a fresh host epoch on every open, pins the application's workspace
incarnation and rechecks it on every public action and before result delivery.
Callers cannot select paths, a household, a mobile principal or a tool permission.
A missing household leaves the backend unavailable until a later application
restart; onboarding does not silently activate it.

Each live conversation owns a direct `InProcessSession` through
`builtinai.Pool`; the installed desktop uses a durable configuration repository
and a host-owned `ReadWrite` session, while the model adapter retains its
current shared closed-world Read definitions by default, or the complete shared registry through the separately enabled Read & write callback. The model
never receives the host review methods. Read-only test/integration pools remain
`ReadOnly` when no repository is supplied. The engine and credential lease remain per-run. This
preserves frozen captures across turns within the original fixed five-minute TTL.
Page/item reads never renew it. The pool is bound to this runtime/ledger and at
most 20 conversations. Delete/retention drops the conversation session; config/key
changes, disable, restore and shutdown revoke all sessions. Revocation is
nonblocking under runtime/restore locks; draining occurs outside the restore gate.
No local HTTP,
external MCP token or generic Wails dispatch is used. Full includes the shared provider-search capability; its existing disclosure contract remains authoritative.
Capture/cursor state does not gain fresh authority from chat history. Reads use
the existing cropping, page limits, missingness and exact-money contracts.
`internal/builtinai` intersects discovery and dispatch with the current shared
closed-world Read definitions and the strictly in-memory attribution adapter.
Read-only loading now includes directory, history, investment, reconciliation and
maintenance-status reads rather than a fixed name allowlist. Non-read and
open-world effects remain unavailable. The shared session remains authoritative.
Continuations use `get_read_page`; the former explicit-scope financial page tools
were removed. It adds `inspect_page` with `source=evidence` only for supported
view selections, and `inspect_page` with `source=navigation` / `navigate_page` only for an explicit
current desktop page lease. The shared session independently rechecks permissions
and budgets.
In read-only mode no configuration, network refresh or projection-maintaining analysis is available. Human approval methods are never model tools. Direct financial writes exist only
through the explicitly enabled Full stage below. The retired cash-proposal and human-confirmation endpoints are no longer exposed.
Their operation receipts remain in their existing household/session namespaces;
chat deletion neither deletes those receipts nor reverses a financial operation.

The current authority is desktop and household-wide. No mobile/scoped principal
may receive it. Live mobile permission intersection, per-account authorization
remain unimplemented. The model cannot manufacture host
approval from text, a summary, a digest, a page snapshot or a saved conversation.

## Explicit Full access — shared MCP capabilities

Full is an optional desktop, household-wide, app-session permission. The sidebar has one shared choice menu with Read-only and Read & Write options. Startup,
configuration/credential changes, restore and shutdown return to Read-only.
Enabling requires an idle configured runtime and the current epoch/revision;
disabling remains available during a run. Every mode change revokes existing
capability sessions and navigation grants. A failed permission-state save closes
Full access. No model argument, chat approval or restored history enables it.

Full discovery reuses the complete `ReadWrite` registry and its exact input
schemas, descriptions and effects. Dispatch uses that same in-process registry;
there is no loopback HTTP, duplicate financial implementation or additional
per-write approval. Full and external MCP read-write mode share the same
79-tool registry, including investment-review management. External MCP has
its own permission setting, credentials and operation namespace.
Ordinary ledger writes use `preview_batch` followed by `commit_plan`, including
single cash changes and transfers. The duplicate `execute_ledger_change` tool
and its desktop command adapter have been removed; saved conversation text
cannot reinstate it. Existing receipt inspection remains read-only.

All shared preview/commit rules, transaction boundaries, version/expiry checks,
operation IDs, idempotency and receipt recovery remain authoritative. Before a
shared writer is dispatched, the runtime validates its arguments with the same
schema and saves the operation identity in the conversation. `commit_plan` uses
the shared deterministic identity when omitted. Other writers require the same
explicit operation UUID as MCP. The original session/run/revision is checked;
revocation fences the acquired session, including writes queued on application
authority. Concurrent write callbacks in one conversation are serialized. If the pre-dispatch reference save fails, the runtime closes and restores the exact prior in-memory references before normal run completion can persist them. This removes a proven never-dispatched attempt and preserves prior resolved retry metadata. If a save actually landed but the process dies before a successful repair save, the durable unresolved reference still remains conservatively fenced.

An attempt proven to precede business dispatch is recorded as unsubmitted; cancellation alone never proves rollback. The shared commit-plan router marks malformed plan IDs and known noncommittable plan types as definite rejections. Missing valid plans retain unknown-outcome semantics. Runtime argument/permission preflight rejections report write_not_submitted rather than implying an unknown new write. An existing unresolved reference instead reports write_blocked_pending_operation. Registry observation tracks actual handler entry. Cancellation at the shared receipt gate is proven unsubmitted only when no prior receipt exists; existing uncertain receipts retain their fence. Unknown or missing receipts after possible dispatch leave an unresolved reference that blocks subsequent
writes, deletion, retention and navigation in that conversation. Inspection,
polling, reconnect and restart never dispatch a mutation. Generic Full records
cannot be confirmed, cancelled or repreviewed through human review actions.
Known failed receipts release the fence; unknown outcomes do not. Stop is not
rollback. Write errors terminate the provider turn with explicit inspection
instructions, rather than read-recapture advice. There is no cross-request
semantic deduplication of a deliberately repeated new instruction.

The desktop displays shared tool inputs, bounded outputs, errors and progress,
with operation results in the normal conversation/tool disclosures, without a separate review panel. Unknown tool display names use readable
words derived from the registry name. Successful inspected writes invalidate
business queries. The existing 128-tool inventory, 64 KiB ordinary input/output
(256 KiB input for bounded `operate_page` batches), 16-call and model/history
budgets remain; larger workflows must use existing MCP paging
and batch limits. Full does not expand those budgets. Evidence capacity is reserved before read, page-content or analysis execution, including concurrent calls; failed reads release their reservation without adding evidence.

Business receipts remain in the private household/conversation capability
namespace; deleting chat does not delete receipts or reverse financial activity.
Single-process ownership remains: no cross-process CAS or concurrent app-process
authority is added. All tests use synthetic SQLite and fake providers. Native
Mac GUI acceptance remains separate from Linux, jsdom and provider tests.

## Operation recovery and retired cash proposals

The reviewed-write mode, `SetWritePermission`, `ReviewAction` and
`propose_cash_change` have been removed. The sole desktop setter is
`SetFullAccess`: false is Read-only, true enables the existing shared MCP
ReadWrite registry. It does not introduce a per-action approval step.
Permission is never persisted; existing configuration and conversations reopen
in Read-only, without enabling writes from history or old model text.

Private operation references and receipts remain readable. Inspection retires
provably unsubmitted legacy human proposals; submitted/succeeded/unknown records
retain their original receipt identity and never dispatch a mutation. A stopped
Full convenience operation's unsubmitted preview is also retired during idle
inspection. Unknown outcomes remain fenced until authoritative receipt recovery;
chat deletion never rolls back a write or discards an unresolved identity.
The legacy proposal source name remains accepted only when validating saved
historical evidence, so old conversations can still be opened.

The sidebar reconciles these references in the background and invalidates
business queries after success. It polls while a run is active, a result is
unresolved, or inspection failed; settled results stop polling. Replies and
expandable tool inputs/outputs are the visible result surface. Receipt references
persist the originating run and tool ordinal; recovered authoritative status
overlays that invocation without rewriting its original transport output. Legacy
references match only exact operation identities, never a tool name alone. A
receipt whose original trace is absent appears as an ordinary collapsed tool
result in the transcript. Inspection failures are visible and retried; none of
these display paths resubmit writes. Preview, schema,
version/expiry, transaction and idempotency checks remain in the shared backend.

## Strictly read-only period analysis

The external `compare_financial_attribution` retains its existing derived-snapshot
maintenance behavior. The built-in adapter attaches the private application
`WithReadOnlyFinancialAttribution` context policy at dispatch and describes its
narrower semantics in model discovery. No wire argument can mint that policy or
turn it off. The same handler, coherent comparison cache, historical valuation,
`ComputeAnalysis`, compatibility proof and attribution projection remain shared.
The strict path reconstructs daily inputs in memory, never calls the snapshot
ensurer/saver, never changes dirty ranges and never invalidates previews.
The existing reentrant coordination permit still protects capture/publication;
that permit does not imply a SQL/financial write. SQLite `query_only` acceptance
and exact maintained-projection parity are regression gates.

Supported attribution uses household or one actual account, base valuation and two
available closed-date endpoints, at most 365 days apart. The reported period is
left endpoint +1 through the right endpoint in the history timezone. At most
20,000 daily snapshot items are reconstructed; the shared ten-second build budget,
wire/capture limits and total run budget also apply. Oversized ranges/data report
unavailable, without silently dropping facts or substituting a different period.
Model instructions ask for narrower dates when necessary. Net-worth movement,
capital flows, residual and signed precision adjustments are not investment return.
Availability, coverage, missingness, precision, original source dates and same-
comparison frozen detail remain authoritative. Instrument/member/multiple-account
analysis and current-day returns are outside this initial period vertical.

## Configuration and database credentials

A versioned private `ModelConfig` contains only provider, model and enabled state.
Only `openai-responses` is implemented. Model names have a bounded syntax; other
providers and key-looking model names are refused. The initial model is the
non-operational placeholder `unconfigured`, with activation off. Host integration
must explicitly choose a model and save a credential before enabling
it. Configuration/key changes increment the revision; stale new requests
are refused. Enabling/configuration replacement and key replacement cannot race
an active run.
Explicit disable cancels active workers and prevents late successful publication.
A provider/model change clears native checkpoints while preserving display
history. Credential
revocation cancels runs and durably disables activation before attempting deletion.

`assistant.Credentials` remains a replaceable host-owned port with `Available`,
`Put`, `With` and `Delete`. The production adapter uses the existing live SQLite
`ConfigurationRepository`, under the independent `assistant.credentials.v1` key
in `app_configuration`. It requires no Keychain, signing identity, environment
variable, command-line secret or external process. Status returns only
`unavailable`, `missing`, or `present`; stored key material and implementation
errors are never returned through IPC, diagnostics, events or conversations.
Desktop Settings → AI / MCP provides a shared masked secret field for new input,
replacement and explicit deletion. Input clears on submission and section exit;
new keys do not enter query/mutation caches. A missing runtime explicitly reports
`unavailable` with an empty epoch. Editing and submission require a nonempty host
epoch and an explicit `missing` or `present` credential status; absent, unknown
and failed status responses keep controls closed. Saving a key does not enable model
calls, select a model, or expose financial writes. Mobile cannot mutate credentials.

This deliberately matches existing main-database provider/token storage: values
have no field encryption and full database backups include them. Restoring an old
backup can restore its old key; deleting/replacing a live key does not remove old
backup copies or promise forensic erasure of SQLite/filesystem remnants. Protect
backup files accordingly. Conversation state remains outside these backups.
Deletion writes an empty configuration tombstone and first disables activation.
Lease buffers are cleared after use; Go/SDK strings do not guarantee complete
memory erasure. No prior supported AI database credential exists, and no existing
provider/MCP setting is migrated or modified. The removed signed-Keychain adapter
was unavailable in ordinary builds; no automatic Keychain import is performed.
No financial schema migration is required.

Provider calls use the #201 adapter: fixed default HTTPS OpenAI origin, no custom
origin exposed by this runtime, no redirects, zero SDK retries, `store:false`,
disabled truncation and encrypted-reasoning inclusion. Tests use only synthetic
keys and loopback fake providers. No real credential or paid request is needed.
Key canaries in submitted prompts, configuration, model output, tool arguments,
checkpoint bytes or subsequent tool-result history are refused before the relevant
persistence/network/execution boundary. Provider failures and panic values are
never formatted or logged. This is a direct key-taint guard, not a general DLP
claim about arbitrary transformations of a secret.

## State, privacy and storage compatibility

`internal/infrastructure/aistate` uses a dedicated SQLite file:

```
<settings directory>/assistant-private/<sha256(canonical DB path + household)>/state.db
```

The dedicated directory must be mode 0700 and state/lock/journal files mode 0600;
symlinks and permissive files are refused. A process lock enforces one owner.
SQLite uses DELETE journaling, FULL synchronization and secure deletion. One
bounded version-1 state document is updated atomically. Version checks precede SQLite's
writable open; structural schema, payload, state, identity and history-binding
checks refuse malformed, unversioned, future or mismatched state. Normal SQLite
version-1 journal recovery is permitted. Integrity digests detect accidental edits
or history swaps, not a malicious owner of the database.

Conversation state does not enter the financial database, configuration table,
ledger audit, operation receipts, native app bundle, normal database snapshot,
JSON ledger export or application backup selection. The end-to-end fake-provider
test creates an actual application backup and verifies that conversation identity,
native reasoning and private AI schema are absent. Its in-memory fake credential
canary remains absent; production database credentials intentionally enter full
backups. Dedicated database-credential tests verify archive/restore behavior.
The OS may independently back up private files under its own policy.
Conversation text is private local plaintext and may
contain financial information. OS disk encryption and device access remain
relevant. Secure deletion does not promise forensic erasure from filesystem
snapshots, previous disk copies or operating-system caches.

Limits are explicit: 20 conversations, 64 attempted runs per conversation,
16 KiB input text, 64 KiB final display text, 1 MiB complete native checkpoint per
conversation, and 16 MiB aggregate serialized private state. Conversations older
than 30 days are pruned on startup and conversation creation; no background timer
or cloud retention service is installed. Budget/storage failures close the runtime
instead of exposing an undurable success. Explicit conversation deletion removes
only private AI state. Public history pages contain one attempted turn plus at most 16 bounded host
read records and 64 KiB of credential-filtered tool details. JSON escaping can expand these prompt/display/detail text budgets;
no private checkpoint or reasoning field is included.

There is no financial schema upgrade. Fresh sidecars initialize transactionally;
future/unknown sidecars are not repaired or downgraded. Rolling back the app leaves
an ignored private directory. With the app closed, explicitly deleting that
ledger's AI directory starts a new disabled namespace and loses its chat/dedup
records, while financial facts and receipts remain untouched. This is not a
migration of an older AI schema, because no supported prior durable schema exists.

## Run lifecycle and recovery

Start requires the current host epoch, current configuration revision, a host-
created conversation ID and canonical client request UUID. Each run pins its provider, model and configuration revision. Admission with an
immutable prompt-and-page-selector digest is durable before a worker or provider is invoked.
An identical request returns its original run, including failed/interrupted runs;
changed content under the same request ID conflicts. At most two runs can be
active, with one per conversation. A second different request to that conversation
conflicts until the active run finishes.

| State | Meaning |
| --- | --- |
| `accepted` | Durable admission; worker may not have started |
| `running` | Durable running state; paid response outcome may become uncertain |
| `succeeded` | Complete display response and full native history saved atomically |
| `failed` | Failure/timeout/budget path; previous successful history retained |
| `cancelled` | Explicit cancellation; no new successful history is published |
| `interrupted` | Restart, revocation or lost completion requires a new explicit request |

The Eino engine uses a serial loop with at most 8 model rounds, 16 tool calls,
128 messages and a total run deadline of at most 120 seconds. The eighth response
is reserved for a final answer with `tool_choice: none`; reaching exactly 16 tool
calls starts this final-answer phase earlier. A transient host policy asks the
model to summarize only evidence read for the current request and disclose missing
or partial coverage. It is not stored in the checkpoint. Any tool call returned
during this phase fails before dispatch. An over-limit call inventory is rejected
as a whole, without partial dispatch. These limits are not increased, and a final
answer still must pass all response/history/display and deadline checks. Each model round
keeps #201's own byte/event/item/60-second limits. A response yields an executable
message only after terminal success, full item/framing validation and clean EOF.
The whole call inventory is checked before dispatch; a response mixing a valid
read with an unavailable tool dispatches neither. Cancellation after `response.completed`
but before EOF also dispatches nothing. Provisional text is displayed separately and never authorizes a tool or advances history. There is no SDK request retry, automatic run replay,
remote checkpoint, compactor or summary replacement. Tool failures preserve the
shared application business diagnostic (`code`, `field`, `message`) without a
second provider-specific code/field allowlist or generic message substitution.
Schema failures also retain their `rule` and `expected` constraints. Validation
asks for corrected input; expired or revoked captures ask for a same-scope
restart; oversize and busy reads have their own guidance. Unexpected untyped
infrastructure errors remain generic at the existing application error boundary;
raw SQL, filesystem errors and provider transport bodies are not forwarded.
Business data in diagnostic text is treated like other authorized tool data,
never as instructions. Write recovery still requires receipt inspection when the
outcome is uncertain. Provider/framing, context cancellation and invalid/oversized
result failures still fail closed.

A successful checkpoint retains the complete Eino messages, native output groups,
opaque reasoning, tool call/result order and provider/model identity. It is never
rebuilt from display text. The next actual SDK request validates/replays those
native groups in place. The first host-owned system policy contains stable common
instructions; current mode policy is appended at the end of prior history before
the new user message only on the first turn or when the latest system snapshot
differs from the current host policy. Unchanged mode adds no snapshot; current
execution authority is still rebuilt every run. Version-1 provider checkpoints migrate that first policy
once; version 2 retains a metadata-only Full catalog high-water mark for the
stable core. The model can use `load_tool_groups` to append task-specific schemas
from the current execution map. Group selection is validated before loading,
available from the next model response, and reset at the start of the next turn.
Loading uses the existing 8-round/16-call budget. Catalog and saved history
confer no authority: `allowed_tools` and the pre-dispatch check admit only
currently permitted tools exposed in that response. External MCP clients still
discover the full registry allowed by their current permission through
`tools/list`; the built-in loader is specific to the desktop model loop.
See [prompt-prefix stability and measured tradeoffs](../development/ai-prompt-cache.md).
Assistant-native outputs and opaque reasoning remain byte-preserved; old capability claims cannot
suppress newly installed features or grant removed authority. Changing models cannot silently reuse the old model's
opaque state; configuration changes clear native history and retain display
turns. Context-budget exhaustion similarly
requires a new conversation until a verified compaction policy exists.

A failure after a previous read can have performed that read; it does not advance
the complete-turn checkpoint. An explicit new request may read again. Pending
tool execution is never recovered or replayed; Full writers retain their existing receipt inspection
and unknown-outcome fences. After abrupt process exit, accepted/running records become `interrupted` and generate a durable
state event. If terminal persistence fails, delivery is fenced; restart recovers
from the last durable admission, not an optimistic in-memory success. A provider
may already have charged an interrupted request; there is no exactly-once or
billing-reversal claim.

Normal shutdown cancels/drains the runtime before application writer shutdown and
database close. Before a validated restore replaces the database under its
exclusive permit, `FenceForRestore` cancels without waiting for provider/tool
workers, durably clears the old conversation namespace and disables activation.
A persistence failure blocks restore. Late results cannot repopulate it. After
replacement the application exits; a new runtime receives a new host epoch. A
database path/household switch selects a separate namespace.

## UI and page integration contracts

The desktop IPC inventory is `Status`, `List`, `Create`, `Inspect`, `Messages`,
`Start`, `Cancel`, `Events`, `Delete`, `SaveCredential`, `DeleteCredential` and
`Configure`, `PublishPage` and `ConsumeNavigation`.
Configuration/navigation authority is desktop-only and uses the current epoch/revision.
`Inspect` returns the latest attempted
turn; `Messages` takes an explicit zero-based offset and returns one turn plus
run count. Events contain only sequence, run ID, state and fixed code: at most
64 per call, 256 retained per conversation. Reconnect queries state/events using
the latest epoch instead of resubmitting a new paid request. A rolled-over/future
cursor returns `reset`; refresh metadata/history. Generated bindings expose
write-only credential input and bounded configuration,
never stored secrets or history internals. `SetFullAccess` and `Reviews` are desktop-only, bounded endpoints; none is a model tool. The desktop polls
status every two seconds and the selected latest turn every second while mounted (every 200 ms during an active run).
The sidebar presents a chronological transcript: the latest attempt and up to
five preceding turns load on session selection. “Load earlier messages” prepends
five more bounded `Messages` reads without replacing the visible transcript.
New attempts retain already displayed history. Scrolling away from the bottom
pauses following; “Latest messages” resumes it. Failed history reads have a
local retry that never replays `Start`.
Completed host read evidence appears in a collapsed disclosure attached to its
answer, with allowlisted localized source names, capture time and conservative
coverage/missingness. Per-tool lifecycle events now show actual serial execution, with expandable input/output details and explicit refused/failed/interrupted states.
Privacy/scope/retention information lives behind an accessible info popover.
Single-message navigation remains a separate explicit send action inside the
page-navigation disclosure; opening it grants nothing. Closing the assistant
retains the session and does not delete its persisted history. No poll invokes a model. Offline state
pauses only new model submissions. All local Status/List/Inspect/Messages queries
use `networkMode:always`; Stop and local history remain available under a browser
offline hint. Reconnect/reload reads persisted state instead of replaying Start.

At desktop widths of at least 1280px, the dock's left edge adjusts its width:
480px initially, bounded to 320–720px and leaving at least 480px of the available
workspace for the business page. ResizeObserver accounts for window and navigation
rail changes; a temporary clamp preserves the user's preferred width. That choice
uses the sidebar's session-only UI store, with no persistent setting. The shared
splitter supports Left/Right (16px, or 64px with Shift), Home/End, Enter to close
and Escape to cancel a drag, with localized accessible width and bounds. Expanding
and returning restores the dock width. Narrow windows keep the shared Sheet.
Resizing keeps the conversation and recording mounted; closing or switching to
the Sheet preserves the draft while voice controls retain their cancellation rules.

`PageHost`, `PageBinding`, `PageSnapshot` and `NavigationIntent` are a Go port for
later structured publishers. `MatchesCurrentPage` is only a prerequisite fence:
ledger, epoch, conversation, permission revision, surface, exact page generation
and fixed expiry of at most two minutes must agree. The publisher must additionally
intersect live user/object permissions and guard unresolved reviews. Navigation
requires a host-recorded explicit user intent and a private opaque-ref mapping;
model-provided routes, DOM selectors, coordinates or approval flags are forbidden.
The current `ClosedPageHost` always refuses full page leases and model navigation.
The narrower selection tool below does not accept #204's client-only generations.

The narrower `PageSelection` carries only a kind (`overview`, `accounts`, `account`)
and optional canonical account UUID. It is an untrusted data-selection hint from
the desktop, not proof of the visible page and not a scoped principal. Runtime
admission rejects unsupported kinds and binds the selection into request dedup.
`inspect_page` with `source=evidence` resolves a new shared `get_overview`, `list_accounts` or
`get_account_valuations` capture, with backend capture time and tool source. IDs are
validated against the live household. Client metrics, labels, filters, permission
claims and routes are never sent through this selector. The separate internal
`inspect_page` with `source=content` tool below exposes supported semantic presentation facts. The model must not claim exact visible-screen
or filter parity; source timestamps/missingness remain in shared capture data.
This does not implement the broader trusted page-publication/navigation port.

Successful turns also persist up to 16 host read records, separate from model
prose: controlled tool source, read timestamp, whether that response advertised
more pages, and a conservative missing-input indicator. A reported incomplete
flag or nonempty gaps/missing-input list marks `reported`; otherwise
`not_summarized` explicitly establishes no completeness claim. Source/quote dates
remain in the original tool data. Per-response page flags never prove that every
relevant continuation was consumed. The UI exposes these records in Data evidence;
separate tool disclosures show credential-filtered input/output values. Private reasoning and credential material are never displayed.

## One-shot model navigation

Ordinary Send publishes a fresh page lease and starts the request; there is no
per-message navigation approval. Leases bind epoch/config revision, conversation,
request, source selection and a fixed two-minute expiry. Backend-issued opaque
target refs permit desktop root pages, fixed Return analysis tabs and accounts
already disclosed by controlled reads. No arbitrary route, URL, coordinate,
SQL, financial proposal or approval flag is accepted.

`navigate_page` records a request, not a completed action. Only successful full model
completion can offer an intent. Grants, intents and navigation generations live
only in runtime memory; stored run lease IDs are non-authoritative audit metadata.
Creation/deletion, new page leases/turns, Stop, configuration/key changes, page changes,
restore and shutdown revoke earlier navigation authority. Publication replaces the
page lease; navigation generations additionally invalidate new-publication and
new-turn races on the same page lease. Native history/restart cannot restore a
grant or automatically navigate.

Consumption verifies the live conversation's latest successful run, request,
config revision, source lease, navigation generation and intent before and after
live object validation. It consumes before validation, so an uncertain response
cannot be retried as the same intent. The frontend independently verifies its
private page/publication generation and unresolved financial review before and
after IPC, and honors App navigation rejection. Stop, a newer submission or
transport uncertainty also withdraws pending frontend application. Full page-
publication/mobile authorization via the broader Go PageHost port remains separate.

`navigate_page` with typed `trend` options is a finite internal-desktop presentation action. It resolves
one disclosed account ref or the current household's trend-root ref, requires an
explicit inclusive civil `from`/`to`, valuation, cash inclusion and display mode,
and optionally accepts catalog currency/asset-class filters. It replaces the
complete filter set; member/instrument filters are not supported by this action.
The model must clarify ambiguous dates, duplicate account names or unsupported
filter requests before calling. No arbitrary DOM, route, script or financial
write capability is added, and external MCP cannot discover or invoke it.

The host rejects ranges outside History Origin and the latest closed household
local day instead of silently clamping. Consumption rechecks the live account.
The ordinary App navigation path preserves Back/Forward state, including trend
display. The mounted ReturnTrendTab then acknowledges its effective query range,
account, actual controls, supported filters and finished query. Merely setting the
store is insufficient. Query failure, unavailable results, clamped ranges,
forced valuation, concurrent filter/page changes or a missing acknowledgement
within five seconds produce an explicit unverified result (the page may already
have changed), never a successful application claim. The model's tool result
remains `requested`; only the desktop reports verified completion. Partial data
keeps its ordinary visible coverage warnings and is not upgraded to complete
financial evidence. After an accepted action, every subsequent action attempt
revokes the pending target and permanently spends that turn, including malformed,
oversized, cancelled or unknown-target requests and attempts using the other
navigation tool. Inventory reads do not revoke it. Before any action is accepted,
an invalid request may be corrected within the still-valid grant. New actions
after a conflict need a fresh page lease.

Remaining gates include richer financial evidence cards, broader analysis filter
navigation, arbitrary page/control coverage, financial proposal/review/receipt
families beyond the reviewed cash path, mobile/scoped principals, native GUI/accessibility, physical
devices, full power-loss/restore acceptance and real-provider encrypted-signature
compatibility. Larger periods/households need measured memory/latency evidence
before budgets are expanded. All unsupported paths stay closed.

### Safe run failure diagnostics

Failed runs expose only host-owned fixed codes: provider authentication/access
(HTTP 401/403), rate/quota limit (429), request rejection (other non-200 responses
below 500), temporary provider failure (5xx), connection failure, unsupported or
incomplete response, unusable native history, unusable local tool result, resource
budget or timeout. Unknown errors retain `provider_or_storage_failure`. The
transport records only the fixed category across SDK wrapping; provider error
bodies, arbitrary errors, request parameters and tool results are never copied
into diagnostics. These categories identify a failure class, not a verified cause
inside the provider. IPC events, durable state and the desktop use the same codes;
English, Simplified Chinese and Traditional Chinese guidance provides next steps.
A failure still leaves the last successful native checkpoint intact and requires
an explicit new request. Older generic failures cannot be retrospectively classified.


Response rejection codes use a closed `provider_response_rejected.<rule>`
vocabulary for request contract, response headers/body/terminal/item, SSE
framing/order/lifecycle/added/done/function/arguments/parts/terminal/event/EOF,
SDK conversion and engine inventory/empty output. These fixed stages/rules survive
SDK wrapping and durable restart. They contain no response content, field values,
provider event names, request/account IDs or credentials. The desktop shows only
exact recognized codes alongside localized reporting guidance; unknown strings
are never reflected. A successful complete refusal is displayable text from the
SDK refusal extension, with its original native item preserved in history.

SSE rejection also writes a warning through the existing application `slog`
logger, with the fixed failure code and a code-owned `terminal_check` category
(response validation/identity, item count, missing item completion, item mismatch,
matched, or not reached). No provider field names, values, IDs, text, ciphertext
or credentials are logged. Accepted reasoning ciphertext refreshes emit one Info
record per completed stream with only the refreshed item count, after clean EOF.
These records use the configured log level and existing bounded, rotating
`logs/nestworth.log` file beside settings; logging off remains off.

Budget diagnostics use the closed `assistant_budget_exceeded.<rule>` vocabulary:
`model_rounds`, `tool_calls`, `history_bytes`, `history_messages`, `prompt_bytes`,
`text_bytes`, `provider_payload`, `tool_result_bytes`, and `read_evidence`.
These identify the actual host guard, rather than API credit balance or an inferred
fixed timeout. `provider_payload` covers the provider adapter's request/response
byte, event or item guards; it does not assert which wire dimension was exceeded.
Known tool budget failures survive the controlled-read refusal boundary after
wrappers are discarded; other read failures retain their existing safe refusal.
The desktop uses exact allowlisted codes for localized next steps and reporting.
No raw sizes, content, prompts, account names, IDs, keys or arbitrary error text
are added. Old generic budget failures remain generic. Failed runs keep the last
successful checkpoint, emit no partial answer/evidence/navigation and can reopen.

## Provisional streaming and tool details

The runtime installs a request-scoped progress observer. Engine Generate then
selects real provider SSE; the transport validates each incoming frame while
assembling the complete response. Only observed output-text/refusal parts reach
the provisional display channel. Reasoning and tool argument fragments do not.
The SDK still receives only the fully verified terminal JSON after clean EOF;
its streaming converter is not used. Existing model-round, tool-inventory,
capability validation, authority, timeout and checkpoint gates remain in place.
Item completion and response completion must agree on all native fields, with
one narrow exception: a reasoning item's nonempty `encrypted_content` string may
be refreshed to another nonempty string in the final response. Both snapshots
must independently validate; IDs, type, status, summary, reasoning text, unknown
native fields, message text and tool identity/arguments remain exact. Missing,
null, empty or malformed ciphertext transitions do not receive this exception.
The final response's original ciphertext bytes are authoritative for SDK
conversion, checkpoint sealing and replay; no decryption or plaintext-equivalence
claim is made. Missing completion events, trailing events and incomplete EOF
still prevent history publication and tool dispatch.
Each model round resets the provisional text snapshot. Text is coalesced to at
most 20 updates/second, and the engine withholds a credential-sized suffix to
prevent an API key split across chunks from escaping. Short replies may therefore
appear only at completion. Completion, cancellation and failure remove preview
text. A refusal can be a complete answer under the existing provider contract.

`Inspect` carries a cumulative `progress` snapshot scoped by epoch, conversation,
run ID and revision. The selected active conversation observes it at most five
times/second through local Wails IPC. A reconnect inspects the current snapshot;
it never resubmits Start. Session switching cannot mix progress because query
keys and the timeline instance include epoch/conversation. No new Wails global
broadcast or raw provider-event bus is introduced. The existing run-state event
journal keeps its original contract. Preview snapshots are memory-only and never
stored as successful text, evidence or native history.

Tools emit start immediately before dispatch and completion/refusal/failure when
they return. A run ending during a tool marks it interrupted, without inventing
an output or a rollback. The compact row expands actual input and returned output
as inert text. Account names, amounts and exact financial number lexemes remain
visible; credential fields and the active provider key are removed. Each input
or output has an 8 KiB display cap, and the entire attempt has a 64 KiB detail
budget, with explicit truncated-prefix labels. Display truncation does not alter
what the tool or model receives. No raw error text is shown. Finished attempts
persist these bounded details in the same private sidecar and retention/deletion
boundary as the conversation, including failed attempts; the last successful
provider checkpoint still advances only for a complete successful answer. Old
runs lacking details remain valid. Abrupt restart retains the admitted attempt,
marks it interrupted and does not restore transient previews or unfinished tool
details. No financial storage schema or permission changes are involved.

Completed replies and incomplete previews use the same allowlisted Markdown
renderer (GFM tables, lists, code, emphasis and visible footnote references/text). Raw HTML is dropped. Images are
inert alt text, links are inert labels, and no Markdown resource can load remotely
or trigger desktop navigation. Unclosed fences/links are parsed safely again on
each snapshot. User prompts remain plain text. ResizeObserver follows viewport
and transcript height changes (Markdown, width and tool disclosures) only when
following is enabled. Scrolling up pauses follow; the explicit Latest messages
button resumes it. History prepend anchors a visible message element, so concurrent growth at the streaming tail does not change the reading position. Chunk
text is not a live region; tool status changes and the completed-reply announcer
provide bounded polite announcements. Native Mac visual and screen-reader
acceptance remain separate from jsdom geometry tests.

## On-demand internal page content

`inspect_page` with `source=content` belongs only to the installed built-in desktop session. The
shared capability registry and external MCP neither discover nor dispatch it.
Reads are on demand and need no separate approval, publication or manual refresh;
configured AI opt-in and the provider privacy notice still apply. No read grants
navigation, arbitrary clicks, field edits or financial-write authority.

AppShell owns one `BusinessPageRoot` around business content. The sidebar, global
navigation, assistant trigger and conversation are outside it. The root binds the
current view/object selector and a workspace/incarnation identity; these are local
freshness fences, never backend object permissions or model-supplied authority.
Every business page mounted in this shell is covered by default, including future
pages, without a page or field-name allowlist. Explicit marked component roots
remain usable in isolated consumers; a unified root takes precedence and an
identity mismatch cannot fall back to another root. Desktop runtime validation,
epoch/config revision, live workspace checks and cancellation remain authoritative.
Mobile/scoped principals and external applications gain no new reader.

The bridge is independent of assistant visibility and the local page-guide opt-in.
It polls request IDs only (750 ms idle, 150 ms after delivery), not page contents.
Each request captures the current synchronous DOM/ARIA properties and expires
after ten seconds. When the assistant expands over the business surface, the
trusted shell marks it occluded: its mounted business state stays readable and
this limitation is reported. Ordinary hidden routes/tabs, private regions and
unmounted content remain excluded.

The tree contains typed roles, accessible names, parent/module relationships,
business field values, native checked/selected properties, disabled/readonly/
invalid/busy/expanded/pressed/current/sort states and descriptive affordances.
Native selects expose eligible public option labels, not stored option IDs;
private/hidden option or optgroup ancestry is inherited, with redacted selection
state when excluded. Shared date controls
publish their canonical business date. Ordinary financial amounts, draft values,
notes and textarea contents are readable; financial data is not classified as a
credential. No raw HTML, URL attributes, event handlers, DOM commands or machine IDs from
select options are exported. Accessible-name references are restricted to eligible
owned content and cannot read into a private ancestor or the surrounding shell.
Presentation remains untrusted data, not instructions or financial authority.
Authoritative assertions still require controlled ledger reads.

Modules derive automatically from semantic sections, shared cards, forms, articles,
dialogs and tab panels, using their accessible labels/headings. Existing authored module hints
may improve names but are optional and do not authorize reading. Selecting a
module includes its descendant modules, not sibling modules; the response lists
the actual mounted module inventory. Business pagination and virtual scrolling
remain separate from node pagination: capturing or continuing never presses Load
more, scrolls, retrieves unmounted records or invokes a provider. Chart canvas/SVG
geometry, hover-only data and custom unowned popups remain limitations; mounted
semantic chart tables and owned disclosures are included. These limits prevent
claims of whole-ledger or whole-application completeness.

Shared PageChrome, Sheet, AlertDialog, Popover/date picker and ActionMenu publishers
carry the business owner plus a nonvisual origin marker at their actual source.
Only published portal contents with a live visible source in the current root or
an already-validated owned portal are read. Hidden retained pages/tabs, private
origins, other workspace owners and assistant portals are refused. Nested popovers
inside sheets retain this provenance. Unpublished custom portals require shared
component integration; the collector does not scrape arbitrary document.body.

SecretField uses a persistent shared SecretRegion, independent of password/text
visibility. AI/provider/backup keys, mobile token input and connection URL/QR
regions, and MCP connection configuration remain marked private even when shown,
readonly or rendered as ordinary text/JSON. The collector emits only a generic
redacted region, and rejects private text reached through label references.
Password/hidden/file input handling remains a second defense, not the credential
classification rule. Backend known-credential rejection remains an additional
check; it does not replace these boundaries. New credential UI must reuse the
shared secret boundary, whereas ordinary business forms need no per-field opt-in.

Capture stays bounded to 8,192 semantic nodes and 512 KiB, with bounded structural
depth/visits and at most 64 published portal candidates. Excess fails closed rather
than silently claiming completeness. Long text uses bounded Unicode-safe chunks;
shortened accessible labels are reported. Individual public control values use a
Unicode-safe preview of at most 2,048 UTF-8 bytes, matching the Go page-text bound.
Overlong fields carry `value_truncated=true` plus an explicit limitation, rather
than making the rest of the page unreadable; the business value is never mutated.
Complete live property comparison in a weak, element-scoped cache supplies a
private revision for truncated fields, so edits outside the preview still revoke
old snapshots. Full values/revisions are not added to tool output or persistence.
A truncated field never establishes complete selection coverage. Responses are at most 24 KiB, default
32 nodes and maximum 64; models select a module and explicit cursor. The module
directory is separately paged at 64 entries: `modules`, `moduleOffset`,
`moduleCount`, `modulesComplete`, `hasMoreModules` and `nextModuleCursor` describe
that directory page. `moduleCursor` requires the same snapshot ID after offset
zero and is revalidated like node continuation. A known module can be selected
without fetching the entire directory, so a large directory cannot unconditionally
inflate every response beyond the tool budget. `complete`
can be true only when the entire requested selection fits and has no limitations.
An advertised continuation or missingness flag remains per-response evidence.

Each continuation recaptures the current state before returning the frozen run's
next slice. Changed captured controls, modules, portals, query state, view/object/workspace
identity, host visit or root remount reject the old snapshot; a fresh initial read
obtains current state automatically. Snapshot state is run-local and memory-only;
normal successful model history may retain tool results under conversation policy.
Every supported page binds default navigation to this same
semantic generation before and after async intent consumption. The fixed opaque
navigation target inventory, source lease, successful-run requirement, expiry,
workspace validation and unresolved-review checks are unchanged. In particular,
generic reading does not add routes, arbitrary UI control or financial writes.
Native GUI, real-provider and physical-device acceptance require separate evidence
from component/protocol tests and builds.

### Conversation titles

The host persists an optional title, source (`local`, `pending`, `luna`, or
`manual`) and one-shot attempt marker in the private conversation sidecar.
Existing untitled history receives a local title at startup; it is never sent
back to a provider merely because the app upgraded or restarted. Older app
versions with strict sidecar decoding may reject the added optional fields;
do not downgrade against the updated private sidecar.

New conversations immediately receive a whitespace-normalized, 48-code-point
excerpt of the first question. After the first successful answer the host makes
at most one separate `gpt-6-luna` Responses request using the existing provider
and credential. There is no extra key, tool/session access, page content, native
history, assistant answer, or ledger attachment. Only the first user question,
limited to 512 code points, accompanies title-only instructions. User-supplied
question text itself may contain financial details. Provider availability is not
assumed: unavailable models, errors, malformed/empty results, cancellation and
a ten-second deadline retain the local title. There are no automatic retries,
including after restart. This additional request can incur provider charges;
the settings and conversation information disclose the model and behavior.

The answer is durable before title work starts, and later turns do not wait for
it. Title workers are isolated by conversation and fenced by runtime/configuration
revision. Disable, credential replacement/deletion, restore, shutdown and
conversation deletion cancel outstanding work; late results cannot recreate a
conversation or replace a manually named title. Desktop Rename persists manual
ownership even when the text equals the existing automatic title. Names are
limited to 64 Unicode code points, reject control/format characters and the active
credential, and render as text. Retention and ledger binding are unchanged.
The UI polls authoritative title metadata and supports keyboard rename, cancel,
and localized English, Simplified Chinese and Traditional Chinese labels.

### Generic current-page operations

The desktop's `inspect_page` with `source=content` tree also discovers native/shared interactive
controls. `targetRef` is an opaque identity for the actual mounted element, held
only in the host's weak map; it is neither a selector nor a business ID. Replacing
a same-label element changes the snapshot. The same business root, workspace/view
generation, secret boundaries and recursively owned portal rules govern reading
and operation discovery. Hidden/unmounted content, foreign portals and credentials
never become operation targets. Native select options have separate opaque refs,
so selection does not require exposing internal option values or guessing names.

`operate_page` is an internal desktop capability, unavailable to external
MCP. Audited presentation controls are enabled in Read-only; other controls require
Full. Roles such as tab, filter and disclosure do not prove a callback has no
durable effect. Dedicated navigation and return-trend tools retain freshness and
completion verification without per-message approval.
This extends the shared bridge rather than adding a per-page tool registry.

Supported operations are button/shared-role activation, native text/number/date
input and textarea values, and exact single-select option selection. They invoke
normal React/native handlers; business validation, queries, form preview,
confirmation and transaction protection remain owned by the application. Custom
comboboxes and date pickers can be operated through their mounted triggers,
search fields and published options/calendar buttons. Links (including external
and native integrations), files, arbitrary keyboard/event injection, arbitrary
routes, HTML, canvas/SVG targets, contenteditable, multiselect and unmounted list
items are unsupported. Controls still need meaningful accessible names and states.
No synthetic financial calculations or direct service calls are introduced.

Each tool call validates a fresh complete semantic generation before enqueueing.
The desktop claims the request once, rechecking current runtime epoch, revision,
the target effect and required financial permission, run cancellation and unresolved review state. Claims carry a pre-dispatch expiry of at
most two seconds and are never republished. Expiry is checked before dispatch;
it cannot cancel a handler that has already started. Immediately before synchronous
dispatch the frontend recaptures the page, resolves the exact target, checks the
current view/workspace generation and rejects disabled, readonly, busy, inert,
secret or occluded controls. An assistant modal or expanded assistant can leave
business content readable while making its controls inoperable; use the visible
side-by-side desktop surface for operations. Nothing clears modal/inert guards.
Existing unresolved Sheet confirmations and assistant financial reviews block
page operations, including attempts to close/navigate away or replay submission;
ordinary Sheets retain their recovery flow; assistant receipts reconcile through background inspection. Generic page
operations do not replace or widen the dedicated Full ledger command contract.

The result states are `dispatched`, `blocked`, `stale`, `invalid_target`,
`not_dispatched` and `unverified`. `snapshotId` in that result names the **source
observation**, not a post-action snapshot. `dispatched` proves event delivery only: it is never proof
of validation, query completion, successful navigation or a financial commit.
The model must read fresh page state afterward, observe loading/error/validation
feedback, and verify financial success with authoritative records. Identical
gestures on one snapshot are not replayed; freshly observed state permits an
intentional further step (for example advancing a second calendar month). A lost
response, dispatch exception or timeout fences further operations for that run;
unknown financial outcomes must be inspected, never retried automatically.
Cancellation before claim prevents dispatch; after an accepted claim, cancellation
cannot undo an already dispatched UI handler or financial transaction. A stopped
frontend does not dispatch a late claim response. Existing financial idempotency
and recovery rules remain authoritative across runs and process restarts.

The bridge returns bounded metadata and polls only pending IDs; full semantic
capture occurs on demand, not every polling interval. Each operation adds one
runtime freshness read and one frontend pre-dispatch capture. No whole-ledger
scan, background provider request or persistent operation queue is introduced.

Security administration is outside generic page-operation authority even in Full
mode. `SecurityOperationRegion` keeps non-secret status and labels readable, but
omits operation targets/actions and marks `operation_protected=true`. It covers
MCP/mobile access and token generation, AI credentials/model configuration and
permission switches, provider credential submission, cloud backup destinations
and access operations, and credential-bearing backup/export/restore controls.
Ordinary formatting preferences and business controls remain operable. This is an
operation boundary, distinct from `SecretRegion` value redaction: revealing a
credential still never publishes its value. Full ledger permission cannot be used
to grant or reconfigure another access channel or to administer its own authority.

Shared portal context and source provenance propagate operation protection through
nested portals. External controls associated with a protected form also cannot
submit/edit it. Assistant occlusion propagates from the owning business root to
already-mounted owned portals; removing occlusion requires a fresh snapshot.
Synchronous React handler failures can be reported through `window.error` rather
than thrown by `click`; the dispatcher observes that event only for the dispatch
interval, returns `unverified`, and always removes its observer. This does not
suppress application error reporting. Later asynchronous failures still require
fresh page/authoritative reads: `dispatched` remains event delivery, never task or
financial success.

Operation refusals always echo the requested source snapshot ID, including stale
refusals; they never disclose an unseen replacement observation. Delivery checks
that source ID against the claimed request. A stopped host or expired lease
observed before dispatch delivers `not_dispatched` once. If that acknowledgement
is lost, the claimed request remains uncertain. On timeout, the runtime atomically
removes an unclaimed request and reports `not_dispatched`; a claimed request
without an accepted result stays `unverified`, even after its lease expires.
A result accepted concurrently with timeout takes precedence. Only unknown
outcomes fence the rest of the run; definite refusal requires fresh discovery
before any new intentional operation and never triggers automatic replay.

The bridge rechecks its claim deadline and live host authority after semantic
capture and immediately before the first DOM effect (click or native value/index
setter). The live check covers stopped host, Full access, unresolved flow,
view and workspace generation. Additional input/change events recheck the same
fence; a refusal after a setter or earlier event returns `unverified`, since a
partial effect cannot be called definitely undispatched. These synchronous
checks do not promise to observe backend revocation before it reaches host state.
Human changes to the desktop permission select additionally revoke frontend
dispatch authority synchronously, before IPC. Operations stay paused through
status reconciliation; claims from before or during that change cannot dispatch
later, even if the permission request fails. New claims still require the
backend's current revision and permission, and Read-only presentation operations
resume after reconciliation. Already dispatched handlers cannot be undone.

`set_value` accepts at most 2,048 Unicode code points and 8,192 UTF-8 bytes, with
no NUL; empty strings are allowed. JSON Schema `maxLength`, frontend code-point
counting and backend rune counting use the same limit. The bounded raw argument
envelope is 32 KiB so equivalent literal or JSON surrogate-escaped emoji at the
limit are both accepted. Existing financial/business field validation remains
unchanged; the semantic read preview still has its separate 2,048-byte limit.

Full MCP writes, the legacy direct ledger wrapper and generic page operations
share one in-memory run write gate. It serializes admission through outcome
classification. An uncertain page result fences both Full write routes for the
remainder of that run with a recovery-required diagnostic, while read-only MCP
inspection remains available. Full MCP unresolved recovery references and busy
admission also refuse page claims. This gate does not persist across runs and
does not replace the MCP operation identity, durable recovery reference or
cancelled-before-commit proof.

Explicit page argument/schema rejection, stale source snapshots and absent
advertised targets carry a per-attempt not-dispatched proof before enqueue. The
provider reports `write_not_submitted` for these refusals instead of inventing an
operation receipt to inspect. Cancelled capture, prior gesture conflicts, shared
run uncertainty and every post-enqueue failure retain conservative diagnostics;
the proof never resolves an older uncertain operation.


### Default desktop page interaction

Ordinary Send publishes a fresh page lease and admits built-in navigation without
an approval button or GrantNavigation IPC. Leases, cancellation, generation,
workspace identity, unresolved financial recovery and one-shot consumption still
protect against stale dispatch; they are freshness checks, not user approvals.
Fixed root targets include asset changes and historical overview. Read-only / Full
remain the financial modes selected in the desktop UI.

`operate_page` has the `interaction` effect, not a blanket financial write
effect. The host classifies a fresh semantic target: audited presentation regions
(time/range/scope filters and analysis tabs) permit interaction in Read-only;
unclassified controls require Full. DOM roles alone never establish read-only
semantics. Published owned portals inherit their source's presentation effect.
Security/credential boundaries override both modes. A model cannot submit the
classification: backend admission reads the current host capture and frontend
dispatch independently checks the fresh DOM. Dispatch still is not a receipt.
The existing run write fence serializes interactions with mutations and preserves
uncertain outcomes, single-use claims and same-ID financial recovery.

A proven pre-dispatch page refusal is returned as `page_operation_refused` with
`not_dispatched`, permitting fresh discovery. Unknown write results still stop
the turn. Generic tool failure copy does not call every tool a local read.

Page context is optional at ordinary Send admission. Capture budget errors,
publication failure or a changed observation drop the deferred navigation lease
without dropping the question or retrying Start. A lease replaced or expired
between publication and Start is likewise ignored by runtime admission. Epoch,
configuration, conversation and financial permission checks remain mandatory.
Only a fresh accepted lease may produce/consume a deferred navigation intent;
on-demand page reads and control operations retain their independent current
snapshot checks. An unavailable observation never becomes navigation authority.

Usage accounting, coverage limits, cost basis and account model selection are
specified in [Built-in AI usage and account models](ai-usage.md).

The current model catalogue and selection contract is [offline model selection](ai-model-selection.md); Settings no longer fetches `/v1/models`.


## Consolidated desktop page tools and sequential actions

The built-in catalog exposes exactly three page tools: `inspect_page`,
`navigate_page` and `operate_page`. Retired identities are not public aliases.
Inspection selects `source=content` (default semantic page tree), `evidence`
(authoritative selected-page backend capture), or `navigation` (opaque destinations).
Only content accepts snapshot/module/pagination fields. Navigation takes an exact
`targetRef` and optional closed, typed `trend` object preserving the required
civil dates, valuation, cash inclusion and display plus optional currency/asset class.

`operate_page` takes `{snapshotId, actions}` with 1–20 ordered steps and a
256 KiB total JSON envelope ceiling. Each step
has `targetRef`, an advertised action, and the action's value or option ref.
The entire envelope is validated before the first dispatch. Each step uses a
fresh host capture and the existing runtime permission, protected-region,
review, disabled/busy/readonly and single-use checks. Later steps may bind to a
new snapshot only when the same original mounted element and page authority
scope still exist. No name/position remapping, arbitrary selectors, automatic
replay, additional permission mode or per-step approval is introduced.

Results list attempted steps and preserve `partial` progress. A refused,
replaced, stale or unknown step stops the suffix; earlier effects are not rolled
back. `dispatched` means delivered UI events, never successful validation or a
financial commit. A final observation may identify fresh state; inability to
observe does not erase acknowledged dispatch. Inspect current state and consult
shared authoritative evidence before claiming completion. Existing unresolved
financial outcomes continue to fence further financial work.

Read-only now discovers all currently permitted shared closed-world Read tools,
plus the existing strictly in-memory attribution adapter. Snapshot-maintaining
Analysis, Preview, Write and open-world tools remain unavailable in read-only.
Read & write adds shared mutation capabilities. Both modes use the same turn-local
loader: loading advertises definitions, never grants execution authority. Dispatch
rechecks the current shared catalog and permissions; restored histories cannot
recreate a grant. Core schemas and sorted optional group schemas form a stable
append-only prefix within a turn, while the next turn starts from core again.
This preserves cache opportunities without claiming measured live cache hits,
latency or billing savings.

## Shared business recipes

The built-in model does not discover or execute the portable `nestworth` skill,
read `SKILL.md`, or load installed references. Its host-owned system/mode policy
and controlled tool metadata remain the instructions it receives. Compact neutral
business recipes have one source in `internal/agentguidance/recipes.json`.
`builtinai.Session` attaches each recipe to its optional entry-tool description;
`load_tool_groups` therefore provides it with that group's schemas on the next
response. No extra tool, filesystem access, model round or skill runtime is added.
The core catalog is unchanged, loaded groups reset every turn, and history cannot
restore schemas or permissions. Recipe text is host-owned metadata; page and
business result text remains untrusted data.

The portable skill's task-specific references contain generated copies, checked
by `tools/check-nestworth-skill.py`; maintainers refresh them with `--sync-recipes`.
The shared source and distributed references require the same skill version bump.
Only business interpretation and workflow knowledge is shared. External MCP
connection/discovery, its analysis-capable read-only mode and identical-retry
recovery remain in that skill. Built-in strict Read-only, memory-only attribution,
Full permission, page controls and conservative unknown-write fences remain in
its adapter policy. Sharing a recipe grants no authority.

`get_context.mode` and capabilities reflect the built-in model's current effective
shared tool inventory, rather than the host storage session's ReadWrite mode.
Household/history/source metadata stays unchanged. External MCP retains its own
mode and capabilities. Investment-review preview/create/update publish a bounded
closed spec grammar; domain normalization, semantic conditions, exact money,
source admission and mutation recovery remain authoritative.

### Tool contract diagnostics

Shared schema validation returns only schema-owned field paths, constraint names
and allowed enum values (`field`, `rule`, `expected`), never rejected values or
raw validator messages. Batch paths retain the command index. Built-in reads and
previews preserve the same business code, field and message as external MCP,
including newly introduced business codes and fields. Recovery guidance remains
separate from the diagnostic, and an unfamiliar code does not erase its message.
Shared preflight includes typed decoding, not only JSON Schema validation. A
schema-valid number that the typed handler cannot decode (for example `1.0` for
an integer field or an out-of-range integer) is rejected with `typed_decode`
before business dispatch or receipt creation. Original numeric lexemes are not
rewritten, preserving exact input and idempotency fingerprints. The dispatch
observation is set only after successful typed decode and the last cancellation
check, immediately before business handler entry. Proven pre-dispatch write
refusals include the cause and `not_dispatched`; unknown post-dispatch outcomes
remain inspection-only. The external MCP adapter uses the same validation before
SDK dispatch. Ledger variants publish their
allowed fields, required values, reason/side enums, paired charges and mutually
exclusive time shapes; correction uses its own fix/undo constraints.

### Product contracts and durable failure diagnostics

Product lifecycle, terms and liquidity-policy tools share enum, settlement-range
and conditional policy schemas. Local product timestamps cannot be combined
with a nonempty local date/time pair; empty local DTO defaults remain accepted.
Managed term deposits retain their maturity-derived unlock date. Decimal/date
parsing, revision checks and financial invariants remain in the domain layer.

The shared schema diagnostic traverses refs, allOf/anyOf, conditionals and
range/length constraints, publishing only schema-owned paths and bounds. The
provider preserves shared product/history business messages, codes and nested
fields without maintaining another diagnostic allowlist.
Only the durable operation gate may mark an exact failed receipt; the provider
returns its safe cause as `write_failed`, distinct from unsubmitted and unknown
outcomes. It is not permission to automatically replay a write.

The obsolete `PreviewFullChange`/`ExecuteFullChange` production methods and their
private single-change preview writer are removed. Legacy review inspection and
cancellation remain for stored recovery references; legacy receipt creation
exists only in test fixtures. New writes use the shared registry and commit path.

Interest mode dependencies are also shared by lifecycle and terms previews:
none/manual/simple modes constrain rate, maturity-interest and paid-through
fields, preserving percent-over-ratio selection and trimmed empty decimals.
Lifecycle creation (open, record-existing and renew) accepts omitted/null
paid-through dates and lets the business layer default them to startOn. Terms
replacement retains its explicit-date requirement for simple interest.
Diagnostic subtree validation clones schema nodes before attaching root `$defs`;
otherwise a shared nullable definition could violate the validator's tree rule
and hide the actual invalid field. Type messages include the schema-owned path
and allowed JSON types in both MCP and built-in AI responses.

### Ledger argument recovery

Built-in AI and MCP share ledger validation and business recipes. Unknown required
`kind`/`action` discriminators report the exact field and schema-owned allowed
values; partial unions retain their general diagnostic. Salary uses
`money_added` with reason `income`; spending uses `money_removed` with `expense`.
Local ledger clocks accept `HH:MM`. A rejected seconds-bearing clock points to
`effectiveLocalTime` and explains how to preserve seconds using RFC3339
`effectiveAt` with a confirmed offset, omitting both local fields. Never truncate
seconds or guess timezone offsets. This format refusal permits input correction;
DST ambiguity and other time/state conflicts retain their existing recovery.
Preview remains non-posting and commit-plan idempotency remains authoritative.
