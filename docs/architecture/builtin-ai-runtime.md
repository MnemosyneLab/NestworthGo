# Built-in AI desktop read-only runtime

The desktop right-hand assistant supports explicit OpenAI Responses model requests,
controlled local financial reads, run feedback, cancellation and paged persistent
history. Activation defaults off. Desktop Settings → Built-in AI credentials saves
a write-only key, a model ID and an explicit enable flag. There are no startup,
reconnect or implicit retry model calls. Financial writes and mobile model access
remain closed. The existing deterministic page guide remains available on mobile.

The enabled vertical covers current overview, accounts, closed-period attribution
and one-shot model-selected navigation after explicit per-message authorization.
Financial proposals/writes, arbitrary page inspection, provider refresh, arbitrary
UI automation and mobile model access remain unavailable. Existing explicit
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

Each live conversation owns a direct `InProcessSession` in `ReadOnly` mode through
`builtinai.Pool`; the provider engine and credential lease remain per-run. This
preserves frozen captures across turns within the original fixed five-minute TTL.
Page/item reads never renew it. The pool is bound to this runtime/ledger and at
most 20 conversations. Delete/retention drops the conversation session; config/key
changes, disable, restore and shutdown revoke all sessions. Revocation is
nonblocking under runtime/restore locks; draining occurs outside the restore gate.
No local HTTP,
external MCP token, generic Wails dispatch or open-world capability is used.
Capture/cursor state does not gain fresh authority from chat history. Reads use
the existing cropping, page limits, missingness and exact-money contracts.
`internal/builtinai` intersects discovery and dispatch with a fixed read allowlist:
`get_financial_context`, `get_financial_context_item`,
`get_overview`, `list_accounts`, `get_account_snapshot`, `get_read_page` and
`get_read_item`, `compare_financial_context` and `compare_financial_attribution`.
Continuations use `get_read_page`; the former explicit-scope financial page tools
were removed. It adds `get_current_page` only for supported
view selections, and `get_navigation_targets`/`open_page` only for an explicit
per-message host grant. The shared session independently rechecks permissions
and budgets.
No configuration, network refresh, projection-maintaining analysis, financial
writer/preview or approval method is available. The host
review methods from #203 remain separate and are not registered by this stage.
Their operation receipts remain in their existing household/session namespaces;
chat deletion neither deletes those receipts nor reverses a financial operation.

The current authority is desktop, household-wide, read-only. No mobile/scoped
principal may receive it. Trusted proposal/review UI, live mobile permission
intersection, per-account authorization and other write families remain required
before any built-in writer can be exposed. The model cannot manufacture host
approval from text, a summary, a digest, a page snapshot or a saved conversation.

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
read records. JSON escaping can expand the 80 KiB prompt/display text budget;
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
read with a writer dispatches neither. Cancellation after `response.completed`
but before EOF also dispatches nothing. There is no provisional token display, SDK request retry, automatic run replay,
remote checkpoint, compactor or summary replacement. A refused controlled tool
returns a fixed model-visible error without implementation details or financial
facts. Within the existing 8-round/16-call budget, the model can re-capture an
initial read and restart its relevant pages; it cannot revive expired references,
combine old/new capture IDs or widen permissions/disclosure. Provider/framing,
context cancellation and invalid/oversized result failures still fail closed.

A successful checkpoint retains the complete Eino messages, native output groups,
opaque reasoning, tool call/result order and provider/model identity. It is never
rebuilt from display text. The next actual SDK request validates/replays those
native groups in place. After loading a checkpoint, each turn replaces only the
first host-owned system policy with current installed policy. Assistant-native
outputs and opaque reasoning remain byte-preserved; old capability claims cannot
suppress newly installed features or grant removed authority. Changing models cannot silently reuse the old model's
opaque state; configuration changes clear native history and retain display
turns. Context-budget exhaustion similarly
requires a new conversation until a verified compaction policy exists.

A failure after a previous read can have performed that read; it does not advance
the complete-turn checkpoint. An explicit new request may read again. No writer
is installed, and no pending tool execution is recovered or replayed. After abrupt
process exit, accepted/running records become `interrupted` and generate a durable
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
`Configure`, `PublishPage`, `GrantNavigation` and `ConsumeNavigation`.
Configuration/navigation authority is desktop-only and uses the current epoch/revision.
`Inspect` returns the latest attempted
turn; `Messages` takes an explicit zero-based offset and returns one turn plus
run count. Events contain only sequence, run ID, state and fixed code: at most
64 per call, 256 retained per conversation. Reconnect queries state/events using
the latest epoch instead of resubmitting a new paid request. A rolled-over/future
cursor returns `reset`; refresh metadata/history. Generated bindings expose
write-only credential input and bounded configuration,
never stored secrets, history internals or host review methods. The desktop polls
status every two seconds and the selected latest turn every second while mounted.
The sidebar presents a chronological transcript: the latest attempt and up to
five preceding turns load on session selection. “Load earlier messages” prepends
five more bounded `Messages` reads without replacing the visible transcript.
New attempts retain already displayed history. Scrolling away from the bottom
pauses following; “Latest messages” resumes it. Failed history reads have a
local retry that never replays `Start`.
Completed host read evidence appears in a collapsed disclosure attached to its
answer, with allowlisted localized source names, capture time and conservative
coverage/missingness. The current API has no per-tool live lifecycle events;
working indicators describe the run only and never invent tool progress.
Privacy/scope/retention information lives behind an accessible info popover.
Single-message navigation remains a separate explicit send action inside the
page-navigation disclosure; opening it grants nothing. Closing the assistant
retains the session and does not delete its persisted history. No poll invokes a model. Offline state
pauses only new model submissions. All local Status/List/Inspect/Messages queries
use `networkMode:always`; Stop and local history remain available under a browser
offline hint. Reconnect/reload reads persisted state instead of replaying Start.

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
`get_current_page` resolves a new shared `get_overview`, `list_accounts` or
`get_account_snapshot` capture, with backend capture time and tool source. IDs are
validated against the live household. Client metrics, labels, filters, permission
claims and routes are never sent through this selector. The separate internal
`read_page_content` tool below exposes supported semantic presentation facts. The model must not claim exact visible-screen
or filter parity; source timestamps/missingness remain in shared capture data.
This does not implement the broader trusted page-publication/navigation port.

Successful turns also persist up to 16 host read records, separate from model
prose: controlled tool source, read timestamp, whether that response advertised
more pages, and a conservative missing-input indicator. A reported incomplete
flag or nonempty gaps/missing-input list marks `reported`; otherwise
`not_summarized` explicitly establishes no completeness claim. Source/quote dates
remain in the original tool data. Per-response page flags never prove that every
relevant continuation was consumed. The UI exposes these records in Data evidence;
it does not render raw tool payloads, private reasoning or credential material.

## One-shot model navigation

Ordinary Send has no navigation tools/grant. The separate Send-and-allow button
publishes a fresh backend page lease, records explicit authorization for a
not-yet-started request and then starts that exact request. A lease/selector alone
cannot grant authority. Grants bind current epoch/config revision, conversation,
request, source selection and a fixed two-minute expiry. Backend-issued opaque
target refs permit only overview, account list and accounts already disclosed by
controlled reads. IDs in the target inventory are correlation data, never accepted
as `open_page` authority. No arbitrary route, URL, coordinate, DOM command, SQL,
financial proposal or approval flag is accepted.

`open_page` records a request, not a completed action. Only successful full model
completion can offer an intent. Grants, intents and navigation generations live
only in runtime memory; stored run lease IDs are non-authoritative audit metadata.
Creation/deletion, new grants/turns, Stop, configuration/key changes, page changes,
restore and shutdown revoke earlier navigation authority. Publication replaces the
page lease; navigation generations additionally invalidate new-authorization and
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

Remaining gates include richer financial evidence cards, analysis filter/date
navigation, arbitrary page/control coverage, trusted financial proposal/review/
receipt wiring, mobile/scoped principals, native GUI/accessibility, physical
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


## On-demand internal page content

`read_page_content` is added only by `builtinai.Session.WithPageReader` in the
installed desktop factory. The shared agent capability registry and external MCP
server neither discover nor dispatch it. Reading is enabled by default for the
built-in assistant and has no separate permission, approval, publish or refresh
step. Existing configured AI opt-in and the displayed provider privacy notice
apply to page content as well as controlled ledger reads. Read access grants no
navigation, DOM execution or financial-write authority.

Each tool invocation asks the mounted desktop bridge for a synchronous capture
of the *then-current* active page, including live input properties rather than
only React/backend state. The desktop host bridge stays mounted independently of sidebar visibility and
local page-guide enablement. While configured AI is available it polls request
IDs (750 ms idle, 150 ms immediately after a delivery), not page contents. Requests expire after ten
seconds and are checked against the runtime epoch, config revision, live
workspace validator, run cancellation and enabled state. Nothing is prepublished.
The current page must have an explicit reviewed root and match the current view
key, excluding retained inactive pages and the assistant's own conversation.
When a modal assistant makes the underlying active page inert, its reviewed
content remains readable and the overlay/inert state is explicitly reported.

The first supported root is Return analysis, across its active calendar, trend
and contribution tabs. Stable module paths are `page`, `views`, `filters` and the
active tab name; named controls use module/control IDs. Anonymous semantic text
IDs are stable only within the same semantic snapshot. Output consists of typed
nodes with parent/module, role, accessible name, displayed value, state and
*descriptive* affordances. Field values are restricted to reviewed analysis
control IDs; password, hidden, file, unknown and textarea inputs are redacted.
`data-assistant-private` subtrees, URLs, option database IDs, event handlers,
raw HTML and SVG/canvas geometry are excluded. Known configured credential
material is also rejected by the backend, including strings escaped in JSON.

Native select option labels/selection, shared date-picker labels/displayed values,
search combobox text/expanded state, all contribution toolbar/ranking/detail
controls (return type, grouping, ordering, count, search),
checkbox/radio values, disabled/read-only/invalid/pressed states, headings,
tables, statuses, alerts and disclosure state are represented. Roleless `aria-busy` containers become semantic groups so
background fetching changes the snapshot even when financial rows are unchanged.
Collapsed
in-root disclosures retain available semantic contents. Portal popovers and
hover-only chart content are outside this capture. Chart geometry is explicitly
reported as unsupported; an available semantic data table can be read. Other
pages return `page:unsupported`, no nodes and explicit limitations; the original
backend overview/account selection tool remains independent. This is not full
application or arbitrary UI control coverage.

A capture is bounded to 8,192 nodes and 512 KiB; excessive content fails closed
without claiming completeness. Long text is split into 400-character nodes
without discarding the remainder; shortened accessible labels are reported as a
limitation. Responses default to 32 nodes (maximum 64) and at most 24 KiB. They
include snapshot ID, capture time, module directory, selection, total nodes,
offset, `hasMore`, optional `nextCursor`, limitations and `complete`. The latter
is true only if the *entire requested selection* fits this response and has no
limitations. It never means all application content is supported. Read records
retain per-response pagination/missingness evidence.

Continue with the same `snapshotId`, module and next offset. Every continuation
captures the current semantic state again before returning the original frozen
snapshot's next slice. A changed semantic generation rejects continuation; a new
initial read without a snapshot ID captures the latest state automatically.
Generation fingerprints include the host visit and root identity, so leaving and
returning or remounting a page revokes old snapshots even when content matches.
Snapshot state is run-local and memory-only; tool results may enter the normal
successful model history under the existing conversation retention rules.
Financial assertions must still use authoritative controlled ledger reads.

Under the existing explicit one-turn navigation grant, opaque target discovery
also offers Return analysis's calendar, trend and contribution destinations.
These are fixed typed read-only routes; they preserve existing analysis filters.
Analysis navigation also compares the captured semantic generation before
intent consumption and again after IPC, rejecting intervening filter/data changes.
No arbitrary click, edit, date/filter application or financial action is added.
The existing successful-run, source-page lease, expiry, live workspace and
unresolved-review checks still gate dispatch. Future pages require reviewed
root/field coverage and privacy/state regression tests before opting in.
