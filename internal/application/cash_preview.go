package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// CashBalancePlan retains the exact server-resolved command; clients only receive
// its display fields. The write token fences changes and restore/restart.
type CashBalancePlan struct {
	Current     string
	Target      string
	Delta       string
	Currency    string
	EffectiveAt time.Time
	Reason      *string
	value       domain.AccountCashValue
	command     any
	token       string
	mutationID  string
	payloadHash string
	expiresAt   time.Time
}

func (s *Service) PreviewCashBalance(ctx context.Context, accountID domain.AccountID, amount, currency string, reason *string) (*CashBalancePlan, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	snapshot, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return nil, onboardingRequired()
	}
	record, ok := accountFromSnapshot(snapshot, accountID)
	if !ok {
		return nil, notFound("account")
	}
	if record.Account.ArchivedAt != nil {
		return nil, &domain.Error{Code: domain.ErrConflict, Field: "accountId", Message: "account is archived"}
	}
	code, err := domain.ParseSupportedCurrency(currency)
	if err != nil {
		return nil, err
	}
	target, err := domain.ParseMoney(amount, code)
	if err != nil {
		return nil, err
	}
	if reason != nil {
		copied := *reason
		reason = &copied
	}
	now := s.clock()
	value, err := domain.NewAccountCashValue(record.Account, target, now, now)
	if err != nil {
		return nil, err
	}
	current, _ := domain.ParseMoney("0", code)
	for _, cash := range snapshot.CashValues {
		if cash.AccountID == accountID && cash.Amount.Currency() == code {
			current = cash.Amount
		}
	}
	delta := target.Amount().Sub(current.Amount())
	if delta.IsZero() {
		return nil, &domain.Error{Code: domain.ErrNoChange, Message: "the new cash value is unchanged"}
	}
	plan := &CashBalancePlan{Current: current.CanonicalAmount(), Target: target.CanonicalAmount(), Delta: delta.String(), Currency: code.String(), EffectiveAt: value.EffectiveAt, Reason: reason, value: value, token: s.writes.previewToken(), mutationID: uuid.NewString(), expiresAt: time.Now().Add(10 * time.Minute)}
	origin, err := s.repository.HistoryOrigin(ctx, household.ID)
	if err != nil {
		return nil, err
	}
	if origin == nil && reason != nil && *reason != "" {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "note", Message: "reasons require History"}
	}
	if origin != nil {
		magnitude, err := domain.NewMoney(delta.Abs(), code)
		if err != nil {
			return nil, err
		}
		if delta.IsPositive() {
			plan.command = domain.MoneyAddedInput{HouseholdID: household.ID, AccountID: accountID, Amount: magnitude, Reason: domain.ReasonReconciliation, EffectiveAt: value.EffectiveAt, Note: reason}
		} else {
			plan.command = domain.MoneyRemovedInput{HouseholdID: household.ID, AccountID: accountID, Amount: magnitude, Reason: domain.ReasonReconciliation, EffectiveAt: value.EffectiveAt, Note: reason}
		}
		if _, err := s.PreviewChange(ctx, plan.command); err != nil {
			return nil, err
		}
	}
	payload, err := json.Marshal(struct {
		Value  domain.AccountCashValue
		Reason *string
	}{value, reason})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(payload)
	plan.payloadHash = hex.EncodeToString(digest[:])
	return plan, nil
}

func (s *Service) ConfirmCashBalance(ctx context.Context, plan *CashBalancePlan) (domain.AccountCashValue, error) {
	if plan == nil {
		return domain.AccountCashValue{}, &domain.Error{Code: domain.ErrStalePreview, Message: "request a fresh preview"}
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	defer unlock()
	token := plan.token
	if time.Now().After(plan.expiresAt) {
		token = "expired"
	}
	if plan.command != nil {
		result, err := s.RecordChangeGuarded(ctx, plan.command, plan.mutationID, plan.payloadHash, token)
		if err != nil {
			return domain.AccountCashValue{}, err
		}
		values, err := s.repository.ListAccountCashValues(ctx, plan.value.AccountID)
		if err != nil {
			return domain.AccountCashValue{}, err
		}
		for _, value := range values {
			if value.ActivityID != nil && *value.ActivityID == result.Activity.ID {
				return value, nil
			}
		}
		return domain.AccountCashValue{}, &domain.Error{Code: domain.ErrIntegrity, Message: "committed cash observation is unavailable"}
	}
	// A baseline has its own immutable observation ID. Retry before the stale
	// check, so a double submit never appends a second observation.
	values, err := s.repository.ListAccountCashValues(ctx, plan.value.AccountID)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	for _, value := range values {
		if value.ID == plan.value.ID {
			return value, nil
		}
	}
	if token != s.writes.previewToken() {
		return domain.AccountCashValue{}, &domain.Error{Code: domain.ErrStalePreview, Message: "preview is stale; request a fresh preview"}
	}
	value := plan.value
	// Freeze the reviewed effective time, while retaining the baseline path's
	// actual confirmation audit time.
	value.CreatedAt = s.clock().UTC().Truncate(time.Millisecond)
	if err := s.repository.AppendAccountCashValue(ctx, value); err != nil {
		return domain.AccountCashValue{}, err
	}
	s.invalidateAnalysis()
	return value, nil
}
