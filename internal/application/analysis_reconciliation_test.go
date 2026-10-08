package application

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReconciliationRemovalUsesCarryingValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, removed, closingPrice, closingQuantity, adjustment, price string
		missingQuote                                                    bool
	}{
		{"clear position", "3", "110", "0", "-300", "0", false},
		{"reduce position", "1", "110", "2", "-100", "20", false},
		{"reverse split", "2", "300", "1", "0", "0", false},
		{"missing opening quote", "3", "110", "0", "0", "0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := domain.NewHouseholdID()
			account := analysisAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
			id, hid := domain.NewInstrumentID(), domain.NewHoldingID()
			instrument := domain.Instrument{ID: id, HouseholdID: h, Type: domain.InstrumentStock, QuoteCurrency: "CNY"}
			state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
			state.Holdings[hid] = domain.ChangeHoldingState{ID: hid, AccountID: account.ID, InstrumentID: id, Currency: "CNY", Current: mustQuantity(t, "3")}
			activity := reviewApplyChange(t, &state, domain.PositionAdjustmentInput{HouseholdID: h, HoldingID: hid, Quantity: mustQuantity(t, tc.removed), Added: false, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
			open := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
			close := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, UnitPrice: mustUnitPrice(t, tc.closingPrice), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
			closing := decimal.RequireFromString(tc.closingPrice).Mul(decimal.RequireFromString(tc.closingQuantity)).String()
			previous := analysisSnapshot("2026-08-01", reviewExactItem(t, account.ID, "CNY", "300", "300", &hid, &id, open.ID.String(), ""))
			quoteID := close.ID.String()
			if tc.closingQuantity == "0" {
				quoteID = ""
			}
			current := analysisSnapshot("2026-08-02", reviewExactItem(t, account.ID, "CNY", closing, closing, &hid, &id, quoteID, ""))
			portfolio := reviewPortfolio(&domain.Household{ID: h, BaseCurrency: "CNY"}, account)
			portfolio.Instruments = []domain.Instrument{instrument}
			portfolio.Holdings = []domain.Holding{{ID: hid, AccountID: account.ID, InstrumentID: id}}
			input := AnalysisInputs{Origin: analysisOrigin(t, h, "UTC"), Portfolio: portfolio, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{open, close}}
			if tc.missingQuote {
				input.InstrumentQuotes = []domain.InstrumentQuote{close}
			}
			result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
			if err != nil {
				t.Fatal(err)
			}
			day := result.Days[0]
			if tc.missingQuote {
				if day.Status == domain.CompletenessOK || day.Residual == nil {
					t.Fatalf("missing quote hidden: %+v", day)
				}
				return
			}
			if day.Status != domain.CompletenessOK || day.Residual != nil {
				t.Fatalf("incomplete removal: %+v", day)
			}
			if !analysisBucket(day, domain.BucketAdjustment).Equal(decimal.RequireFromString(tc.adjustment)) || !analysisBucket(day, domain.BucketPriceChange).Equal(decimal.RequireFromString(tc.price)) {
				t.Fatalf("incorrect attribution: %+v", day.AssetBucketExact)
			}
			if len(day.DietzCapitalFlows) != 0 {
				t.Fatal("reconciliation became external cash flow")
			}
			reviewAssertIdentity(t, day, closing)
		})
	}
}

func TestAnalysisAccountBeforeCreationIsNotMissing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		created  time.Time
		balance  string
		complete bool
	}{
		{"new zero account", time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), "0", true},
		{"new nonzero account needs attribution", time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), "50", false},
		{"existing account missing snapshot", time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC), "0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := domain.NewHouseholdID()
			account := analysisAccount(h, "CNY", domain.TrackingBalance, domain.RoleLiability)
			account.CreatedAt = tc.created
			input := AnalysisInputs{Origin: analysisOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(&domain.Household{ID: h, BaseCurrency: "CNY"}, account), Snapshots: []domain.DailyValuationSnapshot{analysisSnapshot("2026-08-01"), analysisSnapshot("2026-08-02", analysisItem(t, account.ID, "CNY", tc.balance, tc.balance, nil, nil, "", ""))}}
			result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Days[0].Status == domain.CompletenessOK; got != tc.complete {
				t.Fatalf("complete=%v want %v: %+v", got, tc.complete, result.Days[0])
			}
		})
	}
}
