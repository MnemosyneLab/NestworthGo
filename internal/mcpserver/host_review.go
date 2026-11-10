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
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const hostReviewLimit = 256 << 10

// This tag is part of the version 1 private review digest. Live authority is
// checked against the current mode and session, independently of the tag.
const hostReviewPermission = "ledger_write"

const (
	ReviewAwaitingConfirmation = "awaiting_confirmation"
	ReviewCommitting           = "committing"
	ReviewSucceeded            = "succeeded"
	ReviewNeedsPreview         = "needs_preview"
	ReviewCancelled            = "cancelled"
	ReviewUnknown              = "unknown"
)

// HostChangeReview is private host state, never a model tool or chat message.
// Only inspection of existing receipts remains. No production path creates
// or submits these retired convenience-write reviews.
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
	if s.service.config.Mode != ReadWrite || s.service.repository == nil {
		s.mu.Unlock()
		return fail("review_unavailable", "host review requires read-write permission and durable configuration")
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
		b.Permission == hostReviewPermission && b.Tool == "commit_change" && uuid.Validate(b.OperationID) == nil &&
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
