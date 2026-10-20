package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type seriesZeroFixture struct {
	s          *Service
	db         *sqlite.DB
	account    domain.AccountRecord
	instrument domain.Instrument
	holding    domain.Holding
	now        *time.Time
	location   *time.Location
	fx         bool
}

func newSeriesZeroFixture(t *testing.T, fx bool, zone, first, quantity string, databases ...*sqlite.DB) seriesZeroFixture {
	t.Helper()
	s, db, owner, now := overviewFixture(t, databases...)
	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	fixture := seriesZeroFixture{s: s, db: db, now: now, location: location, fx: fx}
	fixture.day(t, first)
	currency := "CNY"
	if fx {
		currency = "USD"
	}
	fixture.account = overviewAccount(t, s, owner, "Synthetic zero exposure", "brokerage", "asset", "holdings", currency, "")
	fixture.instrument, err = s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic stock", Type: "stock", QuoteCurrency: currency, QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.holding, err = s.CreateHolding(t.Context(), HoldingInput{AccountID: fixture.account.Account.ID.String(), InstrumentID: fixture.instrument.ID.String(), Quantity: quantity})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(t.Context(), fixture.instrument.ID, "100", now.Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	if fx {
		if _, err := s.SetFXPreference(t.Context(), "USD", "CNY", "manual"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AppendManualFXQuote(t.Context(), "USD", "CNY", "2", now.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.StartHistoryWithCosts(t.Context(), zone, map[domain.HoldingID]string{fixture.holding.ID: "100"}); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f seriesZeroFixture) day(t *testing.T, date string) {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02", date, f.location)
	if err != nil {
		t.Fatal(err)
	}
	*f.now = at.Add(12 * time.Hour)
}

func (f seriesZeroFixture) quantity(t *testing.T, added bool) {
	t.Helper()
	cost, err := domain.ParseUnitPrice("100")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.RecordChange(t.Context(), domain.PositionAdjustmentInput{HouseholdID: f.account.Account.HouseholdID, HoldingID: f.holding.ID, Quantity: mustQuantity(t, "1"), Added: added, UnitCost: &cost, EffectiveAt: *f.now}); err != nil {
		t.Fatal(err)
	}
}

func (f seriesZeroFixture) agent(t *testing.T) {
	t.Helper()
	item := AgentMarketDataItem{Date: f.now.In(f.location).Format("2006-01-02"), Timezone: f.location.String(), SourceTitle: "Synthetic resumed source"}
	if f.fx {
		item.Kind = "daily_reference"
		item.BaseCurrency = "USD"
		item.QuoteCurrency = "CNY"
		item.Value = "2"
	} else {
		item.Kind = "close"
		item.InstrumentID = f.instrument.ID.String()
		item.Currency = "CNY"
		item.Value = "100"
	}
	if _, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{item}}); err != nil {
		t.Fatal(err)
	}
}

func (f seriesZeroFixture) capture(t *testing.T, from, to string) HistoricalOverviewSeriesResult {
	t.Helper()
	if _, err := f.db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.HistoricalOverviewSeries(t.Context(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	original, err := f.s.HistoricalOverview(t.Context(), from, to)
	if err != nil || !reflect.DeepEqual(original, out.Overview) {
		t.Fatalf("zero comparison changed P1 endpoints: %v", err)
	}
	for _, point := range out.Points {
		if point.Available && point.Assets != nil && *point.Assets == "0" {
			zero, err := f.s.HistoricalOverview(context.Background(), point.Date, "")
			if err != nil {
				t.Fatal(err)
			}
			cell := overviewRow(t, zero, f.account.Account.ID.String()+":holding:"+f.holding.ID.String()).Left
			if cell == nil || cell.Price != nil || cell.FX != nil {
				t.Fatalf("zero acquired invented valuation evidence: %+v", cell)
			}
		}
	}
	return out
}

func TestHistoricalSeriesKnownZeroEvidenceContinuity(t *testing.T) {
	t.Parallel()
	for _, fx := range []bool{false, true} {
		for _, changeSource := range []bool{false, true} {
			name := "price"
			if fx {
				name = "fx"
			}
			if changeSource {
				name += "/changed"
			} else {
				name += "/same"
			}
			t.Run(name, func(t *testing.T) {
				f := newSeriesZeroFixture(t, fx, "UTC", "2026-08-01", "1")
				f.day(t, "2026-08-02")
				f.quantity(t, false)
				f.day(t, "2026-08-04")
				f.quantity(t, true)
				if changeSource {
					f.agent(t)
				}
				f.day(t, "2026-08-05")
				out := f.capture(t, "2026-08-01", "2026-08-04")
				for i, point := range out.Points {
					want := "0"
					if i == 0 || i == 3 {
						want = "100"
						if fx {
							want = "200"
						}
					}
					wantOverviewAmount(t, point.Assets, want)
					wantOverviewAmount(t, point.NetWorth, want)
					changed := !fx && changeSource && i == 3
					connected := i > 0 && !changed
					if point.ScopeChanged || point.BasisChanged != changed || point.ConnectAssets != connected || point.ConnectNetWorth != connected || point.ConnectLiabilities != (i > 0) {
						t.Fatalf("zero transition %d: %+v", i, point)
					}
				}
				// The selected range starts at zero: no earlier actual source exists
				// in this comparison even if a different source preceded the range.
				firstZero := f.capture(t, "2026-08-02", "2026-08-04")
				last := firstZero.Points[2]
				if last.BasisChanged || !last.ConnectAssets || !last.ConnectNetWorth {
					t.Fatalf("first zero invented a prior source: %+v", last)
				}
			})
		}
	}
}

func TestHistoricalSeriesZeroEvidenceMemoryResets(t *testing.T) {
	t.Parallel()
	for _, fx := range []bool{false, true} {
		for _, gap := range []bool{false, true} {
			name := "price"
			if fx {
				name = "fx"
			}
			if gap {
				name += "/civil-gap"
			} else {
				name += "/scope-removal"
			}
			t.Run(name, func(t *testing.T) {
				zone, first, zero, resume, closed := "UTC", "2026-08-01", "2026-08-02", "2026-08-04", "2026-08-05"
				if gap {
					zone, first, zero, resume, closed = "Pacific/Apia", "2011-12-29", "2011-12-31", "2012-01-01", "2012-01-02"
				}
				f := newSeriesZeroFixture(t, fx, zone, first, "1")
				f.day(t, zero)
				f.quantity(t, false)
				if !gap {
					if _, err := f.s.UpdateAccount(t.Context(), f.account.Account.ID, AccountInput{IncludeInNetWorth: false, IncludeInNetWorthSet: true}); err != nil {
						t.Fatal(err)
					}
					f.day(t, "2026-08-03")
					if _, err := f.s.UpdateAccount(t.Context(), f.account.Account.ID, AccountInput{IncludeInNetWorth: true, IncludeInNetWorthSet: true}); err != nil {
						t.Fatal(err)
					}
				}
				f.day(t, resume)
				f.quantity(t, true)
				f.agent(t)
				f.day(t, closed)
				out := f.capture(t, first, resume)
				last := out.Points[len(out.Points)-1]
				if last.ScopeChanged || last.BasisChanged || !last.ConnectAssets || !last.ConnectLiabilities || !last.ConnectNetWorth {
					t.Fatalf("removed/gapped prior source leaked into resumed exposure: %+v", last)
				}
				if gap {
					if out.Points[1].Available || out.Points[1].GapReason != "nonexistent_civil_date" || out.Points[2].ScopeChanged || out.Points[2].BasisChanged || out.Points[2].ConnectAssets || out.Points[2].ConnectNetWorth {
						t.Fatalf("gap comparison state not reset: %+v", out.Points)
					}
				} else if !out.Points[1].ScopeChanged || !out.Points[2].ScopeChanged {
					t.Fatalf("scope changes suppressed: %+v", out.Points)
				}
			})
		}
	}
}

func TestHistoricalSeriesKnownZeroAllowsFXSourcePreferenceChange(t *testing.T) {
	t.Parallel()
	f := newSeriesZeroFixture(t, true, "UTC", "2026-08-01", "1")
	f.agent(t) // Agent FX supplements the saved manual preference on day A.
	f.day(t, "2026-08-02")
	f.quantity(t, false)
	if _, err := f.s.SetFXPreference(t.Context(), "USD", "CNY", "agent"); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-03")
	f.quantity(t, true)
	f.agent(t)
	f.day(t, "2026-08-04")
	out := f.capture(t, "2026-08-01", "2026-08-03")
	zero, resumed := out.Points[1], out.Points[2]
	if zero.ScopeChanged || zero.BasisChanged || !zero.ConnectAssets || !zero.ConnectNetWorth {
		t.Fatalf("source preference disconnected a comparable value: %+v", zero)
	}
	if resumed.BasisChanged || !resumed.ConnectAssets || !resumed.ConnectNetWorth {
		t.Fatalf("unchanged actual Agent basis falsely changed on resume: %+v", resumed)
	}
}

func TestHistoricalSeriesFirstExposureAfterOriginZero(t *testing.T) {
	t.Parallel()
	for _, fx := range []bool{false, true} {
		name := "price"
		if fx {
			name = "fx"
		}
		t.Run(name, func(t *testing.T) {
			f := newSeriesZeroFixture(t, fx, "UTC", "2026-08-01", "0")
			f.day(t, "2026-08-02")
			f.quantity(t, true)
			f.agent(t)
			f.day(t, "2026-08-03")
			out := f.capture(t, "2026-08-01", "2026-08-02")
			first, last := out.Points[0], out.Points[1]
			wantOverviewAmount(t, first.Assets, "0")
			if first.AssetCoverage.Included != 1 || last.ScopeChanged || last.BasisChanged || !last.ConnectAssets || !last.ConnectNetWorth {
				t.Fatalf("origin zero invented an applied source: %+v", out.Points)
			}
		})
	}
}
