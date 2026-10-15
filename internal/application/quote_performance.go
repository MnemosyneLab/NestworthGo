package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type quotePerformanceReader interface {
	ReadQuotePerformanceEvidence(context.Context, domain.Instrument, string) ([]domain.InstrumentQuote, error)
}

type quotePeriodReader interface {
	ReadQuotePerformanceEvidenceWindows(context.Context, domain.Instrument, string, []int) ([]domain.InstrumentQuote, error)
}

// InstrumentQuotePerformance projects the 7/30-day descriptive windows.
func (s *Service) InstrumentQuotePerformance(ctx context.Context, id domain.InstrumentID) (domain.QuotePerformanceReport, error) {
	return s.instrumentQuotePerformance(ctx, id, domain.QuotePerformanceWindows)
}

// InstrumentQuoteReturns projects the returns section: the 7-, 30-, 90-, 180-
// and 365-day windows with the same evidence rules as InstrumentQuotePerformance.
// Windows without enough comparable history are omitted; all history is not one.
func (s *Service) InstrumentQuoteReturns(ctx context.Context, id domain.InstrumentID) (domain.QuotePerformanceReport, error) {
	return s.instrumentQuotePerformance(ctx, id, domain.QuoteReturnWindows)
}

type quoteSeriesEvidenceReader interface {
	ReadQuotePerformanceSeriesEvidence(context.Context, domain.Instrument, string, int) ([]domain.InstrumentQuote, error)
}

// InstrumentQuotePerformanceSeries returns the daily chart and headline of one
// selector period (windowDays 0 is all history). Both come from one evidence
// read and one daily selection, so the chart spans the headline's own dates.
func (s *Service) InstrumentQuotePerformanceSeries(ctx context.Context, id domain.InstrumentID, windowDays int) (domain.QuotePerformanceSeries, error) {
	if !domain.ValidQuotePeriodWindow(windowDays) {
		return domain.QuotePerformanceSeries{}, &domain.Error{Code: domain.ErrValidation, Field: "windowDays", Message: "unsupported performance period"}
	}
	empty := func(reason string) domain.QuotePerformanceSeries {
		return domain.QuotePerformanceSeries{WindowDays: windowDays, Points: []domain.QuotePerformancePoint{}, UnavailableReason: reason}
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return domain.QuotePerformanceSeries{}, err
	}
	if household == nil {
		return empty("insufficient_daily_history"), nil
	}
	i, err := s.repository.Instrument(ctx, household.ID, id)
	if err != nil {
		return domain.QuotePerformanceSeries{}, err
	}
	product, err := s.repository.ProductByInstrument(ctx, id)
	if err != nil {
		return domain.QuotePerformanceSeries{}, err
	}
	if product != nil {
		return empty("unsupported_instrument"), nil
	}
	reader, ok := s.repository.(quoteSeriesEvidenceReader)
	if !ok {
		return domain.QuotePerformanceSeries{}, &domain.Error{Code: domain.ErrUnavailable, Message: "local price history is unavailable"}
	}
	quotes, err := reader.ReadQuotePerformanceSeriesEvidence(ctx, i, s.clock().UTC().Format(time.DateOnly), windowDays)
	if err != nil {
		return domain.QuotePerformanceSeries{}, err
	}
	return domain.ProjectQuotePerformanceSeries(i, quotes, windowDays), nil
}

func (s *Service) instrumentQuotePerformance(ctx context.Context, id domain.InstrumentID, windows []int) (domain.QuotePerformanceReport, error) {
	household, err := s.repository.Household(ctx)
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	if household == nil {
		return domain.QuotePerformanceReport{Items: []domain.QuotePerformance{}, UnavailableReason: "insufficient_daily_history"}, nil
	}
	i, err := s.repository.Instrument(ctx, household.ID, id)
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	product, err := s.repository.ProductByInstrument(ctx, id)
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	if product != nil {
		return domain.QuotePerformanceReport{Items: []domain.QuotePerformance{}, UnavailableReason: "unsupported_instrument"}, nil
	}
	reader, ok := s.repository.(quotePeriodReader)
	if !ok {
		return domain.QuotePerformanceReport{}, &domain.Error{Code: domain.ErrUnavailable, Message: "local price history is unavailable"}
	}
	// The daily cutoff must not depend on a personal History Origin timezone.
	quotes, err := reader.ReadQuotePerformanceEvidenceWindows(ctx, i, s.clock().UTC().Format(time.DateOnly), windows)
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	return domain.ProjectQuotePerformanceReportWindows(i, quotes, windows), nil
}
