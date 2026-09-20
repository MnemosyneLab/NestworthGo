package application

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) LiquidityOverview(ctx context.Context, query LiquidityOverviewQuery) (domain.LiquidityOverview, error) {
	snapshot, err := s.repository.ReadLiquiditySnapshot(ctx)
	if err != nil {
		return domain.LiquidityOverview{}, err
	}
	if snapshot.Portfolio.Household == nil {
		return domain.LiquidityOverview{}, onboardingRequired()
	}
	timezone := "UTC"
	origin := snapshot.Portfolio.Origin
	if origin != nil && strings.TrimSpace(origin.Timezone) != "" {
		timezone = origin.Timezone
	}
	sources, err := s.liquiditySources(snapshot)
	if err != nil {
		return domain.LiquidityOverview{}, err
	}
	convert := func(amount domain.Money) (*decimal.Decimal, bool, error) {
		return s.valuation.ConvertAmount(snapshot.Portfolio, amount)
	}
	return domain.EvaluateLiquidity(domain.LiquidityQuery{
		AsOf: s.clock(), Timezone: timezone, BaseCurrency: snapshot.Portfolio.Household.BaseCurrency,
		CustomHorizonOn: query.CustomHorizonOn, IncludeEarlyWithdrawal: query.IncludeEarlyWithdrawal,
	}, sources, snapshot.Reservations, convert)
}

func (v *ValuationService) ConvertAmount(snapshot domain.PortfolioSnapshot, amount domain.Money) (*decimal.Decimal, bool, error) {
	if snapshot.Household == nil {
		return nil, false, nil
	}
	converted, _, missing, err := v.convert(snapshot, "", amount.Amount(), amount.Currency())
	if err != nil {
		return nil, false, err
	}
	if missing != nil {
		return nil, false, nil
	}
	copy := converted
	return &copy, true, nil
}

func (s *Service) liquiditySources(snapshot domain.LiquiditySnapshot) ([]domain.LiquiditySource, error) {
	valued, _, err := s.valuation.ValueAccounts(snapshot.Portfolio)
	if err != nil {
		return nil, err
	}
	contractsByHolding := map[domain.HoldingID]domain.ProductContract{}
	for _, contract := range snapshot.Contracts {
		contractsByHolding[contract.HoldingID] = contract
	}
	policiesByKey := map[string]domain.LiquidityPolicy{}
	for _, policy := range snapshot.Policies {
		policiesByKey[policy.Source.Key()] = policy
	}
	holdingsByID := map[domain.HoldingID]domain.Holding{}
	for _, holding := range snapshot.Portfolio.Holdings {
		holdingsByID[holding.ID] = holding
	}
	instrumentsByID := map[domain.InstrumentID]domain.Instrument{}
	for _, instrument := range snapshot.Portfolio.Instruments {
		instrumentsByID[instrument.ID] = instrument
	}
	accountsByID := map[domain.AccountID]domain.Account{}
	for _, record := range snapshot.Portfolio.Accounts {
		accountsByID[record.Account.ID] = record.Account
	}
	sources := []domain.LiquiditySource{}
	for _, account := range valued {
		for _, component := range account.Components {
			source, err := liquiditySourceFromComponent(account, component, contractsByHolding, policiesByKey, holdingsByID, instrumentsByID)
			if err != nil {
				return nil, err
			}
			sources = append(sources, source)
		}
	}
	_ = accountsByID
	return sources, nil
}

func liquiditySourceFromComponent(account domain.AccountValuation, component domain.ValuationComponent, contracts map[domain.HoldingID]domain.ProductContract, policies map[string]domain.LiquidityPolicy, holdings map[domain.HoldingID]domain.Holding, instruments map[domain.InstrumentID]domain.Instrument) (domain.LiquiditySource, error) {
	source := domain.LiquiditySource{
		AccountID: account.Account.ID, DisplayName: account.Account.Name, NativeCurrency: component.NativeCurrency,
		IncludeInNetWorth: account.Account.IncludeInNetWorth, Archived: account.Account.ArchivedAt != nil,
		Liability: account.Account.IsLiability(), AccountType: account.Account.AccountType, TrackingMode: account.Account.TrackingMode,
		PriceEvidence: component.PriceEvidence, FXEvidence: component.FXEvidence,
	}
	if component.NativeAmount != "" {
		money, err := domain.ParseMoney(component.NativeAmount, component.NativeCurrency)
		if err != nil {
			return domain.LiquiditySource{}, err
		}
		source.CurrentNativeValue = &money
	}
	if component.HoldingID != nil {
		source.Ref = domain.HoldingSourceRef(account.Account.ID, *component.HoldingID)
		if holding, ok := holdings[*component.HoldingID]; ok {
			source.QuantityZero = holding.Quantity.IsZero()
			if instrument, ok := instruments[holding.InstrumentID]; ok {
				source.DisplayName = instrument.Name
				kind := instrument.Type
				source.InstrumentType = &kind
			}
		}
		if contract, ok := contracts[*component.HoldingID]; ok {
			copy := contract
			source.Managed = true
			source.Contract = &copy
			source.ProductID = &copy.ID
			source.DisplayName = contract.Name
		}
	} else if account.Account.TrackingMode == domain.TrackingHoldings {
		source.Ref = domain.AccountCashSourceRef(account.Account.ID, component.NativeCurrency)
		source.DisplayName = account.Account.Name + " cash"
	} else {
		source.Ref = domain.AccountValueSourceRef(account.Account.ID)
	}
	if policy, ok := policies[source.Ref.Key()]; ok {
		copy := policy
		source.ExplicitPolicy = &copy
	}
	return source, nil
}

func (s *Service) Product(ctx context.Context, productID domain.ProductContractID) (ProductDetail, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	contract, err := s.repository.Product(ctx, household.ID, productID)
	if err != nil {
		return ProductDetail{}, err
	}
	return s.productDetail(ctx, household.ID, contract)
}

func (s *Service) ListProducts(ctx context.Context, accountID *domain.AccountID, includeClosed bool) ([]ProductDetail, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	contracts, err := s.repository.ListProducts(ctx, household.ID, accountID, includeClosed)
	if err != nil {
		return nil, err
	}
	details := make([]ProductDetail, 0, len(contracts))
	for _, contract := range contracts {
		detail, err := s.productDetail(ctx, household.ID, contract)
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
}

func (s *Service) ListProductOperations(ctx context.Context, productID domain.ProductContractID, cursor string, limit int) (ProductOperationPage, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return ProductOperationPage{}, err
	}
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	operations, err := s.repository.ListProductOperations(ctx, household.ID, productID, limit+1, cursor)
	if err != nil {
		return ProductOperationPage{}, err
	}
	page := ProductOperationPage{Operations: operations}
	if len(operations) > limit {
		page.Operations = operations[:limit]
		next := encodeProductOperationCursor(operations[limit-1])
		page.Next = &next
	}
	return page, nil
}

func encodeProductOperationCursor(operation domain.ProductOperation) string {
	return operation.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + operation.ID.String()
}

func (s *Service) productDetail(ctx context.Context, householdID domain.HouseholdID, contract domain.ProductContract) (ProductDetail, error) {
	policies, err := s.repository.ListLiquidityPolicies(ctx, householdID)
	if err != nil {
		return ProductDetail{}, err
	}
	var policy domain.LiquidityPolicy
	key := domain.HoldingSourceRef(contract.AccountID, contract.HoldingID).Key()
	for _, candidate := range policies {
		if candidate.Source.Key() == key {
			policy = candidate
			break
		}
	}
	reservations, err := s.repository.ListLiquidityReservations(ctx, householdID, false)
	if err != nil {
		return ProductDetail{}, err
	}
	active := []domain.LiquidityReservation{}
	for _, reservation := range reservations {
		if reservation.Source.Key() == key && reservation.Active() {
			active = append(active, reservation)
		}
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return ProductDetail{}, err
	}
	valued, _, err := s.valuation.ValueAccounts(snapshot)
	if err != nil {
		return ProductDetail{}, err
	}
	var current *domain.Money
	for _, account := range valued {
		for _, component := range account.Components {
			if component.HoldingID != nil && *component.HoldingID == contract.HoldingID && component.NativeAmount != "" {
				money, err := domain.ParseMoney(component.NativeAmount, component.NativeCurrency)
				if err != nil {
					return ProductDetail{}, err
				}
				current = &money
			}
		}
	}
	display := domain.ProductDisplaySettled
	switch contract.State {
	case domain.ProductStateCancelled:
		display = domain.ProductDisplayCancelled
	case domain.ProductStateOpen:
		display = domain.ProductDisplayLocked
		if policy.UnlockOn != nil {
			localDate, _, err := domain.LocalCivilDate(s.clock(), timezoneOrUTC(snapshot.Origin))
			if err != nil {
				return ProductDetail{}, err
			}
			if compareCivil(localDate, *policy.UnlockOn) >= 0 {
				display = domain.ProductDisplayRedeemable
			}
			if contract.MaturityOn != nil && compareCivil(localDate, *contract.MaturityOn) >= 0 {
				display = domain.ProductDisplayDueUnconfirmed
			}
		}
	}
	disabled := map[string]string{}
	actions := []string{}
	if contract.State == domain.ProductStateOpen {
		actions = append(actions, "settle", "receive_interest", "renew")
		if contract.Kind == domain.ProductLockedProduct {
			actions = append(actions, "value_observation")
		}
	} else {
		disabled["settle"] = "the contract is not open"
	}
	return ProductDetail{
		Contract: contract, Policy: policy, CurrentValue: current, DisplayState: display,
		Reservations: active, PredecessorID: contract.RenewedFromID, PermittedActions: actions, DisabledReasons: disabled,
	}, nil
}

func timezoneOrUTC(origin *domain.HistoryOrigin) string {
	if origin != nil && strings.TrimSpace(origin.Timezone) != "" {
		return origin.Timezone
	}
	return "UTC"
}

func (s *Service) SaveLiquidityPolicy(ctx context.Context, input SavePolicyInput) (domain.LiquidityPolicy, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	defer unlock()
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if err := input.Source.Validate(); err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if input.Source.HoldingID != nil {
		if product, err := s.repository.ProductByHolding(ctx, *input.Source.HoldingID); err != nil {
			return domain.LiquidityPolicy{}, err
		} else if product != nil {
			return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrManagedPosition, Field: "source", Message: "managed product policy is updated with contract terms"}
		}
	}
	existing, err := s.policyBySource(ctx, household.ID, input.Source)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	currency := household.BaseCurrency
	if input.Source.Currency != nil {
		currency = *input.Source.Currency
	}
	policy, err := policyFromInput(household.ID, input.Source, input.Policy, currency, false, s.clock())
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if existing == nil {
		if input.ExpectedRevision != 0 {
			return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "policy does not exist"}
		}
		if err := s.repository.SaveLiquidityPolicy(ctx, policy); err != nil {
			return domain.LiquidityPolicy{}, err
		}
		return policy, nil
	}
	if input.ExpectedRevision != existing.Revision {
		return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "policy revision does not match"}
	}
	policy.ID = existing.ID
	policy.CreatedAt = existing.CreatedAt
	policy.Revision = existing.Revision + 1
	if err := s.repository.SaveLiquidityPolicy(ctx, policy); err != nil {
		return domain.LiquidityPolicy{}, err
	}
	return policy, nil
}

func (s *Service) ResetLiquidityPolicy(ctx context.Context, source domain.LiquiditySourceRef, expectedRevision int) error {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	if source.HoldingID != nil {
		if product, err := s.repository.ProductByHolding(ctx, *source.HoldingID); err != nil {
			return err
		} else if product != nil {
			return &domain.Error{Code: domain.ErrManagedPosition, Field: "source", Message: "managed product policy cannot be reset"}
		}
	}
	existing, err := s.policyBySource(ctx, household.ID, source)
	if err != nil {
		return err
	}
	if existing == nil {
		return &domain.Error{Code: domain.ErrNotFound, Field: "source", Message: "policy was not found"}
	}
	if expectedRevision != existing.Revision {
		return &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "policy revision does not match"}
	}
	return s.repository.DeleteLiquidityPolicy(ctx, existing.ID, expectedRevision)
}

func (s *Service) SaveLiquidityReservation(ctx context.Context, input SaveReservationInput) (domain.LiquidityReservation, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	defer unlock()
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	if err := input.Source.Validate(); err != nil {
		return domain.LiquidityReservation{}, err
	}
	currency := household.BaseCurrency
	if input.Source.Currency != nil {
		currency = *input.Source.Currency
	}
	amount, err := parseRequiredMoney("amount", input.Amount, currency)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	now := s.clock()
	if input.ID == nil {
		if input.ExpectedRevision != 0 {
			return domain.LiquidityReservation{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "reservation does not exist"}
		}
		reservation := domain.LiquidityReservation{
			ID: domain.NewLiquidityReservationID(), HouseholdID: household.ID, Source: input.Source,
			Label: input.Label, Amount: amount, Currency: amount.Currency(), Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := reservation.Validate(); err != nil {
			return domain.LiquidityReservation{}, err
		}
		if err := s.repository.SaveLiquidityReservation(ctx, reservation); err != nil {
			return domain.LiquidityReservation{}, err
		}
		return reservation, nil
	}
	reservations, err := s.repository.ListLiquidityReservations(ctx, household.ID, true)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	var current *domain.LiquidityReservation
	for index := range reservations {
		if reservations[index].ID == *input.ID {
			current = &reservations[index]
			break
		}
	}
	if current == nil {
		return domain.LiquidityReservation{}, &domain.Error{Code: domain.ErrNotFound, Message: "reservation was not found"}
	}
	if current.Revision != input.ExpectedRevision {
		return domain.LiquidityReservation{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "reservation revision does not match"}
	}
	current.Label = input.Label
	current.Amount = amount
	current.UpdatedAt = now
	current.Revision++
	if err := current.Validate(); err != nil {
		return domain.LiquidityReservation{}, err
	}
	if err := s.repository.SaveLiquidityReservation(ctx, *current); err != nil {
		return domain.LiquidityReservation{}, err
	}
	return *current, nil
}

func (s *Service) ReleaseLiquidityReservation(ctx context.Context, id domain.LiquidityReservationID, expectedRevision int) (domain.LiquidityReservation, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	defer unlock()
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	reservations, err := s.repository.ListLiquidityReservations(ctx, household.ID, true)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	var current *domain.LiquidityReservation
	for index := range reservations {
		if reservations[index].ID == id {
			current = &reservations[index]
			break
		}
	}
	if current == nil {
		return domain.LiquidityReservation{}, &domain.Error{Code: domain.ErrNotFound, Message: "reservation was not found"}
	}
	if current.Revision != expectedRevision {
		return domain.LiquidityReservation{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "reservation revision does not match"}
	}
	now := s.clock()
	current.ReleasedAt = &now
	current.UpdatedAt = now
	current.Revision++
	if err := s.repository.SaveLiquidityReservation(ctx, *current); err != nil {
		return domain.LiquidityReservation{}, err
	}
	return *current, nil
}

func (s *Service) UpdateProductTerms(ctx context.Context, input UpdateProductTermsInput) (ProductDetail, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	defer unlock()
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	contract, err := s.repository.Product(ctx, household.ID, input.ProductID)
	if err != nil {
		return ProductDetail{}, err
	}
	if contract.Revision != input.ExpectedRevision {
		return ProductDetail{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "product revision does not match"}
	}
	if contract.State != domain.ProductStateOpen && input.Terms.Name == "" {
		return ProductDetail{}, &domain.Error{Code: domain.ErrConflict, Field: "productId", Message: "settled contracts only accept name and note edits"}
	}
	kind, mode, rate, maturityInterest, paidThrough, err := resolveProductTerms(input.Terms, contract.Currency, contract.Principal)
	if err != nil {
		return ProductDetail{}, err
	}
	now := s.clock()
	contract.Kind = kind
	if input.Terms.Name != "" {
		contract.Name = input.Terms.Name
	}
	contract.Note = input.Terms.Note
	if contract.State == domain.ProductStateOpen {
		contract.MaturityOn = input.Terms.MaturityOn
		contract.InterestMode = mode
		contract.AnnualRate = rate
		contract.MaturityInterest = maturityInterest
		contract.InterestPaidThroughOn = paidThrough
	}
	contract.Revision++
	contract.UpdatedAt = now
	if err := contract.Validate(); err != nil {
		return ProductDetail{}, err
	}
	policy, err := contractPolicyForOpen(household.ID, contract.AccountID, contract.HoldingID, contract.Kind, contract.MaturityOn, input.Policy, contract.Currency, now)
	if err != nil {
		return ProductDetail{}, err
	}
	existing, err := s.policyBySource(ctx, household.ID, domain.HoldingSourceRef(contract.AccountID, contract.HoldingID))
	if err != nil {
		return ProductDetail{}, err
	}
	if existing != nil {
		policy.ID = existing.ID
		policy.CreatedAt = existing.CreatedAt
		policy.Revision = existing.Revision + 1
	}
	if err := s.repository.SaveProductContract(ctx, contract); err != nil {
		return ProductDetail{}, err
	}
	if err := s.repository.SaveLiquidityPolicy(ctx, policy); err != nil {
		return ProductDetail{}, err
	}
	return s.productDetail(ctx, household.ID, contract)
}

func (s *Service) AppendProductValuation(ctx context.Context, input AppendProductValuationInput) (ProductDetail, error) {
	command := ProductCommand{Kind: domain.ProductOpValueObservation}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	defer unlock()
	_ = command
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	operationID, err := domain.ParseProductOperationID(strings.TrimSpace(input.MutationID))
	if err != nil {
		return ProductDetail{}, &domain.Error{Code: domain.ErrValidation, Field: "mutationId", Message: "must be a lowercase UUID"}
	}
	if stored, err := s.repository.LookupProductOperation(ctx, household.ID, operationID); err != nil {
		return ProductDetail{}, err
	} else if stored != nil {
		return s.Product(ctx, input.ProductID)
	}
	contract, err := s.repository.Product(ctx, household.ID, input.ProductID)
	if err != nil {
		return ProductDetail{}, err
	}
	if contract.Kind != domain.ProductLockedProduct {
		return ProductDetail{}, &domain.Error{Code: domain.ErrManagedPosition, Field: "productId", Message: "term-deposit quotes cannot include projected interest"}
	}
	if contract.State != domain.ProductStateOpen {
		return ProductDetail{}, &domain.Error{Code: domain.ErrConflict, Field: "productId", Message: "only an open locked product can be revalued"}
	}
	origin, err := s.repository.HistoryOrigin(ctx, household.ID)
	if err != nil {
		return ProductDetail{}, err
	}
	observedAt, err := parseEffectiveAt(input.ObservedAt, origin, s.clock())
	if err != nil {
		return ProductDetail{}, err
	}
	amount, err := parseRequiredMoney("amount", input.Amount, contract.Currency)
	if err != nil {
		return ProductDetail{}, err
	}
	if !amount.Amount().IsPositive() {
		return ProductDetail{}, &domain.Error{Code: domain.ErrValidation, Field: "amount", Message: "must be greater than zero"}
	}
	instrument, err := s.repository.Instrument(ctx, household.ID, contract.InstrumentID)
	if err != nil {
		return ProductDetail{}, err
	}
	quote, err := buildManagedQuote(instrument, amount, observedAt)
	if err != nil {
		return ProductDetail{}, err
	}
	now := s.clock()
	operation := domain.ProductOperation{
		ID: operationID, HouseholdID: household.ID, Kind: domain.ProductOpValueObservation,
		PayloadSHA256:  hashBytes([]byte(input.ProductID.String() + ":" + input.Amount + ":" + input.ObservedAt)),
		RequestVersion: domain.ProductRequestVersion, RequestJSON: `{"kind":"value_observation"}`, ResultJSON: `{"quoteId":"` + quote.ID.String() + `"}`,
		EffectiveAt: observedAt, CreatedAt: now,
	}
	if err := s.repository.CommitProductBundle(ctx, domain.ProductBundle{
		AsOf: now, Operation: operation, Quotes: []domain.InstrumentQuote{quote},
		ProductLinks: []domain.ProductOperationProduct{{OperationID: operationID, ProductID: contract.ID, Role: domain.ProductRoleValued}},
	}); err != nil {
		return ProductDetail{}, err
	}
	s.invalidateAnalysis()
	return s.productDetail(ctx, household.ID, contract)
}

func (s *Service) policyBySource(ctx context.Context, householdID domain.HouseholdID, source domain.LiquiditySourceRef) (*domain.LiquidityPolicy, error) {
	policies, err := s.repository.ListLiquidityPolicies(ctx, householdID)
	if err != nil {
		return nil, err
	}
	key := source.Key()
	for index := range policies {
		if policies[index].Source.Key() == key {
			return &policies[index], nil
		}
	}
	return nil, nil
}
