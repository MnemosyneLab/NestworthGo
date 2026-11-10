package assistant

import (
	"context"
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/agentcapability"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type fullKey struct{}

func CurrentFullHandler(ctx context.Context) ToolHandler {
	h, _ := ctx.Value(fullKey{}).(ToolHandler)
	return h
}

// Only the shared capability envelope is accepted; ledger changes use preview_batch/commit_plan.
func (r *Runtime) executeFull(ctx context.Context, epoch, id, runID string, revision uint64, input json.RawMessage) (json.RawMessage, error) {
	var request FullCapabilityRequest
	if jsoncodec.Unmarshal(input, &request) != nil || request.Name == "" {
		return nil, unsubmittedFullCapability{agentcapability.ErrInvalidArguments}
	}
	return r.executeCapability(ctx, epoch, id, runID, revision, request)
}
