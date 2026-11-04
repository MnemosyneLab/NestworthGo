package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type readOnlyAttributionKey struct{}

// WithReadOnlyFinancialAttribution is a host policy, never a wire permission.
// The same shared capability handler/kernel is used, but snapshot maintenance is
// prohibited. External MCP callers cannot construct this private context value.
func WithReadOnlyFinancialAttribution(ctx context.Context) context.Context {
	return context.WithValue(ctx, readOnlyAttributionKey{}, true)
}
func readOnlyFinancialAttribution(ctx context.Context) bool {
	value, _ := ctx.Value(readOnlyAttributionKey{}).(bool)
	return value
}

// Reconstruct only the bounded closed-day inputs in memory using the same
// historical valuation/replay engine. Never save snapshots, update dirty ranges,
// invalidate previews, pull quotes or repair the household database.
func (s *Service) readOnlyAttributionSnapshots(ctx context.Context, inputs FinancialContextInputs, start, end, now time.Time, provider string, ttl time.Duration) ([]domain.DailyValuationSnapshot, error) {
	batch := inputs.History
	location, err := time.LoadLocation(batch.Origin.Timezone)
	if err != nil {
		return nil, err
	}
	builder := &Service{repository: s.repository, now: func() time.Time { return now }, fxProviderKey: provider, quoteCacheTTL: ttl}
	ctx = context.WithValue(ctx, historicalSnapshotBatchKey{}, batch)
	quotes := &historicalQuoteCache{instrumentQuotes: map[domain.InstrumentID][]domain.InstrumentQuote{}, fxQuotes: batch.FXQuoteFacts}
	for _, quote := range batch.InstrumentQuoteFacts {
		quotes.instrumentQuotes[quote.InstrumentID] = append(quotes.instrumentQuotes[quote.InstrumentID], quote)
	}
	ctx = context.WithValue(ctx, historicalQuoteCacheKey{}, quotes)
	snapshots := []domain.DailyValuationSnapshot{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cutoff, err := historicalOverviewDayCutoff(day, location)
		if err != nil {
			return nil, err
		}
		snapshot, err := builder.valueHistoricalSnapshot(ctx, &batch.Origin, cutoff, day.Format("2006-01-02"))
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}
