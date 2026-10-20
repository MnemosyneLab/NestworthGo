package quote

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestQuoteSeriesPreservesObservationMetadata(t *testing.T) {
	at := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	kind := "realtime"
	points := []domain.QuoteSeriesPoint{{Currency: "JPY", QuotedAt: at, Value: "76.94", ObservationKind: kind, EffectiveDate: "2026-09-17"}}
	dto := fromQuoteSeries(domain.QuoteSeries{Points: points, Observations: points})
	for _, point := range []QuoteSeriesPointDTO{dto.Points[0], dto.Observations[0]} {
		if point.Currency != "JPY" || point.ObservationKind != kind || point.EffectiveDate != "2026-09-17" || point.Value != "76.94" {
			t.Fatalf("metadata lost: %+v", point)
		}
	}
}
