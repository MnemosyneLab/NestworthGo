package builtinai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

type operationInventory struct{}

func (operationInventory) Definitions() ([]agentcapability.Definition, error) { return nil, nil }
func (operationInventory) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	panic("page operation escaped into shared capabilities")
}
func TestPageOperationRequiresRuntimeInjectedOperator(t *testing.T) {
	called := 0
	handler := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		called++
		return json.RawMessage(`{"state":"dispatched"}`), nil
	}
	for _, full := range []bool{false, true} {
		session := New(operationInventory{}, assistant.PageSelection{})
		// Full ledger command wiring alone does not authorize generic UI execution.
		session.WithFullAccess(handler)
		if full {
			session.WithPageOperator(handler)
		}
		defs, err := session.Definitions()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range defs {
			if d.Name == "operate_page" {
				found = true
				if d.Effect != agentcapability.Interaction || d.Annotations.ReadOnlyHint {
					t.Fatal("operator mislabeled read-only")
				}
			}
		}
		if found != full {
			t.Fatal("operator inventory differs from runtime grant")
		}
		_, err = session.callInternal(t.Context(), "operate_page_control", []byte(`{}`))
		if (err == nil) != full {
			t.Fatal("dispatch bypassed runtime grant")
		}
	}
	if called != 1 {
		t.Fatal("wrong handler call count")
	}
}
