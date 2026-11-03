package analysis

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestContributionSummaryWirePreservesAmountsAndAvailability(t *testing.T) {
	t.Parallel()
	total, err := domain.NewSignedMoney(decimal.RequireFromString("0.005"), "USD")
	if err != nil {
		t.Fatal(err)
	}
	got := fromContribution(application.ContributionResult{Summary: application.ContributionSummary{Total: &total, AmountStatus: application.ReturnAmountPartial}})
	if got.Summary.Total == nil || got.Summary.Total.Amount != "0.005" || got.Summary.Total.Currency != "USD" || got.Summary.AmountStatus != "partial" || got.Summary.Positive != nil || got.Summary.Negative != nil {
		t.Fatalf("summary wire=%+v", got.Summary)
	}
	unavailable := fromContribution(application.ContributionResult{Summary: application.ContributionSummary{AmountStatus: application.ReturnAmountUnavailable}})
	if unavailable.Summary.Total != nil || unavailable.Summary.AmountStatus != "unavailable" {
		t.Fatalf("missing amount fabricated: %+v", unavailable.Summary)
	}
}
