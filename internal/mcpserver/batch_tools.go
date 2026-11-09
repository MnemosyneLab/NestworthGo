package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type BatchChangeInput struct {
	Link     *domain.ReconciliationLink     `json:"reconciliationLink,omitempty"`
	Commands []history.ChangeCommandRequest `json:"commands"`
}

type batchPlan struct {
	Link       *domain.ReconciliationLink     `json:"reconciliationLink,omitempty"`
	Type       string                         `json:"type"`
	ID         string                         `json:"id"`
	MutationID string                         `json:"mutationId"`
	Commands   []history.ChangeCommandRequest `json:"commands"`
	Version    string                         `json:"version"`
	ExpiresAt  time.Time                      `json:"expiresAt"`
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
	readTool(server, "preview_batch", "Preview 1-100 completed ledger records atomically; nothing posts. Use exact decimal strings and explicit currencies. position_import uses original unit cost without a cash leg. position_transfer carries cost without profit. Commands must be in nondecreasing effective-time order (equal times keep array order; omitted times share one frozen instant), so earlier entries can fund later ones. For a buy or sell after a new, imported or transferred position, omit holdingId and give its settlementAccountId and instrumentId. No directory creation or placeholder IDs; one invalid entry rejects the whole preview. Commit with commit_plan.", s.previewBatch)
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
	id := uuid.New()
	plan := batchPlan{Link: in.Link, Type: "batch", ID: id.String(), MutationID: ledgerBatchMutationID(id, "batch"), Commands: commands, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
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

// Distinct plan families write separate mutation IDs in the same durable
// change-batch table. A generic batch receipt cannot prove reconciliation.
func ledgerBatchMutationID(planID uuid.UUID, family string) string {
	namespace := uuid.NewSHA1(uuid.NameSpaceURL, []byte("nestworth:mcp:ledger_receipt:"+family))
	return uuid.NewSHA1(namespace, []byte(planID.String())).String()
}

func (s *Service) recoverLedgerBatch(ctx context.Context, planID, family string, cause error) (any, error) {
	if safeError(cause).Code != "operation_outcome_unknown" {
		return nil, cause
	}
	id, err := uuid.Parse(planID)
	if err != nil {
		return nil, cause
	}
	receipt, err := s.app.ChangeBatchReceipt(ctx, ledgerBatchMutationID(id, family))
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, cause
	}
	return batchChangeResult(receipt), nil
}

func batchChangeResult(previews []domain.ChangePreview) BatchChangeResult {
	result := BatchChangeResult{Changes: make([]wire.ChangePreviewDTO, len(previews))}
	for i, preview := range previews {
		result.Changes[i] = wire.FromChangePreview(preview)
	}
	return result
}

func (s *Service) commitBatch(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "batch")
	if err != nil {
		return s.recoverLedgerBatch(ctx, in.PlanID, "batch", err)
	}
	var plan batchPlan
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(plan.ID)
	if err != nil || plan.MutationID != ledgerBatchMutationID(id, "batch") {
		return nil, fail("stale_preview", "request a fresh ledger preview")
	}
	var result BatchChangeResult
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		commands, err := s.batchCommands(ctx, plan.Commands)
		if err != nil {
			return err
		}
		payload, err := jsoncodec.Marshal(BatchChangeInput{Commands: plan.Commands, Link: plan.Link})
		if err != nil {
			return err
		}
		digest := sha256.Sum256(append([]byte("ledger_batch\n"), payload...))
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		previews, err := s.app.RecordChangesGuarded(domain.WithReconciliationLink(ctx, plan.Link), commands, plan.MutationID, hex.EncodeToString(digest[:]), version)
		if err != nil {
			return err
		}
		result = batchChangeResult(previews)
		return nil
	})
	return result, err
}
