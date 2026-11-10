package mcpserver

// Legacy receipt fixture producer. These methods are deliberately test-only:
// old receipt recovery must be tested without retaining a live mutation API.
import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

// previewLegacyFixture binds a host-owned Full principal, never a model approval.
func (s *InProcessSession) previewLegacyFixture(ctx context.Context, command history.ChangeCommandRequest) (HostChangeReview, error) {
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
			Permission: hostReviewPermission, Tool: "commit_change", OperationID: uuid.NewString(),
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

// executeLegacyFixture is a host-only submission under an explicitly enabled Full
// principal. It cannot adopt an ordinary user-confirmation review.
func (s *InProcessSession) executeLegacyFixture(ctx context.Context, id string) (HostChangeReview, error) {
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

func (s *InProcessSession) cashAccountIdentity(ctx context.Context, command history.ChangeCommandRequest) (*HostCashAccountIdentity, error) {
	if !cashReview(command) {
		return nil, nil
	}
	accounts, err := s.service.app.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	members, err := s.service.app.ListMembers(ctx, true)
	if err != nil {
		return nil, err
	}
	names := map[domain.MemberID]string{}
	for _, member := range members {
		names[member.ID] = strings.Join(strings.Fields(member.Name), " ")
	}
	identities := make([]HostCashAccountIdentity, 0, len(accounts))
	selected := -1
	for _, record := range accounts {
		if record.Account.InstitutionID != nil && strings.TrimSpace(record.InstitutionName) == "" {
			return nil, fail("review_unavailable", "assigned institution identity unavailable")
		}
		owners := []string{}
		for _, share := range record.Ownership.Shares() {
			name := names[share.MemberID]
			if name == "" {
				return nil, fail("review_unavailable", "account owner identity unavailable")
			}
			owners = append(owners, name)
		}
		sort.Strings(owners)
		identity := HostCashAccountIdentity{AccountID: record.Account.ID.String(), Name: strings.Join(strings.Fields(record.Account.Name), " "), Institution: strings.Join(strings.Fields(record.InstitutionName), " "), Owners: owners, AccountType: record.Account.AccountType.String(), DefaultCurrency: record.Account.DefaultCurrency.String()}
		if identity.AccountID == command.AccountID {
			selected = len(identities)
		}
		identities = append(identities, identity)
	}
	if selected < 0 {
		return nil, fail("review_unavailable", "review account unavailable")
	}
	target := identities[selected]
	// Never pretend metadata distinguishes accounts when the rendered identities
	// are identical. Rename an account before using this confirmation path.
	visible := target
	visible.AccountID = ""
	for i, identity := range identities {
		if i == selected {
			continue
		}
		identity.AccountID = ""
		if reflect.DeepEqual(identity, visible) {
			return nil, fail("ambiguous_account_identity", "accounts share the same review identity; give the target a distinct name before proposing")
		}
	}
	return &target, nil
}

// previewHostLedgerChange keeps the host-owned review and receipt format private.
func (s *Service) previewHostLedgerChange(ctx context.Context, in history.ChangeCommandRequest) (ChangePlanPreview, error) {
	var err error
	in, err = normalizeChangeRequest(in, time.Now().UTC())
	if err != nil {
		return ChangePlanPreview{}, err
	}
	var resolved any
	preview, version, err := s.app.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		origin, err := s.app.HistoryOrigin(ctx)
		if err != nil {
			return nil, err
		}
		if origin == nil {
			return nil, fail("history_not_started", "start history in the App before recording changes")
		}
		resolved, err = ledgerCommand(in, origin)
		return resolved, err
	})
	if err != nil {
		return ChangePlanPreview{}, err
	}
	if err := s.app.ValidateReconciliationCommandLink(ctx, in.ReconciliationLink, resolved, preview); err != nil {
		return ChangePlanPreview{}, err
	}
	plan := changePlan{Type: "change", ID: uuid.NewString(), Command: in, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return ChangePlanPreview{}, err
	}
	return ChangePlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Command: in, Preview: wire.FromChangePreview(preview)}, nil
}
