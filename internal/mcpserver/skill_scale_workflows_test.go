package mcpserver

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestSkillScaleExamplesActualCalls(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	fx.app.SetInvestmentReviewConfiguration(&memoryInvestmentReviewStore{})
	fund, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Example NAV fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: fund.ID.String(), Quantity: "10", UnitCost: "1"})
	if err != nil {
		t.Fatal(err)
	}
	source := domain.HoldingSourceRef(domain.AccountID(fx.brokerage), holding.ID)
	zero, calendar := 0, "calendar"
	if _, err := fx.app.SaveLiquidityPolicy(t.Context(), application.SavePolicyInput{Source: source, Policy: application.ProductPolicyInput{AccessKind: "on_request", SettlementDays: &zero, DayBasis: &calendar, EarlyKind: "not_allowed"}}); err != nil {
		t.Fatal(err)
	}
	fx.app.SetClock(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	quotes := []application.AgentMarketDataItem{}
	for n := 0; n < 43; n++ {
		quotes = append(quotes, application.AgentMarketDataItem{InstrumentID: fund.ID.String(), Currency: "USD", Value: "1", Kind: "nav", Date: time.Date(2026, 9, 1+n, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), Timezone: "UTC", SourceTitle: "Synthetic NAV"})
	}
	if _, err := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: quotes}); err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	values := map[string]string{"instrumentId": fund.ID.String()}
	cap := scaleCapture(t, c, "get_liquidity_overview", skillExample(t, "paging-and-batches", "scale-liquidity-funds", nil))
	if len(scaleSection(t, c, cap, "sources")) != 1 {
		t.Fatal("fund/explicit/sort example failed")
	}
	scaleSection(t, c, cap, "unresolvedReservations")
	unconfigured := scaleCapture(t, c, "get_liquidity_overview", skillExample(t, "paging-and-batches", "scale-liquidity-unconfigured", nil))
	for _, v := range scaleSection(t, c, unconfigured, "sources") {
		if v.(map[string]any)["hasRule"] != false {
			t.Fatal("unconfigured example")
		}
	}
	for _, view := range []string{"current", "coverage", "history"} {
		cap := scaleCapture(t, c, "get_market_data_batch", skillExample(t, "paging-and-batches", "scale-market-"+view, values))
		for _, raw := range scaleSection(t, c, cap, "targets") {
			v := raw.(map[string]any)
			if view == "history" {
				targetCap := map[string]any{"captureId": cap["captureId"], "contentHash": cap["contentHash"], "sections": v["sections"]}
				scaleSection(t, c, targetCap, v["pointsSection"].(string))
				scaleSection(t, c, targetCap, v["observationsSection"].(string))
			}
		}
	}
	instrument := scaleCapture(t, c, "list_instruments", skillExample(t, "paging-and-batches", "scale-instruments", values))
	rows := scaleSection(t, c, instrument, "instruments")
	values["metadataVersion"] = rows[0].(map[string]any)["metadataVersion"].(string)
	values["operationId"] = uuid.NewString()
	call(t, c, "update_instruments_batch", skillExample(t, "paging-and-batches", "scale-metadata", values), false)
	for _, name := range []string{"enrollment", "snooze", "resume"} {
		values["operationId"] = uuid.NewString()
		tool := "set_investment_review_snooze_batch"
		if name == "enrollment" {
			tool = "set_investment_review_enrollment_batch"
		}
		call(t, c, tool, skillExample(t, "paging-and-batches", "scale-review-"+name, values), false)
	}
	report, err := fx.app.InvestmentReviews(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	values["episodeId"] = report.Items[0].EpisodeID
	values["operationId"] = uuid.NewString()
	call(t, c, "acknowledge_investment_reviews_batch", skillExample(t, "paging-and-batches", "scale-review-ack", values), false)
}
