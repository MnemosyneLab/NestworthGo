# Built-in AI usage and account models

## Request accounting boundary

The desktop host records **each dispatched HTTP request**, not an Eino message,
UI poll, tool result, or replayed conversation history. A local UUID identifies
one attempt. Its pending record is upserted by its terminal observation, so a
multi-tool turn sums all Responses rounds once. No accounting request is sent
to OpenAI. Existing no-retry, no-redirect and completion/EOF gates remain intact.

The pinned OpenAI Go SDK v3.44.0 represents usage counters as integers with
presence metadata. Eino agenticopenai v0.2.4 `toTokenUsage` copies the integers
into an always-present structure and loses presence. Therefore the transport
reads allowlisted metadata from raw JSON or the SSE terminal response **before**
SDK conversion. It does not persist the response body. `response.incomplete`,
`response.failed`, and completion followed by a protocol error may still provide
billable usage; these counters survive a failed round. No terminal usage means
unknown, including network errors, cancellation and HTTP errors. Request status
means transport/protocol completion, not success of the whole assistant turn or
of a financial operation.

Nullable input, cached input, cache-write, output and reasoning counters preserve
omission separately from explicit zero. Negative, fractional, excessive or
impossible subset counters become unknown. Cached/cache-write are input
breakdowns; reasoning is already included in output. Total tokens sum input plus
output for calls with both counters; partial-call input/output subtotals remain
available separately. Neither reasoning nor cache fields are added again.

Records retain the requested model and, when reported, the actual historical
model. Missing actual model is displayed with `(?)`; changing the configured
model does not relabel older calls. UTC host start and finish timestamps preserve
attempt timing. Pending attempts after restart remain unconfirmed: they are not
retried, treated as free, or promoted to success.

Title generation uses the same request instrumentation, attributed to the
conversation but not its latest user turn. Transcription is recorded separately
because audio can be submitted before any conversation exists. Its provider
input/output token counts are retained if present; duration-based usage retains
exact decimal seconds separately. Browser recording length is never used as a
billing duration. Audio and transcript text are never stored in usage metadata.

## Storage, privacy and failure behavior

`assistant-private/<ledger>/usage.db` is independent of `state.db` and the
financial database. It uses a private 0700 directory, 0600 SQLite file, FULL sync,
WAL journaling with a dedicated read connection, the parent conversation directory's exclusive lock, and
versioned bootstrap. Existing nonprivate files and symlinks are refused. No
financial schema migration or backup/export selection changes are required.
Conversation deletion and automatic conversation retention remove content but
retain accounting metadata. Deleted conversations have anonymous UI labels;
the usage database does not retain their titles, messages, page captures, tool
arguments, response IDs, credentials or encrypted reasoning content.

Accounting writes have a 100 ms context deadline and a 50 ms SQLite lock wait.
They run outside the runtime conversation mutex; failures and observer panics
cannot fail chat. Failed writes set a runtime incompleteness flag and attempt a
0600 `usage-incomplete` marker for later restarts. If the filesystem cannot even
write that marker, a later process cannot reconstruct lost attempts. This is
local observed usage, never a complete provider invoice. Crashes before the
pending write, missing provider usage, and pre-feature requests cannot be
reconstructed. No existing history is backfilled with fabricated zeros.

Readers use a separate connection and WAL snapshot, so scanning a report cannot
consume the single writer connection or lock out new accounting writes.
Reports are local reads with a three-second storage deadline; they scan metadata
and aggregate on the backend. Only an expanded conversation disclosure polls
(every ten seconds). Settings refreshes explicitly or when filters change.
Storage grows with requests and survives conversation deletion; there is no
silent truncation. Large-history query/indexing work is a future scalability
concern, not a measured performance claim.

## Reports and cost basis

The conversation disclosure shows the latest turn and cumulative conversation,
including historical models and nullable detail counters. Settings supports date
ranges, day/conversation/model/request-type grouping, model and conversation
filters, and local or UTC day boundaries. Days use request **start** time and an
IANA timezone, including daylight-saving transitions. Unknown or partially known
values render as `—` or a known subtotal plus `?`; failed and unconfirmed request
counts are visible. Historical usage survives app restarts.

Public USD estimates use versioned observations in `internal/assistant/pricing`.
Each terminal call freezes the source, verification date, applicability window,
actual model, tier, applied rates and exact decimal amount. Reports sum stored
estimates; they never reprice old calls. Old records without snapshots remain
unpriced. The UI separates known estimated subtotal from unpriced request count,
and exposes rates in a collapsed price-basis disclosure. Estimates exclude
account discounts, taxes and other invoice adjustments.

The initial catalogue is verified on 2026-11-06 and expires for new estimates on
2026-12-01 (our review deadline, not a provider price guarantee). It covers exact
listed model IDs on the public OpenAI endpoint, Standard (`default`) responses,
and provider-duration `gpt-transcribe` ASR. Unknown models, missing actual text
model/tier, other endpoints/tiers, expired prices or incomplete usage remain
unpriced. ASR uses the requested model when the response omits it; this basis is
saved. Browser duration and token-to-audio conversions are never substituted.

Ordinary input = input − cached input − cache writes; the latter two must be
nonoverlapping. Separate 30-minute and 12-hour write counts and service tier are
retained. Nonzero writes without complete lifetime counts remain unpriced.
Reasoning is included in output and never charged twice. For listed text models,
input over 272,000 applies the documented entire-request 2× input/cache rates
and 1.5× output rate. The snapshot stores these applied rates. ASR uses exact
provider seconds × USD/minute ÷ 60. Decimal arithmetic avoids binary floats.

To update prices, verify official pricing and model long-context documentation,
add a new versioned catalogue with nonoverlapping applicability intervals and
regression fixtures, and close the previous interval if necessary. Never rewrite
persisted snapshots or extend a review deadline without verification. Overlapping
catalogues fail closed to unknown. This is an auditable estimate, not actual billing.

## Account model discovery

An explicit **Load models** action uses the saved key with `GET /v1/models`,
a ten-second deadline, bounded response, no redirects and no prompt/history.
The host returns sorted, deduplicated model IDs only. This endpoint reports
account-visible models, not a Responses/function-tool capability guarantee.
The UI keeps the existing value even if absent from the catalogue and retains
manual input. Empty lists and failures never install a fixed fallback model or
silently change the selection. Discovery is separate from enabling AI requests.

## Official references

Checked 2026-11-06 against the pinned SDK/converter source and:

- [Responses usage schema](https://developers.openai.com/api/reference/resources/responses/methods/create)
- [Public pricing](https://developers.openai.com/api/docs/pricing)
- [Prompt caching and nonoverlapping counts](https://developers.openai.com/api/docs/guides/prompt-caching#monitor-cache-performance)
- [Model long-context pricing](https://developers.openai.com/api/docs/models/gpt-6.1-sol)
- [Account models](https://developers.openai.com/api/reference/resources/models/methods/list)
- [Transcription usage variants](https://developers.openai.com/api/reference/resources/audio/subresources/transcriptions/methods/create)

Regression fixtures use synthetic keys, responses, audio and temporary stores.
Native macOS UI, live account availability and invoice reconciliation are separate
acceptance checks; no real ledger or credential is needed for automated tests.
