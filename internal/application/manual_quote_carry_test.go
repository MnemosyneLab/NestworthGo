package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Agent provenance must not turn a manual valuation preference into daily supply.
func TestManualPreferenceCarriesAgentPriceAcrossValuationAndHealth(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"nav", "close"} {
		t.Run(kind, func(t *testing.T) {
			ctx := t.Context()
			s, _, owner, now := overviewFixture(t)
			*now = time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)
			account := overviewAccount(t, s, owner, "Synthetic manual account", "brokerage", "asset", "holdings", "CNY", "")
			instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic fixed value", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "100"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: instrument.ID.String(), Currency: "CNY", Kind: kind, Date: "2026-10-09", Timezone: "Asia/Shanghai", Value: "1", SourceTitle: "Synthetic fixed valuation"}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.StartHistoryWithCosts(ctx, "Asia/Shanghai", map[domain.HoldingID]string{holding.ID: "1"}); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)
			trend, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-09:2026-10-20"))
			if err != nil {
				t.Fatal(err)
			}
			for _, point := range trend.Points {
				q := point.DisplayValuation.NetWorth.Quality
				if q.Coverage != domain.CoverageVerified || q.CarriedCount != 0 || q.ProvisionalCount != 0 || q.StaleCount != 0 || point.DisplayValuation.NetWorth.Total == nil || point.DisplayValuation.NetWorth.Total.Amount != "100" {
					t.Fatalf("manual %s: %+v", point.LocalDate, point)
				}
			}
			for _, date := range []string{"2026-10-10", "2026-10-19", "current"} {
				context, err := s.BuildFinancialContext(ctx, FinancialContextRequest{AsOf: date})
				if err != nil {
					t.Fatal(err)
				}
				pending := 0
				for _, position := range context.Content.Positions {
					pending += position.PendingContribution
					if position.Kind == "holding" && (position.DisplayValuation.Quality.CarriedCount != 0 || position.PendingContribution != 0 || !position.CountedLeaf) {
						t.Fatalf("manual leaf %s: %+v", date, position)
					}
				}
				if pending != context.Content.Summary.DisplayValuation.NetWorth.Quality.CarriedCount {
					t.Fatal("pending leaf total differs")
				}
				for _, gap := range context.Content.Gaps {
					if gap.Code == "stale_observation" {
						t.Fatalf("old valid manual quote warned on %s: %+v", date, gap)
					}
				}
				for _, evidence := range context.Content.Evidence {
					if evidence.Kind == "price" && (evidence.Freshness != "manual" || evidence.SourceKind != "agent" || evidence.EffectiveDate != "2026-10-09") {
						t.Fatalf("effective freshness/provenance %s: %+v", date, evidence)
					}
				}
			}
			overview, err := s.HistoricalOverview(ctx, "2026-10-10", "2026-10-19")
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range overview.Rows {
				if row.Kind != "holding" {
					continue
				}
				for _, cell := range []*HistoricalOverviewCell{row.Left, row.Right} {
					if cell.Price.Freshness != "manual" || cell.Price.Source != "agent" || !cell.Manual || cell.Price.ObservationFreshness != "stale" {
						t.Fatalf("manual evidence was replaced by observation age: %+v", cell.Price)
					}
				}
			}
			health, err := s.ScanMarketDataHealth(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !health.Healthy || len(health.Issues) != 0 {
				t.Fatalf("manual health: %+v", health)
			}
			records, err := s.AgentMarketDataRecords(ctx)
			if err != nil || len(records) != 1 {
				t.Fatalf("carry created observations: %d %v", len(records), err)
			}
			// A real replacement still changes valuation from its own effective date.
			if _, err = s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: instrument.ID.String(), Currency: "CNY", Kind: kind, Date: "2026-10-19", Timezone: "Asia/Shanghai", Value: "1.1", SourceTitle: "Synthetic replacement"}}}); err != nil {
				t.Fatal(err)
			}
			replaced, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-18:2026-10-20"))
			if err != nil {
				t.Fatal(err)
			}
			if replaced.Points[0].DisplayValuation.NetWorth.Total.Amount != "100" || replaced.Points[1].DisplayValuation.NetWorth.Total.Amount != "110" {
				t.Fatalf("replacement effective date: %+v", replaced.Points)
			}
			// A later preference change must not rewrite the earlier manual period.
			if _, err = s.UpdateInstrument(ctx, instrument.ID, InstrumentInput{QuoteSource: "agent"}); err != nil {
				t.Fatal(err)
			}
			*now = now.AddDate(0, 0, 1)
			switched, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-19:2026-10-20"))
			if err != nil {
				t.Fatal(err)
			}
			if switched.Points[0].DisplayValuation.NetWorth.Quality.CarriedCount != 0 || !switched.Points[0].Complete || switched.Points[1].Complete || switched.Points[1].DisplayValuation.NetWorth.Quality.CarriedCount != 1 {
				t.Fatalf("preference history: %+v", switched.Points)
			}

		})
	}
}

func TestManualPreferenceDoesNotHideMissingFuturePriceOrFX(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	price, _ := domain.ParseUnitPrice("1")
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price, SourceKind: domain.QuoteSourceAgent, ObservationKind: "close", EffectiveDate: "2026-10-09", ValueEffectiveAt: at, QuotedAt: at, PriceBasis: "agent_raw_close_v1", SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label"}
	quantity, _ := domain.ParseQuantity("10")
	instrument := domain.Instrument{ID: id, Type: domain.InstrumentCrypto, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceManual}
	p := domain.PortfolioSnapshot{Household: &domain.Household{ID: domain.NewHouseholdID(), BaseCurrency: "CNY"}}
	v := NewValuationService(nil, func() time.Time { return at.AddDate(0, 0, 10) })
	v.SetHistorical(true)
	v.SetHistoricalMarketDate("2026-10-19")
	holding := domain.Holding{ID: domain.NewHoldingID(), InstrumentID: id, Quantity: quantity}
	for _, tc := range []struct {
		name    string
		quotes  []domain.InstrumentQuote
		missing domain.MissingInputKind
	}{
		{"absent", nil, domain.MissingInstrumentPrice},
		{"future", []domain.InstrumentQuote{quote}, domain.MissingInstrumentPrice},
		{"missing FX", []domain.InstrumentQuote{quote}, domain.MissingFXRate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.InstrumentQuotes = tc.quotes
			if tc.name == "future" {
				v.SetHistoricalMarketDate("2026-10-08")
			} else {
				v.SetHistoricalMarketDate("2026-10-19")
			}
			c, m, err := v.valueHolding(p, domain.NewAccountID(), holding, instrument)
			if err != nil {
				t.Fatal(err)
			}
			if c.Available || len(m) != 1 || m[0].Kind != tc.missing {
				t.Fatalf("%+v %+v", c, m)
			}
		})
	}
}

func TestSnapshotDisplayUsesEffectiveManualPreference(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	preferenceID := domain.NewInstrumentPreferenceObservationID()
	quoteID := domain.NewInstrumentQuoteID()
	quoteRef := quoteID.String()
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	evidence := newSnapshotDisplayEvidence(domain.HistoricalSnapshotBatch{
		Portfolio:                 domain.PortfolioSnapshot{Instruments: []domain.Instrument{{ID: id, Type: domain.InstrumentCrypto, QuoteSource: domain.QuoteSourceProvider}}},
		OriginData:                domain.HistoryOriginData{InstrumentPreferences: []domain.HistoryOriginInstrumentPreference{{InstrumentID: id, SourceKind: domain.QuoteSourceAgent}}},
		InstrumentPreferenceFacts: []domain.InstrumentPreferenceObservation{{ID: preferenceID, InstrumentID: id, SourceKind: domain.QuoteSourceManual}},
		InstrumentQuoteFacts:      []domain.InstrumentQuote{{ID: quoteID, InstrumentID: id, SourceKind: domain.QuoteSourceAgent, EffectiveDate: "2026-10-09", ValueEffectiveAt: at, QuotedAt: at}},
	}, at.AddDate(0, 0, 10), 12*time.Hour)
	item := domain.DailyValuationSnapshotItem{InstrumentID: &id, QuoteID: &quoteRef, Complete: true, NativeCurrency: "CNY", NativeAmount: "10", BaseAmountExact: "10"}
	for _, manual := range []bool{false, true} {
		if manual {
			item.PreferenceObservationID = &preferenceID
		}
		c, m, _ := snapshotDisplayItem(item, evidence)
		if c.PriceEvidence.Source != domain.QuoteSourceAgent || c.PriceEvidence.EffectiveDate != "2026-10-09" {
			t.Fatal("lost provenance")
		}
		var a displayAccumulator
		if err := a.add(c, m, "2026-10-19", false); err != nil {
			t.Fatal(err)
		}
		if (a.quality.CarriedCount == 0) != manual || (a.quality.StaleCount == 0) != manual {
			t.Fatalf("effective manual=%v: %+v", manual, a.quality)
		}
	}
}
