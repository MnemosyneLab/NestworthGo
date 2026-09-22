package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type productPlan struct {
	state            domain.ChangeState
	previews         []domain.ChangePreview
	instruments      []domain.Instrument
	observations     []domain.InstrumentPreferenceObservation
	holdings         []domain.Holding
	quotes           []domain.InstrumentQuote
	contracts        []domain.ProductContract
	policies         []domain.LiquidityPolicy
	productLinks     []domain.ProductOperationProduct
	activityLinks    []domain.ProductOperationActivity
	reservationLinks []domain.ProductOperationReservation
	beforeContracts  []domain.ProductContract
	afterContracts   []domain.ProductContract
	productIDs       []domain.ProductContractID
	quoteIDs         []domain.InstrumentQuoteID
	releaseIDs       []domain.LiquidityReservationID
	warnings         []string
	effectiveAt      time.Time
	reverses         *domain.ProductOperationID
}

func (s *Service) PreviewProductOperation(ctx context.Context, command ProductCommand) (ProductOperationPreview, error) {
	// Hold the ledger coordinator while collecting the plan and its dependencies.
	// This read-only operation must not hash newer reservation facts than it shows.
	if permit := permitFrom(ctx); permit == nil || !permit.ledger {
		s.changeMu.Lock()
		defer s.changeMu.Unlock()
	}
	normalized, payloadSHA, err := normalizeProductCommand(command)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	state, err := s.changeStateFrom(origin, snapshot)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	// Resolve "now" against the same instant used by domain change validation.
	// A later clock read would make a default effective time appear in the future.
	localDate, _, err := domain.LocalCivilDate(state.Now, origin.Timezone)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	plan, err := s.buildProductPlan(ctx, origin, snapshot, state, command, domain.NewProductOperationID(), state.Now)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	reviewed, err := s.reviewedStateHash(ctx, snapshot, command, payloadSHA, localDate, plan)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	cashBefore, cashAfter := collectCash(state, plan.state, plan)
	productBefore, productAfter, err := s.productPreviewValues(snapshot, plan)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	delta, err := productNetWorthDelta(productBefore, productAfter, cashBefore, cashAfter)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	releases, err := s.repository.ListLiquidityReservations(ctx, origin.HouseholdID, true)
	if err != nil {
		return ProductOperationPreview{}, err
	}
	releaseDetails := []domain.LiquidityReservation{}
	for _, r := range releases {
		for _, id := range plan.releaseIDs {
			if r.ID == id {
				releaseDetails = append(releaseDetails, r)
			}
		}
	}
	return ProductOperationPreview{
		Kind: command.Kind, NormalizedJSON: normalized, PayloadSHA256: payloadSHA,
		ReviewedStateHash: reviewed, LocalDate: localDate, Timezone: origin.Timezone,
		Warnings: plan.warnings, Activities: plan.previews, CashBefore: cashBefore, CashAfter: cashAfter,
		ReservationReleases: plan.releaseIDs, DraftProductIDs: plan.productIDs,
		ProductBefore: productBefore, ProductAfter: productAfter, NetWorthKnown: delta != nil, NetWorthDelta: delta, EffectiveAt: plan.effectiveAt, ReservationReleaseDetails: releaseDetails,
	}, nil
}

func (s *Service) RecordProductOperation(ctx context.Context, command ProductCommand, mutationID, reviewedStateHash string) (ProductOperationReceipt, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	defer unlock()
	normalized, payloadSHA, err := normalizeProductCommand(command)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	operationID, err := domain.ParseProductOperationID(strings.TrimSpace(mutationID))
	if err != nil {
		return ProductOperationReceipt{}, &domain.Error{Code: domain.ErrValidation, Field: "mutationId", Message: "must be a lowercase UUID"}
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	stored, err := s.repository.LookupProductOperation(ctx, household.ID, operationID)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	if stored != nil {
		if stored.PayloadSHA256 != payloadSHA {
			return ProductOperationReceipt{}, &domain.Error{Code: domain.ErrConflict, Field: "mutationId", Message: "this mutation ID was already used with a different command"}
		}
		receipt, err := receiptFromOperation(*stored)
		if err != nil {
			return ProductOperationReceipt{}, err
		}
		receipt.Replayed = true
		return receipt, nil
	}
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	state, err := s.changeStateFrom(origin, snapshot)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	localDate, _, err := domain.LocalCivilDate(state.Now, origin.Timezone)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	plan, err := s.buildProductPlan(ctx, origin, snapshot, state, command, operationID, state.Now)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	currentHash, err := s.reviewedStateHash(ctx, snapshot, command, payloadSHA, localDate, plan)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	if strings.TrimSpace(reviewedStateHash) == "" {
		return ProductOperationReceipt{}, &domain.Error{Code: domain.ErrValidation, Field: "reviewedStateHash", Message: "is required"}
	}
	if currentHash != strings.TrimSpace(reviewedStateHash) {
		return ProductOperationReceipt{}, &domain.Error{Code: domain.ErrStalePreview, Field: "reviewedStateHash", Message: "preview is stale; request a new preview"}
	}
	now := s.clock()
	_, cashAfter := collectCash(state, plan.state, plan)
	receipt := ProductOperationReceipt{
		OperationID: operationID, Kind: command.Kind, ProductIDs: plan.productIDs,
		QuoteIDs: plan.quoteIDs, CashAfter: cashAfter, CreatedAt: now,
	}
	for _, preview := range plan.previews {
		receipt.ActivityIDs = append(receipt.ActivityIDs, preview.Activity.ID)
	}
	evidence := productReceiptEvidence{
		Receipt: receipt, BeforeContracts: plan.beforeContracts, AfterContracts: plan.contracts,
		BeforePolicies: nil, AfterPolicies: plan.policies, CommandSHA: payloadSHA,
		ReviewedStateHash: currentHash, LocalDate: localDate,
	}
	resultJSON, err := json.Marshal(evidence)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	operation := domain.ProductOperation{
		ID: operationID, HouseholdID: household.ID, Kind: command.Kind, PayloadSHA256: payloadSHA,
		RequestVersion: domain.ProductRequestVersion, RequestJSON: normalized, ResultJSON: string(resultJSON),
		EffectiveAt: plan.effectiveAt, CreatedAt: now, ReversesOperationID: plan.reverses,
	}
	commits := make([]domain.ActivityCommit, 0, len(plan.previews))
	for index := range plan.reservationLinks {
		plan.reservationLinks[index].OperationID = operationID
	}
	for _, preview := range plan.previews {
		commits = append(commits, domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting})
	}
	if err := s.repository.CommitProductBundle(ctx, domain.ProductBundle{
		AsOf: now, Operation: operation, Instruments: plan.instruments, InstrumentObservations: plan.observations,
		Holdings: plan.holdings, Quotes: plan.quotes, Activities: commits, Contracts: plan.contracts,
		Policies: plan.policies, ProductLinks: plan.productLinks, ActivityLinks: plan.activityLinks,
		ReservationLinks: plan.reservationLinks,
	}); err != nil {
		return ProductOperationReceipt{}, err
	}
	s.invalidateAnalysis()
	return receipt, nil
}

func (s *Service) buildProductPlan(ctx context.Context, origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot, state domain.ChangeState, command ProductCommand, operationID domain.ProductOperationID, now time.Time) (productPlan, error) {
	if origin == nil {
		return productPlan{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a product operation"}
	}
	switch command.Kind {
	case domain.ProductOpOpen:
		if command.Open == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "open", Message: "open payload is required"}
		}
		return s.planOpen(ctx, origin, snapshot, state, *command.Open, operationID, now)
	case domain.ProductOpRecordExisting:
		if command.RecordExisting == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "recordExisting", Message: "record-existing payload is required"}
		}
		return s.planRecordExisting(ctx, origin, snapshot, state, *command.RecordExisting, operationID, now)
	case domain.ProductOpReceiveInterest:
		if command.ReceiveInterest == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "receiveInterest", Message: "interest payload is required"}
		}
		return s.planReceiveInterest(ctx, origin, snapshot, state, *command.ReceiveInterest, operationID, now)
	case domain.ProductOpSettle:
		if command.Settle == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "settle", Message: "settle payload is required"}
		}
		return s.planSettle(ctx, origin, snapshot, state, *command.Settle, operationID, now, nil)
	case domain.ProductOpRenew:
		if command.Renew == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "renew", Message: "renew payload is required"}
		}
		return s.planRenew(ctx, origin, snapshot, state, *command.Renew, operationID, now)
	case domain.ProductOpUndo:
		if command.Undo == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "undo", Message: "undo payload is required"}
		}
		return s.planUndo(ctx, origin, snapshot, state, *command.Undo, operationID, now)
	default:
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "kind", Message: "is not a supported product operation"}
	}
}

func (s *Service) planOpen(ctx context.Context, origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot, state domain.ChangeState, input OpenProductCommand, operationID domain.ProductOperationID, now time.Time) (productPlan, error) {
	resolved, resolveErr := resolveProductEffectiveTime(input.EffectiveAt, input.EffectiveLocalDate, input.EffectiveLocalTime, origin, now)
	if resolveErr != nil {
		return productPlan{}, resolveErr
	}
	input.EffectiveAt = resolved
	accountID, err := domain.ParseAccountID(input.AccountID)
	if err != nil {
		return productPlan{}, err
	}
	currency, err := domain.ParseCurrency(input.Currency)
	if err != nil {
		return productPlan{}, err
	}
	principal, err := parseRequiredMoney("principal", input.Principal, currency)
	if err != nil {
		return productPlan{}, err
	}
	if principal.IsZero() {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "principal", Message: "must be greater than zero"}
	}
	fee, err := parseOptionalMoney("openingFee", input.OpeningFee, currency)
	if err != nil {
		return productPlan{}, err
	}
	effectiveAt, err := parseEffectiveAt(input.EffectiveAt, origin, now)
	if err != nil {
		return productPlan{}, err
	}
	record, ok := accountFromState(snapshot, accountID)
	if !ok {
		return productPlan{}, &domain.Error{Code: domain.ErrNotFound, Field: "accountId", Message: "Account was not found"}
	}
	if err := domain.ProductAccountEligible(record.Account); err != nil {
		return productPlan{}, err
	}
	kind, mode, rate, maturityInterest, paidThrough, err := resolveProductTerms(input.Terms, currency, principal)
	if err != nil {
		return productPlan{}, err
	}
	if compareCivil(input.Terms.StartOn, effectiveAt.In(mustLocation(origin.Timezone)).Format("2006-01-02")) > 0 {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "opening effective date cannot precede the contract start date"}
	}
	instrument, err := buildManagedInstrument(origin.HouseholdID, input.Terms.Name, currency, effectiveAt)
	if err != nil {
		return productPlan{}, err
	}
	quote, err := buildManagedQuote(instrument, principal, effectiveAt)
	if err != nil {
		return productPlan{}, err
	}
	holding, err := domain.NewHoldingForAccount(record.Account, instrument, zeroQuantity(), nil, 0, effectiveAt)
	if err != nil {
		return productPlan{}, err
	}
	state = addInstrumentToState(state, instrument, holding)
	buy, err := domain.PreviewChange(state, domain.TradeInput{
		HouseholdID: origin.HouseholdID, Side: domain.TradeBuy, SettlementAccountID: accountID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: oneQuantity(), Gross: principal,
		Fee: optionalZeroFee(fee), EffectiveAt: effectiveAt,
	})
	if err != nil {
		return productPlan{}, err
	}
	state, err = applyPreview(state, buy)
	if err != nil {
		return productPlan{}, err
	}
	productID := domain.NewProductContractID()
	if paidThrough == nil && (mode == domain.InterestSimpleAct365 || mode == domain.InterestSimpleAct360) {
		start := input.Terms.StartOn
		paidThrough = &start
	}
	contract := domain.ProductContract{
		ID: productID, HouseholdID: origin.HouseholdID, AccountID: accountID, HoldingID: holding.ID,
		InstrumentID: instrument.ID, Kind: kind, Name: input.Terms.Name, Note: input.Terms.Note, Currency: currency,
		Principal: principal, StartOn: input.Terms.StartOn, MaturityOn: input.Terms.MaturityOn, InterestMode: mode,
		AnnualRate: rate, MaturityInterest: maturityInterest, InterestPaidThroughOn: paidThrough,
		State: domain.ProductStateOpen, OpenedOperationID: operationID, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := contract.Validate(); err != nil {
		return productPlan{}, err
	}
	policy, err := contractPolicyForOpen(origin.HouseholdID, accountID, holding.ID, kind, input.Terms.MaturityOn, input.Policy, currency, now)
	if err != nil {
		return productPlan{}, err
	}
	observation := domain.InstrumentPreferenceObservation{ID: domain.NewInstrumentPreferenceObservationID(), InstrumentID: instrument.ID, SourceKind: domain.QuoteSourceManual, EffectiveAt: effectiveAt, CreatedAt: effectiveAt}
	plan := productPlan{
		state: state, previews: []domain.ChangePreview{buy}, instruments: []domain.Instrument{instrument},
		observations: []domain.InstrumentPreferenceObservation{observation}, holdings: []domain.Holding{holding},
		quotes: []domain.InstrumentQuote{quote}, contracts: []domain.ProductContract{contract}, policies: []domain.LiquidityPolicy{policy},
		productLinks:  []domain.ProductOperationProduct{{OperationID: operationID, ProductID: productID, Role: domain.ProductRoleOpened}},
		activityLinks: []domain.ProductOperationActivity{{OperationID: operationID, ActivityID: buy.Activity.ID, Sequence: 1, Purpose: domain.ProductPurposeAcquisition, ProductID: productID}},
		productIDs:    []domain.ProductContractID{productID}, quoteIDs: []domain.InstrumentQuoteID{quote.ID},
		afterContracts: []domain.ProductContract{contract}, effectiveAt: effectiveAt,
	}
	_ = ctx
	return plan, nil
}

func (s *Service) planRecordExisting(ctx context.Context, origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot, state domain.ChangeState, input RecordExistingProductCommand, operationID domain.ProductOperationID, now time.Time) (productPlan, error) {
	if !input.CashExcludesProduct {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "cashExcludesProduct", Message: "acknowledge that the cash balance excludes this product"}
	}
	if strings.TrimSpace(input.TotalCostBasis) == "" {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "totalCostBasis", Message: "is required; missing basis is not assumed"}
	}
	accountID, err := domain.ParseAccountID(input.AccountID)
	if err != nil {
		return productPlan{}, err
	}
	currency, err := domain.ParseCurrency(input.Currency)
	if err != nil {
		return productPlan{}, err
	}
	principal, err := parseRequiredMoney("principal", input.Principal, currency)
	if err != nil {
		return productPlan{}, err
	}
	if principal.IsZero() {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "principal", Message: "must be greater than zero"}
	}
	totalCost, err := parseRequiredMoney("totalCostBasis", input.TotalCostBasis, currency)
	if err != nil {
		return productPlan{}, err
	}
	currentValue, err := parseRequiredMoney("currentValue", input.CurrentValue, currency)
	if err != nil {
		return productPlan{}, err
	}
	if currentValue.IsZero() {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "currentValue", Message: "must be greater than zero"}
	}
	effectiveAt := now.UTC()
	if strings.TrimSpace(input.EffectiveAt) != "" {
		parsed, parseErr := parseEffectiveAt(input.EffectiveAt, origin, now)
		if parseErr != nil {
			return productPlan{}, parseErr
		}
		if !parsed.Equal(now.UTC()) {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "recording an existing product uses the recording time; the contract start date is descriptive only"}
		}
		effectiveAt = parsed
	}
	record, ok := accountFromState(snapshot, accountID)
	if !ok {
		return productPlan{}, &domain.Error{Code: domain.ErrNotFound, Field: "accountId", Message: "Account was not found"}
	}
	if err := domain.ProductAccountEligible(record.Account); err != nil {
		return productPlan{}, err
	}
	kind, mode, rate, maturityInterest, paidThrough, err := resolveProductTerms(input.Terms, currency, principal)
	if err != nil {
		return productPlan{}, err
	}
	instrument, err := buildManagedInstrument(origin.HouseholdID, input.Terms.Name, currency, effectiveAt)
	if err != nil {
		return productPlan{}, err
	}
	quote, err := buildManagedQuote(instrument, currentValue, effectiveAt)
	if err != nil {
		return productPlan{}, err
	}
	holding, err := domain.NewHoldingForAccount(record.Account, instrument, zeroQuantity(), nil, 0, effectiveAt)
	if err != nil {
		return productPlan{}, err
	}
	state = addInstrumentToState(state, instrument, holding)
	unitCost, err := unitCostFromTotal(totalCost)
	if err != nil {
		return productPlan{}, err
	}
	adjustment, err := domain.PreviewChange(state, domain.PositionAdjustmentInput{
		HouseholdID: origin.HouseholdID, HoldingID: holding.ID, Quantity: oneQuantity(), Added: true, UnitCost: unitCost, EffectiveAt: effectiveAt,
	})
	if err != nil {
		return productPlan{}, err
	}
	state, err = applyPreview(state, adjustment)
	if err != nil {
		return productPlan{}, err
	}
	productID := domain.NewProductContractID()
	if paidThrough == nil && (mode == domain.InterestSimpleAct365 || mode == domain.InterestSimpleAct360) {
		start := input.Terms.StartOn
		paidThrough = &start
	}
	contract := domain.ProductContract{
		ID: productID, HouseholdID: origin.HouseholdID, AccountID: accountID, HoldingID: holding.ID,
		InstrumentID: instrument.ID, Kind: kind, Name: input.Terms.Name, Note: input.Terms.Note, Currency: currency,
		Principal: principal, StartOn: input.Terms.StartOn, MaturityOn: input.Terms.MaturityOn, InterestMode: mode,
		AnnualRate: rate, MaturityInterest: maturityInterest, InterestPaidThroughOn: paidThrough,
		State: domain.ProductStateOpen, OpenedOperationID: operationID, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := contract.Validate(); err != nil {
		return productPlan{}, err
	}
	policy, err := contractPolicyForOpen(origin.HouseholdID, accountID, holding.ID, kind, input.Terms.MaturityOn, input.Policy, currency, now)
	if err != nil {
		return productPlan{}, err
	}
	observation := domain.InstrumentPreferenceObservation{ID: domain.NewInstrumentPreferenceObservationID(), InstrumentID: instrument.ID, SourceKind: domain.QuoteSourceManual, EffectiveAt: effectiveAt, CreatedAt: effectiveAt}
	_ = ctx
	return productPlan{
		state: state, previews: []domain.ChangePreview{adjustment}, instruments: []domain.Instrument{instrument},
		observations: []domain.InstrumentPreferenceObservation{observation}, holdings: []domain.Holding{holding},
		quotes: []domain.InstrumentQuote{quote}, contracts: []domain.ProductContract{contract}, policies: []domain.LiquidityPolicy{policy},
		productLinks:  []domain.ProductOperationProduct{{OperationID: operationID, ProductID: productID, Role: domain.ProductRoleOpened}},
		activityLinks: []domain.ProductOperationActivity{{OperationID: operationID, ActivityID: adjustment.Activity.ID, Sequence: 1, Purpose: domain.ProductPurposeExistingPosition, ProductID: productID}},
		productIDs:    []domain.ProductContractID{productID}, quoteIDs: []domain.InstrumentQuoteID{quote.ID},
		afterContracts: []domain.ProductContract{contract}, effectiveAt: effectiveAt,
		warnings: []string{"existing_position_increases_assets"},
	}, nil
}

func (s *Service) planReceiveInterest(ctx context.Context, origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot, state domain.ChangeState, input ReceiveInterestCommand, operationID domain.ProductOperationID, now time.Time) (productPlan, error) {
	resolved, resolveErr := resolveProductEffectiveTime(input.EffectiveAt, input.EffectiveLocalDate, input.EffectiveLocalTime, origin, now)
	if resolveErr != nil {
		return productPlan{}, resolveErr
	}
	input.EffectiveAt = resolved
	productID, contract, err := s.loadOpenProduct(ctx, origin.HouseholdID, input.ProductID)
	if err != nil {
		return productPlan{}, err
	}
	if err := s.rejectStaleFinancialTime(ctx, origin.HouseholdID, productID, input.EffectiveAt, now, origin); err != nil {
		return productPlan{}, err
	}
	amount, err := parseRequiredMoney("amount", input.Amount, contract.Currency)
	if err != nil {
		return productPlan{}, err
	}
	if !amount.Amount().IsPositive() {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "amount", Message: "interest-only receipt requires an amount greater than zero"}
	}
	effectiveAt, err := parseEffectiveAt(input.EffectiveAt, origin, now)
	if err != nil {
		return productPlan{}, err
	}
	preview, err := domain.PreviewChange(state, domain.MoneyAddedInput{
		HouseholdID: origin.HouseholdID, AccountID: contract.AccountID, Amount: amount, Reason: domain.ReasonInterest, EffectiveAt: effectiveAt,
	})
	if err != nil {
		return productPlan{}, err
	}
	state, err = applyPreview(state, preview)
	if err != nil {
		return productPlan{}, err
	}
	before := contract
	updated := contract
	updated.UpdatedAt = now
	updated.Revision++
	switch contract.InterestMode {
	case domain.InterestSimpleAct365, domain.InterestSimpleAct360:
		if input.InterestPaidThroughOn == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "interestPaidThroughOn", Message: "is required after a simple-interest receipt"}
		}
		paidThrough, err := domain.ParseCivilDate("interestPaidThroughOn", *input.InterestPaidThroughOn)
		if err != nil {
			return productPlan{}, err
		}
		localDate, _, err := domain.LocalCivilDate(now, origin.Timezone)
		if err != nil {
			return productPlan{}, err
		}
		if compareCivil(paidThrough, localDate) > 0 {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "interestPaidThroughOn", Message: "must not be after today"}
		}
		updated.InterestPaidThroughOn = &paidThrough
	case domain.InterestManualMaturityAmount:
		remaining, err := parseOptionalMoney("remainingInterest", input.RemainingInterest, contract.Currency)
		if err != nil {
			return productPlan{}, err
		}
		if remaining == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "remainingInterest", Message: "is required after a manual maturity-interest receipt"}
		}
		updated.MaturityInterest = remaining
	}
	if err := updated.Validate(); err != nil {
		return productPlan{}, err
	}
	_ = snapshot
	return productPlan{
		state: state, previews: []domain.ChangePreview{preview}, contracts: []domain.ProductContract{updated},
		productLinks:  []domain.ProductOperationProduct{{OperationID: operationID, ProductID: productID, Role: domain.ProductRoleIncome}},
		activityLinks: []domain.ProductOperationActivity{{OperationID: operationID, ActivityID: preview.Activity.ID, Sequence: 1, Purpose: domain.ProductPurposeInterest, ProductID: productID}},
		productIDs:    []domain.ProductContractID{productID}, beforeContracts: []domain.ProductContract{before},
		afterContracts: []domain.ProductContract{updated}, effectiveAt: effectiveAt,
	}, nil
}

func (s *Service) planSettle(ctx context.Context, origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot, state domain.ChangeState, input SettleProductCommand, operationID domain.ProductOperationID, now time.Time, renewedFrom *domain.ProductContractID) (productPlan, error) {
	resolved, resolveErr := resolveProductEffectiveTime(input.EffectiveAt, input.EffectiveLocalDate, input.EffectiveLocalTime, origin, now)
	if resolveErr != nil {
		return productPlan{}, resolveErr
	}
	input.EffectiveAt = resolved
	productID, contract, err := s.loadOpenProduct(ctx, origin.HouseholdID, input.ProductID)
	if err != nil {
		return productPlan{}, err
	}
	if err := s.rejectStaleFinancialTime(ctx, origin.HouseholdID, productID, input.EffectiveAt, now, origin); err != nil {
		return productPlan{}, err
	}
	effectiveAt, err := parseEffectiveAt(input.EffectiveAt, origin, now)
	if err != nil {
		return productPlan{}, err
	}
	fee, err := parseOptionalMoney("fee", input.Fee, contract.Currency)
	if err != nil {
		return productPlan{}, err
	}
	var gross domain.Money
	var interest *domain.Money
	if contract.Kind == domain.ProductTermDeposit {
		if input.ReturnedPrincipal == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "returnedPrincipal", Message: "is required for a term-deposit settlement"}
		}
		principal, err := parseRequiredMoney("returnedPrincipal", *input.ReturnedPrincipal, contract.Currency)
		if err != nil {
			return productPlan{}, err
		}
		if !principal.Amount().IsPositive() || principal.Amount().GreaterThan(contract.Principal.Amount()) {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "returnedPrincipal", Message: "must be positive and not greater than original principal"}
		}
		gross = principal
		interest, err = parseOptionalMoney("interest", input.Interest, contract.Currency)
		if err != nil {
			return productPlan{}, err
		}
	} else {
		if input.GrossProceeds == nil {
			return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "grossProceeds", Message: "is required for a locked-product settlement"}
		}
		gross, err = parseRequiredMoney("grossProceeds", *input.GrossProceeds, contract.Currency)
		if err != nil {
			return productPlan{}, err
		}
		if !gross.Amount().IsPositive() {
			return productPlan{}, &domain.Error{Code: domain.ErrUnsupportedPartialOperation, Field: "grossProceeds", Message: "zero-proceeds write-off is not supported"}
		}
	}
	if fee != nil && fee.Amount().GreaterThan(gross.Amount()) {
		return productPlan{}, &domain.Error{Code: domain.ErrValidation, Field: "fee", Message: "must not exceed sale gross proceeds"}
	}
	sell, err := domain.PreviewChange(state, domain.TradeInput{
		HouseholdID: origin.HouseholdID, Side: domain.TradeSell, SettlementAccountID: contract.AccountID,
		HoldingID: contract.HoldingID, InstrumentID: contract.InstrumentID, Quantity: oneQuantity(), Gross: gross,
		Fee: optionalZeroFee(fee), EffectiveAt: effectiveAt,
	})
	if err != nil {
		return productPlan{}, err
	}
	state, err = applyPreview(state, sell)
	if err != nil {
		return productPlan{}, err
	}
	previews := []domain.ChangePreview{sell}
	activityLinks := []domain.ProductOperationActivity{{OperationID: operationID, ActivityID: sell.Activity.ID, Sequence: 1, Purpose: domain.ProductPurposeRedemption, ProductID: productID}}
	if interest != nil && interest.Amount().IsPositive() {
		income, err := domain.PreviewChange(state, domain.MoneyAddedInput{
			HouseholdID: origin.HouseholdID, AccountID: contract.AccountID, Amount: *interest, Reason: domain.ReasonInterest, EffectiveAt: effectiveAt,
		})
		if err != nil {
			return productPlan{}, err
		}
		state, err = applyPreview(state, income)
		if err != nil {
			return productPlan{}, err
		}
		previews = append(previews, income)
		activityLinks = append(activityLinks, domain.ProductOperationActivity{OperationID: operationID, ActivityID: income.Activity.ID, Sequence: 2, Purpose: domain.ProductPurposeInterest, ProductID: productID})
	}
	reservationLinks, releaseIDs, err := s.reservationReleases(ctx, origin.HouseholdID, contract, input.ReleaseReservationIDs, now)
	if err != nil {
		return productPlan{}, err
	}
	before := contract
	updated := contract
	updated.State = domain.ProductStateSettled
	updated.ClosedOperationID = &operationID
	updated.Revision++
	updated.UpdatedAt = now
	if err := updated.Validate(); err != nil {
		return productPlan{}, err
	}
	_ = snapshot
	_ = renewedFrom
	return productPlan{
		state: state, previews: previews, contracts: []domain.ProductContract{updated},
		productLinks:  []domain.ProductOperationProduct{{OperationID: operationID, ProductID: productID, Role: domain.ProductRoleSettled}},
		activityLinks: activityLinks, reservationLinks: reservationLinks, productIDs: []domain.ProductContractID{productID},
		releaseIDs: releaseIDs, beforeContracts: []domain.ProductContract{before}, afterContracts: []domain.ProductContract{updated},
		effectiveAt: effectiveAt,
	}, nil
}

func (s *Service) planRenew(ctx context.Context, origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot, state domain.ChangeState, input RenewProductCommand, operationID domain.ProductOperationID, now time.Time) (productPlan, error) {
	settlePlan, err := s.planSettle(ctx, origin, snapshot, state, input.Settle, operationID, now, nil)
	if err != nil {
		return productPlan{}, err
	}
	openInput := OpenProductCommand{
		AccountID: settlePlan.contracts[0].AccountID.String(), Currency: settlePlan.contracts[0].Currency.String(),
		Principal: input.Principal, OpeningFee: input.OpeningFee, EffectiveAt: settlePlan.effectiveAt.UTC().Format(time.RFC3339Nano),
		Terms: input.Terms, Policy: input.Policy,
	}
	openPlan, err := s.planOpen(ctx, origin, snapshot, settlePlan.state, openInput, operationID, now)
	if err != nil {
		return productPlan{}, err
	}
	if len(openPlan.contracts) == 1 && len(settlePlan.contracts) == 1 {
		predecessor := settlePlan.contracts[0].ID
		openPlan.contracts[0].RenewedFromID = &predecessor
		if err := openPlan.contracts[0].Validate(); err != nil {
			return productPlan{}, err
		}
	}
	sequence := len(settlePlan.activityLinks)
	for index := range openPlan.activityLinks {
		openPlan.activityLinks[index].Sequence = sequence + index + 1
		openPlan.activityLinks[index].OperationID = operationID
	}
	plan := productPlan{
		state:       openPlan.state,
		previews:    append(append([]domain.ChangePreview{}, settlePlan.previews...), openPlan.previews...),
		instruments: openPlan.instruments, observations: openPlan.observations, holdings: openPlan.holdings, quotes: openPlan.quotes,
		contracts:        append(append([]domain.ProductContract{}, settlePlan.contracts...), openPlan.contracts...),
		policies:         openPlan.policies,
		productLinks:     append(settlePlan.productLinks, openPlan.productLinks...),
		activityLinks:    append(settlePlan.activityLinks, openPlan.activityLinks...),
		reservationLinks: settlePlan.reservationLinks,
		productIDs:       append(settlePlan.productIDs, openPlan.productIDs...),
		quoteIDs:         openPlan.quoteIDs, releaseIDs: settlePlan.releaseIDs,
		beforeContracts: settlePlan.beforeContracts, afterContracts: append(append([]domain.ProductContract{}, settlePlan.contracts...), openPlan.contracts...),
		effectiveAt: settlePlan.effectiveAt,
	}
	return plan, nil
}

func (s *Service) planUndo(ctx context.Context, origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot, state domain.ChangeState, input UndoProductCommand, operationID domain.ProductOperationID, now time.Time) (productPlan, error) {
	targetID, err := domain.ParseProductOperationID(input.OperationID)
	if err != nil {
		return productPlan{}, err
	}
	evidence, err := s.repository.ProductOperationEvidence(ctx, origin.HouseholdID, targetID)
	if err != nil {
		return productPlan{}, err
	}
	if evidence.Operation.Kind == domain.ProductOpValueObservation {
		return productPlan{}, &domain.Error{Code: domain.ErrUnsafeUndo, Field: "operationId", Message: "valuation observations are corrected through quote supersession"}
	}
	if evidence.Operation.ReversesOperationID != nil {
		return productPlan{}, &domain.Error{Code: domain.ErrAlreadyUndone, Message: "a reversal cannot be undone again"}
	}
	stored, err := receiptEvidenceJSON(evidence.Operation.ResultJSON)
	if err != nil {
		return productPlan{}, err
	}
	currentContracts := []domain.ProductContract{}
	for _, productID := range uniqueProductIDs(evidence.Products) {
		ops, err := s.productHistoryForUndo(ctx, origin.HouseholdID, productID)
		if err != nil {
			return productPlan{}, err
		}
		if !isLatestActiveOperation(ops, targetID) {
			return productPlan{}, &domain.Error{Code: domain.ErrUnsafeUndo, Field: "operationId", Message: "only the latest operation for each product can be undone"}
		}
		current, err := s.repository.Product(ctx, origin.HouseholdID, productID)
		if err != nil {
			return productPlan{}, err
		}
		currentContracts = append(currentContracts, current)
		if !revisionMatchesAfterReversals(stored.AfterContracts, current, ops) {
			return productPlan{}, &domain.Error{Code: domain.ErrUnsafeUndo, Field: "productId", Message: "later contract or policy edits block undo"}
		}
	}
	previews := []domain.ChangePreview{}
	activityLinks := []domain.ProductOperationActivity{}
	working := state
	for index := len(evidence.Activities) - 1; index >= 0; index-- {
		link := evidence.Activities[index]
		activity, err := s.repository.Activity(ctx, origin.HouseholdID, link.ActivityID)
		if err != nil {
			return productPlan{}, err
		}
		effects := activity.Effects
		if len(effects) == 0 {
			effects, err = s.repository.ActivityEffects(ctx, link.ActivityID)
			if err != nil {
				return productPlan{}, err
			}
		}
		inverse, err := domain.InverseChange(working, activity, effects)
		if err != nil {
			return productPlan{}, err
		}
		working, err = applyPreview(working, inverse)
		if err != nil {
			return productPlan{}, err
		}
		previews = append(previews, inverse)
		activityLinks = append(activityLinks, domain.ProductOperationActivity{
			OperationID: operationID, ActivityID: inverse.Activity.ID, Sequence: len(activityLinks) + 1,
			Purpose: domain.ProductPurposeReversal, ProductID: link.ProductID,
		})
	}
	restored, err := s.contractsAfterUndo(ctx, origin.HouseholdID, stored, operationID, now)
	if err != nil {
		return productPlan{}, err
	}
	reservationLinks := make([]domain.ProductOperationReservation, 0, len(evidence.Reservations))
	reservations, err := s.repository.ListLiquidityReservations(ctx, origin.HouseholdID, true)
	if err != nil {
		return productPlan{}, err
	}
	byID := make(map[domain.LiquidityReservationID]domain.LiquidityReservation, len(reservations))
	for _, reservation := range reservations {
		byID[reservation.ID] = reservation
	}
	for _, link := range evidence.Reservations {
		current, ok := byID[link.ReservationID]
		if !ok || current.Revision != link.ResultingRevision || !sameOptionalTime(current.ReleasedAt, link.ResultingReleasedAt) {
			return productPlan{}, &domain.Error{Code: domain.ErrUnsafeUndo, Field: "reservationId", Message: "later reservation edits block undo"}
		}
		reservationLinks = append(reservationLinks, domain.ProductOperationReservation{
			OperationID: operationID, ReservationID: link.ReservationID,
			PreviousReleasedAt: link.ResultingReleasedAt, ResultingReleasedAt: link.PreviousReleasedAt,
			ResultingRevision: link.ResultingRevision + 1,
		})
	}
	productLinks := make([]domain.ProductOperationProduct, 0, len(evidence.Products))
	for _, link := range evidence.Products {
		role := link.Role
		switch link.Role {
		case domain.ProductRoleOpened:
			role = domain.ProductRoleCancelled
		case domain.ProductRoleSettled:
			role = domain.ProductRoleReopened
		}
		productLinks = append(productLinks, domain.ProductOperationProduct{OperationID: operationID, ProductID: link.ProductID, Role: role})
	}
	_ = snapshot
	return productPlan{
		state: working, previews: previews, contracts: restored, productLinks: productLinks, activityLinks: activityLinks,
		reservationLinks: reservationLinks, productIDs: uniqueProductIDs(evidence.Products),
		beforeContracts: currentContracts, afterContracts: restored, effectiveAt: now, reverses: &targetID,
	}, nil
}

func (s *Service) contractsAfterUndo(ctx context.Context, householdID domain.HouseholdID, stored productReceiptEvidence, operationID domain.ProductOperationID, now time.Time) ([]domain.ProductContract, error) {
	beforeByID := map[domain.ProductContractID]domain.ProductContract{}
	for _, contract := range stored.BeforeContracts {
		beforeByID[contract.ID] = contract
	}
	restored := make([]domain.ProductContract, 0, len(stored.BeforeContracts)+len(stored.AfterContracts))
	seen := map[domain.ProductContractID]struct{}{}
	for _, before := range stored.BeforeContracts {
		current, err := s.repository.Product(ctx, householdID, before.ID)
		if err != nil {
			return nil, err
		}
		next := before
		next.UpdatedAt = now
		if next.State == domain.ProductStateOpen {
			next.ClosedOperationID = nil
		}
		next.Revision = current.Revision + 1
		if err := next.Validate(); err != nil {
			return nil, err
		}
		restored = append(restored, next)
		seen[next.ID] = struct{}{}
	}
	for _, created := range stored.AfterContracts {
		if _, ok := beforeByID[created.ID]; ok {
			continue
		}
		if _, ok := seen[created.ID]; ok {
			continue
		}
		current, err := s.repository.Product(ctx, householdID, created.ID)
		if err != nil {
			return nil, err
		}
		if err := s.rejectActiveReservations(ctx, householdID, current); err != nil {
			return nil, err
		}
		next := current
		next.State = domain.ProductStateCancelled
		closed := operationID
		next.ClosedOperationID = &closed
		next.Revision = current.Revision + 1
		next.UpdatedAt = now
		if err := next.Validate(); err != nil {
			return nil, err
		}
		restored = append(restored, next)
		seen[next.ID] = struct{}{}
	}
	return restored, nil
}

func (s *Service) rejectActiveReservations(ctx context.Context, householdID domain.HouseholdID, contract domain.ProductContract) error {
	reservations, err := s.repository.ListLiquidityReservations(ctx, householdID, false)
	if err != nil {
		return err
	}
	sourceKey := domain.HoldingSourceRef(contract.AccountID, contract.HoldingID).Key()
	for _, reservation := range reservations {
		if reservation.Source.Key() == sourceKey && reservation.Active() {
			return &domain.Error{Code: domain.ErrUnsafeUndo, Field: "reservationId", Message: "release active reservations before undoing the opening"}
		}
	}
	return nil
}

func (s *Service) loadOpenProduct(ctx context.Context, householdID domain.HouseholdID, raw string) (domain.ProductContractID, domain.ProductContract, error) {
	productID, err := domain.ParseProductContractID(raw)
	if err != nil {
		return "", domain.ProductContract{}, err
	}
	contract, err := s.repository.Product(ctx, householdID, productID)
	if err != nil {
		return "", domain.ProductContract{}, err
	}
	if contract.State != domain.ProductStateOpen {
		return "", domain.ProductContract{}, &domain.Error{Code: domain.ErrConflict, Field: "productId", Message: "only an open product can receive this operation"}
	}
	return productID, contract, nil
}

func (s *Service) rejectStaleFinancialTime(ctx context.Context, householdID domain.HouseholdID, productID domain.ProductContractID, effectiveAt string, now time.Time, origin *domain.HistoryOrigin) error {
	parsed, err := parseEffectiveAt(effectiveAt, origin, now)
	if err != nil {
		return err
	}
	ops, err := s.repository.ListProductOperations(ctx, householdID, productID, 100, "")
	if err != nil {
		return err
	}
	for _, operation := range ops {
		if operation.Kind == domain.ProductOpValueObservation || operation.ReversesOperationID != nil {
			continue
		}
		if reversedBy(ops, operation.ID) {
			continue
		}
		if parsed.Before(operation.EffectiveAt) {
			return &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveAt", Message: "a product operation cannot precede the latest financial operation"}
		}
		break
	}
	return nil
}

func (s *Service) reservationReleases(ctx context.Context, householdID domain.HouseholdID, contract domain.ProductContract, requested []string, now time.Time) ([]domain.ProductOperationReservation, []domain.LiquidityReservationID, error) {
	reservations, err := s.repository.ListLiquidityReservations(ctx, householdID, false)
	if err != nil {
		return nil, nil, err
	}
	sourceKey := domain.HoldingSourceRef(contract.AccountID, contract.HoldingID).Key()
	active := []domain.LiquidityReservation{}
	for _, reservation := range reservations {
		if reservation.Source.Key() == sourceKey && reservation.Active() {
			active = append(active, reservation)
		}
	}
	requestedSet := map[string]struct{}{}
	for _, id := range requested {
		requestedSet[strings.TrimSpace(id)] = struct{}{}
	}
	if len(active) > 0 && len(requestedSet) == 0 {
		return nil, nil, &domain.Error{Code: domain.ErrUnresolvedReservationRelease, Field: "releaseReservationIds", Message: "active reservations must be released explicitly"}
	}
	links := []domain.ProductOperationReservation{}
	ids := []domain.LiquidityReservationID{}
	for _, reservation := range active {
		if _, ok := requestedSet[reservation.ID.String()]; !ok {
			return nil, nil, &domain.Error{Code: domain.ErrUnresolvedReservationRelease, Field: "releaseReservationIds", Message: "active reservations must be released explicitly"}
		}
		released := now
		links = append(links, domain.ProductOperationReservation{
			OperationID: domain.ProductOperationID(""), ReservationID: reservation.ID, PreviousReleasedAt: reservation.ReleasedAt,
			ResultingReleasedAt: &released, ResultingRevision: reservation.Revision + 1,
		})
		ids = append(ids, reservation.ID)
	}
	return links, ids, nil
}

func (s *Service) reviewedStateHash(ctx context.Context, snapshot domain.PortfolioSnapshot, command ProductCommand, payloadSHA, localDate string, plan productPlan) (string, error) {
	type fact struct {
		LocalDate    string                        `json:"localDate"`
		CommandSHA   string                        `json:"commandSha"`
		Cash         []string                      `json:"cash"`
		Holdings     []string                      `json:"holdings"`
		Contracts    []string                      `json:"contracts"`
		Quotes       []string                      `json:"quotes"`
		Reservations []domain.LiquidityReservation `json:"reservations"`
		Policies     []domain.LiquidityPolicy      `json:"policies"`
	}
	payload := fact{LocalDate: localDate, CommandSHA: payloadSHA}
	for _, values := range snapshot.CashValues {
		payload.Cash = append(payload.Cash, values.AccountID.String()+":"+values.Amount.Currency().String()+":"+values.Amount.CanonicalAmount())
	}
	sort.Strings(payload.Cash)
	for _, holding := range snapshot.Holdings {
		payload.Holdings = append(payload.Holdings, holding.ID.String()+":"+holding.Quantity.Canonical())
	}
	sort.Strings(payload.Holdings)
	for _, contract := range plan.beforeContracts {
		payload.Contracts = append(payload.Contracts, contract.ID.String()+":"+strconv.Itoa(contract.Revision))
	}
	sort.Strings(payload.Contracts)
	for _, quote := range snapshot.InstrumentQuotes {
		payload.Quotes = append(payload.Quotes, quote.InstrumentID.String()+":"+quote.ID.String())
	}
	sort.Strings(payload.Quotes)
	if len(plan.beforeContracts) > 0 {
		household := plan.beforeContracts[0].HouseholdID
		keys := map[string]bool{}
		for _, c := range plan.beforeContracts {
			keys[domain.HoldingSourceRef(c.AccountID, c.HoldingID).Key()] = true
		}
		reservations, err := s.repository.ListLiquidityReservations(ctx, household, true)
		if err != nil {
			return "", err
		}
		for _, r := range reservations {
			if keys[r.Source.Key()] {
				payload.Reservations = append(payload.Reservations, r)
			}
		}
		policies, err := s.repository.ListLiquidityPolicies(ctx, household)
		if err != nil {
			return "", err
		}
		for _, p := range policies {
			if keys[p.Source.Key()] {
				payload.Policies = append(payload.Policies, p)
			}
		}
		sort.Slice(payload.Reservations, func(i, j int) bool { return payload.Reservations[i].ID.String() < payload.Reservations[j].ID.String() })
		sort.Slice(payload.Policies, func(i, j int) bool { return payload.Policies[i].ID.String() < payload.Policies[j].ID.String() })
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeProductCommand(command ProductCommand) (string, string, error) {
	kind, err := domain.ParseProductOperationKind(string(command.Kind))
	if err != nil {
		return "", "", err
	}
	command.Kind = kind
	filled := 0
	if command.Open != nil {
		filled++
	}
	if command.RecordExisting != nil {
		filled++
	}
	if command.ReceiveInterest != nil {
		filled++
	}
	if command.Settle != nil {
		filled++
	}
	if command.Renew != nil {
		filled++
	}
	if command.Undo != nil {
		filled++
	}
	if filled != 1 {
		return "", "", &domain.Error{Code: domain.ErrValidation, Field: "kind", Message: "exactly one payload must match kind"}
	}
	raw, err := json.Marshal(command)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	return string(raw), hex.EncodeToString(sum[:]), nil
}

func collectCash(before, after domain.ChangeState, plan productPlan) (cashBefore, cashAfter []domain.Money) {
	type endpoint struct {
		account  domain.AccountID
		currency domain.CurrencyCode
	}
	seen := map[endpoint]bool{}
	for _, contracts := range [][]domain.ProductContract{plan.beforeContracts, plan.contracts} {
		for _, c := range contracts {
			seen[endpoint{c.AccountID, c.Currency}] = true
		}
	}
	endpoints := make([]endpoint, 0, len(seen))
	for e := range seen {
		endpoints = append(endpoints, e)
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].account != endpoints[j].account {
			return endpoints[i].account.String() < endpoints[j].account.String()
		}
		return endpoints[i].currency < endpoints[j].currency
	})
	for _, e := range endpoints {
		if money, err := currentCashAmount(before, e.account, e.currency); err == nil {
			cashBefore = append(cashBefore, money)
		}
		if money, err := currentCashAmount(after, e.account, e.currency); err == nil {
			cashAfter = append(cashAfter, money)
		}
	}
	return cashBefore, cashAfter
}

func (p productPlan) OpenAccountID() domain.AccountID {
	if len(p.contracts) > 0 {
		return p.contracts[0].AccountID
	}
	return ""
}

func receiptFromOperation(operation domain.ProductOperation) (ProductOperationReceipt, error) {
	stored, err := receiptEvidenceJSON(operation.ResultJSON)
	if err != nil {
		return ProductOperationReceipt{}, err
	}
	return stored.Receipt, nil
}

func receiptEvidenceJSON(raw string) (productReceiptEvidence, error) {
	var stored productReceiptEvidence
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return productReceiptEvidence{}, err
	}
	return stored, nil
}

func uniqueProductIDs(links []domain.ProductOperationProduct) []domain.ProductContractID {
	seen := map[domain.ProductContractID]struct{}{}
	ids := []domain.ProductContractID{}
	for _, link := range links {
		if _, ok := seen[link.ProductID]; ok {
			continue
		}
		seen[link.ProductID] = struct{}{}
		ids = append(ids, link.ProductID)
	}
	return ids
}

func isLatestActiveOperation(ops []domain.ProductOperation, id domain.ProductOperationID) bool {
	for _, operation := range ops {
		if operation.ReversesOperationID != nil || reversedBy(ops, operation.ID) {
			continue
		}
		return operation.ID == id
	}
	return false
}

func reversedBy(ops []domain.ProductOperation, id domain.ProductOperationID) bool {
	for _, operation := range ops {
		if operation.ReversesOperationID != nil && *operation.ReversesOperationID == id {
			return true
		}
	}
	return false
}

func revisionMatches(after []domain.ProductContract, current domain.ProductContract) bool {
	for _, contract := range after {
		if contract.ID == current.ID {
			return contract.Revision == current.Revision && contract.State == current.State
		}
	}
	return false
}

func compareCivil(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func mustLocation(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return zone
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

// Follow only recorded restoration edges. An intervening edit has no edge,
// even if it happens to leave the same business values, and still blocks undo.
func revisionMatchesAfterReversals(after []domain.ProductContract, current domain.ProductContract, ops []domain.ProductOperation) bool {
	byID := make(map[domain.ProductOperationID]domain.ProductOperation, len(ops))
	for _, op := range ops {
		byID[op.ID] = op
	}
	for attempts := 0; attempts <= len(ops); attempts++ {
		if revisionMatches(after, current) {
			return true
		}
		advanced := false
		for _, op := range ops {
			if op.ReversesOperationID == nil {
				continue
			}
			restored, err := receiptEvidenceJSON(op.ResultJSON)
			if err != nil || !revisionMatches(restored.AfterContracts, current) {
				continue
			}
			original, ok := byID[*op.ReversesOperationID]
			if !ok {
				return false
			}
			before, err := receiptEvidenceJSON(original.ResultJSON)
			if err != nil {
				return false
			}
			for _, contract := range before.BeforeContracts {
				if contract.ID == current.ID && contract.Revision < current.Revision {
					current = contract
					advanced = true
					break
				}
			}
			if advanced {
				break
			}
		}
		if !advanced {
			return false
		}
	}
	return false
}

func (s *Service) productHistoryForUndo(ctx context.Context, householdID domain.HouseholdID, productID domain.ProductContractID) ([]domain.ProductOperation, error) {
	var history []domain.ProductOperation
	cursor := ""
	for {
		page, err := s.repository.ListProductOperations(ctx, householdID, productID, 100, cursor)
		if err != nil {
			return nil, err
		}
		history = append(history, page...)
		if len(page) < 100 {
			return history, nil
		}
		cursor = encodeProductOperationCursor(page[len(page)-1])
	}
}
