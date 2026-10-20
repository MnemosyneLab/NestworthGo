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

// Match the pinned ConfigStd options, adding exact field-name matching for the
// MCP envelope guard that previously used Segmentio's
// DontMatchCaseInsensitiveStructFields. Duplicate keys/null handling and stream
// error details still require compatibility verification.
var caseSensitive = sonic.Config{
	EscapeHTML:       true,
	SortMapKeys:      true,
	CompactMarshaler: true,
	CopyString:       true,
	ValidateString:   true,
	CaseSensitive:    true,
}.Froze()

func Marshal(v any) ([]byte, error) { return standard.Marshal(v) }
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return standard.MarshalIndent(v, prefix, indent)
}
func Unmarshal(data []byte, v any) error                { return standard.Unmarshal(data, v) }
func Valid(data []byte) bool                            { return standard.Valid(data) }
func NewEncoder(w io.Writer) sonic.Encoder              { return standard.NewEncoder(w) }
func NewDecoder(r io.Reader) sonic.Decoder              { return standard.NewDecoder(r) }
func NewCaseSensitiveDecoder(r io.Reader) sonic.Decoder { return caseSensitive.NewDecoder(r) }
