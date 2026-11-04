# Built-in AI desktop read-only runtime

The desktop right-hand assistant supports explicit OpenAI Responses model requests,
controlled local financial reads, run feedback, cancellation and paged persistent
history. Activation defaults off. Desktop Settings → Built-in AI credentials saves
a write-only key, a model ID and an explicit enable flag. There are no startup,
reconnect or implicit retry model calls. Financial writes and mobile model access
remain closed. The existing deterministic page guide remains available on mobile.

The first enabled vertical is current overview, account directory and account
detail. Period returns, financial proposals, arbitrary page inspection, provider
refresh and model-driven navigation are unavailable and the UI discloses those
limits. Existing explicit account target buttons keep the frontend page host's
expiry, opaque-target and unresolved-review guards.

## Ownership and authority

`internal/assistant` owns private state, transitions, model/credential/storage
ports, fixed errors and the future page-host contract. The executable derives
the namespace from the canonical database path and household UUID. The runtime
gets a fresh host epoch on every open, pins the application's workspace
incarnation and rechecks it on every public action and before result delivery.
Callers cannot select paths, a household, a mobile principal or a tool permission.
A missing household leaves the backend unavailable until a later application
restart; onboarding does not silently activate it.

Each model run creates its own direct `InProcessSession` in `ReadOnly` mode,
owned by the conversation UUID. It is closed when the run ends. No local HTTP,
external MCP token, generic Wails dispatch or open-world capability is used.
Capture/cursor state does not gain fresh authority from chat history. Reads use
the existing cropping, page limits, missingness and exact-money contracts.
`internal/builtinai` intersects discovery and dispatch with a fixed read allowlist:
`get_financial_context`, `get_financial_context_page`, `get_financial_context_item`,
`get_overview`, `list_accounts`, `get_account_snapshot`, `get_read_page` and
`get_read_item`. It adds an empty-argument `get_current_page` only for supported
view selections. The shared session independently rechecks permissions and budgets.
No configuration, network refresh, projection-maintaining analysis, writer,
preview or approval method is available. The host
review methods from #203 remain separate and are not registered by this stage.
Their operation receipts remain in their existing household/session namespaces;
chat deletion neither deletes those receipts nor reverses a financial operation.

The current authority is desktop, household-wide, read-only. No mobile/scoped
principal may receive it. Trusted proposal/review UI, live mobile permission
intersection, per-account authorization and other write families remain required
before any built-in writer can be exposed. The model cannot manufacture host
approval from text, a summary, a digest, a page snapshot or a saved conversation.

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
128 messages and a total run deadline of at most 120 seconds. Each model round
keeps #201's own byte/event/item/60-second limits. A response yields an executable
message only after terminal success, full item/framing validation and clean EOF.
The whole call inventory is checked before dispatch; a response mixing a valid
read with a writer dispatches neither. Cancellation after `response.completed`
but before EOF also dispatches nothing. There is no provisional token display,
implicit model retry, remote checkpoint, compactor or summary replacement.

A successful checkpoint retains the complete Eino messages, native output groups,
opaque reasoning, tool call/result order and provider/model identity. It is never
rebuilt from display text. The next actual SDK request validates/replays those
native groups in place. Changing models cannot silently reuse the old model's
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
`Configure`. Configuration is desktop-only and uses the current epoch/revision.
`Inspect` returns the latest attempted
turn; `Messages` takes an explicit zero-based offset and returns one turn plus
run count. Events contain only sequence, run ID, state and fixed code: at most
64 per call, 256 retained per conversation. Reconnect queries state/events using
the latest epoch instead of resubmitting a new paid request. A rolled-over/future
cursor returns `reset`; refresh metadata/history. Generated bindings expose
write-only credential input and bounded configuration,
never stored secrets, history internals or host review methods. The desktop polls
status every two seconds and the selected latest turn every second while mounted.
History pages load on explicit selection. No poll invokes a model. Offline state
pauses actions; reconnect/reload reads persisted state instead of replaying Start.

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
claims and routes are never sent. The model must not claim exact visible-screen
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

Remaining gates include richer financial evidence cards, trusted proposal/
review/receipt wiring, broader page/navigation coverage, mobile/scoped principals,
native GUI/accessibility, physical devices,
power-loss/full backup-restore application acceptance and real-provider encrypted
signature compatibility. All unsupported paths stay closed.
