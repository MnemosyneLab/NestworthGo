package application

import (
	"context"
	"math/big"
	"sort"
	"strings"
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
	inputs, err := g.repository.ReadGainSnapshot(ctx)
	snapshot := inputs.Portfolio
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
	fxQuotes, origin := inputs.HistoricalFXQuotes, snapshot.Origin
	replay := newGainSnapshotReplay(inputs)
	return g.holdingGain(ctx, snapshot, holding, instrument, replay, fxQuotes, origin, nil)
}

func (g *GainService) AccountGain(ctx context.Context, accountID domain.AccountID) (domain.AccountGainView, error) {
	views, err := g.AccountGains(ctx, []domain.AccountID{accountID})
	if err != nil {
		return domain.AccountGainView{}, err
	}
	return views[0], nil
}

// AccountGains returns cost/gain views for the requested Accounts from one
// portfolio snapshot and one shared cost-basis replay. An empty ID list
// returns every Account in the snapshot, ordered by Account ID.
func (g *GainService) AccountGains(ctx context.Context, accountIDs []domain.AccountID) ([]domain.AccountGainView, error) {
	inputs, err := g.repository.ReadGainSnapshot(ctx)
	snapshot := inputs.Portfolio
	if err != nil {
		return nil, err
	}
	if snapshot.Household == nil {
		if len(accountIDs) > 0 {
			return nil, &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
		}
		return []domain.AccountGainView{}, nil
	}
	accountsByID := make(map[domain.AccountID]struct{}, len(snapshot.Accounts))
	for _, record := range snapshot.Accounts {
		accountsByID[record.Account.ID] = struct{}{}
	}
	ids := accountIDs
	if len(ids) == 0 {
		ids = make([]domain.AccountID, 0, len(snapshot.Accounts))
		for _, record := range snapshot.Accounts {
			ids = append(ids, record.Account.ID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	} else {
		for _, id := range ids {
			if _, ok := accountsByID[id]; !ok {
				return nil, &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
			}
		}
	}
	fxQuotes, origin := inputs.HistoricalFXQuotes, snapshot.Origin
	replay := newGainSnapshotReplay(inputs)
	results := make([]domain.AccountGainView, 0, len(ids))
	for _, accountID := range ids {
		view, viewErr := g.accountGainFromSnapshot(ctx, snapshot, accountID, replay, fxQuotes, origin)
		if viewErr != nil {
			return nil, viewErr
		}
		results = append(results, view)
	}
	return results, nil
}

func (g *GainService) accountGainFromSnapshot(ctx context.Context, snapshot domain.PortfolioSnapshot, accountID domain.AccountID, replay *costBasisReplayContext, fxQuotes []domain.FXQuote, origin *domain.HistoryOrigin) (domain.AccountGainView, error) {
	result := domain.AccountGainView{AccountID: accountID, Holdings: make([]domain.HoldingGainView, 0), Available: true}
	exact := make(map[domain.HoldingID]*exactHoldingGain)
	for _, holding := range snapshot.Holdings {
		if holding.AccountID != accountID || holding.ArchivedAt != nil {
			continue
		}
		instrument, ok := gainInstrument(snapshot.Instruments, holding.InstrumentID)
		if !ok {
			continue
		}
		amounts := &exactHoldingGain{}
		replayed, replayErr := replay.replay(ctx, holding.ID, nil)
		if replayErr != nil {
			return domain.AccountGainView{}, replayErr
		}
		realizedBase, realizedAvailable, realizedErr := g.realizedGainInBaseAtEventFX(snapshot, fxQuotes, replayed.Realized)
		if realizedErr != nil {
			return domain.AccountGainView{}, realizedErr
		}
		amounts.RealizedBase = realizedBase
		amounts.RealizedBaseAvailable = realizedAvailable
		view, viewErr := g.holdingGain(ctx, snapshot, holding, instrument, replay, fxQuotes, origin, amounts)
		exact[holding.ID] = amounts
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
	return g.accountTotals(snapshot, result, exact)
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
	inputs, err := g.repository.ReadGainSnapshot(ctx)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	return g.realizedGainFromSnapshot(ctx, scope, from, to, inputs)
}

func (g *GainService) realizedGainFromSnapshot(ctx context.Context, scope domain.GainScope, from, to domain.LocalDate, inputs domain.GainSnapshot) (domain.RealizedGainView, error) {
	snapshot := inputs.Portfolio
	var err error
	if snapshot.Household == nil {
		return domain.RealizedGainView{From: from, To: to, Available: true}, nil
	}
	fxQuotes, origin := inputs.HistoricalFXQuotes, snapshot.Origin
	location := time.UTC
	if origin != nil {
		location, err = time.LoadLocation(origin.Timezone)
		if err != nil {
			return domain.RealizedGainView{}, err
		}
	}
	replay := newGainSnapshotReplay(inputs)
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
			instrumentGroup := groupForInstrument(byInstrument, instrument.ID, instrument.DisplayLabel())
			accountGroup := groupForAccount(byAccount, holding.AccountID, accounts[holding.AccountID])
			converted, rateAvailable, conversionErr := g.convertFXExactAtOrBefore(snapshot, fxQuotes, event.RealizedGain.Amount(), event.RealizedGain.Currency(), event.EffectiveAt)
			if conversionErr != nil {
				return domain.RealizedGainView{}, conversionErr
			}
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
			addExactRealizedGain(instrumentGroup, converted)
			addExactRealizedGain(accountGroup, converted)
		}
	}
	result.ByInstrument, err = finishExactGainGroups(byInstrument, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	result.ByAccount, err = finishExactGainGroups(byAccount, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	result.Total, err = totalFromExactGainGroups(byInstrument, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	return result, nil
}

// RealizedGain resolves the existing Analytics range selector to an
// inclusive local-date range using the service clock and the history
// timezone, then delegates to the explicit range reader.
func (g *GainService) RealizedGain(ctx context.Context, scope domain.GainScope, trendRange domain.TrendRange) (domain.RealizedGainView, error) {
	inputs, err := g.repository.ReadGainSnapshot(ctx)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	from, to, err := g.trendRangeBounds(inputs.Portfolio, trendRange)
	if err != nil {
		return domain.RealizedGainView{}, err
	}
	if from == "" {
		return domain.RealizedGainView{Available: true}, nil
	}
	return g.realizedGainFromSnapshot(ctx, scope, from, to, inputs)
}

const dividendIncomeMissingFX = "dividend income foreign-exchange rate is unavailable"

const historicalRealizedGainMissingFX = "historical realized-gain foreign-exchange rate is unavailable"

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
	inputs, err := g.repository.ReadGainSnapshot(ctx)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	return g.dividendIncomeFromSnapshot(ctx, scope, from, to, inputs)
}

func (g *GainService) dividendIncomeFromSnapshot(ctx context.Context, scope domain.GainScope, from, to domain.LocalDate, inputs domain.GainSnapshot) (domain.DividendIncomeView, error) {
	snapshot := inputs.Portfolio
	var err error
	if snapshot.Household == nil {
		return domain.DividendIncomeView{From: from, To: to, Available: true}, nil
	}
	fxQuotes := inputs.HistoricalFXQuotes
	activities := inputs.Dividends

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
		if activity.EffectiveLocalDate < from || activity.EffectiveLocalDate > to {
			continue
		}
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
			instrumentLabel = instrument.DisplayLabel()
		}
		accountLabel := accountID.String()
		if name := accounts[accountID]; name != "" {
			accountLabel = name
		}
		instrumentGroup := groupForInstrument(byInstrument, detail.InstrumentID, instrumentLabel)
		accountGroup := groupForAccount(byAccount, accountID, accountLabel)
		converted, rateAvailable, conversionErr := g.convertFXAtOrBefore(snapshot, fxQuotes, detail.Amount.Amount(), detail.Amount.Currency(), activity.EffectiveAt)
		if conversionErr != nil {
			return domain.DividendIncomeView{}, conversionErr
		}
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
	result.Total, err = totalFromGainGroups(byInstrument, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	return result, nil
}

// DividendIncome resolves an Analytics trend range, then delegates to the
// explicit dividend-income range reader.
func (g *GainService) DividendIncome(ctx context.Context, scope domain.GainScope, trendRange domain.TrendRange) (domain.DividendIncomeView, error) {
	inputs, err := g.repository.ReadGainSnapshot(ctx)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	from, to, err := g.trendRangeBounds(inputs.Portfolio, trendRange)
	if err != nil {
		return domain.DividendIncomeView{}, err
	}
	if from == "" {
		return domain.DividendIncomeView{Available: true}, nil
	}
	return g.dividendIncomeFromSnapshot(ctx, scope, from, to, inputs)
}

func (g *GainService) trendRangeBounds(snapshot domain.PortfolioSnapshot, trendRange domain.TrendRange) (from, to domain.LocalDate, err error) {
	if snapshot.Household == nil {
		return "", "", nil
	}
	origin := snapshot.Origin
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
	case domain.TrendYearToDate:
		from = time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, location).Format("2006-01-02")
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
	ExactRealized *big.Rat
	Available     bool
	MissingReason string
}

func addExactRealizedGain(group *gainGroupAccumulator, amount *big.Rat) {
	if group.ExactRealized == nil {
		group.ExactRealized = new(big.Rat)
	}
	group.ExactRealized.Add(group.ExactRealized, amount)
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

func totalFromGainGroups[K comparable](groups map[K]*gainGroupAccumulator, currency domain.CurrencyCode) (*domain.SignedMoneyView, error) {
	sum := decimal.Zero
	hasAvailable := false
	for _, group := range groups {
		if !group.Available {
			continue
		}
		sum = sum.Add(group.Total)
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

func totalFromExactGainGroups[K comparable](groups map[K]*gainGroupAccumulator, currency domain.CurrencyCode) (*domain.SignedMoneyView, error) {
	sum := new(big.Rat)
	hasAvailable := false
	for _, group := range groups {
		if !group.Available {
			continue
		}
		if group.ExactRealized != nil {
			sum.Add(sum, group.ExactRealized)
		}
		hasAvailable = true
	}
	if !hasAvailable && len(groups) > 0 {
		return nil, nil
	}
	view, err := rationalSignedMoneyView(sum, currency)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func finishExactGainGroups[K comparable](groups map[K]*gainGroupAccumulator, currency domain.CurrencyCode) ([]domain.GainGroupView, error) {
	result := make([]domain.GainGroupView, 0, len(groups))
	for _, group := range groups {
		amount := group.ExactRealized
		if amount == nil {
			amount = new(big.Rat)
		}
		gain, err := rationalSignedMoneyView(amount, currency)
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

// exactHoldingGain is internal calculation data, never a wire/view value.
// Aggregation uses these decimals rather than reparsing rounded MoneyViews.
type exactHoldingGain struct {
	Cost, Value, Unrealized             decimal.Decimal
	CostBase, ValueBase, UnrealizedBase *big.Rat
	Realized                            decimal.Decimal
	RealizedBase                        *big.Rat
	RealizedBaseAvailable               bool
}

// realizedGainInBaseAtEventFX preserves AccountGainView's historical meaning:
// each replayed sale uses only the FX observation available at that sale's
// EffectiveAt. It returns no subtotal if any required observation is missing;
// the caller applies one Money projection after aggregating the account.
func (g *GainService) realizedGainInBaseAtEventFX(snapshot domain.PortfolioSnapshot, fxQuotes []domain.FXQuote, events []domain.RealizedGainEvent) (*big.Rat, bool, error) {
	total := new(big.Rat)
	for _, event := range events {
		converted, available, err := g.convertFXExactAtOrBefore(snapshot, fxQuotes, event.RealizedGain.Amount(), event.RealizedGain.Currency(), event.EffectiveAt)
		if err != nil {
			return nil, false, err
		}
		if !available {
			return nil, false, nil
		}
		total.Add(total, converted)
	}
	return total, true, nil
}

func (g *GainService) holdingGain(ctx context.Context, snapshot domain.PortfolioSnapshot, holding domain.Holding, instrument domain.Instrument, replay *costBasisReplayContext, fxQuotes []domain.FXQuote, origin *domain.HistoryOrigin, exact *exactHoldingGain) (domain.HoldingGainView, error) {
	if exact == nil {
		exact = &exactHoldingGain{}
	}
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
	exact.Cost = total
	for _, sale := range result.Realized {
		exact.Realized = exact.Realized.Add(sale.RealizedGain.Amount())
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
		InstrumentName: name, InstrumentShortName: instrument.ShortName, InstrumentSymbol: symbol, Quantity: holding.Quantity.Canonical(),
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
		exact.CostBase = new(big.Rat)
		exact.ValueBase = new(big.Rat)
		exact.UnrealizedBase = new(big.Rat)
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
	exact.Value = current
	exact.Unrealized = current.Sub(total)
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
	exact.ValueBase = rationalFromDecimal(currentBase)
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
	exact.CostBase = new(big.Rat).Set(decomposition.exactCostBase)
	exact.UnrealizedBase = new(big.Rat).Set(decomposition.exactUnrealized)
	view.TotalCostBase = &decomposition.CostBase
	view.UnrealizedGainBase = &decomposition.UnrealizedGain
	view.InstrumentMovement = &decomposition.InstrumentMovement
	view.CurrencyMovement = &decomposition.CurrencyMovement
	return view, nil
}

type holdingDecomposition struct {
	exactCostBase, exactUnrealized *big.Rat
	Available                      bool
	MissingReason                  string
	CostBase                       domain.MoneyView
	UnrealizedGain                 domain.SignedMoneyView
	InstrumentMovement             domain.SignedMoneyView
	CurrencyMovement               domain.SignedMoneyView
}

func (g *GainService) decomposeHolding(ctx context.Context, snapshot domain.PortfolioSnapshot, holding domain.Holding, instrument domain.Instrument, replayResult domain.CostBasisResult, replay *costBasisReplayContext, fxQuotes []domain.FXQuote, origin *domain.HistoryOrigin) (holdingDecomposition, error) {
	startingCost, events, err := replay.preparedEvents(ctx, holding.ID, nil)
	if err != nil {
		return holdingDecomposition{}, err
	}
	acquisitionRate, available, err := g.acquisitionFXRatio(ctx, snapshot, instrument.QuoteCurrency, startingCost, events, replay, fxQuotes, origin)
	if err != nil {
		return holdingDecomposition{}, err
	}
	if !available {
		return holdingDecomposition{Available: false, MissingReason: "acquisition foreign-exchange rate is unavailable"}, nil
	}
	nativeCost, err := holding.Quantity.Multiply(replayResult.Current.AverageUnitCost)
	if err != nil {
		return holdingDecomposition{}, err
	}
	baseCost := new(big.Rat).Mul(rationalFromDecimal(nativeCost), acquisitionRate)
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
	currentBaseExact := rationalFromDecimal(currentBase)
	unrealized := new(big.Rat).Sub(currentBaseExact, baseCost)
	nativePriceMovement := new(big.Rat).Sub(rationalFromDecimal(currentQuote.UnitPrice.Decimal()), rationalFromDecimal(replayResult.Current.AverageUnitCost.Decimal()))
	nativePriceMovement.Mul(nativePriceMovement, rationalFromDecimal(holding.Quantity.Decimal()))
	instrumentMovement := new(big.Rat).Mul(nativePriceMovement, acquisitionRate)
	currencyMovement := new(big.Rat).Sub(unrealized, instrumentMovement)
	costView, err := rationalMoneyView(baseCost, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	unrealizedView, err := rationalSignedMoneyView(unrealized, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	instrumentView, err := rationalSignedMoneyView(instrumentMovement, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	currencyView, err := rationalSignedMoneyView(currencyMovement, snapshot.Household.BaseCurrency)
	if err != nil {
		return holdingDecomposition{}, err
	}
	return holdingDecomposition{exactCostBase: baseCost, exactUnrealized: unrealized, Available: true, CostBase: costView, UnrealizedGain: unrealizedView, InstrumentMovement: instrumentView, CurrencyMovement: currencyView}, nil
}

func (g *GainService) acquisitionFXRatio(ctx context.Context, snapshot domain.PortfolioSnapshot, native domain.CurrencyCode, startingCost *domain.UnitPrice, events []domain.CostBasisEvent, replay *costBasisReplayContext, fxQuotes []domain.FXQuote, origin *domain.HistoryOrigin) (*big.Rat, bool, error) {
	base := snapshot.Household.BaseCurrency
	if native == base {
		return new(big.Rat).SetInt64(1), true, nil
	}
	weightedRate := new(big.Rat)
	currentQuantity := new(big.Rat)
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		effectiveAt := event.EffectiveAt
		if event.Kind == domain.CostBasisStartingPoint && effectiveAt.IsZero() && origin != nil {
			effectiveAt = origin.StartedAt
		}
		switch event.Kind {
		case domain.CostBasisCostAdjustment:
			// A corrected native cost does not represent a new acquisition or
			// currency exchange. Keep the existing acquisition FX provenance.
		case domain.CostBasisStartingPoint, domain.CostBasisBuy, domain.CostBasisAdjustmentIn:
			rate, ok := g.acquisitionQuoteRatio(snapshot, fxQuotes, native, effectiveAt)
			if !ok {
				return nil, false, nil
			}
			weightedRate, currentQuantity = blendExactFXRatio(weightedRate, currentQuantity, rate, event.Quantity.Decimal())
		case domain.CostBasisTransferIn:
			var rate *big.Rat
			var ok bool
			if event.SourceHoldingID != nil {
				cutoff := event.Boundary()
				starting, sourceEvents, err := replay.preparedEvents(ctx, *event.SourceHoldingID, &cutoff)
				if err != nil {
					return nil, false, err
				}
				rate, ok, err = g.acquisitionFXRatio(ctx, snapshot, native, starting, sourceEvents, replay, fxQuotes, origin)
				if err != nil {
					return nil, false, err
				}
			} else {
				rate, ok = g.acquisitionQuoteRatio(snapshot, fxQuotes, native, effectiveAt)
			}
			if !ok {
				return nil, false, nil
			}
			weightedRate, currentQuantity = blendExactFXRatio(weightedRate, currentQuantity, rate, event.Quantity.Decimal())
		case domain.CostBasisSell, domain.CostBasisTransferOut, domain.CostBasisAdjustmentOut:
			outQuantity := rationalFromDecimal(event.Quantity.Decimal())
			remaining := new(big.Rat).Sub(currentQuantity, outQuantity)
			if remaining.Sign() < 0 {
				return nil, false, &domain.Error{Code: domain.ErrIntegrity, Field: "quantity", Message: "acquisition FX replay quantity is negative"}
			}
			if remaining.Sign() == 0 {
				weightedRate = new(big.Rat)
				currentQuantity = new(big.Rat)
			} else if currentQuantity.Sign() > 0 {
				average := new(big.Rat).Quo(weightedRate, currentQuantity)
				weightedRate = new(big.Rat).Mul(average, remaining)
				currentQuantity = remaining
			}
		}
	}
	if startingCost != nil && currentQuantity.Sign() == 0 && len(events) == 0 {
		return nil, false, nil
	}
	if currentQuantity.Sign() <= 0 {
		return nil, false, nil
	}
	return new(big.Rat).Quo(weightedRate, currentQuantity), true, nil
}

func (g *GainService) acquisitionQuoteRatio(snapshot domain.PortfolioSnapshot, quotes []domain.FXQuote, native domain.CurrencyCode, cutoff time.Time) (*big.Rat, bool) {
	if snapshot.Household == nil {
		return nil, false
	}
	base := snapshot.Household.BaseCurrency
	if native == base {
		return new(big.Rat).SetInt64(1), true
	}
	quote := g.valuation.selectFXQuote(snapshot, quotes, native, &cutoff)
	if quote == nil || quote.Rate.Decimal().IsZero() {
		return nil, false
	}
	rate := rationalFromDecimal(quote.Rate.Decimal())
	if quote.BaseCurrency == native && quote.QuoteCurrency == base {
		return rate, true
	}
	if quote.BaseCurrency == base && quote.QuoteCurrency == native {
		return new(big.Rat).Inv(rate), true
	}
	return nil, false
}

func blendExactFXRatio(weightedRate, currentQuantity, incomingRate *big.Rat, incomingQuantity decimal.Decimal) (*big.Rat, *big.Rat) {
	quantity := rationalFromDecimal(incomingQuantity)
	weighted := new(big.Rat).Add(weightedRate, new(big.Rat).Mul(incomingRate, quantity))
	return weighted, new(big.Rat).Add(currentQuantity, quantity)
}

func (g *GainService) fxRateAtOrBefore(snapshot domain.PortfolioSnapshot, quotes []domain.FXQuote, native domain.CurrencyCode, cutoff time.Time) (decimal.Decimal, bool) {
	return g.valuation.fxRateAtOrBefore(snapshot, quotes, native, cutoff)
}

func (g *GainService) convertFXAtOrBefore(snapshot domain.PortfolioSnapshot, quotes []domain.FXQuote, amount decimal.Decimal, native domain.CurrencyCode, cutoff time.Time) (decimal.Decimal, bool, error) {
	if snapshot.Household == nil {
		return decimal.Zero, false, nil
	}
	base := snapshot.Household.BaseCurrency
	if native == base {
		return amount, true, nil
	}
	quote := g.fxQuoteAtOrBefore(snapshot, quotes, native, cutoff)
	if quote == nil {
		return decimal.Zero, false, nil
	}
	converted, err := domain.ConvertFXAmount(amount, native, base, *quote)
	return converted, err == nil, err
}

func (g *GainService) convertFXExactAtOrBefore(snapshot domain.PortfolioSnapshot, quotes []domain.FXQuote, amount decimal.Decimal, native domain.CurrencyCode, cutoff time.Time) (*big.Rat, bool, error) {
	if snapshot.Household == nil {
		return nil, false, nil
	}
	base := snapshot.Household.BaseCurrency
	if native == base {
		return rationalFromDecimal(amount), true, nil
	}
	quote := g.fxQuoteAtOrBefore(snapshot, quotes, native, cutoff)
	if quote == nil {
		return nil, false, nil
	}
	converted, err := domain.ConvertFXAmountExact(amount, native, base, *quote)
	if err != nil {
		return nil, false, err
	}
	return converted, true, nil
}

func (g *GainService) fxQuoteAtOrBefore(snapshot domain.PortfolioSnapshot, quotes []domain.FXQuote, native domain.CurrencyCode, cutoff time.Time) *domain.FXQuote {
	if snapshot.Household == nil || native == snapshot.Household.BaseCurrency {
		return nil
	}
	return g.valuation.selectFXQuote(snapshot, quotes, native, &cutoff)
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

func (g *GainService) accountTotals(snapshot domain.PortfolioSnapshot, result domain.AccountGainView, exact map[domain.HoldingID]*exactHoldingGain) (domain.AccountGainView, error) {
	if snapshot.Household == nil {
		return result, nil
	}
	totalCost := new(big.Rat)
	currentValue := new(big.Rat)
	realized := new(big.Rat)
	unrealized := new(big.Rat)
	hasTotalCost := true
	hasRealized := true
	hasCurrent := true
	for _, holding := range result.Holdings {
		amounts := exact[holding.HoldingID]
		if holding.TotalCostBase == nil {
			hasTotalCost = false
		} else {
			totalCost.Add(totalCost, amounts.CostBase)
		}
		if !amounts.RealizedBaseAvailable {
			hasRealized = false
			result.Available = false
			if result.MissingReason == "" {
				result.MissingReason = historicalRealizedGainMissingFX
			} else if !strings.Contains(result.MissingReason, historicalRealizedGainMissingFX) {
				result.MissingReason += "; " + historicalRealizedGainMissingFX
			}
		} else {
			realized.Add(realized, amounts.RealizedBase)
		}
		if holding.CurrentValueBase == nil || holding.UnrealizedGainBase == nil {
			hasCurrent = false
			continue
		}
		currentValue.Add(currentValue, amounts.ValueBase)
		unrealized.Add(unrealized, amounts.UnrealizedBase)
	}
	base := snapshot.Household.BaseCurrency
	if hasTotalCost {
		totalCostView, err := rationalMoneyView(totalCost, base)
		if err != nil {
			return domain.AccountGainView{}, err
		}
		result.TotalCost = &totalCostView
	}
	if hasRealized {
		realizedView, err := rationalSignedMoneyView(realized, base)
		if err != nil {
			return domain.AccountGainView{}, err
		}
		result.RealizedGain = &realizedView
	}
	if hasCurrent {
		valueView, valueErr := rationalMoneyView(currentValue, base)
		if valueErr != nil {
			return domain.AccountGainView{}, valueErr
		}
		result.CurrentValue = &valueView
		unrealView, unrealErr := rationalSignedMoneyView(unrealized, base)
		if unrealErr != nil {
			return domain.AccountGainView{}, unrealErr
		}
		result.UnrealizedGain = &unrealView
	} else {
		result.Available = false
	}
	return result, nil
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

func signedMoneyView(value decimal.Decimal, currency domain.CurrencyCode) (domain.SignedMoneyView, error) {
	money, err := domain.NewSignedMoney(value, currency)
	if err != nil {
		return domain.SignedMoneyView{}, err
	}
	return domain.SignedMoneyView{Amount: money.CanonicalAmount(), Currency: money.Currency()}, nil
}

type replayMemoKey struct {
	holdingID domain.HoldingID
	cutoff    domain.CostBasisBoundary
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

func newGainSnapshotReplay(inputs domain.GainSnapshot) *costBasisReplayContext {
	replay := newCostBasisReplayContext(nil, inputs.Portfolio.Holdings)
	replay.events = inputs.CostEvents
	replay.starting = inputs.StartingCosts
	return replay
}

func newCostBasisReplayContext(repository Repository, holdings ...[]domain.Holding) *costBasisReplayContext {
	quantities := make(map[domain.HoldingID]domain.Quantity)
	if len(holdings) > 0 {
		for _, holding := range holdings[0] {
			quantities[holding.ID] = holding.Quantity
		}
	}
	return &costBasisReplayContext{repository: repository, quantities: quantities, events: make(map[domain.HoldingID][]domain.CostBasisEvent), starting: make(map[domain.HoldingID]*domain.UnitPrice), results: make(map[replayMemoKey]domain.CostBasisResult), active: make(map[replayMemoKey]bool)}
}

func (c *costBasisReplayContext) replay(ctx context.Context, holdingID domain.HoldingID, cutoff *domain.CostBasisBoundary) (domain.CostBasisResult, error) {
	key := replayMemoKey{holdingID: holdingID}
	if cutoff != nil {
		key.cutoff, key.hasCutoff = *cutoff, true
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

func (c *costBasisReplayContext) preparedEvents(ctx context.Context, holdingID domain.HoldingID, cutoff *domain.CostBasisBoundary) (*domain.UnitPrice, []domain.CostBasisEvent, error) {
	if _, ok := c.events[holdingID]; !ok {
		if c.repository == nil {
			return nil, nil, &domain.Error{Code: domain.ErrIntegrity, Message: "cost events are missing from gain snapshot"}
		}
		events, err := c.repository.ListCostBasisEvents(ctx, holdingID, historicalCostBasisFilter)
		if err != nil {
			return nil, nil, err
		}
		c.events[holdingID] = events
	}
	if _, ok := c.starting[holdingID]; !ok {
		if c.repository == nil {
			return nil, nil, &domain.Error{Code: domain.ErrIntegrity, Message: "starting cost is missing from gain snapshot"}
		}
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
			if event.Before(*cutoff) {
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
		boundary := event.Boundary()
		resolved, err := c.replay(ctx, *event.SourceHoldingID, &boundary)
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
