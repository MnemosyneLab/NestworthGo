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

func TestValidationNumberFormsAreExactAndBounded(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		literal string
		want    any
	}{
		{"1e3", int64(1000)},
		{"1.0", int64(1)},
		{"9223372036854775807", int64(9223372036854775807)},
		{"18446744073709551615", uint64(18446744073709551615)},
		{"-0", int64(0)},
		{"1.5", float64(1.5)},
	} {
		got, err := validationView(json.Number(test.literal))
		if err != nil || got != test.want {
			t.Errorf("%s: %v %T, %v", test.literal, got, got, err)
		}
	}
	for _, literal := range []string{"18446744073709551616", "1e99999999", "1e-99999999", "9007199254740993.1", strings.Repeat("0", 1100) + ".1"} {
		if _, err := validationView(json.Number(literal)); err == nil {
			t.Errorf("unsafe/unsupported numeric form accepted: %.40s", literal)
		}
	}
}

// Cancel exactly at either registry context check, without a timing race.
type registryCancelAt struct {
	context.Context
	checks, at int
}

func (c *registryCancelAt) Err() error {
	c.checks++
	if c.checks >= c.at {
		return context.Canceled
	}
	return nil
}
func TestDispatchObservationDistinguishesPreHandlerCancellation(t *testing.T) {
	registry := New("synthetic")
	calls := 0
	Add(registry, &Definition{Name: "write", Effect: Write}, func(context.Context, struct{}) (any, error) { calls++; return nil, context.Canceled })
	for _, at := range []int{1, 2} {
		entered := false
		_, err := registry.CallObserved(&registryCancelAt{Context: context.Background(), at: at}, "write", []byte(`{}`), &entered)
		if !errors.Is(err, context.Canceled) || entered || calls != 0 {
			t.Fatalf("cancel at %d entered handler: %v/%v/%d", at, err, entered, calls)
		}
	}
	entered := false
	_, err := registry.CallObserved(t.Context(), "write", []byte(`{}`), &entered)
	if !errors.Is(err, context.Canceled) || !entered || calls != 1 {
		t.Fatal("handler cancellation incorrectly marked unsubmitted")
	}
}

func TestTypedDecodeIsAdmissionBeforeBusinessDispatch(t *testing.T) {
	t.Parallel()
	registry := New("synthetic")
	type input struct {
		Sequence int64 `json:"sequence"`
	}
	calls := 0
	Add(registry, &Definition{Name: "write", Effect: Write}, func(_ context.Context, in input) (input, error) {
		calls++
		return in, nil
	})
	for _, literal := range []string{"1.0", "1e0", "9223372036854775808"} {
		raw := json.RawMessage(`{"sequence":` + literal + `}`)
		if err := registry.ValidateArguments("write", raw); !errors.Is(err, ErrInvalidArguments) {
			t.Fatalf("typed admission accepted %s: %v", literal, err)
		}
		entered := false
		_, err := registry.CallObserved(t.Context(), "write", raw, &entered)
		var diagnostic ArgumentError
		if !errors.As(err, &diagnostic) || diagnostic.Rule != "typed_decode" || entered || calls != 0 {
			t.Fatalf("decode failure reached business dispatch: %s entered=%v calls=%d err=%v", literal, entered, calls, err)
		}
	}
	entered := false
	if _, err := registry.CallObserved(t.Context(), "write", []byte(`{"sequence":1}`), &entered); err != nil || !entered || calls != 1 {
		t.Fatalf("corrected request did not dispatch: entered=%v calls=%d err=%v", entered, calls, err)
	}
}
