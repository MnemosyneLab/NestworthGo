package builtinai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

type readCapabilityInventory struct {
	definitions []agentcapability.Definition
	calls       []string
	revoked     bool
}

func (s *readCapabilityInventory) Definitions() ([]agentcapability.Definition, error) {
	if s.revoked {
		return nil, assistant.ErrUnavailable
	}
	return s.definitions, nil
}
func (s *readCapabilityInventory) Call(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	if s.revoked {
		return nil, assistant.ErrUnavailable
	}
	s.calls = append(s.calls, name)
	return json.RawMessage(`{"complete":true}`), nil
}

func TestReadOnlyUsesSharedEffectsAndCurrentPermission(t *testing.T) {
	shared := &readCapabilityInventory{definitions: []agentcapability.Definition{
		{Name: "future_shared_read", Effect: agentcapability.Read},
		{Name: "get_products", Effect: agentcapability.Read},
		{Name: "get_overview", Effect: agentcapability.Write}, // familiar name cannot grant access
		{Name: "analyze_period", Effect: agentcapability.Analysis},
		{Name: "preview_batch", Effect: agentcapability.Preview},
		{Name: "commit_plan", Effect: agentcapability.Write},
		{Name: "unknown_effect", Effect: "future"},
	}}
	s := New(shared, assistant.PageSelection{})
	definitions, err := s.Definitions()
	if err != nil {
		t.Fatal(err)
	}
	// Only the two trusted reads are executable; local page tools are separate.
	for _, d := range definitions {
		if d.Name == "future_shared_read" || d.Name == "get_products" {
			continue
		}
		if d.Name == "get_overview" || d.Name == "analyze_period" || d.Name == "preview_batch" || d.Name == "commit_plan" || d.Name == "unknown_effect" {
			t.Fatalf("non-read exposed: %s", d.Name)
		}
	}
	for _, name := range []string{"future_shared_read", "get_products"} {
		if _, err := s.Call(t.Context(), name, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"get_overview", "analyze_period", "preview_batch", "commit_plan", "unknown_effect", "missing"} {
		if _, err := s.Call(t.Context(), name, json.RawMessage(`{}`)); err == nil {
			t.Fatalf("non-read dispatched: %s", name)
		}
	}
	// Previously discovered and cached metadata cannot survive removal or revocation.
	shared.definitions = nil
	if _, err := s.Call(t.Context(), "get_products", json.RawMessage(`{}`)); err == nil {
		t.Fatal("removed read dispatched")
	}
	shared.revoked = true
	if _, err := s.Call(t.Context(), "future_shared_read", json.RawMessage(`{}`)); err == nil {
		t.Fatal("revoked read dispatched")
	}
	if len(shared.calls) != 2 || len(s.Evidence()) != 2 {
		t.Fatal("unauthorized dispatch or evidence")
	}
}
