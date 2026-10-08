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

// Count builder saves as well as physical revisions: equal-hash rebuilds can
// update a row in place and must not hide unnecessary reconstruction in tests.
type coverageRepository struct {
	*sqlite.Repository
	generation     GenerationAwareSnapshotRepository
	saves, batches int
	hook           func(string)
}

func (r *coverageRepository) LoadHistoricalSnapshotBatch(ctx context.Context, id domain.HouseholdID, cutoff time.Time) (domain.HistoricalSnapshotBatch, error) {
	b, err := r.Repository.LoadHistoricalSnapshotBatch(ctx, id, cutoff)
	r.batches++
	if r.hook != nil {
		r.hook("batch")
	}
	return b, err
}
func (r *coverageRepository) ListDailyValuationSnapshots(ctx context.Context, id domain.HouseholdID, from, to time.Time) ([]domain.DailyValuationSnapshot, error) {
	rows, err := r.Repository.ListDailyValuationSnapshots(ctx, id, from, to)
	if r.hook != nil {
		r.hook("coverage")
	}
	return rows, err
}
func (r *coverageRepository) SaveDailyValuationSnapshotAndMarkCompletedAtGeneration(ctx context.Context, snapshot domain.DailyValuationSnapshot, at time.Time, generation int) (bool, error) {
	r.saves++
	changed, err := r.generation.SaveDailyValuationSnapshotAndMarkCompletedAtGeneration(ctx, snapshot, at, generation)
	if r.hook != nil {
		r.hook("save")
	}
	return changed, err
}
func (r *coverageRepository) CompleteDailySnapshotRangeAtGeneration(ctx context.Context, id domain.HouseholdID, to string, at time.Time, generation int) error {
	return r.generation.CompleteDailySnapshotRangeAtGeneration(ctx, id, to, at, generation)
}
func legacyCoverageFixture(t *testing.T) (*Service, *sqlite.DB, *coverageRepository, domain.AccountRecord, domain.Instrument) {
	t.Helper()
	return legacyCoverageFixtureDays(t, 11)
}

func legacyCoverageFixtureDays(t *testing.T, quoteDays int) (*Service, *sqlite.DB, *coverageRepository, domain.AccountRecord, domain.Instrument) {
	t.Helper()
	s, db, owner, now := overviewFixture(t)
	cash := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
	broker := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
	instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= quoteDays; day++ {
		date := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day-1).Format("2006-01-02")
		when, err := s.parseManualQuoteTimestamp(t.Context(), date)
		if err != nil {
			t.Fatal(err)
		}
		price, err := domain.ParseUnitPrice(fmt.Sprint(day + 4))
		if err != nil {
			t.Fatal(err)
		}
		// Seed pre-history facts with the same constructor and persistence path
		// as AppendManualInstrumentQuote, avoiding its growing portfolio recapture.
		// Source-revision tests still invoke the public mutation after StartHistory.
		quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: when}, s.clock())
		if err != nil {
			t.Fatal(err)
		}
		if err := s.repository.AppendInstrumentQuote(t.Context(), quote); err != nil {
			t.Fatal(err)
		}
	}
	holding, err := s.CreateHolding(t.Context(), HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(t.Context(), "UTC", map[domain.HoldingID]string{holding.ID: "5"}); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	r := &coverageRepository{Repository: s.repository.(*sqlite.Repository), generation: s.repository.(GenerationAwareSnapshotRepository)}
	s.repository = r
	state, err := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
	if err != nil || state.DirtyFrom != nil || state.LastCompletedClosedOn != nil {
		t.Fatal("fixture not clean", state, err)
	}
	return s, db, r, cash, instrument
}
func coverageDates(t *testing.T, s *Service, id domain.HouseholdID) []string {
	t.Helper()
	rows, err := s.repository.ListDailyValuationSnapshots(t.Context(), id, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	dates := []string{}
	for _, row := range rows {
		if !row.Complete {
			t.Fatal("incomplete synthetic row", row.LocalDate)
		}
		dates = append(dates, row.LocalDate)
	}
	return dates
}
func legacyCoverageRead(t *testing.T, s *Service, reader, from, to string) {
	t.Helper()
	switch reader {
	case "analysis":
		result, err := s.Analyze(t.Context(), testReturnQuery(from, to))
		if err != nil || result.Status != domain.CompletenessOK || result.ReturnAmount == nil {
			t.Fatal(result, err)
		}
		// On the history-origin day the opening is the Starting point; other
		// periods include the return from their predecessor's close.
		days := 2
		if from == "2026-08-01" {
			days = 1
		}
		if result.ReturnAmount.CanonicalAmount() != fmt.Sprint(days*10) {
			t.Fatal("wrong financial return", result.ReturnAmount)
		}
	case "net_worth_trend":
		result, err := s.NetWorthTrend(t.Context(), domain.TrendRange(from+":"+to))
		if err != nil || !result.Complete || len(result.Points) != 2 || result.Change == nil || result.Change.CanonicalAmount() != "10" {
			t.Fatal(result, err)
		}
		for _, point := range result.Points {
			if point.Status != domain.TrendPointComplete || point.NetWorth == nil {
				t.Fatal(point)
			}
		}
	case "portfolio_trend":
		result, err := s.PortfolioTrend(t.Context(), domain.TrendRange(from+":"+to))
		if err != nil || len(result.Points) != 2 {
			t.Fatal(result, err)
		}
		for _, point := range result.Points {
			if !point.Complete || point.Status != domain.TrendPointComplete || point.ValuedSubtotal == nil {
				t.Fatal(point)
			}
		}
		change := result.Points[1].ValuedSubtotal.Amount().Sub(result.Points[0].ValuedSubtotal.Amount())
		if change.String() != "10" {
			t.Fatal("wrong portfolio change", change)
		}
	case "attribution":
		result := attributionFor(t, s, from, to)
		wantAttributionStatus(t, result, "compatible", "")
		wantOverviewAmount(t, result.Content.Change.NetWorth.Value, "10")
		assertAttributionPrecisionIdentity(t, result)
	}
}
func TestLegacySnapshotCoverageOrder(t *testing.T) {
	t.Parallel()
	for _, first := range []string{"attribution", "analysis"} {
		for _, reader := range []string{"analysis", "net_worth_trend", "portfolio_trend"} {
			for _, lateFirst := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s_%s_lateFirst=%t", first, reader, lateFirst), func(t *testing.T) {
					s, _, r, cash, _ := legacyCoverageFixture(t)
					late := func() { legacyCoverageRead(t, s, first, "2026-08-10", "2026-08-11") }
					early := func() { legacyCoverageRead(t, s, reader, "2026-08-01", "2026-08-02") }
					if lateFirst {
						late()
						t.Log("after late", coverageDates(t, s, cash.Account.HouseholdID))
						early()
					} else {
						early()
						late()
					}
					want := []string{"2026-08-01", "2026-08-02", "2026-08-10", "2026-08-11"}
					if first == "analysis" {
						want = []string{"2026-08-01", "2026-08-02", "2026-08-09", "2026-08-10", "2026-08-11"}
					}
					if got := coverageDates(t, s, cash.Account.HouseholdID); !reflect.DeepEqual(got, want) {
						t.Fatal(got, want)
					}
					saves := r.saves
					early()
					late()
					early()
					if r.saves != saves {
						t.Fatal("repeat reconstructed snapshots", r.saves, saves)
					}
				})
			}
		}
	}
}
func TestLegacySnapshotCoverageRepairsOnlyRequiredDays(t *testing.T) {
	t.Parallel()
	s, db, r, cash, _ := legacyCoverageFixture(t)
	id := cash.Account.HouseholdID
	if _, err := s.RebuildHistoricalSnapshots(t.Context(), "2026-08-05", "2026-08-05"); err != nil {
		t.Fatal(err)
	}
	before := r.saves
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-03", "2026-08-07"); err != nil {
		t.Fatal(err)
	}
	if r.saves != before+4 {
		t.Fatal("partial coverage rebuilt an existing day", r.saves, before)
	}
	for _, tc := range []struct{ column, value string }{{"content_hash", "v2:old"}, {"resolver_policy_version", "old-policy"}} {
		if _, err := db.SQL.Exec("UPDATE daily_valuation_snapshots SET "+tc.column+"=? WHERE local_date=?", tc.value, "2026-08-04"); err != nil {
			t.Fatal(err)
		}
		before = r.saves
		if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-03", "2026-08-07"); err != nil {
			t.Fatal(err)
		}
		if r.saves != before+1 {
			t.Fatal("stale row expanded into history rebuild", tc, r.saves, before)
		}
	}
	if _, err := db.SQL.Exec("UPDATE daily_valuation_snapshots SET content_hash=? WHERE local_date=?", "v2:outside", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	before = r.saves
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-05", "2026-08-07"); err != nil {
		t.Fatal(err)
	}
	if r.saves != before {
		t.Fatal("unrequested old hash rebuilt history")
	}
	if got := coverageDates(t, s, id); len(got) != 5 {
		t.Fatal(got)
	}
}
func TestLegacySnapshotCoverageDirtyPrefixAndTail(t *testing.T) {
	t.Parallel()
	for _, reader := range []string{"analysis", "net_worth_trend", "portfolio_trend"} {
		t.Run(reader, func(t *testing.T) {
			s, _, r, cash, instrument := legacyCoverageFixture(t)
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AppendManualInstrumentQuote(t.Context(), instrument.ID, "7", "2026-08-02", false); err != nil {
				t.Fatal(err)
			}
			legacyCoverageRead(t, s, reader, "2026-08-10", "2026-08-11")
			lateSaves := r.saves
			legacyCoverageRead(t, s, reader, "2026-08-10", "2026-08-11")
			legacyCoverageRead(t, s, "attribution", "2026-08-10", "2026-08-11")
			if r.saves != lateSaves {
				t.Fatal("current-generation later range rebuilt under retained earlier dirty prefix", r.saves, lateSaves)
			}
			state, err := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
			if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-02" {
				t.Fatal("earlier dirty prefix lost", state, err)
			}
			// Read the revised early range without the fixed 10-unit control assertion.
			result, err := s.Analyze(t.Context(), testReturnQuery("2026-08-01", "2026-08-02"))
			if err != nil || result.ReturnAmount == nil || result.ReturnAmount.CanonicalAmount() != "20" {
				t.Fatal("source revision ignored", result, err)
			}
			linked := attributionFor(t, s, "2026-08-01", "2026-08-02")
			wantAttributionStatus(t, linked, "compatible", "")
			wantOverviewAmount(t, linked.Content.Change.NetWorth.Value, "20")
			assertAttributionPrecisionIdentity(t, linked)
			trend, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-01:2026-08-02"))
			if err != nil || !trend.Complete || trend.Change == nil || trend.Change.CanonicalAmount() != "20" {
				t.Fatal("mixed calls disagree", trend, err)
			}
			state, err = s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
			if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-03" || state.DirtyTo != nil || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-08-11" {
				t.Fatal("uncovered tail/watermark lost", state, err)
			}
			before := r.saves
			// Dirty data outside the requested prefix must not force repeated builds.
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-02"); err != nil {
				t.Fatal(err)
			}
			if r.saves != before {
				t.Fatal("clean prefix rebuilt")
			}
		})
	}
}
func TestLegacySnapshotCoverageChunksAndConcurrentRevision(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"none", "coverage", "batch", "between_chunks"} {
		t.Run(stage, func(t *testing.T) {
			s, _, r, cash, _ := legacyCoverageFixtureDays(t, 41)
			if stage == "coverage" {
				if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-09-10"); err != nil {
					t.Fatal(err)
				}
			}
			// A second service models a source revision outside this service's
			// serial write coordinator (e.g. another repository client).
			writer := NewService(r.Repository)
			writer.setClock(s.clock)
			revised := false
			r.hook = func(event string) {
				trigger := event == stage || (stage == "between_chunks" && event == "save" && r.saves == 31)
				if revised || !trigger {
					return
				}
				revised = true
				done := make(chan error, 1)
				go func() {
					_, err := writer.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)})
					done <- err
				}()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
			before := r.batches
			err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-09-10")
			if stage == "none" {
				if err != nil || r.batches != before+2 || r.saves != 41 {
					t.Fatal("31-day chunks", r.batches, r.saves, err)
				}
			} else {
				var problem *domain.Error
				if !errors.As(err, &problem) || problem.Code != domain.ErrConflict || problem.Field != "inputGeneration" {
					t.Fatal("concurrent invalidation not rejected", err)
				}
				state, stateErr := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
				if stateErr != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-02" {
					t.Fatal("revision dirty prefix lost", state, stateErr)
				}
			}
			r.hook = nil
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-09-10"); err != nil {
				t.Fatal(err)
			}
			if got := coverageDates(t, s, cash.Account.HouseholdID); len(got) != 41 {
				t.Fatal("resume failed", got)
			}
			before = r.saves
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-09-10"); err != nil {
				t.Fatal(err)
			}
			if r.saves != before {
				t.Fatal("repeated chunks rebuilt")
			}
		})
	}
}

func TestLegacySnapshotCoverageBoundedDirtyRange(t *testing.T) {
	t.Parallel()
	s, db, r, cash, _ := legacyCoverageFixture(t)
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec("UPDATE history_snapshot_state SET dirty_from=?, dirty_to=?", "2026-08-02", "2026-08-05"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		from, to, remaining string
		saves               int
	}{
		{"2026-08-07", "2026-08-08", "2026-08-02", 0},
		{"2026-08-04", "2026-08-05", "2026-08-02", 2},
		{"2026-08-01", "2026-08-02", "2026-08-03", 1},
		{"2026-08-03", "2026-08-06", "", 3},
	} {
		before := r.saves
		if err := s.ensureClosedDaySnapshots(t.Context(), tc.from, tc.to); err != nil {
			t.Fatal(err)
		}
		state, err := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
		if err != nil || r.saves != before+tc.saves || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-08-11" {
			t.Fatal(tc, state, r.saves, before, err)
		}
		if tc.remaining == "" {
			if state.DirtyFrom != nil || state.DirtyTo != nil {
				t.Fatal(state)
			}
		} else if state.DirtyFrom == nil || *state.DirtyFrom != tc.remaining || state.DirtyTo == nil || *state.DirtyTo != "2026-08-05" {
			t.Fatal(tc, state)
		}
	}
}

func TestLegacySnapshotCoverageRetainsMidnightContract(t *testing.T) {
	t.Parallel()
	s, _, owner, now := overviewFixture(t)
	*now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	cash := overviewAccount(t, s, owner, "Synthetic Havana cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "America/Havana"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	// The repeated midnight at the left date is unsupported by attribution's
	// preflight. Legacy origin-day analysis and trends only need the NEXT
	// midnight to build this date's close and retain their existing support.
	if attributionBoundariesSupported("2026-11-01", "2026-11-02", "America/Havana") {
		t.Fatal("fixture lacks a repeated left midnight")
	}
	result, err := s.Analyze(t.Context(), testReturnQuery("2026-11-01", "2026-11-02"))
	if err != nil || len(result.Days) != 2 {
		t.Fatal(result, err)
	}
	for _, day := range result.Days {
		if day.Status != domain.CompletenessOK || day.EndingValue.CanonicalAmount() != "100" {
			t.Fatal(day)
		}
	}
	trend, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-11-01:2026-11-02"))
	if err != nil || !trend.Complete || len(trend.Points) != 2 || trend.Start == nil || trend.Start.CanonicalAmount() != "100" {
		t.Fatal(trend, err)
	}
	if got := coverageDates(t, s, cash.Account.HouseholdID); !reflect.DeepEqual(got, []string{"2026-11-01", "2026-11-02"}) {
		t.Fatal(got)
	}
	wantAttributionStatus(t, attributionFor(t, s, "2026-11-01", "2026-11-02"), "incompatible", "historical_boundary_unsupported")
}

func TestLegacySnapshotCoverageSourcePreferenceRevision(t *testing.T) {
	t.Parallel()
	s, _, r, cash, instrument := legacyCoverageFixture(t)
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendInstrumentPreferenceObservation(t.Context(), domain.InstrumentPreferenceObservation{InstrumentID: instrument.ID, SourceKind: domain.QuoteSourceProvider, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	before := r.saves
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-02"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.repository.ListDailyValuationSnapshots(t.Context(), cash.Account.HouseholdID, time.Time{}, time.Time{})
	if err != nil || r.saves != before+1 || !rows[0].Complete || rows[1].Complete {
		t.Fatal("source preference revision ignored", r.saves, before, rows, err)
	}
	// Switching back to the historical manual source restores the same amounts.
	if err := s.AppendInstrumentPreferenceObservation(t.Context(), domain.InstrumentPreferenceObservation{InstrumentID: instrument.ID, SourceKind: domain.QuoteSourceManual, EffectiveAt: time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	legacyCoverageRead(t, s, "analysis", "2026-08-01", "2026-08-02")
	legacyCoverageRead(t, s, "net_worth_trend", "2026-08-01", "2026-08-02")
}
