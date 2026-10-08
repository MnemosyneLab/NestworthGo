package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ProductTermsPreview struct {
	Before            ProductDetail
	After             ProductDetail
	ReviewedStateHash string
}

type ProductValuationCommand struct {
	ProductID  domain.ProductContractID `json:"productId"`
	Amount     string                   `json:"amount"`
	ObservedAt string                   `json:"observedAt"`
}

type ProductValuationPreview struct {
	Command           ProductValuationCommand
	Before            *domain.Money
	After             *domain.Money
	NetWorthDelta     *domain.SignedMoney
	ReviewedStateHash string
}

type ProductValuationReceipt = domain.ProductValuationReceipt

func productDataDigest(input any) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return hashBytes(raw), nil
}

func (s *Service) productDataStateHash(ctx context.Context, contract domain.ProductContract, digest string, now time.Time) (string, error) {
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return "", err
	}
	if origin == nil {
		return "", &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "history must be started"}
	}
	date, _, err := domain.LocalCivilDate(now, origin.Timezone)
	if err != nil {
		return "", err
	}
	return s.reviewedStateHash(ctx, snapshot, ProductCommand{}, digest, date, productPlan{beforeContracts: []domain.ProductContract{contract}})
}

func staleProductData() error {
	return &domain.Error{Code: domain.ErrStalePreview, Message: "preview expired or facts changed; request a new preview"}
}

func (s *Service) termsDetails(ctx context.Context, prepared preparedProductTerms, now time.Time) (ProductDetail, ProductDetail, error) {
	before, err := s.productDetail(ctx, prepared.before.HouseholdID, prepared.before)
	if err != nil {
		return ProductDetail{}, ProductDetail{}, err
	}
	after := before
	after.Contract, after.Policy = prepared.contract, prepared.policy
	origin, err := s.repository.HistoryOrigin(ctx, prepared.contract.HouseholdID)
	if err != nil {
		return ProductDetail{}, ProductDetail{}, err
	}
	date, _, err := domain.LocalCivilDate(now, timezoneOrUTC(origin))
	if err != nil {
		return ProductDetail{}, ProductDetail{}, err
	}
	before.DisplayState, err = domain.ProductAvailabilityState(before.Contract, before.Policy, date)
	if err != nil {
		return ProductDetail{}, ProductDetail{}, err
	}
	after.DisplayState, err = domain.ProductAvailabilityState(after.Contract, after.Policy, date)
	return before, after, err
}

func (s *Service) PreviewProductTermsGuarded(ctx context.Context, input UpdateProductTermsInput) (ProductTermsPreview, string, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return ProductTermsPreview{}, "", err
	}
	defer unlock()
	now := s.clock()
	prepared, err := s.prepareProductTerms(ctx, input, now)
	if err != nil {
		return ProductTermsPreview{}, "", err
	}
	before, after, err := s.termsDetails(ctx, prepared, now)
	if err != nil {
		return ProductTermsPreview{}, "", err
	}
	digest, err := productDataDigest(input)
	if err != nil {
		return ProductTermsPreview{}, "", err
	}
	hash, err := s.productDataStateHash(ctx, prepared.before, digest, now)
	return ProductTermsPreview{Before: before, After: after, ReviewedStateHash: hash}, s.writes.previewToken(), err
}

func (s *Service) CommitProductTermsGuarded(ctx context.Context, input UpdateProductTermsInput, mutationID, stateHash, token string) (domain.ProductTermsReceipt, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	defer unlock()
	id, err := domain.ParseProductOperationID(mutationID)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	digest, err := productDataDigest(input)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	stored, err := s.repository.LookupProductTermsMutation(ctx, household.ID, id)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	if stored != nil {
		if stored.PayloadSHA256 != digest {
			return domain.ProductTermsReceipt{}, &domain.Error{Code: domain.ErrConflict, Message: "mutation ID was used with different terms"}
		}
		receipt := stored.Receipt
		receipt.Replayed = true
		return receipt, nil
	}
	if token != s.writes.previewToken() {
		return domain.ProductTermsReceipt{}, staleProductData()
	}
	now := s.clock()
	prepared, err := s.prepareProductTerms(ctx, input, now)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	currentHash, err := s.productDataStateHash(ctx, prepared.before, digest, now)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	if stateHash == "" || stateHash != currentHash {
		return domain.ProductTermsReceipt{}, staleProductData()
	}
	_, after, err := s.termsDetails(ctx, prepared, now)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	receipt := domain.ProductTermsReceipt{MutationID: id, Contract: prepared.contract, Policy: prepared.policy, CurrentValue: after.CurrentValue, DisplayState: after.DisplayState, RecordedAt: now, PermittedActions: after.PermittedActions, SuccessorID: after.SuccessorID}
	commandJSON, err := json.Marshal(input)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	m, err := domain.NewProductTermsMutation(id, household.ID, commandJSON, receipt, now)
	if err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	if err := s.repository.CommitProductTermsMutation(ctx, prepared.contract, prepared.policy, prepared.expectedPolicyRevision, m); err != nil {
		return domain.ProductTermsReceipt{}, err
	}
	// No post-commit query can turn a recorded mutation into a reported failure.
	return receipt, nil
}

func (s *Service) PreviewProductValuationGuarded(ctx context.Context, command ProductValuationCommand) (ProductValuationPreview, string, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return ProductValuationPreview{}, "", err
	}
	defer unlock()
	now := s.clock()
	if strings.TrimSpace(command.ObservedAt) == "" {
		command.ObservedAt = now.UTC().Format(time.RFC3339Nano)
	}
	prepared, err := s.prepareProductValuation(ctx, AppendProductValuationInput{ProductID: command.ProductID, Amount: command.Amount, ObservedAt: command.ObservedAt}, now)
	if err != nil {
		return ProductValuationPreview{}, "", err
	}
	command.ObservedAt = prepared.observedAt.UTC().Format(time.RFC3339Nano)
	digest, err := productDataDigest(command)
	if err != nil {
		return ProductValuationPreview{}, "", err
	}
	hash, err := s.productDataStateHash(ctx, prepared.contract, digest, now)
	if err != nil {
		return ProductValuationPreview{}, "", err
	}
	_, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return ProductValuationPreview{}, "", err
	}
	before, after, err := s.productPreviewValues(snapshot, productPlan{beforeContracts: []domain.ProductContract{prepared.contract}, quotes: []domain.InstrumentQuote{prepared.quote}})
	if err != nil {
		return ProductValuationPreview{}, "", err
	}
	delta, err := productNetWorthDelta(before, after, nil, nil)
	return ProductValuationPreview{Command: command, Before: before, After: after, NetWorthDelta: delta, ReviewedStateHash: hash}, s.writes.previewToken(), err
}

func (s *Service) CommitProductValuationGuarded(ctx context.Context, command ProductValuationCommand, mutationID, stateHash, token string) (ProductValuationReceipt, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	defer unlock()
	id, err := domain.ParseProductOperationID(mutationID)
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	digest, err := productDataDigest(command)
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	stored, err := s.repository.LookupProductOperation(ctx, household.ID, id)
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	if stored != nil {
		if stored.Kind != domain.ProductOpValueObservation || stored.PayloadSHA256 != digest {
			return ProductValuationReceipt{}, &domain.Error{Code: domain.ErrConflict, Message: "mutation ID was used with a different observation"}
		}
		var evidence struct {
			Receipt ProductValuationReceipt `json:"receipt"`
		}
		if err := json.Unmarshal([]byte(stored.ResultJSON), &evidence); err != nil {
			return ProductValuationReceipt{}, err
		}
		r := evidence.Receipt
		if r.OperationID != id || r.ProductID != command.ProductID || r.QuoteID == "" {
			return ProductValuationReceipt{}, &domain.Error{Code: domain.ErrUnavailable, Message: "recorded valuation receipt is unavailable"}
		}
		r.Replayed = true
		return r, nil
	}
	if token != s.writes.previewToken() {
		return ProductValuationReceipt{}, staleProductData()
	}
	if strings.TrimSpace(command.ObservedAt) == "" {
		return ProductValuationReceipt{}, productReviewedTimeError()
	}
	now := s.clock()
	prepared, err := s.prepareProductValuation(ctx, AppendProductValuationInput{ProductID: command.ProductID, Amount: command.Amount, ObservedAt: command.ObservedAt}, now)
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	currentHash, err := s.productDataStateHash(ctx, prepared.contract, digest, now)
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	if stateHash == "" || stateHash != currentHash {
		return ProductValuationReceipt{}, staleProductData()
	}
	receipt := ProductValuationReceipt{OperationID: id, ProductID: command.ProductID, QuoteID: prepared.quote.ID, Amount: prepared.amount, ObservedAt: prepared.observedAt, RecordedAt: now}
	resultJSON, err := json.Marshal(struct {
		QuoteID string                  `json:"quoteId"`
		Receipt ProductValuationReceipt `json:"receipt"`
	}{prepared.quote.ID.String(), receipt})
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	requestJSON, err := json.Marshal(struct {
		Kind    string                  `json:"kind"`
		Command ProductValuationCommand `json:"command"`
	}{"value_observation", command})
	if err != nil {
		return ProductValuationReceipt{}, err
	}
	op := domain.ProductOperation{ID: id, HouseholdID: household.ID, Kind: domain.ProductOpValueObservation, PayloadSHA256: digest, RequestVersion: domain.ProductRequestVersion, RequestJSON: string(requestJSON), ResultJSON: string(resultJSON), EffectiveAt: prepared.observedAt, CreatedAt: now}
	if err := s.repository.CommitProductBundle(ctx, domain.ProductBundle{AsOf: now, Operation: op, Quotes: []domain.InstrumentQuote{prepared.quote}, ProductLinks: []domain.ProductOperationProduct{{OperationID: id, ProductID: command.ProductID, Role: domain.ProductRoleValued}}}); err != nil {
		return ProductValuationReceipt{}, err
	}
	s.invalidateAnalysis()
	return receipt, nil
}
