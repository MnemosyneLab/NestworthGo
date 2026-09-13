package domain

import (
	"strings"
	"time"
)

// InstrumentType is intentionally closed so the persistence and UI labels do
// not drift apart.
type InstrumentType string

const (
	InstrumentStock                 InstrumentType = "stock"
	InstrumentETF                   InstrumentType = "etf"
	InstrumentMutualFund            InstrumentType = "mutual_fund"
	InstrumentCrypto                InstrumentType = "crypto"
	InstrumentBond                  InstrumentType = "bond"
	InstrumentPreciousMetal         InstrumentType = "precious_metal"
	InstrumentBankInvestmentProduct InstrumentType = "bank_investment_product"
	InstrumentOther                 InstrumentType = "other"
)

func ParseInstrumentType(value string) (InstrumentType, error) {
	typeValue := InstrumentType(strings.TrimSpace(value))
	switch typeValue {
	case InstrumentStock, InstrumentETF, InstrumentMutualFund, InstrumentCrypto,
		InstrumentBond, InstrumentPreciousMetal, InstrumentBankInvestmentProduct, InstrumentOther:
		return typeValue, nil
	default:
		return "", validation("instrumentType", "is not supported")
	}
}

func AllInstrumentTypes() []InstrumentType {
	return []InstrumentType{
		InstrumentStock, InstrumentETF, InstrumentMutualFund, InstrumentCrypto,
		InstrumentBond, InstrumentPreciousMetal, InstrumentBankInvestmentProduct, InstrumentOther,
	}
}

// QuoteSourceKind is explicit. There is no implicit fallback between Manual
// and Provider observations.
type QuoteSourceKind string

const (
	QuoteSourceManual   QuoteSourceKind = "manual"
	QuoteSourceProvider QuoteSourceKind = "provider"
)

func ParseQuoteSourceKind(value string) (QuoteSourceKind, error) {
	source := QuoteSourceKind(strings.TrimSpace(value))
	switch source {
	case QuoteSourceManual, QuoteSourceProvider:
		return source, nil
	default:
		return "", validation("sourceKind", "is not supported")
	}
}

func AllQuoteSourceKinds() []QuoteSourceKind {
	return []QuoteSourceKind{QuoteSourceManual, QuoteSourceProvider}
}

// QuoteSourceFilter is the chart-facing source selector. Empty and "all"
// both mean every locally stored fact; manual and provider keep their
// existing QuoteSourceKind meaning.
type QuoteSourceFilter string

const (
	QuoteSourceFilterAll      QuoteSourceFilter = "all"
	QuoteSourceFilterManual   QuoteSourceFilter = "manual"
	QuoteSourceFilterProvider QuoteSourceFilter = "provider"
)

func ParseQuoteSourceFilter(value string) (QuoteSourceFilter, error) {
	filter := QuoteSourceFilter(strings.TrimSpace(value))
	if filter == "" {
		return QuoteSourceFilterAll, nil
	}
	switch filter {
	case QuoteSourceFilterAll, QuoteSourceFilterManual, QuoteSourceFilterProvider:
		return filter, nil
	default:
		return "", validation("sourceFilter", "quote source filter is not supported")
	}
}

func (filter QuoteSourceFilter) SourceKind() *QuoteSourceKind {
	switch filter {
	case QuoteSourceFilterManual:
		kind := QuoteSourceManual
		return &kind
	case QuoteSourceFilterProvider:
		kind := QuoteSourceProvider
		return &kind
	default:
		return nil
	}
}

// QuoteSeriesPoint is one locally stored observation projected for a chart or
// data table. Value is already canonical; the frontend must not invert FX.
type QuoteSeriesPoint struct {
	ID         string
	QuotedAt   time.Time
	CreatedAt  time.Time
	Value      string
	SourceKind QuoteSourceKind
	SourceKey  string
	Delayed    bool
}

// QuoteSeries is the bounded local-history read model for one instrument or
// one FX pair direction. Points are the chart series; Observations retains
// every matching fact, newest first.
type QuoteSeries struct {
	Range           TrendRange
	DisplayCurrency CurrencyCode
	BaseCurrency    CurrencyCode
	QuoteCurrency   CurrencyCode
	Points          []QuoteSeriesPoint
	Observations    []QuoteSeriesPoint
	// OutsideRange is true when this window has no observations but matching
	// local facts exist outside it. The frontend uses this for "show all".
	OutsideRange bool
}

// Instrument is a household-scoped quoted asset. Provider metadata is kept as
// opaque application data; the domain never knows a Yahoo response shape.
type Instrument struct {
	ID                      InstrumentID
	HouseholdID             HouseholdID
	Name                    string
	Type                    InstrumentType
	QuoteCurrency           CurrencyCode
	Symbol                  *string
	MarketCode              *string
	CountryCode             *string
	ISIN                    *string
	Note                    *string
	IconKey                 *string
	SortOrder               int
	QuoteSource             QuoteSourceKind
	ProviderKey             *string
	ProviderSymbol          *string
	ProviderBindingRevision int
	PreferenceObservationID *InstrumentPreferenceObservationID
	CreatedAt               time.Time
	UpdatedAt               time.Time
	ArchivedAt              *time.Time
}

type InstrumentInput struct {
	HouseholdID    HouseholdID
	Name           string
	Type           InstrumentType
	QuoteCurrency  CurrencyCode
	Symbol         *string
	MarketCode     *string
	CountryCode    *string
	ISIN           *string
	Note           *string
	IconKey        *string
	SortOrder      int
	QuoteSource    QuoteSourceKind
	ProviderKey    *string
	ProviderSymbol *string
}

// SupportedInstrumentCountryCodes is the user-facing country/region catalog
// for instruments. It follows the geographic coverage of SupportedCurrencies;
// EU is a region because EUR-denominated instruments are not country-specific.
func SupportedInstrumentCountryCodes() []string {
	return []string{"AU", "CN", "EU", "GB", "HK", "JP", "SG", "TW", "US", "KR", "CH"}
}

// SupportedInstrumentMarketCodes covers the main exchanges for the currencies
// accepted by the app. Instrument persistence remains syntax-compatible with
// older custom codes, while create/edit surfaces use this curated catalog.
func SupportedInstrumentMarketCodes() []string {
	return []string{"ASX", "SSE", "SZSE", "BSE", "EURONEXT", "XETRA", "LSE", "HKEX", "TSE", "SGX", "TWSE", "NASDAQ", "NYSE", "AMEX", "KRX", "SIX"}
}

func NewInstrument(input InstrumentInput, now time.Time) (Instrument, error) {
	name, err := validateName("name", input.Name)
	if err != nil {
		return Instrument{}, err
	}
	instrumentType, err := ParseInstrumentType(string(input.Type))
	if err != nil {
		return Instrument{}, err
	}
	iconKey, err := normalizedIconKey(input.IconKey, DefaultInstrumentIcon(instrumentType))
	if err != nil {
		return Instrument{}, err
	}
	currency, err := ParseCurrency(input.QuoteCurrency.String())
	if err != nil {
		return Instrument{}, validation("quoteCurrency", "must be a valid currency")
	}
	quoteSource := input.QuoteSource
	if quoteSource == "" {
		quoteSource = QuoteSourceManual
	}
	quoteSource, err = ParseQuoteSourceKind(string(quoteSource))
	if err != nil {
		return Instrument{}, err
	}
	symbol, err := normalizePortfolioText("symbol", input.Symbol, 64, true)
	if err != nil {
		return Instrument{}, err
	}
	marketCode, err := normalizePortfolioText("marketCode", input.MarketCode, 32, true)
	if err != nil {
		return Instrument{}, err
	}
	if marketCode != nil {
		value := strings.ToUpper(*marketCode)
		marketCode = &value
	}
	countryCode, err := normalizeCountryCode(input.CountryCode)
	if err != nil {
		return Instrument{}, err
	}
	isin, err := normalizePortfolioText("isin", input.ISIN, 32, true)
	if err != nil {
		return Instrument{}, err
	}
	if isin != nil {
		value := strings.ToUpper(*isin)
		isin = &value
	}
	note, err := validateNote("note", input.Note)
	if err != nil {
		return Instrument{}, err
	}
	providerKey, err := normalizePortfolioText("providerKey", input.ProviderKey, 64, false)
	if err != nil {
		return Instrument{}, err
	}
	if providerKey != nil {
		value := strings.ToLower(*providerKey)
		providerKey = &value
	}
	providerSymbol, err := normalizePortfolioText("providerSymbol", input.ProviderSymbol, 128, true)
	if err != nil {
		return Instrument{}, err
	}
	if providerSymbol != nil {
		value := strings.ToUpper(*providerSymbol)
		providerSymbol = &value
	}
	if quoteSource == QuoteSourceProvider && (providerKey == nil || providerSymbol == nil) {
		return Instrument{}, validation("provider", "provider key and symbol are required when Provider is selected")
	}
	now = normalizeTime(now)
	return Instrument{
		ID: NewInstrumentID(), HouseholdID: input.HouseholdID, Name: name, Type: instrumentType,
		QuoteCurrency: currency, Symbol: symbol, MarketCode: marketCode, CountryCode: countryCode,
		ISIN: isin, Note: note, IconKey: iconKey, SortOrder: input.SortOrder,
		QuoteSource: quoteSource, ProviderKey: providerKey, ProviderSymbol: providerSymbol,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (i Instrument) UsesProvider() bool { return i.QuoteSource == QuoteSourceProvider }

// Holding stores the current quantity, not an activity or trade history.
type Holding struct {
	ID           HoldingID
	AccountID    AccountID
	InstrumentID InstrumentID
	Quantity     Quantity
	Note         *string
	SortOrder    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ArchivedAt   *time.Time
}

type HoldingInput struct {
	AccountID    AccountID
	InstrumentID InstrumentID
	Quantity     Quantity
	Note         *string
	SortOrder    int
}

func NewHolding(input HoldingInput, now time.Time) (Holding, error) {
	if _, err := ParseAccountID(input.AccountID.String()); err != nil {
		return Holding{}, validation("accountId", "must be a valid account ID")
	}
	if _, err := ParseInstrumentID(input.InstrumentID.String()); err != nil {
		return Holding{}, validation("instrumentId", "must be a valid instrument ID")
	}
	note, err := validateNote("note", input.Note)
	if err != nil {
		return Holding{}, err
	}
	now = normalizeTime(now)
	return Holding{ID: NewHoldingID(), AccountID: input.AccountID, InstrumentID: input.InstrumentID, Quantity: input.Quantity, Note: note, SortOrder: input.SortOrder, CreatedAt: now, UpdatedAt: now}, nil
}

func NewHoldingForAccount(account Account, instrument Instrument, quantity Quantity, note *string, sortOrder int, now time.Time) (Holding, error) {
	if account.TrackingMode != TrackingHoldings {
		return Holding{}, validation("trackingMode", "Holdings must belong to a Holdings account")
	}
	if account.AccountType == TypeCashOnHand {
		return Holding{}, validation("accountType", "Cash on hand accounts can only contain cash balances")
	}
	if account.HouseholdID != instrument.HouseholdID {
		return Holding{}, validation("householdId", "account and instrument must belong to the same Household")
	}
	return NewHolding(HoldingInput{AccountID: account.ID, InstrumentID: instrument.ID, Quantity: quantity, Note: note, SortOrder: sortOrder}, now)
}

func (h Holding) ReplaceQuantity(quantity Quantity, now time.Time) Holding {
	h.Quantity = quantity
	h.UpdatedAt = normalizeTime(now)
	return h
}

// AccountCashValue is an append-only cash observation for a Holdings Account.
type AccountCashValue struct {
	ID          AccountCashValueID
	AccountID   AccountID
	Amount      Money
	EffectiveAt time.Time
	CreatedAt   time.Time
}

func NewAccountCashValue(account Account, amount Money, effectiveAt, createdAt time.Time) (AccountCashValue, error) {
	if account.TrackingMode != TrackingHoldings {
		return AccountCashValue{}, validation("trackingMode", "cash observations require a Holdings account")
	}
	if amount.Amount().IsNegative() {
		return AccountCashValue{}, validation("amount", "cash must not be negative")
	}
	if amount.Currency() == "" {
		return AccountCashValue{}, validation("amount", "currency is required")
	}
	return AccountCashValue{ID: NewAccountCashValueID(), AccountID: account.ID, Amount: amount, EffectiveAt: normalizeTime(effectiveAt), CreatedAt: normalizeTime(createdAt)}, nil
}

// InstrumentQuote is immutable history. Selection is handled by the
// application/valuation layer using QuoteSource and timestamps.
type InstrumentQuote struct {
	ID                  InstrumentQuoteID
	InstrumentID        InstrumentID
	UnitPrice           UnitPrice
	Currency            CurrencyCode
	SourceKind          QuoteSourceKind
	SourceKey           string
	QuotedAt            time.Time
	CreatedAt           time.Time
	Delayed             bool
	ObservationKind     string
	EffectiveDate       string
	ProviderTimestamp   time.Time
	FetchedAt           time.Time
	ValueEffectiveAt    time.Time
	BindingRevision     int
	SourcePolicyVersion string
	PriceBasis          string
	TimestampBasis      string
	Revision            int
	SupersedesQuoteID   *string
	SplitFactor         string
	DividendCash        string
}

type InstrumentQuoteInput struct {
	UnitPrice  UnitPrice
	Currency   CurrencyCode
	SourceKind QuoteSourceKind
	SourceKey  string
	QuotedAt   time.Time
	Delayed    bool
}

func NewInstrumentQuote(instrument Instrument, input InstrumentQuoteInput, createdAt time.Time) (InstrumentQuote, error) {
	source, sourceKey, err := normalizeQuoteSource(input.SourceKind, input.SourceKey)
	if err != nil {
		return InstrumentQuote{}, err
	}
	currency := instrument.QuoteCurrency
	if input.Currency != "" {
		currency, err = ParseCurrency(input.Currency.String())
		if err != nil {
			return InstrumentQuote{}, validation("currency", "must be a valid currency")
		}
	}
	if currency != instrument.QuoteCurrency {
		return InstrumentQuote{}, validation("currency", "must match instrument quote currency")
	}
	return InstrumentQuote{ID: NewInstrumentQuoteID(), InstrumentID: instrument.ID, UnitPrice: input.UnitPrice, Currency: currency, SourceKind: source, SourceKey: sourceKey, QuotedAt: normalizeTime(input.QuotedAt), CreatedAt: normalizeTime(createdAt), Delayed: input.Delayed}, nil
}

// FXQuote stores an oriented observation: 1 BaseCurrency = Rate
// QuoteCurrency. The pair preference is normalized separately.
type FXQuote struct {
	ID                  FXQuoteID
	HouseholdID         HouseholdID
	BaseCurrency        CurrencyCode
	QuoteCurrency       CurrencyCode
	Rate                FxRate
	SourceKind          QuoteSourceKind
	SourceKey           string
	QuotedAt            time.Time
	CreatedAt           time.Time
	Delayed             bool
	ObservationKind     string
	EffectiveDate       string
	FetchedAt           time.Time
	ValueEffectiveAt    time.Time
	SourcePolicyVersion string
	TimestampBasis      string
	Revision            int
	SupersedesQuoteID   *string
}

type FXQuoteInput struct {
	HouseholdID   HouseholdID
	BaseCurrency  CurrencyCode
	QuoteCurrency CurrencyCode
	Rate          FxRate
	SourceKind    QuoteSourceKind
	SourceKey     string
	QuotedAt      time.Time
	Delayed       bool
}

func NewFXQuote(input FXQuoteInput, createdAt time.Time) (FXQuote, error) {
	base, err := ParseCurrency(input.BaseCurrency.String())
	if err != nil {
		return FXQuote{}, validation("baseCurrency", "must be a valid currency")
	}
	quote, err := ParseCurrency(input.QuoteCurrency.String())
	if err != nil {
		return FXQuote{}, validation("quoteCurrency", "must be a valid currency")
	}
	if base == quote {
		return FXQuote{}, validation("currencyPair", "base and quote currencies must differ")
	}
	source, sourceKey, err := normalizeQuoteSource(input.SourceKind, input.SourceKey)
	if err != nil {
		return FXQuote{}, err
	}
	return FXQuote{ID: NewFXQuoteID(), HouseholdID: input.HouseholdID, BaseCurrency: base, QuoteCurrency: quote, Rate: input.Rate, SourceKind: source, SourceKey: sourceKey, QuotedAt: normalizeTime(input.QuotedAt), CreatedAt: normalizeTime(createdAt), Delayed: input.Delayed}, nil
}

// FXPreference is Household-scoped and keyed by a canonical unordered pair.
// It intentionally stores no rate or orientation.
type FXPreference struct {
	HouseholdID   HouseholdID
	CurrencyA     CurrencyCode
	CurrencyB     CurrencyCode
	SourceKind    QuoteSourceKind
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ObservationID *FXPreferenceObservationID
}

func NewFXPreference(householdID HouseholdID, currencyA, currencyB CurrencyCode, source QuoteSourceKind, now time.Time) (FXPreference, error) {
	a, b, err := NormalizeFXPair(currencyA, currencyB)
	if err != nil {
		return FXPreference{}, err
	}
	parsedSource, err := ParseQuoteSourceKind(string(source))
	if err != nil {
		return FXPreference{}, err
	}
	now = normalizeTime(now)
	return FXPreference{HouseholdID: householdID, CurrencyA: a, CurrencyB: b, SourceKind: parsedSource, CreatedAt: now, UpdatedAt: now}, nil
}

func NormalizeFXPair(currencyA, currencyB CurrencyCode) (CurrencyCode, CurrencyCode, error) {
	a, err := ParseCurrency(currencyA.String())
	if err != nil {
		return "", "", validation("currencyPair", "first currency is invalid")
	}
	b, err := ParseCurrency(currencyB.String())
	if err != nil {
		return "", "", validation("currencyPair", "second currency is invalid")
	}
	if a == b {
		return "", "", validation("currencyPair", "currencies must differ")
	}
	if a > b {
		a, b = b, a
	}
	return a, b, nil
}

func normalizeQuoteSource(source QuoteSourceKind, sourceKey string) (QuoteSourceKind, string, error) {
	if source == "" {
		source = QuoteSourceManual
	}
	parsed, err := ParseQuoteSourceKind(string(source))
	if err != nil {
		return "", "", err
	}
	sourceKey = strings.TrimSpace(sourceKey)
	if parsed == QuoteSourceManual {
		if sourceKey == "" {
			sourceKey = string(QuoteSourceManual)
		}
	} else if sourceKey == "" {
		return "", "", validation("sourceKey", "is required for provider observations")
	}
	if len([]rune(sourceKey)) > 64 {
		return "", "", validation("sourceKey", "must be 64 characters or fewer")
	}
	return parsed, sourceKey, nil
}

func normalizePortfolioText(field string, value *string, maxRunes int, upper bool) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > maxRunes {
		return nil, validation(field, "is too long")
	}
	if upper {
		trimmed = strings.ToUpper(trimmed)
	}
	return &trimmed, nil
}

func normalizeCountryCode(value *string) (*string, error) {
	country, err := normalizePortfolioText("countryCode", value, 2, true)
	if err != nil || country == nil {
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	for _, char := range *country {
		if char < 'A' || char > 'Z' {
			return nil, validation("countryCode", "must contain two uppercase letters")
		}
	}
	if len(*country) != 2 {
		return nil, validation("countryCode", "must contain two uppercase letters")
	}
	return country, nil
}
