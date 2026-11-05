# MCP Wave 2 G: toolGroups and dual results

Conclusion: delete the optional `tools/list` `_meta.nestworth.toolGroups`
filter. Keep both `structuredContent` and JSON text on success results.

## toolGroups

The middleware in the removed `task_tool_discovery.go` ran only when
`params.Meta["nestworth.toolGroups"]` was present. Absence called the next
handler, which is the SDK's full `tools/list` for the permission mode. The
file's own comment said the groups are discovery categories, never
permission tokens. `tools/call` stayed on the mode allowlist either way.

Nothing in this repository sends the key:

- The distributable skill is tested not to contain `nestworth.toolGroups`
  (`tools/tests/test_nestworth_skill.py`).
- The built-in assistant selects tools with `allowed()` in
  `internal/builtinai/session.go`. It does not call `tools/list` with this
  metadata.
- Server instructions do not mention the key.

[Wave 1 A2](mcp-wave1-a-baseline.md) used `@modelcontextprotocol/client` 2.2.0
and Inspector CLI 2.10.0 against loopback HTTP. The recorded session did not
send `_meta.toolGroups`. No Cursor, Claude, or other model host was configured,
so this is not a survey of production hosts. It is evidence that the clients
actually measured, and every maintained caller in this tree, work without the
key. Correctness does not depend on sending it.

Removal behavior:

- Default discovery is unchanged: same tools, same page size of 16, same
  instructions (2,300 bytes). `TestDiscoveryBudget` bytes moved only because
  the C3 description edits added 325 bytes in each mode.
- A client that still sends the key gets that same full list.
  `TestToolGroupMetadataDoesNotFilterDiscovery` checks a `summary` value and
  an unrecognized group against a request with no metadata.
- Previously, an unrecognized group or a mismatched HMAC cursor was
  `invalid_params`. Those requests now succeed as ordinary discovery. An
  in-flight group cursor from a process that still had the middleware does
  not survive a restart; the signing key was random per process.
- The `//go:build perf` combination sweep left with the middleware. Ordinary
  checks no longer categorize every new tool into a discovery group.

This is a breaking change for a client that used the filter to shrink its
catalog. No such client was observed. The skill version note records it.

## Dual response

A2 compared canonical JSON of `content[].text` and `structuredContent` on all
28 successful `tools/call` results. They were equal. The TypeScript client
handed the script both copies. The script read `structuredContent` and did
not forward either copy to a model. Inspector's `get_financial_context` result
also had both keys and they matched. No host was observed dropping one copy
or putting both into a prompt.

The server still emits both. The typed SDK builds the text fallback when
structured output is present. The built-in Eino adapter takes one direct
capability JSON string, not this external envelope. Dropping text without a
host that shows it is unused could remove the copy a client actually reads.
Both copies stay.
