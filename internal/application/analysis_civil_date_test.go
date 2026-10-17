package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAnalysisCivilDateLabelsRemainMonotonicAcrossTimezoneTransitions(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name        string
		timezone    string
		from        string
		to          string
		nonexistent string
	}{
		{name: "Apia skipped day", timezone: "Pacific/Apia", from: "2011-12-29", to: "2011-12-31", nonexistent: "2011-12-30"},
		{name: "Havana midnight transition", timezone: "America/Havana", from: "2026-03-07", to: "2026-03-09"},
		{name: "Santiago midnight transition", timezone: "America/Santiago", from: "2024-09-07", to: "2024-09-09"},
		{name: "Shanghai control", timezone: "Asia/Shanghai", from: "2026-03-07", to: "2026-03-09"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			householdID := domain.NewHouseholdID()
			startedAt := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
			origin, err := domain.NewHistoryOrigin(householdID, testCase.timezone, startedAt, startedAt)
			if err != nil {
				t.Fatal(err)
			}
			account := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
			input := AnalysisInputs{Origin: origin, Portfolio: domain.PortfolioSnapshot{
				Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"},
				Accounts:  []domain.AccountRecord{{Account: account}},
			}}
			query := domain.AnalysisQuery{
				Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
				From:  domain.LocalDate(testCase.from), To: domain.LocalDate(testCase.to),
				Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment,
			}
			from, _ := time.Parse(time.DateOnly, testCase.from)
			previousDate := from.AddDate(0, 0, -1).Format(time.DateOnly)
			input.Snapshots = append(input.Snapshots, analysisSnapshot(previousDate, analysisItem(t, account.ID, "CNY", "100", "100", nil, nil, "", "")))
			to, _ := time.Parse(time.DateOnly, testCase.to)
			for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
				amount := "100"
				if day.Format(time.DateOnly) == testCase.nonexistent {
					amount = "999"
				}
				input.Snapshots = append(input.Snapshots, analysisSnapshot(day.Format(time.DateOnly), analysisItem(t, account.ID, "CNY", amount, amount, nil, nil, "", "")))
			}
			result, err := ComputeAnalysis(input, query)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := int(to.Sub(from)/(24*time.Hour)) + 1
			if len(result.DailyReturns) != wantCount {
				t.Fatalf("daily returns = %d, want %d: %+v", len(result.DailyReturns), wantCount, result.DailyReturns)
			}
			for index, daily := range result.DailyReturns {
				wantDate := domain.LocalDate(from.AddDate(0, 0, index).Format(time.DateOnly))
				if daily.Date != wantDate {
					t.Fatalf("daily return %d date = %s, want %s", index, daily.Date, wantDate)
				}
				if daily.Date == domain.LocalDate(testCase.nonexistent) && (daily.Amount != nil || daily.Status != domain.CompletenessUnavailable) {
					t.Fatalf("skipped synthetic civil date fabricated data: %+v", daily)
				}
			}
			if testCase.nonexistent != "" {
				foundSkippedDay := false
				for _, componentDay := range result.Days {
					if componentDay.Date == domain.LocalDate(testCase.nonexistent) {
						foundSkippedDay = true
						if componentDay.ReturnAmount != nil || componentDay.Status != domain.CompletenessUnavailable {
							t.Fatalf("skipped snapshot was treated as an observation: %+v", componentDay)
						}
					}
				}
				if !foundSkippedDay {
					t.Fatal("skipped civil label is missing from component-day results")
				}
			}
		})
	}
}

func TestAnalysisRangeIsBoundedAndCancellationIsObserved(t *testing.T) {
	t.Parallel()
	query := domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  "0001-01-01", To: "9999-12-31",
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment,
	}
	if _, err := ComputeAnalysis(AnalysisInputs{}, query); err == nil {
		t.Fatal("accepted an analysis range longer than the bounded period contract")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	query.From, query.To = "2026-03-07", "2026-03-09"
	if _, err := computeAnalysisContext(cancelled, AnalysisInputs{}, query); err != context.Canceled {
		t.Fatalf("canceled analysis error = %v, want context.Canceled", err)
	}
}
