package agentcapability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestPatchStringDistinguishesOmitNullAndValue(t *testing.T) {
	t.Parallel()
	type input struct {
		Note PatchString `json:"note,omitempty,omitzero"`
		Name *string     `json:"name,omitempty"`
	}
	registry := New("patch")
	var got input
	Add(registry, &Definition{Name: "write", Effect: Write}, func(_ context.Context, in input) (any, error) {
		got = in
		return map[string]bool{"ok": true}, nil
	})
	definition, err := registry.Definition("write")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jsoncodec.Marshal(definition.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if jsonNullUnion(raw) {
		t.Fatalf("patch string published a null type union: %s", raw)
	}
	var schema map[string]any
	if err := jsoncodec.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	note := schema["properties"].(map[string]any)["note"].(map[string]any)
	if !anyOfNull(note) || note["type"] != nil {
		t.Fatalf("note schema = %#v, want anyOf string|null", note)
	}
	name := schema["properties"].(map[string]any)["name"].(map[string]any)
	if name["type"] != "string" || name["anyOf"] != nil {
		t.Fatalf("optional pointer name = %#v, want a plain string", name)
	}

	if _, err := registry.Call(t.Context(), "write", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if got.Note.Present() || got.Note.Null() {
		t.Fatalf("omitted note = %+v", got.Note)
	}
	if _, err := registry.Call(t.Context(), "write", json.RawMessage(`{"note":null}`)); err != nil {
		t.Fatal(err)
	}
	if !got.Note.Null() {
		t.Fatalf("null note = %+v", got.Note)
	}
	if _, err := registry.Call(t.Context(), "write", json.RawMessage(`{"note":"kept"}`)); err != nil {
		t.Fatal(err)
	}
	value, ok := got.Note.Value()
	if !ok || value != "kept" || got.Note.Null() {
		t.Fatalf("string note = %+v %q", got.Note, value)
	}
	if _, err := registry.Call(t.Context(), "write", json.RawMessage(`{"note":""}`)); err != nil {
		t.Fatal(err)
	}
	value, ok = got.Note.Value()
	if !ok || value != "" || got.Note.Null() {
		t.Fatalf("empty string must stay a present value, got %+v", got.Note)
	}
	if _, err := registry.Call(t.Context(), "write", json.RawMessage(`{"note":null,"noteSet":true}`)); !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("removed Set flag error=%v", err)
	}
	if _, err := registry.Call(t.Context(), "write", json.RawMessage(`{"name":null}`)); !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("null optional pointer error=%v", err)
	}

	encoded, err := jsoncodec.Marshal(struct {
		Note PatchString `json:"note,omitempty,omitzero"`
	}{})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{}" {
		t.Fatalf("omitted patch string encoded %s", encoded)
	}
	cleared := PatchString{}
	if err := jsoncodec.Unmarshal([]byte("null"), &cleared); err != nil {
		t.Fatal(err)
	}
	encoded, err = jsoncodec.Marshal(struct {
		Note PatchString `json:"note,omitempty,omitzero"`
	}{Note: cleared})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"note":null}` {
		t.Fatalf("cleared patch string encoded %s", encoded)
	}
}
