package application

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestLegacyRepairRevisionRestartsFromEarlierDirtyPrefix(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprint("restart=", restart), func(t *testing.T) {
			s, db, r, cash, _ := legacyCoverageFixtureDays(t, 62)
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-01"); err != nil {
				t.Fatal(err)
			}
			income := func(service *Service, day int) {
				t.Helper()
				if _, err := service.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, day, 13, 0, 0, 0, time.UTC)}); err != nil {
					t.Fatal(err)
				}
			}
			income(s, 2)
			writer := NewService(sqlite.NewRepository(db))
			writer.setClock(s.clock)
			r.batches, r.saves = 0, 0
			revised := false
			r.hook = func(stage string) {
				if stage == "batch" && r.batches == 2 && !revised {
					revised = true
					income(writer, 3)
				}
			}
			_, err := s.RebuildDirtySnapshots(t.Context())
			r.hook = nil
			state, stateErr := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
			rows, rowsErr := r.ListDailyValuationSnapshots(t.Context(), cash.Account.HouseholdID, time.Time{}, time.Time{})
			if stateErr != nil || rowsErr != nil || len(rows) < 3 {
				t.Fatal(stateErr, rowsErr, rows)
			}
			health, healthErr := s.ScanMarketDataHealth(t.Context())
			dirty := ""
			if state.DirtyFrom != nil {
				dirty = *state.DirtyFrom
			}
			t.Logf("repair err=%v; Aug3=%s/gen%d; state gen=%d dirty=%s; healthy=%t", err, rows[2].NetWorthAmount.CanonicalAmount(), rows[2].InputGeneration, state.InputGeneration, dirty, health.Healthy)
			var problem *domain.Error
			if !errors.As(err, &problem) || problem.Code != domain.ErrConflict || problem.Field != "inputGeneration" || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-03" || healthErr != nil || health.Healthy {
				t.Errorf("must retain unrepaired early prefix and report conflict: err=%v state=%+v healthy=%t healthErr=%v", err, state, health.Healthy, healthErr)
			}
			if restart {
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				reopened, err := sqlite.Open(db.Path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = reopened.Close() })
				s = NewService(sqlite.NewRepository(reopened))
				s.setClock(writer.clock)
			}
			if _, err := s.RebuildDirtySnapshots(t.Context()); err != nil {
				t.Fatal(err)
			}
			rows, err = s.repository.ListDailyValuationSnapshots(t.Context(), cash.Account.HouseholdID, time.Time{}, time.Time{})
			if err != nil || len(rows) != 63 {
				t.Fatal(rows, err)
			}
			for _, row := range rows {
				date, _ := time.Parse("2006-01-02", row.LocalDate)
				days := int(date.Sub(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
				priceDays := days
				if priceDays > 61 {
					priceDays = 61
				} // last manual quote is Oct 1.
				want := 150 + priceDays*10
				if days >= 1 {
					want++
				}
				if days >= 2 {
					want++
				}
				if !row.Complete || row.NetWorthAmount.CanonicalAmount() != fmt.Sprint(want) || (days >= 2 && row.InputGeneration != 2) {
					t.Errorf("unrepaired %s amount=%s/gen%d; want %d/gen2", row.LocalDate, row.NetWorthAmount.CanonicalAmount(), row.InputGeneration, want)
				}
			}
			state, err = s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
			health, healthErr = s.ScanMarketDataHealth(t.Context())
			if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-10-03" || state.DirtyTo != nil || healthErr != nil || !health.Healthy {
				t.Fatal("repair not recovered", state, health, err, healthErr)
			}
		})
	}
}
