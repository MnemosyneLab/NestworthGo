# Sonic JSON migration validation

This draft replaces maintained first-party Go encoding/decoding with Sonic
v1.15.4. It is **not ready for a byte-identical rollout**. Exact-byte differential
tests still fail for floating-point negative zero. No stored JSON, fixtures,
financial hash/MAC rules, certificate authority, or application settings were
changed to accept a new representation.

## Implementation

`internal/jsoncodec` uses `sonic.ConfigStd`. Stream configurations copy its
pinned options and preserve `UseNumber`, `DisallowUnknownFields`, and
`CaseSensitive` (the replacement for Segmentio's exact-field matching).
Custom marshalers/unmarshalers retain their signatures. `encoding/json`
`RawMessage`, `Number`, and `Marshaler` types remain. The two `json.Compact`
formatting calls in `product_terms_mutation.go` remain to preserve whitespace
normalization before hashing; they do not decode or re-encode values.

Sonic's own stream implementation was not sufficiently compatible: it could
return EOF for trailing garbage and discard the second value in `01`. The
adapter now frames with Sonic's public `decoder.Skip` and decodes each frame
with the configured Sonic API. It retains read-ahead, numeric lexemes, strict
fields, reader failures, and sequential values, including one-byte readers.
There is no stdlib engine fallback. `Valid` uses a Sonic `UseNumber` decode
because Sonic's fast validator accepted raw control characters in strings.
This validation allocates a decoded value; stream framing also scans before
decoding. No performance claim or performance matrix is included.

Sonic can reuse slices in existing map values. The bounded-read cache now
decodes into a fresh capture, preserving the existing immutable-input contract.
The existing mutation regression caught this issue and passes after the fix.

`github.com/segmentio/encoding` remains an indirect MCP SDK dependency. No
first-party runtime/default-test calls use its codec. Only explicitly tagged
`jsoncompat` tests invoke stdlib codecs as comparison oracles. The temporary
Segmentio differential comparison passed; existing SDK HTTP guard tests retain
coverage of exact-case shadow keys, repeated keys/nulls and trailing data.

## Evidence on the local Mac

Baseline: `main` at `8a6bdbcefadb9e965280b6bd1bde7bedf13dc102`.
Validation used `GOTOOLCHAIN=go1.26.4` and an isolated build cache. The first
focused test pass also ran on the host's Go 1.27.2; that is not a full second
version matrix.

- Full non-race `go test -count=1 -p 2 -timeout 20m ./...`: passed (40 packages
  with tests).
- `go vet ./...`: passed.
- `go build -o /tmp/nestworth-sonic-build ./cmd/nestworth`: passed on darwin/arm64.
  The native linker emitted macOS 27.0-object versus 11.0 deployment-target
  warnings. No application launch or native UI acceptance was performed.
- `gofmt -l cmd internal` and `git diff --check`: clean.
- Sixty existing JSON fixtures: equivalent decoded values and encoded bytes.
- Synthetic persisted/domain snapshots generated from the original domain
  source and current source: all 13 output files identical. These include
  historical and suffix fingerprints, certificate bytes, exact-decimal product
  contract/receipt bytes, large integer payloads, and six HMACs. The test also
  checks exact-decimal custom contract round-tripping.
- UseNumber, strict unknown fields, exact-case null semantics, trailing data,
  malformed skipped fields/RawMessage, invalid JSON, and chunked streams:
  focused differential/regression cases passed after fixes.
- 10,000 deterministic finite random float bit patterns matched stdlib output;
  this does not cover all floats, and the explicit negative-zero case fails.

The snapshot test has build tag `jsoncompat` and skips unless an explicit
`NESTWORTH_JSON_SNAPSHOT_DIR` is provided. It only generates synthetic data.
To reproduce the prior-source comparison, archive the baseline's `go.mod`,
`go.sum` and `internal/domain` into a temporary directory, copy
`internal/domain/json_compatibility_snapshot_test.go` there, and replace its
`jsoncodec` import with the prior `encoding/json` import. Run the named test
on both sources with separate empty temporary output directories and compare
all files. Do not point this experiment at real ledger/settings directories.

## Remaining blockers and differences

Run the opt-in exact-byte oracle with:

```sh
go test -tags jsoncompat -count=1 -v ./internal/jsoncodec
```

It deliberately remains failing for these observed differences; expected bytes
were not changed to bless Sonic output:

1. `float64` negative zero encodes as `0` in Sonic versus `-0` in stdlib.
   A negative zero passing through a generic numeric payload can change exact
   persisted bytes, fingerprints, MACs or equality. There is no exposed Sonic
   configuration mapping that restores this. Resolving universal byte identity
   requires a separately reviewed approach; stored-data migration or weakening
   integrity checks is not part of this PR.
2. With `SetEscapeHTML(false)`, Sonic leaves U+2028/U+2029 literal while stdlib
   still escapes them. There are no current production callers setting this
   option. ConfigStd's normal escaping matched the tested cases.

Error types/messages differ: malformed JSON returns Sonic `errors.SyntaxError`
rather than `*json.SyntaxError`; a string-to-int mismatch returns
`*errors.MismatchTypeError`; numeric overflow can be a Sonic syntax error where
stdlib uses `*json.UnmarshalTypeError`. Source inspection found no existing
first-party assertions on those stdlib error types. `io.EOF` remains distinct
from malformed trailing data; truncated framing uses `io.ErrUnexpectedEOF`.

Sonic v1.15.4 selects its native engine on amd64 with Go 1.17–1.27 and arm64
with Go 1.20–1.27. The project itself requires Go 1.26. An APIKind compile-time
guard intentionally rejects unsupported architectures/toolchain versions
instead of allowing Sonic's stdlib fallback. Local execution was darwin/arm64;
other platform results must be attributed to the automatic CI, not inferred
from local compilation. No Windows/native distribution acceptance was run.

The draft PR's automatic checks supply the remaining ordinary frontend and
Linux checks. No manual workflow dispatch, race/performance matrix, app launch,
release or merge is authorized by this validation step.
