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

func TestSchemaDiagnosticConstraintsAndBranches(t *testing.T) {
	for _, tc := range []struct{ name, schema, input, field, rule string }{
		{"range", `{"type":"object","properties":{"limit":{"type":"integer","maximum":100}}}`, `{"limit":101}`, "limit", "maximum"},
		{"length", `{"type":"object","properties":{"name":{"type":"string","maxLength":3}}}`, `{"name":"PRIVATE"}`, "name", "maxLength"},
		{"allOf", `{"type":"object","allOf":[{"properties":{"limit":{"minimum":0}}}]}`, `{"limit":-1}`, "limit", "minimum"},
		{"then", `{"type":"object","if":{"properties":{"mode":{"const":"local"}}},"then":{"required":["date"]}}`, `{"mode":"local"}`, "date", "required"},
		{"else", `{"type":"object","if":{"properties":{"mode":{"const":"local"}}},"else":{"required":["timestamp"]}}`, `{"mode":"remote"}`, "timestamp", "required"},
		{"ref union", `{"type":"object","$defs":{"bounded":{"type":"integer","maximum":100}},"properties":{"limit":{"anyOf":[{"type":"null"},{"$ref":"#/$defs/bounded"}]}}}`, `{"limit":101}`, "limit", "maximum"},
		{"nullable ref union", `{"type":"object","$defs":{"bounded":{"type":["integer","null"],"maximum":100}},"properties":{"limit":{"anyOf":[{"type":"string"},{"$ref":"#/$defs/bounded"}]}}}`, `{"limit":101}`, "limit", "maximum"},
		{"compressed property", `{"type":"object","propertyNames":{"enum":["name"]},"additionalProperties":{"type":"string"}}`, `{"name":123}`, "name", "type"},
		{"nullable type", `{"type":"object","properties":{"limit":{"type":["integer","null"]}}}`, `{"limit":"PRIVATE"}`, "limit", "type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s, v any
			if err := jsoncodec.Unmarshal([]byte(tc.schema), &s); err != nil {
				t.Fatal(err)
			}
			if err := jsoncodec.Unmarshal([]byte(tc.input), &v); err != nil {
				t.Fatal(err)
			}
			var detail ArgumentError
			if !errors.As(argumentError(s, v), &detail) || detail.Field != tc.field || detail.Rule != tc.rule {
				t.Fatalf("diagnostic: %+v", detail)
			}
			raw, _ := jsoncodec.Marshal(detail)
			if strings.Contains(string(raw), "PRIVATE") {
				t.Fatal("input leaked")
			}
		})
	}
}

func TestNumericMoneyDiagnosticBeforeTypedDecode(t *testing.T) {
	type command struct {
		Amount string `json:"amount"`
	}
	type input struct {
		Commands []command `json:"commands"`
	}
	r := New("")
	calls := 0
	Add(r, &Definition{Name: "money", Effect: Preview}, func(context.Context, input) (any, error) { calls++; return nil, nil })
	_, err := r.Call(t.Context(), "money", json.RawMessage(`{"commands":[{"amount":15708.56}]}`))
	var detail ArgumentError
	if !errors.As(err, &detail) || detail.Field != "commands[0].amount" || detail.Rule != "type" || len(detail.Expected) != 1 || detail.Expected[0] != "string" {
		t.Fatalf("wrong diagnostic: %v %+v", err, detail)
	}
	if calls != 0 {
		t.Fatal("invalid money reached handler")
	}
}

func TestUnknownDiscriminatorDiagnostic(t *testing.T) {
	for _, tc := range []struct{ input, field, rule string }{
		{`{"kind":"PRIVATE"}`, "kind", "enum"},
		{`{}`, "kind", "required"},
		{`{"kind":"a"}`, "amount", "required"},
	} {
		var s, v any
		jsoncodec.Unmarshal([]byte(`{"type":"object","oneOf":[{"properties":{"kind":{"enum":["a"]}},"required":["kind","amount"]},{"properties":{"kind":{"enum":["b"]}},"required":["kind"]}]}`), &s)
		jsoncodec.Unmarshal([]byte(tc.input), &v)
		var e ArgumentError
		if !errors.As(argumentError(s, v), &e) || e.Field != tc.field || e.Rule != tc.rule {
			t.Fatalf("wrong diagnostic: %+v", e)
		}
		if strings.Contains(e.Message(), "PRIVATE") {
			t.Fatal("input leaked")
		}
		if tc.rule == "enum" && strings.Join(e.Expected, ",") != "a,b" {
			t.Fatalf("missing enum: %+v", e)
		}
	}
	// A branch that does not require the discriminator may be valid without it.
	var s, v any
	jsoncodec.Unmarshal([]byte(`{"type":"object","oneOf":[{"properties":{"kind":{"enum":["a"]}},"required":["kind"]},{"required":["other"]}]}`), &s)
	jsoncodec.Unmarshal([]byte(`{"kind":"PRIVATE"}`), &v)
	var e ArgumentError
	if !errors.As(argumentError(s, v), &e) || e.Rule != "one_of" {
		t.Fatalf("guessed partial union: %+v", e)
	}
}
