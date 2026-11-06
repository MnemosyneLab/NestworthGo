# Built-in AI model selection

The model catalogue is offline. `internal/assistant/model_selection.go` is the
single source for exact IDs and reasoning effort values, exposed through Status
for Settings and the sidebar. It is not an account entitlement check. No model
list request, credential lease, inference probe or automatic fallback is made.
The former remote model-list implementation has been removed.

Official API model pages verified on 2026-11-06:

| Exact ID | Explicit reasoning efforts | Source |
| --- | --- | --- |
| `gpt-5.6-sol` | none, low, medium, high, xhigh, max | [Sol](https://developers.openai.com/api/docs/models/gpt-5.6-sol) |
| `gpt-5.6-terra` | none, low, medium, high, xhigh, max | [Terra](https://developers.openai.com/api/docs/models/gpt-5.6-terra) |
| `gpt-5.6-luna` | none, low, medium, high, xhigh, max | [Luna](https://developers.openai.com/api/docs/models/gpt-5.6-luna) |
| `gpt-6-sol` | none, low, medium, high, xhigh, max | [Sol](https://developers.openai.com/api/docs/models/gpt-6-sol) |
| `gpt-6-luna` | none, low, medium, high, xhigh, max | [Luna](https://developers.openai.com/api/docs/models/gpt-6-luna) |
| `gpt-6-astra` | low, medium, high, xhigh, max | [Astra](https://developers.openai.com/api/docs/models/gpt-6-astra) |
| `gpt-6.1-sol` | low, medium, high, xhigh, max | [Sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol) |
| `gpt-6.1-terra` | none, low, medium, high, xhigh, max | [Terra](https://developers.openai.com/api/docs/models/gpt-6.1-terra) |

The default choice omits `reasoning` and lets the provider select its default;
it is distinct from explicit `none`. These are API values, not ChatGPT/Codex
product effort labels (in particular, no `ultra`). Do not infer additional IDs
or snapshot capabilities from prefixes. Updating the catalogue requires checking
the corresponding model page and extending capability/wire tests. The documented
`gpt-5.6` legacy alias uses Sol's capability set in backend validation, but the
curated list uses the explicit Sol ID. Pro mode is a separate API parameter,
not an invented model ID or effort; it is outside this feature.

Settings enables one to 32 exact IDs, with a separate custom/legacy section.
Older models are never inserted into the built-in catalogue. A legacy single
model remains enabled until explicitly removed; it is not silently migrated to
a newer model. Unknown custom IDs omit reasoning and may still be rejected for
lack of Responses/function-tool support or account access. Failed requests keep
the selected model and existing history, expose the established localized provider
failure, and never retry another model. API access is not proven by this catalogue.

The sidebar's preference persists in the existing ledger-bound private AI sidecar,
without browser storage. Switching models explicitly resets effort to provider
default. Removing the selected model keeps the obsolete choice visible and blocks
new sends until the user chooses an enabled model. Settings changes retain the
existing epoch/revision, cancellation and Full-permission revocation rules.
Changing a sidebar preference alone grants no permissions and does not mutate
active runs. The controls remain inside SecurityOperationRegion.

Start captures and validates the exact model/effort under the runtime lock. Both
are recorded on the run and checked on same-request-ID retries. Requests with
multiple enabled models must name one explicitly; omission is supported only
for the legacy single-model caller. Each engine has
its own immutable configuration. Usage still comes from each actual wire call's
requested and returned model, including failed requests; it is never read back
from the current sidebar preference. Title generation stays on its independent
fixed model, clears conversation effort, and retains separate accounting.

Same-model turns replay the complete native checkpoint. On a model switch, the
host supplies only successful visible user/assistant turns, in order and under
existing budgets. The new model gets no other model's encrypted reasoning,
function calls, tool receipts, or host permission. Future facts require fresh
controlled reads. After success the new native checkpoint becomes authoritative;
a failed switch leaves the last successful checkpoint intact. Switching back
rebuilds visible history instead of reviving an obsolete model-specific fork.
Display history and recovery references are preserved across configuration changes.

The additive sidecar fields retain version-1 read compatibility for existing data;
older strict-decoding binaries may reject new fields, so downgrading against this
sidecar is not supported. No financial database migration or real credential/data
access is involved in tests. Linux tests and purego binding generation do not
constitute native macOS GUI acceptance.
