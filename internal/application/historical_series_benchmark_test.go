package application

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Opt-in profiling evidence: sampling is an approximation of Go heap high-water,
// not an RSS limit or a guarantee on another host. Setup is excluded.
func TestHistoricalSeriesMemoryEvidence(t *testing.T) {
	if os.Getenv("NW_SERIES_MEMORY_EVIDENCE") != "1" {
		t.Skip("opt-in heap profiling")
	}
	for _, dense := range []bool{false, true} {
		s, _ := seriesBenchmarkFixture(t, dense)
		for _, days := range []int{1, 7, 31} {
			runtime.GC()
			var baseline runtime.MemStats
			runtime.ReadMemStats(&baseline)
			stop, peak := make(chan struct{}), make(chan uint64)
			go func() {
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				max := baseline.HeapAlloc
				for {
					var current runtime.MemStats
					runtime.ReadMemStats(&current)
					if current.HeapAlloc > max {
						max = current.HeapAlloc
					}
					select {
					case <-ticker.C:
					case <-stop:
						peak <- max
						return
					}
				}
			}()
			start := time.Now()
			out, err := s.HistoricalOverviewSeries(context.Background(), "2026-08-01", fmt.Sprintf("2026-08-%02d", days))
			close(stop)
			maximum := <-peak
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("dense=%t days=%d elapsed=%s baselineHeap=%d sampledPeakHeap=%d addedHeap=%d facts=%d queries=%d", dense, days, time.Since(start), baseline.HeapAlloc, maximum, maximum-baseline.HeapAlloc, out.Work.CandidateFacts, out.Work.Queries)
		}
	}
}

func seriesBenchmarkFixture(t testing.TB, dense bool) (*Service, *sqlite.DB) {
	backfills, activities := 0, 0
	if dense {
		backfills, activities = 1000, 25000
	}
	return seriesHistoryScaleFixture(t, 100, backfills, activities)
}

func seriesHistoryScaleFixture(t testing.TB, accountCount, backfillsPerInstrument, activityCount int) (*Service, *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/series.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic series benchmark", BaseCurrency: "CNY", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := bootstrap.Members[0].ID
	instruments := []domain.Instrument{}
	for n := 0; n < 9; n++ {
		i, err := s.CreateInstrument(ctx, InstrumentInput{Name: fmt.Sprintf("Synthetic %d", n), Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		instruments = append(instruments, i)
		if _, err := s.AppendManualInstrumentQuote(ctx, i.ID, "0.00006", "2020-01-01", false); err != nil {
			t.Fatal(err)
		}
	}
	accounts := []domain.AccountRecord{}
	costs := map[domain.HoldingID]string{}
	for n := 0; n < accountCount; n++ {
		a, err := s.CreateAccount(ctx, AccountInput{Name: fmt.Sprintf("Synthetic broker %03d", n), AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{owner}})
		if err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, a)
		if _, err := s.AppendAccountCashValue(ctx, a.Account.ID, "0", "CNY", "2020-01-01"); err != nil {
			t.Fatal(err)
		}
		for _, i := range instruments {
			h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "2"})
			if err != nil {
				t.Fatal(err)
			}
			costs[h.ID] = "0.00006"
		}
	}
	if _, err := s.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
		t.Fatal(err)
	}
	if backfillsPerInstrument > 0 || activityCount > 0 {
		tx, err := db.SQL.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		quote, err := tx.Prepare(`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed) VALUES(?,?,'0.00006','CNY','manual','manual',?,?,0)`)
		if err != nil {
			t.Fatal(err)
		}
		defer quote.Close()
		for _, i := range instruments {
			for n := 0; n < backfillsPerInstrument; n++ {
				at := now.AddDate(0, 0, n+1).Format(time.RFC3339Nano)
				if _, err := quote.Exec(domain.NewInstrumentQuoteID().String(), i.ID.String(), at, at); err != nil {
					t.Fatal(err)
				}
			}
		}
		activity, err := tx.Prepare(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'value_update','other',?,?,?)`)
		if err != nil {
			t.Fatal(err)
		}
		defer activity.Close()
		effect, err := tx.Prepare(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'observation','added','account_cash','adjustment',?,'0','CNY')`)
		if err != nil {
			t.Fatal(err)
		}
		defer effect.Close()
		value, err := tx.Prepare(`INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'0','CNY',?,?,?,'event')`)
		if err != nil {
			t.Fatal(err)
		}
		defer value.Close()
		for n := 0; n < activityCount; n++ {
			id, fx := domain.NewActivityID().String(), domain.NewActivityEffectID().String()
			at := now.AddDate(0, 0, 1+n%2350).Add(time.Duration(n) * time.Microsecond).Format(time.RFC3339Nano)
			local := at[:10]
			account := accounts[n%len(accounts)].Account.ID.String()
			if _, err := activity.Exec(id, bootstrap.Household.ID.String(), at, local, at); err != nil {
				t.Fatal(err)
			}
			if _, err := effect.Exec(fx, id, account); err != nil {
				t.Fatal(err)
			}
			if _, err := value.Exec(domain.NewAccountCashValueID().String(), account, at, at, fx); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	return s, db
}

func BenchmarkHistoricalSeriesBounded(b *testing.B) {
	for _, dense := range []bool{false, true} {
		b.Run(fmt.Sprintf("dense=%t", dense), func(b *testing.B) {
			s, _ := seriesBenchmarkFixture(b, dense)
			for _, days := range []int{1, 7, 31} {
				b.Run(fmt.Sprintf("days=%d", days), func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						out, err := s.HistoricalOverviewSeries(context.Background(), "2026-08-01", time.Date(2026, 8, days, 0, 0, 0, 0, time.UTC).Format("2006-01-02"))
						if err != nil {
							b.Fatal(err)
						}
						b.ReportMetric(float64(out.Work.Queries), "queries/op")
						b.ReportMetric(float64(out.Work.CandidateFacts), "facts/op")
						b.ReportMetric(float64(out.Work.CandidateBytes), "input-B/op")
						if out.Work.Leaves != 1000 {
							b.Fatalf("leaves=%d", out.Work.Leaves)
						}
					}
				})
			}
		})
	}
}

func TestHistoricalSeriesBenchmarkCancellationEvidence(t *testing.T) {
	if os.Getenv("NESTWORTH_PERF") != "1" {
		t.Skip("25,000-activity cancellation latency runs in the weekly perf workflow (NESTWORTH_PERF=1)")
	}
	if testing.Short() {
		t.Skip("dense synthetic cancellation evidence")
	}
	s, _ := seriesBenchmarkFixture(t, true)
	runtime.GC()
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	defer timer.Stop()
	start := time.Now()
	_, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-31")
	elapsed := time.Since(start)
	if err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("slow cancellation %s", elapsed)
	}
	t.Logf("dense cancellation latency=%s (cancel scheduled at 10ms)", elapsed)
}
