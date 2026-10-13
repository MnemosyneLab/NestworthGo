package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type countingQueryer struct {
	queryer
	queries int
}

func seedCurrentGainFixture(t *testing.T, path string) *DB {
	t.Helper()
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "current", "gain-data.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.Exec(string(script)); err != nil {
		t.Fatal(err)
	}
	if err := database.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	return database
}

func (q *countingQueryer) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	q.queries++
	return q.queryer.QueryContext(ctx, statement, args...)
}

func TestCostBasisEventsUseOneBoundedQueryForAllEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded-cost-basis.db")
	database := seedCurrentGainFixture(t, path)
	defer database.Close()
	counter := &countingQueryer{queryer: database.SQL}
	events, err := listCostBasisEventsQuery(context.Background(), counter, domain.HoldingID("00000000-0000-4000-8000-000000000050"), false)
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
	database := seedCurrentGainFixture(t, path)
	defer database.Close()
	repository := NewRepository(database)
	ctx := context.Background()

	holding050 := domain.HoldingID("00000000-0000-4000-8000-000000000050")
	startingCost, err := repository.StartingPointCost(ctx, holding050, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if startingCost == nil || startingCost.Canonical() != "720" {
		t.Fatalf("StartingPointCost = %v, want 720", startingCost)
	}
	events, err := repository.ListCostBasisEvents(ctx, holding050, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("cost-basis events = %d, want sell, transfer-out, and Fix replacement buy", len(events))
	}
	if events[1].Kind != domain.CostBasisSell || events[1].Gross == nil || events[1].Gross.CanonicalAmount() != "130" || events[1].UnitPrice == nil || events[1].UnitPrice.Canonical() != "130" || events[1].Fee == nil || events[1].Fee.CanonicalAmount() != "1" {
		t.Fatalf("sell event = %+v", events[1])
	}
	if events[2].Kind != domain.CostBasisTransferOut || events[2].Quantity.Canonical() != "1" {
		t.Fatalf("transfer-out event = %+v", events[2])
	}
	if events[0].Kind != domain.CostBasisBuy || events[0].UnitPrice == nil || events[0].UnitPrice.Canonical() != "115" {
		t.Fatalf("replacement buy event = %+v", events[0])
	}

	holding053 := domain.HoldingID("00000000-0000-4000-8000-000000000053")
	transferCost, err := repository.StartingPointCost(ctx, holding053, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if transferCost == nil || transferCost.Canonical() != "720" {
		t.Fatalf("transfer Holding StartingPointCost = %v, want 720", transferCost)
	}
	transferEvents, err := repository.ListCostBasisEvents(ctx, holding053, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(transferEvents) != 1 || transferEvents[0].Kind != domain.CostBasisTransferIn || transferEvents[0].SourceHoldingID == nil || *transferEvents[0].SourceHoldingID != holding050 {
		t.Fatalf("transfer-in events = %+v", transferEvents)
	}

	zeroHolding := domain.HoldingID("00000000-0000-4000-8000-000000000052")
	zeroCost, err := repository.StartingPointCost(ctx, zeroHolding, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if zeroCost != nil {
		t.Fatalf("zero Holding StartingPointCost = %v, want nil", zeroCost)
	}
	zeroEvents, err := repository.ListCostBasisEvents(ctx, zeroHolding, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(zeroEvents) != 0 {
		t.Fatalf("zero Holding events = %+v, want none", zeroEvents)
	}
}

func TestCostBasisRepositoryExcludesArchivedHoldingUnlessRequested(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archived-cost-basis.db")
	database := seedCurrentGainFixture(t, path)
	defer database.Close()
	if _, err := database.SQL.Exec(`UPDATE holdings SET archived_at = '2026-08-24T00:00:00.000Z' WHERE id = '00000000-0000-4000-8000-000000000050'`); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(database)
	holdingID := domain.HoldingID("00000000-0000-4000-8000-000000000050")
	events, err := repository.ListCostBasisEvents(context.Background(), holdingID, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("archived Holding events = %+v, want none without IncludeArchivedHoldings", events)
	}
	cost, err := repository.StartingPointCost(context.Background(), holdingID, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if cost != nil {
		t.Fatalf("archived Holding cost = %v, want nil without IncludeArchivedHoldings", cost)
	}
	historicalEvents, err := repository.ListCostBasisEvents(context.Background(), holdingID, domain.CostBasisReadFilter{IncludeArchivedHoldings: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(historicalEvents) != 3 {
		t.Fatalf("archived Holding historical events = %d, want 3", len(historicalEvents))
	}
	historicalCost, err := repository.StartingPointCost(context.Background(), holdingID, domain.CostBasisReadFilter{IncludeArchivedHoldings: true})
	if err != nil {
		t.Fatal(err)
	}
	if historicalCost == nil || historicalCost.Canonical() != "720" {
		t.Fatalf("archived Holding historical cost = %v, want 720", historicalCost)
	}
}

func TestCostBasisEventsRetainCorrectionRootAndEffectOrder(t *testing.T) {
	database := seedCurrentGainFixture(t, filepath.Join(t.TempDir(), "order.db"))
	repo := NewRepository(database)
	events, err := repo.ListCostBasisEvents(t.Context(), domain.HoldingID("00000000-0000-4000-8000-000000000050"), domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		var effective, created, ordering string
		var sequence int
		if err := database.SQL.QueryRowContext(t.Context(), correctedActivityCTE+`SELECT a.effective_at, a.created_at, a.ordering_id, e.sequence FROM economic_activities a JOIN activity_effects e ON e.activity_id=a.id WHERE a.id=? AND e.holding_id=?`, event.ActivityID.String(), "00000000-0000-4000-8000-000000000050").Scan(&effective, &created, &ordering, &sequence); err != nil {
			t.Fatal(err)
		}
		effectiveTime, err := time.Parse(time.RFC3339Nano, effective)
		if err != nil {
			t.Fatal(err)
		}
		createdTime, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			t.Fatal(err)
		}
		if !event.EffectiveAt.Equal(effectiveTime) || !event.CreatedAt.Equal(createdTime) || event.OrderingID.String() != ordering || event.Sequence != sequence {
			t.Fatalf("economic order lost: %+v", event)
		}
	}
	// The first event is the fixture's corrected buy, with a different audit ID.
	if events[0].OrderingID == events[0].ActivityID {
		t.Fatal("corrected buy lost its root ordering ID")
	}
}
