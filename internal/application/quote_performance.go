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

// InstrumentQuotePeriods projects every selector period (1W, 1M, 3M, 6M, 1Y and
// all history) with the same evidence rules as InstrumentQuotePerformance.
func (s *Service) InstrumentQuotePeriods(ctx context.Context, id domain.InstrumentID) (domain.QuotePerformanceReport, error) {
	return s.instrumentQuotePerformance(ctx, id, domain.QuotePeriodWindows)
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
