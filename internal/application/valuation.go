package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ValuationService is the sole authoritative reader for current native/base
// values. It consumes persisted observations from one PortfolioSnapshot and
// never performs network I/O.
type ValuationService struct {
	repository    Repository
	now           func() time.Time
	fxProviderKey func() string
}

func NewValuationService(repository Repository, clocks ...func() time.Time) *ValuationService {
	now := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		now = clocks[0]
	}
	return &ValuationService{repository: repository, now: now}
}

// SetFXProviderKey wires the current application-level provider selection
// into valuation without making valuation perform any provider or registry
// work. A nil callback preserves the source-kind-only behavior useful for
// historical and isolated domain callers.
func (v *ValuationService) SetFXProviderKey(providerKey func() string) {
	v.fxProviderKey = providerKey
}

func (v *ValuationService) ValueAccounts(snapshot domain.PortfolioSnapshot) ([]domain.AccountValuation, []domain.MissingInputView, error) {
	valued, missing, err := v.evaluate(snapshot)
	if err != nil {
		return nil, nil, err
	}
	accounts := make([]domain.AccountValuation, 0, len(valued))
	for _, item := range valued {
		accounts = append(accounts, item.model)
	}
	return accounts, missing, nil
}

func (v *ValuationService) Account(ctx context.Context, id domain.AccountID) (domain.AccountValuation, error) {
	snapshot, err := v.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.AccountValuation{}, err
	}
	valued, _, err := v.evaluate(snapshot)
	if err != nil {
		return domain.AccountValuation{}, err
	}
	for _, item := range valued {
		if item.model.Account.ID == id {
			return item.model, nil
		}
	}
	return domain.AccountValuation{}, &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
}

func (v *ValuationService) Portfolio(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioValuation, error) {
	snapshot, err := v.repository.ReadPortfolioSnapshot(ctx, filter)
	if err != nil {
		return domain.PortfolioValuation{}, err
	}
	return v.PortfolioSnapshot(snapshot)
}

func (v *ValuationService) PortfolioSnapshot(snapshot domain.PortfolioSnapshot) (domain.PortfolioValuation, error) {
	if snapshot.Household == nil {
		return domain.PortfolioValuation{}, nil
	}
	valued, _, err := v.evaluate(snapshot)
	if err != nil {
		return domain.PortfolioValuation{}, err
	}
	portfolio := domain.PortfolioValuation{Currency: snapshot.Household.BaseCurrency, Complete: true}
	valuesByCurrency := map[string]decimal.Decimal{}
	valuesByCountry := map[string]decimal.Decimal{}
	valuesByType := map[string]decimal.Decimal{}
	labelsByCurrency := map[string]string{}
	labelsByCountry := map[string]string{}
	labelsByType := map[string]string{}
	total := decimal.Zero
	hasValue := false
	for _, item := range valued {
		account := item.model.Account
		if !account.IncludeInPortfolio || account.IsLiability() {
			continue
		}
		portfolio.Accounts = append(portfolio.Accounts, item.model)
		if !item.model.Complete {
			portfolio.Complete = false
		}
		if item.hasBase {
			total = total.Add(item.baseExact)
			hasValue = true
		}
		for _, component := range item.model.Components {
			if component.BaseAmount == nil || component.BaseAmountExact == "" {
				continue
			}
			baseAmount, parseErr := decimal.NewFromString(component.BaseAmountExact)
			if parseErr != nil {
				return domain.PortfolioValuation{}, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "stored valuation amount is invalid"}
			}
			currencyKey := component.NativeCurrency.String()
			valuesByCurrency[currencyKey] = valuesByCurrency[currencyKey].Add(baseAmount)
			labelsByCurrency[currencyKey] = currencyKey
			countryKey, countryLabel := "unknown", "Unknown"
			typeKey, typeLabel := "manual", "Manual"
			var instrument *domain.Instrument
			if component.InstrumentID != nil {
				if found, ok := instrumentByID(snapshot.Instruments, *component.InstrumentID); ok {
					copied := found
					instrument = &copied
					if instrument.CountryCode != nil {
						countryKey, countryLabel = *instrument.CountryCode, *instrument.CountryCode
					}
				}
			}
			cash := account.TrackingMode == domain.TrackingHoldings && component.InstrumentID == nil
			class, classErr := domain.ClassifyAccountComponent(account, instrument, cash)
			if classErr != nil {
				return domain.PortfolioValuation{}, classErr
			}
			if class.MissingInstrument {
				portfolio.Complete = false
				continue
			}
			if class.Bucket != "" {
				typeKey, typeLabel = class.Bucket, class.Bucket
			}
			valuesByCountry[countryKey] = valuesByCountry[countryKey].Add(baseAmount)
			labelsByCountry[countryKey] = countryLabel
			valuesByType[typeKey] = valuesByType[typeKey].Add(baseAmount)
			labelsByType[typeKey] = typeLabel
		}
	}
	portfolio.MissingInputs = missingForAccounts(portfolio.Accounts)
	if hasValue {
		view, err := moneyView(total, snapshot.Household.BaseCurrency)
		if err != nil {
			return domain.PortfolioValuation{}, err
		}
		portfolio.ValuedSubtotal = &view
	} else if len(portfolio.MissingInputs) == 0 {
		view, err := moneyView(decimal.Zero, snapshot.Household.BaseCurrency)
		if err != nil {
			return domain.PortfolioValuation{}, err
		}
		portfolio.ValuedSubtotal = &view
	}
	denominator := total
	portfolio.ByCurrency, err = makeAllocations(valuesByCurrency, labelsByCurrency, denominator, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.PortfolioValuation{}, err
	}
	portfolio.ByCountry, err = makeAllocations(valuesByCountry, labelsByCountry, denominator, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.PortfolioValuation{}, err
	}
	portfolio.ByInstrumentType, err = makeAllocations(valuesByType, labelsByType, denominator, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.PortfolioValuation{}, err
	}
	return portfolio, nil
}

type valuedAccount struct {
	model     domain.AccountValuation
	baseExact decimal.Decimal
	hasBase   bool
}

func (v *ValuationService) evaluate(snapshot domain.PortfolioSnapshot) ([]valuedAccount, []domain.MissingInputView, error) {
	if snapshot.Household == nil {
		return nil, nil, nil
	}
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	holdingsByAccount := make(map[domain.AccountID][]domain.Holding)
	for _, holding := range snapshot.Holdings {
		if holding.ArchivedAt == nil {
			holdingsByAccount[holding.AccountID] = append(holdingsByAccount[holding.AccountID], holding)
		}
	}
	cashByAccount := make(map[domain.AccountID][]domain.AccountCashValue)
	for _, cash := range snapshot.CashValues {
		cashByAccount[cash.AccountID] = append(cashByAccount[cash.AccountID], cash)
	}
	valued := make([]valuedAccount, 0, len(snapshot.Accounts))
	allMissing := make([]domain.MissingInputView, 0)
	for _, record := range snapshot.Accounts {
		if record.Account.ArchivedAt != nil {
			continue
		}
		item, err := v.valueAccount(snapshot, record, instruments, holdingsByAccount[record.Account.ID], cashByAccount[record.Account.ID])
		if err != nil {
			return nil, nil, err
		}
		valued = append(valued, item)
		allMissing = append(allMissing, item.model.MissingInputs...)
	}
	sortMissing(allMissing)
	return valued, deduplicateMissing(allMissing), nil
}

func (v *ValuationService) valueAccount(snapshot domain.PortfolioSnapshot, record domain.AccountRecord, instruments map[domain.InstrumentID]domain.Instrument, holdings []domain.Holding, cash []domain.AccountCashValue) (valuedAccount, error) {
	model := domain.AccountValuation{Account: record.Account, Ownership: record.Ownership, InstitutionName: record.InstitutionName, GroupName: record.GroupName, Complete: true}
	baseTotal := decimal.Zero
	hasBase := false
	add := func(component domain.ValuationComponent, missing []domain.MissingInputView) {
		component.StateObservationID = record.StateObservationID
		model.Components = append(model.Components, component)
		model.MissingInputs = append(model.MissingInputs, missing...)
		if !component.Available {
			model.Complete = false
		}
		if component.BaseAmountExact != "" {
			if value, err := decimal.NewFromString(component.BaseAmountExact); err == nil {
				baseTotal = baseTotal.Add(value)
				hasBase = true
			}
		}
	}
	if record.Account.TrackingMode == domain.TrackingHoldings {
		sort.Slice(holdings, func(i, j int) bool {
			if holdings[i].SortOrder != holdings[j].SortOrder {
				return holdings[i].SortOrder < holdings[j].SortOrder
			}
			return holdings[i].ID.String() < holdings[j].ID.String()
		})
		for _, holding := range holdings {
			if holding.ArchivedAt != nil {
				continue
			}
			instrument, ok := instruments[holding.InstrumentID]
			// Archiving an Instrument stops new holdings and quote writes, but
			// does not erase an active retained Holding from valuation/history.
			if !ok {
				missing := domain.MissingInputView{Kind: domain.MissingInstrument, AccountID: record.Account.ID, InstrumentID: &holding.InstrumentID}
				add(domain.ValuationComponent{AccountID: record.Account.ID, HoldingID: &holding.ID, InstrumentID: &holding.InstrumentID, NativeCurrency: record.Account.DefaultCurrency, Available: false}, []domain.MissingInputView{missing})
				continue
			}
			component, missing, err := v.valueHolding(snapshot, record.Account.ID, holding, instrument)
			if err != nil {
				return valuedAccount{}, err
			}
			add(component, missing)
		}
		for _, value := range latestCashValues(cash) {
			component, missing, err := v.valueCash(snapshot, record.Account.ID, value)
			if err != nil {
				return valuedAccount{}, err
			}
			add(component, missing)
		}
	} else if record.LatestValue == nil {
		missing := domain.MissingInputView{Kind: domain.MissingAccountValue, AccountID: record.Account.ID, QuoteCurrency: record.Account.DefaultCurrency}
		add(domain.ValuationComponent{AccountID: record.Account.ID, NativeCurrency: record.Account.DefaultCurrency, Available: false}, []domain.MissingInputView{missing})
	} else {
		component, missing, err := v.valueNative(snapshot, record.Account.ID, nil, record.LatestValue.Amount.Amount(), record.LatestValue.Amount.Currency(), nil)
		if err != nil {
			return valuedAccount{}, err
		}
		component.StateObservationID = record.StateObservationID
		add(component, missing)
	}
	sortMissing(model.MissingInputs)
	model.MissingInputs = deduplicateMissing(model.MissingInputs)
	if hasBase {
		view, err := moneyView(baseTotal, snapshot.Household.BaseCurrency)
		if err != nil {
			return valuedAccount{}, err
		}
		model.BaseValue = &view
	} else if model.Complete {
		view, err := moneyView(decimal.Zero, snapshot.Household.BaseCurrency)
		if err != nil {
			return valuedAccount{}, err
		}
		model.BaseValue = &view
	}
	return valuedAccount{model: model, baseExact: baseTotal, hasBase: hasBase}, nil
}

func (v *ValuationService) valueHolding(snapshot domain.PortfolioSnapshot, accountID domain.AccountID, holding domain.Holding, instrument domain.Instrument) (domain.ValuationComponent, []domain.MissingInputView, error) {
	id := instrument.ID
	name, symbol := instrumentIdentity(instrument)
	if holding.Quantity.IsZero() {
		baseView, err := moneyView(decimal.Zero, snapshot.Household.BaseCurrency)
		if err != nil {
			return domain.ValuationComponent{}, nil, err
		}
		return domain.ValuationComponent{
			AccountID: accountID, HoldingID: &holding.ID, InstrumentID: &id, InstrumentName: name, InstrumentSymbol: symbol,
			NativeAmount: "0", NativeCurrency: instrument.QuoteCurrency, BaseAmount: &baseView,
			PreferenceObservationID: instrument.PreferenceObservationID,
			BaseAmountExact:         "0", Available: true,
		}, nil, nil
	}
	quote := selectInstrumentQuote(instrument, snapshot.InstrumentQuotes)
	if quote == nil {
		return domain.ValuationComponent{AccountID: accountID, HoldingID: &holding.ID, InstrumentID: &id, InstrumentName: name, InstrumentSymbol: symbol, PreferenceObservationID: instrument.PreferenceObservationID, NativeCurrency: instrument.QuoteCurrency, Available: false}, []domain.MissingInputView{{Kind: domain.MissingInstrumentPrice, AccountID: accountID, InstrumentID: &id, InstrumentName: name, InstrumentSymbol: symbol, QuoteCurrency: instrument.QuoteCurrency}}, nil
	}
	native, err := holding.Quantity.Multiply(quote.UnitPrice)
	if err != nil {
		return domain.ValuationComponent{}, nil, err
	}
	evidence := quoteEvidence(quote.ID.String(), quote.SourceKind, quote.SourceKey, quote.QuotedAt, quote.Delayed, v.now())
	component, missing, err := v.valueNative(snapshot, accountID, &holding.InstrumentID, native, instrument.QuoteCurrency, &evidence)
	if err != nil {
		return domain.ValuationComponent{}, nil, err
	}
	component.InstrumentName = name
	component.InstrumentSymbol = symbol
	holdingID := holding.ID
	component.HoldingID = &holdingID
	component.PreferenceObservationID = instrument.PreferenceObservationID
	return component, missing, nil
}

func instrumentIdentity(instrument domain.Instrument) (string, string) {
	symbol := ""
	if instrument.Symbol != nil {
		symbol = strings.TrimSpace(*instrument.Symbol)
	}
	return strings.TrimSpace(instrument.Name), symbol
}

func (v *ValuationService) valueCash(snapshot domain.PortfolioSnapshot, accountID domain.AccountID, cash domain.AccountCashValue) (domain.ValuationComponent, []domain.MissingInputView, error) {
	return v.valueNative(snapshot, accountID, nil, cash.Amount.Amount(), cash.Amount.Currency(), nil)
}

func (v *ValuationService) valueNative(snapshot domain.PortfolioSnapshot, accountID domain.AccountID, instrumentID *domain.InstrumentID, native decimal.Decimal, currency domain.CurrencyCode, priceEvidence *domain.QuoteEvidenceView) (domain.ValuationComponent, []domain.MissingInputView, error) {
	component := domain.ValuationComponent{AccountID: accountID, InstrumentID: cloneInstrumentID(instrumentID), NativeAmount: native.String(), NativeCurrency: currency, PriceEvidence: priceEvidence, Available: true}
	if preference := findFXPreference(snapshot.FXPreferences, currency, snapshot.Household.BaseCurrency); preference != nil {
		component.FXPreferenceObservationID = preference.ObservationID
	}
	base, fxEvidence, missing, err := v.convert(snapshot, accountID, native, currency)
	if err != nil {
		return domain.ValuationComponent{}, nil, err
	}
	if missing != nil {
		component.Available = false
		return component, []domain.MissingInputView{*missing}, nil
	}
	component.FXEvidence = fxEvidence
	component.BaseAmountExact = base.String()
	view, err := moneyView(base, snapshot.Household.BaseCurrency)
	if err != nil {
		return domain.ValuationComponent{}, nil, err
	}
	component.BaseAmount = &view
	return component, nil, nil
}

func (v *ValuationService) convert(snapshot domain.PortfolioSnapshot, accountID domain.AccountID, native decimal.Decimal, currency domain.CurrencyCode) (decimal.Decimal, *domain.QuoteEvidenceView, *domain.MissingInputView, error) {
	baseCurrency := snapshot.Household.BaseCurrency
	if currency == baseCurrency {
		return native, nil, nil, nil
	}
	preference := findFXPreference(snapshot.FXPreferences, currency, baseCurrency)
	if preference == nil {
		return decimal.Zero, nil, &domain.MissingInputView{Kind: domain.MissingFXRate, AccountID: accountID, BaseCurrency: baseCurrency, QuoteCurrency: currency}, nil
	}
	providerKey := ""
	if preference.SourceKind == domain.QuoteSourceProvider && v.fxProviderKey != nil {
		providerKey = strings.ToLower(strings.TrimSpace(v.fxProviderKey()))
	}
	quote := selectFXQuote(*preference, snapshot.FXQuotes, currency, baseCurrency, providerKey)
	if quote == nil {
		return decimal.Zero, nil, &domain.MissingInputView{Kind: domain.MissingFXRate, AccountID: accountID, BaseCurrency: baseCurrency, QuoteCurrency: currency}, nil
	}
	var converted decimal.Decimal
	var err error
	if quote.BaseCurrency == currency && quote.QuoteCurrency == baseCurrency {
		converted, err = domain.MultiplyByFxRate(native, quote.Rate)
	} else {
		converted, err = domain.DivideByFxRate(native, quote.Rate)
	}
	if err != nil {
		return decimal.Zero, nil, nil, err
	}
	evidence := quoteEvidence(quote.ID.String(), quote.SourceKind, quote.SourceKey, quote.QuotedAt, quote.Delayed, v.now())
	return converted, &evidence, nil, nil
}

func selectInstrumentQuote(instrument domain.Instrument, quotes []domain.InstrumentQuote) *domain.InstrumentQuote {
	var selected *domain.InstrumentQuote
	for index := range quotes {
		quote := &quotes[index]
		if quote.InstrumentID != instrument.ID || quote.SourceKind != instrument.QuoteSource || quote.Currency != instrument.QuoteCurrency {
			continue
		}
		if selected == nil || quoteLater(quote.QuotedAt, quote.CreatedAt, quote.ID.String(), selected.QuotedAt, selected.CreatedAt, selected.ID.String()) {
			selected = quote
		}
	}
	return selected
}

func selectFXQuote(preference domain.FXPreference, quotes []domain.FXQuote, native, householdBase domain.CurrencyCode, providerKeys ...string) *domain.FXQuote {
	providerKey := ""
	if len(providerKeys) > 0 {
		providerKey = strings.ToLower(strings.TrimSpace(providerKeys[0]))
	}
	var selected *domain.FXQuote
	for index := range quotes {
		quote := &quotes[index]
		if quote.HouseholdID != preference.HouseholdID || quote.SourceKind != preference.SourceKind {
			continue
		}
		if preference.SourceKind == domain.QuoteSourceProvider && providerKey != "" && strings.ToLower(strings.TrimSpace(quote.SourceKey)) != providerKey {
			continue
		}
		if !((quote.BaseCurrency == native && quote.QuoteCurrency == householdBase) || (quote.BaseCurrency == householdBase && quote.QuoteCurrency == native)) {
			continue
		}
		if selected == nil || quoteLater(quote.QuotedAt, quote.CreatedAt, quote.ID.String(), selected.QuotedAt, selected.CreatedAt, selected.ID.String()) {
			selected = quote
		}
	}
	return selected
}

func findFXPreference(preferences []domain.FXPreference, first, second domain.CurrencyCode) *domain.FXPreference {
	a, b, err := domain.NormalizeFXPair(first, second)
	if err != nil {
		return nil
	}
	for index := range preferences {
		if preferences[index].CurrencyA == a && preferences[index].CurrencyB == b {
			return &preferences[index]
		}
	}
	return nil
}

func quoteLater(quotedAt, createdAt time.Time, id string, otherQuotedAt, otherCreatedAt time.Time, otherID string) bool {
	if !quotedAt.Equal(otherQuotedAt) {
		return quotedAt.After(otherQuotedAt)
	}
	if !createdAt.Equal(otherCreatedAt) {
		return createdAt.After(otherCreatedAt)
	}
	return id > otherID
}

func quoteEvidence(id string, source domain.QuoteSourceKind, key string, quotedAt time.Time, delayed bool, now time.Time) domain.QuoteEvidenceView {
	return domain.QuoteEvidenceView{ObservationID: id, Source: source, SourceKey: key, QuotedAt: quotedAt, Freshness: domain.QuoteFreshness(source, delayed, quotedAt, now), Delayed: delayed}
}

func latestCashValues(values []domain.AccountCashValue) []domain.AccountCashValue {
	latest := map[domain.CurrencyCode]domain.AccountCashValue{}
	for _, value := range values {
		current, ok := latest[value.Amount.Currency()]
		if !ok || quoteLater(value.EffectiveAt, value.CreatedAt, value.ID.String(), current.EffectiveAt, current.CreatedAt, current.ID.String()) {
			latest[value.Amount.Currency()] = value
		}
	}
	result := make([]domain.AccountCashValue, 0, len(latest))
	for _, value := range latest {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Amount.Currency() != result[j].Amount.Currency() {
			return result[i].Amount.Currency() < result[j].Amount.Currency()
		}
		return result[i].ID.String() < result[j].ID.String()
	})
	return result
}

func moneyView(value decimal.Decimal, currency domain.CurrencyCode) (domain.MoneyView, error) {
	money, err := domain.NewMoney(value, currency)
	if err != nil {
		return domain.MoneyView{}, err
	}
	return domain.MoneyView{Amount: money.CanonicalAmount(), Currency: money.Currency()}, nil
}

func makeAllocations(values map[string]decimal.Decimal, labels map[string]string, denominator decimal.Decimal, currency domain.CurrencyCode) ([]domain.AllocationView, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	shares := make(map[string]int, len(keys))
	type remainder struct {
		key   string
		value decimal.Decimal
	}
	remainders := make([]remainder, 0, len(keys))
	allocated := 0
	if !denominator.IsZero() {
		for _, key := range keys {
			exact := values[key].Mul(decimal.NewFromInt(domain.TotalOwnershipBPS)).Div(denominator)
			floor := int(exact.IntPart())
			shares[key] = floor
			allocated += floor
			remainders = append(remainders, remainder{key: key, value: exact.Sub(decimal.NewFromInt(int64(floor)))})
		}
	}
	remaining := domain.TotalOwnershipBPS - allocated
	sort.SliceStable(remainders, func(i, j int) bool {
		if remainders[i].value.Equal(remainders[j].value) {
			return remainders[i].key < remainders[j].key
		}
		return remainders[i].value.GreaterThan(remainders[j].value)
	})
	for index := 0; index < remaining && index < len(remainders); index++ {
		shares[remainders[index].key]++
	}
	result := make([]domain.AllocationView, 0, len(keys))
	for _, key := range keys {
		amount, err := moneyView(values[key], currency)
		if err != nil {
			return nil, err
		}
		label := labels[key]
		if label == "" {
			label = key
		}
		result = append(result, domain.AllocationView{Key: key, Label: label, Amount: amount, ShareBPS: shares[key]})
	}
	return result, nil
}

func missingForAccounts(accounts []domain.AccountValuation) []domain.MissingInputView {
	missing := make([]domain.MissingInputView, 0)
	for _, account := range accounts {
		missing = append(missing, account.MissingInputs...)
	}
	sortMissing(missing)
	return deduplicateMissing(missing)
}

func sortMissing(missing []domain.MissingInputView) {
	sort.Slice(missing, func(i, j int) bool {
		left, right := missing[i], missing[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.AccountID != right.AccountID {
			return left.AccountID < right.AccountID
		}
		leftInstrument, rightInstrument := "", ""
		if left.InstrumentID != nil {
			leftInstrument = left.InstrumentID.String()
		}
		if right.InstrumentID != nil {
			rightInstrument = right.InstrumentID.String()
		}
		if leftInstrument != rightInstrument {
			return leftInstrument < rightInstrument
		}
		if left.BaseCurrency != right.BaseCurrency {
			return left.BaseCurrency < right.BaseCurrency
		}
		return left.QuoteCurrency < right.QuoteCurrency
	})
}

func deduplicateMissing(missing []domain.MissingInputView) []domain.MissingInputView {
	result := make([]domain.MissingInputView, 0, len(missing))
	seen := map[string]struct{}{}
	for _, item := range missing {
		instrumentID := ""
		if item.InstrumentID != nil {
			instrumentID = item.InstrumentID.String()
		}
		key := strings.Join([]string{string(item.Kind), item.AccountID.String(), instrumentID, item.BaseCurrency.String(), item.QuoteCurrency.String()}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func instrumentByID(instruments []domain.Instrument, id domain.InstrumentID) (domain.Instrument, bool) {
	for _, instrument := range instruments {
		if instrument.ID == id {
			return instrument, true
		}
	}
	return domain.Instrument{}, false
}

func cloneInstrumentID(id *domain.InstrumentID) *domain.InstrumentID {
	if id == nil {
		return nil
	}
	copy := *id
	return &copy
}

func exactBaseAmount(account domain.AccountValuation) (decimal.Decimal, error) {
	total := decimal.Zero
	for _, component := range account.Components {
		if !component.Available || component.BaseAmountExact == "" {
			continue
		}
		value, err := decimal.NewFromString(component.BaseAmountExact)
		if err != nil {
			return decimal.Zero, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "stored valuation amount is invalid"}
		}
		total = total.Add(value)
	}
	return total, nil
}
