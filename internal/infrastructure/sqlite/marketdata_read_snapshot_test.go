package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestMarketReadSnapshotConcurrentPairsReadOnlyAndBudget(t *testing.T) {
	database, repository, _, _, first := seedPortfolioRepository(t)
	second := first
	second.ID = domain.NewInstrumentID()
	second.Name = "Second target"
	if err := repository.CreateInstrument(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	database.SQL.SetMaxOpenConns(2)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	request := domain.MarketReadRequest{Targets: []domain.MarketReadTarget{{InstrumentID: first.ID.String()}, {InstrumentID: second.ID.String()}}, View: "history", Range: "all", SourceFilter: "all"}
	pair := func(n int) error {
		price, _ := domain.ParseUnitPrice(fmt.Sprint(n))
		at := now.Add(-time.Hour).Add(time.Duration(n) * time.Second)
		return database.WithTx(t.Context(), func(tx *sql.Tx) error {
			for _, instrument := range []domain.Instrument{first, second} {
				q, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: at}, at)
				if err != nil {
					return err
				}
				if err := appendInstrumentQuoteTx(t.Context(), tx, q); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := pair(1); err != nil {
		t.Fatal(err)
	}
	start, done := make(chan struct{}), make(chan error, 1)
	var writes atomic.Int32
	go func() {
		<-start
		for n := 2; n <= 40; n++ {
			if err := pair(n); err != nil {
				done <- err
				return
			}
			writes.Add(1)
		}
		done <- nil
	}()
	close(start)
	for range 40 {
		ctx, cancel := context.WithTimeout(domain.WithBoundedRead(t.Context()), 5*time.Second)
		snapshot, err := repository.ReadMarketDataSnapshot(ctx, request, now)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.CurrentInstrumentQuotes) != 2 {
			t.Fatal("missing current target")
		}
		if snapshot.CurrentInstrumentQuotes[0].UnitPrice.Canonical() != snapshot.CurrentInstrumentQuotes[1].UnitPrice.Canonical() {
			t.Fatal("mixed current target generations")
		}
		a, b := snapshot.InstrumentHistory[request.Targets[0].Key()], snapshot.InstrumentHistory[request.Targets[1].Key()]
		if len(a) != len(b) || a[0].UnitPrice.Canonical() != b[0].UnitPrice.Canonical() {
			t.Fatal("mixed history generations")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 39 {
		t.Fatal("concurrent writer not exercised")
	}
	database.SQL.SetMaxOpenConns(1)
	before, err := repository.ReadPortfolioSnapshot(t.Context(), domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	var changesBefore, changesAfter int
	if err := database.SQL.QueryRowContext(t.Context(), `SELECT total_changes()`).Scan(&changesBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(t.Context(), `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReadMarketDataSnapshot(domain.WithBoundedRead(t.Context()), request, now); err != nil {
		t.Fatalf("snapshot cannot run on enforced read-only connection: %v", err)
	}
	if err := database.SQL.QueryRowContext(t.Context(), `SELECT total_changes()`).Scan(&changesAfter); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(t.Context(), `PRAGMA query_only=OFF`); err != nil {
		t.Fatal(err)
	}
	after, err := repository.ReadPortfolioSnapshot(t.Context(), domain.AccountFilter{IncludeArchived: true})
	if err != nil || !reflect.DeepEqual(before, after) || changesBefore != changesAfter {
		t.Fatal("snapshot changed DB facts/preferences", err)
	}
	if _, err := database.SQL.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed,observation_kind,revision) SELECT printf('%08x',x)||'-0000-4000-8000-000000000000',?,'1','USD','manual','','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z',0,'latest',1 FROM n`, first.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReadMarketDataSnapshot(domain.WithBoundedRead(t.Context()), request, now); err == nil {
		t.Fatal("history budget silently accepted oversized facts")
	} else if e, ok := err.(*domain.Error); !ok || e.Code != "too_large" {
		t.Fatal("budget must fail explicitly", err)
	}
}
