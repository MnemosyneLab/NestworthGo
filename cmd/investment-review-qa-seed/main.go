// investment-review-qa-seed creates a new, synthetic-only isolated household.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func main() {
	dir := flag.String("dir", "", "required new isolated QA directory")
	anchor := flag.String("date", time.Now().UTC().Format(time.DateOnly), "synthetic evaluation date (YYYY-MM-DD)")
	flag.Parse()
	if err := seed(*dir, *anchor); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func seed(dir, date string) error {
	if dir == "" {
		return fmt.Errorf("-dir is required; use a new temporary QA directory")
	}
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return err
	}
	// Refuse existing paths, including symlinks. This command never resets data.
	if err = os.Mkdir(dir, 0700); err != nil {
		return err
	}
	db, err := sqlite.Open(filepath.Join(dir, "household.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	s := application.NewService(sqlite.NewRepository(db))
	s.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	s.SetClock(func() time.Time { return day.Add(-100 * 24 * time.Hour) })
	ctx := context.Background()
	if err = s.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic investment review QA", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}}); err != nil {
		return err
	}
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	account, err := s.CreateAccount(ctx, application.AccountInput{Name: "Synthetic fund account", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		return err
	}
	s.SetClock(func() time.Time { return day.Add(12 * time.Hour) })
	ids := map[string]string{}
	for _, scenario := range []string{"flat", "loss", "missing", "distribution", "stale"} {
		i, err := s.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic " + scenario + " fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
		if err != nil {
			return err
		}
		ids[scenario] = i.ID.String()
		if _, err = s.CreateHolding(ctx, application.HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"}); err != nil {
			return err
		}
		quotes := []application.AgentMarketDataItem{}
		for n := 0; n < 50; n++ {
			observed := day.AddDate(0, 0, -53+n)
			if scenario == "missing" && n < 48 {
				continue
			}
			if scenario == "stale" && n > 36 {
				continue
			}
			value := "1"
			if scenario == "loss" && n >= 30 {
				value = "0.98"
			}
			item := application.AgentMarketDataItem{InstrumentID: i.ID.String(), Currency: "USD", Value: value, Kind: "nav", Date: observed.Format(time.DateOnly), Timezone: "UTC", SourceTitle: "Synthetic QA observation"}
			if scenario == "distribution" && n == 40 {
				item.DividendCash = "0.01"
			}
			quotes = append(quotes, item)
		}
		if _, err = s.ImportAgentMarketData(ctx, uuid.NewString(), application.AgentMarketDataInput{Items: quotes}); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Database    string            `json:"database"`
		Date        string            `json:"evaluationDate"`
		Instruments map[string]string `json:"instruments"`
	}{filepath.Join(dir, "household.db"), date, ids})
}
