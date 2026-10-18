// seed.go creates a small synthetic household for the PR101/PR102 review
// replay reproducer. It deliberately uses application.Service for all ledger,
// history, instrument, and quote writes; it does not write database rows.
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

type fixtureSummary struct {
	Database       string `json:"database"`
	HouseholdID    string `json:"householdId"`
	HistoryOrigin  string `json:"historyOrigin"`
	OriginCreated  string `json:"originCreatedAt"`
	AccountID      string `json:"accountId"`
	AccountCreated string `json:"accountCreatedAt"`
	AccountOpened  string `json:"accountOpenedOn"`
	InstrumentID   string `json:"instrumentId"`
	ContributionAt string `json:"contributionEffectiveAt"`
	ContributionBy string `json:"contributionCreatedAt"`
	BuyAt          string `json:"buyEffectiveAt"`
	BuyBy          string `json:"buyCreatedAt"`
	QuoteCount     int    `json:"quoteCount"`
	QuoteFrom      string `json:"quoteFrom"`
	QuoteThrough   string `json:"quoteThrough"`
	QuoteImported  string `json:"quoteImportedAt"`
	SnapshotStatus string `json:"snapshotStatus"`
}

func main() {
	dir := flag.String("dir", "", "required new synthetic QA directory")
	flag.Parse()
	if err := seed(*dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func seed(dir string) error {
	if dir == "" {
		return fmt.Errorf("-dir is required; use a new temporary QA directory")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("create fresh QA directory: %w", err)
	}
	databasePath, err := filepath.Abs(filepath.Join(dir, "household.db"))
	if err != nil {
		return err
	}
	// Pin the same isolated file in the environment as well as passing it to
	// sqlite.Open. An inherited profile path must never select a user's ledger.
	if err := os.Setenv("NESTWORTH_DATABASE_PATH", databasePath); err != nil {
		return err
	}
	db, err := sqlite.Open(databasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	s := application.NewService(sqlite.NewRepository(db))
	clock := func(value string) {
		instant, parseErr := time.Parse(time.RFC3339Nano, value)
		if parseErr != nil {
			panic(parseErr)
		}
		s.SetClock(func() time.Time { return instant.UTC() })
	}

	clock("2026-10-18T00:35:19.076Z")
	if err := s.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName:    "Synthetic review replay QA",
		BaseCurrency:     "USD",
		Timezone:         "UTC",
		HistoryStartDate: "2026-10-01",
		MemberNames:      []string{"Synthetic owner"},
	}); err != nil {
		return fmt.Errorf("complete synthetic onboarding: %w", err)
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if len(bootstrap.Members) == 0 {
		return fmt.Errorf("synthetic owner was not created")
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return fmt.Errorf("read synthetic History Origin: %w", err)
	}
	if origin == nil {
		return fmt.Errorf("synthetic History Origin was not created")
	}

	openedOn := "2026-10-01"
	clock("2026-10-18T00:35:37.890Z")
	account, err := s.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic review account opened Oct 1", AccountType: "brokerage",
		BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD",
		IncludeInNetWorth: true, IncludeInPortfolio: true,
		OpenedOn: &openedOn, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		return fmt.Errorf("create synthetic account: %w", err)
	}

	clock("2026-10-18T00:35:47.462Z")
	instrument, err := s.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic opened daily NAV fund", Type: "mutual_fund",
		QuoteCurrency: "USD", QuoteSource: "agent", Symbol: "SYNOPEN", CountryCode: "US",
	})
	if err != nil {
		return fmt.Errorf("create synthetic instrument: %w", err)
	}

	contributionEffective, _ := time.Parse(time.RFC3339Nano, "2026-10-01T00:10:00.000Z")
	clock("2026-10-18T00:36:21.100Z")
	contribution, err := domain.ParseMoney("2000", "USD")
	if err != nil {
		return err
	}
	if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{
		HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID,
		Amount: contribution, Reason: domain.ReasonContribution,
		EffectiveAt: contributionEffective,
	}); err != nil {
		return fmt.Errorf("record synthetic contribution: %w", err)
	}

	buyEffective, _ := time.Parse(time.RFC3339Nano, "2026-10-01T00:20:00.000Z")
	clock("2026-10-18T00:36:24.077Z")
	quantity, err := domain.ParseQuantity("10")
	if err != nil {
		return err
	}
	gross, err := domain.ParseMoney("1000", "USD")
	if err != nil {
		return err
	}
	fee, err := domain.ParseMoney("5", "USD")
	if err != nil {
		return err
	}
	if _, err := s.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy,
		SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID,
		Quantity: quantity, Gross: gross, Fee: &fee, EffectiveAt: buyEffective,
	}); err != nil {
		return fmt.Errorf("record synthetic buy: %w", err)
	}

	clock("2026-10-18T12:00:00.000Z")
	quotes := make([]application.AgentMarketDataItem, 0, 17)
	for day := 1; day <= 17; day++ {
		date := time.Date(2026, time.October, day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		quotes = append(quotes, application.AgentMarketDataItem{
			InstrumentID: instrument.ID.String(), Currency: "USD", Value: "100",
			Kind: "nav", Date: date, Timezone: "UTC", SourceTitle: "Synthetic QA NAV observation",
		})
	}
	imported, err := s.ImportAgentMarketData(ctx, uuid.NewString(), application.AgentMarketDataInput{Items: quotes})
	if err != nil {
		return fmt.Errorf("import synthetic daily NAVs: %w", err)
	}

	summary := fixtureSummary{
		Database: databasePath, HouseholdID: bootstrap.Household.ID.String(),
		HistoryOrigin: origin.StartedAt.UTC().Format(time.RFC3339Nano),
		OriginCreated: origin.CreatedAt.UTC().Format(time.RFC3339Nano),
		AccountID:     account.Account.ID.String(), AccountCreated: account.Account.CreatedAt.UTC().Format(time.RFC3339Nano),
		AccountOpened: openedOn, InstrumentID: instrument.ID.String(),
		ContributionAt: contributionEffective.UTC().Format(time.RFC3339Nano),
		ContributionBy: "2026-10-18T00:36:21.100Z", BuyAt: buyEffective.UTC().Format(time.RFC3339Nano),
		BuyBy: "2026-10-18T00:36:24.077Z", QuoteCount: imported.Inserted,
		QuoteFrom: "2026-10-01", QuoteThrough: "2026-10-17", QuoteImported: "2026-10-18T12:00:00.000Z",
		SnapshotStatus: imported.SnapshotStatus,
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}
