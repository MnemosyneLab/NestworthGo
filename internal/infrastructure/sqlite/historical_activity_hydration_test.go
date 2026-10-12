package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type historicalHydrationProbe struct {
	queryer
	calls, maxArgs int
	cancel         context.CancelFunc
	afterChunk     func() error
}

func (q *historicalHydrationProbe) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	q.calls++
	q.maxArgs = max(q.maxArgs, len(args))
	rows, err := q.queryer.QueryContext(ctx, statement, args...)
	if q.calls == 5 && q.afterChunk != nil {
		if hookErr := q.afterChunk(); hookErr != nil {
			if rows != nil {
				rows.Close()
			}
			return nil, hookErr
		}
	}
	if q.calls == 5 && q.cancel != nil {
		q.cancel()
	}
	return rows, err
}

func TestHistoricalActivityHydrationRetainsOneTransactionAcrossChunks(t *testing.T) {
	t.Parallel()
	db, _, household, account, _ := seedPortfolioRepository(t)
	activities := make([]domain.Activity, 257)
	var lastEffect domain.ActivityEffectID
	seed, err := db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Rollback()
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for i := range activities {
		activities[i].ID = domain.NewActivityID()
		fx := domain.NewActivityEffectID()
		lastEffect = fx
		if _, err := seed.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'cash_in','other',?,'2026-08-23',?)`, activities[i].ID.String(), household.ID.String(), at, at); err != nil {
			t.Fatal(err)
		}
		if _, err := seed.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'principal','added','account_cash','income',?,'1','CNY')`, fx.String(), activities[i].ID.String(), account.ID.String()); err != nil {
			t.Fatal(err)
		}
		if _, err := seed.Exec(`INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'1','CNY',?,?,?,'event')`, domain.NewAccountCashValueID().String(), account.ID.String(), at, at, fx.String()); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}
	writer, err := Open(db.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	read, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer read.Rollback()
	probe := &historicalHydrationProbe{queryer: read, afterChunk: func() error {
		// Simulate a concurrent recomputation of a derived event projection,
		// leaving immutable ledger evidence untouched.
		_, err := writer.SQL.Exec(`UPDATE account_cash_values SET amount='2' WHERE activity_effect_id=?`, lastEffect.String())
		return err
	}}
	if err := hydrateHistoricalActivities(t.Context(), probe, activities); err != nil {
		t.Fatal(err)
	}
	if len(activities[256].Resulting) != 1 || activities[256].Resulting[0].Amount != "1" {
		t.Fatalf("second hydration chunk saw concurrent writer: %+v", activities[256].Resulting)
	}
	if err := read.Commit(); err != nil {
		t.Fatal(err)
	}
	var latest string
	if err := db.SQL.QueryRow(`SELECT amount FROM account_cash_values WHERE activity_effect_id=?`, lastEffect.String()).Scan(&latest); err != nil || latest != "2" {
		t.Fatalf("concurrent synthetic writer not committed: %q %v", latest, err)
	}
}

func TestHistoricalActivityHydrationBoundsAndCancellation(t *testing.T) {
	t.Parallel()
	db, err := Open(filepath.Join(t.TempDir(), "historical-hydration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	activities := make([]domain.Activity, 513)
	expectedIDs := make([]domain.ActivityID, len(activities))
	for i := range activities {
		activities[i].ID = domain.NewActivityID()
		expectedIDs[i] = activities[i].ID
	}
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		tx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		probe := &historicalHydrationProbe{queryer: tx}
		if cancelled {
			probe.cancel = cancel
		}
		err = hydrateHistoricalActivities(ctx, probe, activities)
		cancel()
		tx.Rollback()
		if probe.maxArgs > 1024 {
			t.Fatalf("unbounded historical SQL parameters: %d", probe.maxArgs)
		}
		if cancelled {
			if !errors.Is(err, context.Canceled) || probe.calls != 5 {
				t.Fatalf("continued hydration after cancellation: calls=%d err=%v", probe.calls, err)
			}
		} else if err != nil || probe.calls != 15 {
			t.Fatalf("expected five child queries per each of three chunks: calls=%d err=%v", probe.calls, err)
		}
		for i, a := range activities {
			if a.ID != expectedIDs[i] || len(a.Effects) != 0 || len(a.Resulting) != 0 {
				t.Fatal("hydration changed empty-source order/evidence")
			}
		}
	}
}

func TestStandaloneActivitiesUntilSnapshotAndTiedOrdering(t *testing.T) {
	t.Parallel()
	db, repository, household, account, _ := seedPortfolioRepository(t)
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	seed, err := db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Rollback()
	const count = 257
	for i := count; i >= 1; i-- {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		fx := domain.NewActivityEffectID()
		created := at
		if i > 128 {
			created = at.Add(time.Second)
		}
		if _, err := seed.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at,note) VALUES(?,?,'cash_in','other',?,'2026-08-23',?,'1')`, id, household.ID.String(), formatTimestamp(at), formatTimestamp(created)); err != nil {
			t.Fatal(err)
		}
		if _, err := seed.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'principal','added','account_cash','income',?,'1','CNY')`, fx.String(), id, account.ID.String()); err != nil {
			t.Fatal(err)
		}
		if _, err := seed.Exec(`INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'1','CNY',?,?,?,'event')`, domain.NewAccountCashValueID().String(), account.ID.String(), formatTimestamp(at), formatTimestamp(created), fx.String()); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}
	writer, err := Open(db.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	stop, done, started := make(chan struct{}), make(chan error, 1), make(chan struct{})
	go func() {
		for version := 2; ; version++ {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			tx, err := writer.SQL.BeginTx(t.Context(), nil)
			if err != nil {
				done <- err
				return
			}
			marker := fmt.Sprint(version)
			_, err = tx.Exec(`UPDATE activities SET note=? WHERE household_id=?`, marker, household.ID.String())
			if err == nil {
				_, err = tx.Exec(`UPDATE account_cash_values SET amount=? WHERE projection_kind='event'`, marker)
			}
			if err == nil {
				err = tx.Commit()
			} else {
				tx.Rollback()
			}
			if err != nil {
				done <- err
				return
			}
			if version == 2 {
				close(started)
			}
		}
	}()
	defer func() {
		close(stop)
		if err := <-done; err != nil {
			t.Errorf("synthetic concurrent writer: %v", err)
		}
	}()
	select {
	case <-started:
	case err := <-done:
		done <- err
		t.Fatalf("writer start: %v", err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	for read := 0; read < 8; read++ {
		out, err := repository.ListActivitiesUntil(t.Context(), household.ID, at.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != count {
			t.Fatalf("lost tied activities: %d", len(out))
		}
		marker := *out[0].Note
		for i, a := range out {
			wantID := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)
			if a.ID.String() != wantID || a.Note == nil || *a.Note != marker || len(a.Resulting) != 1 || a.Resulting[0].Amount != marker {
				t.Fatalf("standalone read mixed snapshots or tie order at %d: id=%s want=%s marker=%s note=%v resulting=%+v", i, a.ID, wantID, marker, a.Note, a.Resulting)
			}
		}
	}
}
