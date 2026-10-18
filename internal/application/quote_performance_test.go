package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func importQuoteNAV(t *testing.T, s *Service, i domain.Instrument, date, value string) {
	t.Helper()
	if _, err := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{agentNAV(i, value, date)}}); err != nil {
		t.Fatal(err)
	}
}

// Exercise real writers: existing-position imports and calibrations must not
// become instrument comparability evidence, regardless of their stored reason.
func TestInstrumentPerformanceIndependentOfPersonalLedger(t *testing.T) {
	for _, kind := range []string{"mutual_fund", "bank_investment_product"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx, b, setClock := newOnboardedService(t, "synthetic-performance", []string{"Synthetic owner"})
			setClock(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
			i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic NAV", Type: kind, QuoteCurrency: "USD", QuoteSource: "agent"})
			if err != nil {
				t.Fatal(err)
			}
			a := createHoldingsAccount(t, s, ctx, b.Members[0].ID, "Synthetic A")
			if _, err = s.AppendAccountCashValue(ctx, a.Account.ID, "1000", "USD", "2026-09-01"); err != nil {
				t.Fatal(err)
			}
			if _, err = s.StartHistory(ctx, "America/Los_Angeles"); err != nil {
				t.Fatal(err)
			}
			setClock(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
			for _, p := range []struct{ on, value string }{{"2026-09-01", "1"}, {"2026-10-01", "1.01"}, {"2026-10-14", "1.02"}} {
				importQuoteNAV(t, s, i, p.on, p.value)
			}
			want, err := s.InstrumentQuotePerformance(ctx, i.ID)
			if err != nil || len(want.Items) != 2 || want.Items[0].ElapsedDays != 13 || want.Items[1].ElapsedDays != 43 {
				t.Fatalf("holiday-gap windows %+v: %v", want, err)
			}
			if kind == "bank_investment_product" && (want.Items[0].AnnualizedPercent == nil || *want.Items[0].AnnualizedPercent != "27.798933739528") {
				t.Fatalf("actual-day annualization %+v", want)
			}
			check := func(stage string) {
				t.Helper()
				got, err := s.InstrumentQuotePerformance(ctx, i.ID)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("%s changed instrument performance: %+v want %+v err %v", stage, got, want, err)
				}
			}
			second := createHoldingsAccount(t, s, ctx, b.Members[0].ID, "Synthetic B")
			check("multiple accounts without holdings")
			at := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			cost := reconciliationUnitCost(t, "1")
			post := func(stage string, command any) domain.ChangePreview {
				t.Helper()
				result, err := s.RecordChange(ctx, command)
				if err != nil {
					t.Fatalf("%s: %v", stage, err)
				}
				check(stage)
				return result
			}
			post("existing-position import", domain.PositionImportInput{HouseholdID: b.Household.ID, AccountID: a.Account.ID, InstrumentID: i.ID, Quantity: mustQuantity(t, "10"), UnitCost: &cost, Currency: "USD", EffectiveAt: at})
			holdings, err := s.ListHoldings(ctx, a.Account.ID, false)
			if err != nil || len(holdings) != 1 {
				t.Fatalf("holdings %v %v", holdings, err)
			}
			h := holdings[0]
			db, err := sqlite.Open(s.repository.Path())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			post("quantity calibration", domain.PositionAdjustmentInput{HouseholdID: b.Household.ID, HoldingID: h.ID, Quantity: mustQuantity(t, "2"), Added: true, UnitCost: &cost, EffectiveAt: at.Add(time.Hour)})
			post("cost calibration", domain.PositionCostAdjustmentInput{HouseholdID: b.Household.ID, HoldingID: h.ID, UnitCost: reconciliationUnitCost(t, "0.9"), EffectiveAt: at.Add(2 * time.Hour)})
			for n, side := range []domain.TradeSide{domain.TradeBuy, domain.TradeSell} {
				post(string(side), domain.TradeInput{HouseholdID: b.Household.ID, Side: side, SettlementAccountID: a.Account.ID, HoldingID: h.ID, InstrumentID: i.ID, Quantity: mustQuantity(t, "1"), Gross: reconciliationMoney(t, "1", "USD"), EffectiveAt: at.Add(time.Duration(3+n) * time.Hour)})
			}
			post("transfer", domain.PositionTransferInput{HouseholdID: b.Household.ID, FromHoldingID: h.ID, ToAccountID: &second.Account.ID, Quantity: mustQuantity(t, "1"), EffectiveAt: at.Add(5 * time.Hour)})
			post("personal cash dividend", domain.CashDividendInput{HouseholdID: b.Household.ID, HoldingID: h.ID, Amount: reconciliationMoney(t, "0.5", "USD"), EffectiveAt: at.Add(6 * time.Hour)})
			// Verify the actual import metadata, including its instrument-only basis.
			quotes, err := s.repository.(quotePerformanceReader).ReadQuotePerformanceEvidence(ctx, i, "2026-10-14")
			if err != nil || len(quotes) != 3 {
				t.Fatalf("quotes %v %v", quotes, err)
			}
			for _, q := range quotes {
				if q.PriceBasis != "agent_unit_nav_v1" || q.TimestampBasis != "date_label" || q.ObservationKind != "close" {
					t.Fatalf("write-path metadata %+v", q)
				}
			}
			// The household is still on Oct 13, but the instrument's actual Oct 14
			// observation is eligible on the current UTC day independently of it.
			setClock(time.Date(2026, 10, 14, 0, 30, 0, 0, time.UTC))
			check("household timezone date boundary")
			// Only this disposable fixture is damaged. A personal-ledger read must
			// fail here, so equality proves the service path cannot depend on it.
			if _, err = db.SQL.Exec("PRAGMA foreign_keys=OFF"); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{"activity_correction_groups", "activity_dividend_details", "activity_trade_details", "activity_effects", "activities"} {
				if _, err = db.SQL.Exec("DROP TABLE " + table); err != nil {
					t.Fatal(err)
				}
			}
			check("personal ledger tables absent")
			setClock(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
			for _, table := range []string{"holdings", "accounts", "history_origin_components", "history_origins"} {
				if _, err = db.SQL.Exec("DROP TABLE " + table); err != nil {
					t.Fatal(err)
				}
			}
			check("holdings, accounts and History Origin absent")
		})
	}
}

func TestInstrumentPerformanceImportedCorporateActionsAndRetraction(t *testing.T) {
	for _, marker := range []string{"distribution", "split"} {
		t.Run(marker, func(t *testing.T) {
			s, i, setClock := newAgentMarketDataFixture(t)
			i.Type = domain.InstrumentBankInvestmentProduct
			// Persist the type through the normal instrument writer.
			var err error
			i, err = s.UpdateInstrument(t.Context(), i.ID, InstrumentInput{Name: i.Name, Type: string(i.Type), QuoteCurrency: "USD", QuoteSource: "agent"})
			if err != nil {
				t.Fatal(err)
			}
			setClock(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
			importQuoteNAV(t, s, i, "2026-10-01", "1")
			importQuoteNAV(t, s, i, "2026-10-14", "1.013")
			report, err := s.InstrumentQuotePerformance(t.Context(), i.ID)
			if err != nil || len(report.Items) != 1 || report.Items[0].ElapsedDays != 13 || *report.Items[0].AnnualizedPercent != "36.5" {
				t.Fatalf("short holiday history %+v %v", report, err)
			}
			current, err := s.CurrentInstrumentQuote(t.Context(), i.ID)
			if err != nil {
				t.Fatal(err)
			}
			item := agentNAV(i, "1.013", "2026-10-14")
			item.Operation = "correct"
			item.TargetQuoteID = current.ID.String()
			if marker == "distribution" {
				item.DividendCash = "0.01"
			} else {
				item.SplitFactor = "2"
			}
			result, err := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{item}})
			if err != nil {
				t.Fatal(err)
			}
			quotes, err := s.repository.(quotePerformanceReader).ReadQuotePerformanceEvidence(t.Context(), i, "2026-10-14")
			if err != nil || len(quotes) != 2 || (marker == "split" && quotes[1].SplitFactor != "2") || (marker == "distribution" && quotes[1].DividendCash != "0.01") {
				t.Fatalf("issuer metadata %+v %v", quotes, err)
			}
			report, err = s.InstrumentQuotePerformance(t.Context(), i.ID)
			if err != nil || len(report.Items) != 0 || report.UnavailableReason != "incomparable_daily_prices" {
				t.Fatalf("issuer action accepted %+v %v", report, err)
			}
			retract := AgentMarketDataItem{Operation: "retract", TargetQuoteID: result.QuoteIDs[0], SourceTitle: "Synthetic withdrawn observation"}
			if _, err = s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{retract}}); err != nil {
				t.Fatal(err)
			}
			quotes, err = s.repository.(quotePerformanceReader).ReadQuotePerformanceEvidence(t.Context(), i, "2026-10-14")
			if err != nil || len(quotes) != 1 || quotes[0].EffectiveDate != "2026-10-01" {
				t.Fatalf("revoked quote used %+v %v", quotes, err)
			}
			report, err = s.InstrumentQuotePerformance(t.Context(), i.ID)
			if err != nil || len(report.Items) != 0 || report.UnavailableReason != "insufficient_daily_history" {
				t.Fatalf("revoked baseline fallback %+v %v", report, err)
			}
		})
	}
}
