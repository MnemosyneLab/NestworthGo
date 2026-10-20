# Sonic JSON migration validation

This draft replaces maintained first-party Go encoding/decoding with Sonic
v1.15.4. Generic encoding is not universally byte-identical, but the remaining
known differences do not affect supported App, MCP, or persistence workflows.
No stored JSON, fixtures, financial hash/MAC rules, certificate authority, or
application settings were changed to accept a new representation.

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

## Application impact of remaining differences

Run the opt-in exact-byte oracle with:

```sh
go test -tags jsoncompat -count=1 -v ./internal/jsoncodec
```

The oracle now passes while separately asserting the exact expected bytes for
the two generic exceptions. All other selected values, fixtures, stream cases,
and synthetic integrity outputs retain strict comparisons:

1. `float64` negative zero encodes as `0` in Sonic versus `-0` in stdlib.
   **No supported integrity or financial workflow impact found.** The review
   historical/suffix captures, temporal certificate, source capture, daily
   evidence, and product contract/receipt types contain no binary floats; a
   regular domain test guards these shapes. Financial values are exact-decimal
   strings or domain values. SQLite certified row seals preserve REAL bits
   directly, without JSON. Presentation settings have the only first-party
   persisted float fields (`window_width`/`window_height`); zero, including
   negative zero, fails the 640/480 minimum and is salvaged to defaults.
   Export's generic reconciliation evidence is produced from typed string and
   integer fields or SQLite JSON built from decimal TEXT columns, and is an
   output rather than an integrity authority. The MCP frozen-read decoder uses
   `UseNumber` for heterogeneous values, retaining the `-0` lexeme. A caller
   supplying an arbitrary raw `float64(-0)` directly to the generic codec
   still gets changed bytes; universal byte identity is not claimed.
2. With `SetEscapeHTML(false)`, Sonic leaves U+2028/U+2029 literal while stdlib
   still escapes them. **No reachable application impact:** the only
   `SetEscapeHTML` calls are in the opt-in comparison test. Production uses
   ConfigStd's default escaping, including on integrity and transport output.
   Both encodings decode to the same string.

Error types/messages differ: malformed JSON returns Sonic `errors.SyntaxError`
rather than `*json.SyntaxError`; a string-to-int mismatch returns
`*errors.MismatchTypeError`; numeric overflow can be a Sonic syntax error where
stdlib uses `*json.UnmarshalTypeError`. **No control-flow impact found:**
first-party callers check non-nil errors or wrap them in stable domain/wire
categories; none branches on the stdlib concrete JSON error types. Diagnostic
text can differ. `io.EOF` remains distinct from malformed trailing data;
truncated framing uses `io.ErrUnexpectedEOF`. The actual MCP HTTP envelope's
exact-case, repeated-key/null and trailing-body guard behavior is exercised
by its SDK-backed tests; malformed requests are rejected or scrubbed by the
existing bounded response path. This is the pinned SDK boundary, not a claim
about every possible future SDK version.

The previous universal byte-parity rollout blocker is lifted for the supported
application contract. This assessment does not bless arbitrary user-supplied
floating JSON as a signed/persisted format or guarantee byte identity for
future fields. No persisted-format migration or validation/authentication
weakening was needed.

Sonic v1.15.4 selects its native engine on amd64 with Go 1.17–1.27 and arm64
with Go 1.20–1.27. The project itself requires Go 1.26. An APIKind compile-time
guard intentionally rejects unsupported architectures/toolchain versions
instead of allowing Sonic's stdlib fallback. Local execution was darwin/arm64;
other platform results must be attributed to the automatic CI, not inferred
from local compilation. No Windows/native distribution acceptance was run.

The draft PR's automatic checks supply the remaining ordinary frontend and
Linux checks. No manual workflow dispatch, race/performance matrix, app launch,
release or merge is authorized by this validation step.
