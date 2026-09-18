package application

import (
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestPricedPositionAdjustmentAttributesSubsequentPriceMovement(t *testing.T) {
	for _, tc := range []struct {
		name, initial, final, openPrice, cost, closePrice, adjustment, price string
		added                                                                bool
	}{
		{"silver first holding", "0", "1", "14.1144701", "14.51148682", "14.33939583", "14.51148682", "-0.17209099", true},
		{"increase existing holding", "2", "3", "10", "11", "12", "11", "5", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := domain.NewHouseholdID()
			account := analysisAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
			id, hid := domain.NewInstrumentID(), domain.NewHoldingID()
			instrument := domain.Instrument{ID: id, HouseholdID: h, Name: "silver", Type: domain.InstrumentPreciousMetal, QuoteCurrency: "CNY"}
			state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
			state.Holdings[hid] = domain.ChangeHoldingState{ID: hid, AccountID: account.ID, InstrumentID: id, Currency: "CNY", Current: mustQuantity(t, tc.initial)}
			cost := mustUnitPrice(t, tc.cost)
			activity := reviewApplyChange(t, &state, domain.PositionAdjustmentInput{HouseholdID: h, HoldingID: hid, Quantity: mustQuantity(t, "1"), Added: tc.added, UnitCost: &cost, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
			open := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, UnitPrice: mustUnitPrice(t, tc.openPrice), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
			close := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, UnitPrice: mustUnitPrice(t, tc.closePrice), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
			begin := decimal.RequireFromString(tc.initial).Mul(open.UnitPrice.Decimal()).String()
			end := decimal.RequireFromString(tc.final).Mul(close.UnitPrice.Decimal()).String()
			previous := analysisSnapshot("2026-08-01", reviewExactItem(t, account.ID, "CNY", begin, begin, &hid, &id, open.ID.String(), ""))
			current := analysisSnapshot("2026-08-02", reviewExactItem(t, account.ID, "CNY", end, end, &hid, &id, close.ID.String(), ""))
			portfolio := reviewPortfolio(&domain.Household{ID: h, BaseCurrency: "CNY"}, account)
			portfolio.Instruments = []domain.Instrument{instrument}
			portfolio.Holdings = []domain.Holding{{ID: hid, AccountID: account.ID, InstrumentID: id}}
			input := AnalysisInputs{Origin: analysisOrigin(t, h, "UTC"), Portfolio: portfolio, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{open, close}}
			result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
			if err != nil {
				t.Fatal(err)
			}
			day := analysisFindDay(t, result, domain.ComponentID{AccountID: account.ID, HoldingID: &hid, InstrumentID: &id, Currency: "CNY"})
			if day.Status != domain.CompletenessOK || day.Residual != nil {
				t.Fatalf("incomplete or residual: %+v", day)
			}
			if !day.AssetBucketExact[domain.BucketPriceChange].Equal(decimal.RequireFromString(tc.price)) || !day.AssetBucketExact[domain.BucketAdjustment].Equal(decimal.RequireFromString(tc.adjustment)) {
				t.Fatalf("wrong attribution: %+v", day.AssetBucketExact)
			}
			if got := day.ReturnComponents[domain.ReturnPriceChange].Amount(); !got.Equal(decimal.RequireFromString(tc.price).Round(4)) {
				t.Fatalf("return=%s", got)
			}
		})
	}
}
