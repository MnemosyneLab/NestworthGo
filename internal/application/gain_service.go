package application

import (
	"context"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// GainService builds cost and gain read models from immutable activity facts.
// It deliberately has no write or network path; current quote and FX
// selection remains owned by ValuationService.
type GainService struct {
	repository Repository
	valuation  *ValuationService
	now        func() time.Time
}

func NewGainService(repository Repository, clocks ...func() time.Time) *GainService {
	now := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		now = clocks[0]
	}
	return &GainService{repository: repository, valuation: NewValuationService(repository, now), now: now}
}

func (g *GainService) SetFXProviderKey(providerKey func() string) {
	g.valuation.SetFXProviderKey(providerKey)
}

func (g *GainService) HoldingGain(ctx context.Context, holdingID domain.HoldingID) (domain.HoldingGainView, error) {
	snapshot, err := g.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	if snapshot.Household == nil {
		return domain.HoldingGainView{}, &domain.Error{Code: domain.ErrNotFound, Message: "holding was not found"}
	}
	holding, instrument, err := gainHolding(snapshot, holdingID)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	fxQuotes, origin, err := g.gainFXInputs(ctx, snapshot)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	replay := newCostBasisReplayContext(g.repository, snapshot.Holdings)
	return g.holdingGain(ctx, snapshot, holding, instrument, replay, fxQuotes, origin)
}

func (g *GainService) AccountGain(ctx context.Context, accountID domain.AccountID) (domain.AccountGainView, error) {
	snapshot, err := g.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.AccountGainView{}, err
	}
	if snapshot.Household == nil {
		return domain.AccountGainView{}, &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
	}
	accountExists := false
	for _, record := range snapshot.Accounts {
		if record.Account.ID == accountID {
			accountExists = true
			break
		}
	}
	if !accountExists {
		return domain.AccountGainView{}, &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
	}
	fxQuotes, origin, err := g.gainFXInputs(ctx, snapshot)
	if err != nil {
		return domain.AccountGainView{}, err
	}
	replay := newCostBasisReplayContext(g.repository, snapshot.Holdings)
	result := domain.AccountGainView{AccountID: accountID, Holdings: make([]domain.HoldingGainView, 0), Available: true}
	for _, holding := range snapshot.Holdings {
		if holding.AccountID != accountID || holding.ArchivedAt != nil {
			continue
		}
		instrument, ok := gainInstrument(snapshot.Instruments, holding.InstrumentID)
		if !ok {
			continue
		}
		view, viewErr := g.holdingGain(ctx, snapshot, holding, instrument, replay, fxQuotes, origin)
		if viewErr != nil {
			return domain.AccountGainView{}, viewErr
		}
		result.Holdings = append(result.Holdings, view)
		if !view.Available {
			result.Available = false
			if result.MissingReason == "" {
				result.MissingReason = view.MissingReason
			}
		}
	}
	sort.Slice(result.Holdings, func(i, j int) bool {
		return result.Holdings[i].HoldingID.String() < result.Holdings[j].HoldingID.String()
	})
	return g.accountTotals(snapshot, result)
}

// RealizedGainInRange returns realized gains in the inclusive local-date
// range. Each sale is converted with the FX observation available at that
// sale's own effective time; a missing observation affects only its groups.
func (g *GainService) RealizedGainInRange(ctx context.Context, scope domain.GainScope, from, to domain.LocalDate) (domain.RealizedGainView, error) {
	fromTime, err := time.Parse("2006-01-02", from)
	if err != nil {
		return domain.RealizedGainView{}, &domain.Error{Code: domain.ErrValidation, Field: "from", Message: "from date must use YYYY-MM-DD"}
	}
	toTime, err := time.Parse("2006-01-02", to)
	if err != nil {
		return domain.RealizedGainView{}, &domain.Error{Code: domain.ErrValidation, Field: "to", Message: "to date must use YYYY-MM-DD"}
	}
	if fromTime.After(toTime) {
		return domain.RealizedGainView{}, &domain.Error{Code: domain.ErrValidation, Field: "range", Message: "from date must not be after to date"}
	}
	snapshot, err := g.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	if snapshot.Household == nil {
		return domain.RealizedGainView{From: from, To: to, Available: true}, nil
	}
	fxQuotes, origin, err := g.gainFXInputs(ctx, snapshot)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	location := time.UTC
	if origin != nil {
		location, err = time.LoadLocation(origin.Timezone)
		if err != nil {
			return domain.RealizedGainView{}, err
		}
	}
	replay := newCostBasisReplayContext(g.repository, snapshot.Holdings)
	accounts := make(map[domain.AccountID]string, len(snapshot.Accounts))
	for _, record := range snapshot.Accounts {
		accounts[record.Account.ID] = record.Account.Name
	}
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	byInstrument := make(map[domain.InstrumentID]*gainGroupAccumulator)
	byAccount := make(map[domain.AccountID]*gainGroupAccumulator)
	result := domain.RealizedGainView{From: from, To: to, Currency: snapshot.Household.BaseCurrency, Available: true}
	for _, holding := range snapshot.Holdings {
		if (scope.AccountID != nil && holding.AccountID != *scope.AccountID) || (scope.InstrumentID != nil && holding.InstrumentID != *scope.InstrumentID) {
			continue
		}
		instrument, ok := instruments[holding.InstrumentID]
		if !ok {
			continue
		}
		replayed, replayErr := replay.replay(ctx, holding.ID, nil)
		if replayErr != nil {
			return domain.RealizedGainView{}, replayErr
		}
		for _, event := range replayed.Realized {
			localDate := event.EffectiveAt.In(location).Format("2006-01-02")
			if localDate < from || localDate > to {
				continue
			}
			instrumentGroup := groupForInstrument(byInstrument, instrument.ID, instrument.Name)
			accountGroup := groupForAccount(byAccount, holding.AccountID, accounts[holding.AccountID])
			rate, rateAvailable := gainFXRateAtOrBefore(snapshot, fxQuotes, event.RealizedGain.Currency(), event.EffectiveAt)
			if !rateAvailable {
				result.Available = false
				if result.MissingReason == "" {
					result.MissingReason = "realized gain foreign-exchange rate is unavailable"
				}
				instrumentGroup.Available = false
				instrumentGroup.MissingReason = "realized gain foreign-exchange rate is unavailable"
				accountGroup.Available = false
				accountGroup.MissingReason = "realized gain foreign-exchange rate is unavailable"
				continue
			}
			converted := event.RealizedGain.Amount().Mul(rate)
			instrumentGroup.Total = instrumentGroup.Total.Add(converted)
			accountGroup.Total = accountGroup.Total.Add(converted)
		}
	}
	result.ByInstrument, err = finishGainGroups(byInstrument, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	result.ByAccount, err = finishGainGroups(byAccount, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	result.Total, err = totalFromGainGroups(result.ByInstrument, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	return result, nil
}

// RealizedGain resolves the existing Analytics range selector to an
// inclusive local-date range using the service clock and the history
// timezone, then delegates to the explicit range reader.
func (g *GainService) RealizedGain(ctx context.Context, scope domain.GainScope, trendRange domain.TrendRange) (domain.RealizedGainView, error) {
	from, to, err := g.trendRangeBounds(ctx, trendRange)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	if from == "" {
		return domain.RealizedGainView{Available: true}, nil
	}
	return g.RealizedGainInRange(ctx, scope, from, to)
}

const dividendIncomeMissingFX = "dividend income foreign-exchange rate is unavailable"

// DividendIncomeInRange sums cash-dividend income in the inclusive local-date
// range. Amounts convert with the FX observation available at each activity's
// own effective time. Missing FX marks only the affected groups; dividends are
// never mixed into realized sell gain.
func (g *GainService) DividendIncomeInRange(ctx context.Context, scope domain.GainScope, from, to domain.LocalDate) (domain.DividendIncomeView, error) {
	fromTime, err := time.Parse("2006-01-02", from)
	if err != nil {
		return domain.DividendIncomeView{}, &domain.Error{Code: domain.ErrValidation, Field: "from", Message: "from date must use YYYY-MM-DD"}
	}
	toTime, err := time.Parse("2006-01-02", to)
	if err != nil {
		return domain.DividendIncomeView{}, &domain.Error{Code: domain.ErrValidation, Field: "to", Message: "to date must use YYYY-MM-DD"}
	}
	if fromTime.After(toTime) {
		return domain.DividendIncomeView{}, &domain.Error{Code: domain.ErrValidation, Field: "range", Message: "from date must not be after to date"}
	}
	snapshot, err := g.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	if snapshot.Household == nil {
		return domain.DividendIncomeView{From: from, To: to, Available: true}, nil
	}
	fxQuotes, _, err := g.gainFXInputs(ctx, snapshot)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	activities, err := g.listDividendActivities(ctx, snapshot.Household.ID, from, to)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	accounts := make(map[domain.AccountID]string, len(snapshot.Accounts))
	for _, record := range snapshot.Accounts {
		accounts[record.Account.ID] = record.Account.Name
	}
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	holdings := make(map[domain.HoldingID]domain.Holding, len(snapshot.Holdings))
	for _, holding := range snapshot.Holdings {
		holdings[holding.ID] = holding
	}
	byInstrument := make(map[domain.InstrumentID]*gainGroupAccumulator)
	byAccount := make(map[domain.AccountID]*gainGroupAccumulator)
	result := domain.DividendIncomeView{From: from, To: to, Currency: snapshot.Household.BaseCurrency, Available: true}
	for _, activity := range activities {
		detail := activity.DividendDetail
		if detail == nil {
			continue
		}
		if scope.InstrumentID != nil && detail.InstrumentID != *scope.InstrumentID {
			continue
		}
		accountID, ok := dividendAccountID(activity, holdings)
		if !ok {
			continue
		}
		if scope.AccountID != nil && accountID != *scope.AccountID {
			continue
		}
		instrumentLabel := detail.InstrumentID.String()
		if instrument, found := instruments[detail.InstrumentID]; found {
			instrumentLabel = instrument.Name
		}
		accountLabel := accountID.String()
		if name := accounts[accountID]; name != "" {
			accountLabel = name
		}
		instrumentGroup := groupForInstrument(byInstrument, detail.InstrumentID, instrumentLabel)
		accountGroup := groupForAccount(byAccount, accountID, accountLabel)
		rate, rateAvailable := gainFXRateAtOrBefore(snapshot, fxQuotes, detail.Amount.Currency(), activity.EffectiveAt)
		if !rateAvailable {
			result.Available = false
			if result.MissingReason == "" {
				result.MissingReason = dividendIncomeMissingFX
			}
			instrumentGroup.Available = false
			instrumentGroup.MissingReason = dividendIncomeMissingFX
			accountGroup.Available = false
			accountGroup.MissingReason = dividendIncomeMissingFX
			continue
		}
		converted := detail.Amount.Amount().Mul(rate)
		instrumentGroup.Total = instrumentGroup.Total.Add(converted)
		accountGroup.Total = accountGroup.Total.Add(converted)
	}
	result.ByInstrument, err = finishGainGroups(byInstrument, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	result.ByAccount, err = finishGainGroups(byAccount, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	result.Total, err = totalFromGainGroups(result.ByInstrument, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	return result, nil
}

// DividendIncome resolves an Analytics trend range, then delegates to the
// explicit dividend-income range reader.
func (g *GainService) DividendIncome(ctx context.Context, scope domain.GainScope, trendRange domain.TrendRange) (domain.DividendIncomeView, error) {
	from, to, err := g.trendRangeBounds(ctx, trendRange)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	if from == "" {
		return domain.DividendIncomeView{Available: true}, nil
	}
	return g.DividendIncomeInRange(ctx, scope, from, to)
}

func (g *GainService) trendRangeBounds(ctx context.Context, trendRange domain.TrendRange) (from, to domain.LocalDate, err error) {
	household, err := g.repository.Household(ctx)
	if err != nil {
		return "", "", err
	}
	if household == nil {
		return "", "", nil
	}
	origin, err := g.repository.HistoryOrigin(ctx, household.ID)
	if err != nil {
		return "", "", err
	}
	location := time.UTC
	if origin != nil {
		location, err = time.LoadLocation(origin.Timezone)
		if err != nil {
			return "", "", err
		}
	}
	now := g.now().In(location)
	today := now.Format("2006-01-02")
	from = today
	switch trendRange {
	case domain.Trend30Days:
		from = now.AddDate(0, 0, -29).Format("2006-01-02")
	case domain.TrendOneYear:
		from = now.AddDate(0, 0, -364).Format("2006-01-02")
	case domain.TrendAllTime:
		if origin != nil {
			from = origin.StartedAt.In(location).Format("2006-01-02")
		}
	default:
		return "", "", &domain.Error{Code: domain.ErrValidation, Field: "range", Message: "trend range is not supported"}
	}
	return from, today, nil
}

func (g *GainService) listDividendActivities(ctx context.Context, householdID domain.HouseholdID, from, to domain.LocalDate) ([]domain.Activity, error) {
	var activities []domain.Activity
	query := domain.ActivityQuery{
		Kinds:           []domain.ActivityKind{domain.ActivityCashDividend},
		FromLocalDate:   from,
		ToLocalDate:     to,
		ExcludeReversed: true,
		Limit:           100,
	}
	for {
		page, err := g.repository.ListActivityPage(ctx, householdID, query)
		if err != nil {
			return nil, err
		}
		activities = append(activities, page.Activities...)
		if !page.HasMore || page.Next == nil {
			break
		}
		query.After = page.Next
	}
	return activities, nil
}

func dividendAccountID(activity domain.Activity, holdings map[domain.HoldingID]domain.Holding) (domain.AccountID, bool) {
	for _, effect := range activity.Effects {
		if effect.AccountID != nil {
			return *effect.AccountID, true
		}
	}
	if activity.DividendDetail == nil {
		return "", false
	}
	holding, ok := holdings[activity.DividendDetail.HoldingID]
	if !ok {
		return "", false
	}
	return holding.AccountID, true
}

type gainGroupAccumulator struct {
	Key           string
	Label         string
	Total         decimal.Decimal
	Available     bool
	MissingReason string
}

func groupForInstrument(groups map[domain.InstrumentID]*gainGroupAccumulator, id domain.InstrumentID, label string) *gainGroupAccumulator {
	if group, ok := groups[id]; ok {
		return group
	}
	group := &gainGroupAccumulator{Key: id.String(), Label: label, Available: true}
	groups[id] = group
	return group
}

func groupForAccount(groups map[domain.AccountID]*gainGroupAccumulator, id domain.AccountID, label string) *gainGroupAccumulator {
	if group, ok := groups[id]; ok {
		return group
	}
	group := &gainGroupAccumulator{Key: id.String(), Label: label, Available: true}
	groups[id] = group
	return group
}

func totalFromGainGroups(groups []domain.GainGroupView, currency domain.CurrencyCode) (*domain.SignedMoneyView, error) {
	sum := decimal.Zero
	hasAvailable := false
	for _, group := range groups {
		if !group.Available {
			continue
		}
		amount, err := decimal.NewFromString(group.Gain.Amount)
		if err != nil {
			return nil, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "stored gain amount is invalid"}
		}
		sum = sum.Add(amount)
		hasAvailable = true
	}
	if !hasAvailable && len(groups) > 0 {
		return nil, nil
	}
	view, err := signedMoneyView(sum, currency)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func finishGainGroups[K comparable](groups map[K]*gainGroupAccumulator, currency domain.CurrencyCode) ([]domain.GainGroupView, error) {
	result := make([]domain.GainGroupView, 0, len(groups))
	for _, group := range groups {
		gain, err := signedMoneyView(group.Total, currency)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.GainGroupView{Key: group.Key, Label: group.Label, Gain: gain, Available: group.Available, MissingReason: group.MissingReason})
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, leftErr := decimal.NewFromString(result[i].Gain.Amount)
		right, rightErr := decimal.NewFromString(result[j].Gain.Amount)
		if leftErr != nil || rightErr != nil || left.Equal(right) {
			return result[i].Key < result[j].Key
		}
		return left.GreaterThan(right)
	})
	return result, nil
}

func (g *GainService) holdingGain(ctx context.Context, snapshot domain.PortfolioSnapshot, holding domain.Holding, instrument domain.Instrument, replay *costBasisReplayContext, fxQuotes []domain.FXQuote, origin *domain.HistoryOrigin) (domain.HoldingGainView, error) {
	result, err := replay.replay(ctx, holding.ID, nil)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	average, err := moneyView(result.Current.AverageUnitCost.Decimal(), instrument.QuoteCurrency)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	total, err := holding.Quantity.Multiply(result.Current.AverageUnitCost)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	totalView, err := moneyView(total, instrument.QuoteCurrency)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	realized, err := realizedGainView(result.Realized, instrument.QuoteCurrency)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	name, symbol := instrumentIdentity(instrument)
	view := domain.HoldingGainView{
		HoldingID: holding.ID, AccountID: holding.AccountID, InstrumentID: holding.InstrumentID,
		InstrumentName: name, InstrumentSymbol: symbol, Quantity: holding.Quantity.Canonical(),
		AverageCost: average, TotalCost: totalView, RealizedGain: realized, Available: true,
	}
	if holding.Quantity.IsZero() {
		zero, zeroErr := moneyView(decimal.Zero, instrument.QuoteCurrency)
		if zeroErr != nil {
			return domain.HoldingGainView{}, zeroErr
		}
		view.CurrentValue = &zero
		unrealized, unrealizedErr := signedMoneyView(decimal.Zero, instrument.QuoteCurrency)
		if unrealizedErr != nil {
			return domain.HoldingGainView{}, unrealizedErr
		}
		view.UnrealizedGain = &unrealized
		view.TotalCostBase = moneyViewPointer(decimal.Zero, snapshot.Household.BaseCurrency)
		view.CurrentValueBase = moneyViewPointer(decimal.Zero, snapshot.Household.BaseCurrency)
		view.UnrealizedGainBase = signedMoneyViewPointer(decimal.Zero, snapshot.Household.BaseCurrency)
		view.InstrumentMovement = signedMoneyViewPointer(decimal.Zero, snapshot.Household.BaseCurrency)
		view.CurrencyMovement = signedMoneyViewPointer(decimal.Zero, snapshot.Household.BaseCurrency)
		return view, nil
	}
	quote := selectInstrumentQuote(instrument, snapshot.InstrumentQuotes)
	if quote == nil {
		view.Available = false
		view.MissingReason = "current instrument price is unavailable"
		return view, nil
	}
	current, err := holding.Quantity.Multiply(quote.UnitPrice)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	currentView, err := moneyView(current, instrument.QuoteCurrency)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	view.CurrentValue = &currentView
	unrealizedValue := current.Sub(total)
	unrealized, err := signedMoneyView(unrealizedValue, instrument.QuoteCurrency)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	view.UnrealizedGain = &unrealized
	currentBase, _, missing, convertErr := g.valuation.convert(snapshot, holding.AccountID, current, instrument.QuoteCurrency)
	if convertErr != nil {
		return domain.HoldingGainView{}, convertErr
	}
	if missing != nil {
		view.Available = false
		view.MissingReason = "current foreign-exchange rate is unavailable"
		return view, nil
	}
	currentBaseView, err := moneyView(currentBase, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.HoldingGainView{}, err
	}
	view.CurrentValueBase = &currentBaseView
	decomposition, decompositionErr := g.decomposeHolding(ctx, snapshot, holding, instrument, result, replay, fxQuotes, origin)
	if decompositionErr != nil {
		return domain.HoldingGainView{}, decompositionErr
	}
	if !decomposition.Available {
		view.Available = false
		view.MissingReason = decomposition.MissingReason
		return view, nil
	}
	view.TotalCostBase = &decomposition.CostBase
	view.UnrealizedGainBase = &decomposition.UnrealizedGain
	view.InstrumentMovement = &decomposition.InstrumentMovement
	view.CurrencyMovement = &decomposition.CurrencyMovement
	return view, nil
}

func (g *GainService) gainFXInputs(ctx context.Context, snapshot domain.PortfolioSnapshot) ([]domain.FXQuote, *domain.HistoryOrigin, error) {
	quotes, err := g.repository.ListFXQuotes(ctx, snapshot.Household.ID)
	if err != nil {
		return nil, nil, err
	}
	origin, err := g.repository.HistoryOrigin(ctx, snapshot.Household.ID)
	if err != nil {
		return nil, nil, err
	}
	return quotes, origin, nil
}

type holdingDecomposition struct {
	Available          bool
	MissingReason      string
	CostBase           domain.MoneyView
	UnrealizedGain     domain.SignedMoneyView
	InstrumentMovement domain.SignedMoneyView
	CurrencyMovement   domain.SignedMoneyView
}

func (g *GainService) decomposeHolding(ctx context.Context, snapshot domain.PortfolioSnapshot, holding domain.Holding, instrument domain.Instrument, replayResult domain.CostBasisResult, replay *costBasisReplayContext, fxQuotes []domain.FXQuote, origin *domain.HistoryOrigin) (holdingDecomposition, error) {
	startingCost, events, err := replay.preparedEvents(ctx, holding.ID, nil)
	if err != nil {
		return holdingDecomposition{}, err
	}
	acquisitionRate, available := g.acquisitionFXRate(ctx, snapshot, instrument.QuoteCurrency, startingCost, events, replay, fxQuotes, origin)
	if !available {
		return holdingDecomposition{Available: false, MissingReason: "acquisition foreign-exchange rate is unavailable"}, nil
	}
	baseCost := replayResult.Current.AverageUnitCost.Decimal().Mul(holding.Quantity.Decimal()).Mul(acquisitionRate)
	// The current value is converted through ValuationService so the current
	// quote and FX orientation remain exactly the same as Portfolio valuation.
	currentQuote := selectInstrumentQuote(instrument, snapshot.InstrumentQuotes)
	if currentQuote == nil {
		return holdingDecomposition{Available: false, MissingReason: "current instrument price is unavailable"}, nil
	}
	currentNative, err := holding.Quantity.Multiply(currentQuote.UnitPrice)
	if err != nil {
		return holdingDecomposition{}, err
	}
	currentBase, _, missing, err := g.valuation.convert(snapshot, holding.AccountID, currentNative, instrument.QuoteCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	if missing != nil {
		return holdingDecomposition{Available: false, MissingReason: "current foreign-exchange rate is unavailable"}, nil
	}
	unrealized := currentBase.Sub(baseCost)
	instrumentMovement := currentQuote.UnitPrice.Decimal().Sub(replayResult.Current.AverageUnitCost.Decimal()).Mul(holding.Quantity.Decimal()).Mul(acquisitionRate)
	currencyMovement := unrealized.Sub(instrumentMovement)
	costView, err := moneyView(baseCost, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	unrealizedView, err := signedMoneyView(unrealized, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	instrumentView, err := signedMoneyView(instrumentMovement, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	currencyView, err := signedMoneyView(currencyMovement, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	return holdingDecomposition{Available: true, CostBase: costView, UnrealizedGain: unrealizedView, InstrumentMovement: instrumentView, CurrencyMovement: currencyView}, nil
}

func (g *GainService) acquisitionFXRate(ctx context.Context, snapshot domain.PortfolioSnapshot, native domain.CurrencyCode, startingCost *domain.UnitPrice, events []domain.CostBasisEvent, replay *costBasisReplayContext, fxQuotes []domain.FXQuote, origin *domain.HistoryOrigin) (decimal.Decimal, bool) {
	base := snapshot.Household.BaseCurrency
	if native == base {
		return decimal.NewFromInt(1), true
	}
	currentRate := decimal.Zero
	currentQuantity := decimal.Zero
	for _, event := range events {
		effectiveAt := event.EffectiveAt
		if event.Kind == domain.CostBasisStartingPoint && effectiveAt.IsZero() && origin != nil {
			effectiveAt = origin.StartedAt
		}
		switch event.Kind {
		case domain.CostBasisStartingPoint, domain.CostBasisBuy, domain.CostBasisAdjustmentIn:
			rate, ok := gainFXRateAtOrBefore(snapshot, fxQuotes, native, effectiveAt)
			if !ok {
				return decimal.Zero, false
			}
			currentRate, currentQuantity = blendFXRate(currentRate, currentQuantity, rate, event.Quantity.Decimal())
		case domain.CostBasisTransferIn:
			rate := decimal.Zero
			ok := false
			if event.SourceHoldingID != nil {
				cutoff := event.EffectiveAt
				starting, sourceEvents, err := replay.preparedEvents(ctx, *event.SourceHoldingID, &cutoff)
				if err != nil {
					return decimal.Zero, false
				}
				rate, ok = g.acquisitionFXRate(ctx, snapshot, native, starting, sourceEvents, replay, fxQuotes, origin)
			} else {
				rate, ok = gainFXRateAtOrBefore(snapshot, fxQuotes, native, effectiveAt)
			}
			if !ok {
				return decimal.Zero, false
			}
			currentRate, currentQuantity = blendFXRate(currentRate, currentQuantity, rate, event.Quantity.Decimal())
		case domain.CostBasisSell, domain.CostBasisTransferOut, domain.CostBasisAdjustmentOut:
			currentQuantity = currentQuantity.Sub(event.Quantity.Decimal())
			if currentQuantity.IsZero() {
				currentRate = decimal.Zero
			}
		}
	}
	if startingCost != nil && currentQuantity.IsZero() && len(events) == 0 {
		return decimal.Zero, false
	}
	if currentQuantity.IsZero() {
		return decimal.Zero, false
	}
	return currentRate, true
}

func blendFXRate(currentRate, currentQuantity, incomingRate, incomingQuantity decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if currentQuantity.IsZero() {
		return incomingRate, incomingQuantity
	}
	totalQuantity := currentQuantity.Add(incomingQuantity)
	return currentRate.Mul(currentQuantity).Add(incomingRate.Mul(incomingQuantity)).Div(totalQuantity), totalQuantity
}

func gainFXRateAtOrBefore(snapshot domain.PortfolioSnapshot, quotes []domain.FXQuote, native domain.CurrencyCode, cutoff time.Time) (decimal.Decimal, bool) {
	base := snapshot.Household.BaseCurrency
	if native == base {
		return decimal.NewFromInt(1), true
	}
	preference := findFXPreference(snapshot.FXPreferences, native, base)
	if preference == nil {
		return decimal.Zero, false
	}
	var selected *domain.FXQuote
	for index := range quotes {
		quote := &quotes[index]
		if quote.HouseholdID != preference.HouseholdID || quote.SourceKind != preference.SourceKind || quote.QuotedAt.After(cutoff) {
			continue
		}
		if !((quote.BaseCurrency == native && quote.QuoteCurrency == base) || (quote.BaseCurrency == base && quote.QuoteCurrency == native)) {
			continue
		}
		if selected == nil || quoteLater(quote.QuotedAt, quote.CreatedAt, quote.ID.String(), selected.QuotedAt, selected.CreatedAt, selected.ID.String()) {
			selected = quote
		}
	}
	if selected == nil {
		return decimal.Zero, false
	}
	if selected.BaseCurrency == native && selected.QuoteCurrency == base {
		return selected.Rate.Decimal(), true
	}
	return decimal.NewFromInt(1).Div(selected.Rate.Decimal()), true
}

func moneyViewPointer(value decimal.Decimal, currency domain.CurrencyCode) *domain.MoneyView {
	view, err := moneyView(value, currency)
	if err != nil {
		return nil
	}
	return &view
}

func signedMoneyViewPointer(value decimal.Decimal, currency domain.CurrencyCode) *domain.SignedMoneyView {
	view, err := signedMoneyView(value, currency)
	if err != nil {
		return nil
	}
	return &view
}

func (g *GainService) accountTotals(snapshot domain.PortfolioSnapshot, result domain.AccountGainView) (domain.AccountGainView, error) {
	if snapshot.Household == nil {
		return result, nil
	}
	totalCost := decimal.Zero
	currentValue := decimal.Zero
	realized := decimal.Zero
	unrealized := decimal.Zero
	hasTotalCost := true
	hasRealized := true
	hasCurrent := true
	for _, holding := range result.Holdings {
		if holding.TotalCostBase == nil {
			hasTotalCost = false
		} else {
			cost, costErr := decimal.NewFromString(holding.TotalCostBase.Amount)
			if costErr != nil {
				return domain.AccountGainView{}, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "gain amount is invalid"}
			}
			totalCost = totalCost.Add(cost)
		}
		gain, gainErr := convertSignedGain(g.valuation, snapshot, result.AccountID, holding.RealizedGain)
		if gainErr != nil {
			if isGainUnavailable(gainErr) {
				hasRealized = false
			} else {
				return domain.AccountGainView{}, gainErr
			}
		} else {
			realized = realized.Add(gain)
		}
		if holding.CurrentValueBase == nil || holding.UnrealizedGainBase == nil {
			hasCurrent = false
			continue
		}
		value, valueErr := decimal.NewFromString(holding.CurrentValueBase.Amount)
		if valueErr != nil {
			return domain.AccountGainView{}, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "gain amount is invalid"}
		}
		unreal, unrealErr := decimal.NewFromString(holding.UnrealizedGainBase.Amount)
		if unrealErr != nil {
			return domain.AccountGainView{}, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "gain amount is invalid"}
		}
		currentValue = currentValue.Add(value)
		unrealized = unrealized.Add(unreal)
	}
	base := snapshot.Household.BaseCurrency
	if hasTotalCost {
		totalCostView, err := moneyView(totalCost, base)
		if err != nil {
			return domain.AccountGainView{}, err
		}
		result.TotalCost = &totalCostView
	}
	if hasRealized {
		realizedView, err := signedMoneyView(realized, base)
		if err != nil {
			return domain.AccountGainView{}, err
		}
		result.RealizedGain = &realizedView
	}
	if hasCurrent {
		valueView, valueErr := moneyView(currentValue, base)
		if valueErr != nil {
			return domain.AccountGainView{}, valueErr
		}
		result.CurrentValue = &valueView
		unrealView, unrealErr := signedMoneyView(unrealized, base)
		if unrealErr != nil {
			return domain.AccountGainView{}, unrealErr
		}
		result.UnrealizedGain = &unrealView
	} else {
		result.Available = false
	}
	return result, nil
}

func isGainUnavailable(err error) bool {
	appErr, ok := err.(*domain.Error)
	return ok && appErr.Code == domain.ErrUnavailable
}

func gainHolding(snapshot domain.PortfolioSnapshot, id domain.HoldingID) (domain.Holding, domain.Instrument, error) {
	for _, holding := range snapshot.Holdings {
		if holding.ID != id || holding.ArchivedAt != nil {
			continue
		}
		instrument, ok := gainInstrument(snapshot.Instruments, holding.InstrumentID)
		if !ok {
			break
		}
		return holding, instrument, nil
	}
	return domain.Holding{}, domain.Instrument{}, &domain.Error{Code: domain.ErrNotFound, Message: "holding was not found"}
}

func gainInstrument(instruments []domain.Instrument, id domain.InstrumentID) (domain.Instrument, bool) {
	for _, instrument := range instruments {
		if instrument.ID == id {
			return instrument, true
		}
	}
	return domain.Instrument{}, false
}

func realizedGainView(events []domain.RealizedGainEvent, currency domain.CurrencyCode) (domain.SignedMoneyView, error) {
	total := decimal.Zero
	for _, event := range events {
		if event.RealizedGain.Currency() != currency {
			return domain.SignedMoneyView{}, &domain.Error{Code: domain.ErrIntegrity, Field: "currency", Message: "realized gains use inconsistent settlement currencies"}
		}
		total = total.Add(event.RealizedGain.Amount())
	}
	return signedMoneyView(total, currency)
}

func convertGainAmount(valuation *ValuationService, snapshot domain.PortfolioSnapshot, accountID domain.AccountID, amount string, currency domain.CurrencyCode) (decimal.Decimal, error) {
	native, err := decimal.NewFromString(amount)
	if err != nil {
		return decimal.Zero, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "gain amount is invalid"}
	}
	negative := native.IsNegative()
	converted, _, missing, err := valuation.convert(snapshot, accountID, native.Abs(), currency)
	if err != nil {
		return decimal.Zero, err
	}
	if missing != nil {
		return decimal.Zero, &domain.Error{Code: domain.ErrUnavailable, Field: "fxRate", Message: "current foreign-exchange rate is unavailable"}
	}
	if negative {
		converted = converted.Neg()
	}
	return converted, nil
}

func convertSignedGain(valuation *ValuationService, snapshot domain.PortfolioSnapshot, accountID domain.AccountID, amount domain.SignedMoneyView) (decimal.Decimal, error) {
	converted, err := convertGainAmount(valuation, snapshot, accountID, amount.Amount, amount.Currency)
	return converted, err
}

func signedMoneyView(value decimal.Decimal, currency domain.CurrencyCode) (domain.SignedMoneyView, error) {
	money, err := domain.NewSignedMoney(value, currency)
	if err != nil {
		return domain.SignedMoneyView{}, err
	}
	return domain.SignedMoneyView{Amount: money.CanonicalAmount(), Currency: money.Currency()}, nil
}

type replayMemoKey struct {
	holdingID domain.HoldingID
	cutoff    time.Time
	hasCutoff bool
}

type costBasisReplayContext struct {
	repository Repository
	quantities map[domain.HoldingID]domain.Quantity
	events     map[domain.HoldingID][]domain.CostBasisEvent
	starting   map[domain.HoldingID]*domain.UnitPrice
	results    map[replayMemoKey]domain.CostBasisResult
	active     map[replayMemoKey]bool
}

// historicalCostBasisFilter loads immutable cost facts after a Holding is
// archived so period realized gain and transfer-source replay stay complete.
var historicalCostBasisFilter = domain.CostBasisReadFilter{IncludeArchivedHoldings: true}

func newCostBasisReplayContext(repository Repository, holdings ...[]domain.Holding) *costBasisReplayContext {
	quantities := make(map[domain.HoldingID]domain.Quantity)
	if len(holdings) > 0 {
		for _, holding := range holdings[0] {
			quantities[holding.ID] = holding.Quantity
		}
	}
	return &costBasisReplayContext{repository: repository, quantities: quantities, events: make(map[domain.HoldingID][]domain.CostBasisEvent), starting: make(map[domain.HoldingID]*domain.UnitPrice), results: make(map[replayMemoKey]domain.CostBasisResult), active: make(map[replayMemoKey]bool)}
}

func (c *costBasisReplayContext) replay(ctx context.Context, holdingID domain.HoldingID, cutoff *time.Time) (domain.CostBasisResult, error) {
	key := replayMemoKey{holdingID: holdingID}
	if cutoff != nil {
		key.cutoff, key.hasCutoff = cutoff.UTC(), true
	}
	if result, ok := c.results[key]; ok {
		return result, nil
	}
	if c.active[key] {
		return domain.CostBasisResult{}, &domain.Error{Code: domain.ErrIntegrity, Message: "position transfer cost cycle detected"}
	}
	c.active[key] = true
	defer delete(c.active, key)
	starting, events, err := c.preparedEvents(ctx, holdingID, cutoff)
	if err != nil {
		return domain.CostBasisResult{}, err
	}
	result, err := domain.ReplayCostBasis(starting, events)
	if err != nil {
		return domain.CostBasisResult{}, err
	}
	c.results[key] = result
	return result, nil
}

func (c *costBasisReplayContext) preparedEvents(ctx context.Context, holdingID domain.HoldingID, cutoff *time.Time) (*domain.UnitPrice, []domain.CostBasisEvent, error) {
	if _, ok := c.events[holdingID]; !ok {
		events, err := c.repository.ListCostBasisEvents(ctx, holdingID, historicalCostBasisFilter)
		if err != nil {
			return nil, nil, err
		}
		c.events[holdingID] = events
	}
	if _, ok := c.starting[holdingID]; !ok {
		starting, err := c.repository.StartingPointCost(ctx, holdingID, historicalCostBasisFilter)
		if err != nil {
			return nil, nil, err
		}
		c.starting[holdingID] = starting
	}
	events := c.events[holdingID]
	if cutoff != nil {
		filtered := make([]domain.CostBasisEvent, 0, len(events))
		for _, event := range events {
			if !event.EffectiveAt.After(*cutoff) {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	} else {
		events = append([]domain.CostBasisEvent(nil), events...)
	}
	if c.starting[holdingID] != nil && !hasStartingPointEvent(events) {
		quantity, ok := c.quantities[holdingID]
		if !ok {
			return nil, nil, &domain.Error{Code: domain.ErrCostBasisRequired, Field: "quantity", Message: "a Starting Point quantity is unavailable"}
		}
		startingQuantity := quantity.Decimal()
		for _, event := range c.events[holdingID] {
			switch event.Kind {
			case domain.CostBasisBuy, domain.CostBasisTransferIn, domain.CostBasisAdjustmentIn:
				startingQuantity = startingQuantity.Sub(event.Quantity.Decimal())
			case domain.CostBasisSell, domain.CostBasisTransferOut, domain.CostBasisAdjustmentOut:
				startingQuantity = startingQuantity.Add(event.Quantity.Decimal())
			}
		}
		parsedQuantity, quantityErr := domain.NewQuantity(startingQuantity)
		if quantityErr != nil {
			return nil, nil, quantityErr
		}
		events = append([]domain.CostBasisEvent{{Kind: domain.CostBasisStartingPoint, Quantity: parsedQuantity}}, events...)
	}
	for index := range events {
		event := &events[index]
		if event.Kind != domain.CostBasisTransferIn || event.UnitCost != nil || event.SourceHoldingID == nil {
			continue
		}
		resolved, err := c.replay(ctx, *event.SourceHoldingID, &event.EffectiveAt)
		if err != nil {
			return nil, nil, err
		}
		if resolved.Current.Quantity.IsZero() {
			return nil, nil, &domain.Error{Code: domain.ErrCostBasisRequired, Field: "unitCost", Message: "a position transfer source has no resolvable cost"}
		}
		cost := resolved.Current.AverageUnitCost
		event.UnitCost = &cost
	}
	return c.starting[holdingID], events, nil
}

func hasStartingPointEvent(events []domain.CostBasisEvent) bool {
	for _, event := range events {
		if event.Kind == domain.CostBasisStartingPoint {
			return true
		}
	}
	return false
}
