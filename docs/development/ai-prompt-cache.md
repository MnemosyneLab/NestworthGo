# Built-in AI prompt-prefix stability

The built-in model starts each turn with a small core catalog. Full mode can
load more tools when a task needs them. Local wire measurements do not establish
provider cache hits, billed input or latency savings.

## Protocol and scope

OpenAI's [prompt caching guide](https://developers.openai.com/api/docs/guides/prompt-caching)
describes a rendered prefix with tools before application instructions and
conversation history. Tool definitions, descriptions, schemas and order matter.
The [function calling guide](https://developers.openai.com/api/docs/guides/function-calling#tool-choice)
supports `tool_choice: {type: "allowed_tools", mode: "auto", tools: [...]}` to
restrict callable functions without changing definitions. The pinned Eino
v0.9.22 / agenticopenai v0.2.4 already serialize this through
`WithAgenticToolChoice`; no dependency update or transport JSON injection is used.

The desktop now supplies two independent surfaces:

- `Definitions()` and `CapabilityTools()` create **only this run's executable
  tools**. Session dispatch retains permission, revocation, revision, receipt,
  snapshot and page-lease checks. The entire returned call inventory is checked
  before any business call, including a valid read mixed with a forbidden write
  or unavailable page tool.
- `CatalogDefinitions(full)` supplies host metadata for the core, including
  basic financial reads, paging, operation receipts when present, and six
  desktop page/navigation definitions. A successful Full turn raises a
  checkpoint marker for metadata only; after downgrade an operation receipt
  schema can remain in the core without being callable. No Full tool enters a
  Read-only execution map.
- `load_tool_groups` is advertised only when there are additional permitted
  tools. The model selects categories by name: accounts, history, investments,
  market data, reconciliation, maintenance, and changes. The host validates all
  selected names before changing the catalog. It appends the permitted schemas
  in stable order, preserving the previous list as a prefix. A tool can only be
  called in a later response where the host exposed it. Loaded groups reset on
  each turn and are not read from checkpoints. A load consumes one tool call
  and the model needs a later response to use the tools.

The initial common system policy is independent of mode/page. Current host mode
is appended on the first turn or when the latest system snapshot differs from
the current host mode/policy, before the new user message; earlier mode
messages and complete native output/reasoning stay in place. Unchanged mode adds no repeated system messages. Snapshot reuse compares the
latest system message with the exact current policy, ignoring user/assistant text
and the catalog marker. No extra permission or mode flag is restored. A constant
mode supports 63 complete no-tool turns within the 128-message budget (two initial
system messages plus 126 user/assistant messages); genuine transitions consume
one additional message each. Existing duplicate snapshots are preserved rather
than compacted, so old checkpoints do not retroactively recover capacity.
The latest host mode explicitly overrides earlier claims and retained descriptions.
Final budget-exhausted rounds retain the catalog and send `tool_choice: "none"`.

Provider checkpoints use version 2 with an optional `catalog_full` boolean.
Version 1 is accepted and its initial host policy is migrated once; native items
are preserved. A legacy conversation starts with the current run's catalog level,
then records its new high-water mark. The private sidecar format is unchanged.
Provider/model changes retain the existing history reset behavior.

Page switches preserve core metadata. Permission changes can alter the
availability of group loading; loading groups can also alter the prefix that the
provider renders. Installed schema or policy changes may change the prefix.
The loader is built-in only. External MCP clients still discover the complete
registry for their current permission using `tools/list`.

`store:false`, `EnableAutoCache:false`, encrypted reasoning replay, no remote
conversation/previous-response IDs, and no explicit cache-key/retention settings
remain unchanged. Eino auto-cache is remote conversation reuse, not OpenAI's
server prompt cache. The host loader uses ordinary tool definitions and calls;
it does not use model-specific deferred loading, `additional_tools`, explicit
cache breakpoints or retention tuning.

## Task group wire measurements

`TestPromptCacheCatalogSizes` sends real SDK requests to a local fake provider.
This paired comparison uses the same synthetic desktop configuration and
current tool schemas on macOS arm64 with Go 1.27.3. The unscoped control
disables stable/scoped metadata and sends all executable definitions. It is a
test control, not a setting. Measurements are compact JSON bytes in the first
request's `tools` array; messages, allowed-name lists, and later requests are
outside the comparison.

| Desktop mode | Unscoped tools | Unscoped bytes | Initial core tools | Initial core bytes |
| --- | ---: | ---: | ---: | ---: |
| Full | 85 | 107,291 | 17 | 18,876 |
| Read-only | 14 | 17,256 | 15 | 17,711 |

The Full core is about 82% smaller. The extra Read-only definition holds the
backend page-selector metadata stable when no page is selected; it is excluded
from `allowed_tools` in that case. A task that needs additional groups uses an
extra round and sends more schemas on later requests. The total cost depends on
the groups loaded, number of rounds and actual provider billing.

## Historical metadata and cost tradeoff

The data below records the earlier prompt prefix experiment at main `ac5a9c54`
(#258), reviewed 2026-11-06, before task groups. The retained Full catalog and its
cost illustrations describe that version.

Actual fake-HTTP requests from `TestPromptCacheCatalogSizes`, Linux amd64 / Go
1.27.3, synthetic ledger, compact tool JSON. The desktop fixture attaches page
reader/operator/navigation but has no backend overview/account selector.
“Before” disables only the new catalog path to capture original definitions/order.
The full toolkit is **89 tools**, so always sending it to Read-only users was
rejected.

| Scenario | Tools | JSON bytes | o200k_base tokens |
| --- | ---: | ---: | ---: |
| Before, minimal Read-only adapter | 9 | 7,544 | 1,603 |
| Before, desktop Read-only | 14 | 14,229 | 2,913 |
| New Read-only catalog, any page | 15 | 14,684 | 2,997 |
| Before, minimal Full adapter | 83 | 100,890 | 22,172 |
| Before, desktop Full | 88 | 107,575 | 23,482 |
| New Full catalog / subsequent Read-only | 89 | 108,030 | 23,566 |

These are **local tokenizer estimates of serialized metadata**, not billed input
counts or the model's hidden tool rendering. o200k_base is a comparison proxy,
not a claim that every account-selected model uses it. Names-only `allowed_tools`
adds 159 local tokens for desktop Read-only and 973 for Full (109/923 for minimal
adapters); this is separate from the table. Messages, native history, provider
instructions, output and cache-write charging are excluded from the token table.

Illustrative USD per request for the tool catalog alone, using #258's versioned
`openai-public-2026-11-06.v1` GPT-6.1-sol standard rates from
[OpenAI pricing](https://developers.openai.com/api/docs/pricing): $2/M uncached,
$0.10/M cached read, $2.50/M 30-minute write. Each column hypothetically assigns
all catalog tokens to that billing class; they are alternatives, not additive.

| Catalog | All uncached | All cached reads | All 30-minute writes |
| --- | ---: | ---: | ---: |
| Before desktop Read-only, 2,913 tokens | $0.005826 | $0.0002913 | $0.0072825 |
| New Read-only, 2,997 tokens | $0.005994 | $0.0002997 | $0.0074925 |
| Before desktop Full, 23,482 tokens | $0.046964 | $0.0023482 | $0.0587050 |
| Full retained after downgrade, 23,566 tokens | $0.047132 | $0.0023566 | $0.0589150 |

Normal desktop Read-only pays 84 estimated extra metadata tokens (2.9%) to avoid
page-selector changes. A minimal adapter pays 1,394 extra tokens for static page
metadata; normal desktop adapters already advertise most of it. Always-Full
would cost about 7.9 times the small catalog in metadata tokens. After downgrade,
the retained catalog still has that size: misses, expiry and cache writes can
make it more expensive than shrinking. Even a fully cached retained Full catalog
costs more than a fully cached small one. Reusing the larger **entire history**
prefix is the tradeoff; this change makes no unconditional savings promise.

For example, a four-turn Read-only → Full → Read-only → Full sequence transmits
52,790 original desktop tool tokens versus 73,695 new catalog tokens. If every
request is uncached, illustrative metadata cost rises from $0.10558 to $0.14739.
If only the first small catalog and first Full catalog are uncached and the last
two Full catalogs are cached reads, the latter becomes $0.0578392. Neither
scenario establishes actual cache behavior; the original can also find earlier
cached prefixes, and provider writes can cost more than ordinary uncached input.

## Verification and reproduction

`prompt_cache_test.go` runs real SDK → local HTTP server requests for both JSON
and SSE. It compares earlier wire input exactly across turns, verifies stable
core schemas across page/downgrade/recreated engines, observes `allowed_tools`
on the wire, checks remote-storage settings, and refuses whole mixed call
batches before any read. Task group tests exercise validation, loading and
dispatch, monotonic catalog expansion within a turn, native history restore
without loaded groups, and rejection of hidden calls after downgrade.
A forged Full catalog checkpoint marker cannot authorize a Full-only receipt read.
The v1 migration regression verifies opaque native reasoning survives unchanged.
A real-wire boundary regression recreates the engine for each of 63 completed
no-tool turns in both Read-only and Full, then refuses turn 64 before HTTP.
Mode/page switching asserts snapshot counts; old duplicate snapshots, obsolete
policy, later unknown system messages and user text cannot suppress a required
mode update. Existing histories are never rewritten to reclaim capacity.
Existing Full/revocation/receipt/page tests exercise the unchanged host gates.

The same wire tests return explicitly synthetic cached/write token fields through
#258's `WithUsage` pipeline; they verify accounting preservation, not cache hits.
Production evaluation should use the existing durable input/cached/write counts,
pricing snapshots and estimated cost reports. Unknown fields remain unknown.
No live provider request, real cache hit, latency reduction or native Mac GUI
verification is claimed.

To reproduce metadata captures (directory must already exist):

```sh
cd providers/einoopenai
NESTWORTH_CATALOG_EVIDENCE_DIR=/tmp/catalog-evidence \
  go test -count=1 -run TestPromptCacheCatalogSizes -v ./...
```

The opt-in export contains tool metadata and allowed-name lists only. With
`tiktoken` installed, count each file with
`len(tiktoken.get_encoding("o200k_base").encode(path.read_text()))`.
The vocabulary SHA-256 is
`446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d`.
No tokenizer or runtime dependency was added to Nestworth.
