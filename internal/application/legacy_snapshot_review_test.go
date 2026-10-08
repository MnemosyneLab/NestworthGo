package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestLegacySnapshotBoundedInvalidationThenOrdinaryIncome(t *testing.T) {
	s, _, r, cash, instrument := legacyCoverageFixture(t)
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	// The production history-commit entry point creates a bounded invalidation;
	// no SQL dirty-state fabrication or live provider is involved.
	if _, err := r.CommitInstrumentHistory(t.Context(), sqlite.InstrumentHistoryCommit{
		HouseholdID: cash.Account.HouseholdID, InstrumentID: instrument.ID,
		ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "SYNTHETIC", QuoteCurrency: "CNY",
		Status: "mapped", Adapter: "yahoo_chart", SourcePolicy: string(PriceBasisYahooClose), FetchedAt: now,
		VerifiedRanges: []sqlite.DateSpan{{Start: "2026-08-02", End: "2026-08-02"}},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-02" || state.DirtyTo == nil || *state.DirtyTo != "2026-08-05" {
		t.Fatal("real bounded invalidation", state, err)
	}
	now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-06", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "50", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 3, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	state, err = s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
	t.Logf("after ordinary income: state=%+v err=%v", state, err)
	late, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-10:2026-08-11"))
	if late.Start != nil {
		t.Log("Aug 10", late.Start.CanonicalAmount())
	}
	if err != nil || late.Start == nil || late.Start.CanonicalAmount() != "290" {
		t.Errorf("Aug 10 net worth: %+v err=%v; want 290", late.Start, err)
	}
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-02", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	boundary, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-05:2026-08-06"))
	if boundary.Change != nil {
		t.Log("Aug 5→6", boundary.Change.CanonicalAmount())
	}
	if err != nil || boundary.Change == nil || boundary.Change.CanonicalAmount() != "10" {
		t.Errorf("Aug 5→6 change: %+v err=%v; want 10", boundary.Change, err)
	}
	late, err = s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-10:2026-08-11"))
	if late.Start != nil {
		t.Log("Aug 10 after clearing", late.Start.CanonicalAmount())
	}
	if err != nil || late.Start == nil || late.Start.CanonicalAmount() != "290" {
		t.Errorf("after clearing dirty range, Aug 10: %+v err=%v; want 290", late.Start, err)
	}
	analysis, err := s.Analyze(t.Context(), testReturnQuery("2026-08-05", "2026-08-06"))
	if err != nil || analysis.ReturnAmount == nil || analysis.ReturnAmount.CanonicalAmount() != "20" {
		t.Fatal("inclusive-period return", analysis, err)
	}
	attribution := attributionFor(t, s, "2026-08-05", "2026-08-06")
	wantAttributionStatus(t, attribution, "compatible", "")
	wantOverviewAmount(t, attribution.Content.Change.NetWorth.Value, "10")
	assertAttributionPrecisionIdentity(t, attribution)
}

func TestLegacySQLBoundedDirtyThenOrdinaryIncome(t *testing.T) {
	s, db, r, cash, _ := legacyCoverageFixture(t)
	s.setClock(func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) })
	if _, err := db.SQL.Exec("UPDATE history_snapshot_state SET dirty_from=?,dirty_to=?", "2026-08-02", "2026-08-05"); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-06", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "50", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 3, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	for _, early := range []bool{false, true} {
		if early {
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-02", "2026-08-11"); err != nil {
				t.Fatal(err)
			}
		}
		trend, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-10:2026-08-11"))
		if err != nil || !trend.Complete || trend.Start.CanonicalAmount() != "290" {
			t.Fatal("SQL bounded-state financial amount", early, trend, err)
		}
	}
	boundary, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-05:2026-08-06"))
	if err != nil || !boundary.Complete || boundary.Change.CanonicalAmount() != "10" {
		t.Fatal("SQL bounded-state complete delta", boundary, err)
	}
	before := r.saves
	if _, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-05:2026-08-06")); err != nil || r.saves != before {
		t.Fatal("repeat rebuilt", r.saves-before, err)
	}
}

func TestLegacyMixedGenerationReadersRecoverAfterRestart(t *testing.T) {
	for _, reader := range []string{"analysis", "net_worth_trend", "portfolio_trend", "attribution"} {
		for _, restart := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_restart=%t", reader, restart), func(t *testing.T) {
				s, db, r, cash, _ := legacyCoverageFixtureDays(t, 41)
				writer := NewService(sqlite.NewRepository(db))
				writer.setClock(s.clock)
				revised := false
				r.hook = func(stage string) {
					if stage != "save" || r.saves != 31 || revised {
						return
					}
					revised = true
					if _, err := writer.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)}); err != nil {
						t.Fatal(err)
					}
				}
				err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-09-10")
				var problem *domain.Error
				if !errors.As(err, &problem) || problem.Code != domain.ErrConflict || problem.Field != "inputGeneration" {
					t.Fatal("mixed publication must conflict", err)
				}
				r.hook = nil
				rows, err := r.ListDailyValuationSnapshots(t.Context(), cash.Account.HouseholdID, time.Time{}, time.Time{})
				if err != nil || len(rows) != 41 || rows[1].InputGeneration == rows[40].InputGeneration || rows[1].NetWorthAmount.CanonicalAmount() != "160" || rows[40].NetWorthAmount.CanonicalAmount() != "551" {
					t.Fatal("mixed-generation fixture", rows, err)
				}
				if restart {
					path := db.Path
					if err := db.Close(); err != nil {
						t.Fatal(err)
					}
					reopened, err := sqlite.Open(path)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = reopened.Close() })
					s = NewService(sqlite.NewRepository(reopened))
					s.setClock(writer.clock)
				}
				health, err := s.ScanMarketDataHealth(t.Context())
				if err != nil || health.Healthy || !hasKind(health, HealthKindSnapshotOutdated) {
					t.Fatal("mixed rows accepted by health", health, err)
				}
				switch reader {
				case "analysis":
					result, err := s.Analyze(t.Context(), testReturnQuery("2026-08-01", "2026-09-10"))
					if err != nil || result.Status != domain.CompletenessOK || len(result.DailyReturns) != 41 || result.ReturnAmount == nil || result.ReturnAmount.CanonicalAmount() != "400" {
						t.Fatal("analysis return", result.Status, result.ReturnAmount, err)
					}
					aug2, last := mustMoney(t, "0", "CNY").Amount(), mustMoney(t, "0", "CNY").Amount()
					for _, day := range result.Days {
						if day.Date == "2026-08-02" {
							aug2 = aug2.Add(day.EndingValue.Amount())
						}
						if day.Date == "2026-09-10" {
							last = last.Add(day.EndingValue.Amount())
						}
					}
					if aug2.String() != "161" || last.String() != "551" {
						t.Fatal("analysis accepted mixed component values", aug2, last)
					}
				case "net_worth_trend":
					result, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-01:2026-09-10"))
					if err != nil || !result.Complete || result.Start.CanonicalAmount() != "150" || result.End.CanonicalAmount() != "551" || result.Change.CanonicalAmount() != "401" {
						t.Fatal("trend accepted mixed values", result, err)
					}
				case "portfolio_trend":
					result, err := s.PortfolioTrend(t.Context(), domain.TrendRange("2026-08-01:2026-09-10"))
					if err != nil || len(result.Points) != 41 || result.Points[1].ValuedSubtotal.CanonicalAmount() != "60" || result.Points[40].ValuedSubtotal.CanonicalAmount() != "450" {
						t.Fatal("portfolio instrument values", result, err)
					}
				case "attribution":
					result := attributionFor(t, s, "2026-08-01", "2026-09-10")
					wantAttributionStatus(t, result, "compatible", "")
					wantOverviewAmount(t, result.Content.Change.NetWorth.Value, "401")
					assertAttributionPrecisionIdentity(t, result)
				}
				rows, err = s.repository.ListDailyValuationSnapshots(t.Context(), cash.Account.HouseholdID, time.Time{}, time.Time{})
				if err != nil || len(rows) != 41 {
					t.Fatal(rows, err)
				}
				for _, row := range rows {
					date, _ := time.Parse("2006-01-02", row.LocalDate)
					days := int(date.Sub(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
					want := 150 + days*10
					if days > 0 {
						want++
					}
					if !row.Complete || row.NetWorthAmount.CanonicalAmount() != fmt.Sprint(want) || (days > 0 && row.InputGeneration != 1) {
						t.Fatal("unrepaired amount/provenance", row.LocalDate, row.NetWorthAmount, row.InputGeneration, want)
					}
				}
			})
		}
	}
}

func TestLegacySparseSnapshotHealthAndRepair(t *testing.T) {
	s, db, r, cash, _ := legacyCoverageFixtureDays(t, 62)
	legacyCoverageRead(t, s, "attribution", "2026-08-10", "2026-08-11")
	if _, err := s.NetWorthTrend(t.Context(), domain.Trend30Days); err != nil {
		t.Fatal(err)
	}
	rows := coverageDates(t, s, cash.Account.HouseholdID)
	if len(rows) != 31 {
		t.Fatal("sparse fixture", rows)
	}
	before := r.saves
	batches := r.batches
	report, err := s.ScanMarketDataHealth(t.Context())
	if err != nil || report.Healthy || report.SnapshotDays != 32 || !hasKind(report, HealthKindSnapshotMissing) {
		t.Errorf("32 missing closes must be reported: %+v err=%v", report, err)
	}
	preview, err := s.PreviewMarketDataSync(t.Context(), SyncRequest{Scope: SyncScopeRepairAll})
	if err != nil || preview.SnapshotWorkEstimate != 32 || r.saves != before {
		t.Errorf("read-only repair estimate: %+v saves=%d err=%v", preview, r.saves-before, err)
	}
	count, err := s.RebuildDirtySnapshots(context.Background())
	if err != nil || count != 32 || r.saves != before+32 || r.batches != batches+2 {
		t.Fatal("repair only holes", count, r.saves-before, err)
	}
	report, err = s.ScanMarketDataHealth(t.Context())
	if err != nil || !report.Healthy || report.SnapshotDays != 0 {
		t.Fatal("repaired health", report, err)
	}
	before = r.saves
	count, err = s.RebuildDirtySnapshots(t.Context())
	if err != nil || count != 0 || r.saves != before {
		t.Fatal("repeat repair rebuilt valid rows", count, r.saves-before, err)
	}
	for _, tc := range []struct{ column, value string }{{"content_hash", "v2:old"}, {"resolver_policy_version", "old-policy"}} {
		if _, err := db.SQL.Exec("UPDATE daily_valuation_snapshots SET "+tc.column+"=? WHERE local_date=?", tc.value, "2026-08-10"); err != nil {
			t.Fatal(err)
		}
		report, err = s.ScanMarketDataHealth(t.Context())
		if err != nil || report.Healthy || report.SnapshotDays != 1 || !hasKind(report, HealthKindSnapshotOutdated) {
			t.Fatal("stale-row health", report, err)
		}
		before = r.saves
		if _, err := s.RebuildDirtySnapshots(t.Context()); err != nil || r.saves != before+1 {
			t.Fatal("repair only stale row", r.saves-before, err)
		}
	}
}
