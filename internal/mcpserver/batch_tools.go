package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type BatchChangeInput struct {
	Commands []history.ChangeCommandRequest `json:"commands" jsonschema:"One to 100 ledger commands in nondecreasing effective-time order. Same-time commands execute in array order. Omitted times share one frozen instant. No per-command mutationId."`
}

type batchPlan struct {
	Type      string                         `json:"type"`
	ID        string                         `json:"id"`
	Commands  []history.ChangeCommandRequest `json:"commands"`
	Version   string                         `json:"version"`
	ExpiresAt time.Time                      `json:"expiresAt"`
}

type BatchPlanPreview struct {
	PlanID    string                         `json:"planId"`
	ExpiresAt string                         `json:"expiresAt"`
	Commands  []history.ChangeCommandRequest `json:"commands"`
	Previews  []wire.ChangePreviewDTO        `json:"previews"`
}

type BatchChangeResult struct {
	Changes []wire.ChangePreviewDTO `json:"changes"`
}

func (s *Service) batchTools(server *mcp.Server) {
	readTool(server, "preview_batch", "Preview an atomic group of 1 to 100 ledger records without posting any. Requires ledger_write. Commands use preview_change's kinds and fields, including position_import and position_transfer. Supply commands in nondecreasing effective-time order; equal times use array order. All omitted timestamps share one frozen instant. Earlier entries fund later trades. For a buy/sell following a new, imported, or transferred position, omit holdingId and use its account and instrument IDs. Preview IDs are provisional. No directory creation, cross-command placeholder IDs, or target-state reconciliation. An invalid entry rejects the entire preview. Inspect all previews before commit_batch; plans expire in ten minutes and become stale on other application writes.", s.previewBatch)
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "commit_batch", Description: "Commit exactly the stored preview_batch plan as one database transaction: all holdings, activities and the batch mutation receipt succeed together or all roll back. Requires ledger_write. Pass operationId and input.planId only. Never submit a single-change plan here. Identical retries and reuse of the same plan return the original activities, including after interruption or restart; changed operation arguments conflict. Expired or stale uncommitted plans require a fresh preview.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_batch", in.Input, true, func(ctx context.Context) (any, error) { return s.commitBatch(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

func batchInputError(index int, err error) error {
	value := *safeError(err)
	prefix := fmt.Sprintf("commands[%d]", index)
	if value.Field != "" {
		prefix += "." + value.Field
	}
	value.Field = prefix
	return &value
}

func (s *Service) previewBatch(ctx context.Context, in BatchChangeInput) (any, error) {
	if len(in.Commands) < 1 || len(in.Commands) > 100 {
		return nil, fail("validation", "a batch requires 1 to 100 commands")
	}
	now := time.Now().UTC()
	// Do not mutate the caller's command slice while freezing implicit times.
	commands := make([]history.ChangeCommandRequest, len(in.Commands))
	for i, command := range in.Commands {
		var err error
		commands[i], err = normalizeChangeRequest(command, now)
		if err != nil {
			return nil, batchInputError(i, err)
		}
	}
	previews, version, err := s.app.PreviewChangesGuardedWith(ctx, func(ctx context.Context) ([]any, error) { return s.batchCommands(ctx, commands) })
	if err != nil {
		return nil, err
	}
	plan := batchPlan{Type: "batch", ID: uuid.NewString(), Commands: commands, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	result := BatchPlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Commands: commands, Previews: make([]wire.ChangePreviewDTO, len(previews))}
	for i, preview := range previews {
		result.Previews[i] = wire.FromChangePreview(preview)
	}
	return result, nil
}

func (s *Service) batchCommands(ctx context.Context, requests []history.ChangeCommandRequest) ([]any, error) {
	origin, err := s.app.HistoryOrigin(ctx)
	if err != nil {
		return nil, err
	}
	if origin == nil {
		return nil, fail("history_not_started", "start history in the App before recording changes")
	}
	commands := make([]any, len(requests))
	for i, request := range requests {
		commands[i], err = ledgerCommand(request, origin)
		if err != nil {
			return nil, batchInputError(i, err)
		}
	}
	return commands, nil
}

func (s *Service) commitBatch(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "batch")
	if err != nil {
		return nil, err
	}
	var plan batchPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	var result BatchChangeResult
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		commands, err := s.batchCommands(ctx, plan.Commands)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(BatchChangeInput{Commands: plan.Commands})
		if err != nil {
			return err
		}
		digest := sha256.Sum256(append([]byte("ledger_batch\n"), payload...))
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		previews, err := s.app.RecordChangesGuarded(ctx, commands, plan.ID, hex.EncodeToString(digest[:]), version)
		if err != nil {
			return err
		}
		result.Changes = make([]wire.ChangePreviewDTO, len(previews))
		for i, preview := range previews {
			result.Changes[i] = wire.FromChangePreview(preview)
		}
		return nil
	})
	return result, err
}
