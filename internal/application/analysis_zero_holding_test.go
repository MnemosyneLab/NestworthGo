package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAnalysisZeroHoldingDaysBeforePurchase(t *testing.T) {
	input, account, _, _ := reviewTradeScenario(t)
	// The manual product and its first quote only exist on the purchase day.
	input.Activities[0].EffectiveAt = input.Activities[0].EffectiveAt.AddDate(0, 0, 2)
	input.Activities[0].EffectiveLocalDate = "2026-08-04"
	input.InstrumentQuotes[0].QuotedAt = input.InstrumentQuotes[0].QuotedAt.AddDate(0, 0, 2)
	input.InstrumentQuotes[0].SourceKind = domain.QuoteSourceManual
	input.Portfolio.Instruments[0].Type = domain.InstrumentBankInvestmentProduct
	input.Portfolio.Instruments[0].QuoteSource = domain.QuoteSourceManual
	purchased := analysisSnapshot("2026-08-04", input.Snapshots[1].Items...)
	input.Snapshots = nil
	for _, date := range []string{"2026-08-01", "2026-08-02", "2026-08-03"} {
		// A complete snapshot legitimately omits a holding that does not exist yet.
		input.Snapshots = append(input.Snapshots, analysisSnapshot(date,
			analysisItem(t, account.ID, "CNY", "100", "100", nil, nil, "", "")))
	}
	input.Snapshots = append(input.Snapshots, purchased)
	query := analysisBaseQuery(domain.ValuationBase)
	query.To = "2026-08-04"
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.CompletenessOK || result.Coverage.RatedDays != 3 || result.Coverage.TotalDays != 3 {
		t.Fatalf("inactive days reduced coverage: status=%s coverage=%+v", result.Status, result.Coverage)
	}
	if result.ReturnAmount == nil || !result.ReturnAmount.Amount().IsZero() {
		t.Fatalf("purchase should not create a return: %+v", result.ReturnAmount)
	}
	for _, day := range result.Days {
		if day.Status != domain.CompletenessOK || day.Residual != nil {
			t.Fatalf("component day incomplete: %+v", day)
		}
	}
	asset := foldAssetChange(result, "")
	fx := foldAssetDriverDetail(result, "", string(domain.BucketFXImpact))
	if asset.Status != domain.CompletenessOK || asset.Summary.ChangeRate == nil || fx.Status != domain.CompletenessOK {
		t.Fatalf("inactive days leaked into projections: asset=%+v fx=%+v", asset, fx)
	}
}

func TestAnalysisZeroHoldingDoesNotHideMissingValuationsOrTrading(t *testing.T) {
	for _, scenario := range []string{"missing-opening-value", "incomplete-closing-item", "nonzero-opening", "same-day-trading"} {
		t.Run(scenario, func(t *testing.T) {
			input, account, instrument, holdingID := reviewTradeScenario(t)
			input.InstrumentQuotes = nil
			input.Activities = nil
			previous := analysisItem(t, account.ID, "CNY", "0", "0", &holdingID, &instrument.ID, "", "")
			current := previous
			switch scenario {
			case "missing-opening-value":
				previous.NativeAmount = ""
			case "incomplete-closing-item":
				current.Complete = false
			case "nonzero-opening":
				previous = analysisItem(t, account.ID, "CNY", "100", "100", &holdingID, &instrument.ID, "", "")
			case "same-day-trading":
				state := reviewChangeState(t, account.HouseholdID, reviewAccountState(t, account, "0"))
				state.Now = time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC)
				state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"CNY": mustMoney(t, "100", "CNY")}
				state.Holdings[holdingID] = domain.ChangeHoldingState{ID: holdingID, AccountID: account.ID, InstrumentID: instrument.ID, Currency: "CNY", Current: mustQuantity(t, "0")}
				state.Instruments[instrument.ID] = instrument
				when := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
				for index, side := range []domain.TradeSide{domain.TradeBuy, domain.TradeSell} {
					input.Activities = append(input.Activities, reviewApplyChange(t, &state, domain.TradeInput{
						HouseholdID: account.HouseholdID, Side: side, SettlementAccountID: account.ID, HoldingID: holdingID,
						InstrumentID: instrument.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "100", "CNY"), EffectiveAt: when.Add(time.Duration(index) * time.Minute),
					}))
				}
			}
			input.Snapshots = []domain.DailyValuationSnapshot{analysisSnapshot("2026-08-01", previous), analysisSnapshot("2026-08-02", current)}
			result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
			if err != nil {
				t.Fatal(err)
			}
			day := analysisFindDay(t, result, domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrument.ID, Currency: "CNY"})
			if day.Status == domain.CompletenessOK || day.ReturnRate != nil {
				t.Fatalf("missing valuation or trading evidence was hidden: %+v", day)
			}
		})
	}
}
