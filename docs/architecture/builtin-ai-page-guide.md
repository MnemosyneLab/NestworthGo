# Built-in assistant page guide stage

Desktop model conversation now supersedes the local desktop composer described
in this historical page-guide stage; see [the runtime contract](builtin-ai-runtime.md).
Mobile retains this deterministic guide and its closed model permissions.

This is a runnable UI foundation for the intended complete read/write assistant.
It is deliberately a **local deterministic page guide**, not a model-backed
assistant. The composer, credentials, proposals and AI financial confirmation
are visibly unavailable. No model call, paid request or secret is needed.

## Installed behavior

Desktop keeps the existing navigation and page history, business page and a
collapsible right panel at widths of at least 1280 CSS pixels. Expand replaces
the center with a full conversation workspace while retaining the mounted
business page. Smaller desktop windows use the shared Sheet. Mobile opens a
touch-sized Sheet from its header; the existing bottom navigation and central
recording action remain unchanged. The separate Financial review Sheet explains
the closed writer and directs users to the existing trusted recording flow.

Page capture is off until the assistant opens and is revoked when it closes.
No assistant status request or conversation view mounts on the initial closed
desktop surface. Overview, Accounts and account detail publish explicitly constructed DTOs from
their existing queries. The guide explains selected visible figures using
localized calculation definitions and retained evidence snapshots. It does not
calculate totals or returns. All three languages use the existing control,
formatting, sign and valuation helpers. Missing values remain missing, zero
remains zero and known subtotals remain incomplete. Snapshot time is the local
query capture time, not an invented market quote time. Unsupported pages expose
no metrics. No query cache enumeration, DOM scraping, notes or input values are
included. The visible account list supplies only its displayed candidates,
bounded at 64; it does not search hidden accounts or fetch additional pages.

Account-currency guide values, completeness and missing reasons come together
from `displayValue`, independently of household/base `complete` and
`missingInputs`. Missing USD/CNY FX must not make a complete displayed USD
balance partial. An omitted native value stays unavailable, with its known
native currency retained, even when the household projection is complete.
Desktop and mobile share this metric mapping; desktop keeps its existing signed
display amount without changing the projection's completeness or gaps.

Both desktop IPC `assistant.Service.Status` and the mobile
`GET /mobile-api/v1/assistant/status` return the shared version-1 closed status.
The mobile route retains existing live authentication and Read checks. Record,
market and manual grants do not enable an AI runner or a household-wide local
capability session. No assistant POST endpoints, credentials input, generic
tool dispatch, preview or confirm method are installed. External MCP semantics,
registrations, token, permissions and protocol are unchanged.

## Context and navigation boundary

`PageContextV1` is a client-only guide snapshot with ledger/view generation,
assistant session, permission generation, surface, page version, capture/expiry,
scope, timezone, currency, filter flags, typed metrics and opaque target refs.
These client generations are **not backend principal credentials** and must not
be accepted by a future runner as authorization. This stage does not claim a
production ledger-incarnation or scoped-model principal protocol. Mobile
contexts only reuse data already delivered through its authenticated UI. On
remembered-session reconnect, an incarnation change revokes/replaces the guide
host and remounts only its guide UI; it must not remount the financial page
subtree or discard an existing draft/review.

`PageHost` bounds snapshots to 64 KiB, 32 metrics, 64 targets, 240-character
labels, 16 missing-reason labels and 128-byte decimal strings. It retains at
most one current context and refuses unavailable, old and future captures.
Expiry is fixed at at most two minutes from query capture; explanation,
reconnect and history do not renew it. Refresh page evidence invokes only the
active publisher's controlled local read; expiry stays in force until a new
query result is actually published. No provider refresh is hidden in that action. A publication change, route change or
owner replacement revokes previous navigation authority. Publishers on retained
hidden pages cannot replace the active context. Financial facts remain detached
exact strings; destinations/business IDs stay in a separate host mapping.

The only installed semantic action is an explicit account button. It binds a
fresh user-turn/intent identity to the current context and page version, checks
expiry and the existing unresolved-confirmation guard, then invokes desktop
`open({page: "accounts", accountId})` or mobile's account-detail route. It uses
no second history stack, automatic name resolution or freeform model command.
Identical business labels receive page-order ordinals and remain separate.
Create/update flags and additional destination fields are refused. Duplicate
intents are consumed once. Stale intents return a refusal without navigation.

Local explanations yield before delivery so Stop, disconnect and context changes
can discard pending results. Reconnect never reruns an explanation or navigation.
The view stores at most 20 detached messages; their old evidence cannot navigate
or approve a change. Hiding the wide panel retains its mounted memory. Closing a
Sheet, changing responsive host, reloading, changing ledger/session or closing
the window can clear messages. There is no durable conversation/checkpoint,
provider history or browser storage. Raw provider-native reasoning is absent.

## Remaining dependent delivery

The target remains full read/write. Before activating it, implement and verify:

1. Backend-owned session/incarnation lifecycle, current scoped permissions and
   mobile capability intersection; restore, shutdown and revocation hooks.
2. Database credential settings (implemented in the runtime stage); separate
   provider/model/scope configuration, no secret
   values in tools, model context, logs or frontend responses.
3. Durable conversations/runs/evidence and complete Eino checkpoints; interrupted
   model requests must not retry automatically or claim successful completion.
4. Real model integration using the accepted provider adapter and bounded shared
   capabilities; metric/object disambiguation and explicit navigation intent.
5. Instrument/product/activity/chart publishers and semantic actions, range and
   filter contracts, broader page coverage, full provenance and pagination.
6. Trusted structured proposal review wired to #203's host APIs, including exact
   immutable preview/digest, expiry, unknown-result receipt inspection,
   duplicate/concurrent confirmation and recovery independent of chat deletion.
   The model must never invoke confirmation. Do not expose household-wide tools
   to mobile before its live permission intersection exists.
7. Complete desktop/mobile synthetic read -> model proposal -> new trusted
   preview -> explicit confirmation -> receipt/read acceptance; native macOS
   GUI, VoiceOver, credential entry, packaging and physical-phone acceptance.

The current UI never fabricates a preview, successful mutation, streaming model
answer or durable checkpoint. Existing backend review/provider tests remain
separate evidence; the local guide tests do not establish a complete AI run.
