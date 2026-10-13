package domain

import "time"

type MarketReadTarget struct {
	InstrumentID  string `json:"instrumentId,omitempty"`
	BaseCurrency  string `json:"baseCurrency,omitempty"`
	QuoteCurrency string `json:"quoteCurrency,omitempty"`
}

func (t MarketReadTarget) Key() string {
	if t.InstrumentID != "" {
		return "instrument/" + t.InstrumentID
	}
	return "fx/" + t.BaseCurrency + "/" + t.QuoteCurrency
}

type MarketReadRequest struct {
	Targets      []MarketReadTarget `json:"targets"`
	View         string             `json:"view,omitempty"`
	Range        string             `json:"range,omitempty"`
	SourceFilter string             `json:"sourceFilter,omitempty"`
}
type MarketReadSnapshot struct {
	Household               *Household
	Origin                  *HistoryOrigin
	Instruments             []Instrument
	CurrentInstrumentQuotes []InstrumentQuote
	CurrentFXQuotes         []FXQuote
	FXPreferences           []FXPreference
	InstrumentHistory       map[string][]InstrumentQuote
	FXHistory               map[string][]FXQuote
	OutsideRange            map[string]bool
}

type QuoteReadWindow struct {
	From *time.Time
	To   *time.Time
	// Civil-date bounds apply to daily observations; intraday data keeps From/To.
	FromDate   string
	ToDate     string
	SourceKind *QuoteSourceKind
	CurrencyA  CurrencyCode
	CurrencyB  CurrencyCode
	Limit      int
	Ascending  bool
}

func NewQuoteReadWindow(trendRange TrendRange, sourceFilter QuoteSourceFilter, now time.Time, location *time.Location, currencyA, currencyB CurrencyCode) QuoteReadWindow {
	query := QuoteReadWindow{SourceKind: sourceFilter.SourceKind(), CurrencyA: currencyA, CurrencyB: currencyB}
	if location == nil {
		location = time.UTC
	}
	if fromDate, toDate, ok := trendRange.DateBounds(); ok {
		from, _ := time.ParseInLocation("2006-01-02", fromDate, location)
		to, _ := time.ParseInLocation("2006-01-02", toDate, location)
		to = to.AddDate(0, 0, 1).Add(-time.Nanosecond)
		query.From, query.To = &from, &to
		query.FromDate, query.ToDate = fromDate, toDate
		return query
	}
	localNow := now.In(location)
	year, month, day := localNow.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, location)
	switch trendRange {
	case Trend30Days:
		from := today.AddDate(0, 0, -29)
		query.From = &from
		query.FromDate = from.Format(time.DateOnly)
	case TrendYearToDate:
		from := time.Date(today.Year(), time.January, 1, 0, 0, 0, 0, location)
		query.From = &from
		query.FromDate = from.Format(time.DateOnly)
	case TrendOneYear:
		from := today.AddDate(0, 0, -364)
		query.From = &from
		query.FromDate = from.Format(time.DateOnly)
	}
	return query
}
