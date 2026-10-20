package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestCurrentClosedCarryRequiresDailyAnchor(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	friday := time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)
	id := domain.NewInstrumentID()
	household := &domain.Household{ID: domain.NewHouseholdID(), BaseCurrency: "USD"}
	price, _ := domain.ParseUnitPrice("1.23456789")
	quantity, _ := domain.ParseQuantity("10")
	instrument := domain.Instrument{ID: id, Type: domain.InstrumentStock, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceAgent}
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price, SourceKind: domain.QuoteSourceAgent, QuotedAt: friday, ObservationKind: "latest"}
	v := NewValuationService(nil, func() time.Time { return now })
	for _, scenario := range []string{"intraday", "close", "missing-policy", "open-gap", "manual", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			i, q := instrument, quote
			if scenario != "intraday" {
				q.ObservationKind, q.EffectiveDate, q.ValueEffectiveAt = "close", "2026-10-16", friday
				q.SourcePolicyVersion, q.PriceBasis, q.TimestampBasis = "agent_raw_close_v1", "agent_raw_close_v1", "date_label"
			}
			if scenario == "missing-policy" {
				q.SourcePolicyVersion = ""
			}
			if scenario == "open-gap" {
				q.EffectiveDate = "2026-10-15"
			}
			if scenario == "manual" {
				i.QuoteSource = domain.QuoteSourceManual
			}
			quotes := []domain.InstrumentQuote{q}
			if scenario == "missing" {
				quotes = nil
			}
			snapshot := domain.PortfolioSnapshot{Household: household, InstrumentQuotes: quotes}
			c, _, err := v.valueHolding(snapshot, domain.NewAccountID(), domain.Holding{InstrumentID: id, Quantity: quantity}, i)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "missing" {
				if c.Available {
					t.Fatal("missing anchor became available")
				}
				return
			}
			e := c.PriceEvidence
			if e.ClosedCarry != (scenario == "close") {
				t.Fatalf("carry: %+v", e)
			}
			if scenario == "manual" {
				if e.Freshness != domain.FreshnessManual {
					t.Fatalf("manual: %+v", e)
				}
			} else if scenario != "close" && e.Freshness == domain.FreshnessFresh {
				t.Fatalf("ineligible quote certified fresh: %+v", e)
			}
			if c.BaseAmountExact != "12.3456789" || e.QuotedAt != q.QuotedAt || e.EffectiveDate != q.EffectiveDate {
				t.Fatalf("amount/provenance changed: %+v", c)
			}
			var display displayAccumulator
			if err := display.add(c, nil, "2026-10-18", true); err != nil {
				t.Fatal(err)
			}
			if scenario == "intraday" && (display.quality.ProvisionalCount != 1 || display.quality.StaleCount != 1 || display.quality.CarriedCount != 1) {
				t.Fatalf("intraday warning suppressed: %+v", display.quality)
			}
			if scenario == "intraday" && selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(i, quotes, nil, "2026-10-18", now).quote != nil {
				t.Fatal("intraday accepted as historical close")
			}
		})
	}
}

func TestCurrentFXClosedCarryRequiresDailyReference(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	friday := time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)
	household := &domain.Household{ID: domain.NewHouseholdID(), BaseCurrency: "USD"}
	v := NewValuationService(nil, func() time.Time { return now })
	for _, scenario := range []string{"intraday", "reference", "missing-policy", "open-gap", "manual-source", "manual-preference"} {
		t.Run(scenario, func(t *testing.T) {
			q := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: household.ID, BaseCurrency: "USD", QuoteCurrency: "CNY", SourceKind: domain.QuoteSourceAgent, QuotedAt: friday}
			if scenario != "intraday" {
				q.ObservationKind, q.EffectiveDate, q.ValueEffectiveAt, q.SourcePolicyVersion, q.TimestampBasis = "daily_reference", "2026-10-16", friday, "agent_daily_reference_v1", "date_label"
			}
			if scenario == "missing-policy" {
				q.SourcePolicyVersion = ""
			}
			if scenario == "open-gap" {
				q.EffectiveDate = "2026-10-15"
			}
			if scenario == "manual-source" {
				q.SourceKind = domain.QuoteSourceManual
			}
			snapshot := domain.PortfolioSnapshot{Household: household}
			if scenario == "manual-preference" {
				snapshot.FXPreferences = []domain.FXPreference{{HouseholdID: household.ID, CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}}
			}
			e := v.quoteEvidenceForFXConversion(snapshot, q)
			if e.ClosedCarry != (scenario == "reference") {
				t.Fatalf("carry: %+v", e)
			}
			if scenario != "reference" && e.Freshness == domain.FreshnessFresh {
				t.Fatalf("ineligible FX certified fresh: %+v", e)
			}
			if e.QuotedAt != q.QuotedAt || e.EffectiveDate != q.EffectiveDate {
				t.Fatalf("provenance changed: %+v", e)
			}
		})
	}
}
