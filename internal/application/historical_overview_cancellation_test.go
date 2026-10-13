package application

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type historicalCaptureCancelRepository struct {
	Repository
	after   func()
	loadErr error
}

func (r historicalCaptureCancelRepository) LoadHistoricalSnapshotBatch(ctx context.Context, id domain.HouseholdID, cutoff time.Time) (domain.HistoricalSnapshotBatch, error) {
	batch, err := r.Repository.LoadHistoricalSnapshotBatch(ctx, id, cutoff)
	r.after()
	if r.loadErr != nil {
		return domain.HistoricalSnapshotBatch{}, r.loadErr
	}
	return batch, err
}

func TestHistoricalOverviewCancellationErrorRace(t *testing.T) {
	t.Parallel()
	s, _, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Synthetic cancellation race", "bank_account", "asset", "balance", "CNY", "1")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	original := s.repository
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.repository = historicalCaptureCancelRepository{Repository: original, after: cancel, loadErr: sql.ErrTxDone}
	out, err := s.HistoricalOverview(ctx, "2026-08-01", "current")
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, HistoricalOverviewResult{}) {
		t.Fatalf("transaction-close race lost cancellation identity: %+v %v", out, err)
	}
	s.repository = historicalCaptureCancelRepository{Repository: original, after: func() {}, loadErr: sql.ErrTxDone}
	if _, err := s.HistoricalOverview(t.Context(), "2026-08-01", ""); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("live infrastructure error was suppressed: %v", err)
	}
}

func TestHistoricalOverviewCancellationAfterRead(t *testing.T) {
	t.Parallel()
	s, _, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Synthetic cancellation", "bank_account", "asset", "balance", "CNY", "0.0001")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 3)
	original := s.repository
	for _, compare := range []string{"", "2026-08-03", "current"} {
		t.Run(compare, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s.repository = historicalCaptureCancelRepository{Repository: original, after: cancel}
			out, err := s.HistoricalOverview(ctx, "2026-08-01", compare)
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, HistoricalOverviewResult{}) {
				t.Fatalf("abandoned capture returned partial data: %+v %v", out, err)
			}
			s.repository = original
			final, err := s.HistoricalOverview(t.Context(), "2026-08-01", compare)
			if err != nil {
				t.Fatal(err)
			}
			wantOverviewAmount(t, final.Left.Assets, "0.0001")
			if compare != "" {
				wantOverviewAmount(t, final.Right.Assets, "0.0001")
				wantOverviewAmount(t, final.Change.NetWorth, "0")
			}
		})
	}
}

func TestHistoricalOverviewCancelledCurrentSide(t *testing.T) {
	t.Parallel()
	s, _, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Synthetic current", "bank_account", "asset", "balance", "CNY", "1")
	origin, err := s.StartHistory(t.Context(), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	batch, err := s.repository.LoadHistoricalSnapshotBatch(t.Context(), origin.HouseholdID, *now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	state, rows, err := s.historicalOverviewSide(ctx, &batch, "current", *now, true, s.FXProviderKey(), s.QuoteCacheTTL())
	if !errors.Is(err, context.Canceled) || len(rows) != 0 || !reflect.DeepEqual(state, HistoricalOverviewState{}) {
		t.Fatalf("cancelled current side returned partial data: %+v %v", state, err)
	}
}

func TestHistoricalOverviewCancellationWhileWaitingForConnection(t *testing.T) {
	t.Parallel()
	s, db, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Synthetic queued read", "bank_account", "asset", "balance", "CNY", "7.0001")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	held, err := db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback()
	before := db.SQL.Stats().WaitCount
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		out, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
		if !reflect.DeepEqual(out, HistoricalOverviewResult{}) {
			err = errors.New("queued cancelled request returned partial data")
		}
		done <- err
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for db.SQL.Stats().WaitCount == before {
		select {
		case <-ticker.C:
		case <-timeout.C:
			t.Fatal("request did not wait for the occupied connection")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-timeout.C:
		t.Fatal("obsolete queued request waited for the original transaction")
	}
	if db.SQL.Stats().InUse != 1 {
		t.Fatal("cancelling a waiter closed another request's transaction")
	}
	if err := held.Rollback(); err != nil {
		t.Fatal(err)
	}
	final, err := s.HistoricalOverview(t.Context(), "2026-08-02", "")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, final.Left.NetWorth, "7.0001")
}
