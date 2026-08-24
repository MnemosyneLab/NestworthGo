package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type countingQueryer struct {
	queryer
	queries int
}

func seedSchema6Fixture(t *testing.T, path string) {
	t.Helper()
	scriptPath := filepath.Join("..", "..", "..", "testdata", "v0.1.4", "schema6-fixture.sql")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(script)); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
}

func (q *countingQueryer) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	q.queries++
	return q.queryer.QueryContext(ctx, statement, args...)
}

func TestCostBasisEventsUseOneBoundedQueryForAllEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded-cost-basis.db")
	seedSchema6Fixture(t, path)
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	counter := &countingQueryer{queryer: database.SQL}
	events, err := listCostBasisEventsQuery(context.Background(), counter, domain.HoldingID("00000000-0000-4000-8000-000000000050"))
	if err != nil {
		t.Fatal(err)
	}
	if counter.queries != 1 {
		t.Fatalf("ListCostBasisEvents query count = %d, want one bounded query", counter.queries)
	}
	if len(events) != 3 {
		t.Fatalf("bounded event count = %d, want 3", len(events))
	}
}

func TestCostBasisRepositoryReturnsOrderedNonReversedEventsAndCosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cost-basis.db")
	seedSchema6Fixture(t, path)
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := NewRepository(database)
	ctx := context.Background()

	holding050 := domain.HoldingID("00000000-0000-4000-8000-000000000050")
	startingCost, err := repository.StartingPointCost(ctx, holding050)
	if err != nil {
		t.Fatal(err)
	}
	if startingCost == nil || startingCost.Canonical() != "720" {
		t.Fatalf("StartingPointCost = %v, want 720", startingCost)
	}
	events, err := repository.ListCostBasisEvents(ctx, holding050)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("cost-basis events = %d, want sell, transfer-out, and Fix replacement buy", len(events))
	}
	if events[0].Kind != domain.CostBasisSell || events[0].UnitPrice == nil || events[0].UnitPrice.Canonical() != "130" || events[0].Fee == nil || events[0].Fee.CanonicalAmount() != "1" {
		t.Fatalf("sell event = %+v", events[0])
	}
	if events[1].Kind != domain.CostBasisTransferOut || events[1].Quantity.Canonical() != "1" {
		t.Fatalf("transfer-out event = %+v", events[1])
	}
	if events[2].Kind != domain.CostBasisBuy || events[2].UnitPrice == nil || events[2].UnitPrice.Canonical() != "115" {
		t.Fatalf("replacement buy event = %+v", events[2])
	}

	holding053 := domain.HoldingID("00000000-0000-4000-8000-000000000053")
	transferCost, err := repository.StartingPointCost(ctx, holding053)
	if err != nil {
		t.Fatal(err)
	}
	if transferCost == nil || transferCost.Canonical() != "720" {
		t.Fatalf("transfer Holding StartingPointCost = %v, want 720", transferCost)
	}
	transferEvents, err := repository.ListCostBasisEvents(ctx, holding053)
	if err != nil {
		t.Fatal(err)
	}
	if len(transferEvents) != 1 || transferEvents[0].Kind != domain.CostBasisTransferIn || transferEvents[0].SourceHoldingID == nil || *transferEvents[0].SourceHoldingID != holding050 {
		t.Fatalf("transfer-in events = %+v", transferEvents)
	}

	zeroHolding := domain.HoldingID("00000000-0000-4000-8000-000000000052")
	zeroCost, err := repository.StartingPointCost(ctx, zeroHolding)
	if err != nil {
		t.Fatal(err)
	}
	if zeroCost != nil {
		t.Fatalf("zero Holding StartingPointCost = %v, want nil", zeroCost)
	}
	zeroEvents, err := repository.ListCostBasisEvents(ctx, zeroHolding)
	if err != nil {
		t.Fatal(err)
	}
	if len(zeroEvents) != 0 {
		t.Fatalf("zero Holding events = %+v, want none", zeroEvents)
	}
}

func TestCostBasisRepositoryExcludesArchivedHolding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archived-cost-basis.db")
	seedSchema6Fixture(t, path)
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.SQL.Exec(`UPDATE holdings SET archived_at = '2026-08-24T00:00:00.000Z' WHERE id = '00000000-0000-4000-8000-000000000050'`); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(database)
	events, err := repository.ListCostBasisEvents(context.Background(), domain.HoldingID("00000000-0000-4000-8000-000000000050"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("archived Holding events = %+v, want none", events)
	}
	cost, err := repository.StartingPointCost(context.Background(), domain.HoldingID("00000000-0000-4000-8000-000000000050"))
	if err != nil {
		t.Fatal(err)
	}
	if cost != nil {
		t.Fatalf("archived Holding cost = %v, want nil", cost)
	}
}
