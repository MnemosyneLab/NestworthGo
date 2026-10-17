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

func TestAnalysisContinuesAcrossSkippedCivilDateOnly(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	origin, err := domain.NewHistoryOrigin(householdID, "Pacific/Apia", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	account := analysisAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "Apia holding", Type: domain.InstrumentETF, QuoteCurrency: "CNY"}
	holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}
	portfolio := domain.PortfolioSnapshot{
		Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"},
		Accounts:  []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding},
	}
	quotes := make([]domain.InstrumentQuote, 0, 3)
	quoteIDs := make(map[string]string, 3)
	for _, datePrice := range [][2]string{{"2011-12-28", "100"}, {"2011-12-29", "100"}, {"2011-12-31", "110"}} {
		date, price := datePrice[0], datePrice[1]
		parsedDate, _ := time.Parse(time.DateOnly, date)
		unitPrice, priceErr := domain.ParseUnitPrice(price)
		if priceErr != nil {
			t.Fatal(priceErr)
		}
		quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: unitPrice, Currency: "CNY", QuotedAt: time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 9, 0, 0, 0, time.UTC)}
		quotes = append(quotes, quote)
		quoteIDs[date] = quote.ID.String()
	}
	item := func(date, amount string) domain.DailyValuationSnapshotItem {
		return analysisItem(t, account.ID, "CNY", amount, amount, &holdingID, &instrumentID, quoteIDs[date], "")
	}
	for _, testCase := range []struct {
		name       string
		from       string
		snapshots  []domain.DailyValuationSnapshot
		wantAmount string
		wantRate   string
	}{
		{
			name: "range crosses skipped label and ignores its snapshot",
			from: "2011-12-29",
			snapshots: []domain.DailyValuationSnapshot{
				analysisSnapshot("2011-12-28", item("2011-12-28", "100")),
				analysisSnapshot("2011-12-29", item("2011-12-29", "100")),
				analysisSnapshot("2011-12-30", item("", "999")),
				analysisSnapshot("2011-12-31", item("2011-12-31", "110")),
			},
			wantAmount: "10", wantRate: "0.1",
		},
		{
			name: "query starts on first existing date after skipped label",
			from: "2011-12-31",
			snapshots: []domain.DailyValuationSnapshot{
				analysisSnapshot("2011-12-29", item("2011-12-29", "100")),
				analysisSnapshot("2011-12-30", item("", "999")),
				analysisSnapshot("2011-12-31", item("2011-12-31", "110")),
			},
			wantAmount: "10", wantRate: "0.1",
		},
		{
			name: "missing existing civil day is not bridged",
			from: "2011-12-31",
			snapshots: []domain.DailyValuationSnapshot{
				analysisSnapshot("2011-12-28", item("2011-12-28", "100")),
				analysisSnapshot("2011-12-30", item("", "999")),
				analysisSnapshot("2011-12-31", item("2011-12-31", "110")),
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			query := domain.AnalysisQuery{
				Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
				From:  domain.LocalDate(testCase.from), To: "2011-12-31",
				Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment,
			}
			result, err := ComputeAnalysis(AnalysisInputs{Origin: origin, Portfolio: portfolio, Snapshots: testCase.snapshots, InstrumentQuotes: quotes}, query)
			if err != nil {
				t.Fatal(err)
			}
			var dec31 *domain.ComponentDay
			for index := range result.Days {
				if result.Days[index].Date == "2011-12-31" {
					dec31 = &result.Days[index]
					break
				}
			}
			if dec31 == nil {
				t.Fatal("missing 2011-12-31 component day")
			}
			if testCase.wantAmount == "" {
				if dec31.Status != domain.CompletenessUnavailable || dec31.ReturnAmount != nil {
					t.Fatalf("missing existing civil day was bridged: %+v", dec31)
				}
				return
			}
			if dec31.Status != domain.CompletenessOK || dec31.ReturnAmount == nil || dec31.ReturnAmount.CanonicalAmount() != testCase.wantAmount {
				t.Fatalf("2011-12-31 return = %+v, want complete amount %s", dec31, testCase.wantAmount)
			}
			lastDaily := result.DailyReturns[len(result.DailyReturns)-1]
			if lastDaily.Rate == nil || lastDaily.Rate.String() != testCase.wantRate {
				t.Fatalf("2011-12-31 daily rate = %+v, want %s", lastDaily.Rate, testCase.wantRate)
			}
			if testCase.from == "2011-12-29" {
				skipped := result.DailyReturns[1]
				if skipped.Date != "2011-12-30" || skipped.Status != domain.CompletenessUnavailable || skipped.Amount != nil {
					t.Fatalf("skipped daily return = %+v", skipped)
				}
			}
		})
	}
}

func TestAnalysisLoadInputsLooksBackToPreviousExistingCivilDate(t *testing.T) {
	t.Parallel()
	service, ctx, _, setClock := newOnboardedService(t, "apia-analysis-lookback", nil)
	setClock(time.Date(2011, 12, 29, 12, 0, 0, 0, time.UTC))
	if _, err := service.StartHistoryOnDate(ctx, "Pacific/Apia", "2011-12-29", nil); err != nil {
		t.Fatal(err)
	}
	setClock(time.Date(2012, 1, 2, 12, 0, 0, 0, time.UTC))
	analysis := NewAnalysisService(service.repository, service.clock)
	var ensuredFrom string
	analysis.SetSnapshotEnsurer(func(_ context.Context, from, _ string) error {
		ensuredFrom = from
		return nil
	})
	query := domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  "2011-12-31", To: "2011-12-31",
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment,
	}
	if _, err := analysis.loadInputs(ctx, query); err != nil {
		t.Fatal(err)
	}
	if ensuredFrom != "2011-12-29" {
		t.Fatalf("snapshot range begins on %s, want previous existing civil day 2011-12-29", ensuredFrom)
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
