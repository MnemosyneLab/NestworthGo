package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistoricalSeriesConnectsYahooAndAgentCloses(t *testing.T) {
	t.Parallel()
	s, db, owner, now := overviewFixture(t)
	ctx := t.Context()
	broker := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic stock", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: "yahoo_finance", ProviderSymbol: "SYNTHETIC"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.repository.(*sqlite.Repository).CommitInstrumentHistory(ctx, sqlite.InstrumentHistoryCommit{
		HouseholdID: i.HouseholdID, InstrumentID: i.ID, ProviderKey: "yahoo_finance", ProviderSymbol: "SYNTHETIC", QuoteCurrency: "CNY", Status: "mapped", SourcePolicy: domain.YahooRawClosePriceBasis, FetchedAt: *now,
		Observations:   []sqlite.InstrumentHistoryObservation{{MarketDate: "2026-08-01", Value: "100", Currency: "CNY", Kind: "close", ValueEffectiveAt: *now, PriceBasis: domain.YahooRawClosePriceBasis, TimestampBasis: "session_close"}},
		VerifiedRanges: []sqlite.DateSpan{{Start: "2026-08-01", End: "2026-08-01"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "100"}); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	if _, err := s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "CNY", Date: "2026-08-02", Timezone: "UTC", Kind: "close", Value: "101", SourceTitle: "Synthetic Agent close"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	out, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, out.Points[0].Assets, "100")
	wantOverviewAmount(t, out.Points[1].Assets, "101")
	last := out.Points[1]
	if last.BasisChanged || last.ScopeChanged || !last.ConnectAssets || !last.ConnectLiabilities || !last.ConnectNetWorth {
		t.Fatalf("cross-source close disconnected: %+v", last)
	}
	original, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil || !reflect.DeepEqual(original, out.Overview) {
		t.Fatalf("endpoint facts changed: %v", err)
	}
}

func TestHistoricalSeriesPriceUnitsRetainRealDiscontinuities(t *testing.T) {
	t.Parallel()
	i := domain.Instrument{Type: domain.InstrumentStock}
	base := domain.InstrumentQuote{Currency: "USD", ObservationKind: "close", PriceBasis: domain.YahooRawClosePriceBasis, TimestampBasis: "session_close", SourceKind: domain.QuoteSourceProvider, SourceKey: "yahoo_finance", SourcePolicyVersion: "yahoo_policy", BindingRevision: 1}
	first := seriesEvidenceBasis(seriesPriceBasis(i, base))
	for _, tc := range []struct {
		name   string
		change func(*domain.InstrumentQuote)
		same   bool
	}{
		{"Agent date label", func(q *domain.InstrumentQuote) {
			q.SourceKind = domain.QuoteSourceAgent
			q.SourceKey = "agent"
			q.PriceBasis = "agent_raw_close_v1"
			q.TimestampBasis = "date_label"
			q.SourcePolicyVersion = "agent_policy"
		}, true},
		{"manual dated close", func(q *domain.InstrumentQuote) {
			q.SourceKind = domain.QuoteSourceManual
			q.SourceKey = ""
			q.TimestampBasis = "source_timestamp"
		}, true},
		{"currency", func(q *domain.InstrumentQuote) { q.Currency = "SGD" }, false},
		{"adjusted close", func(q *domain.InstrumentQuote) { q.PriceBasis = "adjusted_close" }, false},
		{"unknown basis", func(q *domain.InstrumentQuote) { q.PriceBasis = "unknown" }, false},
		{"unknown raw_close alias", func(q *domain.InstrumentQuote) { q.PriceBasis = "raw_close" }, false},
		{"binding", func(q *domain.InstrumentQuote) { q.BindingRevision++ }, false},
		{"split", func(q *domain.InstrumentQuote) { q.SplitFactor = "2" }, false},
		{"no split", func(q *domain.InstrumentQuote) { q.SplitFactor = "1.00" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := base
			tc.change(&q)
			if same := first == seriesEvidenceBasis(seriesPriceBasis(i, q)); same != tc.same {
				t.Fatalf("same unit=%v, want %v", same, tc.same)
			}
		})
	}
}
