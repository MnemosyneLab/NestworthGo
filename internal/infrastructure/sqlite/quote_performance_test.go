package sqlite

import (
	"database/sql"
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestQuotePerformanceEvidenceLatestAndNearestEarlierBaselines(t *testing.T) {
	database, repository, _, _, i := seedPortfolioRepository(t)
	i.ID = domain.NewInstrumentID()
	i.Name = "Synthetic NAV"
	i.Type = domain.InstrumentBankInvestmentProduct
	i.QuoteSource = domain.QuoteSourceAgent
	if err := repository.CreateInstrument(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ date, price string }{{"2026-01-01", "0.5"}, {"2026-09-01", "1"}, {"2026-09-24", "1.01"}, {"2026-10-05", "1.02"}, {"2026-10-14", "1.03"}, {"2026-10-20", "2"}} {
		at, _ := time.Parse(time.DateOnly, item.date)
		price, _ := domain.ParseUnitPrice(item.price)
		q := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: i.ID, UnitPrice: price, Currency: i.QuoteCurrency, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: at, CreatedAt: at, EffectiveDate: item.date, ObservationKind: "close", PriceBasis: "agent_unit_nav_v1", TimestampBasis: "date_label", Revision: 1, BindingRevision: i.ProviderBindingRevision}
		if err := database.WithTx(t.Context(), func(tx *sql.Tx) error { return insertAgentInstrumentQuoteTx(t.Context(), tx, i.HouseholdID, q, "") }); err != nil {
			t.Fatal(err)
		}
	}
	quotes, unsafe, err := repository.ReadQuotePerformanceEvidence(t.Context(), i, "2026-10-19")
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 4 {
		t.Fatalf("bounded quotes %+v", quotes)
	}
	got := domain.ProjectQuotePerformance(i, quotes, unsafe)
	if len(got) != 2 || got[0].BaselineOn != "2026-10-05" || got[0].ElapsedDays != 9 || got[1].BaselineOn != "2026-09-01" || got[1].LatestOn != "2026-10-14" {
		t.Fatalf("performance %+v", got)
	}
}
