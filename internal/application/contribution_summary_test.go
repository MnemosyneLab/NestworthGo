package application

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestContributionSummaryPreservesExactKnownAmountsAndMissingStates(t *testing.T) {
	t.Parallel()
	known := AnalysisAvailability{Available: true, Status: domain.CompletenessOK}
	row := func(amount string) ContributionRow {
		return ContributionRow{Amount: signedPointer(decimal.RequireFromString(amount), "USD"), AnalysisAvailability: known}
	}
	for _, test := range []struct {
		name                              string
		rows                              []ContributionRow
		availability                      AnalysisAvailability
		status, total, positive, negative string
	}{
		{"exact subcent fold", []ContributionRow{row("0.004"), row("0.004"), row("-0.003")}, known, ReturnAmountComplete, "0.005", "0.008", "-0.003"},
		{"known zero", []ContributionRow{row("0")}, known, ReturnAmountComplete, "0", "0", "0"},
		{"missing row", []ContributionRow{row("2"), {}}, known, ReturnAmountPartial, "2", "2", "0"},
		{"negative only", []ContributionRow{row("-5")}, known, ReturnAmountComplete, "-5", "0", "-5"},
		{"period partial", []ContributionRow{row("3")}, AnalysisAvailability{Available: true, Status: domain.CompletenessPartial}, ReturnAmountPartial, "3", "3", "0"},
		{"all unknown", []ContributionRow{{}}, known, ReturnAmountUnavailable, "", "", ""},
		{"unavailable view", []ContributionRow{row("9")}, unavailableReturn("missing"), ReturnAmountUnavailable, "", "", ""},
		{"empty view", nil, known, ReturnAmountUnavailable, "", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := summarizeContribution(test.rows, test.availability)
			if err != nil {
				t.Fatal(err)
			}
			if got.AmountStatus != test.status {
				t.Fatalf("status %s, want %s", got.AmountStatus, test.status)
			}
			for i, amount := range []*domain.SignedMoney{got.Total, got.Positive, got.Negative} {
				want := []string{test.total, test.positive, test.negative}[i]
				if want == "" {
					if amount != nil {
						t.Fatalf("unknown aggregate fabricated: %v", amount)
					}
					continue
				}
				if amount == nil || amount.CanonicalAmount() != want || amount.Currency() != "USD" {
					t.Fatalf("aggregate %d = %v, want %s USD", i, amount, want)
				}
			}
		})
	}
}

func TestContributionSummaryRejectsMixedCurrencies(t *testing.T) {
	t.Parallel()
	_, err := summarizeContribution([]ContributionRow{{Amount: signedPointer(decimal.NewFromInt(1), "USD")}, {Amount: signedPointer(decimal.NewFromInt(1), "CNY")}}, AnalysisAvailability{Available: true, Status: domain.CompletenessOK})
	if err == nil {
		t.Fatal("mixed currency summary accepted")
	}
}

func TestContributionSummaryOverflowDoesNotDiscardValidRows(t *testing.T) {
	t.Parallel()
	availability := AnalysisAvailability{Available: true, Status: domain.CompletenessOK}
	rows := []ContributionRow{
		{Key: "first", Amount: signedPointer(decimal.RequireFromString("999999999999.9999"), "USD"), AnalysisAvailability: availability},
		{Key: "second", Amount: signedPointer(decimal.RequireFromString("0.0001"), "USD"), AnalysisAvailability: availability},
	}
	result := withContributionSummary(ContributionResult{AnalysisAvailability: availability, ReturnType: ContributionRealized, GroupBy: ContributionGroupInstrument, Rows: rows})
	if !result.Available || result.ReturnType != ContributionRealized || len(result.Rows) != 2 || result.Rows[0].Amount != rows[0].Amount || result.Summary.AmountStatus != ReturnAmountUnavailable || result.Summary.Total != nil {
		t.Fatalf("optional summary replaced valid response: %+v", result)
	}
}
