package application

import (
	"testing"
	"time"
)

func TestManualQuoteDatesUseHistoryTimezone(t *testing.T) {
	for _, zone := range []string{"Pacific/Kiritimati", "America/Los_Angeles", "Asia/Singapore"} {
		t.Run(zone, func(t *testing.T) {
			service, ctx, _, _ := newOnboardedService(t, "manual-date", nil)
			instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Manual", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.StartHistory(ctx, zone); err != nil {
				t.Fatal(err)
			}
			location, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			for _, date := range []string{"2026-03-08", "2026-11-01"} {
				want, err := time.ParseInLocation("2006-01-02", date, location)
				if err != nil {
					t.Fatal(err)
				}
				quote, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "12", date, false)
				if err != nil || !quote.QuotedAt.Equal(want) {
					t.Fatalf("instrument: got %v want %v err %v", quote.QuotedAt, want, err)
				}
				fx, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", date)
				if err != nil || !fx.QuotedAt.Equal(want) {
					t.Fatalf("FX: got %v want %v err %v", fx.QuotedAt, want, err)
				}
			}
			explicit := "2026-09-01T12:00:00+08:00"
			want, _ := time.Parse(time.RFC3339, explicit)
			got, err := service.parseManualQuoteTimestamp(ctx, explicit)
			if err != nil || !got.Equal(want) {
				t.Fatalf("explicit instant changed: %v %v", got, err)
			}
			if _, err := service.parseManualQuoteTimestamp(ctx, "2026-02-30"); err == nil {
				t.Fatal("accepted invalid date")
			}
		})
	}
}
