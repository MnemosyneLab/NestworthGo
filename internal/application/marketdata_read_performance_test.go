package application

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	sqlitedriver "modernc.org/sqlite"
)

type marketReadDriver struct {
	driver.Driver
	queries, quoteQueries atomic.Int64
}
type marketReadConn struct {
	driver.Conn
	owner *marketReadDriver
}

var marketReadID atomic.Int64

func (d *marketReadDriver) Open(name string) (driver.Conn, error) {
	c, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return &marketReadConn{Conn: c, owner: d}, nil
}
func (c *marketReadConn) QueryContext(ctx context.Context, statement string, args []driver.NamedValue) (driver.Rows, error) {
	c.owner.queries.Add(1)
	if strings.Contains(statement, "FROM instrument_quotes WHERE instrument_id") {
		c.owner.quoteQueries.Add(1)
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, statement, args)
}
func (c *marketReadConn) ExecContext(ctx context.Context, statement string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, statement, args)
}
func (c *marketReadConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func marketReadFixture(t testing.TB, count, quotes int) (*Service, *marketReadDriver) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "market-read.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	s := NewService(repo)
	start := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return start })
	ctx := context.Background()
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic market reads", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		source := []string{"manual", "agent", "provider"}[i%3]
		input := InstrumentInput{Name: fmt.Sprintf("Instrument %d", i), Type: "stock", QuoteCurrency: "USD", QuoteSource: source}
		if source == "provider" {
			input.ProviderKey = YahooFinanceProviderKey
			input.ProviderSymbol = fmt.Sprintf("S%d", i)
			input.MarketCode = "US"
		}
		instrument, err := s.CreateInstrument(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.SQL.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for j := 0; j < quotes; j++ {
			when := start.AddDate(0, 0, -j).Format("2006-01-02T15:04:05.000Z")
			if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at) VALUES(?, ?, '100.12345678', 'USD', ?, ?, ?, ?)`, domain.NewInstrumentQuoteID().String(), instrument.ID.String(), source, source, when, when); err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	s.setClock(func() time.Time { return start.AddDate(0, 0, 4) })
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	d := &marketReadDriver{Driver: &sqlitedriver.Driver{}}
	name := fmt.Sprintf("market-read-%d", marketReadID.Add(1))
	sql.Register(name, d)
	db.SQL, err = sql.Open(name, path+"?_txlock=immediate&_pragma=busy_timeout%3d5000&_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	db.SQL.SetMaxOpenConns(1)
	return s, d
}

// Count actual driver queries, including QueryRowContext and reads inside
// transactions. Fixture construction and SQL setup are outside measurements.
func TestMarketDataHealthReadQueries(t *testing.T) {
	for _, fixture := range []struct {
		instruments           int
		queries, quoteQueries int64
	}{
		{1, 47, 2}, {50, 177, 84}, {200, 577, 334},
	} {
		t.Run(fmt.Sprint(fixture.instruments), func(t *testing.T) {
			s, d := marketReadFixture(t, fixture.instruments, 8)
			if _, err := s.ScanMarketDataHealth(context.Background()); err != nil {
				t.Fatal(err)
			}
			if d.queries.Load() != fixture.queries || d.quoteQueries.Load() != fixture.quoteQueries {
				t.Fatalf("queries=%d want=%d quote_queries=%d want=%d", d.queries.Load(), fixture.queries, d.quoteQueries.Load(), fixture.quoteQueries)
			}
		})
	}
}

func BenchmarkMarketDataHealthReads(b *testing.B) {
	for _, size := range [][2]int{{1, 8}, {50, 0}, {50, 8}, {50, 365}, {200, 8}} {
		b.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(b *testing.B) {
			s, d := marketReadFixture(b, size[0], size[1])
			for b.Loop() {
				if _, err := s.ScanMarketDataHealth(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(d.queries.Load())/float64(b.N), "queries/op")
			b.ReportMetric(float64(d.quoteQueries.Load())/float64(b.N), "quote_queries/op")
		})
	}
}
