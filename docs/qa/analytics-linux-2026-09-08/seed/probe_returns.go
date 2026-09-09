//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func main() {
	dbPath := os.Getenv("NESTWORTH_DATABASE_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("/workspace/nestworth-analytics-qa/data", "nestworth.db")
	}
	database, err := sqlite.Open(dbPath)
	if err != nil {
		panic(err)
	}
	defer database.Close()
	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()

	query := domain.AnalysisQuery{
		Scope:       domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:        envOr("NESTWORTH_PROBE_FROM", "2026-08-01"),
		To:          envOr("NESTWORTH_PROBE_TO", "2026-09-07"),
		Valuation:   domain.ValuationBase,
		IncludeCash: true,
		Basis:       domain.ReturnBasisInvestment,
	}
	result, err := svc.Analyze(ctx, query)
	if err != nil {
		panic(err)
	}
	ok, partial, unavailable := 0, 0, 0
	for _, day := range result.DailyReturns {
		switch day.Status {
		case domain.CompletenessOK:
			ok++
		case domain.CompletenessPartial:
			partial++
			fmt.Printf("PARTIAL %s amount=%v rate=%v capital=%v\n", day.Date, day.Amount, day.Rate, day.InvestedCapital)
		default:
			unavailable++
			fmt.Printf("UNAVAIL %s\n", day.Date)
		}
	}
	fmt.Printf("coverage rated=%d total=%d status=%s\n", result.Coverage.RatedDays, result.Coverage.TotalDays, result.Status)
	fmt.Printf("day_statuses ok=%d partial=%d unavailable=%d\n", ok, partial, unavailable)

	contrib, err := svc.Contribution(ctx, query, application.ContributionTotalReturn, application.ContributionGroupInstrument, application.ContributionSortAmountDesc)
	if err != nil {
		panic(err)
	}
	fmt.Printf("contribution available=%v status=%s reason=%q rated=%d/%d rows=%d valuationForced=%v\n",
		contrib.Available, contrib.Status, contrib.MissingReason, contrib.Coverage.RatedDays, contrib.Coverage.TotalDays, len(contrib.Rows), contrib.ValuationForced)
	for _, row := range contrib.Rows {
		fmt.Printf("  row key=%s amount=%v rate=%v rated=%d/%d status=%s\n", row.Key, row.Amount, row.Rate, row.Coverage.RatedDays, row.Coverage.TotalDays, row.Status)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
