package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// MobileRecordInput is a closed set of daily operations. It deliberately has
// no generic command, household, reconciliation link or investment fields.
type MobileRecordInput struct {
	OperationID      string `json:"operationId"`
	Kind             string `json:"kind"`
	AccountID        string `json:"accountId"`
	OtherAccountID   string `json:"otherAccountId,omitempty"`
	Amount           string `json:"amount"`
	Currency         string `json:"currency"`
	Received         string `json:"received,omitempty"`
	ReceivedCurrency string `json:"receivedCurrency,omitempty"`
	Fee              string `json:"fee,omitempty"`
	LocalDate        string `json:"effectiveLocalDate,omitempty"`
	LocalTime        string `json:"effectiveLocalTime,omitempty"`
	Note             string `json:"note,omitempty"`
}

type MobileRecordPlan struct {
	OperationID string
	Kind        string
	Timezone    string
	ExpiresAt   time.Time
	Preview     domain.ChangePreview
	Before      []domain.EndpointView
	Cash        *CashBalancePlan
	command     any
	token       string
	hash        string
	mutationID  string
}

func mobileInvalid(field string) error {
	return &domain.Error{Code: domain.ErrValidation, Field: field, Message: "invalid mobile operation"}
}

// MobileMutationID gives this entry point its own receipt namespace, bound to
// the current household. The client operation ID alone is never a credential.
func MobileMutationID(household domain.HouseholdID, operationID string) (string, error) {
	id, err := uuid.Parse(operationID)
	if err != nil || id == uuid.Nil {
		return "", mobileInvalid("operationId")
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("nestworth.mobile.record.v1:"+household.String()+":"+id.String())).String(), nil
}

func (s *Service) PreviewMobileRecord(ctx context.Context, input MobileRecordInput) (*MobileRecordPlan, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := MobileMutationID(origin.HouseholdID, input.OperationID)
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(input.Note) > 500 {
		return nil, mobileInvalid("note")
	}
	record, ok := accountFromSnapshot(snapshot, domain.AccountID(input.AccountID))
	if !ok {
		return nil, notFound("account")
	}
	a := record.Account
	if a.ArchivedAt != nil || a.TrackingMode == domain.TrackingManualValue {
		return nil, mobileInvalid("accountId")
	}
	currency, err := domain.ParseSupportedCurrency(input.Currency)
	if err != nil {
		return nil, err
	}
	amount, err := domain.ParseMoney(input.Amount, currency)
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC().Truncate(time.Millisecond)
	at := now
	if input.Kind != "balance" && (input.LocalDate != "" || input.LocalTime != "") {
		at, err = mobileLocalTime(input.LocalDate, input.LocalTime, origin.Timezone)
		if err != nil {
			return nil, err
		}
	}
	note := &input.Note
	p := &MobileRecordPlan{OperationID: input.OperationID, Kind: input.Kind, Timezone: origin.Timezone, ExpiresAt: time.Now().Add(10 * time.Minute), token: s.writes.previewToken(), mutationID: id}
	switch input.Kind {
	case "income", "expense", "card_purchase", "card_refund":
		card := strings.HasPrefix(input.Kind, "card_")
		if card && a.AccountType != domain.TypeCreditCard || !card && a.IsLiability() {
			return nil, mobileInvalid("accountId")
		}
		reason := domain.ActivityReason(input.Kind)
		if input.Kind == "income" || input.Kind == "card_purchase" {
			p.command = domain.MoneyAddedInput{HouseholdID: origin.HouseholdID, AccountID: a.ID, Amount: amount, Reason: reason, EffectiveAt: at, Note: note}
		} else {
			p.command = domain.MoneyRemovedInput{HouseholdID: origin.HouseholdID, AccountID: a.ID, Amount: amount, Reason: reason, EffectiveAt: at, Note: note}
		}
	case "transfer":
		other, ok := accountFromSnapshot(snapshot, domain.AccountID(input.OtherAccountID))
		if !ok || other.Account.ArchivedAt != nil || other.Account.IsLiability() || a.IsLiability() || other.Account.TrackingMode == domain.TrackingManualValue {
			return nil, mobileInvalid("otherAccountId")
		}
		code, err := domain.ParseSupportedCurrency(input.ReceivedCurrency)
		if err != nil {
			return nil, err
		}
		received, err := domain.ParseMoney(input.Received, code)
		if err != nil {
			return nil, err
		}
		fee, err := mobileOptionalMoney(input.Fee, currency)
		if err != nil {
			return nil, err
		}
		p.command = domain.CashTransferInput{HouseholdID: origin.HouseholdID, FromAccountID: a.ID, ToAccountID: other.Account.ID, Sent: amount, Received: received, Fee: fee, EffectiveAt: at, Note: note}
	case "repayment":
		fee, err := mobileOptionalMoney(input.Fee, currency)
		if err != nil {
			return nil, err
		}
		p.command = domain.DebtPaymentInput{HouseholdID: origin.HouseholdID, DebtAccountID: a.ID, CashAccountID: domain.AccountID(input.OtherAccountID), Principal: amount, InterestOrFee: fee, EffectiveAt: at, Note: note}
	case "balance":
		if a.TrackingMode == domain.TrackingHoldings {
			// Reuse the absolute current-cash plan and its time conflict check.
			cash, err := s.PreviewCashBalance(ctxWithPreviewPermit(ctx), a.ID, input.Amount, input.Currency, note)
			if err != nil {
				return nil, err
			}
			cash.mutationID = id
			p.Cash = cash
			p.command = cash.command
			p.hash = cash.payloadHash
		} else {
			p.command = domain.ValueUpdateInput{HouseholdID: origin.HouseholdID, AccountID: a.ID, NewValue: amount, Reason: domain.ReasonReconciliation, EffectiveAt: at, Note: note}
		}
	default:
		return nil, mobileInvalid("kind")
	}
	// Holding the preview permit makes both the frozen command and token
	// describe one state. No financial fact is persisted here.
	p.Preview, err = s.PreviewChange(ctx, p.command)
	if err != nil {
		return nil, err
	}
	state, err := s.changeStateFrom(origin, snapshot)
	if err != nil {
		return nil, err
	}
	for _, endpoint := range p.Preview.Resulting {
		before := endpoint
		if endpoint.AccountID != nil {
			a := state.Accounts[*endpoint.AccountID]
			before.Amount = a.Current.CanonicalAmount()
			if a.Mode == domain.TrackingHoldings {
				before.Amount = "0"
				if cash, ok := state.Cash[*endpoint.AccountID][endpoint.Currency]; ok {
					before.Amount = cash.CanonicalAmount()
				}
			}
		}
		p.Before = append(p.Before, before)
	}
	if p.hash == "" {
		input.Amount = amount.CanonicalAmount()
		input.Currency = currency.String()
		raw, err := jsoncodec.Marshal(struct {
			Version     int
			Input       MobileRecordInput
			EffectiveAt time.Time
		}{1, input, at})
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(raw)
		p.hash = hex.EncodeToString(sum[:])
	}
	if p.Cash != nil {
		p.Cash.payloadHash = p.hash
	}
	return p, nil
}

// Mobile accepts the explicit seconds in its HTTP contract while sharing the
// domain's civil-date/DST resolution. Pickers may also submit plain HH:MM.
func mobileLocalTime(date, clock, zone string) (time.Time, error) {
	seconds := time.Duration(0)
	if len(clock) == 8 {
		parsed, err := time.Parse("15:04:05", clock)
		if err != nil {
			return time.Time{}, mobileInvalid("time")
		}
		seconds = time.Duration(parsed.Second()) * time.Second
		clock = clock[:5]
	}
	at, err := domain.ResolveLocalDateTime(date, clock, zone)
	return at.Add(seconds), err
}

// This context is used only by nested preview helpers while the caller owns
// beginPreviewRead. It avoids reacquiring the non-reentrant coordinator.
func ctxWithPreviewPermit(ctx context.Context) context.Context {
	return context.WithValue(ctx, writePermitKey{}, &writePermit{})
}

func mobileOptionalMoney(value string, currency domain.CurrencyCode) (*domain.Money, error) {
	if value == "" {
		return nil, nil
	}
	m, err := domain.ParseMoney(value, currency)
	return &m, err
}

// MobileAuthorizedWrite orders revocation after the application write gate,
// before persistence. Restore holds that same outer gate, avoiding inversion.
// authorize must return a release function held through the atomic write.
func (s *Service) MobileAuthorizedWrite(ctx context.Context, authorize func() (func(), error), run func(context.Context) error) error {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	release, err := authorize()
	if err != nil {
		return err
	}
	defer release()
	return run(ctx)
}

func (s *Service) ConfirmMobileRecord(ctx context.Context, p *MobileRecordPlan) (domain.ChangePreview, error) {
	if p == nil {
		return domain.ChangePreview{}, &domain.Error{Code: domain.ErrStalePreview, Field: "planId"}
	}
	if p.Cash != nil {
		_, err := s.ConfirmCashBalance(ctx, p.Cash)
		if err != nil {
			return domain.ChangePreview{}, err
		}
		return s.RecordChangeGuarded(ctx, p.command, p.mutationID, p.hash, "committed-cash-replay")
	}
	token := p.token
	if time.Now().After(p.ExpiresAt) {
		token = "expired"
	}
	return s.RecordChangeGuarded(ctx, p.command, p.mutationID, p.hash, token)
}

// LookupMobileRecord is receipt-only; absence is unknown, never proof of a
// failed write. It also works after plans and sessions are lost at restart.
func (s *Service) LookupMobileRecord(ctx context.Context, operationID string) (*domain.ChangePreview, error) {
	hh, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	id, err := MobileMutationID(hh.ID, operationID)
	if err != nil {
		return nil, err
	}
	m, err := s.repository.LookupActivityMutation(ctx, hh.ID, domain.MutationID(id))
	if err != nil || m == nil {
		return nil, err
	}
	a, err := s.repository.Activity(ctx, hh.ID, m.ActivityID)
	if err != nil {
		return nil, err
	}
	return &domain.ChangePreview{Activity: a, Effects: a.Effects, Resulting: a.Resulting}, nil
}

// MobileDataVersion is opaque to clients and changes for every application
// write attempt. Workspace additionally fences restore and process restart.
func (s *Service) MobileDataVersion() string { return s.writes.previewToken() }
func (s *Service) MobileReadSnapshot(ctx context.Context, read func(context.Context) error) (version string, err error) {
	if err = s.writes.acquireWrite(); err != nil {
		return "", err
	}
	permit := &writePermit{}
	defer func() { version = s.writes.releaseSnapshotVersion(permit.writeRequested.Load()) }()
	err = read(context.WithValue(ctx, writePermitKey{}, permit))
	return
}
func (s *Service) MobileWorkspace() string {
	sum := sha256.Sum256([]byte(s.workspaceIdentity() + ":" + strings.Split(s.writes.previewToken(), ":")[0]))
	return hex.EncodeToString(sum[:])
}
