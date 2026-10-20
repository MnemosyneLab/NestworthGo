// Package jsoncodec centralizes first-party JSON encoding and decoding in Sonic.
// ConfigStd targets standard-library compatibility; it does not guarantee byte
// identity for historical fingerprints, MACs, certificates, CAS or equality.
package jsoncodec

import (
	"io"

	"github.com/bytedance/sonic"
)

// Refuse Sonic's encoding/json fallback on unsupported Go/architecture targets.
// Such targets need explicit support instead of silently using the old engine.
var _ [sonic.APIKind - sonic.UseSonicJSON]struct{}

var standard = sonic.ConfigStd

// Match the pinned ConfigStd options for per-stream UseNumber, unknown-field
// and exact-case settings. The latter replaces Segmentio's
// DontMatchCaseInsensitiveStructFields on the MCP envelope guard.
var standardOptions = sonic.Config{
	EscapeHTML:       true,
	SortMapKeys:      true,
	CompactMarshaler: true,
	CopyString:       true,
	ValidateString:   true,
}

func Marshal(v any) ([]byte, error) { return standard.Marshal(v) }
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return standard.MarshalIndent(v, prefix, indent)
}
func Unmarshal(data []byte, v any) error { return standard.Unmarshal(data, v) }

// ConfigStd.Valid delegates to Sonic's fast validator, which accepts raw
// control characters in strings. Decode with UseNumber to validate strictly
// without rejecting syntactically valid numbers outside float64's range.
func Valid(data []byte) bool {
	options := standardOptions
	options.UseNumber = true
	var value any
	return options.Froze().Unmarshal(data, &value) == nil
}
func NewEncoder(w io.Writer) sonic.Encoder              { return standard.NewEncoder(w) }
func NewDecoder(r io.Reader) sonic.Decoder              { return newStreamDecoder(r, false) }
func NewCaseSensitiveDecoder(r io.Reader) sonic.Decoder { return newStreamDecoder(r, true) }
