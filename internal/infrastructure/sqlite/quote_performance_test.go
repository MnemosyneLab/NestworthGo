package sqlite

import (
	"database/sql"
	"github.com/waltwang/nestworth-go/internal/domain"
	"path/filepath"
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

func TestQuotePerformanceLegacyCorrectedDividendUsesEconomicDate(t *testing.T) {
	database := seedCurrentGainFixture(t, filepath.Join(t.TempDir(), "performance-correction.db"))
	repository := NewRepository(database)
	const instrumentID = "00000000-0000-4000-8000-000000000040"
	const householdID = "00000000-0000-4000-8000-000000000001"
	i, err := repository.Instrument(t.Context(), domain.HouseholdID(householdID), domain.InstrumentID(instrumentID))
	if err != nil {
		t.Fatal(err)
	}
	i.Type = domain.InstrumentMutualFund
	i.QuoteSource = domain.QuoteSourceAgent
	for _, item := range []struct{ date, price string }{{"2026-10-01", "1"}, {"2026-10-09", "1.05"}} {
		at, _ := time.Parse(time.DateOnly, item.date)
		price, _ := domain.ParseUnitPrice(item.price)
		q := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: i.ID, UnitPrice: price, Currency: i.QuoteCurrency, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: at, CreatedAt: at, EffectiveDate: item.date, ObservationKind: "close", PriceBasis: "agent_unit_nav_v1", TimestampBasis: "date_label", Revision: 1, BindingRevision: i.ProviderBindingRevision}
		if err := database.WithTx(t.Context(), func(tx *sql.Tx) error { return insertAgentInstrumentQuoteTx(t.Context(), tx, i.HouseholdID, q, "") }); err != nil {
			t.Fatal(err)
		}
	}
	quotes, unsafe, err := repository.ReadQuotePerformanceEvidence(t.Context(), i, "2026-10-21")
	if err != nil || len(domain.ProjectQuotePerformance(i, quotes, unsafe)) != 1 {
		t.Fatalf("initial performance %v %v", unsafe, err)
	}
	// Legacy replacements retain the recording day in the audit ledger, but
	// describe a dividend economically paid inside the Oct 1–9 NAV interval.
	_, err = database.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at,reverses_activity_id,correction_group_id) VALUES
 ('div-original','00000000-0000-4000-8000-000000000001','cash_dividend','income','2026-10-05T00:00:00Z','2026-10-05','2026-10-05T00:00:00Z',NULL,NULL),
 ('div-reversal','00000000-0000-4000-8000-000000000001','reversal','correction','2026-10-20T00:00:00Z','2026-10-20','2026-10-20T00:00:00Z','div-original','div-fix'),
 ('div-replacement','00000000-0000-4000-8000-000000000001','cash_dividend','income','2026-10-20T00:00:00Z','2026-10-20','2026-10-20T00:00:00Z',NULL,'div-fix');
 INSERT INTO activity_correction_groups VALUES('div-fix','00000000-0000-4000-8000-000000000001','div-original','div-replacement','2026-10-20T00:00:00Z');
 INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES('div-effect','div-replacement',1,'income','added','account_cash','income','00000000-0000-4000-8000-000000000021','1','USD');
 INSERT INTO activity_dividend_details VALUES('div-replacement','00000000-0000-4000-8000-000000000050','00000000-0000-4000-8000-000000000040','1','USD');`)
	if err != nil {
		t.Fatal(err)
	}
	quotes, unsafe, err = repository.ReadQuotePerformanceEvidence(t.Context(), i, "2026-10-21")
	if err != nil || len(unsafe) != 1 || unsafe[0] != "2026-10-05" || len(domain.ProjectQuotePerformance(i, quotes, unsafe)) != 0 {
		t.Fatalf("legacy dividend dates %v performance %+v err %v", unsafe, domain.ProjectQuotePerformance(i, quotes, unsafe), err)
	}
}
