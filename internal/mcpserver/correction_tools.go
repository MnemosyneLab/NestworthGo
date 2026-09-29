package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type CorrectionInput struct {
	Action      string                        `json:"action" jsonschema:"fix replaces an original historical activity and replays later records; undo posts an inverse at preview time"`
	ActivityID  string                        `json:"activityId"`
	Replacement *history.ChangeCommandRequest `json:"replacement,omitempty" jsonschema:"Required only for fix; a full supported ledger command without timestamps or mutationId. The original effective time is preserved. Trades require an existing holdingId; position_import is unsupported."`
}

type correctionPlan struct {
	Type        string          `json:"type"`
	ID          string          `json:"id"`
	Input       CorrectionInput `json:"input"`
	EffectiveAt time.Time       `json:"effectiveAt"`
	Version     string          `json:"version"`
	ExpiresAt   time.Time       `json:"expiresAt"`
}

type CorrectionPlanPreview struct {
	PlanID      string                        `json:"planId"`
	ExpiresAt   string                        `json:"expiresAt"`
	Action      string                        `json:"action"`
	ActivityID  string                        `json:"activityId"`
	Replacement *history.ChangeCommandRequest `json:"replacement,omitempty"`
	Preview     wire.ChangePreviewDTO         `json:"preview"`
}

func (s *Service) correctionTools(server *mcp.Server) {
	readTool(server, "preview_correction", "Preview fixing or undoing an existing activity. Requires ledger_write. action fix replaces the historical record at its original timestamp and validates/replays later records; provide a full replacement with no timestamps. action undo posts a reversal at preview time and preserves the original history; omit replacement. These have different reporting effects. Read get_activity first and inspect the preview before commit_correction. For fix, preview.resulting describes the state immediately after the replacement at its historical time; query current valuations after commit. Already reversed/corrected records, managed products, unsupported replacements, or invalid dependent balances/positions are rejected. Preview IDs are provisional; plans expire after ten minutes or become stale on writes.", s.previewCorrection)
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "commit_correction", Description: "Commit exactly a stored preview_correction plan. Requires ledger_write. Supply operationId and input.planId only. The correction and database mutation receipt are atomic. For fix, returned resulting values describe the historical replacement; query current valuations for today. Identical retries or the same plan with a different operationId return the original activity, including after expiry or restart. Uncommitted stale plans need a new preview. Never pass another kind of plan.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_correction", in.Input, true, func(ctx context.Context) (any, error) { return s.commitCorrection(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

func (s *Service) previewCorrection(ctx context.Context, in CorrectionInput) (any, error) {
	id, err := domain.ParseActivityID(in.ActivityID)
	if err != nil {
		return nil, err
	}
	if in.Action != "fix" && in.Action != "undo" {
		return nil, fail("validation", "action must be fix or undo")
	}
	if in.Action == "undo" && in.Replacement != nil {
		return nil, fail("validation", "undo does not accept a replacement")
	}
	var preview domain.ChangePreview
	var version string
	if in.Action == "fix" {
		if in.Replacement == nil {
			return nil, fail("validation", "fix requires a replacement")
		}
		request := *in.Replacement
		if request.EffectiveAt != "" || request.EffectiveLocalDate != "" || request.EffectiveLocalTime != "" {
			return nil, fail("validation", "fix preserves the original effective time; omit replacement timestamps")
		}
		if request.Kind == "position_import" {
			return nil, fail("validation", "position_import is not a correction replacement")
		}
		if err := validateChangeRequest(request); err != nil {
			return nil, err
		}
		preview, version, err = s.app.PreviewFixChangeGuardedWith(ctx, id, func(ctx context.Context) (any, error) {
			original, err := s.app.Activity(ctx, id)
			if err != nil {
				return nil, err
			}
			request.EffectiveAt = original.EffectiveAt.Format(time.RFC3339Nano)
			origin, err := s.app.HistoryOrigin(ctx)
			if err != nil {
				return nil, err
			}
			if origin == nil {
				return nil, fail("history_not_started", "start history in the App before correcting records")
			}
			return ledgerCommand(request, origin)
		})
		in.Replacement = &request
	} else {
		preview, version, err = s.app.PreviewUndoChangeGuarded(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	plan := correctionPlan{Type: "correction", ID: uuid.NewString(), Input: in, EffectiveAt: preview.Activity.EffectiveAt, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	return CorrectionPlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Action: in.Action, ActivityID: in.ActivityID, Replacement: in.Replacement, Preview: wire.FromChangePreview(preview)}, nil
}

func (s *Service) commitCorrection(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "correction")
	if err != nil {
		return nil, err
	}
	var plan correctionPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	id, err := domain.ParseActivityID(plan.Input.ActivityID)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		Input       CorrectionInput
		EffectiveAt time.Time
	}{plan.Input, plan.EffectiveAt})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append([]byte("ledger_correction\n"), payload...))
	hash := hex.EncodeToString(digest[:])
	var preview domain.ChangePreview
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		var err error
		switch plan.Input.Action {
		case "undo":
			preview, err = s.app.RecordUndoChangeGuardedAt(ctx, id, plan.EffectiveAt, plan.ID, hash, version)
		case "fix":
			if plan.Input.Replacement == nil {
				return fail("validation", "stored correction has no replacement")
			}
			origin, originErr := s.app.HistoryOrigin(ctx)
			if originErr != nil {
				return originErr
			}
			if origin == nil {
				return fail("history_not_started", "history is not started")
			}
			command, commandErr := ledgerCommand(*plan.Input.Replacement, origin)
			if commandErr != nil {
				return commandErr
			}
			preview, err = s.app.RecordFixChangeGuarded(ctx, id, command, plan.ID, hash, version)
		default:
			return fail("validation", "invalid stored correction action")
		}
		return err
	})
	if err != nil {
		// Snapshot refresh follows the atomic correction commit. Keep a failed
		// refresh recoverable even when it returns a domain validation error.
		if preview.Activity.ID != "" {
			return nil, fail("internal", "correction committed but snapshot refresh failed; retry the same operation and plan")
		}
		return nil, err
	}
	return wire.FromChangePreview(preview), nil
}
