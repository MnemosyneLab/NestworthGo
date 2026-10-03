package analytics

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// HistoricalOverview returns the immutable presentation projection. It does
// not materialize daily snapshots or refresh market data.
func (s *Service) HistoricalOverview(ctx context.Context, date, compareTo string) (application.HistoricalOverviewResult, error) {
	result, err := s.app.HistoricalOverview(ctx, date, compareTo)
	return result, apierror.Wrap(err)
}
