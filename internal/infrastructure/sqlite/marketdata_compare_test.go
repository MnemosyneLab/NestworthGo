package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestMarketComparisonSnapshotReadOnlyAndConcurrent(t *testing.T) {
	db, r, _, _, first := seedPortfolioRepository(t)
	second := first
	second.ID = domain.NewInstrumentID()
	second.Name = "Second comparison target"
	if err := r.CreateInstrument(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	db.SQL.SetMaxOpenConns(2)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	scopes := []domain.MarketComparisonScope{{Target: domain.MarketReadTarget{InstrumentID: first.ID.String()}, Dates: []string{"2026-10-12"}}, {Target: domain.MarketReadTarget{InstrumentID: second.ID.String()}, Dates: []string{"2026-10-12"}}}
	pair := func(n int) error {
		return db.WithTx(t.Context(), func(tx *sql.Tx) error {
			for _, i := range []domain.Instrument{first, second} {
				p, _ := domain.ParseUnitPrice(fmt.Sprint(n))
				at := now.Add(-24 * time.Hour).Add(time.Duration(n) * time.Second)
				q, e := domain.NewInstrumentQuote(i, domain.InstrumentQuoteInput{UnitPrice: p, SourceKind: domain.QuoteSourceManual, QuotedAt: at}, at)
				if e != nil {
					return e
				}
				q.ObservationKind = "close"
				q.EffectiveDate = "2026-10-12"
				q.PriceBasis = domain.YahooRawClosePriceBasis
				if e = appendInstrumentQuoteTx(t.Context(), tx, q); e != nil {
					return e
				}
			}
			return nil
		})
	}
	if e := pair(1); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		for n := 2; n <= 12; n++ {
			if e := pair(n); e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	for range 12 {
		ctx, cancel := context.WithTimeout(domain.WithBoundedRead(t.Context()), 5*time.Second)
		snap, e := r.ReadMarketComparisonSnapshot(ctx, scopes, now)
		cancel()
		if e != nil {
			t.Fatal(e)
		}
		if len(snap.CurrentInstrumentQuotes) != 2 || len(snap.InstrumentQuotes)%2 != 0 {
			t.Fatal("mixed target generations")
		}
		if snap.CurrentInstrumentQuotes[0].UnitPrice.Canonical() != snap.CurrentInstrumentQuotes[1].UnitPrice.Canonical() {
			t.Fatal("mixed current generations")
		}
		counts := map[string]int{}
		for _, q := range snap.InstrumentQuotes {
			counts[q.UnitPrice.Canonical()]++
		}
		for _, n := range counts {
			if n != 2 {
				t.Fatal("mixed historical generations")
			}
		}
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	db.SQL.SetMaxOpenConns(1)
	before, e := r.ReadPortfolioSnapshot(t.Context(), domain.AccountFilter{IncludeArchived: true})
	if e != nil {
		t.Fatal(e)
	}
	var changesBefore, changesAfter int
	if e = db.SQL.QueryRowContext(t.Context(), `SELECT total_changes()`).Scan(&changesBefore); e != nil {
		t.Fatal(e)
	}
	if _, e = db.SQL.ExecContext(t.Context(), `PRAGMA query_only=ON`); e != nil {
		t.Fatal(e)
	}
	if _, e = r.ReadMarketComparisonSnapshot(domain.WithBoundedRead(t.Context()), scopes, now); e != nil {
		t.Fatal("read attempted mutation", e)
	}
	if e = db.SQL.QueryRowContext(t.Context(), `SELECT total_changes()`).Scan(&changesAfter); e != nil {
		t.Fatal(e)
	}
	if _, e = db.SQL.ExecContext(t.Context(), `PRAGMA query_only=OFF`); e != nil {
		t.Fatal(e)
	}
	after, e := r.ReadPortfolioSnapshot(t.Context(), domain.AccountFilter{IncludeArchived: true})
	if e != nil || changesBefore != changesAfter || !reflect.DeepEqual(before, after) {
		t.Fatal("comparison changed facts/preferences", e)
	}
}
func TestMarketComparisonSnapshotBudgetsAndSQLFailure(t *testing.T) {
	for _, tc := range []string{"bytes", "rows", "sql", "timezone"} {
		t.Run(tc, func(t *testing.T) {
			db, r, h, _, i := seedPortfolioRepository(t)
			now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
			scopes := []domain.MarketComparisonScope{{Target: domain.MarketReadTarget{InstrumentID: i.ID.String()}, Dates: []string{"2026-10-12"}}}
			p, _ := domain.ParseUnitPrice("1")
			q, e := domain.NewInstrumentQuote(i, domain.InstrumentQuoteInput{UnitPrice: p, SourceKind: domain.QuoteSourceManual, QuotedAt: now.Add(-time.Hour)}, now)
			if e != nil {
				t.Fatal(e)
			}
			if e = r.AppendInstrumentQuote(t.Context(), q); e != nil {
				t.Fatal(e)
			}
			switch tc {
			case "bytes":
				_, e = db.SQL.ExecContext(t.Context(), `UPDATE instrument_quotes SET source_key=? WHERE id=?`, strings.Repeat("x", 4<<20), q.ID.String())
			case "rows":
				_, e = db.SQL.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20001) INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed,observation_kind,effective_date,price_basis) SELECT printf('00000000-0000-4000-8000-%012x',x),?,'1',?,'manual','manual',?, ?,0,'close','2026-10-12','agent_raw_close_v1' FROM n`, i.ID.String(), i.QuoteCurrency.String(), formatTimestamp(now.Add(-time.Hour)), formatTimestamp(now))
			case "sql":
				_, e = db.SQL.ExecContext(t.Context(), `DROP TABLE agent_quote_records`)
			case "timezone":
				_, e = db.SQL.ExecContext(t.Context(), `INSERT INTO history_origins(id,household_id,timezone,started_at,created_at) VALUES(?,?,?,?,?)`, domain.NewHistoryOriginID().String(), h.ID.String(), "Invalid/Zone", formatTimestamp(now), formatTimestamp(now))
			}
			if e != nil {
				t.Fatal("invalid synthetic setup", e)
			}
			_, e = r.ReadMarketComparisonSnapshot(domain.WithBoundedRead(t.Context()), scopes, now)
			if e == nil {
				t.Fatal("incomplete evidence returned success")
			}
			var de *domain.Error
			if (tc == "bytes" || tc == "rows") && (!errors.As(e, &de) || de.Code != "too_large") {
				t.Fatal("budget not explicit", e)
			}
		})
	}
}
