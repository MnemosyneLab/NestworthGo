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

func TestQuotePerformanceAllSourcesIgnorePreferenceAndExcludeIntraday(t *testing.T) {
	for _, preference := range []domain.QuoteSourceKind{domain.QuoteSourceAgent, domain.QuoteSourceManual, domain.QuoteSourceProvider} {
		t.Run(string(preference), func(t *testing.T) {
			database, repository, _, _, i := seedPortfolioRepository(t)
			i.ID = domain.NewInstrumentID()
			i.Name = "Synthetic daily prices"
			i.Type = domain.InstrumentStock
			i.QuoteSource = preference
			if preference == domain.QuoteSourceProvider {
				key, symbol := domain.YahooFinanceProviderKey, "SYN"
				i.ProviderKey = &key
				i.Symbol = &symbol
				i.ProviderSymbol = &symbol
			}
			if err := repository.CreateInstrument(t.Context(), i); err != nil {
				t.Fatal(err)
			}
			for n, source := range []domain.QuoteSourceKind{domain.QuoteSourceManual, domain.QuoteSourceProvider, domain.QuoteSourceAgent} {
				on := []string{"2026-10-05", "2026-10-10", "2026-10-14"}[n]
				at, _ := time.Parse(time.DateOnly, on)
				price, _ := domain.ParseUnitPrice("100")
				q := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: i.ID, UnitPrice: price, Currency: i.QuoteCurrency, SourceKind: source, SourceKey: "synthetic", QuotedAt: at, CreatedAt: at, EffectiveDate: on, ObservationKind: "close", PriceBasis: domain.TiingoRawClosePriceBasis, TimestampBasis: "session_close", Revision: 1, BindingRevision: i.ProviderBindingRevision}
				if err := database.WithTx(t.Context(), func(tx *sql.Tx) error { return insertAgentInstrumentQuoteTx(t.Context(), tx, i.HouseholdID, q, "") }); err != nil {
					t.Fatal(err)
				}
			}
			// A dated realtime quote must not move the daily anchor; nor may a future close.
			for _, kind := range []string{"realtime", "close"} {
				on := "2026-10-15"
				if kind == "close" {
					on = "2026-10-16"
				}
				at, _ := time.Parse(time.DateOnly, on)
				price, _ := domain.ParseUnitPrice("999")
				q := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: i.ID, UnitPrice: price, Currency: i.QuoteCurrency, SourceKind: domain.QuoteSourceAgent, QuotedAt: at, CreatedAt: at, EffectiveDate: on, ObservationKind: kind, PriceBasis: "agent_raw_close_v1", TimestampBasis: "date_label", Revision: 1, BindingRevision: i.ProviderBindingRevision}
				if err := database.WithTx(t.Context(), func(tx *sql.Tx) error { return insertAgentInstrumentQuoteTx(t.Context(), tx, i.HouseholdID, q, "") }); err != nil {
					t.Fatal(err)
				}
			}
			quotes, unsafe, err := repository.ReadQuotePerformanceEvidence(t.Context(), i, "2026-10-15")
			if err != nil || len(quotes) != 3 {
				t.Fatalf("daily read count=%d err=%v", len(quotes), err)
			}
			got := domain.ProjectQuotePerformance(i, quotes, unsafe)
			if len(got) != 1 || got[0].LatestOn != "2026-10-14" || got[0].PeriodPercent != "0" || got[0].ElapsedDays != 9 {
				t.Fatalf("mixed daily %+v", got)
			}
		})
	}
}
