package quote_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

// Use the real temporary SQLite repository: an abandoned read must complete
// quietly after Wails has already cancelled its JavaScript promise.
func TestQuoteReadCancellationAndLiveErrors(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := t.Context()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{HouseholdName: "Synthetic cancellation", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	i, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{Name: "Synthetic stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	s := quote.NewService(app)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := app.InstrumentQuotePerformance(cancelled, domain.InstrumentID(i.ID)); !errors.Is(err, context.Canceled) {
		t.Fatalf("synthetic SQLite read did not reproduce cancellation: %v", err)
	}
	tests := []struct {
		name string
		read func(context.Context) error
	}{
		{"history", func(ctx context.Context) error {
			_, err := s.InstrumentQuoteSeries(ctx, i.ID, "all", "all")
			return err
		}},
		{"performance", func(ctx context.Context) error { _, err := s.InstrumentQuotePerformance(ctx, i.ID); return err }},
		{"returns", func(ctx context.Context) error { _, err := s.InstrumentQuoteReturns(ctx, i.ID); return err }},
		{"daily series", func(ctx context.Context) error {
			_, err := s.InstrumentQuotePerformanceSeries(ctx, i.ID, 30)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.read(cancelled); err != nil {
				t.Fatalf("abandoned read became a binding failure: %v", err)
			}
			if err := tt.read(ctx); err != nil {
				t.Fatalf("replacement read failed: %v", err)
			}
			// A deadline is a real read failure, not observer abandonment.
			deadline, stop := context.WithDeadline(t.Context(), time.Unix(0, 0))
			defer stop()
			err := tt.read(deadline)
			if err == nil {
				t.Fatal("expired deadline was hidden as cancellation")
			}
			wire, ok := apierror.Parse(err.Error())
			if !ok || wire.Code != "internal" {
				t.Fatalf("deadline error contract changed: %v", err)
			}
		})
	}
	_, err = s.InstrumentQuotePerformanceSeries(ctx, i.ID, 45)
	assertQuoteWireCode(t, err, "validation")
	_, err = s.InstrumentQuoteReturns(ctx, "invalid-id")
	assertQuoteWireCode(t, err, "validation")
	_, err = s.InstrumentQuoteSeries(ctx, i.ID, "week", "all")
	assertQuoteWireCode(t, err, "validation")
}

func assertQuoteWireCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("live invalid read succeeded")
	}
	wire, ok := apierror.Parse(err.Error())
	if !ok || wire.Code != code {
		t.Fatalf("live error contract changed: %v", err)
	}
}
