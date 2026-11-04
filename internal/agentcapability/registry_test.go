package agentcapability

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestTypedValidationExactNumbersAndDefaults(t *testing.T) {
	t.Parallel()
	type input struct {
		Sequence int64  `json:"sequence"`
		Label    string `json:"label"`
	}
	registry := New("policy")
	var calls atomic.Int32
	Add(registry, &Definition{Name: "read", Effect: Read, InputSchema: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"sequence"},
		"properties": map[string]any{"sequence": map[string]any{"type": "integer"}, "label": map[string]any{"type": "string", "default": "synthetic"}},
	}}, func(_ context.Context, in input) (input, error) {
		calls.Add(1)
		if in.Sequence != 9007199254740993 || in.Label != "synthetic" {
			t.Errorf("typed input changed: %+v", in)
		}
		return in, nil
	})
	result, err := registry.Call(t.Context(), "read", json.RawMessage(`{"sequence":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jsoncodec.Marshal(result)
	if err != nil || !strings.Contains(string(raw), `9007199254740993`) {
		t.Fatalf("integer rounded: %s, %v", raw, err)
	}
	for _, raw := range []string{`{}`, `[]`, `null`, `{"sequence":"9007199254740993"}`, `{"sequence":1,"extra":true}`, `{"sequence":1} {}`, `{"sequence":`, `{"sequence":9223372036854775808}`} {
		if _, err := registry.Call(t.Context(), "read", json.RawMessage(raw)); !errors.Is(err, ErrInvalidArguments) {
			t.Errorf("input %s: %v", raw, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("invalid inputs reached handler: %d", calls.Load())
	}
}

func TestDefinitionsAreDetachedAndDoNotWeakenValidation(t *testing.T) {
	t.Parallel()
	type input struct {
		Value string `json:"value"`
	}
	readOnly := false
	definition := &Definition{Name: "read", Effect: Read, Annotations: &Annotations{OpenWorldHint: &readOnly}}
	registry := New("")
	Add(registry, definition, func(_ context.Context, in input) (input, error) { return in, nil })
	definition.Name = "changed"
	*definition.Annotations.OpenWorldHint = true
	for range 2 {
		d := registry.Definitions()[0]
		if d.Name != "read" || *d.Annotations.OpenWorldHint {
			t.Fatalf("policy alias: %+v", d)
		}
		*d.Annotations.OpenWorldHint = true
		d.InputSchema.(map[string]any)["required"] = []any{}
		d.InputSchema.(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)["type"] = "number"
		d.OutputSchema.(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)["type"] = "number"
	}
	if _, err := registry.Call(t.Context(), "read", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidArguments) {
		t.Fatal("returned definition weakened required field", err)
	}
	if _, err := registry.Call(t.Context(), "read", json.RawMessage(`{"value":"exact"}`)); err != nil {
		t.Fatal("returned definition changed validation", err)
	}
}

func TestOutputValidationAndCancellation(t *testing.T) {
	t.Parallel()
	type input struct{}
	registry := New("")
	var calls atomic.Int32
	Add(registry, &Definition{Name: "read", Effect: Read, OutputSchema: map[string]any{"type": "object", "required": []string{"count"}, "properties": map[string]any{"count": map[string]any{"type": "integer"}}}}, func(context.Context, input) (map[string]any, error) {
		calls.Add(1)
		return map[string]any{"count": "not an integer"}, nil
	})
	if _, err := registry.Call(t.Context(), "read", nil); !errors.Is(err, ErrInvalidResult) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := registry.Call(ctx, "read", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("canceled handler called")
	}
	if _, err := registry.Call(t.Context(), "missing", nil); !errors.Is(err, ErrUnknownTool) {
		t.Fatal(err)
	}
}
