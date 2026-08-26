package application

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestGainReadPathsAreConcurrentAndDoNotWriteFinancialFacts(t *testing.T) {
	fixture := newGoldenValuationFixture(t, true)
	ctx := context.Background()
	if _, err := fixture.service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	holdingID := findHoldingForInstrument(t, fixture.repository, fixture.account.Account.ID, fixture.qqq.ID)
	before := financialFactCounts(t, fixture.database.SQL)

	var group sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := fixture.service.HoldingGain(ctx, holdingID); err != nil {
				errs <- err
				return
			}
			if _, err := fixture.service.AccountGain(ctx, fixture.account.Account.ID); err != nil {
				errs <- err
				return
			}
			_, err := fixture.service.RealizedGain(ctx, domain.GainScope{}, domain.TrendAllTime)
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	after := financialFactCounts(t, fixture.database.SQL)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("gain read paths changed financial facts: before=%v after=%v", before, after)
	}
}

func TestSchema6FixtureSupportsGainReads(t *testing.T) {
	database := seedGainSchema6Fixture(t)
	defer database.Close()
	repository := sqlite.NewRepository(database)
	ctx := context.Background()
	snapshot, err := repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Household == nil || len(snapshot.Accounts) == 0 {
		t.Fatal("migrated fixture has no Household or accounts")
	}
	gain := NewGainService(repository, func() time.Time { return time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC) })
	for _, account := range snapshot.Accounts {
		view, viewErr := gain.AccountGain(ctx, account.Account.ID)
		if viewErr != nil {
			t.Fatalf("AccountGain(%s): %v", account.Account.ID, viewErr)
		}
		if len(view.Holdings) == 0 {
			continue
		}
		_, periodErr := gain.RealizedGain(ctx, domain.GainScope{AccountID: &account.Account.ID}, domain.TrendAllTime)
		if periodErr != nil {
			t.Fatalf("RealizedGain(%s): %v", account.Account.ID, periodErr)
		}
	}
}

func financialFactCounts(t *testing.T, database *sql.DB) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	for _, table := range []string{
		"activities",
		"activity_effects",
		"activity_trade_details",
		"history_origins",
		"history_origin_components",
		"daily_valuation_snapshots",
		"daily_valuation_snapshot_items",
		"history_snapshot_state",
	} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	return counts
}
