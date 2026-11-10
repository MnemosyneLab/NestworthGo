package agentcapability

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"strings"
	"testing"
)

func TestSchemaDiagnosticNeverEchoesDynamicKeysOrValues(t *testing.T) {
	registry := New("")
	calls := 0
	Add(registry, &Definition{Name: "read", Effect: Read, InputSchema: map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"type": "string", "enum": []string{"summary", "detail"}}}, "required": []string{"mode"}, "additionalProperties": false}}, func(context.Context, map[string]any) (any, error) { calls++; return map[string]any{}, nil })
	for _, raw := range []string{`{"mode":"PRIVATE_VALUE"}`, `{"mode":"summary","PRIVATE_KEY":"PRIVATE_VALUE"}`, `{}`} {
		_, err := registry.Call(t.Context(), "read", json.RawMessage(raw))
		var detail ArgumentError
		if !errors.As(err, &detail) || !errors.Is(err, ErrInvalidArguments) {
			t.Fatalf("lost structured refusal: %v", err)
		}
		encoded, _ := jsoncodec.Marshal(detail)
		if strings.Contains(string(encoded), "PRIVATE") {
			t.Fatal("sensitive input echoed", string(encoded))
		}
	}
	if calls != 0 {
		t.Fatal("invalid arguments entered handler")
	}
	if _, err := registry.Call(t.Context(), "read", json.RawMessage(`{"mode":"summary"}`)); err != nil || calls != 1 {
		t.Fatal("valid input rejected", err)
	}
}
