package analytics

import (
	"context"
	"errors"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

func (s *Service) HistoricalOverviewSeries(ctx context.Context, from, to string) (application.HistoricalOverviewSeriesResult, error) {
	result, err := s.app.HistoricalOverviewSeries(ctx, from, to)
	// See HistoricalOverview: cancelled Wails promises must not reject again.
	if errors.Is(ctx.Err(), context.Canceled) {
		return application.HistoricalOverviewSeriesResult{}, nil
	}
	return result, apierror.Wrap(err)
}
