package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type BatchChangeInput struct {
	Link     *domain.ReconciliationLink     `json:"reconciliationLink,omitempty"`
	Commands []history.ChangeCommandRequest `json:"commands" jsonschema:"One to 100 ledger commands in nondecreasing effective-time order. Same-time commands execute in array order. Omitted times share one frozen instant. No per-command mutationId."`
}

type batchPlan struct {
	Link      *domain.ReconciliationLink     `json:"reconciliationLink,omitempty"`
	Type      string                         `json:"type"`
	ID        string                         `json:"id"`
	Commands  []history.ChangeCommandRequest `json:"commands"`
	Version   string                         `json:"version"`
	ExpiresAt time.Time                      `json:"expiresAt"`
}

type BatchPlanPreview struct {
	Link      *domain.ReconciliationLink     `json:"reconciliationLink,omitempty"`
	PlanID    string                         `json:"planId"`
	ExpiresAt string                         `json:"expiresAt"`
	Commands  []history.ChangeCommandRequest `json:"commands"`
	Previews  []wire.ChangePreviewDTO        `json:"previews"`
}

type BatchChangeResult struct {
	Changes []wire.ChangePreviewDTO `json:"changes"`
}

func (s *Service) batchTools(server *toolSet) {
	readTool(server, "preview_batch", "Preview an atomic group of 1-100 ledger records (same kinds and fields as preview_change, including position_import and position_transfer); nothing posts. Commands must be in nondecreasing effective-time order (equal times keep array order; omitted times share one frozen instant), so earlier entries can fund later ones. For a buy or sell after a new, imported or transferred position, omit holdingId and give its account and instrument IDs. No directory creation or placeholder IDs; one invalid entry rejects the whole preview. Commit with commit_batch.", s.previewBatch)
	closed := false
	registerTool(server, &agentcapability.Definition{Name: "commit_batch", Description: "Commit the stored preview_batch plan atomically: all holdings, activities and the batch receipt, or nothing. Input is operationId and input.planId only; never a single-change plan. Retrying the same plan returns the original activities. Expired or stale plans need a new preview.", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, in Mutation[CommitChangeInput]) (Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_batch", in.Input, true, func(ctx context.Context) (any, error) { return s.commitBatch(ctx, in.Input) })
		if err != nil {
			return Response{}, safeError(err)
		}
		return Response{Data: value}, nil
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
		if command.ReconciliationLink != nil {
			return nil, batchInputError(i, fail("validation", "use the batch-level reconciliationLink, not a per-command link"))
		}
		var err error
		commands[i], err = normalizeChangeRequest(command, now)
		if err != nil {
			return nil, batchInputError(i, err)
		}
	}
	var resolved []any
	previews, version, err := s.app.PreviewChangesGuardedWith(ctx, func(ctx context.Context) ([]any, error) {
		var err error
		resolved, err = s.batchCommands(ctx, commands)
		return resolved, err
	})
	if err != nil {
		return nil, err
	}
	for i, p := range previews {
		if err := s.app.ValidateReconciliationCommandLink(ctx, in.Link, resolved[i], p); err != nil {
			return nil, batchInputError(i, err)
		}
	}
	plan := batchPlan{Link: in.Link, Type: "batch", ID: uuid.NewString(), Commands: commands, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	result := BatchPlanPreview{Link: in.Link, PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Commands: commands, Previews: make([]wire.ChangePreviewDTO, len(previews))}
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
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	var result BatchChangeResult
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		commands, err := s.batchCommands(ctx, plan.Commands)
		if err != nil {
			return err
		}
		payload, err := jsoncodec.Marshal(BatchChangeInput{Commands: plan.Commands})
		if err != nil {
			return err
		}
		digest := sha256.Sum256(append([]byte("ledger_batch\n"), payload...))
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		previews, err := s.app.RecordChangesGuarded(domain.WithReconciliationLink(ctx, plan.Link), commands, plan.ID, hex.EncodeToString(digest[:]), version)
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
