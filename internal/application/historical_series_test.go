package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistoricalSeriesReadOnlyExactP1EndpointsAndSingleCapture(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "0.0001")
	overviewAccount(t, s, owner, "Debt", "loan", "liability", "balance", "USD", "10")
	if _, err := s.StartHistory(ctx, "Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 31)
	var before, after int
	db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before)
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	original, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-31")
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Points) != 31 || !reflect.DeepEqual(original, result.Overview) {
		t.Fatalf("not equal to P1: original=%+v\nseries=%+v", original, result.Overview)
	}
	for _, point := range result.Points {
		wantOverviewAmount(t, point.Assets, "0.0001")
		if point.NetWorth != nil || point.Liabilities != nil || point.KnownLiabilities != nil || point.LiabilitiesComplete || point.LiabilityCoverage.Missing != 1 || len(point.Missing) != 1 {
			t.Fatalf("missing liability replaced: %+v", point)
		}
	}
	if !result.Points[1].ConnectAssets || result.Points[1].ConnectNetWorth {
		t.Fatal("partial roles not independent")
	}
	if result.Work.CandidateFacts == 0 || result.Work.Queries == 0 {
		t.Fatal("missing bounded read evidence")
	}
	db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after)
	if before != after {
		t.Fatal("series wrote data")
	}
}

func TestHistoricalSeriesBoundsSkippedDatesCancellationAndScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	account := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "0")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if err := s.ArchiveAccount(ctx, account.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 40)
	for _, dates := range [][2]string{{"2026-08-01", "2026-09-01"}, {"2026-08-02", "2026-08-01"}, {"current", "2026-08-01"}, {"2026-07-31", "2026-08-01"}, {"2026-02-30", "2026-03-01"}} {
		if _, err := s.HistoricalOverviewSeries(ctx, dates[0], dates[1]); err == nil {
			t.Fatalf("accepted %v", dates)
		}
	}
	r, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-04")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Points[2].ScopeChanged || r.Points[2].ConnectNetWorth {
		t.Fatalf("scope line not broken %+v", r.Points)
	}
	wantOverviewAmount(t, r.Points[0].NetWorth, "0")
	one, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-01")
	if err != nil || len(one.Points) != 1 {
		t.Fatalf("single day: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.HistoricalOverviewSeries(cancelled, "2026-08-01", "2026-08-04"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored %v", err)
	}
	repo := sqlite.NewRepository(db)
	_, work, err := repo.LoadHistoricalOverviewSeriesBatch(ctx, *now, domain.HistoricalSeriesLimits{MaxLeaves: 1, MaxFacts: 1, MaxBytes: 1000})
	var budget *domain.Error
	if !errors.As(err, &budget) || budget.Code != domain.ErrHistoricalSeriesBudget || work.CandidateFacts > 2 {
		t.Fatalf("budget not bounded %+v %v", work, err)
	}
}

func TestHistoricalSeriesInteriorSkippedCivilDate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	*now = time.Date(2011, 12, 29, 12, 0, 0, 0, time.UTC)
	overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "10")
	if _, err := s.StartHistory(ctx, "Pacific/Apia"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2012, 1, 2, 12, 0, 0, 0, time.UTC)
	r, err := s.HistoricalOverviewSeries(ctx, "2011-12-29", "2011-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if r.Points[1].Available || r.Points[1].GapReason != "nonexistent_civil_date" || r.Points[2].ConnectAssets {
		t.Fatalf("skipped date connected %+v", r.Points)
	}
	if _, err := s.HistoricalOverviewSeries(ctx, "2011-12-30", "2011-12-31"); err == nil {
		t.Fatal("skipped endpoint accepted")
	}
}

func TestHistoricalSeriesEmptyScopeTransitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Synthetic cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: false, OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		day      int
		included bool
	}{{2, true}, {3, false}, {5, true}} {
		*now = time.Date(2026, 8, change.day, 12, 0, 0, 0, time.UTC)
		if _, err := s.UpdateAccount(ctx, account.Account.ID, AccountInput{IncludeInNetWorth: change.included, IncludeInNetWorthSet: true}); err != nil {
			t.Fatal(err)
		}
	}
	*now = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	out, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-06")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"0", "100", "0", "0", "100", "100"} {
		point := out.Points[i]
		wantOverviewAmount(t, point.Assets, want)
		wantOverviewAmount(t, point.NetWorth, want)
		changed := i == 1 || i == 2 || i == 4
		connected := i == 3 || i == 5
		if !point.Available || point.ScopeChanged != changed || point.BasisChanged != changed || point.ConnectAssets != connected || point.ConnectLiabilities != connected || point.ConnectNetWorth != connected {
			t.Fatalf("day %d has incorrect transition: %+v", i+1, point)
		}
	}
	original, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-06")
	if err != nil || !reflect.DeepEqual(original, out.Overview) {
		t.Fatalf("scope transition changed endpoint totals/evidence: %v", err)
	}
}

func TestHistoricalSeriesScopeAfterMissingCivilDate(t *testing.T) {
	t.Parallel()
	for _, initialIncluded := range []bool{false, true} {
		t.Run(fmt.Sprintf("initialIncluded=%t", initialIncluded), func(t *testing.T) {
			ctx := context.Background()
			s, _, owner, now := overviewFixture(t)
			*now = time.Date(2011, 12, 29, 12, 0, 0, 0, time.UTC)
			account, err := s.CreateAccount(ctx, AccountInput{Name: "Synthetic Apia cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: initialIncluded, OwnerIDs: []domain.MemberID{owner}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(ctx, "Pacific/Apia"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2011, 12, 30, 12, 0, 0, 0, time.UTC) // Dec 31 in Apia, after its skipped Dec 30.
			if _, err := s.UpdateAccount(ctx, account.Account.ID, AccountInput{IncludeInNetWorth: !initialIncluded, IncludeInNetWorthSet: true}); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2012, 1, 3, 12, 0, 0, 0, time.UTC)
			out, err := s.HistoricalOverviewSeries(ctx, "2011-12-29", "2012-01-01")
			if err != nil {
				t.Fatal(err)
			}
			if out.Points[1].Available || out.Points[1].GapReason != "nonexistent_civil_date" {
				t.Fatalf("missing day lost: %+v", out.Points[1])
			}
			for i, point := range out.Points {
				if point.ScopeChanged || point.BasisChanged || point.ConnectAssets != (i == 3) || point.ConnectLiabilities != (i == 3) || point.ConnectNetWorth != (i == 3) {
					t.Fatalf("flags crossed gap or lost subsequent continuity: %+v", point)
				}
			}
		})
	}
}

func TestHistoricalSeriesSelectedEvidenceBasis(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"price", "fx"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, db, owner, now := overviewFixture(t)
			var instrument domain.Instrument
			if kind == "price" {
				broker := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
				var err error
				instrument, err = s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic stock", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "1", "2026-08-01", false); err != nil {
					t.Fatal(err)
				}
				if _, err := s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "1"}); err != nil {
					t.Fatal(err)
				}
			} else {
				overviewAccount(t, s, owner, "Synthetic USD cash", "bank_account", "asset", "balance", "USD", "1")
				if _, err := s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
					t.Fatal(err)
				}
				if _, err := s.AppendManualFXQuote(ctx, "USD", "CNY", "1", "2026-08-01"); err != nil {
					t.Fatal(err)
				}
				if _, err := s.StartHistory(ctx, "UTC"); err != nil {
					t.Fatal(err)
				}
			}
			*now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
			if kind == "price" {
				if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "2", "2026-08-02", false); err != nil {
					t.Fatal(err)
				}
			} else if _, err := s.AppendManualFXQuote(ctx, "USD", "CNY", "2", "2026-08-02"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
			var items []AgentMarketDataItem
			for day := 3; day <= 5; day++ {
				item := AgentMarketDataItem{Date: fmt.Sprintf("2026-08-%02d", day), Timezone: "UTC", Value: fmt.Sprint(day), SourceTitle: "Synthetic source"}
				if kind == "price" {
					item.InstrumentID = instrument.ID.String()
					item.Currency = "CNY"
					item.Kind = "close"
				} else {
					item.BaseCurrency = "USD"
					item.QuoteCurrency = "CNY"
					item.Kind = "daily_reference"
				}
				// Same Agent source/observation on day 5, different timestamp basis.
				if day == 5 {
					item.QuotedAt = "2026-08-05T12:00:00Z"
				}
				items = append(items, item)
			}
			if _, err := s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: items}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
				t.Fatal(err)
			}
			out, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-05")
			if err != nil {
				t.Fatal(err)
			}
			for i, point := range out.Points {
				wantOverviewAmount(t, point.Assets, fmt.Sprint(i+1))
				wantOverviewAmount(t, point.NetWorth, fmt.Sprint(i+1))
				changed := i == 2 || i == 4
				connected := i == 1 || i == 3
				if point.ScopeChanged || point.BasisChanged != changed || point.ConnectAssets != connected || point.ConnectLiabilities != connected || point.ConnectNetWorth != connected {
					t.Fatalf("%s day %d selected evidence basis/continuity: %+v", kind, i+1, point)
				}
			}
			original, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-05")
			if err != nil || !reflect.DeepEqual(original, out.Overview) {
				t.Fatalf("selected %s changed P1 endpoints: %v", kind, err)
			}
			for _, row := range out.Overview.Rows {
				if row.Kind == "account" {
					continue
				}
				left, right := row.Left.Price, row.Right.Price
				if kind == "fx" {
					left, right = row.Left.FX, row.Right.FX
				}
				if left == nil || right == nil || left.Source != "manual" || right.Source != "agent" || right.TimestampBasis != "source_timestamp" {
					t.Fatalf("fixture did not select actual sources: %+v", row)
				}
			}
		})
	}
}

type historicalSeriesProbe struct {
	Repository
	reader historicalSeriesRepository
	loads  int
	after  func()
}

func (r *historicalSeriesProbe) LoadHistoricalOverviewSeriesBatch(ctx context.Context, at time.Time, limits domain.HistoricalSeriesLimits) (domain.HistoricalSnapshotBatch, domain.HistoricalSeriesWork, error) {
	r.loads++
	b, w, err := r.reader.LoadHistoricalOverviewSeriesBatch(ctx, at, limits)
	if r.after != nil {
		r.after()
	}
	return b, w, err
}
func (r *historicalSeriesProbe) LoadHistoricalSnapshotBatch(context.Context, domain.HouseholdID, time.Time) (domain.HistoricalSnapshotBatch, error) {
	panic("series must not call the unbounded snapshot/overview loader")
}
func (r *historicalSeriesProbe) ReadPortfolioSnapshot(context.Context, domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	panic("series must not separately read current state")
}

func TestHistoricalSeriesConcurrentCaptureNoSecondaryReads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "10")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 3)
	original := s.repository
	writer := NewService(original)
	writer.setClock(func() time.Time { return *now })
	probe := &historicalSeriesProbe{Repository: original, reader: original.(historicalSeriesRepository)}
	probe.after = func() {
		if _, err := writer.AppendAccountValue(ctx, a.Account.ID, "20", "2026-08-02"); err != nil {
			t.Fatal(err)
		}
	}
	s.repository = probe
	result, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	if probe.loads != 1 {
		t.Fatalf("loads=%d", probe.loads)
	}
	for _, point := range result.Points {
		wantOverviewAmount(t, point.Assets, "10")
	}
	wantOverviewAmount(t, result.Overview.Right.Assets, "10")
	current, err := writer.HistoricalOverview(ctx, "2026-08-01", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, current.Right.Assets, "20")
}

func TestHistoricalSeriesBackfillManualCarryQuoteIdentityAndTinyValues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	broker := overviewAccount(t, s, owner, "Broker", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Tiny", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, i.ID, "0.00006", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "0.00006"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if _, err := db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id=?`, i.ID.String()); err != nil {
		t.Fatal(err)
	}
	missing, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range missing.Points {
		if p.Assets != nil || p.KnownAssets != nil || p.NetWorth != nil || p.AssetCoverage.Missing != 1 {
			t.Fatalf("all missing became zero: %+v", p)
		}
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, i.ID, "0.00006", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, i.ID, "0.00006", "2026-08-02", false); err != nil {
		t.Fatal(err)
	}
	captured, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range captured.Points {
		wantOverviewAmount(t, p.Assets, "0.00012")
	}
	if captured.Points[1].BasisChanged || captured.Points[1].ScopeChanged || !captured.Points[1].ConnectNetWorth {
		t.Fatal("quote identity caused a break")
	}
	original, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, captured.Overview) {
		t.Fatal("holding evidence differs from P1")
	}
}

func TestHistoricalSeriesDSTAndNegativeNetWorth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	*now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "10")
	overviewAccount(t, s, owner, "Loan", "loan", "liability", "balance", "CNY", "100")
	if _, err := s.StartHistory(ctx, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	r, err := s.HistoricalOverviewSeries(ctx, "2026-03-07", "2026-03-09")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := time.Parse(time.RFC3339Nano, r.Points[0].CutoffAt)
	next, _ := time.Parse(time.RFC3339Nano, r.Points[1].CutoffAt)
	if next.Sub(first) != 23*time.Hour {
		t.Fatal("civil DST boundary normalized")
	}
	for _, p := range r.Points {
		wantOverviewAmount(t, p.NetWorth, "-90")
	}
}

func TestHistoricalSeriesPreflightLeafAndByteBudgets(t *testing.T) {
	t.Parallel()
	s, db, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Synthetic A", "bank_account", "asset", "balance", "CNY", "1")
	overviewAccount(t, s, owner, "Synthetic B", "bank_account", "asset", "balance", "CNY", "2")
	if _, err := s.StartHistory(context.Background(), "UTC"); err != nil {
		t.Fatal(err)
	}
	for _, limits := range []domain.HistoricalSeriesLimits{{MaxLeaves: 1, MaxFacts: 100000, MaxBytes: 32 << 20}, {MaxLeaves: 1000, MaxFacts: 100000, MaxBytes: 1}} {
		batch, work, err := sqlite.NewRepository(db).LoadHistoricalOverviewSeriesBatch(context.Background(), *now, limits)
		var budget *domain.Error
		if !errors.As(err, &budget) || budget.Code != domain.ErrHistoricalSeriesBudget || batch.Origin.ID != "" {
			t.Fatalf("preflight hydrated a rejected profile: %+v %v", work, err)
		}
		if work.Leaves > limits.MaxLeaves+1 {
			t.Fatal("leaf preflight exceeded bounded lookahead")
		}
	}
}

func TestHistoricalSeriesDefaultFactLimitBeforeHydration(t *testing.T) {
	s, db, _, now := overviewFixture(t)
	if _, err := s.StartHistory(context.Background(), "UTC"); err != nil {
		t.Fatal(err)
	}
	// Facts need not be economically selected to exceed the conservative cap.
	// There are deliberately no effects to decode; the preflight must stop first.
	_, err := db.SQL.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<100001)
INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at)
SELECT 'synthetic-budget-'||x,(SELECT id FROM households),'value_update','other','2026-08-01T12:00:00Z','2026-08-01','2026-08-01T12:00:00Z' FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	batch, work, err := sqlite.NewRepository(db).LoadHistoricalOverviewSeriesBatch(context.Background(), *now, historicalSeriesLimits)
	var budget *domain.Error
	if !errors.As(err, &budget) || budget.Code != domain.ErrHistoricalSeriesBudget || work.CandidateFacts != 100001 || batch.Origin.ID != "" {
		t.Fatalf("unbounded/default hydration: %+v %v", work, err)
	}
	if work.Queries > 40 {
		t.Fatalf("continued hydrating after fact rejection: %d queries", work.Queries)
	}
}

func TestHistoricalSeriesBasisBreakAndCancellationAfterRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Synthetic foreign cash", "bank_account", "asset", "balance", "USD", "10")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	out, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Points[1].BasisChanged || out.Points[1].ScopeChanged || out.Points[1].ConnectAssets {
		t.Fatalf("FX basis not explicit: %+v", out.Points[1])
	}
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	original := s.repository
	s.repository = &historicalSeriesProbe{Repository: original, reader: original.(historicalSeriesRepository), after: cancel}
	out, err = s.HistoricalOverviewSeries(cancelled, "2026-08-01", "2026-08-03")
	if !errors.Is(err, context.Canceled) || len(out.Points) != 0 || out.Overview.CapturedAt != "" {
		t.Fatalf("cancel after read returned partial data: %+v %v", out, err)
	}
}

func TestHistoricalSeriesCorrectionsMatchP1EconomicTime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	command := domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "20", "CNY"), Reason: domain.ReasonOther, EffectiveAt: *now}
	change, err := s.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.AppendAccountValue(ctx, a.Account.ID, "150", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	command.Amount = mustMoney(t, "30", "CNY")
	command.EffectiveAt = *now
	if _, err := s.FixChange(ctx, change.Activity.ID, command); err != nil {
		t.Fatal(err)
	}
	out, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"100", "130", "150"} {
		wantOverviewAmount(t, out.Points[i].NetWorth, want)
	}
	original, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-03")
	if err != nil || !reflect.DeepEqual(original, out.Overview) {
		t.Fatalf("correction endpoints differ: %v", err)
	}
}

func TestHistoricalSeriesMarketCloseAndCoverageEquivalence(t *testing.T) {
	t.Parallel()
	// Singapore's household day ends before the US market close. The existing
	// daily-summary policy still prices its market-date label, not one instant.
	ctx := context.Background()
	s, _, _, _ := overviewFixture(t)
	accountID, instrumentID, holdingID := domain.NewAccountID(), domain.NewInstrumentID(), domain.NewHoldingID()
	provider := domain.TiingoProviderKey
	originAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 9, 9, 15, 59, 59, 999000000, time.UTC)
	quantity := mustQuantity(t, "10")
	account := domain.Account{ID: accountID, Name: "US broker", AccountType: "brokerage", TrackingMode: domain.TrackingHoldings, DefaultCurrency: "USD", BalanceSheetRole: domain.RoleAsset, IncludeInNetWorth: true, CreatedAt: originAt}
	instrument := domain.Instrument{ID: instrumentID, Name: "Stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &provider, ProviderBindingRevision: 1, CreatedAt: originAt}
	quote := func(price, date string, at time.Time) domain.InstrumentQuote {
		unitPrice, err := domain.ParseUnitPrice(price)
		if err != nil {
			t.Fatal(err)
		}
		return domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: unitPrice, Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: date, ValueEffectiveAt: at, QuotedAt: at.Add(48 * time.Hour), BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose), Revision: 1}
	}
	batch := domain.HistoricalSnapshotBatch{
		Origin:                    domain.HistoryOrigin{StartedAt: originAt, Timezone: "Asia/Singapore"},
		OriginData:                domain.HistoryOriginData{Components: []domain.HistoryOriginComponent{{Kind: domain.HistoryOriginHoldingQuantity, AccountID: &accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity}}},
		Portfolio:                 domain.PortfolioSnapshot{Household: &domain.Household{BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: accountID, InstrumentID: instrumentID, Quantity: quantity, CreatedAt: originAt}}},
		InstrumentQuoteFacts:      []domain.InstrumentQuote{quote("180", "2026-09-08", cutoff.Add(-20*time.Hour))},
		InstrumentHistoryCoverage: []domain.InstrumentHistoryCoverage{{InstrumentID: instrumentID, ProviderKey: provider, BindingRevision: 1, SourcePolicyVersion: string(PriceBasisTiingoRawClose), CloseMarketDates: []string{"2026-09-08"}, UnverifiedDates: []string{"2026-09-09"}}},
	}
	checkSeries := func() {
		t.Helper()
		index, err := indexHistoricalSeries(ctx, &batch)
		if err != nil {
			t.Fatal(err)
		}
		portfolio, err := (HistoricalReplay{repository: s.repository, batch: &batch, quotes: index.quotes}).Snapshot(ctx, &batch.Origin, cutoff)
		if err != nil {
			t.Fatal(err)
		}
		point, state, _, _, selected, err := s.historicalSeriesPoint(ctx, portfolio, index, "2026-09-09", cutoff, "", 12*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		original, rows, err := s.historicalOverviewSide(ctx, &batch, "2026-09-09", cutoff, false, "", 12*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(point.NetWorth, original.NetWorth) || state.KnownAssets != original.KnownAssets || point.AssetsComplete != original.Complete {
			t.Fatalf("series resolver differs: %+v / %+v", point, original)
		}
		endpoint, err := s.historicalSeriesEndpoint(ctx, &batch, selected, index, "2026-09-09", cutoff, "", 12*time.Hour)
		if err != nil || !reflect.DeepEqual(rows, endpoint) {
			t.Fatalf("endpoint provenance differs: %v", err)
		}
	}
	checkSeries()
	state, rows, err := s.historicalOverviewSide(ctx, &batch, "2026-09-09", cutoff, false, "", 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if state.Complete || state.NetWorth != nil || state.KnownAssets != "1800" {
		t.Fatalf("pending close: %+v", state)
	}
	var pending *HistoricalOverviewCell
	for _, row := range rows {
		if row.Kind == "holding" {
			pending = row.Left
		}
	}
	if pending == nil || pending.Complete || !reflect.DeepEqual(pending.Missing, []string{string(domain.MissingHistoryCoverage)}) {
		t.Fatalf("missing coverage hidden: %+v", pending)
	}
	late := quote("186", "2026-09-09", cutoff.Add(4*time.Hour))
	batch.InstrumentQuoteFacts = append(batch.InstrumentQuoteFacts, late)
	batch.InstrumentHistoryCoverage[0].CloseMarketDates = append(batch.InstrumentHistoryCoverage[0].CloseMarketDates, "2026-09-09")
	batch.InstrumentHistoryCoverage[0].UnverifiedDates = nil
	checkSeries()
	state, rows, err = s.historicalOverviewSide(ctx, &batch, "2026-09-09", cutoff, false, "", 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, state.NetWorth, "1860")
	for _, row := range rows {
		if row.Kind == "holding" && (row.Left.Price == nil || row.Left.Price.MarketDate != "2026-09-09" || row.Left.Price.EffectiveAt != late.ValueEffectiveAt.Format(time.RFC3339Nano) || row.Left.Price.Freshness != "fresh") {
			t.Fatalf("late-close provenance: %+v", row.Left.Price)
		}
	}
}
