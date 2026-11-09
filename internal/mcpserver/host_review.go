package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const hostReviewLimit = 256 << 10

const (
	ReviewAwaitingConfirmation = "awaiting_confirmation"
	ReviewCommitting           = "committing"
	ReviewSucceeded            = "succeeded"
	ReviewNeedsPreview         = "needs_preview"
	ReviewCancelled            = "cancelled"
	ReviewUnknown              = "unknown"
)

// HostChangeReview is private host state, never a model tool or chat message.
// The Full convenience writer uses the frozen preview for validation. Mobile
// and model tools never receive this host API; the digest detects changed plans.
type HostChangeReview struct {
	Binding HostChangeReviewBinding `json:"binding"`
	Digest  string                  `json:"digest"`
	State   string                  `json:"state"`
	Receipt *Operation              `json:"receipt,omitempty"`
}

type HostChangeReviewBinding struct {
	Source             string                   `json:"source,omitempty"`
	Version            int                      `json:"version"`
	ID                 string                   `json:"id"`
	HouseholdID        string                   `json:"householdId"`
	SessionID          string                   `json:"sessionId"`
	LedgerIncarnation  string                   `json:"ledgerIncarnation"`
	IncarnationID      string                   `json:"incarnationId"`
	PermissionRevision string                   `json:"permissionRevision"`
	Permission         string                   `json:"permission"`
	Tool               string                   `json:"tool"`
	OperationID        string                   `json:"operationId"`
	CreatedAt          time.Time                `json:"createdAt"`
	ParameterDigest    string                   `json:"parameterDigest"`
	Plan               changePlan               `json:"plan"`
	Preview            ChangePlanPreview        `json:"preview"`
	CashAccount        *HostCashAccountIdentity `json:"cashAccount,omitempty"`
}

func reviewDigest(value any) (string, error) {
	raw, err := jsoncodec.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// withHostReview retains session ownership while waiting on the review and
// application gates. RevokeForRestore never takes reviewMu or drains a writer.
// A single live InProcessSession owns these records; no multiprocess CAS claim.
func (s *InProcessSession) withHostReview(ctx context.Context, fn func(context.Context) error) error {
	s.mu.Lock()
	if s.closed || !s.service.contexts.active(s.service.contextGeneration) {
		s.mu.Unlock()
		return fail("session_revoked", "in-process session revoked")
	}
	if s.service.config.Mode != LedgerWrite || s.service.repository == nil {
		s.mu.Unlock()
		return fail("review_unavailable", "host review requires ledger permission and durable configuration")
	}
	s.inflight.Add(1)
	callContext, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	s.mu.Unlock()
	defer s.inflight.Done()
	defer cancel()
	defer stop()
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	err := s.service.app.WithPrivateConfiguration(callContext, func(ctx context.Context) error {
		if s.ctx.Err() != nil || !s.service.contexts.active(s.service.contextGeneration) {
			return fail("session_revoked", "in-process session revoked before host review")
		}
		if ctx.Err() != nil {
			return fail("cancelled", "request cancelled before host review")
		}
		bootstrap, err := s.service.app.Bootstrap(ctx)
		if err != nil {
			return safeError(err)
		}
		if bootstrap.Household == nil || bootstrap.Household.ID.String() != s.householdID {
			_ = s.RevokeForRestore(ctx)
			return fail("session_revoked", "household changed; start a new session")
		}
		return fn(ctx)
	})
	if s.ctx.Err() != nil {
		return fail("session_revoked", "in-process session revoked; inspect the original review with a new host")
	}
	if err != nil {
		return safeError(err)
	}
	return nil
}

// PreviewFullChange binds a host-owned Full principal, never a model approval.
func (s *InProcessSession) PreviewFullChange(ctx context.Context, command history.ChangeCommandRequest) (HostChangeReview, error) {
	if command.Kind != history.ChangeMoneyAdded && command.Kind != history.ChangeMoneyRemoved && command.Kind != history.ChangeCashTransfer {
		return HostChangeReview{}, fail("validation", "Full stage supports cash changes and transfers only")
	}
	if command.MutationID != "" || command.ReconciliationLink != nil || command.HoldingID != "" {
		return HostChangeReview{}, fail("validation", "unsupported Full fields")
	}
	return s.previewHostChange(ctx, command)
}

func (s *InProcessSession) previewHostChange(ctx context.Context, command history.ChangeCommandRequest) (HostChangeReview, error) {
	var review HostChangeReview
	// Canonicalize at the authoritative boundary, before the command/plan/digest
	// are persisted. Projection must agree with domain-normalized endpoint codes.
	if cashReview(command) {
		currency, err := domain.ParseSupportedCurrency(command.Currency)
		if err != nil {
			return review, safeError(err)
		}
		command.Currency = currency.String()
	}
	// Detach pointer fields before retaining any caller input.
	arguments, err := jsoncodec.Marshal(command)
	if err != nil || len(arguments) > inProcessInputLimit {
		return review, fail("validation", "invalid host proposal")
	}
	err = s.withHostReview(ctx, func(ctx context.Context) error {
		identity, err := s.cashAccountIdentity(ctx, command)
		if err != nil {
			return err
		}
		var detached history.ChangeCommandRequest
		if err := jsoncodec.Unmarshal(arguments, &detached); err != nil {
			return fail("validation", "invalid host proposal")
		}
		if ctx.Err() != nil {
			return fail("cancelled", "request cancelled before preview")
		}
		preview, err := s.service.previewHostLedgerChange(ctx, detached)
		if err != nil {
			return err
		}
		planRaw, err := s.service.loadPreviewPlan(preview.PlanID, "change")
		if err != nil {
			return err
		}
		var plan changePlan
		if jsoncodec.Unmarshal(planRaw, &plan) != nil {
			return fail("review_unavailable", "authoritative plan unavailable")
		}
		if identity != nil {
			found := false
			for _, endpoint := range preview.Preview.Resulting {
				if endpoint.AccountID != nil && *endpoint.AccountID == identity.AccountID && endpoint.Currency == command.Currency && endpoint.Amount != "" {
					found = true
				}
			}
			if !found {
				return fail("review_unavailable", "cash review result unavailable")
			}
		}
		parameters, err := reviewDigest(plan.Command)
		if err != nil {
			return err
		}
		review = HostChangeReview{State: ReviewAwaitingConfirmation, Binding: HostChangeReviewBinding{
			Source: "ai_full", Version: 1, ID: uuid.NewString(), HouseholdID: s.householdID, SessionID: s.owner.sessionID,
			LedgerIncarnation: s.service.app.MobileWorkspace(),
			IncarnationID:     s.service.config.InstanceID, PermissionRevision: s.service.config.InstanceID,
			Permission: LedgerWrite, Tool: "commit_change", OperationID: uuid.NewString(),
			CashAccount: identity, CreatedAt: time.Now().UTC(), ParameterDigest: parameters, Plan: plan, Preview: preview,
		}}
		review.Digest, err = reviewDigest(review.Binding)
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return fail("cancelled", "request cancelled before saving review")
		}
		return s.saveHostReview(review)
	})
	if err != nil {
		return HostChangeReview{}, err
	}
	return review, nil
}

func (s *InProcessSession) hostReviewPath(id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil {
		return "", fail("validation", "review identity must be a nonzero UUID")
	}
	return filepath.Join(s.service.dir, "reviews", parsed.String()+".json"), nil
}

func (s *InProcessSession) saveHostReview(review HostChangeReview) error {
	path, err := s.hostReviewPath(review.Binding.ID)
	if err != nil {
		return err
	}
	raw, err := jsoncodec.Marshal(review)
	if err != nil || len(raw) > hostReviewLimit {
		return fail("review_unavailable", "review exceeds the private state budget")
	}
	if err := s.service.writePrivate(path, review); err != nil {
		return fail("review_unavailable", "private review state could not be saved; inspect the original review")
	}
	return nil
}

func (s *InProcessSession) loadHostReview(id string) (HostChangeReview, error) {
	var review HostChangeReview
	path, err := s.hostReviewPath(id)
	if err != nil {
		return review, err
	}
	raw, err := s.service.readPrivate(path)
	if errors.Is(err, os.ErrNotExist) {
		return review, fail("not_found", "host review unavailable")
	}
	if err != nil {
		return review, fail("review_unavailable", "private review state unavailable")
	}
	if len(raw) > hostReviewLimit || jsoncodec.Unmarshal(raw, &review) != nil {
		return HostChangeReview{}, fail("review_unavailable", "invalid private review state")
	}
	// Only our canonical versioned encoding is accepted. This also rejects
	// duplicate/unknown typed review fields and edited immutable bindings, without
	// treating the digest as protection against a malicious database owner.
	canonical, encodeErr := jsoncodec.Marshal(review)
	b := review.Binding
	digest, digestErr := reviewDigest(b)
	parameters, parameterErr := reviewDigest(b.Plan.Command)
	previewParameters, previewErr := reviewDigest(b.Preview.Command)
	parsed, _ := uuid.Parse(id)
	valid := encodeErr == nil && bytes.Equal(canonical, raw) && digestErr == nil && digest == review.Digest &&
		parameterErr == nil && previewErr == nil && parameters == b.ParameterDigest && parameters == previewParameters &&
		(b.Source == "" || b.Source == "human_review" || b.Source == "ai_full") && b.Version == 1 && b.ID == parsed.String() && b.HouseholdID == s.householdID && b.SessionID == s.owner.sessionID &&
		len(b.LedgerIncarnation) == 64 &&
		b.Permission == LedgerWrite && b.Tool == "commit_change" && uuid.Validate(b.OperationID) == nil &&
		uuid.Validate(b.IncarnationID) == nil && b.PermissionRevision == b.IncarnationID && !b.CreatedAt.IsZero() &&
		b.Plan.Type == "change" && uuid.Validate(b.Plan.ID) == nil && b.Plan.ID == b.Preview.PlanID && b.Plan.Version != "" &&
		b.Preview.ExpiresAt == b.Plan.ExpiresAt.Format(time.RFC3339Nano) && b.Plan.ExpiresAt.After(b.CreatedAt)
	if b.CashAccount != nil {
		identity := b.CashAccount
		valid = valid && cashReview(b.Plan.Command) && identity.AccountID == b.Plan.Command.AccountID && identity.Name != "" && identity.AccountType != "" && identity.DefaultCurrency != "" && identity.Owners != nil
	}
	switch review.State {
	case ReviewAwaitingConfirmation, ReviewCommitting, ReviewSucceeded, ReviewNeedsPreview, ReviewCancelled, ReviewUnknown:
	default:
		valid = false
	}
	if !valid {
		return HostChangeReview{}, fail("review_unavailable", "unsupported or inconsistent private review state")
	}
	return review, nil
}

// InspectHostChange never dispatches a writer. Pending/unknown reviews inspect
// both receipt layers; only the exact ledger mutation may recover success.
func (s *InProcessSession) InspectHostChange(ctx context.Context, id string) (HostChangeReview, error) {
	var review HostChangeReview
	err := s.withHostReview(ctx, func(ctx context.Context) error {
		var err error
		review, err = s.loadHostReview(id)
		if err != nil {
			return err
		}
		if review.State == ReviewCommitting || review.State == ReviewUnknown || review.State == ReviewSucceeded {
			return s.reconcileHostReview(ctx, &review)
		}
		if review.Binding.Source != "ai_full" && (review.State == ReviewAwaitingConfirmation || review.State == ReviewNeedsPreview) {
			review.State = ReviewCancelled
			return s.saveHostReview(review)
		}
		if review.State == ReviewAwaitingConfirmation && !s.hostReviewCurrent(review) {
			review.State = ReviewNeedsPreview
			return s.saveHostReview(review)
		}
		return nil
	})
	if err != nil {
		return HostChangeReview{}, err
	}
	return review, nil
}

// ExecuteFullChange is a host-only submission under an explicitly enabled Full
// principal. It cannot adopt an ordinary user-confirmation review.
func (s *InProcessSession) ExecuteFullChange(ctx context.Context, id string) (HostChangeReview, error) {
	return s.submitHostChange(ctx, id)
}

func (s *InProcessSession) submitHostChange(ctx context.Context, id string) (HostChangeReview, error) {
	var review HostChangeReview
	err := s.withHostReview(ctx, func(ctx context.Context) error {
		var err error
		review, err = s.loadHostReview(id)
		if err != nil {
			return err
		}
		if review.Binding.Source != "ai_full" {
			return fail("permission_denied", "operation belongs to another authority")
		}
		if review.State == ReviewCommitting || review.State == ReviewUnknown || review.State == ReviewSucceeded {
			if err := s.reconcileHostReview(ctx, &review); err != nil {
				return err
			}
			if review.State != ReviewSucceeded {
				return fail("operation_outcome_unknown", "original submission unresolved; no new dispatch is permitted")
			}
			return nil
		}
		if review.State != ReviewAwaitingConfirmation {
			return fail("review_required", "a fresh preview is required")
		}
		if !s.hostReviewCurrent(review) {
			review.State = ReviewNeedsPreview
			if err := s.saveHostReview(review); err != nil {
				return err
			}
			return fail("review_required", "a fresh preview is required")
		}
		// Check the immutable stored plan before admitting the submission.
		raw, err := s.service.loadPreviewPlan(review.Binding.Plan.ID, "change")
		if err != nil {
			return err
		}
		planRaw, _ := jsoncodec.Marshal(review.Binding.Plan)
		if !bytes.Equal(raw, planRaw) {
			return fail("review_changed", "authoritative plan changed; obtain a fresh preview")
		}
		if ctx.Err() != nil {
			return fail("cancelled", "request cancelled before submission")
		}
		review.State = ReviewCommitting
		if err := s.saveHostReview(review); err != nil {
			return err // No dispatch unless durable admission succeeded.
		}
		input, _ := jsoncodec.Marshal(Mutation[CommitChangeInput]{OperationID: review.Binding.OperationID, Input: CommitChangeInput{PlanID: review.Binding.Plan.ID}})
		_, callErr := s.call(ctx, "commit_plan", input, true)
		review.State = ReviewUnknown
		if callErr == nil {
			// Use the persisted receipt's exact bytes. The generic capability
			// envelope may reorder object fields while validating its output.
			op, err := s.service.GetOperation(review.Binding.OperationID)
			hash, _ := operationHash(review.Binding.Tool, CommitChangeInput{PlanID: review.Binding.Plan.ID})
			if err == nil && op.Status == "succeeded" && op.Tool == review.Binding.Tool && op.Hash == hash {
				review.State, review.Receipt = ReviewSucceeded, &op
			}
		} else {
			failure := safeError(callErr)
			if failure.Code == "stale_preview" && failure.Field == "previewToken" {
				review.State = ReviewNeedsPreview
			} else if failure.Code == "cancelled" {
				review.State = ReviewCancelled
			}
		}
		if err := s.saveHostReview(review); err != nil {
			return fail("operation_outcome_unknown", "submission result could not be saved; inspect the original review")
		}
		if callErr != nil {
			return callErr
		}
		if review.State != ReviewSucceeded {
			return fail("operation_outcome_unknown", "submission result unavailable; inspect the original review")
		}
		return nil
	})
	if err != nil {
		return HostChangeReview{}, err
	}
	return review, nil
}

func (s *InProcessSession) hostReviewCurrent(review HostChangeReview) bool {
	return (!cashReview(review.Binding.Plan.Command) || review.Binding.CashAccount != nil) && review.Binding.IncarnationID == s.service.config.InstanceID &&
		review.Binding.LedgerIncarnation == s.service.app.MobileWorkspace() &&
		review.Binding.Plan.Version == s.service.app.MobileDataVersion() && time.Now().Before(review.Binding.Plan.ExpiresAt)
}

func (s *InProcessSession) reconcileHostReview(ctx context.Context, review *HostChangeReview) error {
	b := review.Binding
	hash, err := operationHash(b.Tool, CommitChangeInput{PlanID: b.Plan.ID})
	if err != nil {
		return err
	}
	op, err := s.service.GetOperation(b.OperationID)
	if err == nil && (op.Tool != b.Tool || op.Hash != hash) {
		return fail("operation_outcome_unknown", "operation receipt identity conflict")
	}
	if err == nil && op.Status == "succeeded" {
		review.State, review.Receipt = ReviewSucceeded, &op
		return s.saveHostReview(*review)
	}
	if err != nil {
		var wireError *apierror.WireError
		if !errors.As(err, &wireError) || wireError.Code != "not_found" {
			return fail("operation_outcome_unknown", "operation receipt unavailable")
		}
	}
	command := b.Plan.Command
	command.MutationID = b.Plan.ID
	mutationID, payloadHash, err := command.MutationEnvelope()
	if err != nil {
		return fail("review_unavailable", "invalid reviewed mutation")
	}
	receipt, err := s.service.app.InspectChangeReceipt(ctx, mutationID, payloadHash)
	if err != nil {
		return safeError(err)
	}
	review.State, review.Receipt = ReviewUnknown, nil
	if receipt != nil {
		result, err := jsoncodec.Marshal(wire.FromChangePreview(*receipt))
		if err != nil {
			return err
		}
		op = Operation{ID: b.OperationID, Tool: b.Tool, Hash: hash, Status: "succeeded", CreatedAt: b.CreatedAt.Format(time.RFC3339Nano), Result: result}
		path, _ := s.service.operationPath(b.OperationID)
		if err := s.service.writePrivate(path, op); err != nil {
			return fail("operation_outcome_unknown", "recovered receipt could not be saved")
		}
		review.State, review.Receipt = ReviewSucceeded, &op
	}
	return s.saveHostReview(*review)
}

// CancelHostChange only dismisses an unsubmitted review. Cancellation cannot
// undo a financial write or discard an unresolved operation identity.
func (s *InProcessSession) CancelHostChange(ctx context.Context, id string) error {
	return s.withHostReview(ctx, func(ctx context.Context) error {
		review, err := s.loadHostReview(id)
		if err != nil {
			return err
		}
		switch review.State {
		case ReviewAwaitingConfirmation, ReviewNeedsPreview, ReviewCancelled:
			review.State = ReviewCancelled
			return s.saveHostReview(review)
		default:
			return fail("operation_outcome_unknown", "submitted review must retain its original recovery identity")
		}
	})
}
