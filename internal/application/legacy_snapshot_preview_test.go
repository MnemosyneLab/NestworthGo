package application

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestLegacySnapshotWarmReadsPreserveGuardedPreview(t *testing.T) {
	for _, reader := range []string{"net_worth_trend", "portfolio_trend", "analysis"} {
		t.Run(reader, func(t *testing.T) {
			s, db, owner, now := overviewFixture(t)
			cash := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
			if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
				t.Fatal(err)
			}
			state, err := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
			if err != nil || state.DirtyFrom != nil {
				t.Fatal("fixture is not clean", state, err)
			}
			command := domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: *now}
			_, token, err := s.PreviewChangeGuarded(t.Context(), command)
			if err != nil {
				t.Fatal(err)
			}
			var before, after int
			if err := db.SQL.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
				t.Fatal(err)
			}
			switch reader {
			case "net_worth_trend":
				_, err = s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-10:2026-08-11"))
			case "portfolio_trend":
				_, err = s.PortfolioTrend(t.Context(), domain.TrendRange("2026-08-10:2026-08-11"))
			case "analysis":
				_, err = s.Analyze(t.Context(), testReturnQuery("2026-08-10", "2026-08-11"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.SQL.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatal("warm read wrote snapshots", before, after)
			}
			_, err = s.RecordChangeGuarded(t.Context(), command, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
			t.Logf("reader=%s snapshot/database changes=%d guarded commit err=%v", reader, after-before, err)
			if err != nil {
				t.Fatal("warm read invalidated original preview", err)
			}
		})
	}
}

func TestLegacySnapshotWritesInvalidateGuardedPreview(t *testing.T) {
	for _, reader := range []string{"net_worth_trend", "portfolio_trend", "analysis"} {
		t.Run(reader, func(t *testing.T) {
			s, db, owner, now := overviewFixture(t)
			cash := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
			if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
			if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-10"); err != nil {
				t.Fatal(err)
			}
			command := domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: *now}
			_, token, err := s.PreviewChangeGuarded(t.Context(), command)
			if err != nil {
				t.Fatal(err)
			}
			var before, after int
			if err := db.SQL.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
				t.Fatal(err)
			}
			switch reader {
			case "net_worth_trend":
				_, err = s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-10:2026-08-11"))
			case "portfolio_trend":
				_, err = s.PortfolioTrend(t.Context(), domain.TrendRange("2026-08-10:2026-08-11"))
			case "analysis":
				_, err = s.Analyze(t.Context(), testReturnQuery("2026-08-10", "2026-08-11"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.SQL.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after <= before {
				t.Fatal("fixture did not write a missing snapshot", before, after)
			}
			_, err = s.RecordChangeGuarded(t.Context(), command, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
			t.Logf("reader=%s snapshot/database changes=%d guarded commit err=%v", reader, after-before, err)
			if !hasDomainCode(err, domain.ErrStalePreview) {
				t.Fatal("snapshot write retained stale preview", err)
			}
		})
	}
}

func TestLegacySnapshotSameHashWritesInvalidateGuardedPreview(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	cash := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec("UPDATE history_snapshot_state SET dirty_from=?, dirty_to=?, input_generation=input_generation+1", "2026-08-10", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	command := domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: *now}
	_, token, err := s.PreviewChangeGuarded(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-10", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("fixture appended instead of reusing the same hash", before, after)
	}
	_, err = s.RecordChangeGuarded(t.Context(), command, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
	if !hasDomainCode(err, domain.ErrStalePreview) {
		t.Fatal("same-hash metadata writes retained stale preview", err)
	}
}

func TestLegacySnapshotPureCoverageReadFencesGuardedWrite(t *testing.T) {
	s, _, r, cash, _ := legacyCoverageFixture(t)
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	command := domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: s.clock()}
	_, token, err := s.PreviewChangeGuarded(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	r.hook = func(event string) {
		if event == "coverage" {
			once.Do(func() { close(entered); <-release })
		}
	}
	readDone := make(chan error, 1)
	go func() { readDone <- s.ensureClosedDaySnapshots(t.Context(), "2026-08-10", "2026-08-11") }()
	<-entered
	writeStarted, writeDone := make(chan struct{}), make(chan error, 1)
	go func() {
		close(writeStarted)
		_, err := s.RecordChangeGuarded(t.Context(), command, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
		writeDone <- err
	}()
	<-writeStarted
	select {
	case err := <-writeDone:
		close(release)
		<-readDone
		t.Fatal("writer entered during coverage check", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal("pure coverage check invalidated waiting preview", err)
	}
	r.hook = nil
}
