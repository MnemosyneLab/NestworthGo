package sqlite

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestMarketSnapshotExplicitDatesUseMarketDateAndLocalInstants(t *testing.T) {
	db, repo, household, _, instrument := seedPortfolioRepository(t)
	price, _ := domain.ParseUnitPrice("1.12345678")
	ids := map[string]string{}
	for _, tc := range []struct{ name, at, effective string }{
		{"before", "2026-03-08T04:59:59Z", ""},
		{"start", "2026-03-08T05:00:00Z", ""},
		{"end", "2026-03-09T03:59:59.999999999Z", ""},
		{"after", "2026-03-09T04:00:00Z", ""},
		{"market_day", "2026-03-10T12:00:00Z", "2026-03-08"},
		{"other_market_day", "2026-03-08T12:00:00Z", "2026-03-09"},
	} {
		at, err := time.Parse(time.RFC3339Nano, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		q := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrument.ID, UnitPrice: price, Currency: "USD", SourceKind: domain.QuoteSourceManual, QuotedAt: at, CreatedAt: at, ObservationKind: "latest", Revision: 1, EffectiveDate: tc.effective}
		if tc.effective != "" {
			q.ObservationKind, q.PriceBasis, q.TimestampBasis = "close", "raw_close", "market_date"
		}
		if err := repo.AppendInstrumentQuote(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		// The manual latest append intentionally supplies no daily metadata.
		// Seed daily storage facts explicitly, without changing that API.
		if tc.effective != "" {
			if _, err := db.SQL.ExecContext(t.Context(), `UPDATE instrument_quotes SET effective_date=?,observation_kind='close',price_basis='raw_close',timestamp_basis='market_date' WHERE id=?`, tc.effective, q.ID.String()); err != nil {
				t.Fatal(err)
			}
		}
		ids[tc.name] = q.ID.String()
	}
	date := "2026-03-08"
	request := domain.MarketReadRequest{Targets: []domain.MarketReadTarget{{InstrumentID: instrument.ID.String()}}, View: "history", FromDate: &date, ToDate: &date, SourceFilter: "all"}
	check := func(want []string) {
		t.Helper()
		out, err := repo.ReadMarketDataSnapshot(domain.WithBoundedRead(t.Context()), request, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, q := range out.InstrumentHistory[request.Targets[0].Key()] {
			got[q.ID.String()] = true
		}
		if len(got) != len(want) {
			t.Fatalf("quotes=%v, want %v", got, want)
		}
		for _, name := range want {
			if !got[ids[name]] {
				t.Fatalf("missing %s", name)
			}
		}
	}
	// No origin: explicitly verified UTC fallback, not the machine timezone.
	check([]string{"before", "start", "market_day"})
	if _, err := db.SQL.ExecContext(t.Context(), `INSERT INTO history_origins(id,household_id,timezone,started_at,created_at) VALUES(?,?,?,?,?)`, domain.NewHistoryOriginID().String(), household.ID.String(), "America/New_York", "2026-03-01T00:00:00Z", "2026-03-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	check([]string{"start", "end", "market_day"})
	// Invalid configured timezone must fail instead of treating evidence as absent.
	if _, err := db.SQL.ExecContext(t.Context(), `UPDATE history_origins SET timezone='Invalid/Timezone'`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadMarketDataSnapshot(t.Context(), request, time.Now()); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}
