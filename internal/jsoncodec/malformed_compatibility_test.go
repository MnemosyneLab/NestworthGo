//go:build jsoncompat

package jsoncodec_test

import (
	stdjson "encoding/json"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"testing"
)

func TestDifferentialSkippedMalformed(t *testing.T) {
	for _, raw := range []string{`{"ignored":"a` + "\x00" + `b"}`, `{"raw":"a` + "\x01" + `b"}`, `{"ignored":[1,]}`, `{"raw":01}`, `{"ignored":{"x":}}`} {
		type shape struct {
			Raw stdjson.RawMessage `json:"raw"`
		}
		var a, b shape
		old, got := stdjson.Unmarshal([]byte(raw), &a), jsoncodec.Unmarshal([]byte(raw), &b)
		if (old == nil) != (got == nil) {
			t.Errorf("skipped malformed %q: std=%v sonic=%v", raw, old, got)
		}
	}
}

func TestDifferentialErrorTypes(t *testing.T) {
	for _, raw := range []string{`{`, `"not-an-integer"`, `1e400`} {
		var a, b int
		old, got := stdjson.Unmarshal([]byte(raw), &a), jsoncodec.Unmarshal([]byte(raw), &b)
		if old == nil || got == nil {
			t.Fatalf("expected both decoders to reject %q", raw)
		}
		t.Logf("%q: stdlib=%T; Sonic=%T", raw, old, got)
	}
}
