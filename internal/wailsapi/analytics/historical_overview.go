package analytics

import (
	"context"
	"errors"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// HistoricalOverview returns the immutable presentation projection. It does
// not materialize daily snapshots or refresh market data.
func (s *Service) HistoricalOverview(ctx context.Context, date, compareTo string) (application.HistoricalOverviewResult, error) {
	result, err := s.app.HistoricalOverview(ctx, date, compareTo)
	// Wails has already rejected the cancelled JS promise with CancelError.
	// A late Go error would become an unhandled CancelledRejectionError in
	// beta.28. Complete the abandoned call quietly, with no partial capture.
	if errors.Is(ctx.Err(), context.Canceled) {
		return application.HistoricalOverviewResult{}, nil
	}
	return result, apierror.Wrap(err)
}
