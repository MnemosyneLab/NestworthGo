package marketdata

import (
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestYahooChartNormalizationRejectsUntrustedObservationTimes(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		timestamp int64
		fallback  bool
	}{
		{name: "regular price beyond skew", timestamp: now.Add(domain.QuoteClockSkewTolerance + time.Second).Unix()},
		{name: "fallback price beyond skew", timestamp: now.Add(domain.QuoteClockSkewTolerance + time.Second).Unix(), fallback: true},
		{name: "largest positive unix value", timestamp: 1<<63 - 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"chart":{"result":[{"meta":{"currency":"USD","regularMarketPrice":%s,"regularMarketTime":%d},"timestamp":[%d],"indicators":{"quote":[{"close":[700.25]}]}}]}}`, fallbackPrice(testCase.fallback), testCase.timestamp, testCase.timestamp))
			if testCase.fallback {
				body = []byte(fmt.Sprintf(`{"chart":{"result":[{"meta":{"currency":"USD","regularMarketPrice":null},"timestamp":[%d],"indicators":{"quote":[{"close":[700.25]}]}}]}}`, testCase.timestamp))
			}
			if _, err := normalizeChartAt(body, domain.CurrencyCode("USD"), false, now); err == nil {
				t.Fatal("normalizeChartAt accepted an untrusted observation time")
			}
		})
	}
}

func TestYahooChartNormalizationRoundTripsAcceptedBoundary(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 123456000, time.UTC)
	timestamp := now.Add(domain.QuoteClockSkewTolerance).Unix()
	body := []byte(fmt.Sprintf(`{"chart":{"result":[{"meta":{"currency":"USD","regularMarketPrice":700.25,"regularMarketTime":%d},"timestamp":[],"indicators":{"quote":[]}}]}}`, timestamp))
	quote, err := normalizeChartAt(body, domain.CurrencyCode("USD"), false, now)
	if err != nil {
		t.Fatal(err)
	}
	if !quote.QuotedAt.Equal(time.Unix(timestamp, 0).UTC()) {
		t.Fatalf("quotedAt = %v, want %v", quote.QuotedAt, time.Unix(timestamp, 0).UTC())
	}
}

func fallbackPrice(fallback bool) string {
	if fallback {
		return "null"
	}
	return "700.25"
}
