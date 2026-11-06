package builtinai

import (
	"context"
	"encoding/json"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
)

func (s reviewSession) Capability(ctx context.Context, name string, args json.RawMessage) (agentcapability.Definition, string, error) {
	d, err := s.session.FullDefinition(name)
	if err != nil {
		return d, "", err
	}
	if err := s.session.ValidateFull(name, args); err != nil {
		return d, "", err
	}
	if d.Effect == agentcapability.Write {
		id, err := mcpserver.FullOperationID(name, args)
		return d, id, err
	}
	return d, "", nil
}
func (s reviewSession) CallCapability(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return s.session.CallFull(ctx, name, args)
}
func (s reviewSession) InspectCapability(ctx context.Context, id, name string) (assistant.ChangeReview, error) {
	op, err := s.session.InspectFullOperation(id)
	// Missing receipt after dispatch is uncertain, never evidence of rollback.
	state := "unknown"
	if err == nil {
		state = op.Status
	}
	return assistant.ChangeReview{ID: id, Source: "ai_full", State: state, Kind: "capability", Summary: name}, nil
}
