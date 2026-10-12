package analytics

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

func (s *Service) HistoricalOverviewSeries(ctx context.Context, from, to string) (application.HistoricalOverviewSeriesResult, error) {
	result, err := s.app.HistoricalOverviewSeries(ctx, from, to)
	return result, apierror.Wrap(err)
}
