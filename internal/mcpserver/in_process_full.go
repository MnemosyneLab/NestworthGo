package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// FullDefinitions is desktop host discovery. Call remains read-only; only the
// runtime's explicit, revision-fenced Full grant may invoke CallFull.
func (s *InProcessSession) FullDefinitions() ([]agentcapability.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.service.contexts.active(s.service.contextGeneration) {
		return nil, fail("session_revoked", "in-process session revoked")
	}
	return s.registry.Definitions(), nil
}

func (s *InProcessSession) CallFull(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	dispatched := false
	raw, err := s.callObserved(ctx, name, args, true, &dispatched)
	if err != nil && !dispatched {
		return raw, unsubmittedCapability{err}
	}
	return raw, err
}

// FullOperationID uses the exact MCP envelope and commit-plan derivation. It
// performs no mutation; the runtime saves this recovery identity before dispatch.
func FullOperationID(name string, args json.RawMessage) (string, error) {
	var in struct {
		OperationID string            `json:"operationId"`
		Input       CommitChangeInput `json:"input"`
	}
	if jsoncodec.Unmarshal(args, &in) != nil {
		return "", fail("validation", "invalid capability arguments")
	}
	id := in.OperationID
	var err error
	if name == "commit_plan" {
		id, err = commitPlanOperationID(id, in.Input.PlanID)
	}
	if err != nil {
		return "", err
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil {
		return "", fail("validation", "operationId must be a nonzero UUID")
	}
	return parsed.String(), nil
}

// InspectFullOperation never dispatches a tool, including on missing receipts.
func (s *InProcessSession) InspectFullOperation(id string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Operation{}, fail("session_revoked", "in-process session revoked")
	}
	return s.service.GetOperation(id)
}

func (s *InProcessSession) ValidateFull(name string, args json.RawMessage) error {
	return s.registry.ValidateArguments(name, args)
}

// This marker is emitted only before the registry is entered. It must never be
// inferred merely from cancellation or a missing operation receipt.
type unsubmittedCapability struct{ error }

func (e unsubmittedCapability) Unwrap() error     { return e.error }
func (unsubmittedCapability) NotDispatched() bool { return true }

func (s *InProcessSession) FullDefinition(name string) (agentcapability.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.service.contexts.active(s.service.contextGeneration) {
		return agentcapability.Definition{}, fail("session_revoked", "in-process session revoked")
	}
	return s.registry.Definition(name)
}
