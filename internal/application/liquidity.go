package application

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) LiquidityOverview(ctx context.Context, query LiquidityOverviewQuery) (domain.LiquidityOverview, error) {
	overview, _, _, err := s.prepareLiquidity(ctx, query, s.clock())
	return overview, err
}

// prepareLiquidity captures one repository snapshot and uses request-local valuation
// configuration. Neither the shared clock nor its provider/TTL callbacks are mutated.
func (s *Service) prepareLiquidity(ctx context.Context, query LiquidityOverviewQuery, asOf time.Time) (domain.LiquidityOverview, domain.LiquiditySnapshot, []domain.LiquiditySource, error) {
	snapshot, err := s.repository.ReadLiquiditySnapshot(ctx)
	if err != nil {
		return domain.LiquidityOverview{}, snapshot, nil, err
	}
	if snapshot.Portfolio.Household == nil {
		return domain.LiquidityOverview{}, snapshot, nil, onboardingRequired()
	}
	valuation := *s.valuation
	valuation.now = func() time.Time { return asOf }
	valuation.readContext = ctx
	provider, ttl := valuation.providerKey(), valuation.quoteTTL()
	valuation.fxProviderKey = func() string { return provider }
	valuation.quoteCacheTTL = func() time.Duration { return ttl }
	sources, err := s.liquiditySourcesWithValuation(snapshot, &valuation)
	if err != nil {
		return domain.LiquidityOverview{}, snapshot, nil, err
	}
	convert := func(amount decimal.Decimal, currency domain.CurrencyCode) (*decimal.Decimal, bool, error) {
		return valuation.ConvertExactAmount(snapshot.Portfolio, amount, currency)
	}
	overview, err := domain.EvaluateLiquidity(domain.LiquidityQuery{
		AsOf: asOf, Timezone: timezoneOrUTC(snapshot.Portfolio.Origin), BaseCurrency: snapshot.Portfolio.Household.BaseCurrency,
		CustomHorizonOn: query.CustomHorizonOn, IncludeEarlyWithdrawal: query.IncludeEarlyWithdrawal,
	}, sources, snapshot.Reservations, convert)
	return overview, snapshot, sources, err
}

func (v *ValuationService) ConvertAmount(snapshot domain.PortfolioSnapshot, amount domain.Money) (*decimal.Decimal, bool, error) {
	return v.ConvertExactAmount(snapshot, amount.Amount(), amount.Currency())
}

func (v *ValuationService) ConvertExactAmount(snapshot domain.PortfolioSnapshot, amount decimal.Decimal, currency domain.CurrencyCode) (*decimal.Decimal, bool, error) {
	if snapshot.Household == nil {
		return nil, false, nil
	}
	converted, _, missing, err := v.convert(snapshot, "", amount, currency)
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
	return s.liquiditySourcesWithValuation(snapshot, s.valuation)
}

func (s *Service) liquiditySourcesWithValuation(snapshot domain.LiquiditySnapshot, valuation *ValuationService) ([]domain.LiquiditySource, error) {
	valued, _, err := valuation.ValueAccounts(snapshot.Portfolio)
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
	if component.BaseAmountExact != "" {
		value, err := decimal.NewFromString(component.BaseAmountExact)
		if err != nil {
			return domain.LiquiditySource{}, err
		}
		source.CurrentBaseValue = &value
	}
	if component.NativeAmount != "" {
		money, err := displayNativeValuation(component.NativeAmount, component.NativeCurrency)
		if err != nil {
			return domain.LiquiditySource{}, err
		}
		source.CurrentNativeValue = &money
		source.CurrentNativeAmount = component.NativeAmount
	}
	if component.HoldingID != nil {
		source.Ref = domain.HoldingSourceRef(account.Account.ID, *component.HoldingID)
		if holding, ok := holdings[*component.HoldingID]; ok {
			source.QuantityZero = holding.Quantity.IsZero()
			if instrument, ok := instruments[holding.InstrumentID]; ok {
				source.DisplayName = domain.InstrumentDisplayLabel(instrument.Name, pointerText(instrument.Symbol), instrument.ShortName)
				kind := instrument.Type
				source.InstrumentType = &kind
			}
		}
		if contract, ok := contracts[*component.HoldingID]; ok {
			copy := contract
			source.Managed = true
			source.Contract = &copy
			source.ProductID = &copy.ID
			source.DisplayName = domain.InstrumentDisplayLabel(contract.Name, component.InstrumentSymbol, contract.ShortName)
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
	snapshot, err := s.productDetailSnapshot(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	for _, contract := range snapshot.Contracts {
		if contract.ID == productID {
			materialized, err := s.materializeProductDetails(ctx, snapshot, s.clock())
			if err != nil {
				return ProductDetail{}, err
			}
			return materialized.detail(contract)
		}
	}
	return ProductDetail{}, &domain.Error{Code: domain.ErrNotFound, Message: "product was not found"}
}

func (s *Service) ListProducts(ctx context.Context, accountID *domain.AccountID, includeClosed bool) ([]ProductDetail, error) {
	snapshot, err := s.productDetailSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	contracts := make([]domain.ProductContract, 0, len(snapshot.Contracts))
	for _, contract := range snapshot.Contracts {
		if accountID != nil && contract.AccountID != *accountID {
			continue
		}
		if !includeClosed && contract.State != domain.ProductStateOpen {
			continue
		}
		contracts = append(contracts, contract)
	}
	// Preserve the repository list's public ordering; the snapshot is ordered for replay.
	sort.Slice(contracts, func(i, j int) bool {
		a, b := contracts[i], contracts[j]
		if a.StartOn != b.StartOn {
			return a.StartOn > b.StartOn
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ID.String() > b.ID.String()
	})
	details := make([]ProductDetail, 0, len(contracts))
	if len(contracts) == 0 {
		return details, nil
	}
	materialized, err := s.materializeProductDetails(ctx, snapshot, s.clock())
	if err != nil {
		return nil, err
	}
	for _, contract := range contracts {
		detail, err := materialized.detail(contract)
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
}

func (s *Service) productDetailSnapshot(ctx context.Context) (domain.LiquiditySnapshot, error) {
	snapshot, err := s.repository.ReadLiquiditySnapshot(ctx)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Portfolio.Household == nil {
		return snapshot, onboardingRequired()
	}
	return snapshot, nil
}

func (s *Service) ListProductOperations(ctx context.Context, productID domain.ProductContractID, cursor string, limit int) (ProductOperationPage, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return ProductOperationPage{}, err
	}
	defer unlock()

	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	operations, err := s.repository.ReadProductOperationHistorySnapshot(ctx, productID, limit+1, cursor)
	if err != nil {
		return ProductOperationPage{}, err
	}
	page := ProductOperationPage{Operations: operations.Rows, LatestRecordedOperationID: operations.LatestRecordedOperationID}
	page.Undo, err = s.productUndoAvailability(operations)
	if err != nil {
		return ProductOperationPage{}, err
	}
	if len(operations.Rows) > limit {
		page.Operations = operations.Rows[:limit]
		next := encodeProductOperationCursor(operations.Rows[limit-1].ProductOperation)
		page.Next = &next
	}
	return page, nil
}

func encodeProductOperationCursor(operation domain.ProductOperation) string {
	return operation.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + operation.ID.String()
}

func (s *Service) productDetail(ctx context.Context, householdID domain.HouseholdID, contract domain.ProductContract) (ProductDetail, error) {
	snapshot, err := s.productDetailSnapshot(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	if snapshot.Portfolio.Household.ID != householdID {
		return ProductDetail{}, &domain.Error{Code: domain.ErrNotFound, Message: "product was not found"}
	}
	materialized, err := s.materializeProductDetails(ctx, snapshot, s.clock())
	if err != nil {
		return ProductDetail{}, err
	}
	return materialized.detail(contract)
}

// productDetailMaterialization lives only for one response. All facts come from
// the same repository transaction, and valuation runs once with fixed read settings.
type productDetailMaterialization struct {
	policies     map[string]domain.LiquidityPolicy
	reservations map[string][]domain.LiquidityReservation
	components   map[domain.HoldingID]domain.ValuationComponent
	symbols      map[domain.InstrumentID]string
	localDate    string
}

func (s *Service) materializeProductDetails(ctx context.Context, snapshot domain.LiquiditySnapshot, asOf time.Time) (productDetailMaterialization, error) {
	result := productDetailMaterialization{
		policies:     make(map[string]domain.LiquidityPolicy, len(snapshot.Policies)),
		reservations: make(map[string][]domain.LiquidityReservation),
		components:   make(map[domain.HoldingID]domain.ValuationComponent, len(snapshot.Portfolio.Holdings)),
		symbols:      make(map[domain.InstrumentID]string, len(snapshot.Portfolio.Instruments)),
	}
	valuation := *s.valuation
	valuation.now = func() time.Time { return asOf }
	valuation.readContext = ctx
	provider, ttl := valuation.providerKey(), valuation.quoteTTL()
	valuation.fxProviderKey = func() string { return provider }
	valuation.quoteCacheTTL = func() time.Duration { return ttl }
	valued, _, err := valuation.ValueAccounts(snapshot.Portfolio)
	if err != nil {
		return result, err
	}
	for _, account := range valued {
		for _, component := range account.Components {
			if component.HoldingID != nil && component.NativeAmount != "" {
				result.components[*component.HoldingID] = component
			}
		}
	}
	for _, policy := range snapshot.Policies {
		key := policy.Source.Key()
		if _, exists := result.policies[key]; !exists {
			result.policies[key] = policy
		}
	}
	for _, reservation := range snapshot.Reservations {
		if reservation.Active() {
			key := reservation.Source.Key()
			result.reservations[key] = append(result.reservations[key], reservation)
		}
	}
	for _, instrument := range snapshot.Portfolio.Instruments {
		if _, exists := result.symbols[instrument.ID]; !exists {
			result.symbols[instrument.ID] = pointerText(instrument.Symbol)
		}
	}
	result.localDate, _, err = domain.LocalCivilDate(asOf, timezoneOrUTC(snapshot.Portfolio.Origin))
	return result, err
}

func (m productDetailMaterialization) detail(contract domain.ProductContract) (ProductDetail, error) {
	key := domain.HoldingSourceRef(contract.AccountID, contract.HoldingID).Key()
	policy := m.policies[key]
	active := m.reservations[key]
	if active == nil {
		active = []domain.LiquidityReservation{}
	}
	var current *domain.Money
	if component, ok := m.components[contract.HoldingID]; ok {
		money, err := displayNativeValuation(component.NativeAmount, component.NativeCurrency)
		if err != nil {
			return ProductDetail{}, err
		}
		current = &money
	}
	display, err := domain.ProductAvailabilityState(contract, policy, m.localDate)
	if err != nil {
		return ProductDetail{}, err
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
		Symbol: m.symbols[contract.InstrumentID], Contract: contract, Policy: policy, CurrentValue: current, DisplayState: display,
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
	policy, err := s.prepareLiquidityPolicy(ctx, input)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if err := s.repository.SaveLiquidityPolicy(ctx, policy); err != nil {
		return domain.LiquidityPolicy{}, err
	}
	return policy, nil
}

// prepareLiquidityPolicy is shared by GUI save and MCP preview. It has no writes.
func (s *Service) prepareLiquidityPolicy(ctx context.Context, input SavePolicyInput) (domain.LiquidityPolicy, error) {
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
	currency, err := s.liquiditySourceCurrency(ctx, household.ID, input.Source)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	policy, err := policyFromInput(household.ID, input.Source, input.Policy, currency, false, s.clock())
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if existing == nil {
		if input.ExpectedRevision != 0 {
			return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "policy does not exist"}
		}
		return policy, nil
	}
	if input.ExpectedRevision != existing.Revision {
		return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "policy revision does not match"}
	}
	policy.ID = existing.ID
	policy.CreatedAt = existing.CreatedAt
	policy.Revision = existing.Revision + 1
	return policy, nil
}

func (s *Service) ResetLiquidityPolicy(ctx context.Context, source domain.LiquiditySourceRef, expectedRevision int) error {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	existing, err := s.prepareLiquidityPolicyReset(ctx, source, expectedRevision)
	if err != nil {
		return err
	}
	return s.repository.DeleteLiquidityPolicy(ctx, existing.ID, expectedRevision)
}

func (s *Service) prepareLiquidityPolicyReset(ctx context.Context, source domain.LiquiditySourceRef, expectedRevision int) (domain.LiquidityPolicy, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if source.HoldingID != nil {
		if product, err := s.repository.ProductByHolding(ctx, *source.HoldingID); err != nil {
			return domain.LiquidityPolicy{}, err
		} else if product != nil {
			return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrManagedPosition, Field: "source", Message: "managed product policy cannot be reset"}
		}
	}
	existing, err := s.policyBySource(ctx, household.ID, source)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if existing == nil {
		return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrNotFound, Field: "source", Message: "policy was not found"}
	}
	if expectedRevision != existing.Revision {
		return domain.LiquidityPolicy{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "policy revision does not match"}
	}
	return *existing, nil
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
	currency, err := s.liquiditySourceCurrency(ctx, household.ID, input.Source)
	if err != nil {
		return domain.LiquidityReservation{}, err
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
	if input.ReconciliationLink != nil {
		return ProductDetail{}, reconciliationError("reconciliationLink", "linked terms require the guarded preview and commit workflow")
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	defer unlock()
	prepared, err := s.prepareProductTerms(ctx, input, s.clock())
	if err != nil {
		return ProductDetail{}, err
	}
	if err := s.repository.SaveProductTerms(ctx, prepared.contract, prepared.policy, prepared.expectedPolicyRevision); err != nil {
		return ProductDetail{}, err
	}
	s.invalidateAnalysis()
	return s.productDetail(ctx, prepared.contract.HouseholdID, prepared.contract)
}

type preparedProductTerms struct {
	before                 domain.ProductContract
	contract               domain.ProductContract
	policy                 domain.LiquidityPolicy
	expectedPolicyRevision int
}

// prepareProductTerms is pure with respect to persisted facts; GUI and stored plans share every validation.
func (s *Service) prepareProductTerms(ctx context.Context, input UpdateProductTermsInput, now time.Time) (preparedProductTerms, error) {
	// Match the authoritative SQLite/IPC UTC millisecond representation before
	// constructing either the preview or the durable terms receipt.
	now = normalizeNow(now)
	terms, err := normalizeProductTermDates(input.Terms)
	if err != nil {
		return preparedProductTerms{}, err
	}
	input.Terms = terms
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return preparedProductTerms{}, err
	}
	contract, err := s.repository.Product(ctx, household.ID, input.ProductID)
	if err != nil {
		return preparedProductTerms{}, err
	}
	if err := contract.NormalizeCivilDates(); err != nil {
		return preparedProductTerms{}, err
	}
	if contract.Revision != input.ExpectedRevision {
		return preparedProductTerms{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "expectedRevision", Message: "product revision does not match"}
	}
	before := contract
	if input.Terms.ShortName != nil {
		contract.ShortName, err = domain.NormalizeInstrumentShortName(*input.Terms.ShortName)
		if err != nil {
			return preparedProductTerms{}, err
		}
	}
	if contract.State != domain.ProductStateOpen && input.Terms.Name == "" {
		return preparedProductTerms{}, &domain.Error{Code: domain.ErrConflict, Field: "productId", Message: "settled contracts only accept name and note edits"}
	}
	kind, mode, rate, maturityInterest, paidThrough, err := resolveProductTerms(terms, contract.Currency, contract.Principal)
	if err != nil {
		return preparedProductTerms{}, err
	}
	if kind != contract.Kind || terms.StartOn != contract.StartOn {
		return preparedProductTerms{}, &domain.Error{Code: domain.ErrConflict, Field: "terms", Message: "product kind and start date are immutable"}
	}
	if input.Terms.Name != "" {
		contract.Name = input.Terms.Name
	}
	contract.Note = input.Terms.Note
	if contract.State == domain.ProductStateOpen {
		contract.MaturityOn = terms.MaturityOn
		contract.InterestMode = mode
		contract.AnnualRate = rate
		contract.MaturityInterest = maturityInterest
		contract.InterestPaidThroughOn = paidThrough
	}
	contract.Revision++
	contract.UpdatedAt = now
	if err := contract.NormalizeCivilDates(); err != nil {
		return preparedProductTerms{}, err
	}
	if err := contract.Validate(); err != nil {
		return preparedProductTerms{}, err
	}
	policy, err := contractPolicyForOpen(household.ID, contract.AccountID, contract.HoldingID, contract.Kind, contract.MaturityOn, input.Policy, contract.Currency, now)
	if err != nil {
		return preparedProductTerms{}, err
	}
	existing, err := s.policyBySource(ctx, household.ID, domain.HoldingSourceRef(contract.AccountID, contract.HoldingID))
	if err != nil {
		return preparedProductTerms{}, err
	}
	if existing != nil {
		policy.ID = existing.ID
		policy.CreatedAt = existing.CreatedAt
		policy.Revision = existing.Revision + 1
	}
	expectedPolicyRevision := 0
	if existing != nil {
		expectedPolicyRevision = existing.Revision
	}
	if before.State != domain.ProductStateOpen {
		if existing == nil {
			return preparedProductTerms{}, &domain.Error{Code: domain.ErrConflict, Message: "closed product policy is missing"}
		}
		candidate := before
		candidate.MaturityOn = terms.MaturityOn
		candidate.InterestMode = mode
		candidate.AnnualRate = rate
		if rate != nil && before.AnnualRate != nil && rate.Canonical() == before.AnnualRate.Canonical() {
			candidate.AnnualRate = before.AnnualRate
		}
		candidate.MaturityInterest = maturityInterest
		if sameMoneyValue(maturityInterest, before.MaturityInterest) {
			candidate.MaturityInterest = before.MaturityInterest
		}
		candidate.InterestPaidThroughOn = paidThrough
		oldPolicy := *existing
		policy.UpdatedAt = oldPolicy.UpdatedAt
		policy.Revision = oldPolicy.Revision
		policy.ConfirmedAt = oldPolicy.ConfirmedAt
		if sameMoneyValue(policy.NormalExitFee, oldPolicy.NormalExitFee) {
			policy.NormalExitFee = oldPolicy.NormalExitFee
		}
		if sameMoneyValue(policy.EarlyFee, oldPolicy.EarlyFee) {
			policy.EarlyFee = oldPolicy.EarlyFee
		}
		if sameMoneyValue(policy.EarlyGrossAmount, oldPolicy.EarlyGrossAmount) {
			policy.EarlyGrossAmount = oldPolicy.EarlyGrossAmount
		}
		if sameMoneyValue(policy.AccessibleAmountCap, oldPolicy.AccessibleAmountCap) {
			policy.AccessibleAmountCap = oldPolicy.AccessibleAmountCap
		}
		if !reflect.DeepEqual(candidate, before) || !reflect.DeepEqual(policy, oldPolicy) {
			return preparedProductTerms{}, &domain.Error{Code: domain.ErrConflict, Field: "terms", Message: "closed contracts only accept name and note edits"}
		}
		policy = oldPolicy
	}
	return preparedProductTerms{before: before, contract: contract, policy: policy, expectedPolicyRevision: expectedPolicyRevision}, nil
}

func (s *Service) AppendProductValuation(ctx context.Context, input AppendProductValuationInput) (ProductDetail, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return ProductDetail{}, err
	}
	defer unlock()
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
		expected := hashBytes([]byte(input.ProductID.String() + ":" + input.Amount + ":" + input.ObservedAt))
		if stored.Kind != domain.ProductOpValueObservation || stored.PayloadSHA256 != expected {
			return ProductDetail{}, &domain.Error{Code: domain.ErrConflict, Field: "mutationId", Message: "this mutation ID was already used with a different command"}
		}
		return s.Product(ctx, input.ProductID)
	}
	prepared, err := s.prepareProductValuation(ctx, input, s.clock())
	if err != nil {
		return ProductDetail{}, err
	}
	contract, quote, observedAt := prepared.contract, prepared.quote, prepared.observedAt
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

func (s *Service) liquiditySourceCurrency(ctx context.Context, householdID domain.HouseholdID, source domain.LiquiditySourceRef) (domain.CurrencyCode, error) {
	switch source.Kind {
	case domain.SourceAccountCash:
		if source.Currency == nil {
			return "", &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "is required for an account cash source"}
		}
		return *source.Currency, nil
	case domain.SourceAccountValue:
		record, err := s.repository.AccountRecord(ctx, householdID, source.AccountID)
		if err != nil {
			return "", err
		}
		return record.Account.DefaultCurrency, nil
	case domain.SourceHolding:
		if source.HoldingID == nil {
			return "", &domain.Error{Code: domain.ErrValidation, Field: "holdingId", Message: "is required for a holding source"}
		}
		holding, err := s.repository.Holding(ctx, *source.HoldingID)
		if err != nil {
			return "", err
		}
		if holding.AccountID != source.AccountID {
			return "", &domain.Error{Code: domain.ErrValidation, Field: "accountId", Message: "does not own this holding"}
		}
		instrument, err := s.repository.Instrument(ctx, householdID, holding.InstrumentID)
		if err != nil {
			return "", err
		}
		return instrument.QuoteCurrency, nil
	default:
		return "", &domain.Error{Code: domain.ErrValidation, Field: "sourceKind", Message: "is not a supported liquidity source"}
	}
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

func (s *Service) ListLiquidityReservations(ctx context.Context) ([]domain.LiquidityReservation, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	return s.repository.ListLiquidityReservations(ctx, household.ID, true)
}

func sameMoneyValue(a, b *domain.Money) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Currency() == b.Currency() && a.Amount().Equal(b.Amount())
}

// Native valuations are quantity × price (up to sixteen fractional digits).
// Money here is a display projection; liquidity sources retain the exact input.
func displayNativeValuation(raw string, currency domain.CurrencyCode) (domain.Money, error) {
	canonical, err := domain.ParseNativeAmount(raw)
	if err != nil {
		return domain.Money{}, err
	}
	amount, err := decimal.NewFromString(canonical)
	if err != nil {
		return domain.Money{}, err
	}
	return domain.NewMoney(amount, currency)
}

type preparedProductValuation struct {
	contract   domain.ProductContract
	quote      domain.InstrumentQuote
	amount     domain.Money
	observedAt time.Time
}

func (s *Service) prepareProductValuation(ctx context.Context, input AppendProductValuationInput, now time.Time) (preparedProductValuation, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return preparedProductValuation{}, err
	}
	contract, err := s.repository.Product(ctx, household.ID, input.ProductID)
	if err != nil {
		return preparedProductValuation{}, err
	}
	if contract.Kind != domain.ProductLockedProduct {
		return preparedProductValuation{}, &domain.Error{Code: domain.ErrManagedPosition, Field: "productId", Message: "term-deposit quotes cannot include projected interest"}
	}
	if contract.State != domain.ProductStateOpen {
		return preparedProductValuation{}, &domain.Error{Code: domain.ErrConflict, Field: "productId", Message: "only an open locked product can be revalued"}
	}
	origin, err := s.repository.HistoryOrigin(ctx, household.ID)
	if err != nil {
		return preparedProductValuation{}, err
	}
	observedAt, err := parseEffectiveAt(input.ObservedAt, origin, now)
	if err != nil {
		return preparedProductValuation{}, err
	}
	// Validate the supplied instant before truncating: a future instant within
	// the same millisecond must not become valid through normalization.
	observedAt = normalizeNow(observedAt)
	amount, err := parseRequiredMoney("amount", input.Amount, contract.Currency)
	if err != nil {
		return preparedProductValuation{}, err
	}
	if !amount.Amount().IsPositive() {
		return preparedProductValuation{}, &domain.Error{Code: domain.ErrValidation, Field: "amount", Message: "must be greater than zero"}
	}
	instrument, err := s.repository.Instrument(ctx, household.ID, contract.InstrumentID)
	if err != nil {
		return preparedProductValuation{}, err
	}
	quote, err := buildManagedQuote(instrument, amount, observedAt)
	if err != nil {
		return preparedProductValuation{}, err
	}
	quote.CreatedAt = normalizeNow(now)
	return preparedProductValuation{contract: contract, quote: quote, amount: amount, observedAt: observedAt}, nil
}
