// Package quote adapts internal/application.Service's manual/current
// Instrument-quote and FX-quote/preference surface for the Wails IPC
// boundary. Provider-driven refresh lives in the marketdata service;
// this service only reads and manually appends quotes.
package quote

import (
	"context"
	"errors"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

func (s *Service) CurrentInstrumentQuote(ctx context.Context, instrumentID string) (*wire.InstrumentQuoteDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	quote, err := s.app.CurrentInstrumentQuote(ctx, id)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromInstrumentQuotePtr(quote), nil
}

func (s *Service) AppendManualInstrumentQuote(ctx context.Context, instrumentID, unitPrice, quotedAt string, delayed bool) (wire.InstrumentQuoteDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return wire.InstrumentQuoteDTO{}, apierror.Wrap(err)
	}
	quote, err := s.app.AppendManualInstrumentQuote(ctx, id, unitPrice, quotedAt, delayed)
	if err != nil {
		return wire.InstrumentQuoteDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstrumentQuote(quote), nil
}

func (s *Service) CurrentFXQuote(ctx context.Context, currencyA, currencyB string) (*wire.FXQuoteDTO, error) {
	a, err := domain.ParseCurrency(currencyA)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	b, err := domain.ParseCurrency(currencyB)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	quote, err := s.app.CurrentFXQuote(ctx, a, b)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromFXQuotePtr(quote), nil
}

// QuoteSeriesPointDTO is one local observation in the requested display
// direction. Value is a canonical decimal string.
type QuoteSeriesPointDTO struct {
	Currency        string `json:"currency,omitempty"`
	ObservationKind string `json:"observationKind,omitempty"`
	EffectiveDate   string `json:"effectiveDate,omitempty"`
	PriceBasis      string `json:"priceBasis,omitempty"`
	TimestampBasis  string `json:"timestampBasis,omitempty"`
	QuotedAt        string `json:"quotedAt"`
	Value           string `json:"value"`
	SourceKind      string `json:"sourceKind"`
	SourceKey       string `json:"sourceKey"`
	Delayed         bool   `json:"delayed"`
}

// QuoteSeriesDTO is the bounded local quote-history read model used by
// Market Data charts. Observations keep every matching fact; Points are the
// deterministically selected chart series.
type QuoteSeriesDTO struct {
	Range           string                `json:"range"`
	DisplayCurrency string                `json:"displayCurrency,omitempty"`
	BaseCurrency    string                `json:"baseCurrency,omitempty"`
	QuoteCurrency   string                `json:"quoteCurrency,omitempty"`
	Points          []QuoteSeriesPointDTO `json:"points"`
	Observations    []QuoteSeriesPointDTO `json:"observations"`
	OutsideRange    bool                  `json:"outsideRange"`
}

func fromQuoteSeries(value domain.QuoteSeries) QuoteSeriesDTO {
	return QuoteSeriesDTO{
		Range:           string(value.Range),
		DisplayCurrency: value.DisplayCurrency.String(),
		BaseCurrency:    value.BaseCurrency.String(),
		QuoteCurrency:   value.QuoteCurrency.String(),
		Points:          fromQuoteSeriesPoints(value.Points),
		Observations:    fromQuoteSeriesPoints(value.Observations),
		OutsideRange:    value.OutsideRange,
	}
}

func fromQuoteSeriesPoints(values []domain.QuoteSeriesPoint) []QuoteSeriesPointDTO {
	result := make([]QuoteSeriesPointDTO, 0, len(values))
	for _, point := range values {
		result = append(result, QuoteSeriesPointDTO{
			Currency:        point.Currency.String(),
			QuotedAt:        wire.FormatTime(point.QuotedAt),
			ObservationKind: point.ObservationKind,
			EffectiveDate:   point.EffectiveDate,
			PriceBasis:      point.PriceBasis,
			TimestampBasis:  point.TimestampBasis,
			Value:           point.Value,
			SourceKind:      string(point.SourceKind),
			SourceKey:       point.SourceKey,
			Delayed:         point.Delayed,
		})
	}
	return result
}

func (s *Service) InstrumentQuoteSeries(ctx context.Context, instrumentID, trendRange, sourceFilter string) (QuoteSeriesDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return QuoteSeriesDTO{}, apierror.Wrap(err)
	}
	parsedRange, err := domain.ParseTrendRange(trendRange)
	if err != nil {
		return QuoteSeriesDTO{}, apierror.Wrap(err)
	}
	parsedFilter, err := domain.ParseQuoteSourceFilter(sourceFilter)
	if err != nil {
		return QuoteSeriesDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.InstrumentQuoteSeries(ctx, id, parsedRange, parsedFilter)
	// Wails has already cancelled the abandoned JavaScript promise.
	// Match HistoricalOverview: do not log/reject again or return partial data.
	if errors.Is(ctx.Err(), context.Canceled) {
		return QuoteSeriesDTO{}, nil
	}
	if err != nil {
		return QuoteSeriesDTO{}, apierror.Wrap(err)
	}
	return fromQuoteSeries(result), nil
}

func (s *Service) FXQuoteSeries(ctx context.Context, currencyA, currencyB, trendRange, sourceFilter string) (QuoteSeriesDTO, error) {
	parsedRange, err := domain.ParseTrendRange(trendRange)
	if err != nil {
		return QuoteSeriesDTO{}, apierror.Wrap(err)
	}
	parsedFilter, err := domain.ParseQuoteSourceFilter(sourceFilter)
	if err != nil {
		return QuoteSeriesDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.FXQuoteSeries(ctx, domain.CurrencyCode(currencyA), domain.CurrencyCode(currencyB), parsedRange, parsedFilter)
	if err != nil {
		return QuoteSeriesDTO{}, apierror.Wrap(err)
	}
	return fromQuoteSeries(result), nil
}

func (s *Service) AppendManualFXQuote(ctx context.Context, baseCurrency, quoteCurrency, rate, quotedAt string) (wire.FXQuoteDTO, error) {
	quote, err := s.app.AppendManualFXQuote(ctx, baseCurrency, quoteCurrency, rate, quotedAt)
	if err != nil {
		return wire.FXQuoteDTO{}, apierror.Wrap(err)
	}
	return wire.FromFXQuote(quote), nil
}

func (s *Service) SetFXPreference(ctx context.Context, currencyA, currencyB, source string) (wire.FXPreferenceDTO, error) {
	preference, err := s.app.SetFXPreference(ctx, currencyA, currencyB, source)
	if err != nil {
		return wire.FXPreferenceDTO{}, apierror.Wrap(err)
	}
	return wire.FromFXPreference(preference), nil
}

func (s *Service) ListFXPreferences(ctx context.Context) ([]wire.FXPreferenceDTO, error) {
	preferences, err := s.app.ListFXPreferences(ctx)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromFXPreferences(preferences), nil
}

// InstrumentQuoteReturns is the read-only descriptive change for the 7-, 30-,
// 90-, 180- and 365-day windows. Windows without enough history are omitted.
func (s *Service) InstrumentQuoteReturns(ctx context.Context, instrumentID string) (domain.QuotePerformanceReport, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return domain.QuotePerformanceReport{}, apierror.Wrap(err)
	}
	result, err := s.app.InstrumentQuoteReturns(ctx, id)
	// Wails has already cancelled the abandoned JavaScript promise.
	// Match HistoricalOverview: do not log/reject again or return partial data.
	if errors.Is(ctx.Err(), context.Canceled) {
		return domain.QuotePerformanceReport{}, nil
	}
	return result, apierror.Wrap(err)
}

// PerformancePointDTO is one daily observation of the performance chart.
// BreakBefore marks a price-basis discontinuity the chart must not bridge.
type PerformancePointDTO struct {
	Currency        string `json:"currency,omitempty"`
	ObservationKind string `json:"observationKind,omitempty"`
	EffectiveDate   string `json:"effectiveDate,omitempty"`
	PriceBasis      string `json:"priceBasis,omitempty"`
	TimestampBasis  string `json:"timestampBasis,omitempty"`
	QuotedAt        string `json:"quotedAt"`
	Value           string `json:"value"`
	SourceKind      string `json:"sourceKind"`
	SourceKey       string `json:"sourceKey"`
	Delayed         bool   `json:"delayed"`
	BreakBefore     bool   `json:"breakBefore"`
}

// PerformanceSeriesDTO carries the headline and the daily chart of one period.
// From and To are the dates the headline is calculated between.
type PerformanceSeriesDTO struct {
	WindowDays        int                      `json:"windowDays"`
	From              string                   `json:"from,omitempty"`
	To                string                   `json:"to,omitempty"`
	Item              *domain.QuotePerformance `json:"item,omitempty"`
	UnavailableReason string                   `json:"unavailableReason,omitempty"`
	Points            []PerformancePointDTO    `json:"points"`
}

// InstrumentQuotePerformanceSeries is the daily-only chart and headline of one
// selector period (windowDays 0 is all history). Market Data history is not changed.
func (s *Service) InstrumentQuotePerformanceSeries(ctx context.Context, instrumentID string, windowDays int) (PerformanceSeriesDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return PerformanceSeriesDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.InstrumentQuotePerformanceSeries(ctx, id, windowDays)
	// Wails has already cancelled the abandoned JavaScript promise.
	// Match HistoricalOverview: do not log/reject again or return partial data.
	if errors.Is(ctx.Err(), context.Canceled) {
		return PerformanceSeriesDTO{}, nil
	}
	if err != nil {
		return PerformanceSeriesDTO{}, apierror.Wrap(err)
	}
	points := make([]PerformancePointDTO, 0, len(result.Points))
	for _, point := range result.Points {
		points = append(points, PerformancePointDTO{
			Currency: point.Currency.String(), ObservationKind: point.ObservationKind, EffectiveDate: point.EffectiveDate,
			PriceBasis: point.PriceBasis, TimestampBasis: point.TimestampBasis, QuotedAt: wire.FormatTime(point.QuotedAt),
			Value: point.Value, SourceKind: string(point.SourceKind), SourceKey: point.SourceKey, Delayed: point.Delayed, BreakBefore: point.BreakBefore,
		})
	}
	return PerformanceSeriesDTO{WindowDays: result.WindowDays, From: result.From, To: result.To, Item: result.Item, UnavailableReason: result.UnavailableReason, Points: points}, nil
}

// InstrumentQuotePerformance is a read-only descriptive price history projection.
func (s *Service) InstrumentQuotePerformance(ctx context.Context, instrumentID string) (domain.QuotePerformanceReport, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return domain.QuotePerformanceReport{}, apierror.Wrap(err)
	}
	result, err := s.app.InstrumentQuotePerformance(ctx, id)
	// Wails has already cancelled the abandoned JavaScript promise.
	// Match HistoricalOverview: do not log/reject again or return partial data.
	if errors.Is(ctx.Err(), context.Canceled) {
		return domain.QuotePerformanceReport{}, nil
	}
	return result, apierror.Wrap(err)
}
