package analytics_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestHistoricalCaptureCancellationAndRealErrors(t *testing.T) {
	t.Parallel()
	app := wailstest.NewService(t)
	s := analytics.NewService(app)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	out, err := s.HistoricalOverview(ctx, "2026-08-01", "current")
	if err != nil || !reflect.DeepEqual(out, application.HistoricalOverviewResult{}) {
		t.Fatalf("overview cancellation became an internal failure/partial result: %+v %v", out, err)
	}
	series, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-31")
	if err != nil || !reflect.DeepEqual(series, application.HistoricalOverviewSeriesResult{}) {
		t.Fatalf("series cancellation became an internal failure/partial result: %+v %v", series, err)
	}
	// Cancellation has no wire payload; live requests still preserve domain codes.
	_, err = s.HistoricalOverviewSeries(t.Context(), "not-date", "2026-08-31")
	wire, ok := apierror.Parse(err.Error())
	if !ok || wire.Code != string(domain.ErrInvalidChangeTime) || wire.Field != "range" {
		t.Fatalf("real date error lost its wire contract: %v", err)
	}
	_, err = s.HistoricalOverview(t.Context(), "2026-08-01", "")
	wire, ok = apierror.Parse(err.Error())
	if !ok || wire.Code != string(domain.ErrHistoryNotStarted) {
		t.Fatalf("live missing-history failure lost its wire contract: %v", err)
	}
}
